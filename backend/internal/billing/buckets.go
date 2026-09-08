package billing

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
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
