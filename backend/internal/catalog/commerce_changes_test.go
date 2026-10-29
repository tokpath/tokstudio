package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"testing"
	"time"
)

type testNoticeAudience struct{}

func TestPostgresPriceRevisionKeepsTheOperatorEffectiveTime(t *testing.T) {
	s := newCommerceTestStore(t)
	seedCommerceModel(t, s)
	c, now, sink := testController(t, s, "platform")
	scope := PriceScope{Kind: "retail", SellerID: "platform"}
	base := priceAt("base", scope, now.Add(-time.Hour), "1")
	if err := s.PublishPrice(context.Background(), "platform", base, c.ValidateCoverage); err != nil {
		t.Fatal(err)
	}
	missing := priceAt(uuid.NewString(), scope, time.Time{}, "2")
	if _, err := c.SchedulePrice(context.Background(), "platform", missing, ""); !errors.Is(err, ErrEffectiveTimeRequired) {
		t.Fatal("missing effective time was invented", err)
	}
	chosen := now.Add(30 * time.Second).Truncate(time.Microsecond)
	next := priceAt(uuid.NewString(), scope, chosen, "2")
	change, err := c.SchedulePrice(context.Background(), "platform", next, "")
	if err != nil || !change.EffectiveAt.Equal(chosen.UTC()) || change.AnnouncedAt == nil {
		t.Fatal("revision did not keep the chosen time and notify immediately", change, err)
	}
	var notices int64
	if err = s.DB.Table("test_catalog_announcements").Where("change_id=?", change.ID).Count(&notices).Error; err != nil || notices != 1 {
		t.Fatal("notice was not sent on submit", notices, err)
	}
	current, err := (PriceResolver{Source: s}).Resolve(context.Background(), scope, "model", *now)
	if err != nil || current.Lineage[0].VersionID != "base" {
		t.Fatal("future price replaced the current price", current, err)
	}
	*now = now.Add(10 * time.Minute)
	if _, err = c.Run(context.Background(), 20); err != nil {
		t.Fatal(err)
	}
	var stored CatalogChange
	if err = s.DB.First(&stored, "id=?", change.ID).Error; err != nil || !stored.EffectiveAt.Equal(chosen.UTC()) {
		t.Fatal("worker moved the operator's effective time", stored, err)
	}
	current, err = (PriceResolver{Source: s}).Resolve(context.Background(), scope, "model", chosen)
	if err != nil || current.Lineage[0].VersionID != next.ID {
		t.Fatal("price did not start at the chosen time", current, err)
	}
	if sink == nil {
		t.Fatal("notice sink missing")
	}
}

func (testNoticeAudience) CatalogNotificationOperators(_ context.Context, _ *gorm.DB, operator string) ([]string, error) {
	return []string{operator}, nil
}
func (testNoticeAudience) CatalogNotificationAudience(_ context.Context, _ *gorm.DB, _, cursor string, _ int) ([]string, error) {
	if cursor != "" {
		return []string{}, nil
	}
	return []string{"test-recipient"}, nil
}

type testCatalogNoticeSink struct{ fail bool }

func (s *testCatalogNoticeSink) RecordCatalogNotice(ctx context.Context, tx *gorm.DB, c CatalogChange, operator, recipient string) error {
	if s.fail {
		return errors.New("synthetic notification transaction failure")
	}
	return tx.WithContext(ctx).Table("test_catalog_announcements").Clauses(clause.OnConflict{DoNothing: true}).Create(map[string]any{"change_id": c.ID, "operator_id": operator, "recipient_id": recipient}).Error
}
func testController(t *testing.T, s CommerceCatalog, actor string) (ChangeController, *time.Time, *testCatalogNoticeSink) {
	t.Helper()
	if e := s.DB.Exec(`CREATE TABLE IF NOT EXISTS test_catalog_announcements(change_id text,operator_id text,recipient_id text,PRIMARY KEY(change_id,operator_id,recipient_id))`).Error; e != nil {
		t.Fatal(e)
	}
	now := time.Now().UTC()
	sink := &testCatalogNoticeSink{}
	c := ChangeController{Catalog: s, Audience: testNoticeAudience{}, Notices: sink, Now: func() time.Time { return now }, ValidateCoverage: func(context.Context, *gorm.DB, PriceVersion) error { return nil }, ValidateChange: func(context.Context, *gorm.DB, CatalogChange) error { return nil }}
	return c, &now, sink
}
func applyTestAuthorizationChange(t *testing.T, s CommerceCatalog, actor, buyer, model string, enabled bool, expected uint64) {
	t.Helper()
	c, now, _ := testController(t, s, actor)
	effective := now.Add(3 * time.Minute)
	change, e := c.ScheduleAuthorization(context.Background(), actor, buyer, model, enabled, expected, effective, "accepted planned grant change")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = c.Run(context.Background(), 20); e != nil {
		t.Fatal(e)
	}
	*now = effective
	if _, e = c.Run(context.Background(), 20); e != nil {
		t.Fatal(e)
	}
	var final CatalogChange
	if e = s.DB.First(&final, "id=?", change.ID).Error; e != nil || final.State != "applied" {
		t.Fatalf("scheduled authorization state=%s %v", final.State, e)
	}
}
func TestPostgresCatalogPriceRequiresActualAdvanceNotice(t *testing.T) {
	s := newCommerceTestStore(t)
	seedCommerceModel(t, s)
	c, now, sink := testController(t, s, "platform")
	// Keep a sub-microsecond remainder so the database/JSON precision boundary
	// is exercised on every platform instead of depending on the host clock.
	*now = now.Truncate(time.Second).Add(123456789 * time.Nanosecond)
	scope := PriceScope{Kind: "retail", SellerID: "platform"}
	base := priceAt("base", scope, now.Add(-time.Hour), "0.001")
	base.Published = true
	base.NoticeID = "first-product"
	if e := s.PublishPrice(context.Background(), "platform", base, c.ValidateCoverage); e != nil {
		t.Fatal(e)
	}
	next := priceAt(uuid.NewString(), scope, now.Add(3*time.Minute), "0.002")
	next.Published = true
	next.NoticeID = "invented-notice"
	if e := s.PublishPrice(context.Background(), "platform", next, c.ValidateCoverage); !errors.Is(e, ErrChangeNotice) {
		t.Fatal("arbitrary notice string accepted as real notification")
	}
	sink.fail = true
	change, e := c.SchedulePrice(context.Background(), "platform", next, "approved price plan")
	if e == nil {
		t.Fatal("notification transaction failure ignored")
	}
	sink.fail = false
	var count int64
	_ = s.DB.Table("tokenhub_model_price_versions").Where("id=?", next.ID).Count(&count).Error
	if count != 0 {
		t.Fatal("price applied before notification commit")
	}
	sink.fail = false
	if _, e = c.Run(context.Background(), 20); e != nil {
		t.Fatal(e)
	}
	var stored CatalogChange
	if e = s.DB.First(&stored, "id=?", change.ID).Error; e != nil || stored.AnnouncedAt == nil || stored.State != "applied" {
		t.Fatalf("announced future price %+v %v", stored, e)
	}
	versions, e := s.PriceVersions(context.Background(), scope, "model")
	if e != nil {
		t.Fatal(e)
	}
	found := false
	for _, v := range versions {
		if v.ID == next.ID {
			found = true
			if v.NoticeID != change.ID {
				t.Fatal("price lost actual notice reference")
			}
		}
	}
	if !found {
		t.Fatal("announced price missing")
	}
	preview, e := s.PublicChanges(context.Background(), "platform", "model", *now)
	if e != nil || len(preview) != 1 || preview[0].Status != "scheduled" || !preview[0].EffectiveAt.Equal(next.EffectiveAt) || len(preview[0].Prices) != 1 || preview[0].Prices[0].AmountATG != "0.002" {
		t.Fatalf("public future price preview: %+v %v", preview, e)
	}
	private, e := s.PublicChanges(context.Background(), "other", "model", *now)
	if e != nil || len(private) != 0 {
		t.Fatalf("unrelated operator received price plan: %+v %v", private, e)
	}
	old, e := (PriceResolver{Source: s}).Resolve(context.Background(), scope, "model", *now)
	if e != nil || old.Lineage[0].VersionID != "base" {
		t.Fatal("future price applied early", e)
	}
	current, e := (PriceResolver{Source: s}).Resolve(context.Background(), scope, "model", next.EffectiveAt)
	if e != nil || current.Lineage[0].VersionID != next.ID {
		t.Fatal("price did not become effective at announced time", e)
	}
}

func TestPostgresModelChangeRollsBackWhenCoverageCannotBeVerified(t *testing.T) {
	s := newCommerceTestStore(t)
	seedCommerceModel(t, s)
	c, now, _ := testController(t, s, "platform")
	m, e := s.PublicModel(context.Background(), "model")
	if e != nil {
		t.Fatal(e)
	}
	m.Enabled = false
	effective := now.Add(3 * time.Minute)
	change, e := c.SchedulePublicModel(context.Background(), "platform", m, m.Version, effective, "change requiring responsibility evaluation")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = c.Run(context.Background(), 20); e != nil {
		t.Fatal(e)
	}
	*now = effective
	c.ValidateChange = nil
	if _, e = c.Run(context.Background(), 20); e != nil {
		t.Fatal(e)
	}
	unchanged, e := s.PublicModel(context.Background(), "model")
	if e != nil || !unchanged.Enabled {
		t.Fatal("unverified change became effective", e)
	}
	var stored CatalogChange
	if e = s.DB.First(&stored, "id=?", change.ID).Error; e != nil || stored.State != "announced" || stored.ErrorCode != "application_requires_review" {
		t.Fatalf("missing durable review state: %+v %v", stored, e)
	}
	c.ValidateChange = func(_ context.Context, tx *gorm.DB, candidate CatalogChange) error {
		var row catalogJSONRow
		if e := tx.Table("tokenhub_public_models").Where("id=?", candidate.ResourceID).Take(&row).Error; e != nil {
			return e
		}
		var applied PublicModel
		if e := json.Unmarshal(row.Definition, &applied); e != nil {
			return e
		}
		if applied.Enabled {
			return errors.New("coverage validator did not see candidate state")
		}
		return nil
	}
	if _, e = c.Run(context.Background(), 20); e != nil {
		t.Fatal(e)
	}
	applied, e := s.PublicModel(context.Background(), "model")
	if e != nil || applied.Enabled {
		t.Fatal("verified candidate did not apply", e)
	}
}
func TestPostgresLateNoticeBlocksChangeAndEmergencyStopsImmediately(t *testing.T) {
	s := newCommerceTestStore(t)
	seedCommerceModel(t, s)
	if e := s.SetListing(context.Background(), "platform", "model", true, 0); e != nil {
		t.Fatal(e)
	}
	c, now, _ := testController(t, s, "platform")
	m, e := s.PublicModel(context.Background(), "model")
	if e != nil {
		t.Fatal(e)
	}
	m.Enabled = false
	if e = s.SavePublicModel(context.Background(), "platform", m, m.Version); !errors.Is(e, ErrChangeNotice) {
		t.Fatal("ordinary model change bypassed plan")
	}
	chosen := now.Add(30 * time.Second).Truncate(time.Microsecond)
	change, e := c.SchedulePublicModel(context.Background(), "platform", m, m.Version, chosen, "planned model metadata change")
	if e != nil || !change.EffectiveAt.Equal(chosen.UTC()) || change.State != "announced" {
		t.Fatal("soon effective time was rejected or notices waited", change, e)
	}
	still, _ := s.PublicModel(context.Background(), "model")
	if !still.Enabled {
		t.Fatal("model changed before the chosen time")
	}
	*now = chosen
	if _, e = c.Run(context.Background(), 20); e != nil {
		t.Fatal(e)
	}
	updated, _ := s.PublicModel(context.Background(), "model")
	if updated.Enabled {
		t.Fatal("model stayed enabled after the chosen time")
	}
	still = updated
	emergency, e := c.EmergencyDisablePublicModel(context.Background(), "platform", "model", "verified emergency reference", still.Version)
	if e != nil {
		t.Fatal(e)
	}
	disabled, _ := s.PublicModel(context.Background(), "model")
	if disabled.Enabled || emergency.AppliedAt == nil {
		t.Fatal("emergency did not immediately stop new requests")
	}
	if _, e = c.Run(context.Background(), 20); e != nil {
		t.Fatal(e)
	}
	var audit CatalogChange
	_ = s.DB.First(&audit, "id=?", emergency.ID).Error
	raw, _ := json.Marshal(audit)
	if audit.State != "applied" || audit.AnnouncedAt == nil || len(raw) == 0 {
		t.Fatal("emergency notice or audit missing")
	}
	public, e := s.PublicCatalog(context.Background(), "platform", *now)
	if e != nil || len(public) != 1 || public[0].Status != "disabled" || len(public[0].UpcomingChanges) == 0 {
		t.Fatalf("emergency state disappeared from authorized public directory: %+v %v", public, e)
	}
	available, e := s.AvailableModels(context.Background(), "platform", true)
	if e != nil || len(available) != 0 {
		t.Fatalf("disabled public model remained callable: %+v %v", available, e)
	}
}
