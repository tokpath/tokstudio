package billing

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/platform/id"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type OEMPurchaseInput struct {
	OperationID       string    `json:"operation_id"`
	OEMChannelOrgID   string    `json:"oem_channel_org_id"`
	CashAmountMinor   int64     `json:"cash_amount_minor"`
	CashCurrency      string    `json:"cash_currency"`
	SaleAmountMinor   int64     `json:"sale_amount_minor"` // agreed USD sale value, not an inferred exchange rate
	QuotaAmountMinor  int64     `json:"quota_amount_minor"`
	OccurredAt        time.Time `json:"occurred_at"`
	ExternalReference string    `json:"external_reference"`
	Note              string    `json:"note"`
	Confirmed         bool      `json:"confirmed"`
}
type OEMPurchaseReversalInput struct {
	OperationID string `json:"operation_id"`
	Reason      string `json:"reason"`
}
type oemPurchaseRow struct {
	ID                  string
	OperationID         string
	ActorUserID         string
	OEMChannelOrgID     string
	CashAmountMinor     int64
	CashCurrency        string
	SaleAmountMinor     int64
	QuotaAmountMinor    int64
	OccurredAt          time.Time
	CompletedAt         time.Time
	ExternalReference   string
	Note                string
	ResultJSON          []byte
	Status              string
	ReversalOperationID string
	ReversedBy          string
	ReversalReason      string
	ReversedAt          *time.Time
	ReversalResultJSON  []byte
}

func (oemPurchaseRow) TableName() string { return "billing_oem_purchases" }

type OEMPurchaseView struct {
	ID                  string     `json:"id"`
	OperationID         string     `json:"operation_id"`
	ActorUserID         string     `json:"actor_user_id"`
	OEMChannelOrgID     string     `json:"oem_channel_org_id"`
	CashAmountMinor     int64      `json:"cash_amount_minor"`
	CashCurrency        string     `json:"cash_currency"`
	SaleAmountMinor     int64      `json:"sale_amount_minor"`
	QuotaAmountMinor    int64      `json:"quota_amount_minor"`
	OccurredAt          time.Time  `json:"occurred_at"`
	CompletedAt         time.Time  `json:"completed_at"`
	ExternalReference   string     `json:"external_reference,omitempty"`
	Note                string     `json:"note,omitempty"`
	Quota               *QuotaView `json:"quota"`
	Status              string     `json:"status"`
	ReversalOperationID string     `json:"reversal_operation_id,omitempty"`
	ReversedBy          string     `json:"reversed_by,omitempty"`
	ReversalReason      string     `json:"reversal_reason,omitempty"`
	ReversedAt          *time.Time `json:"reversed_at,omitempty"`
	ReversalQuota       *QuotaView `json:"reversal_quota,omitempty"`
}
type OEMPurchaseReferenceConflict struct{ PurchaseID string }

func (e *OEMPurchaseReferenceConflict) Error() string {
	return "OEM purchase transaction already registered"
}
func purchaseView(row oemPurchaseRow) (*OEMPurchaseView, error) {
	var quota QuotaView
	if err := json.Unmarshal(row.ResultJSON, &quota); err != nil {
		return nil, err
	}
	out := &OEMPurchaseView{ID: row.ID, OperationID: row.OperationID, ActorUserID: row.ActorUserID, OEMChannelOrgID: row.OEMChannelOrgID, CashAmountMinor: row.CashAmountMinor, CashCurrency: row.CashCurrency, SaleAmountMinor: row.SaleAmountMinor, QuotaAmountMinor: row.QuotaAmountMinor, OccurredAt: row.OccurredAt, CompletedAt: row.CompletedAt, ExternalReference: row.ExternalReference, Note: row.Note, Quota: &quota, Status: row.Status, ReversalOperationID: row.ReversalOperationID, ReversedBy: row.ReversedBy, ReversalReason: row.ReversalReason, ReversedAt: row.ReversedAt}
	if len(row.ReversalResultJSON) > 0 {
		var result QuotaView
		if err := json.Unmarshal(row.ReversalResultJSON, &result); err != nil {
			return nil, err
		}
		out.ReversalQuota = &result
	}
	return out, nil
}
func (s *Service) CompleteOEMPurchase(ctx context.Context, actor string, in OEMPurchaseInput) (*OEMPurchaseView, error) {
	var out *OEMPurchaseView
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error { var err error; out, err = s.CompleteOEMPurchaseTx(tx, actor, in); return err })
	return out, err
}
func (s *Service) CompleteOEMPurchaseTx(tx *gorm.DB, actor string, in OEMPurchaseInput) (*OEMPurchaseView, error) {
	in.OperationID = strings.TrimSpace(in.OperationID)
	in.ExternalReference = strings.TrimSpace(in.ExternalReference)
	in.Note = strings.TrimSpace(in.Note)
	in.CashCurrency = strings.ToUpper(strings.TrimSpace(in.CashCurrency))
	in.OccurredAt = in.OccurredAt.UTC()
	if actor == "" || in.OperationID == "" || len(in.OperationID) > 100 || in.CashAmountMinor <= 0 || in.SaleAmountMinor <= 0 || in.QuotaAmountMinor <= 0 || !in.Confirmed || in.OccurredAt.IsZero() || in.OccurredAt.After(time.Now().Add(5*time.Minute)) || (in.CashCurrency != "USD" && in.CashCurrency != "CNY") || len(in.Note) > 2000 || len(in.ExternalReference) > 200 || (in.CashCurrency == "USD" && in.SaleAmountMinor != in.CashAmountMinor) {
		return nil, ErrInvalidAmount
	}
	owner, err := identity.New(tx).ResolvePaymentOwnerID(tx.Statement.Context, in.OEMChannelOrgID)
	if err != nil {
		return nil, err
	}
	if owner != in.OEMChannelOrgID || owner == identity.OfficialChannelID {
		return nil, ErrInvalidAmount
	}
	if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", "oem-purchase-operation:"+in.OperationID).Error; err != nil {
		return nil, err
	}
	var previous oemPurchaseRow
	err = tx.Where("operation_id=?", in.OperationID).First(&previous).Error
	if err == nil {
		if previous.ActorUserID != actor || previous.OEMChannelOrgID != in.OEMChannelOrgID || previous.CashAmountMinor != in.CashAmountMinor || previous.CashCurrency != in.CashCurrency || previous.SaleAmountMinor != in.SaleAmountMinor || previous.QuotaAmountMinor != in.QuotaAmountMinor || !previous.OccurredAt.Equal(in.OccurredAt) || previous.ExternalReference != in.ExternalReference || previous.Note != in.Note {
			return nil, ErrConflict
		}
		return purchaseView(previous)
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if in.ExternalReference != "" {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", "oem-purchase-external:"+in.ExternalReference).Error; err != nil {
			return nil, err
		}
		err = tx.Where("external_reference=? AND status='completed'", in.ExternalReference).First(&previous).Error
		if err == nil {
			return nil, &OEMPurchaseReferenceConflict{previous.ID}
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	}
	if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", "quota-owner:"+owner).Error; err != nil {
		return nil, err
	}
	scoped := *s
	scoped.db = tx
	recordID := id.New("oem_purchase")
	quota, err := scoped.grantChannelQuota(tx.Statement.Context, owner, in.QuotaAmountMinor, actor, "oem_purchase", recordID, "oem-purchase:"+in.OperationID)
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(quota)
	if err != nil {
		return nil, err
	}
	row := oemPurchaseRow{ID: recordID, OperationID: in.OperationID, ActorUserID: actor, OEMChannelOrgID: owner, CashAmountMinor: in.CashAmountMinor, CashCurrency: in.CashCurrency, SaleAmountMinor: in.SaleAmountMinor, QuotaAmountMinor: in.QuotaAmountMinor, OccurredAt: in.OccurredAt, CompletedAt: time.Now().UTC(), ExternalReference: in.ExternalReference, Note: in.Note, ResultJSON: body, Status: "completed"}
	if err := tx.Create(&row).Error; err != nil {
		return nil, err
	}
	return purchaseView(row)
}
func (s *Service) OEMPurchaseOperation(ctx context.Context, actor, owner, operation string) (*OEMPurchaseView, error) {
	var row oemPurchaseRow
	err := s.db.WithContext(ctx).Where("actor_user_id=? AND oem_channel_org_id=? AND operation_id=?", actor, owner, operation).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return purchaseView(row)
}
func (s *Service) OEMPurchase(ctx context.Context, owner, purchaseID string) (*OEMPurchaseView, error) {
	var row oemPurchaseRow
	q := s.db.WithContext(ctx).Where("id=?", purchaseID)
	if owner != "" {
		q = q.Where("oem_channel_org_id=?", owner)
	}
	if err := q.First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return purchaseView(row)
}

func (s *Service) ListOEMPurchases(ctx context.Context, owner, cursor string, limit int) ([]OEMPurchaseView, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	q := s.db.WithContext(ctx).Order("completed_at DESC,id DESC").Limit(limit + 1)
	if owner != "" {
		q = q.Where("oem_channel_org_id=?", owner)
	}
	if cursor != "" {
		var anchor oemPurchaseRow
		anchorQuery := s.db.WithContext(ctx).Where("id=?", cursor)
		if owner != "" {
			anchorQuery = anchorQuery.Where("oem_channel_org_id=?", owner)
		}
		if err := anchorQuery.First(&anchor).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, ErrNotFound
			}
			return nil, err
		}
		q = q.Where("(completed_at,id)<(?,?)", anchor.CompletedAt, anchor.ID)
	}
	var rows []oemPurchaseRow
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]OEMPurchaseView, 0, len(rows))
	for _, row := range rows {
		v, err := purchaseView(row)
		if err != nil {
			return nil, err
		}
		out = append(out, *v)
	}
	return out, nil
}

// ReverseOEMPurchaseTx corrects the registration, not the external payment.
// Only unissued quota is reclaimable; wallets and customer balances are untouched.
func (s *Service) ReverseOEMPurchaseTx(tx *gorm.DB, actor, purchaseID string, in OEMPurchaseReversalInput) (*OEMPurchaseView, error) {
	in.OperationID = strings.TrimSpace(in.OperationID)
	in.Reason = strings.TrimSpace(in.Reason)
	if actor == "" || purchaseID == "" || in.OperationID == "" || len(in.OperationID) > 100 || in.Reason == "" || len(in.Reason) > 2000 {
		return nil, ErrInvalidAmount
	}
	if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", "oem-purchase-reversal:"+in.OperationID).Error; err != nil {
		return nil, err
	}
	var previous oemPurchaseRow
	err := tx.Where("reversal_operation_id=?", in.OperationID).First(&previous).Error
	if err == nil && (previous.ID != purchaseID || previous.ReversedBy != actor || previous.ReversalReason != in.Reason) {
		return nil, ErrConflict
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	var row oemPurchaseRow
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=?", purchaseID).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if row.Status == "reversed" {
		if row.ReversalOperationID != in.OperationID || row.ReversedBy != actor || row.ReversalReason != in.Reason {
			return nil, ErrConflict
		}
		return purchaseView(row)
	}
	if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", "quota-owner:"+row.OEMChannelOrgID).Error; err != nil {
		return nil, err
	}
	scoped := *s
	scoped.db = tx
	quota, err := scoped.grantChannelQuota(tx.Statement.Context, row.OEMChannelOrgID, -row.QuotaAmountMinor, actor, "oem_purchase_reversal", row.ID, "oem-purchase-reversal:"+in.OperationID)
	if err != nil {
		return nil, err
	}
	result, err := json.Marshal(quota)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	row.Status = "reversed"
	row.ReversalOperationID = in.OperationID
	row.ReversalReason = in.Reason
	row.ReversedBy = actor
	row.ReversedAt = &now
	row.ReversalResultJSON = result
	if err := tx.Save(&row).Error; err != nil {
		return nil, err
	}
	return purchaseView(row)
}
