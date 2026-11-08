package portal

import (
	"context"
	"encoding/json"
	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/paymentmethods"
	"github.com/tokpath/tokstudio/backend/internal/payments"
	"gorm.io/gorm"
)

func collectionFeeForOrder(ctx context.Context, tx *gorm.DB, operator, order string) (*paymentmethods.Quote, error) {
	if order == "" {
		return nil, nil
	}
	var retail checkoutRow
	e := tx.WithContext(ctx).Joins("JOIN commerce_orders o ON o.id=portal_order_checkout.order_id AND o.operator_id=?", operator).Where("portal_order_checkout.order_id=?", order).Limit(1).Find(&retail).Error
	if e != nil {
		return nil, e
	}
	var q *paymentmethods.Quote
	if len(retail.FeeQuote) > 0 {
		e = json.Unmarshal(retail.FeeQuote, &q)
		return q, e
	}
	var wholesale procurementCheckout
	e = tx.WithContext(ctx).Joins("JOIN cooperation_purchase_orders o ON o.id=portal_procurement_checkouts.order_id AND o.supplier_id=?", operator).Where("portal_procurement_checkouts.order_id=?", order).Limit(1).Find(&wholesale).Error
	if e != nil {
		return nil, e
	}
	if len(wholesale.PaymentOrder) > 0 {
		var o payments.Order
		if e = json.Unmarshal(wholesale.PaymentOrder, &o); e != nil {
			return nil, e
		}
		return o.FeeQuote, nil
	}
	return nil, nil
}
func applyManualCollectionFee(ctx context.Context, tx *gorm.DB, operator, order string, fact *commerce.VerifiedReceipt) error {
	q, e := collectionFeeForOrder(ctx, tx, operator, order)
	if e != nil {
		return e
	}
	if q == nil {
		return nil
	}
	if fact.GrossMoney.Cmp(q.TotalMoney) != 0 {
		return nil
	}
	fact.PaymentFeeMoney = q.FeeMoney
	return nil
}

func (a *Application) validateCollectionReceipt(ctx context.Context, tx *gorm.DB, op, order string, r *commerce.Receipt) error {
	q, e := collectionFeeForOrder(ctx, tx, op, order)
	if e != nil {
		return e
	}
	if q == nil {
		return nil
	}
	if r.GrossMoney.Cmp(q.TotalMoney) != 0 {
		return commerce.ErrWholePayment
	}
	if r.PaymentFeeMoney.Cmp(q.FeeMoney) == 0 {
		return nil
	}
	if r.Provider != "manual" || !r.PaymentFeeMoney.IsZero() || !r.AllocatedMoney.IsZero() {
		return commerce.ErrWholePayment
	}
	if e = a.Commerce.AssignManualPaymentFee(ctx, tx, op, r.ID, q.FeeMoney); e != nil {
		return e
	}
	r.PaymentFeeMoney = q.FeeMoney
	return nil
}
