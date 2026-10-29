package remediation

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/cooperation"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"gorm.io/gorm"
)

func TestPostgresManualGiftUsesCoverageZeroPaidBasisAndTransfersWithoutRewritingOrigin(t *testing.T) {
	brand, channel, _, _ := takeoverFixture(t)
	com := brand.s.Commerce
	var at time.Time
	if e := brand.db.Raw("SELECT clock_timestamp()").Row().Scan(&at); e != nil {
		t.Fatal(e)
	}
	expires := at.Add(24 * time.Hour)
	in := commerce.ManualGiftInput{ActorStaffID: channel.owner, OperatorID: channel.operator, CustomerID: channel.customer, BusinessKey: "support-case-481", Reason: "compensation", Evidence: "approved outage compensation; actual support record 481", AmountATG: d("80"), ExpiresAt: &expires, ExpectedOwnershipVersion: 1}
	wrong := in
	wrong.ActorStaffID = brand.owner
	if _, e := com.IssueManualGift(channel.ctx, nil, wrong); !errors.Is(e, organization.ErrForbidden) {
		t.Fatal("parent staff gained child marketing authority", e)
	}
	// Fail after the grant and coverage transition. The caller must never retain
	// a grant when its independent gift record could not be committed.
	if e := brand.db.Exec(`CREATE FUNCTION test_abort_manual_gift() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected gift persistence failure'; END $$; CREATE TRIGGER test_abort_manual_gift BEFORE INSERT ON commerce_manual_gifts FOR EACH ROW EXECUTE FUNCTION test_abort_manual_gift();`).Error; e != nil {
		t.Fatal(e)
	}
	if _, e := com.IssueManualGift(channel.ctx, nil, in); e == nil {
		t.Fatal("gift storage failure was ignored")
	}
	grants, e := brand.s.Ledger.CustomerGrants(channel.ctx, nil, channel.operator, channel.customer)
	if e != nil || len(grants) != 1 {
		t.Fatal("failed gift leaked ledger credit", grants, e)
	}
	var commitments int64
	if e = brand.db.Table("cooperation_sales_commitments").Where("operator_id=?", channel.operator).Count(&commitments).Error; e != nil || commitments != 1 {
		t.Fatal("failed gift leaked coverage", commitments, e)
	}
	if e = brand.db.Exec("DROP TRIGGER test_abort_manual_gift ON commerce_manual_gifts").Error; e != nil {
		t.Fatal(e)
	}
	gift, e := com.IssueManualGift(channel.ctx, nil, in)
	if e != nil {
		t.Fatal(e)
	}
	replay, e := com.IssueManualGift(channel.ctx, nil, in)
	if e != nil || replay.ID != gift.ID || replay.GrantID != gift.GrantID {
		t.Fatal(gift, replay, e)
	}
	changed := in
	changed.AmountATG = d("81")
	if _, e = com.IssueManualGift(channel.ctx, nil, changed); !errors.Is(e, commerce.ErrConflict) {
		t.Fatal("same command changed gift amount", e)
	}
	// There are precisely 20 ATG left behind 100 package + 80 manual ATG.
	// Two independent commands contend on the real cooperation business lock.
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, key := range []string{"gift-a", "gift-b"} {
		wg.Add(1)
		go func(key string) {
			defer wg.Done()
			next := in
			next.BusinessKey, next.Reason, next.AmountATG = key, "targeted_gift", d("20")
			_, err := com.IssueManualGift(channel.ctx, nil, next)
			results <- err
		}(key)
	}
	wg.Wait()
	close(results)
	ok, refused := 0, 0
	for err := range results {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, cooperation.ErrCoverage):
			refused++
		default:
			t.Fatal(err)
		}
	}
	if ok != 1 || refused != 1 {
		t.Fatalf("coverage race succeeded=%d refused=%d", ok, refused)
	}
	if e = brand.db.Raw("SELECT clock_timestamp()").Row().Scan(&at); e != nil {
		t.Fatal(e)
	}
	coverage, e := brand.s.Cooperation.Coverage(channel.ctx, nil, channel.operator, at)
	if e != nil || !coverage.Space.IsZero() || coverage.Required.Cmp(d("200")) != 0 || !coverage.Pending.IsZero() {
		t.Fatal(coverage, e)
	}
	grants, e = brand.s.Ledger.CustomerGrants(channel.ctx, nil, channel.operator, channel.customer)
	if e != nil || len(grants) != 3 {
		t.Fatal(grants, e)
	}
	var manual ledger.Grant
	for _, g := range grants {
		if g.ID == gift.GrantID {
			manual = g
		}
	}
	if manual.ID == "" || manual.OrderID != "" || !manual.PaidMoney.IsZero() || manual.Total.Cmp(d("80")) != 0 {
		t.Fatal("gift fabricated a paid sale", manual)
	}
	var fakeOrders, giftSources int64
	if e = brand.db.Table("commerce_orders").Where("id=?", gift.ID).Count(&fakeOrders).Error; e != nil || fakeOrders != 0 {
		t.Fatal("gift fabricated a purchase order", fakeOrders, e)
	}
	if e = brand.db.Table("ledger_grant_sources").Where("grant_id=? AND kind='manual_gift' AND quantity=80", gift.GrantID).Count(&giftSources).Error; e != nil || giftSources != 1 {
		t.Fatal(giftSources, e)
	}
	// Earlier expiry makes the gift the FEFO source. Actual gift usage must not
	// increase paid qualification or generate an inviter commission.
	e = brand.db.Transaction(func(tx *gorm.DB) error {
		if err := brand.s.Incentives.LockBusiness(channel.ctx, tx, channel.operator); err != nil {
			return err
		}
		var now time.Time
		if err := tx.Raw("SELECT clock_timestamp()").Row().Scan(&now); err != nil {
			return err
		}
		hold, err := brand.s.Ledger.ReserveCredits(channel.ctx, tx, ledger.CreditReservation{OperatorID: channel.operator, CustomerID: channel.customer, BusinessKey: "gift-only-usage", Estimate: d("10"), At: now, ExpiresAt: now.Add(time.Hour)})
		if err != nil {
			return err
		}
		result, err := brand.s.Ledger.SettleCredits(channel.ctx, tx, channel.operator, channel.customer, hold.ID, "gift-only-usage", d("10"), now)
		if err != nil {
			return err
		}
		if !result.PaidMoney.IsZero() || len(result.Events) != 1 || result.Events[0].GrantID != gift.GrantID {
			t.Fatal("gift usage acquired paid basis", result)
		}
		return brand.s.Incentives.ObserveGrantEvent(channel.ctx, tx, manual, result.Events[0])
	})
	if e != nil {
		t.Fatal(e)
	}
	q, e := brand.s.Incentives.GetQualification(channel.ctx, nil, channel.operator, channel.customer)
	if e != nil || !q.ConfirmedMoney.IsZero() || q.AcquiredAt != nil {
		t.Fatal(q, e)
	}
	if a := channel.balance(t); a.Balance.Cmp(d("25")) != 0 || !a.Held.IsZero() {
		t.Fatal("gift changed cash wallet", a)
	}
	rows, cursor, e := com.ListManualGifts(channel.ctx, nil, channel.owner, channel.operator, channel.customer, "", 1)
	if e != nil || len(rows) != 1 || cursor == "" {
		t.Fatal(rows, cursor, e)
	}
	page, _, e := com.ListManualGifts(channel.ctx, nil, channel.owner, channel.operator, channel.customer, cursor, 1)
	if e != nil || len(page) != 1 || rows[0].ID == page[0].ID {
		t.Fatal(page, e)
	}
	var events int64
	if e = brand.db.Table("commerce_events").Where("aggregate_id=? AND kind='manual_gift.delivered' AND payload->>'actor_id'=? AND payload->>'evidence'=?", gift.ID, channel.owner, in.Evidence).Count(&events).Error; e != nil || events != 1 {
		t.Fatal("gift delivery audit missing or duplicated", events, e)
	}
	if e = brand.db.Model(&commerce.ManualGift{}).Where("id=?", gift.ID).Update("amount_atg", d("999")).Error; e == nil {
		t.Fatal("delivered gift economics are mutable")
	}
	p := beginTakeover(t, brand, channel)
	p, e = brand.s.RefreshTakeover(brand.ctx, brand.owner, brand.operator, p.ID)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = brand.s.ExecuteTakeover(brand.ctx, brand.owner, brand.operator, p.ID, p.SnapshotFingerprint); e != nil {
		t.Fatal(e)
	}
	rows, _, e = com.ListManualGifts(brand.ctx, nil, brand.owner, brand.operator, channel.customer, "", 10)
	if e != nil || len(rows) != 2 {
		t.Fatal(rows, e)
	}
	for _, row := range rows {
		if row.OperatorID != brand.operator || row.OriginalOperatorID != channel.operator || row.OwnershipVersion != 2 || row.OriginalOwnershipVersion != 1 {
			t.Fatal("gift provenance lost on takeover", row)
		}
	}
	if _, _, e = com.ListManualGifts(brand.ctx, nil, channel.owner, brand.operator, channel.customer, "", 10); !errors.Is(e, organization.ErrForbidden) {
		t.Fatal("former channel staff read brand gift records", e)
	}
}
