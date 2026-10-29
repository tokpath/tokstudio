package operations

import (
	"context"
	"database/sql"
	"sort"
	"strings"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/cooperation"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/payments"
	"github.com/tokpath/tokstudio/backend/internal/usage"
	"gorm.io/gorm"
)

// Pending amounts are current at CapturedAt, including old unresolved facts.
// They must never be interpreted as confirmed zero fees or forgiven debt.
type RefundFinancialFacts struct {
	PendingFees       int64          `json:"pending_fees"`
	PendingExecutions int64          `json:"pending_executions"`
	OutstandingMoney  ledger.Decimal `json:"outstanding_money"`
	MismatchCount     int64          `json:"mismatch_count"`
}
type RefundFinancialSource interface {
	FinancialRefundFacts(context.Context, *gorm.DB, string, time.Time, time.Time) (RefundFinancialFacts, error)
}
type RefundFinancialSourceFunc func(context.Context, *gorm.DB, string, time.Time, time.Time) (RefundFinancialFacts, error)

func (f RefundFinancialSourceFunc) FinancialRefundFacts(ctx context.Context, t *gorm.DB, op string, from, to time.Time) (RefundFinancialFacts, error) {
	return f(ctx, t, op, from, to)
}

type FinancialSource interface {
	FinancialReport(context.Context, *gorm.DB, string, time.Time, time.Time) (FinancialReport, error)
}

type PendingPaymentSource interface {
	PendingReceiptObligations(context.Context, *gorm.DB, string, []string) ([]payments.PendingReceiptObligation, error)
}
type PendingOriginalMoney struct {
	Currency string         `json:"currency"`
	Amount   ledger.Decimal `json:"amount"`
	Facts    int64          `json:"facts"`
}
type PaymentFinancialFacts struct {
	PendingReceiptFacts int64                  `json:"pending_receipt_facts"`
	AwaitingSettlement  int64                  `json:"awaiting_settlement"`
	OriginalMoney       []PendingOriginalMoney `json:"original_money"`
}

type FinancialReport struct {
	OperatorID             string                        `json:"operator_id"`
	From                   time.Time                     `json:"from"`
	To                     time.Time                     `json:"to"`
	CapturedAt             time.Time                     `json:"captured_at"`
	Status                 string                        `json:"status"`
	Sales                  commerce.FinancialSnapshot    `json:"sales"`
	Procurement            cooperation.FinancialSnapshot `json:"procurement"`
	Usage                  usage.FinancialSnapshot       `json:"usage"`
	Refunds                RefundFinancialFacts          `json:"refunds"`
	Payments               PaymentFinancialFacts         `json:"payments"`
	Revenue                FinancialRevenue              `json:"revenue"`
	Costs                  FinancialCosts                `json:"costs"`
	Cash                   FinancialCash                 `json:"cash"`
	Liabilities            FinancialLiabilities          `json:"liabilities"`
	SettlementCurrency     string                        `json:"settlement_currency"`
	OriginalCurrencyTotals []OriginalCurrencyTotal       `json:"original_currency_totals"`
	TakeoverFlows          []ledger.PurposeFlow          `json:"takeover_flows"`
	TakeoverBalances       []ledger.PurposeBalance       `json:"takeover_balances"`
	ClosingBalances        []ledger.PurposeBalance       `json:"closing_balances"`
	CurrentHeld            []ledger.PurposeBalance       `json:"current_held"`
	Notes                  []string                      `json:"notes"`
}
type FinancialRevenue struct {
	PaidInvocationMoney         ledger.Decimal `json:"paid_invocation_money"`
	ExpiredUnusedPaidMoney      ledger.Decimal `json:"expired_unused_paid_money"`
	WholesaleConsumedMoney      ledger.Decimal `json:"wholesale_consumed_money"`
	WithdrawalFeesRetainedMoney ledger.Decimal `json:"withdrawal_fees_retained_money"`
	TotalMoney                  ledger.Decimal `json:"total_money"`
}
type FinancialCosts struct {
	FulfillmentMoney      ledger.Decimal `json:"fulfillment_money"`
	RetailGiftSubsetMoney ledger.Decimal `json:"retail_gift_subset_money"`
	CommissionMoney       ledger.Decimal `json:"commission_money"`
	PaymentFeesMoney      ledger.Decimal `json:"payment_fees_money"`
	ExceptionLossMoney    ledger.Decimal `json:"exception_loss_money"`
	TotalMoney            ledger.Decimal `json:"total_money"`
	PendingFees           int64          `json:"pending_fees"`
	PendingSupplierCosts  int64          `json:"pending_supplier_costs"`
}
type FinancialCash struct {
	ClosingRecordedNetMoney  ledger.Decimal `json:"closing_recorded_net_money"`
	RecordedInMoney          ledger.Decimal `json:"recorded_in_money"`
	RecordedOutMoney         ledger.Decimal `json:"recorded_out_money"`
	ProcurementPaymentsMoney ledger.Decimal `json:"procurement_payments_money"`
	CommissionPayoutsMoney   ledger.Decimal `json:"commission_payouts_money"`
	TakeoverInMoney          ledger.Decimal `json:"takeover_in_money"`
	TakeoverOutMoney         ledger.Decimal `json:"takeover_out_money"`
	ProviderReceivableMoney  ledger.Decimal `json:"provider_receivable_money"`
}
type FinancialLiabilities struct {
	WholesalePrepaymentMoney   ledger.Decimal `json:"wholesale_prepayment_money"`
	CustomerWalletMoney        ledger.Decimal `json:"customer_wallet_money"`
	OutstandingGrantPaidMoney  ledger.Decimal `json:"outstanding_grant_paid_money"`
	OutstandingGrantATG        ledger.Decimal `json:"outstanding_grant_atg"`
	CommissionMoney            ledger.Decimal `json:"commission_money"`
	UnfulfilledOrderFundsMoney ledger.Decimal `json:"unfulfilled_order_funds_money"`
	SupplierPayableMoney       ledger.Decimal `json:"supplier_payable_money"`
}

// FinancialReport authorizes one exact merchant scope and reads all domains in
// one repeatable-read snapshot. Missing facts fail visibly instead of returning
// a report made from zero-valued placeholders.
func (s *Service) FinancialReport(ctx context.Context, actor, operator string, from, to time.Time) (FinancialReport, error) {
	var out FinancialReport
	if !validID(operator) || from.IsZero() || !to.After(from) || to.Sub(from) > 366*24*time.Hour {
		return out, ErrInvalid
	}
	source, ok := s.Source.(FinancialSource)
	if !ok {
		return out, ErrNotReady
	}
	e := s.DB.WithContext(ctx).Transaction(func(t *gorm.DB) error {
		if e := s.require(ctx, t, actor, operator, "reports.view"); e != nil {
			return e
		}
		var e error
		out, e = source.FinancialReport(ctx, t, operator, from.UTC(), to.UTC())
		return e
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	return out, e
}

func (d DomainSource) FinancialReport(ctx context.Context, t *gorm.DB, op string, from, to time.Time) (FinancialReport, error) {
	var out FinancialReport
	if t == nil || d.ready() != nil || d.Commerce == nil || d.Refunds == nil || d.Payments == nil || !validID(d.Usage.Catalog.PlatformOperatorID) {
		return out, ErrNotReady
	}
	l, e := d.Ledger.FinancialFacts(ctx, t, op, from, to)
	if e != nil {
		return out, e
	}
	c, e := d.Commerce.FinancialFacts(ctx, t, op, from, to)
	if e != nil {
		return out, e
	}
	p, e := d.Cooperation.FinancialFacts(ctx, t, op, from, to)
	if e != nil {
		return out, e
	}
	u, e := d.Usage.FinancialFacts(ctx, t, op, from, to)
	if e != nil {
		return out, e
	}
	r, e := d.Refunds.FinancialRefundFacts(ctx, t, op, from, to)
	if e != nil {
		return out, e
	}
	out = composeFinancial(op, l, c, p, u, r, op == d.Usage.Catalog.PlatformOperatorID)
	monetary, e := ledger.GetMonetary(ctx, t)
	if e != nil {
		return out, e
	}
	out.SettlementCurrency = monetary.Currency
	out.OriginalCurrencyTotals, e = originalCurrencyFacts(ctx, t, op, from, to, u, op == d.Usage.Catalog.PlatformOperatorID)
	if e != nil {
		return out, e
	}
	facts, e := d.Payments.PendingReceiptObligations(ctx, t, op, nil)
	if e != nil {
		return out, e
	}
	out.Payments = PaymentFinancialFacts{PendingReceiptFacts: int64(len(facts)), OriginalMoney: []PendingOriginalMoney{}}
	amounts := map[string]PendingOriginalMoney{}
	for _, f := range facts {
		if f.State != "awaiting_settlement" {
			continue
		}
		out.Payments.AwaitingSettlement++
		entry := amounts[f.Money.Currency]
		entry.Currency = f.Money.Currency
		entry.Amount = entry.Amount.Add(f.Money.Amount)
		entry.Facts++
		if !entry.Amount.Valid() {
			return out, ErrInvalid
		}
		amounts[entry.Currency] = entry
	}
	for _, a := range amounts {
		out.Payments.OriginalMoney = append(out.Payments.OriginalMoney, a)
	}
	sort.Slice(out.Payments.OriginalMoney, func(i, j int) bool {
		return out.Payments.OriginalMoney[i].Currency < out.Payments.OriginalMoney[j].Currency
	})
	if len(facts) > 0 {
		out.Status = "review_required"
		out.Notes = append(out.Notes, "Verified original-currency payments awaiting settlement confirmation remain current obligations regardless of commercial order expiry; no exchange rate is inferred.")
	}
	return out, nil
}

func composeFinancial(op string, l ledger.FinancialSnapshot, c commerce.FinancialSnapshot, p cooperation.FinancialSnapshot, u usage.FinancialSnapshot, r RefundFinancialFacts, platform bool) FinancialReport {
	out := FinancialReport{OperatorID: op, From: l.From, To: l.To, CapturedAt: l.CapturedAt, Status: "confirmed_facts", Sales: c, Procurement: p, Usage: u, Refunds: r, ClosingBalances: l.Closing, CurrentHeld: l.CurrentHeld, TakeoverFlows: []ledger.PurposeFlow{}, TakeoverBalances: []ledger.PurposeBalance{}}
	out.Revenue.PaidInvocationMoney = u.PaidInvocationMoney.Sub(u.CorrectionPaidMoney)
	out.Revenue.WholesaleConsumedMoney = p.WholesaleEarnedMoney.Sub(p.WholesaleCorrectionMoney)
	for _, e := range l.GrantEvents {
		if e.Kind == "expiry" {
			out.Revenue.ExpiredUnusedPaidMoney = out.Revenue.ExpiredUnusedPaidMoney.Add(e.PaidMoney)
		}
	}
	out.Costs.FulfillmentMoney = p.ProcurementCostMoney.Sub(p.ProcurementCorrectionMoney)
	if platform {
		// Supplier observations and statement costs have separate native-currency bases.
		out.Costs.FulfillmentMoney = ledger.Zero()
	}
	out.Costs.RetailGiftSubsetMoney = u.OwnRetailGiftCostMoney
	out.Costs.PendingFees = c.PendingReceiptFees + p.PendingRefundFees + r.PendingFees
	out.Costs.PendingSupplierCosts = u.PendingSupplierCosts
	out.Cash.ProcurementPaymentsMoney = p.VerifiedProcurementPaymentsMoney
	out.Cash.ClosingRecordedNetMoney = p.ClosingProcurementPaymentsMoney.Neg()
	out.Liabilities.WholesalePrepaymentMoney = p.ClosingWholesalePrepaymentMoney
	for _, f := range l.Flows {
		if f.Unit != ledger.MoneyUnit {
			continue
		}
		takeover := strings.HasPrefix(f.Kind, "takeover_") || strings.Contains(f.Purpose, "takeover")
		if takeover {
			out.TakeoverFlows = append(out.TakeoverFlows, f)
		}
		if f.Purpose == "control:external_cash" {
			out.Cash.RecordedInMoney = out.Cash.RecordedInMoney.Add(f.Decrease)
			out.Cash.RecordedOutMoney = out.Cash.RecordedOutMoney.Add(f.Increase)
			if takeover {
				out.Cash.TakeoverInMoney = out.Cash.TakeoverInMoney.Add(f.Decrease)
				out.Cash.TakeoverOutMoney = out.Cash.TakeoverOutMoney.Add(f.Increase)
			}
		}
		if f.Purpose == "control:external_receipt" {
			out.Cash.RecordedInMoney = out.Cash.RecordedInMoney.Add(f.Increase)
		}
		if f.Purpose == "control:commission_payouts" {
			out.Cash.CommissionPayoutsMoney = out.Cash.CommissionPayoutsMoney.Add(f.Net)
		}
		if takeover {
			continue
		}
		switch f.Purpose {
		case "control:commissions", "control:commission_recoveries":
			out.Costs.CommissionMoney = out.Costs.CommissionMoney.Sub(f.Net)
		case "control:merchant_fees", "control:refund_fees":
			out.Costs.PaymentFeesMoney = out.Costs.PaymentFeesMoney.Sub(f.Net)
		case "control:collection_loss", "control:refund_overpayment", "control:refund_misdirected":
			out.Costs.ExceptionLossMoney = out.Costs.ExceptionLossMoney.Sub(f.Net)
		case "control:withdrawal_fees":
			out.Revenue.WithdrawalFeesRetainedMoney = out.Revenue.WithdrawalFeesRetainedMoney.Add(f.Net)
		}
	}
	out.Cash.RecordedOutMoney = out.Cash.RecordedOutMoney.Add(out.Cash.ProcurementPaymentsMoney).Add(out.Cash.CommissionPayoutsMoney)
	for _, b := range l.Closing {
		if strings.Contains(b.Purpose, "takeover") {
			out.TakeoverBalances = append(out.TakeoverBalances, b)
			continue
		}
		if b.Unit != ledger.MoneyUnit {
			continue
		}
		switch b.Purpose {
		case "control:external_cash":
			out.Cash.ClosingRecordedNetMoney = out.Cash.ClosingRecordedNetMoney.Sub(b.Balance)
		case "control:external_receipt":
			out.Cash.ClosingRecordedNetMoney = out.Cash.ClosingRecordedNetMoney.Add(b.Balance)
		case "control:commission_payouts":
			out.Cash.ClosingRecordedNetMoney = out.Cash.ClosingRecordedNetMoney.Sub(b.Balance)
		case "customer:money":
			out.Liabilities.CustomerWalletMoney = out.Liabilities.CustomerWalletMoney.Add(b.Balance)
		case "commission:available", "commission:frozen":
			out.Liabilities.CommissionMoney = out.Liabilities.CommissionMoney.Add(b.Balance)
		case "order:funds":
			out.Liabilities.UnfulfilledOrderFundsMoney = out.Liabilities.UnfulfilledOrderFundsMoney.Add(b.Balance)
		case "control:supplier":
			out.Liabilities.SupplierPayableMoney = out.Liabilities.SupplierPayableMoney.Sub(b.Balance)
		case "control:provider_receivable":
			out.Cash.ProviderReceivableMoney = out.Cash.ProviderReceivableMoney.Sub(b.Balance)
		}
	}
	out.Liabilities.OutstandingGrantPaidMoney = l.OutstandingGrantPaidMoney
	out.Liabilities.OutstandingGrantATG = l.OutstandingGrantATG
	out.Revenue.TotalMoney = out.Revenue.PaidInvocationMoney.Add(out.Revenue.ExpiredUnusedPaidMoney).Add(out.Revenue.WholesaleConsumedMoney).Add(out.Revenue.WithdrawalFeesRetainedMoney)
	out.Costs.TotalMoney = out.Costs.FulfillmentMoney.Add(out.Costs.CommissionMoney).Add(out.Costs.PaymentFeesMoney).Add(out.Costs.ExceptionLossMoney)
	if out.Costs.PendingFees > 0 || out.Costs.PendingSupplierCosts > 0 {
		out.Status = "pending_costs"
	}
	if r.PendingExecutions+p.PendingRefundExecutions+r.MismatchCount+p.RefundMismatchCount > 0 {
		out.Status = "review_required"
	}
	out.Notes = []string{"按原币分别汇总收支和成本，不做汇率换算或利润计算。", "参考成本、供应商报告成本与登记账单分别展示，不应相加为一个总成本。", "收支按所选期间统计，期末余额截至结束时刻，待处理项反映查询时状态。", "实际账单按登记时间汇总并保留原服务期间；来源不明的费用保持待核实。", "接管款同时保留独立明细和原币收支，采购付款仅在整笔收款可明确归属采购方时纳入该方原币支出。"}
	return out
}
