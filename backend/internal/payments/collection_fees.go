package payments

import (
	"context"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/paymentmethods"
	"gorm.io/gorm"
)

// CollectionFee fixes the fee before a business order is committed. Missing
// acceptance is allowed only for a zero-fee rule (e.g. internal integrations).
func CollectionFee(ctx context.Context, tx *gorm.DB, operator, method, accepted string, amount ledger.Decimal, currency string) (*paymentmethods.Quote, error) {
	originalMethod := method
	var binding struct{ PaymentMethodID string }
	if e := tx.WithContext(ctx).Table("payments_merchants").Select("payment_method_id").Where("operator_id=? AND id=?", operator, method).Take(&binding).Error; e != nil {
		return nil, e
	}
	if binding.PaymentMethodID != "" {
		method = binding.PaymentMethodID
	}
	rule, e := paymentmethods.Current(ctx, tx, operator, method)
	if e != nil {
		return nil, e
	}
	if rule.Kind != "" {
		var identity paymentmethods.Identity
		if e := tx.WithContext(ctx).Where("operator_id=? AND method_id=?", operator, rule.MethodID).First(&identity).Error; e != nil {
			return nil, e
		}
		if identity.MerchantID != originalMethod {
			return nil, ErrNotConfigured
		}
	}
	if rule.Paused {
		return nil, ErrNotConfigured
	}
	q, e := paymentmethods.CollectionQuote(rule, amount, currency)
	if e != nil {
		return nil, e
	}
	if accepted != rule.ID && (accepted != "" || q.FeeMoney.Sign() > 0) {
		return nil, paymentmethods.ErrChanged
	}
	return &q, nil
}
func (s *Service) addCollectionFee(ctx context.Context, m Merchant, o Order) (Order, error) {
	principal := o.Settlement()
	if o.FeeQuote == nil {
		q, e := CollectionFee(ctx, s.DB, m.OperatorID, m.ID, "", principal.Amount, principal.Currency)
		if e != nil {
			return o, e
		}
		o.FeeQuote = q
	}
	q := o.FeeQuote
	rule, e := paymentmethods.Version(ctx, s.DB, m.OperatorID, q.RuleID)
	if e != nil {
		return o, e
	}
	expected, e := paymentmethods.CollectionQuote(rule, principal.Amount, principal.Currency)
	if e != nil || q.MethodID != methodIdentity(m) || rule.MethodID != methodIdentity(m) || q.AmountMoney.Cmp(expected.AmountMoney) != 0 || q.FeeMoney.Cmp(expected.FeeMoney) != 0 || q.TotalMoney.Cmp(expected.TotalMoney) != 0 || q.Currency != principal.Currency {
		return o, ErrConflict
	}
	o.SettlementMoney = Money{Currency: principal.Currency, Amount: q.TotalMoney}
	if o.Money.Currency == principal.Currency {
		o.Money.Amount = q.TotalMoney
	}
	return o, nil
}

func methodIdentity(m Merchant) string {
	if m.PaymentMethodID != "" {
		return m.PaymentMethodID
	}
	return m.ID
}
