package cooperation

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/payments"
	appcrypto "github.com/tokpath/tokstudio/backend/internal/platform/crypto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strings"
	"time"
)

var ErrReplenishment = errors.New("automatic procurement authorization is unavailable or exceeded")

type ReplenishmentNotice struct{ BuyerID, RecipientID, AttemptID, OrderID, Kind, Reason string }
type ReplenishmentObserver interface {
	RecordReplenishmentNotice(context.Context, *gorm.DB, ReplenishmentNotice) error
}
type Replenisher struct {
	Cooperation   *Service
	Payments      *payments.Service
	Collector     *commerce.Service
	EncryptionKey string
	Observer      ReplenishmentObserver
}
type ReplenishmentAuthorization struct {
	ID                  string         `json:"id"`
	BuyerID             string         `json:"buyer_id"`
	SupplierID          string         `json:"supplier_id"`
	QuoteID             string         `json:"quote_id"`
	TriggerAvailableATG ledger.Decimal `json:"trigger_available_atg"`
	SingleLimitUSD      ledger.Decimal `json:"single_limit_usd"`
	PeriodLimitUSD      ledger.Decimal `json:"period_limit_usd"`
	Period              ledger.Period  `json:"period"`
	Mode                string         `json:"mode"`
	MerchantID          *string        `json:"merchant_id,omitempty"`
	PaymentCiphertext   string         `json:"-"`
	ConsentReference    string         `json:"consent_reference"`
	Enabled             bool           `json:"enabled"`
	Version             int64          `json:"version"`
	AuthorizedBy        string         `json:"authorized_by"`
	UpdatedAt           time.Time      `json:"updated_at"`
}

func (ReplenishmentAuthorization) TableName() string {
	return "cooperation_replenishment_authorizations"
}

type ReplenishmentPayment struct {
	PaymentMethod string `json:"payment_method"`
	CustomerRef   string `json:"customer_ref"`
}
type ReplenishmentAuthorizationInput struct {
	ReplenishmentAuthorization
	Payment         ReplenishmentPayment `json:"payment"`
	ExpectedVersion int64                `json:"expected_version"`
}
type ReplenishmentAttempt struct {
	ID                   string         `json:"id"`
	BuyerID              string         `json:"buyer_id"`
	AuthorizationID      string         `json:"authorization_id"`
	AuthorizationVersion int64          `json:"authorization_version"`
	PurchaseOrderID      string         `json:"purchase_order_id"`
	SnapshotCiphertext   string         `json:"-"`
	PeriodStart          time.Time      `json:"period_start"`
	PeriodEnd            time.Time      `json:"period_end"`
	AmountUSD            ledger.Decimal `json:"amount_usd"`
	State                string         `json:"state"`
	ProviderID           string         `json:"provider_id,omitempty"`
	ErrorCode            string         `json:"error_code,omitempty"`
	LeaseUntil           *time.Time     `json:"-"`
	CreatedAt            time.Time      `json:"created_at"`
	UpdatedAt            time.Time      `json:"updated_at"`
}

func (ReplenishmentAttempt) TableName() string { return "cooperation_replenishment_attempts" }

type replenishmentSnapshot struct {
	Authorization ReplenishmentAuthorization
	Payment       ReplenishmentPayment
}

func (r *Replenisher) ready() error {
	if r.Cooperation == nil || r.Payments == nil || r.Collector == nil || r.EncryptionKey == "" || r.Observer == nil {
		return ErrReplenishment
	}
	return nil
}
func (r *Replenisher) seal(v any) (string, error) {
	if r.EncryptionKey == "" {
		return "", ErrReplenishment
	}
	b, e := json.Marshal(v)
	if e != nil {
		return "", e
	}
	return appcrypto.Seal(r.EncryptionKey, string(b))
}
func (r *Replenisher) open(s string, v any) error {
	b, e := appcrypto.Open(r.EncryptionKey, s)
	if e != nil {
		return e
	}
	return json.Unmarshal([]byte(b), v)
}
func (r *Replenisher) SaveAuthorization(ctx context.Context, actor string, in ReplenishmentAuthorizationInput) (ReplenishmentAuthorization, error) {
	a := in.ReplenishmentAuthorization
	if e := r.ready(); e != nil {
		return a, e
	}
	if a.TriggerAvailableATG.Sign() < 0 || a.SingleLimitUSD.Sign() <= 0 || a.PeriodLimitUSD.Cmp(a.SingleLimitUSD) < 0 || !a.TriggerAvailableATG.Valid() || !a.SingleLimitUSD.Valid() || !a.PeriodLimitUSD.Valid() || a.ConsentReference == "" || (a.Period != ledger.Month && a.Period != ledger.Quarter && a.Period != ledger.Year) || (a.Mode != "automatic" && a.Mode != "offline") {
		return a, ErrReplenishment
	}
	e := r.Cooperation.within(ctx, nil, func(t *gorm.DB) error {
		if e := r.Cooperation.Organization.RequirePermission(ctx, t, actor, a.BuyerID, "cooperation.manage"); e != nil {
			return e
		}
		if e := r.Cooperation.Lock(ctx, t, a.BuyerID); e != nil {
			return e
		}
		var q PurchaseQuote
		if e := t.First(&q, "id=? AND buyer_id=? AND enabled", a.QuoteID, a.BuyerID).Error; e != nil {
			return e
		}
		if q.PriceUSD.Cmp(a.SingleLimitUSD) > 0 || q.SupplierID != a.SupplierID {
			return ErrReplenishment
		}
		if a.Mode == "automatic" {
			if a.MerchantID == nil || !strings.HasPrefix(in.Payment.PaymentMethod, "pm_") || !strings.HasPrefix(in.Payment.CustomerRef, "cus_") {
				return ErrReplenishment
			}
			m, e := r.Payments.Merchant(ctx, a.SupplierID, *a.MerchantID)
			if e != nil {
				return e
			}
			if m.Driver != "stripe" || !m.Enabled || len(r.Payments.Drivers[m.Driver].Validate(m)) > 0 {
				return ErrReplenishment
			}
		} else {
			a.MerchantID = nil
			in.Payment = ReplenishmentPayment{}
		}
		var old ReplenishmentAuthorization
		found := t.Where("buyer_id=?", a.BuyerID).Limit(1).Find(&old)
		if found.Error != nil {
			return found.Error
		}
		if (found.RowsAffected == 0 && in.ExpectedVersion != 0) || (found.RowsAffected > 0 && old.Version != in.ExpectedVersion) {
			return ErrConflict
		}
		a.ID = old.ID
		if a.ID == "" {
			a.ID = uuid.NewString()
		}
		a.Version = in.ExpectedVersion + 1
		a.AuthorizedBy = actor
		now, clockErr := businessTime(ctx, t)
		if clockErr != nil {
			return clockErr
		}
		a.UpdatedAt = now
		cipher, e := r.seal(in.Payment)
		if e != nil {
			return e
		}
		a.PaymentCiphertext = cipher
		return t.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "buyer_id"}}, UpdateAll: true}).Create(&a).Error
	})
	return a, e
}
func (r *Replenisher) CancelAuthorization(ctx context.Context, actor, buyer string, expected int64) error {
	return r.Cooperation.within(ctx, nil, func(t *gorm.DB) error {
		if e := r.Cooperation.Organization.RequirePermission(ctx, t, actor, buyer, "cooperation.manage"); e != nil {
			return e
		}
		result := t.Model(&ReplenishmentAuthorization{}).Where("buyer_id=? AND version=?", buyer, expected).Updates(map[string]any{"enabled": false, "version": expected + 1, "updated_at": gorm.Expr("clock_timestamp()")})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrConflict
		}
		return nil
	})
}
func (r *Replenisher) List(ctx context.Context, actor, buyer string) ([]ReplenishmentAuthorization, []ReplenishmentAttempt, error) {
	a := []ReplenishmentAuthorization{}
	attempts := []ReplenishmentAttempt{}
	e := r.Cooperation.within(ctx, nil, func(t *gorm.DB) error {
		if e := r.Cooperation.Organization.RequirePermission(ctx, t, actor, buyer, "cooperation.manage"); e != nil {
			return e
		}
		if e := t.Where("buyer_id=?", buyer).Find(&a).Error; e != nil {
			return e
		}
		return t.Where("buyer_id=?", buyer).Order("created_at DESC").Limit(100).Find(&attempts).Error
	})
	return a, attempts, e
}
func (r *Replenisher) prepare(ctx context.Context, a ReplenishmentAuthorization) (*ReplenishmentAttempt, error) {
	var out *ReplenishmentAttempt
	e := r.Cooperation.within(ctx, nil, func(t *gorm.DB) error {
		if e := r.Cooperation.Lock(ctx, t, a.BuyerID); e != nil {
			return e
		}
		if e := t.First(&a, "id=?", a.ID).Error; e != nil {
			return e
		}
		if !a.Enabled {
			return nil
		}
		var active int64
		if e := t.Model(&ReplenishmentAttempt{}).Where("buyer_id=? AND state NOT IN ('completed','cancelled_unpaid','failed_unpaid')", a.BuyerID).Count(&active).Error; e != nil {
			return e
		}
		if active > 0 {
			return nil
		}
		now, e := businessTime(ctx, t)
		if e != nil {
			return e
		}
		coverage, e := r.Cooperation.Coverage(ctx, t, a.BuyerID, now)
		if e != nil {
			return e
		}
		if coverage.ProcurementAvailable.Cmp(a.TriggerAvailableATG) > 0 {
			return nil
		}
		var quote PurchaseQuote
		if e = t.First(&quote, "id=? AND buyer_id=? AND supplier_id=? AND enabled", a.QuoteID, a.BuyerID, a.SupplierID).Error; e != nil {
			return e
		}
		window, e := ledger.BudgetWindow(now, now, a.Period, "UTC")
		if e != nil || window.End == nil {
			return ErrReplenishment
		}
		var committed ledger.Decimal
		if e = t.Model(&ReplenishmentAttempt{}).Where("buyer_id=? AND created_at>=? AND created_at<? AND state NOT IN ('cancelled_unpaid','failed_unpaid')", a.BuyerID, window.Start, *window.End).Select("COALESCE(sum(amount_usd),0)::text").Row().Scan(&committed); e != nil {
			return e
		}
		if quote.PriceUSD.Cmp(a.SingleLimitUSD) > 0 || committed.Add(quote.PriceUSD).Cmp(a.PeriodLimitUSD) > 0 {
			return ErrReplenishment
		}
		id := uuid.NewString()
		order, e := r.Cooperation.CreatePurchaseOrder(ctx, t, a.AuthorizedBy, a.BuyerID, a.QuoteID, "auto:"+id)
		if e != nil {
			return e
		}
		var pay ReplenishmentPayment
		if e = r.open(a.PaymentCiphertext, &pay); e != nil {
			return e
		}
		cipher, e := r.seal(replenishmentSnapshot{a, pay})
		if e != nil {
			return e
		}
		attempt := ReplenishmentAttempt{ID: id, BuyerID: a.BuyerID, AuthorizationID: a.ID, AuthorizationVersion: a.Version, PurchaseOrderID: order.ID, SnapshotCiphertext: cipher, PeriodStart: window.Start, PeriodEnd: *window.End, AmountUSD: quote.PriceUSD, State: "prepared", CreatedAt: now, UpdatedAt: now}
		if a.Mode == "offline" {
			attempt.State = "awaiting_manual_payment"
		}
		if e = t.Create(&attempt).Error; e != nil {
			return e
		}
		if e = r.Observer.RecordReplenishmentNotice(ctx, t, ReplenishmentNotice{BuyerID: a.BuyerID, RecipientID: a.AuthorizedBy, AttemptID: id, OrderID: order.ID, Kind: "created", Reason: attempt.State}); e != nil {
			return e
		}
		out = &attempt
		return nil
	})
	return out, e
}

// Run performs network calls only after the immutable attempt has committed.
// Once submission starts, all future ticks query the original payment; even an
// accepted-but-disconnected create cannot result in a new automatic collection.
func (r *Replenisher) Run(ctx context.Context, limit int) ([]ReplenishmentAttempt, error) {
	if e := r.ready(); e != nil {
		return nil, e
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	authorizations := []ReplenishmentAuthorization{}
	if e := r.Cooperation.DB.WithContext(ctx).Where("enabled").Order("last_checked_at,buyer_id").Limit(limit).Find(&authorizations).Error; e != nil {
		return nil, e
	}
	for _, a := range authorizations {
		if e := r.Cooperation.DB.WithContext(ctx).Model(&ReplenishmentAuthorization{}).Where("id=?", a.ID).Update("last_checked_at", gorm.Expr("clock_timestamp()")).Error; e != nil {
			return nil, e
		}
		if _, e := r.prepare(ctx, a); e != nil {
			if e = r.noticePreparationFailure(ctx, a, e); e != nil {
				return nil, e
			}
		}
	}
	jobs := []ReplenishmentAttempt{}
	e := r.Cooperation.DB.WithContext(ctx).Transaction(func(t *gorm.DB) error {
		if e := t.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).Where("state NOT IN ('completed','cancelled_unpaid','failed_unpaid') AND (lease_until IS NULL OR lease_until<clock_timestamp())").Order("updated_at,id").Limit(limit).Find(&jobs).Error; e != nil {
			return e
		}
		for i := range jobs {
			j := jobs[i]
			updates := map[string]any{"lease_until": gorm.Expr("clock_timestamp()+interval '1 minute'")}
			if j.State == "prepared" {
				var current ReplenishmentAuthorization
				if e := t.First(&current, "id=?", j.AuthorizationID).Error; e != nil {
					return e
				}
				if !current.Enabled || current.Version != j.AuthorizationVersion {
					updates["state"] = "needs_attention"
					updates["error_code"] = "authorization_changed_before_submission"
					jobs[i].State = "needs_attention"
					jobs[i].ErrorCode = "authorization_changed_before_submission"
				} else {
					updates["state"] = "submitting"
				}
			}
			if e := t.Model(&ReplenishmentAttempt{}).Where("id=?", j.ID).Updates(updates).Error; e != nil {
				return e
			}
		}
		return nil
	})
	if e != nil {
		return nil, e
	}
	for i := range jobs {
		job := &jobs[i]
		work, cancel := context.WithTimeout(ctx, 25*time.Second)
		state, provider, code := r.process(work, *job)
		cancel()
		persist, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		e = r.Cooperation.DB.WithContext(persist).Transaction(func(t *gorm.DB) error {
			updates := map[string]any{"state": state, "error_code": code, "lease_until": nil, "updated_at": gorm.Expr("clock_timestamp()")}
			if provider != "" {
				updates["provider_id"] = provider
			}
			if e := t.Model(&ReplenishmentAttempt{}).Where("id=?", job.ID).Updates(updates).Error; e != nil {
				return e
			}
			if code != "" && code != job.ErrorCode {
				var snap replenishmentSnapshot
				if e := r.open(job.SnapshotCiphertext, &snap); e != nil {
					return e
				}
				return r.Observer.RecordReplenishmentNotice(persist, t, ReplenishmentNotice{BuyerID: job.BuyerID, RecipientID: snap.Authorization.AuthorizedBy, AttemptID: job.ID, OrderID: job.PurchaseOrderID, Kind: "pending", Reason: code})
			}
			return nil
		})
		cancel()
		if e != nil {
			return jobs, e
		}
		job.State = state
		job.ErrorCode = code
		if provider != "" {
			job.ProviderID = provider
		}
	}
	return jobs, nil
}
func (r *Replenisher) noticePreparationFailure(ctx context.Context, a ReplenishmentAuthorization, cause error) error {
	return r.Cooperation.within(ctx, nil, func(t *gorm.DB) error {
		now, e := businessTime(ctx, t)
		if e != nil {
			return e
		}
		return r.Observer.RecordReplenishmentNotice(ctx, t, ReplenishmentNotice{BuyerID: a.BuyerID, RecipientID: a.AuthorizedBy, AttemptID: "authorization:" + a.ID + ":" + now.Format("2006-01-02"), Kind: "authorization_blocked", Reason: "quote_or_budget_or_permission_unavailable"})
	})
}
func (r *Replenisher) process(ctx context.Context, j ReplenishmentAttempt) (state, provider, code string) {
	if j.ErrorCode == "authorization_changed_before_submission" {
		return "needs_attention", j.ProviderID, j.ErrorCode
	}
	var snapshot replenishmentSnapshot
	if e := r.open(j.SnapshotCiphertext, &snapshot); e != nil {
		return "needs_attention", j.ProviderID, "authorization_unreadable"
	}
	a := snapshot.Authorization
	var order PurchaseOrder
	if e := r.Cooperation.DB.WithContext(ctx).First(&order, "id=?", j.PurchaseOrderID).Error; e != nil {
		return "needs_attention", j.ProviderID, "order_unavailable"
	}
	if order.Status == "completed" {
		return "completed", j.ProviderID, ""
	}
	if order.Status == "closed" {
		return "cancelled_unpaid", j.ProviderID, ""
	}
	if order.Status == "paid" {
		if _, e := r.Cooperation.FulfillPurchase(ctx, nil, order.ID, "system:verified-procurement"); e != nil {
			return "needs_attention", j.ProviderID, "verified_payment_delivery_pending"
		}
		return "completed", j.ProviderID, ""
	}
	if a.Mode == "automatic" && a.MerchantID != nil {
		if j.State == "prepared" {
			now, clockErr := businessTime(ctx, r.Cooperation.DB)
			if clockErr != nil {
				return "needs_attention", j.ProviderID, "business_clock_unavailable"
			}
			checkout, e := r.Payments.Create(ctx, a.SupplierID, *a.MerchantID, payments.Order{ID: order.ID, OperatorID: a.SupplierID, MerchantID: *a.MerchantID, Money: payments.Money{Currency: "USD", Amount: order.DueUSD}, Valuation: payments.Valuation{ID: "native-usd-procurement:" + j.ID, Currency: "USD", USDPerUnit: ledger.FromInt64(1), Source: "native_usd", EffectiveAt: now.Add(-time.Second), ExpiresAt: now.Add(time.Hour)}, AcceptedAt: now, ExpiresAt: order.ExpiresAt, Description: "Authorized prepaid procurement", PaymentMethod: snapshot.Payment.PaymentMethod, CustomerRef: snapshot.Payment.CustomerRef, AuthorizationID: a.ConsentReference})
			provider = checkout.ProviderID
			if e != nil {
				return "outcome_unknown", provider, "payment_requires_verification"
			}
		} else {
			_, _ = r.Payments.Reconcile(ctx, a.SupplierID, *a.MerchantID, order.ID)
		}
		links, e := r.Payments.ReceiptsForOrder(ctx, a.SupplierID, *a.MerchantID, order.ID)
		if e != nil {
			return "awaiting_payment", provider, "receipt_query_unavailable"
		}
		for _, link := range links {
			if e = r.applyReceipt(ctx, order, link.ReceiptID); e != nil {
				return "needs_attention", provider, "receipt_requires_review"
			}
		}
	} else {
		return "awaiting_manual_payment", j.ProviderID, "manual_payment_required"
	}
	if e := r.Cooperation.DB.WithContext(ctx).First(&order, "id=?", j.PurchaseOrderID).Error; e != nil {
		return "needs_attention", provider, "order_unavailable"
	}
	if order.Status == "completed" {
		return "completed", provider, ""
	}
	return "awaiting_payment", provider, "payment_not_verified"
}
func (r *Replenisher) applyReceipt(ctx context.Context, o PurchaseOrder, receiptID string) error {
	return r.Cooperation.within(ctx, nil, func(t *gorm.DB) error {
		if e := t.Clauses(clause.Locking{Strength: "UPDATE"}).First(&o, "id=?", o.ID).Error; e != nil {
			return e
		}
		if o.Status == "completed" {
			return nil
		}
		receipt, e := r.Collector.GetReceipt(ctx, t, o.SupplierID, receiptID)
		if e != nil {
			return e
		}
		available := receipt.GrossUSD.Sub(receipt.AllocatedUSD)
		need := o.DueUSD.Sub(o.AllocatedUSD)
		if need.IsZero() {
			_, e = r.Cooperation.FulfillPurchase(ctx, t, o.ID, "system:automatic-procurement")
			return e
		}
		if available.Cmp(need) > 0 {
			available = need
		}
		if available.Sign() <= 0 {
			return nil
		}
		o, e = r.Cooperation.ApplyVerifiedPurchaseReceipt(ctx, t, r.Collector, o.ID, receiptID, "auto:"+receiptID, available)
		if e != nil {
			return e
		}
		if o.Status == "paid" {
			_, e = r.Cooperation.FulfillPurchase(ctx, t, o.ID, "system:automatic-procurement")
		}
		return e
	})
}

// SuspendReplenishment is a trusted takeover/exit hook. Already submitted
// attempts retain their original verification responsibility.
func (s *Service) SuspendReplenishment(ctx context.Context, tx *gorm.DB, buyer, evidence string) error {
	if tx == nil || evidence == "" {
		return ErrConflict
	}
	if e := s.Lock(ctx, tx, buyer); e != nil {
		return e
	}
	return tx.WithContext(ctx).Model(&ReplenishmentAuthorization{}).Where("buyer_id=? AND enabled", buyer).Updates(map[string]any{"enabled": false, "version": gorm.Expr("version+1"), "updated_at": gorm.Expr("clock_timestamp()")}).Error
}

type PendingProcurement struct {
	AutomaticAttempts int64    `json:"automatic_attempts"`
	PurchaseOrders    int64    `json:"purchase_orders"`
	Refunds           int64    `json:"refunds"`
	AttemptIDs        []string `json:"attempt_ids"`
	OrderIDs          []string `json:"order_ids"`
}

func (s *Service) PendingProcurement(ctx context.Context, tx *gorm.DB, buyer string) (PendingProcurement, error) {
	out := PendingProcurement{AttemptIDs: []string{}, OrderIDs: []string{}}
	e := s.within(ctx, tx, func(t *gorm.DB) error {
		if e := t.Model(&ReplenishmentAttempt{}).Where("buyer_id=? AND state NOT IN ('completed','cancelled_unpaid','failed_unpaid')", buyer).Order("id").Pluck("id", &out.AttemptIDs).Error; e != nil {
			return e
		}
		if e := t.Model(&PurchaseOrder{}).Where("buyer_id=? AND status NOT IN ('completed','closed')", buyer).Order("id").Pluck("id", &out.OrderIDs).Error; e != nil {
			return e
		}
		out.AutomaticAttempts = int64(len(out.AttemptIDs))
		out.PurchaseOrders = int64(len(out.OrderIDs))
		return t.Model(&ProcurementExitRefund{}).Where("buyer_id=? AND state NOT IN ('completed','released_unpaid')", buyer).Count(&out.Refunds).Error
	})
	return out, e
}
