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
	ActorEmail     string    `json:"actor_email,omitempty" gorm:"-"`
	ID             string    `json:"id" gorm:"column:id;primaryKey"`
	RecoveryID     string    `json:"recovery_id" gorm:"column:recovery_id"`
	AmountMinor    int64     `json:"amount_minor" gorm:"column:amount_minor"`
	Reference      string    `json:"reference" gorm:"column:reference"`
	Note           string    `json:"note" gorm:"column:note"`
	ActorUserID    string    `json:"actor_user_id,omitempty" gorm:"column:actor_user_id"`
	IdempotencyKey string    `json:"-" gorm:"column:idempotency_key"`
	CreatedAt      time.Time `json:"created_at" gorm:"column:created_at"`
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
	items := []CommissionRecoveryView{}
	q := s.db.WithContext(ctx).Table("billing_commission_recoveries r").Select("r.id, w.user_id, r.settlement_id, r.commission_entry_id, r.amount_minor, r.recovered_minor, r.status, r.created_at").Joins("JOIN billing_wallets w ON w.id=r.wallet_id")
	if userID != "" {
		q = q.Where("w.user_id = ?", userID)
	}
	if err := q.Order("CASE WHEN r.status = 'pending' THEN 0 ELSE 1 END, r.created_at DESC, r.id").Limit(100).Scan(&items).Error; err != nil {
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
	AmountMinor    int64  `json:"amount_minor"`
	Reference      string `json:"reference"`
	Note           string `json:"note"`
	IdempotencyKey string `json:"idempotency_key"`
}

// Caller commits the receipt, aggregate and audit together. No wallet balance changes.
func (s *Service) RecordCommissionRecoveryTx(tx *gorm.DB, recoveryID, actor string, in RecoveryReceiptInput) (*RecoveryReceipt, bool, error) {
	in.Reference = strings.TrimSpace(in.Reference)
	in.Note = strings.TrimSpace(in.Note)
	in.IdempotencyKey = strings.TrimSpace(in.IdempotencyKey)
	if recoveryID == "" || actor == "" || in.AmountMinor <= 0 || in.Reference == "" || len(in.Reference) > 200 || len(in.Note) > 1000 || in.IdempotencyKey == "" || len(in.IdempotencyKey) > 200 {
		return nil, false, ErrInvalidAmount
	}
	if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", "commission-recovery-receipt:"+in.IdempotencyKey).Error; err != nil {
		return nil, false, err
	}
	var receipt RecoveryReceipt
	if err := tx.Where("idempotency_key = ?", in.IdempotencyKey).First(&receipt).Error; err == nil {
		if receipt.RecoveryID != recoveryID || receipt.AmountMinor != in.AmountMinor || receipt.Reference != in.Reference || receipt.Note != in.Note {
			return nil, false, ErrConflict
		}
		return &receipt, false, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, err
	}
	var recovery commissionRecoveryRow
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", recoveryID).First(&recovery).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, false, ErrNotFound
		}
		return nil, false, err
	}
	var duplicate int64
	if err := tx.Model(&RecoveryReceipt{}).Where("recovery_id = ? AND reference = ?", recoveryID, in.Reference).Count(&duplicate).Error; err != nil {
		return nil, false, err
	}
	if duplicate > 0 || recovery.Status != "pending" {
		return nil, false, ErrConflict
	}
	if in.AmountMinor > recovery.AmountMinor-recovery.RecoveredMinor {
		return nil, false, ErrInvalidAmount
	}
	receipt = RecoveryReceipt{ID: id.New("crr"), RecoveryID: recoveryID, AmountMinor: in.AmountMinor, Reference: in.Reference, Note: in.Note, ActorUserID: actor, IdempotencyKey: in.IdempotencyKey, CreatedAt: time.Now().UTC()}
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
