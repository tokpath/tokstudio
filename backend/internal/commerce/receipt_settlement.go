package commerce

import (
	"context"
	"strings"

	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"golang.org/x/text/currency"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// RecordReceiptSettlement records the actual asset received. It does not
// revalue the business order or change its customer entitlement.
func (s *Service) RecordReceiptSettlement(ctx context.Context, tx *gorm.DB, receiptID, unit string, amount ledger.Decimal, actor string) error {
	_, currencyErr := currency.ParseISO(unit)
	if !validID(receiptID) || currencyErr != nil || unit == "XXX" || unit == "XTS" || unit != strings.ToUpper(unit) || !amount.Valid() || amount.Sign() < 0 || actor == "" {
		return ErrInvalidInput
	}
	return s.within(ctx, tx, func(tx *gorm.DB) error {
		var r receiptRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&r, "id=?", receiptID).Error; err != nil {
			return err
		}
		if r.SettlementNetAmount != nil {
			if r.SettlementCurrency != unit || r.SettlementNetAmount.Cmp(amount) != 0 {
				return ErrConflict
			}
			return nil
		}
		if amount.Sign() > 0 {
			cash, err := s.ledger.EnsureAccount(ctx, tx, ledger.Account{OperatorID: r.OperatorID, OwnerID: r.OperatorID, Purpose: "control:external_cash", Unit: unit, AllowNegative: true})
			if err != nil {
				return err
			}
			counter, err := s.ledger.EnsureAccount(ctx, tx, ledger.Account{OperatorID: r.OperatorID, OwnerID: r.OperatorID, Purpose: "control:receipt_settlement", Unit: unit, AllowNegative: true})
			if err != nil {
				return err
			}
			if _, err = s.ledger.Post(ctx, tx, ledger.Posting{OperatorID: r.OperatorID, BusinessKey: "receipt-native:" + r.ID, Kind: "provider_settlement", SourceID: r.ID, ActorID: actor, Reason: "已核实收款", Lines: []ledger.Line{{AccountID: cash.ID, Amount: amount.Neg()}, {AccountID: counter.ID, Amount: amount}}}); err != nil {
				return err
			}
		}
		return tx.Model(&r).Updates(map[string]any{"settlement_currency": unit, "settlement_net_amount": amount}).Error
	})
}
