package billing

import (
	"context"
	"errors"
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
	Confirmed      bool      `json:"confirmed"`
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
	ReversedBy     string    `json:"reversed_by,omitempty"`
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
	var out *SupplierView
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error { var e error; out, _, e = s.RecordSupplierTx(tx, actorUserID, in); return e })
	return out, err
}

// RecordSupplierTx shares the original fact and the caller's audit transaction.
func (s *Service) RecordSupplierTx(tx *gorm.DB, actorUserID string, in SupplierInput) (*SupplierView, bool, error) {
	in.ChannelOrgID = strings.TrimSpace(in.ChannelOrgID)
	in.SourceType = strings.TrimSpace(in.SourceType)
	in.IdempotencyKey = strings.TrimSpace(in.IdempotencyKey)
	if in.ChannelOrgID == "" || actorUserID == "" || in.IdempotencyKey == "" || len(in.IdempotencyKey) > 200 || in.AmountMinor <= 0 {
		return nil, false, ErrInvalidAmount
	}
	if !AllowedSupplierSource(in.SourceType) || InventedSupplierSource(in.SourceType) {
		return nil, false, ErrInventedCost
	}
	if s.pool != nil {
		pool, err := s.pool.ResolvePoolChannelID(tx.Statement.Context, in.ChannelOrgID)
		if err != nil {
			return nil, false, err
		}
		if pool != "" {
			in.ChannelOrgID = pool
		}
	}
	if in.Currency == "" {
		in.Currency = CurrencyUSD
	}
	explicitTime := !in.OccurredAt.IsZero()
	if explicitTime {
		in.OccurredAt = in.OccurredAt.UTC().Truncate(time.Microsecond)
	} else {
		in.OccurredAt = time.Now().UTC().Truncate(time.Microsecond)
	}
	if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", "supplier-operation:"+in.IdempotencyKey).Error; err != nil {
		return nil, false, err
	}
	var existing supplierRow
	err := tx.Where("idempotency_key=?", in.IdempotencyKey).First(&existing).Error
	if err == nil {
		if existing.ReversalOf != nil || existing.ActorUserID != actorUserID || existing.ChannelOrgID != in.ChannelOrgID || existing.AmountMinor != -in.AmountMinor || existing.Currency != in.Currency || existing.SourceType != in.SourceType || (explicitTime && !existing.OccurredAt.Equal(in.OccurredAt)) || !sameSupplierOptional(existing, in) {
			return nil, false, ErrConflict
		}
		return supplierView(existing), false, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, err
	}
	reference := strings.TrimSpace(in.BankRef)
	if reference != "" {
		if err = tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", "supplier-reference:"+in.ChannelOrgID+":"+reference).Error; err != nil {
			return nil, false, err
		}
		var duplicate int64
		if err = tx.Model(&supplierRow{}).Where("channel_org_id=? AND bank_ref=? AND reversal_of IS NULL", in.ChannelOrgID, reference).Count(&duplicate).Error; err != nil {
			return nil, false, err
		}
		if duplicate > 0 {
			return nil, false, ErrConflict
		}
	}
	row := supplierRow{ID: id.New("spe"), ChannelOrgID: in.ChannelOrgID, AmountMinor: -in.AmountMinor, Currency: in.Currency, OccurredAt: in.OccurredAt, SourceType: in.SourceType, SourceID: optStr(in.SourceID), ProviderID: optStr(in.ProviderID), VendorName: optStr(in.VendorName), InvoiceNo: optStr(in.InvoiceNo), PaymentMethod: optStr(in.PaymentMethod), BankRef: optStr(in.BankRef), Counterparty: optStr(in.Counterparty), Memo: optStr(in.Memo), AttachmentURL: optStr(in.AttachmentURL), ActorUserID: actorUserID, IdempotencyKey: in.IdempotencyKey, CreatedAt: time.Now().UTC()}
	if err = tx.Create(&row).Error; err != nil {
		return nil, false, err
	}
	return supplierView(row), true, nil
}

func sameSupplierOptional(row supplierRow, in SupplierInput) bool {
	value := func(v *string) string {
		if v == nil {
			return ""
		}
		return *v
	}
	return value(row.SourceID) == strings.TrimSpace(in.SourceID) && value(row.ProviderID) == strings.TrimSpace(in.ProviderID) && value(row.VendorName) == strings.TrimSpace(in.VendorName) && value(row.InvoiceNo) == strings.TrimSpace(in.InvoiceNo) && value(row.PaymentMethod) == strings.TrimSpace(in.PaymentMethod) && value(row.BankRef) == strings.TrimSpace(in.BankRef) && value(row.Counterparty) == strings.TrimSpace(in.Counterparty) && value(row.Memo) == strings.TrimSpace(in.Memo) && value(row.AttachmentURL) == strings.TrimSpace(in.AttachmentURL)
}

func (s *Service) ReverseSupplier(ctx context.Context, actorUserID, entryID, reason string) (*SupplierView, error) {
	var out *SupplierView
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var e error
		out, _, e = s.ReverseSupplierTx(tx, actorUserID, entryID, "spe-rev:"+entryID, reason, "")
		return e
	})
	return out, err
}

func (s *Service) ReverseSupplierTx(tx *gorm.DB, actor, entryID, operationID, reason, book string) (*SupplierView, bool, error) {
	reason = strings.TrimSpace(reason)
	if actor == "" || entryID == "" || operationID == "" || len(operationID) > 200 || len(reason) > 1000 {
		return nil, false, ErrInvalidAmount
	}
	if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", "supplier-operation:"+operationID).Error; err != nil {
		return nil, false, err
	}
	var orig supplierRow
	q := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=?", entryID)
	if book != "" {
		q = q.Where("channel_org_id=?", book)
	}
	if err := q.First(&orig).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, false, ErrNotFound
		}
		return nil, false, err
	}
	if orig.ReversalOf != nil || orig.AmountMinor >= 0 {
		return nil, false, ErrConflict
	}
	var existing supplierRow
	if err := tx.Where("idempotency_key=?", operationID).First(&existing).Error; err == nil {
		memo := ""
		if existing.Memo != nil {
			memo = *existing.Memo
		}
		if existing.ActorUserID != actor || existing.ChannelOrgID != orig.ChannelOrgID || existing.ReversalOf == nil || *existing.ReversalOf != entryID || memo != reason {
			return nil, false, ErrConflict
		}
		return supplierView(existing), false, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, err
	}
	var count int64
	if err := tx.Model(&supplierRow{}).Where("reversal_of=?", entryID).Count(&count).Error; err != nil {
		return nil, false, err
	}
	if count > 0 {
		return nil, false, ErrConflict
	}
	now := time.Now().UTC()
	rev := supplierRow{ID: id.New("spe"), ChannelOrgID: orig.ChannelOrgID, AmountMinor: -orig.AmountMinor, Currency: orig.Currency, OccurredAt: now, SourceType: orig.SourceType, SourceID: orig.SourceID, ProviderID: orig.ProviderID, VendorName: orig.VendorName, InvoiceNo: orig.InvoiceNo, PaymentMethod: orig.PaymentMethod, BankRef: orig.BankRef, Counterparty: orig.Counterparty, AttachmentURL: orig.AttachmentURL, ActorUserID: actor, ReversalOf: &orig.ID, IdempotencyKey: operationID, CreatedAt: now, Memo: optStr(reason)}
	if err := tx.Create(&rev).Error; err != nil {
		return nil, false, err
	}
	return supplierView(rev), true, nil
}

func (s *Service) ListSupplier(ctx context.Context, channelOrgID string, limit int) ([]SupplierView, error) {
	q := s.db.WithContext(ctx).Order("created_at DESC, id DESC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	if channelOrgID != "" {
		q = q.Where("channel_org_id = ?", channelOrgID)
	}
	var rows []supplierRow
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]SupplierView, 0, len(rows))
	ids := []string{}
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	var reversals []supplierRow
	if len(ids) > 0 {
		if err := s.db.WithContext(ctx).Where("reversal_of IN ?", ids).Find(&reversals).Error; err != nil {
			return nil, err
		}
	}
	reversed := map[string]string{}
	for _, row := range reversals {
		if row.ReversalOf != nil {
			reversed[*row.ReversalOf] = row.ID
		}
	}
	for _, row := range rows {
		v := *supplierView(row)
		v.ReversedBy = reversed[row.ID]
		out = append(out, v)
	}
	return out, nil
}

func (s *Service) GetSupplier(ctx context.Context, entryID string) (*SupplierView, error) {
	var row supplierRow
	if err := s.db.WithContext(ctx).Where("id = ?", entryID).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return supplierView(row), nil
}

// SupplierOperation only returns the caller's own operation in its original book.
func (s *Service) SupplierOperation(ctx context.Context, actorID, channelID, operationID string) (*SupplierView, error) {
	var row supplierRow
	err := s.db.WithContext(ctx).Where("actor_user_id = ? AND channel_org_id = ? AND idempotency_key = ?", actorID, channelID, operationID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
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
