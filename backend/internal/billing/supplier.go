package billing

import (
	"context"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/tokpath/tokstudio/backend/internal/platform/id"
)

type supplierRow struct {
	ID             string    `gorm:"column:id;primaryKey"`
	ChannelOrgID   string    `gorm:"column:channel_org_id"`
	AmountMinor    int64     `gorm:"column:amount_minor"`
	Currency       string    `gorm:"column:currency"`
	OccurredAt     time.Time `gorm:"column:occurred_at"`
	SourceType     string    `gorm:"column:source_type"`
	SourceID       *string   `gorm:"column:source_id"`
	ProviderID     *string   `gorm:"column:provider_id"`
	VendorName     *string   `gorm:"column:vendor_name"`
	InvoiceNo      *string   `gorm:"column:invoice_no"`
	PaymentMethod  *string   `gorm:"column:payment_method"`
	BankRef        *string   `gorm:"column:bank_ref"`
	Counterparty   *string   `gorm:"column:counterparty"`
	Memo           *string   `gorm:"column:memo"`
	AttachmentURL  *string   `gorm:"column:attachment_url"`
	ActorUserID    string    `gorm:"column:actor_user_id"`
	ReversalOf     *string   `gorm:"column:reversal_of"`
	IdempotencyKey string    `gorm:"column:idempotency_key"`
	CreatedAt      time.Time `gorm:"column:created_at"`
}

func (supplierRow) TableName() string { return "billing_supplier_entries" }

type SupplierInput struct {
	ChannelOrgID   string    `json:"-"`
	AmountMinor    int64     `json:"amount_minor"`
	Currency       string    `json:"currency"`
	OccurredAt     time.Time `json:"occurred_at"`
	SourceType     string    `json:"source_type"`
	SourceID       string    `json:"source_id"`
	ProviderID     string    `json:"provider_id"`
	VendorName     string    `json:"vendor_name"`
	InvoiceNo      string    `json:"invoice_no"`
	PaymentMethod  string    `json:"payment_method"`
	BankRef        string    `json:"bank_ref"`
	Counterparty   string    `json:"counterparty"`
	Memo           string    `json:"memo"`
	AttachmentURL  string    `json:"attachment_url"`
	IdempotencyKey string    `json:"idempotency_key"`
}

type SupplierView struct {
	ID             string    `json:"id"`
	ChannelOrgID   string    `json:"channel_org_id"`
	AmountMinor    int64     `json:"amount_minor"`
	Currency       string    `json:"currency"`
	OccurredAt     time.Time `json:"occurred_at"`
	SourceType     string    `json:"source_type"`
	SourceID       string    `json:"source_id,omitempty"`
	ProviderID     string    `json:"provider_id,omitempty"`
	VendorName     string    `json:"vendor_name,omitempty"`
	InvoiceNo      string    `json:"invoice_no,omitempty"`
	PaymentMethod  string    `json:"payment_method,omitempty"`
	BankRef        string    `json:"bank_ref,omitempty"`
	Counterparty   string    `json:"counterparty,omitempty"`
	Memo           string    `json:"memo,omitempty"`
	AttachmentURL  string    `json:"attachment_url,omitempty"`
	ActorUserID    string    `json:"actor_user_id"`
	ReversalOf     string    `json:"reversal_of,omitempty"`
	IdempotencyKey string    `json:"idempotency_key"`
	CreatedAt      time.Time `json:"created_at"`
}

func supplierView(row supplierRow) *SupplierView {
	v := &SupplierView{
		ID: row.ID, ChannelOrgID: row.ChannelOrgID, AmountMinor: row.AmountMinor,
		Currency: row.Currency, OccurredAt: row.OccurredAt, SourceType: row.SourceType,
		ActorUserID: row.ActorUserID, IdempotencyKey: row.IdempotencyKey, CreatedAt: row.CreatedAt,
	}
	if row.SourceID != nil {
		v.SourceID = *row.SourceID
	}
	if row.ProviderID != nil {
		v.ProviderID = *row.ProviderID
	}
	if row.VendorName != nil {
		v.VendorName = *row.VendorName
	}
	if row.InvoiceNo != nil {
		v.InvoiceNo = *row.InvoiceNo
	}
	if row.PaymentMethod != nil {
		v.PaymentMethod = *row.PaymentMethod
	}
	if row.BankRef != nil {
		v.BankRef = *row.BankRef
	}
	if row.Counterparty != nil {
		v.Counterparty = *row.Counterparty
	}
	if row.Memo != nil {
		v.Memo = *row.Memo
	}
	if row.AttachmentURL != nil {
		v.AttachmentURL = *row.AttachmentURL
	}
	if row.ReversalOf != nil {
		v.ReversalOf = *row.ReversalOf
	}
	return v
}

func optStr(v string) *string {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	return &v
}

// RecordSupplier 记录线下已付给供应商的款项，不改钱包或额度。
func (s *Service) RecordSupplier(ctx context.Context, actorUserID string, in SupplierInput) (*SupplierView, error) {
	in.ChannelOrgID = strings.TrimSpace(in.ChannelOrgID)
	in.SourceType = strings.TrimSpace(in.SourceType)
	in.IdempotencyKey = strings.TrimSpace(in.IdempotencyKey)
	if in.ChannelOrgID == "" || in.SourceType == "" || in.IdempotencyKey == "" || actorUserID == "" {
		return nil, ErrInvalidAmount
	}
	if s.pool != nil {
		if pool, err := s.pool.ResolvePoolChannelID(ctx, in.ChannelOrgID); err == nil && pool != "" {
			in.ChannelOrgID = pool
		}
	}
	if in.AmountMinor <= 0 {
		return nil, ErrInvalidAmount
	}
	if in.Currency == "" {
		in.Currency = CurrencyUSD
	}
	if in.OccurredAt.IsZero() {
		in.OccurredAt = time.Now().UTC()
	}
	var out *SupplierView
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing supplierRow
		if err := tx.Where("idempotency_key = ?", in.IdempotencyKey).First(&existing).Error; err == nil {
			out = supplierView(existing)
			return nil
		}
		row := supplierRow{
			ID: id.New("spe"), ChannelOrgID: in.ChannelOrgID, AmountMinor: -in.AmountMinor,
			Currency: in.Currency, OccurredAt: in.OccurredAt.UTC(), SourceType: in.SourceType,
			SourceID: optStr(in.SourceID), ProviderID: optStr(in.ProviderID), VendorName: optStr(in.VendorName),
			InvoiceNo: optStr(in.InvoiceNo), PaymentMethod: optStr(in.PaymentMethod), BankRef: optStr(in.BankRef),
			Counterparty: optStr(in.Counterparty), Memo: optStr(in.Memo), AttachmentURL: optStr(in.AttachmentURL),
			ActorUserID: actorUserID, IdempotencyKey: in.IdempotencyKey, CreatedAt: time.Now().UTC(),
		}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		out = supplierView(row)
		return nil
	})
	return out, err
}

func (s *Service) ReverseSupplier(ctx context.Context, actorUserID, entryID, reason string) (*SupplierView, error) {
	if actorUserID == "" || entryID == "" {
		return nil, ErrInvalidAmount
	}
	idem := "spe-rev:" + entryID
	var out *SupplierView
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing supplierRow
		if err := tx.Where("idempotency_key = ?", idem).First(&existing).Error; err == nil {
			out = supplierView(existing)
			return nil
		}
		var orig supplierRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", entryID).First(&orig).Error; err != nil {
			return ErrNotFound
		}
		if orig.ReversalOf != nil {
			return ErrConflict
		}
		memo := strings.TrimSpace(reason)
		rev := supplierRow{
			ID: id.New("spe"), ChannelOrgID: orig.ChannelOrgID, AmountMinor: -orig.AmountMinor,
			Currency: orig.Currency, OccurredAt: time.Now().UTC(), SourceType: orig.SourceType,
			SourceID: orig.SourceID, ProviderID: orig.ProviderID, VendorName: orig.VendorName,
			InvoiceNo: orig.InvoiceNo, PaymentMethod: orig.PaymentMethod, BankRef: orig.BankRef,
			Counterparty: orig.Counterparty, AttachmentURL: orig.AttachmentURL,
			ActorUserID: actorUserID, ReversalOf: &orig.ID, IdempotencyKey: idem, CreatedAt: time.Now().UTC(),
		}
		if memo != "" {
			rev.Memo = &memo
		}
		if err := tx.Create(&rev).Error; err != nil {
			return err
		}
		out = supplierView(rev)
		return nil
	})
	return out, err
}

func (s *Service) ListSupplier(ctx context.Context, channelOrgID string, limit int) ([]SupplierView, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	q := s.db.WithContext(ctx).Order("occurred_at DESC").Limit(limit)
	if channelOrgID != "" {
		q = q.Where("channel_org_id = ?", channelOrgID)
	}
	var rows []supplierRow
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]SupplierView, 0, len(rows))
	for _, row := range rows {
		out = append(out, *supplierView(row))
	}
	return out, nil
}

func (s *Service) GetSupplier(ctx context.Context, entryID string) (*SupplierView, error) {
	var row supplierRow
	if err := s.db.WithContext(ctx).Where("id = ?", entryID).First(&row).Error; err != nil {
		return nil, ErrNotFound
	}
	return supplierView(row), nil
}

func (s *Service) SupplierTotal(ctx context.Context, channelOrgID string) (int64, error) {
	if channelOrgID == "" {
		return 0, nil
	}
	var total int64
	err := s.db.WithContext(ctx).Model(&supplierRow{}).
		Where("channel_org_id = ?", channelOrgID).
		Select("COALESCE(SUM(amount_minor),0)").Scan(&total).Error
	return total, err
}
