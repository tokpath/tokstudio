package portal

import (
	"context"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"gorm.io/gorm"
)

const (
	paymentExpiryEvidence       = "付款期限已过且核实没有未完成的线上付款或到账记录，自动关闭订单"
	legacyPaymentExpiryEvidence = "超过付款期限仍未记下到账，系统自动取消并释放占用"
)

const customerOrderReceiptSQL = `EXISTS (SELECT 1 FROM payments_verified_facts f
 WHERE f.order_id=commerce_orders.id::text AND f.kind='receipt'
 AND f.operator_id IN (commerce_orders.operator_id::text, commerce_orders.original_operator_id::text))`

// An old worker may record its final closures while the new build rolls out.
const customerOrderClosedForExpirySQL = `(commerce_orders.status='closed' AND (
 commerce_orders.closed_for_payment_expiry OR EXISTS (SELECT 1 FROM commerce_events e
 WHERE e.aggregate_id=commerce_orders.id AND e.kind='order.closed'
 AND e.payload->>'evidence' IN ('` + paymentExpiryEvidence + `','` + legacyPaymentExpiryEvidence + `'))))`

// The deadline ends customer checkout. The persisted order stays available
// for outstanding payments and holds until reconciliation establishes closure.
const customerOrderStatusSQL = `CASE
 WHEN ` + customerOrderClosedForExpirySQL + ` THEN 'expired'
 WHEN commerce_orders.status='pending' AND commerce_orders.expires_at<=statement_timestamp()
  AND commerce_orders.external_allocated_money=0 AND commerce_orders.internal_status<>'captured'
  AND NOT ` + customerOrderReceiptSQL + ` THEN 'expired'
 ELSE commerce_orders.status END`

type customerOrderPayment struct {
	OrderID, Status, PaymentState, PaymentDriver string
	DeadlinePassed, HasVerifiedReceipt           bool
}

func (p customerOrderPayment) canReconcile() bool {
	if (p.Status != "pending" && p.Status != "expired") || p.PaymentDriver == "" || p.PaymentDriver == "manual" || p.HasVerifiedReceipt {
		return false
	}
	switch p.PaymentState {
	case "created", "executing", "outcome_unknown":
		return true
	default:
		return false
	}
}

func (a *Application) customerOrderPayments(ctx context.Context, customerID string, orders []commerce.Order) (map[string]customerOrderPayment, error) {
	out := make(map[string]customerOrderPayment, len(orders))
	if len(orders) == 0 {
		return out, nil
	}
	ids := make([]string, len(orders))
	for i, o := range orders {
		ids[i] = o.ID
	}
	var rows []customerOrderPayment
	err := a.DB.WithContext(ctx).Table("commerce_orders").
		Select(`commerce_orders.id AS order_id, `+customerOrderStatusSQL+` AS status, `+customerOrderReceiptSQL+` AS has_verified_receipt,
 commerce_orders.expires_at<=statement_timestamp() AS deadline_passed,
 COALESCE(i.state,'') AS payment_state,
 CASE WHEN i.operator_id=commerce_orders.operator_id::text THEN COALESCE(m.driver,'') ELSE '' END AS payment_driver`).
		Joins("LEFT JOIN portal_order_checkout c ON c.order_id=commerce_orders.id AND c.customer_id=commerce_orders.customer_id").
		Joins(`LEFT JOIN payments_intents i ON i.order_id=commerce_orders.id::text AND i.merchant_id=c.merchant_id::text
 AND i.operator_id IN (commerce_orders.operator_id::text, commerce_orders.original_operator_id::text)`).
		Joins("LEFT JOIN payments_merchants m ON m.id=i.merchant_id").
		Where("commerce_orders.customer_id=? AND commerce_orders.id IN ?", customerID, ids).Find(&rows).Error
	if err != nil {
		return nil, err
	}
	if len(rows) != len(orders) {
		return nil, commerce.ErrNotFound
	}
	for _, row := range rows {
		out[row.OrderID] = row
	}
	return out, nil
}

func customerOrderDeadlinePassed(ctx context.Context, db *gorm.DB, deadline time.Time) (bool, error) {
	var passed bool
	err := db.WithContext(ctx).Raw("SELECT ?::timestamptz <= clock_timestamp()", deadline).Scan(&passed).Error
	return passed, err
}

func requireOpenCustomerOrder(ctx context.Context, db *gorm.DB, o commerce.Order) error {
	if o.Status == "closed" {
		var expired bool
		err := db.WithContext(ctx).Table("commerce_orders").Select(customerOrderClosedForExpirySQL).Where("id=? AND customer_id=?", o.ID, o.CustomerID).Scan(&expired).Error
		if err != nil {
			return err
		}
		if expired {
			return commerce.ErrOrderExpired
		}
	}
	if o.Status != "pending" || !o.ExternalAllocatedMoney.IsZero() || o.InternalStatus == "captured" {
		return commerce.ErrOrderState
	}
	passed, err := customerOrderDeadlinePassed(ctx, db, o.ExpiresAt)
	if err != nil {
		return err
	}
	if passed {
		return commerce.ErrOrderExpired
	}
	return nil
}
