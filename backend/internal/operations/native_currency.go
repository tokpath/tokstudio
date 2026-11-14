package operations

import (
	"context"
	"sort"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/usage"
	"gorm.io/gorm"
)

// OriginalCurrencyTotal preserves distinct bases. Income and expense are actual
// external transfers, not recognized revenue or inferred settlement equivalents.
type OriginalCurrencyTotal struct {
	Currency              string         `json:"currency"`
	Income                ledger.Decimal `json:"income"`
	Expense               ledger.Decimal `json:"expense"`
	ReceiptFees           ledger.Decimal `json:"receipt_fees"`
	NetReceipts           ledger.Decimal `json:"net_receipts"`
	PendingReceiptFees    int64          `json:"pending_receipt_fees"`
	SupplierReportedCost  ledger.Decimal `json:"supplier_reported_cost"`
	SupplierReferenceCost ledger.Decimal `json:"supplier_reference_cost"`
	SupplierActualCost    ledger.Decimal `json:"supplier_actual_cost"`
}

// A posted receipt supersedes its provider observation. Before posting, a
// confirmed provider net still represents cash in its original currency.
const recordedReceiptInflows = `
 SELECT settlement_currency AS currency,settlement_net_amount AS amount,COALESCE(paid_at,observed_at) AS paid_at
 FROM commerce_receipts WHERE operator_id=? AND settlement_net_amount IS NOT NULL
 UNION ALL
 SELECT f.fact->'merchant_net'->>'currency',(f.fact->'merchant_net'->>'amount')::numeric,
   COALESCE((f.fact->>'occurred_at')::timestamptz,f.observed_at)
 FROM payments_verified_facts f WHERE f.operator_id=? AND f.kind='receipt'
  AND f.fact->'merchant_net'->>'currency' IS NOT NULL
  AND NOT EXISTS(SELECT 1 FROM commerce_receipts r WHERE r.id::text=f.receipt_id)`

func settlementCurrencyReceiptCash(ctx context.Context, tx *gorm.DB, op, currency string, from, to time.Time) (received, closing ledger.Decimal, err error) {
	err = tx.WithContext(ctx).Raw(`SELECT COALESCE(sum(amount) FILTER(WHERE paid_at>=?),0)::text,COALESCE(sum(amount),0)::text
 FROM (`+recordedReceiptInflows+`) receipts WHERE currency=? AND paid_at<?`, from, op, op, currency, to).Row().Scan(&received, &closing)
	return
}

func originalCurrencyFacts(ctx context.Context, tx *gorm.DB, op string, from, to time.Time, u usage.FinancialSnapshot, platform bool) ([]OriginalCurrencyTotal, error) {
	totals := map[string]OriginalCurrencyTotal{}
	type transfer struct {
		Currency        string
		Income, Expense ledger.Decimal
	}
	rows := []transfer{}
	// A misdirected refund records an expense for the sender alone. Unknown
	// payment times use the initial observation time during accounting recovery.
	err := tx.WithContext(ctx).Raw(`SELECT currency,sum(income)::text AS income,sum(expense)::text AS expense FROM (
 SELECT currency,amount AS income,0::numeric AS expense FROM (`+recordedReceiptInflows+`) receipts WHERE paid_at>=? AND paid_at<?
 UNION ALL SELECT p.currency,0::numeric,p.amount::numeric FROM remediation_payment_observations p JOIN remediation_refund_legs l ON l.id=p.leg_id WHERE l.operator_id=? AND p.occurred_at>=? AND p.occurred_at<?
 UNION ALL SELECT p.currency,p.amount::numeric,0::numeric FROM remediation_payment_observations p JOIN remediation_refund_legs l ON l.id=p.leg_id JOIN remediation_cases c ON c.id=l.case_id JOIN cooperation_purchase_orders o ON o.id=c.order_id WHERE c.kind='procurement_nondelivery' AND o.buyer_id=? AND p.occurred_at>=? AND p.occurred_at<? AND NOT EXISTS(SELECT 1 FROM remediation_external_refund_observations x WHERE x.observation_id=p.id AND lower(x.address)<>lower(x.recipient_address))
 UNION ALL SELECT fact->'money'->>'currency',0::numeric,(fact->'money'->>'amount')::numeric FROM payments_verified_facts f WHERE f.operator_id=? AND f.kind='refund' AND COALESCE((fact->>'occurred_at')::timestamptz,f.observed_at)>=? AND COALESCE((fact->>'occurred_at')::timestamptz,f.observed_at)<? AND NOT EXISTS(SELECT 1 FROM remediation_payment_observations o WHERE o.business_key='provider:'||f.id)
 UNION ALL SELECT payload->'proof'->>'currency',0::numeric,(payload->'proof'->>'original_amount')::numeric FROM incentives_events WHERE operator_id=? AND kind='withdrawal.payment_verified' AND (payload->'proof'->>'paid_at')::timestamptz>=? AND (payload->'proof'->>'paid_at')::timestamptz<?
 UNION ALL SELECT r.currency,0::numeric,r.original_amount::numeric FROM commerce_receipts r JOIN (SELECT a.receipt_id,min(o.buyer_id::text) AS buyer_id FROM cooperation_purchase_allocations a JOIN cooperation_purchase_orders o ON o.id=a.order_id GROUP BY a.receipt_id HAVING count(DISTINCT o.buyer_id)=1) own ON own.receipt_id=r.id WHERE own.buyer_id=? AND COALESCE(r.paid_at,r.observed_at)>=? AND COALESCE(r.paid_at,r.observed_at)<? AND (SELECT sum(a.amount_money) FROM cooperation_purchase_allocations a WHERE a.receipt_id=r.id)=r.gross_money-r.payment_fee_money AND NOT EXISTS(SELECT 1 FROM commerce_receipt_allocations a WHERE a.receipt_id=r.id)
 ) transfers GROUP BY currency ORDER BY currency`, op, op, from, to, op, from, to, op, from, to, op, from, to, op, from, to, op, from, to).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		totals[r.Currency] = OriginalCurrencyTotal{Currency: r.Currency, Income: r.Income, Expense: r.Expense}
	}
	var feeRows []struct {
		Currency                 string
		ReceiptFees, NetReceipts ledger.Decimal
		PendingReceiptFees       int64
	}
	err = tx.WithContext(ctx).Raw(`SELECT currency,COALESCE(sum(fee),0)::text AS receipt_fees,COALESCE(sum(net),0)::text AS net_receipts,count(*) FILTER(WHERE fee IS NULL) AS pending_receipt_fees FROM (
 SELECT COALESCE(NULLIF(r.settlement_currency,''),r.currency) AS currency,CASE WHEN f.fact->'merchant_fee'->>'currency'=r.settlement_currency THEN (f.fact->'merchant_fee'->>'amount')::numeric WHEN r.settlement_currency=r.currency THEN r.original_fee_amount ELSE NULL END AS fee,r.settlement_net_amount AS net FROM commerce_receipts r LEFT JOIN payments_verified_facts f ON f.receipt_id=r.id::text AND f.kind='receipt' WHERE r.operator_id=? AND COALESCE(r.paid_at,r.observed_at)>=? AND COALESCE(r.paid_at,r.observed_at)<?
 UNION ALL SELECT COALESCE(fact->'merchant_net'->>'currency',fact->'money'->>'currency'),CASE WHEN fact->'merchant_fee'->>'currency'=COALESCE(fact->'merchant_net'->>'currency',fact->'money'->>'currency') THEN (fact->'merchant_fee'->>'amount')::numeric ELSE NULL END,(fact->'merchant_net'->>'amount')::numeric FROM payments_verified_facts f WHERE f.operator_id=? AND f.kind='receipt' AND COALESCE((fact->>'occurred_at')::timestamptz,f.observed_at)>=? AND COALESCE((fact->>'occurred_at')::timestamptz,f.observed_at)<? AND NOT EXISTS(SELECT 1 FROM commerce_receipts r WHERE r.id::text=f.receipt_id)
 ) receipts GROUP BY currency`, op, from, to, op, from, to).Scan(&feeRows).Error
	if err != nil {
		return nil, err
	}
	for _, f := range feeRows {
		r := totals[f.Currency]
		r.Currency = f.Currency
		r.ReceiptFees = f.ReceiptFees
		r.NetReceipts = f.NetReceipts
		r.PendingReceiptFees = f.PendingReceiptFees
		// Income is already the actual net received; do not deduct fees again.
		totals[f.Currency] = r
	}
	if platform {
		for _, c := range u.SupplierCosts {
			r := totals[c.Currency]
			r.Currency = c.Currency
			r.SupplierReferenceCost = c.ReferenceCost
			r.SupplierReportedCost = c.ReportedCost
			totals[c.Currency] = r
		}
		var actuals []struct {
			Currency string
			Amount   ledger.Decimal
		}
		if err = tx.WithContext(ctx).Table("model_actual_costs").Select("currency,sum(amount)::text AS amount").Where("created_at>=? AND created_at<?", from, to).Group("currency").Scan(&actuals).Error; err != nil {
			return nil, err
		}
		for _, c := range actuals {
			r := totals[c.Currency]
			r.Currency = c.Currency
			r.SupplierActualCost = c.Amount
			totals[c.Currency] = r
		}
	}
	out := make([]OriginalCurrencyTotal, 0, len(totals))
	for _, r := range totals {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Currency < out[j].Currency })
	return out, nil
}
