package billing

import (
	"context"
	"errors"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/platform/id"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	idemCommissionCash    = "comm-cash:"
	idemCommissionCashRev = "comm-cash-rev:"
)

// GrantGift 增加可抵 API 的赠送积分（available 与 gift 同步增加）。不可退款。
func (s *Service) GrantGift(ctx context.Context, userID, idempotencyKey string, amount int64) error {
	if amount <= 0 || idempotencyKey == "" || userID == "" {
		return ErrInvalidAmount
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return creditGift(tx, userID, amount, EventGiftCredit, "gift", idempotencyKey, "gift:"+idempotencyKey)
	})
}

func creditGift(tx *gorm.DB, userID string, amount int64, event, refType, refID, idem string) error {
	var existing ledgerRow
	if err := tx.Where("idempotency_key = ?", idem).First(&existing).Error; err == nil {
		return nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	wallet, err := lockWallet(tx, userID)
	if err != nil {
		return err
	}
	wallet.AvailableMinor += amount
	wallet.GiftMinor += amount
	wallet.Version++
	wallet.UpdatedAt = time.Now().UTC()
	if err := tx.Save(wallet).Error; err != nil {
		return err
	}
	return writeLedger(tx, wallet, event, amount, refType, refID, idem)
}

// CreditCommissionTx 把已解冻佣金记入佣金钱包。必须与佣金解冻同一事务。
func (s *Service) CreditCommissionTx(tx *gorm.DB, userID, entryID string, amount int64) error {
	if amount <= 0 || entryID == "" {
		return nil
	}
	if userID == "" {
		return ErrInvalidAmount
	}
	idem := idemCommissionCash + entryID
	var existing ledgerRow
	if err := tx.Where("idempotency_key = ?", idem).First(&existing).Error; err == nil {
		return nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	wallet, err := lockWallet(tx, userID)
	if err != nil {
		return err
	}
	wallet.CommissionAvailableMinor += amount
	wallet.Version++
	wallet.UpdatedAt = time.Now().UTC()
	if err := tx.Save(wallet).Error; err != nil {
		return err
	}
	return writeLedger(tx, wallet, EventCommissionCredit, amount, "commission_entry", entryID, idem)
}

// ReverseCommissionTx 冲正已入佣金钱包的分录。未入账则空操作。
func (s *Service) ReverseCommissionTx(tx *gorm.DB, entryID string) error {
	if entryID == "" {
		return nil
	}
	revIdem := idemCommissionCashRev + entryID
	var existing ledgerRow
	if err := tx.Where("idempotency_key = ?", revIdem).First(&existing).Error; err == nil {
		return nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	var credit ledgerRow
	if err := tx.Where("idempotency_key = ?", idemCommissionCash+entryID).First(&credit).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	amount := credit.AmountMinor
	if amount <= 0 {
		return nil
	}
	var payout ledgerRow
	if err := tx.Where("idempotency_key = ?", "comm-cash-payout:"+entryID).First(&payout).Error; err == nil {
		// Cash already left this wallet. Track recovery without charging the wallet twice.
		recovery := commissionRecoveryRow{ID: id.New("ccr"), CommissionEntryID: entryID, WalletID: credit.WalletID, SettlementID: payout.ReferenceID, CreditLedgerID: credit.ID, PayoutLedgerID: payout.ID, AmountMinor: amount, Status: "pending", CreatedAt: time.Now().UTC()}
		return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "commission_entry_id"}}, DoNothing: true}).Create(&recovery).Error
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	wallet, err := lockWalletByID(tx, credit.WalletID)
	if err != nil {
		return err
	}
	if wallet.CommissionAvailableMinor < amount {
		return ErrInsufficientBalance
	}
	wallet.CommissionAvailableMinor -= amount
	wallet.Version++
	wallet.UpdatedAt = time.Now().UTC()
	if err := tx.Save(wallet).Error; err != nil {
		return err
	}
	return writeLedger(tx, wallet, EventCommissionDebit, -amount, "commission_entry", entryID, revIdem)
}

// PayoutCommissionTx records an actual manual payout against the original cash credit.
// The caller commits the settlement receipt and this debit in the same transaction.
func (s *Service) PayoutCommissionTx(tx *gorm.DB, entryID, settlementID string, amount int64) error {
	if entryID == "" || settlementID == "" || amount <= 0 {
		return ErrInvalidAmount
	}
	idem := "comm-cash-payout:" + entryID
	var existing ledgerRow
	if err := tx.Where("idempotency_key = ?", idem).First(&existing).Error; err == nil {
		if existing.ReferenceID != settlementID || existing.AmountMinor != -amount {
			return ErrConflict
		}
		return nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	var credit ledgerRow
	if err := tx.Where("idempotency_key = ?", idemCommissionCash+entryID).First(&credit).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		return err
	}
	if credit.AmountMinor != amount {
		return ErrConflict
	}
	var reversed int64
	if err := tx.Model(&ledgerRow{}).Where("idempotency_key = ?", idemCommissionCashRev+entryID).Count(&reversed).Error; err != nil {
		return err
	}
	if reversed > 0 {
		return ErrConflict
	}
	wallet, err := lockWalletByID(tx, credit.WalletID)
	if err != nil {
		return err
	}
	if wallet.CommissionAvailableMinor < amount {
		return ErrInsufficientBalance
	}
	wallet.CommissionAvailableMinor -= amount
	wallet.Version++
	wallet.UpdatedAt = time.Now().UTC()
	if err := tx.Save(wallet).Error; err != nil {
		return err
	}
	return writeLedger(tx, wallet, EventCommissionPayout, -amount, "commission_settlement", settlementID, idem)
}

type commissionRecoveryRow struct {
	RecoveredMinor    int64     `gorm:"column:recovered_minor"`
	ID                string    `gorm:"column:id;primaryKey"`
	CommissionEntryID string    `gorm:"column:commission_entry_id"`
	WalletID          string    `gorm:"column:wallet_id"`
	SettlementID      string    `gorm:"column:settlement_id"`
	CreditLedgerID    string    `gorm:"column:credit_ledger_id"`
	PayoutLedgerID    string    `gorm:"column:payout_ledger_id"`
	AmountMinor       int64     `gorm:"column:amount_minor"`
	Status            string    `gorm:"column:status"`
	CreatedAt         time.Time `gorm:"column:created_at"`
}

func (commissionRecoveryRow) TableName() string { return "billing_commission_recoveries" }
