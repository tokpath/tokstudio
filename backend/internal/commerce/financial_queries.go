package commerce

import (
	"context"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"gorm.io/gorm"
)

// FinancialSnapshot separates sales from wallet deposits. Neither is itself
// consumption revenue. Current exceptions are not limited to the flow window.
type FinancialSnapshot struct {
	CompletedSales             int64          `json:"completed_sales"`
	SalesMoney                 ledger.Decimal `json:"sales_money"`
	WalletTopUps               int64          `json:"wallet_top_ups"`
	WalletTopUpMoney           ledger.Decimal `json:"wallet_top_up_money"`
	VerifiedReceiptGrossMoney  ledger.Decimal `json:"verified_receipt_gross_money"`
	PendingReceiptFees         int64          `json:"pending_receipt_fees"`
	PendingProviderSettlements int64          `json:"pending_provider_settlements"`
	UnknownPaymentTimes        int64          `json:"unknown_payment_times"`
	PendingDeliveries          int64          `json:"pending_deliveries"`
}

// FinancialFacts is a trusted read for an authorized repeatable-read report.
// The immutable payment journal identifies the merchant at the actual sale;
// moving current order ownership during a takeover cannot move past sales.
func (s *Service) FinancialFacts(ctx context.Context, tx *gorm.DB, operator string, from, to time.Time) (FinancialSnapshot, error) {
	var out FinancialSnapshot
	if !validID(operator) || from.IsZero() || !to.After(from) || to.Sub(from) > 366*24*time.Hour {
		return out, ErrInvalidInput
	}
	err := s.within(ctx, tx, func(t *gorm.DB) error {
		var rows []struct {
			Kind   string
			Count  int64
			Amount ledger.Decimal
		}
		if e := t.Raw(`SELECT o.kind,count(*) AS count,sum(o.due_money)::text AS amount FROM commerce_orders o JOIN commerce_deliveries d ON d.order_id=o.id WHERE d.delivered_at>=? AND d.delivered_at<? AND EXISTS(SELECT 1 FROM ledger_journals j WHERE j.operator_id=? AND j.source_id=o.id::text AND j.kind IN ('package_payment','wallet_topup')) GROUP BY o.kind`, from, to, operator).Scan(&rows).Error; e != nil {
			return e
		}
		for _, r := range rows {
			if r.Kind == "topup" {
				out.WalletTopUps += r.Count
				out.WalletTopUpMoney = out.WalletTopUpMoney.Add(r.Amount)
			} else {
				out.CompletedSales += r.Count
				out.SalesMoney = out.SalesMoney.Add(r.Amount)
			}
		}
		if e := t.Model(&receiptRow{}).Where("operator_id=? AND created_at>=? AND created_at<?", operator, from, to).Select("COALESCE(sum(gross_money),0)::text").Row().Scan(&out.VerifiedReceiptGrossMoney); e != nil {
			return e
		}
		for _, q := range []struct {
			Where  string
			Target *int64
		}{{"fee_status='pending'", &out.PendingReceiptFees}, {"settled_at IS NULL", &out.PendingProviderSettlements}, {"paid_at IS NULL", &out.UnknownPaymentTimes}} {
			if e := t.Model(&receiptRow{}).Where("operator_id=?", operator).Where(q.Where).Count(q.Target).Error; e != nil {
				return e
			}
		}
		return t.Model(&orderRow{}).Where("operator_id=? AND status IN ('paid','exception')", operator).Count(&out.PendingDeliveries).Error
	})
	return out, err
}
