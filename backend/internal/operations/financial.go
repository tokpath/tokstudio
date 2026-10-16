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
	OutstandingUSD    ledger.Decimal `json:"outstanding_usd"`
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
	AwaitingValuation   int64                  `json:"awaiting_valuation"`
	OriginalMoney       []PendingOriginalMoney `json:"original_money"`
}

type FinancialReport struct {
	OperatorID         string                        `json:"operator_id"`
	From               time.Time                     `json:"from"`
	To                 time.Time                     `json:"to"`
	CapturedAt         time.Time                     `json:"captured_at"`
	Status             string                        `json:"status"`
	Sales              commerce.FinancialSnapshot    `json:"sales"`
	Procurement        cooperation.FinancialSnapshot `json:"procurement"`
	Usage              usage.FinancialSnapshot       `json:"usage"`
	Refunds            RefundFinancialFacts          `json:"refunds"`
	Payments           PaymentFinancialFacts         `json:"payments"`
	Revenue            FinancialRevenue              `json:"revenue"`
	Costs              FinancialCosts                `json:"costs"`
	Cash               FinancialCash                 `json:"cash"`
	Liabilities        FinancialLiabilities          `json:"liabilities"`
	OperatingResultUSD ledger.Decimal                `json:"operating_result_usd"`
	TakeoverFlows      []ledger.PurposeFlow          `json:"takeover_flows"`
	TakeoverBalances   []ledger.PurposeBalance       `json:"takeover_balances"`
	ClosingBalances    []ledger.PurposeBalance       `json:"closing_balances"`
	CurrentHeld        []ledger.PurposeBalance       `json:"current_held"`
	Notes              []string                      `json:"notes"`
}
type FinancialRevenue struct {
	PaidInvocationUSD         ledger.Decimal `json:"paid_invocation_usd"`
	ExpiredUnusedPaidUSD      ledger.Decimal `json:"expired_unused_paid_usd"`
	WholesaleConsumedUSD      ledger.Decimal `json:"wholesale_consumed_usd"`
	WithdrawalFeesRetainedUSD ledger.Decimal `json:"withdrawal_fees_retained_usd"`
	TotalUSD                  ledger.Decimal `json:"total_usd"`
}
type FinancialCosts struct {
	FulfillmentUSD       ledger.Decimal `json:"fulfillment_usd"`
	RetailGiftSubsetUSD  ledger.Decimal `json:"retail_gift_subset_usd"`
	CommissionUSD        ledger.Decimal `json:"commission_usd"`
	PaymentFeesUSD       ledger.Decimal `json:"payment_fees_usd"`
	ExceptionLossUSD     ledger.Decimal `json:"exception_loss_usd"`
	TotalUSD             ledger.Decimal `json:"total_usd"`
	PendingFees          int64          `json:"pending_fees"`
	PendingSupplierCosts int64          `json:"pending_supplier_costs"`
}
type FinancialCash struct {
	ClosingRecordedNetUSD  ledger.Decimal `json:"closing_recorded_net_usd"`
	RecordedInUSD          ledger.Decimal `json:"recorded_in_usd"`
	RecordedOutUSD         ledger.Decimal `json:"recorded_out_usd"`
	ProcurementPaymentsUSD ledger.Decimal `json:"procurement_payments_usd"`
	CommissionPayoutsUSD   ledger.Decimal `json:"commission_payouts_usd"`
	TakeoverInUSD          ledger.Decimal `json:"takeover_in_usd"`
	TakeoverOutUSD         ledger.Decimal `json:"takeover_out_usd"`
	ProviderReceivableUSD  ledger.Decimal `json:"provider_receivable_usd"`
}
type FinancialLiabilities struct {
	WholesalePrepaymentUSD   ledger.Decimal `json:"wholesale_prepayment_usd"`
	CustomerWalletUSD        ledger.Decimal `json:"customer_wallet_usd"`
	OutstandingGrantPaidUSD  ledger.Decimal `json:"outstanding_grant_paid_usd"`
	OutstandingGrantATG      ledger.Decimal `json:"outstanding_grant_atg"`
	CommissionUSD            ledger.Decimal `json:"commission_usd"`
	UnfulfilledOrderFundsUSD ledger.Decimal `json:"unfulfilled_order_funds_usd"`
	SupplierPayableUSD       ledger.Decimal `json:"supplier_payable_usd"`
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
	facts, e := d.Payments.PendingReceiptObligations(ctx, t, op, nil)
	if e != nil {
		return out, e
	}
	out.Payments = PaymentFinancialFacts{PendingReceiptFacts: int64(len(facts)), OriginalMoney: []PendingOriginalMoney{}}
	amounts := map[string]PendingOriginalMoney{}
	for _, f := range facts {
		if f.State != "awaiting_valuation" {
			continue
		}
		out.Payments.AwaitingValuation++
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
		out.Notes = append(out.Notes, "Verified original-currency payments awaiting execution-time valuation are listed by currency, without an invented USD amount. They remain current obligations regardless of commercial order expiry.")
	}
	return out, nil
}

func composeFinancial(op string, l ledger.FinancialSnapshot, c commerce.FinancialSnapshot, p cooperation.FinancialSnapshot, u usage.FinancialSnapshot, r RefundFinancialFacts, platform bool) FinancialReport {
	out := FinancialReport{OperatorID: op, From: l.From, To: l.To, CapturedAt: l.CapturedAt, Status: "confirmed_facts", Sales: c, Procurement: p, Usage: u, Refunds: r, ClosingBalances: l.Closing, CurrentHeld: l.CurrentHeld, TakeoverFlows: []ledger.PurposeFlow{}, TakeoverBalances: []ledger.PurposeBalance{}}
	out.Revenue.PaidInvocationUSD = u.PaidInvocationUSD.Sub(u.CorrectionPaidUSD)
	out.Revenue.WholesaleConsumedUSD = p.WholesaleEarnedUSD.Sub(p.WholesaleCorrectionUSD)
	for _, e := range l.GrantEvents {
		if e.Kind == "expiry" {
			out.Revenue.ExpiredUnusedPaidUSD = out.Revenue.ExpiredUnusedPaidUSD.Add(e.PaidUSD)
		}
	}
	out.Costs.FulfillmentUSD = p.ProcurementCostUSD.Sub(p.ProcurementCorrectionUSD)
	if platform {
		out.Costs.FulfillmentUSD = u.SupplierCostUSD
	}
	out.Costs.RetailGiftSubsetUSD = u.OwnRetailGiftCostUSD
	out.Costs.PendingFees = c.PendingReceiptFees + p.PendingRefundFees + r.PendingFees
	out.Costs.PendingSupplierCosts = u.PendingSupplierCosts
	out.Cash.ProcurementPaymentsUSD = p.VerifiedProcurementPaymentsUSD
	out.Cash.ClosingRecordedNetUSD = p.ClosingProcurementPaymentsUSD.Neg()
	out.Liabilities.WholesalePrepaymentUSD = p.ClosingWholesalePrepaymentUSD
	for _, f := range l.Flows {
		if f.Unit != "USD" {
			continue
		}
		takeover := strings.HasPrefix(f.Kind, "takeover_") || strings.Contains(f.Purpose, "takeover")
		if takeover {
			out.TakeoverFlows = append(out.TakeoverFlows, f)
		}
		if f.Purpose == "control:external_cash" {
			out.Cash.RecordedInUSD = out.Cash.RecordedInUSD.Add(f.Decrease)
			out.Cash.RecordedOutUSD = out.Cash.RecordedOutUSD.Add(f.Increase)
			if takeover {
				out.Cash.TakeoverInUSD = out.Cash.TakeoverInUSD.Add(f.Decrease)
				out.Cash.TakeoverOutUSD = out.Cash.TakeoverOutUSD.Add(f.Increase)
			}
		}
		if f.Purpose == "control:external_receipt" {
			out.Cash.RecordedInUSD = out.Cash.RecordedInUSD.Add(f.Increase)
		}
		if f.Purpose == "control:commission_payouts" {
			out.Cash.CommissionPayoutsUSD = out.Cash.CommissionPayoutsUSD.Add(f.Net)
		}
		if takeover {
			continue
		}
		switch f.Purpose {
		case "control:commissions", "control:commission_recoveries":
			out.Costs.CommissionUSD = out.Costs.CommissionUSD.Sub(f.Net)
		case "control:merchant_fees", "control:refund_fees":
			out.Costs.PaymentFeesUSD = out.Costs.PaymentFeesUSD.Sub(f.Net)
		case "control:collection_loss", "control:refund_overpayment", "control:refund_misdirected":
			out.Costs.ExceptionLossUSD = out.Costs.ExceptionLossUSD.Sub(f.Net)
		case "control:withdrawal_fees":
			out.Revenue.WithdrawalFeesRetainedUSD = out.Revenue.WithdrawalFeesRetainedUSD.Add(f.Net)
		}
	}
	out.Cash.RecordedOutUSD = out.Cash.RecordedOutUSD.Add(out.Cash.ProcurementPaymentsUSD).Add(out.Cash.CommissionPayoutsUSD)
	for _, b := range l.Closing {
		if strings.Contains(b.Purpose, "takeover") {
			out.TakeoverBalances = append(out.TakeoverBalances, b)
			continue
		}
		if b.Unit != "USD" {
			continue
		}
		switch b.Purpose {
		case "control:external_cash":
			out.Cash.ClosingRecordedNetUSD = out.Cash.ClosingRecordedNetUSD.Sub(b.Balance)
		case "control:external_receipt":
			out.Cash.ClosingRecordedNetUSD = out.Cash.ClosingRecordedNetUSD.Add(b.Balance)
		case "control:commission_payouts":
			out.Cash.ClosingRecordedNetUSD = out.Cash.ClosingRecordedNetUSD.Sub(b.Balance)
		case "customer:usd":
			out.Liabilities.CustomerWalletUSD = out.Liabilities.CustomerWalletUSD.Add(b.Balance)
		case "commission:available", "commission:frozen":
			out.Liabilities.CommissionUSD = out.Liabilities.CommissionUSD.Add(b.Balance)
		case "order:funds":
			out.Liabilities.UnfulfilledOrderFundsUSD = out.Liabilities.UnfulfilledOrderFundsUSD.Add(b.Balance)
		case "control:supplier":
			out.Liabilities.SupplierPayableUSD = out.Liabilities.SupplierPayableUSD.Sub(b.Balance)
		case "control:provider_receivable":
			out.Cash.ProviderReceivableUSD = out.Cash.ProviderReceivableUSD.Sub(b.Balance)
		}
	}
	out.Liabilities.OutstandingGrantPaidUSD = l.OutstandingGrantPaidUSD
	out.Liabilities.OutstandingGrantATG = l.OutstandingGrantATG
	out.Revenue.TotalUSD = out.Revenue.PaidInvocationUSD.Add(out.Revenue.ExpiredUnusedPaidUSD).Add(out.Revenue.WholesaleConsumedUSD).Add(out.Revenue.WithdrawalFeesRetainedUSD)
	out.Costs.TotalUSD = out.Costs.FulfillmentUSD.Add(out.Costs.CommissionUSD).Add(out.Costs.PaymentFeesUSD).Add(out.Costs.ExceptionLossUSD)
	out.OperatingResultUSD = out.Revenue.TotalUSD.Sub(out.Costs.TotalUSD)
	if out.Costs.PendingFees > 0 || out.Costs.PendingSupplierCosts > 0 {
		out.Status = "pending_costs"
	}
	if r.PendingExecutions+p.PendingRefundExecutions+r.MismatchCount+p.RefundMismatchCount > 0 {
		out.Status = "review_required"
	}
	out.Notes = []string{"Revenue is paid consumption less corrections, posted unused paid expiry, direct-buyer wholesale consumption less reversals, and explicitly retained withdrawal fees. Sales and wallet deposits are not added again.", "Fulfillment cost is consumed weighted procurement cost, or verified supplier cost for the platform. Gift cost is a subset and is not deducted twice. Pending costs are unknown, not zero.", "Cash shows verified recorded transfers. Procurement payments and net commission payouts are included once; supplier payables are accrued obligations, not confirmed supplier cash payments.", "Takeover settlements and their fees are shown separately and excluded from operating result. This is an operating projection from recorded facts, not a tax or statutory profit statement.", "Flows use the selected window; closing balances use To. Pending statuses and CurrentHeld are current at CapturedAt, including earlier unresolved items."}
	return out
}
