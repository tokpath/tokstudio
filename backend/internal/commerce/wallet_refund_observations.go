package commerce

import (
	"context"
	"errors"

	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"gorm.io/gorm"
)

// WalletRefundAllocations is a trusted source query for the refund workflow.
// RefundedMoney is actual consumed principal; the rest remains reserved.
func (s *Service) WalletRefundAllocations(ctx context.Context, tx *gorm.DB, op, customer, refund string) ([]WalletFundingAllocation, error) {
	if !walletInput(tx, op, customer, refund) {
		return nil, ErrInvalidInput
	}
	if e := walletScope(tx, op, customer); e != nil {
		return nil, e
	}
	c, found, e := s.latestWalletCommand(tx, op, customer, "refund", refund)
	if e != nil {
		return nil, e
	}
	if !found {
		return nil, ErrNotFound
	}
	return s.fundingAllocations(tx, c)
}

// ObserveWalletRefundAllocation consumes only this payment's actual cleared
// principal. It neither clears the remainder nor posts cash a second time.
func (s *Service) ObserveWalletRefundAllocation(ctx context.Context, tx *gorm.DB, op, customer, refund, allocationID, factID string, amount ledger.Decimal) error {
	if !walletInput(tx, op, customer, refund) || !validID(allocationID) || factID == "" || len(factID) > 500 || !amount.Valid() || amount.Sign() <= 0 {
		return ErrInvalidInput
	}
	if e := walletScope(tx, op, customer); e != nil {
		return e
	}
	c, found, e := s.latestWalletCommand(tx, op, customer, "refund", refund)
	if e != nil {
		return e
	}
	if !found {
		return ErrNotFound
	}
	var a walletAllocation
	if e = tx.First(&a, "id=? AND command_id=?", allocationID, c.ID).Error; e != nil {
		return e
	}
	var old struct{ AmountMoney ledger.Decimal }
	e = tx.Table("commerce_wallet_refund_observations").Where("allocation_id=? AND fact_id=?", allocationID, factID).Take(&old).Error
	if e == nil {
		if old.AmountMoney.Cmp(amount) != 0 {
			return ErrConflict
		}
		return nil
	}
	if !errors.Is(e, gorm.ErrRecordNotFound) {
		return e
	}
	if a.Status != "reserved" || a.AmountMoney.Sub(a.RefundedMoney).Cmp(amount) < 0 {
		return ErrOrderState
	}
	if e = tx.Table("commerce_wallet_refund_observations").Create(map[string]any{"allocation_id": allocationID, "fact_id": factID, "amount_money": amount, "created_at": s.now()}).Error; e != nil {
		return e
	}
	if e = tx.Model(&walletLot{}).Where("id=?", a.LotID).Updates(map[string]any{"reserved_money": gorm.Expr("reserved_money-?", amount), "spent_money": gorm.Expr("spent_money+?", amount)}).Error; e != nil {
		return e
	}
	refunded := a.RefundedMoney.Add(amount)
	status := "reserved"
	if refunded.Cmp(a.AmountMoney) == 0 {
		status = "consumed"
	}
	if e = tx.Model(&walletAllocation{}).Where("id=?", a.ID).Updates(map[string]any{"refunded_money": refunded, "status": status}).Error; e != nil {
		return e
	}
	var n int64
	if e = tx.Model(&walletAllocation{}).Where("command_id=? AND status='reserved'", c.ID).Count(&n).Error; e != nil {
		return e
	}
	commandStatus := "partial"
	if n == 0 {
		commandStatus = "consumed"
	}
	return tx.Model(&walletCommand{}).Where("id=?", c.ID).Update("status", commandStatus).Error
}
