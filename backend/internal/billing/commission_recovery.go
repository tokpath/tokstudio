package billing

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/platform/id"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type RecoveryReceipt struct {
	OccurredAt     *time.Time `json:"occurred_at,omitempty" gorm:"column:occurred_at"`
	ScopeID        string     `json:"-" gorm:"column:scope_id"`
	ActorEmail     string     `json:"actor_email,omitempty" gorm:"-"`
	ID             string     `json:"id" gorm:"column:id;primaryKey"`
	RecoveryID     string     `json:"recovery_id" gorm:"column:recovery_id"`
	AmountMinor    int64      `json:"amount_minor" gorm:"column:amount_minor"`
	Reference      string     `json:"reference" gorm:"column:reference"`
	Note           string     `json:"note" gorm:"column:note"`
	ActorUserID    string     `json:"actor_user_id,omitempty" gorm:"column:actor_user_id"`
	IdempotencyKey string     `json:"-" gorm:"column:idempotency_key"`
	CreatedAt      time.Time  `json:"created_at" gorm:"column:created_at"`
}

func (RecoveryReceipt) TableName() string { return "billing_commission_recovery_receipts" }

type CommissionRecoveryView struct {
	ID                string            `json:"id"`
	UserID            string            `json:"user_id"`
	SettlementID      string            `json:"settlement_id"`
	CommissionEntryID string            `json:"commission_entry_id"`
	AmountMinor       int64             `json:"amount_minor"`
	RecoveredMinor    int64             `json:"recovered_minor"`
	Status            string            `json:"status"`
	CreatedAt         time.Time         `json:"created_at"`
	Receipts          []RecoveryReceipt `json:"receipts" gorm:"-"`
}

func (s *Service) ListCommissionRecoveries(ctx context.Context, userID string) ([]CommissionRecoveryView, error) {
	return s.listCommissionRecoveries(ctx, userID, nil)
}
func (s *Service) ListScopedCommissionRecoveries(ctx context.Context, settlementIDs []string) ([]CommissionRecoveryView, error) {
	if len(settlementIDs) == 0 {
		return []CommissionRecoveryView{}, nil
	}
	return s.listCommissionRecoveries(ctx, "", settlementIDs)
}
func (s *Service) listCommissionRecoveries(ctx context.Context, userID string, settlementIDs []string) ([]CommissionRecoveryView, error) {
	items := []CommissionRecoveryView{}
	q := s.db.WithContext(ctx).Table("billing_commission_recoveries r").Select("r.id, w.user_id, r.settlement_id, r.commission_entry_id, r.amount_minor, r.recovered_minor, r.status, r.created_at").Joins("JOIN billing_wallets w ON w.id=r.wallet_id")
	if userID != "" {
		q = q.Where("w.user_id = ?", userID)
	}
	if settlementIDs != nil {
		q = q.Where("r.settlement_id IN ?", settlementIDs)
	}
	if err := q.Order("CASE WHEN r.status = 'pending' THEN 0 ELSE 1 END, r.created_at DESC, r.id").Scan(&items).Error; err != nil {
		return nil, err
	}
	ids := []string{}
	byID := map[string]int{}
	for i := range items {
		ids = append(ids, items[i].ID)
		byID[items[i].ID] = i
		items[i].Receipts = []RecoveryReceipt{}
	}
	if len(ids) > 0 {
		var receipts []RecoveryReceipt
		if err := s.db.WithContext(ctx).Where("recovery_id IN ?", ids).Order("created_at, id").Find(&receipts).Error; err != nil {
			return nil, err
		}
		for _, receipt := range receipts {
			if userID != "" {
				receipt.ActorUserID = ""
				receipt.Note = ""
			}
			i := byID[receipt.RecoveryID]
			items[i].Receipts = append(items[i].Receipts, receipt)
		}
	}
	return items, nil
}

type RecoveryReceiptInput struct {
	OccurredAt     time.Time `json:"occurred_at"`
	Confirmed      bool      `json:"confirmed"`
	AmountMinor    int64     `json:"amount_minor"`
	Reference      string    `json:"reference"`
	Note           string    `json:"note"`
	IdempotencyKey string    `json:"idempotency_key"`
}

// Caller commits the receipt, aggregate and audit together. No wallet balance changes.
func (s *Service) RecordCommissionRecoveryTx(tx *gorm.DB, recoveryID, actor string, in RecoveryReceiptInput) (*RecoveryReceipt, bool, error) {
	return s.recordCommissionRecoveryTx(tx, recoveryID, actor, "legacy:"+recoveryID, nil, in)
}

func (s *Service) RecordScopedCommissionRecoveryTx(tx *gorm.DB, recoveryID, actor, ownerID string, settlementIDs []string, in RecoveryReceiptInput, referenceIDs ...[]string) (*RecoveryReceipt, bool, error) {
	if ownerID == "" || len(settlementIDs) == 0 || !in.Confirmed || in.OccurredAt.IsZero() || in.OccurredAt.After(time.Now().UTC().Add(5*time.Minute)) {
		return nil, false, ErrInvalidAmount
	}
	return s.recordCommissionRecoveryTx(tx, recoveryID, actor, ownerID, settlementIDs, in, referenceIDs...)
}
func (s *Service) recordCommissionRecoveryTx(tx *gorm.DB, recoveryID, actor, scopeID string, settlementIDs []string, in RecoveryReceiptInput, referenceIDs ...[]string) (*RecoveryReceipt, bool, error) {
	in.Reference = strings.TrimSpace(in.Reference)
	in.Note = strings.TrimSpace(in.Note)
	in.IdempotencyKey = strings.TrimSpace(in.IdempotencyKey)
	if !in.OccurredAt.IsZero() {
		in.OccurredAt = in.OccurredAt.UTC().Truncate(time.Microsecond)
	}
	if recoveryID == "" || actor == "" || in.AmountMinor <= 0 || len(in.Reference) > 200 || len(in.Note) > 1000 || in.IdempotencyKey == "" || len(in.IdempotencyKey) > 200 {
		return nil, false, ErrInvalidAmount
	}
	if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", "commission-recovery-receipt:"+scopeID+":"+actor+":"+in.IdempotencyKey).Error; err != nil {
		return nil, false, err
	}
	if in.Reference != "" {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", "commission-recovery-reference:"+scopeID+":"+in.Reference).Error; err != nil {
			return nil, false, err
		}
	}
	var recovery commissionRecoveryRow
	q := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=?", recoveryID)
	if settlementIDs != nil {
		q = q.Where("settlement_id IN ?", settlementIDs)
	}
	if err := q.First(&recovery).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, false, ErrNotFound
		}
		return nil, false, err
	}
	var receipt RecoveryReceipt
	if err := tx.Where("(scope_id=? OR (scope_id=? AND recovery_id=?)) AND actor_user_id=? AND idempotency_key = ?", scopeID, "legacy:"+recoveryID, recoveryID, actor, in.IdempotencyKey).First(&receipt).Error; err == nil {
		if receipt.RecoveryID != recoveryID || receipt.AmountMinor != in.AmountMinor || receipt.Reference != in.Reference || receipt.Note != in.Note || (receipt.OccurredAt == nil) != in.OccurredAt.IsZero() || (receipt.OccurredAt != nil && !receipt.OccurredAt.Equal(in.OccurredAt)) {
			return nil, false, ErrConflict
		}
		return &receipt, false, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, err
	}
	var duplicate int64
	if in.Reference != "" {
		ids := settlementIDs
		if len(referenceIDs) > 0 {
			ids = referenceIDs[0]
		}
		q := tx.Model(&RecoveryReceipt{}).Where("scope_id=?", scopeID)
		if len(ids) > 0 {
			claims := tx.Model(&commissionRecoveryRow{}).Select("id").Where("settlement_id IN ?", ids)
			q = tx.Model(&RecoveryReceipt{}).Where("scope_id=? OR (scope_id LIKE 'legacy:%' AND recovery_id IN (?))", scopeID, claims)
		}
		if err := q.Where("reference=?", in.Reference).Count(&duplicate).Error; err != nil {
			return nil, false, err
		}
	}
	if duplicate > 0 || recovery.Status != "pending" {
		return nil, false, ErrConflict
	}
	if in.AmountMinor > recovery.AmountMinor-recovery.RecoveredMinor {
		return nil, false, ErrInvalidAmount
	}
	receipt = RecoveryReceipt{ID: id.New("crr"), RecoveryID: recoveryID, AmountMinor: in.AmountMinor, Reference: in.Reference, Note: in.Note, ActorUserID: actor, IdempotencyKey: in.IdempotencyKey, ScopeID: scopeID, CreatedAt: time.Now().UTC()}
	if !in.OccurredAt.IsZero() {
		at := in.OccurredAt.UTC()
		receipt.OccurredAt = &at
	}
	if err := tx.Create(&receipt).Error; err != nil {
		return nil, false, err
	}
	recovery.RecoveredMinor += in.AmountMinor
	if recovery.RecoveredMinor == recovery.AmountMinor {
		recovery.Status = "closed"
	}
	if err := tx.Save(&recovery).Error; err != nil {
		return nil, false, err
	}
	return &receipt, true, nil
}

func (s *Service) CommissionRecoveryOperation(ctx context.Context, actor, ownerID, operationID string, settlementIDs []string) (*RecoveryReceipt, error) {
	if actor == "" || ownerID == "" || len(settlementIDs) == 0 {
		return nil, ErrNotFound
	}
	var receipt RecoveryReceipt
	err := s.db.WithContext(ctx).Table("billing_commission_recovery_receipts rr").Select("rr.*").Joins("JOIN billing_commission_recoveries r ON r.id=rr.recovery_id").Where("(rr.scope_id=? OR rr.scope_id LIKE 'legacy:%') AND rr.actor_user_id=? AND rr.idempotency_key=? AND r.settlement_id IN ?", ownerID, actor, operationID, settlementIDs).First(&receipt).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return &receipt, err
}

type CommissionRecoveryTotals struct {
	SettlementID   string
	AmountMinor    int64
	RecoveredMinor int64
}

func (s *Service) CommissionRecoveryTotals(ctx context.Context, ids []string) (map[string]CommissionRecoveryTotals, error) {
	out := map[string]CommissionRecoveryTotals{}
	if len(ids) == 0 {
		return out, nil
	}
	var rows []CommissionRecoveryTotals
	if err := s.db.WithContext(ctx).Model(&commissionRecoveryRow{}).Where("settlement_id IN ?", ids).Select("settlement_id, SUM(amount_minor) AS amount_minor, SUM(recovered_minor) AS recovered_minor").Group("settlement_id").Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.SettlementID] = row
	}
	return out, nil
}
