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

func originalCurrencyFacts(ctx context.Context, tx *gorm.DB, op string, from, to time.Time, u usage.FinancialSnapshot, platform bool) ([]OriginalCurrencyTotal, error) {
	totals := map[string]OriginalCurrencyTotal{}
	type transfer struct {
		Currency        string
		Income, Expense ledger.Decimal
	}
	rows := []transfer{}
	// Read original amounts once from the owning facts. Raw verified payments
	// remain visible during internal accounting recovery, then their domain
	// observations replace them. A misdirected refund is an expense for the sender
	// and never income for its intended buyer. Unknown payment times use the
	// original observation time, so later data entry does not move the flow.
	err := tx.WithContext(ctx).Raw(`SELECT currency,sum(income)::text AS income,sum(expense)::text AS expense FROM (
 SELECT currency,original_amount::numeric AS income,0::numeric AS expense FROM commerce_receipts WHERE operator_id=? AND COALESCE(paid_at,observed_at)>=? AND COALESCE(paid_at,observed_at)<?
 UNION ALL SELECT fact->'money'->>'currency',(fact->'money'->>'amount')::numeric,0::numeric FROM payments_verified_facts f WHERE f.operator_id=? AND f.kind='receipt' AND COALESCE((fact->>'occurred_at')::timestamptz,f.observed_at)>=? AND COALESCE((fact->>'occurred_at')::timestamptz,f.observed_at)<? AND NOT EXISTS(SELECT 1 FROM commerce_receipts r WHERE r.id::text=f.receipt_id)
 UNION ALL SELECT p.currency,0::numeric,p.amount::numeric FROM remediation_payment_observations p JOIN remediation_refund_legs l ON l.id=p.leg_id WHERE l.operator_id=? AND p.occurred_at>=? AND p.occurred_at<?
 UNION ALL SELECT o.money->>'currency',0::numeric,(o.money->>'amount')::numeric FROM cooperation_procurement_refund_observations o JOIN cooperation_procurement_exit_refunds l ON l.id=o.leg_id LEFT JOIN payments_verified_facts f ON f.id=o.fact_id WHERE l.supplier_id=? AND COALESCE(o.occurred_at,f.observed_at,o.created_at)>=? AND COALESCE(o.occurred_at,f.observed_at,o.created_at)<?
 UNION ALL SELECT o.money->>'currency',(o.money->>'amount')::numeric,0::numeric FROM cooperation_procurement_refund_observations o JOIN cooperation_procurement_exit_refunds l ON l.id=o.leg_id LEFT JOIN payments_verified_facts f ON f.id=o.fact_id WHERE l.buyer_id=? AND o.recipient_matches AND COALESCE(o.occurred_at,f.observed_at,o.created_at)>=? AND COALESCE(o.occurred_at,f.observed_at,o.created_at)<?
 UNION ALL SELECT fact->'money'->>'currency',0::numeric,(fact->'money'->>'amount')::numeric FROM payments_verified_facts f WHERE f.operator_id=? AND f.kind='refund' AND COALESCE((fact->>'occurred_at')::timestamptz,f.observed_at)>=? AND COALESCE((fact->>'occurred_at')::timestamptz,f.observed_at)<? AND NOT EXISTS(SELECT 1 FROM remediation_payment_observations o WHERE o.business_key='provider:'||f.id) AND NOT EXISTS(SELECT 1 FROM cooperation_procurement_refund_observations o WHERE o.fact_id=f.id)
 UNION ALL SELECT f.proof->'money'->>'currency',0::numeric,(f.proof->'money'->>'amount')::numeric FROM cooperation_manual_receipt_refund_facts f JOIN cooperation_procurement_exit_refunds l ON l.id=f.leg_id WHERE l.supplier_id=? AND (f.proof->>'executed_at')::timestamptz>=? AND (f.proof->>'executed_at')::timestamptz<? AND NOT EXISTS(SELECT 1 FROM cooperation_procurement_refund_observations o WHERE o.fact_id=f.fact_id)
 UNION ALL SELECT payload->'proof'->>'currency',0::numeric,(payload->'proof'->>'original_amount')::numeric FROM incentives_events WHERE operator_id=? AND kind='withdrawal.payment_verified' AND (payload->'proof'->>'paid_at')::timestamptz>=? AND (payload->'proof'->>'paid_at')::timestamptz<?
 UNION ALL SELECT r.currency,0::numeric,r.original_amount::numeric FROM commerce_receipts r JOIN (SELECT a.receipt_id,min(o.buyer_id::text) AS buyer_id FROM cooperation_purchase_allocations a JOIN cooperation_purchase_orders o ON o.id=a.order_id GROUP BY a.receipt_id HAVING count(DISTINCT o.buyer_id)=1) own ON own.receipt_id=r.id WHERE own.buyer_id=? AND COALESCE(r.paid_at,r.observed_at)>=? AND COALESCE(r.paid_at,r.observed_at)<? AND (SELECT sum(a.amount_money) FROM cooperation_purchase_allocations a WHERE a.receipt_id=r.id)=r.gross_money AND NOT EXISTS(SELECT 1 FROM commerce_receipt_allocations a WHERE a.receipt_id=r.id)
 UNION ALL SELECT s.currency,s.original_amount::numeric,0::numeric FROM remediation_takeover_settlements s JOIN remediation_takeovers t ON t.id=s.takeover_id WHERE (CASE WHEN t.settlement_due_money>=0 THEN t.brand_operator_id ELSE t.channel_id END)=? AND s.occurred_at>=? AND s.occurred_at<?
 UNION ALL SELECT s.currency,0::numeric,s.original_amount::numeric FROM remediation_takeover_settlements s JOIN remediation_takeovers t ON t.id=s.takeover_id WHERE (CASE WHEN t.settlement_due_money>=0 THEN t.channel_id ELSE t.brand_operator_id END)=? AND s.occurred_at>=? AND s.occurred_at<?
 ) transfers GROUP BY currency ORDER BY currency`, op, from, to, op, from, to, op, from, to, op, from, to, op, from, to, op, from, to, op, from, to, op, from, to, op, from, to, op, from, to, op, from, to).Scan(&rows).Error
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
 SELECT currency,original_fee_amount AS fee,original_net_amount AS net FROM commerce_receipts WHERE operator_id=? AND COALESCE(paid_at,observed_at)>=? AND COALESCE(paid_at,observed_at)<?
 UNION ALL SELECT fact->'money'->>'currency',NULL::numeric,NULL::numeric FROM payments_verified_facts f WHERE f.operator_id=? AND f.kind='receipt' AND COALESCE((fact->>'occurred_at')::timestamptz,f.observed_at)>=? AND COALESCE((fact->>'occurred_at')::timestamptz,f.observed_at)<? AND NOT EXISTS(SELECT 1 FROM commerce_receipts r WHERE r.id::text=f.receipt_id)
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
		r.Expense = r.Expense.Add(f.ReceiptFees)
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
