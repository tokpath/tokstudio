package catalog

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
)

var ErrChangeNotice = errors.New("catalog change requires an actual advance notification plan")

// ErrEffectiveTimeRequired means an existing service change has no operator-chosen time.
// A five-minute buffer is only a suggestion and is not validated here.
var ErrEffectiveTimeRequired = errors.New("catalog change requires an operator chosen effective time")

type CatalogChange struct {
	ID              string          `json:"id"`
	OperatorID      string          `json:"operator_id"`
	Kind            string          `json:"kind"`
	ResourceID      string          `json:"resource_id"`
	Payload         json.RawMessage `gorm:"type:jsonb" json:"payload"`
	Fingerprint     string          `json:"-"`
	EffectiveAt     time.Time       `json:"effective_at"`
	LeadSeconds     int64           `json:"lead_seconds"`
	ExpectedVersion uint64          `json:"expected_version"`
	State           string          `json:"state"`
	Evidence        string          `json:"evidence"`
	CreatedAt       time.Time       `json:"created_at"`
	AnnouncedAt     *time.Time      `json:"announced_at,omitempty"`
	AppliedAt       *time.Time      `json:"applied_at,omitempty"`
	ErrorCode       string          `json:"error_code,omitempty"`
}

func (CatalogChange) TableName() string { return "tokenhub_catalog_changes" }

type CatalogNoticeAudience interface {
	CatalogNotificationOperators(context.Context, *gorm.DB, string) ([]string, error)
	CatalogNotificationAudience(context.Context, *gorm.DB, string, string, int) ([]string, error)
}
type CatalogNoticeSink interface {
	RecordCatalogNotice(context.Context, *gorm.DB, CatalogChange, string, string) error
}
type ChangeController struct {
	Catalog          CommerceCatalog
	Audience         CatalogNoticeAudience
	Notices          CatalogNoticeSink
	ValidateCoverage func(context.Context, *gorm.DB, PriceVersion) error
	// ValidateChange evaluates actual outstanding obligations after a model,
	// listing or authorization change, in the same transaction as that change.
	ValidateChange func(context.Context, *gorm.DB, CatalogChange) error
	Now            func() time.Time
}
type catalogApplyKey struct{}

func applying(ctx context.Context) bool { v, _ := ctx.Value(catalogApplyKey{}).(bool); return v }
func changeFingerprint(kind, resource string, payload json.RawMessage) string {
	var value any
	_ = json.Unmarshal(payload, &value)
	b, _ := json.Marshal([]any{kind, resource, value})
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func (c ChangeController) schedule(ctx context.Context, actor, kind, resource, evidence string, payload any, expected uint64, effective time.Time, scope PriceScope) (CatalogChange, error) {
	out := CatalogChange{}
	if c.Audience == nil || c.Notices == nil {
		return out, ErrChangeNotice
	}
	if effective.IsZero() {
		return out, ErrEffectiveTimeRequired
	}
	if evidence == "" {
		evidence = "system:catalog-publication"
	}
	raw, e := json.Marshal(payload)
	if e != nil {
		return out, e
	}
	e = c.Catalog.mutate(ctx, func(tx CommerceCatalog) error {
		now := c.now()
		affected := []string{actor}
		if kind == "price" && scope.Kind == "supply" {
			affected = []string{scope.BuyerID}
		}
		if kind == "model" || kind == "authorization" || kind == "price" && scope.Kind == "supply" {
			start := actor
			if scope.Kind == "supply" || kind == "authorization" {
				start = scope.BuyerID
			}
			affected, e = c.Audience.CatalogNotificationOperators(ctx, tx.DB, start)
			if e != nil {
				return e
			}
		}
		out = CatalogChange{ID: uuid.NewString(), OperatorID: actor, Kind: kind, ResourceID: resource, Payload: raw, Fingerprint: changeFingerprint(kind, resource, raw), EffectiveAt: effective.UTC(), ExpectedVersion: expected, State: "announcing", Evidence: evidence, CreatedAt: now}
		if kind != "price" {
			previous := tx.DB.WithContext(ctx).Model(&CatalogChange{}).Where("operator_id=? AND kind=? AND resource_id=? AND state IN ('announcing','announced')", actor, kind, resource)
			if kind == "authorization" {
				previous = previous.Where("payload->>'buyer_id'=?", scope.BuyerID)
			}
			if e := previous.Updates(map[string]any{"state": "superseded", "error_code": ""}).Error; e != nil {
				return e
			}
		}
		if e = tx.DB.WithContext(ctx).Create(&out).Error; e != nil {
			return e
		}
		for _, operator := range affected {
			if e = tx.DB.Table("tokenhub_catalog_change_audiences").Create(map[string]any{"change_id": out.ID, "operator_id": operator}).Error; e != nil {
				return e
			}
		}
		return nil
	})
	if e != nil {
		return out, e
	}
	return c.finishNotice(ctx, out)
}

// finishNotice sends the saved plan's notices before the request returns.
// The operator's effective time is kept even when it is soon or already due.
func (c ChangeController) finishNotice(ctx context.Context, change CatalogChange) (CatalogChange, error) {
	for i := 0; i < 1000 && change.State == "announcing"; i++ {
		if _, e := c.Run(ctx, 20); e != nil {
			return change, e
		}
		if e := c.Catalog.DB.WithContext(ctx).First(&change, "id=?", change.ID).Error; e != nil {
			return change, e
		}
	}
	if change.State == "announcing" {
		return change, ErrChangeNotice
	}
	return change, nil
}
func (c ChangeController) SchedulePrice(ctx context.Context, actor string, v PriceVersion, evidence string) (CatalogChange, error) {
	if v.Scope.SellerID != actor {
		return CatalogChange{}, ErrCatalogForbidden
	}
	if v.Scope.Kind == "supply" {
		if c.Catalog.Operators == nil {
			return CatalogChange{}, ErrCatalogForbidden
		}
		buyer, e := c.Catalog.Operators.CatalogOperator(ctx, v.Scope.BuyerID)
		if e != nil || buyer.ParentID != actor {
			return CatalogChange{}, ErrCatalogForbidden
		}
	}
	if v.EffectiveAt.IsZero() {
		return CatalogChange{}, ErrEffectiveTimeRequired
	}
	if e := ValidatePriceVersion(v); e != nil {
		return CatalogChange{}, e
	}
	v.Published = true
	v.NoticeID = ""
	return c.schedule(ctx, actor, "price", v.ModelID, evidence, v, 0, v.EffectiveAt, v.Scope)
}

func (c ChangeController) SchedulePublicModel(ctx context.Context, actor string, m PublicModel, expected uint64, effective time.Time, evidence string) (CatalogChange, error) {
	if !c.Catalog.platform(actor) {
		return CatalogChange{}, ErrCatalogForbidden
	}
	return c.schedule(ctx, actor, "model", m.ID, evidence, m, expected, effective, PriceScope{})
}
func (s CommerceCatalog) CatalogChanges(ctx context.Context, operator string) ([]CatalogChange, error) {
	out := []CatalogChange{}
	e := s.DB.WithContext(ctx).Where("operator_id=?", operator).Order("created_at DESC,id").Limit(100).Find(&out).Error
	return out, e
}
func (s CommerceCatalog) requirePriceNotice(ctx context.Context, v PriceVersion) error {
	var change CatalogChange
	if e := s.DB.WithContext(ctx).First(&change, "id=? AND kind='price' AND state IN ('announced','applied')", v.NoticeID).Error; e != nil {
		return ErrChangeNotice
	}
	candidate := v
	candidate.NoticeID = ""
	raw, _ := json.Marshal(candidate)
	if change.Fingerprint != changeFingerprint("price", v.ModelID, raw) || change.AnnouncedAt == nil {
		return ErrChangeNotice
	}
	return nil
}

// Run completes notice fanout, then applies a change once the operator's time
// has arrived. That time is not postponed when notices take longer than expected.
// Accepted request snapshots are untouched.
func (c ChangeController) Run(ctx context.Context, limit int) ([]CatalogChange, error) {
	if c.Audience == nil || c.Notices == nil {
		return nil, ErrChangeNotice
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	rows := []CatalogChange{}
	if e := c.Catalog.DB.WithContext(ctx).Where("state IN ('announcing','announced')").Order("last_checked_at,id").Limit(limit).Find(&rows).Error; e != nil {
		return nil, e
	}
	for i := range rows {
		change := &rows[i]
		if e := c.Catalog.DB.WithContext(ctx).Model(&CatalogChange{}).Where("id=?", change.ID).Update("last_checked_at", time.Now().UTC()).Error; e != nil {
			return rows, e
		}
		if change.State == "announcing" {
			e := c.Catalog.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
				var current CatalogChange
				if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&current, "id=?", change.ID).Error; e != nil {
					return e
				}
				if current.State != "announcing" {
					*change = current
					return nil
				}
				var audiences []struct {
					OperatorID, Cursor string
					Done               bool
				}
				if e := tx.Table("tokenhub_catalog_change_audiences").Where("change_id=? AND NOT done", change.ID).Order("operator_id").Limit(1).Find(&audiences).Error; e != nil {
					return e
				}
				for _, audience := range audiences {
					ids, e := c.Audience.CatalogNotificationAudience(ctx, tx, audience.OperatorID, audience.Cursor, 200)
					if timed, ok := c.Audience.(interface {
						CatalogNotificationAudienceAt(context.Context, *gorm.DB, string, string, time.Time, int) ([]string, error)
					}); ok {
						ids, e = timed.CatalogNotificationAudienceAt(ctx, tx, audience.OperatorID, audience.Cursor, current.CreatedAt, 200)
					}
					if e != nil {
						return e
					}
					for _, id := range ids {
						if e = c.Notices.RecordCatalogNotice(ctx, tx, current, audience.OperatorID, id); e != nil {
							return e
						}
						if e = tx.Table("tokenhub_catalog_change_notices").Clauses(clause.OnConflict{DoNothing: true}).Create(map[string]any{"change_id": current.ID, "operator_id": audience.OperatorID, "recipient_id": id}).Error; e != nil {
							return e
						}
					}
					cursor := audience.Cursor
					if len(ids) > 0 {
						cursor = ids[len(ids)-1]
					}
					if e = tx.Table("tokenhub_catalog_change_audiences").Where("change_id=? AND operator_id=?", current.ID, audience.OperatorID).Updates(map[string]any{"cursor": cursor, "done": len(ids) < 200}).Error; e != nil {
						return e
					}
				}
				var remaining int64
				if e := tx.Table("tokenhub_catalog_change_audiences").Where("change_id=? AND NOT done", current.ID).Count(&remaining).Error; e != nil {
					return e
				}
				if remaining == 0 {
					now := c.now().Truncate(time.Microsecond)
					state := "announced"
					code := ""
					if current.Kind == "emergency_model_disable" {
						state = "applied"
					}
					if e := tx.Model(&current).Updates(map[string]any{"state": state, "announced_at": now, "error_code": code}).Error; e != nil {
						return e
					}
					current.State = state
					current.AnnouncedAt = &now
					current.ErrorCode = code
				}
				*change = current
				return nil
			})
			if e != nil {
				return rows, e
			}
		}
		if change.State == "announced" {
			if change.Kind != "price" && c.now().Before(change.EffectiveAt) {
				continue
			}
			e := c.Catalog.mutate(ctx, func(store CommerceCatalog) error {
				var current CatalogChange
				if e := store.DB.Clauses(clause.Locking{Strength: "UPDATE"}).First(&current, "id=?", change.ID).Error; e != nil {
					return e
				}
				if current.State != "announced" {
					return nil
				}
				if current.Kind != "price" && current.Evidence == "system:catalog-publication" && current.EffectiveAt.Before(c.now()) {
					// Readiness may recover after the original activation time.
					// Check the prices actually in force now, not an obsolete date.
					current.EffectiveAt = c.now().Truncate(time.Microsecond)
				}
				applyCtx := context.WithValue(ctx, catalogApplyKey{}, true)
				switch current.Kind {
				case "price":
					var v PriceVersion
					if json.Unmarshal(current.Payload, &v) != nil {
						return ErrInvalidInput
					}
					v.NoticeID = current.ID
					if e := store.publishPrice(applyCtx, current.OperatorID, v, c.ValidateCoverage); e != nil {
						return e
					}
				case "model":
					var m PublicModel
					if json.Unmarshal(current.Payload, &m) != nil {
						return ErrInvalidInput
					}
					if e := store.savePublicModel(applyCtx, current.OperatorID, m, current.ExpectedVersion); e != nil {
						return e
					}
				case "listing", "authorization":
					var flag catalogFlagChange
					if json.Unmarshal(current.Payload, &flag) != nil {
						return ErrInvalidInput
					}
					var e error
					if current.Kind == "listing" {
						e = store.setListing(applyCtx, current.OperatorID, flag.ModelID, flag.Enabled, current.ExpectedVersion)
					} else {
						e = store.setAuthorization(applyCtx, current.OperatorID, flag.BuyerID, flag.ModelID, flag.Enabled, current.ExpectedVersion)
					}
					if e != nil {
						return e
					}
				default:
					return ErrInvalidInput
				}
				if current.Kind != "price" {
					if c.ValidateChange == nil {
						return ErrChangeNotice
					}
					if e := c.ValidateChange(applyCtx, store.DB, current); e != nil {
						return e
					}
				}
				now := c.now()
				return store.DB.Model(&current).Updates(map[string]any{"state": "applied", "applied_at": now, "effective_at": current.EffectiveAt, "error_code": ""}).Error
			})
			if e != nil {
				_ = c.Catalog.DB.WithContext(ctx).Model(&CatalogChange{}).Where("id=?", change.ID).Update("error_code", "application_requires_review").Error
				change.ErrorCode = "application_requires_review"
			} else {
				change.State = "applied"
			}
		}
	}
	return rows, nil
}

// EmergencyDisablePublicModel records the emergency before immediately
// disabling acceptance. Its notification fanout remains durable for retry.
func (c ChangeController) EmergencyDisablePublicModel(ctx context.Context, actor, id, evidence string, expected uint64) (CatalogChange, error) {
	out := CatalogChange{}
	if !c.Catalog.platform(actor) || evidence == "" || c.Audience == nil || c.Notices == nil {
		return out, ErrCatalogForbidden
	}
	e := c.Catalog.mutate(ctx, func(tx CommerceCatalog) error {
		m, e := tx.PublicModel(ctx, id)
		if e != nil {
			return e
		}
		if m.Version != expected {
			return ErrCatalogVersion
		}
		m.Enabled = false
		applyCtx := context.WithValue(ctx, catalogApplyKey{}, true)
		if e = tx.savePublicModel(applyCtx, actor, m, expected); e != nil {
			return e
		}
		now := c.now()
		raw, _ := json.Marshal(m)
		out = CatalogChange{ID: uuid.NewString(), OperatorID: actor, Kind: "emergency_model_disable", ResourceID: id, Payload: raw, Fingerprint: changeFingerprint("emergency_model_disable", id, raw), EffectiveAt: now, ExpectedVersion: expected, State: "announcing", Evidence: evidence, CreatedAt: now, AppliedAt: &now}
		if e = tx.DB.Create(&out).Error; e != nil {
			return e
		}
		operators, e := c.Audience.CatalogNotificationOperators(ctx, tx.DB, actor)
		if e != nil {
			return e
		}
		for _, operator := range operators {
			if e = tx.DB.Table("tokenhub_catalog_change_audiences").Create(map[string]any{"change_id": out.ID, "operator_id": operator}).Error; e != nil {
				return e
			}
		}
		return nil
	})
	return out, e
}

type catalogFlagChange struct {
	ModelID string `json:"model_id"`
	BuyerID string `json:"buyer_id,omitempty"`
	Enabled bool   `json:"enabled"`
}

func (c ChangeController) ScheduleListing(ctx context.Context, actor, model string, enabled bool, expected uint64, effective time.Time, evidence string) (CatalogChange, error) {
	if enabled {
		if e := c.Catalog.authorized(ctx, actor, model); e != nil {
			return CatalogChange{}, e
		}
	}
	return c.schedule(ctx, actor, "listing", model, evidence, catalogFlagChange{ModelID: model, Enabled: enabled}, expected, effective, PriceScope{})
}
func (c ChangeController) ScheduleAuthorization(ctx context.Context, actor, buyer, model string, enabled bool, expected uint64, effective time.Time, evidence string) (CatalogChange, error) {
	if c.Catalog.Operators == nil {
		return CatalogChange{}, ErrCatalogForbidden
	}
	child, e := c.Catalog.Operators.CatalogOperator(ctx, buyer)
	if e != nil || child.ParentID != actor {
		return CatalogChange{}, ErrCatalogForbidden
	}
	return c.schedule(ctx, actor, "authorization", model, evidence, catalogFlagChange{ModelID: model, BuyerID: buyer, Enabled: enabled}, expected, effective, PriceScope{Kind: "supply", SellerID: actor, BuyerID: buyer})
}

func (c ChangeController) now() time.Time {
	if c.Now != nil {
		return c.Now().UTC()
	}
	return time.Now().UTC()
}
