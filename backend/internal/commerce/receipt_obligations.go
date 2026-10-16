package commerce

import (
	"context"

	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"gorm.io/gorm"
)

type ReceiptFundObligation struct {
	ReceiptID      string         `json:"receipt_id"`
	OperatorID     string         `json:"operator_id"`
	Currency       string         `json:"currency"`
	OriginalAmount string         `json:"original_amount"`
	GrossUSD       ledger.Decimal `json:"gross_usd"`
	UnallocatedUSD ledger.Decimal `json:"unallocated_usd"`
	HeldUSD        ledger.Decimal `json:"held_usd"`
}

// UnsettledReceiptFunds is a trusted lifecycle projection. It follows the
// remaining receipt account balance, including pending refund holds. Gross
// minus allocations would incorrectly keep a fully repaid receipt unresolved.
// Nil IDs means the operator's whole scope; non-nil empty means no sources.
func (s *Service) UnsettledReceiptFunds(ctx context.Context, tx *gorm.DB, operator string, receiptIDs []string) ([]ReceiptFundObligation, error) {
	out := []ReceiptFundObligation{}
	if tx == nil || !validID(operator) || len(receiptIDs) > 1000 {
		return out, ErrInvalidInput
	}
	if _, ok := tx.Statement.ConnPool.(gorm.TxCommitter); !ok {
		return out, ErrInvalidInput
	}
	if receiptIDs != nil && len(receiptIDs) == 0 {
		return out, nil
	}
	q := s.dbFor(ctx, tx).Where("operator_id=?", operator)
	unique := map[string]bool{}
	if receiptIDs != nil {
		for _, id := range receiptIDs {
			if !validID(id) {
				return out, ErrInvalidInput
			}
			unique[id] = true
		}
		q = q.Where("id IN ?", receiptIDs)
	} else {
		q = q.Where("allocated_usd<gross_usd")
	}
	var rows []receiptRow
	if e := q.Order("id").Find(&rows).Error; e != nil {
		return out, e
	}
	if receiptIDs != nil && len(rows) != len(unique) {
		return out, ErrScope
	}
	for _, r := range rows {
		accounts, e := s.ledger.ListAccounts(ctx, tx, operator, r.ID)
		if e != nil {
			return out, e
		}
		found := false
		for _, a := range accounts {
			if a.Purpose != "receipt:unallocated" {
				continue
			}
			found = true
			if a.Unit != "USD" || a.Balance.Sign() < 0 || a.Held.Cmp(a.Balance) > 0 {
				return out, ErrConflict
			}
			if a.Balance.Sign() > 0 {
				out = append(out, ReceiptFundObligation{ReceiptID: r.ID, OperatorID: operator, Currency: r.Currency, OriginalAmount: r.OriginalAmount, GrossUSD: r.GrossUSD, UnallocatedUSD: a.Balance, HeldUSD: a.Held})
			}
		}
		if !found {
			return out, ErrConflict
		}
	}
	return out, nil
}
