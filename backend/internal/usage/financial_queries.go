package usage

import (
	"context"
	"encoding/json"
	"math/big"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"gorm.io/gorm"
)

type FinancialSnapshot struct {
	CompletedRequests             int64                   `json:"completed_requests"`
	PaidInvocationMoney           ledger.Decimal          `json:"paid_invocation_money"`
	CorrectionPaidMoney           ledger.Decimal          `json:"correction_paid_money"`
	ChargedATG                    ledger.Decimal          `json:"charged_atg"`
	ActualATG                     ledger.Decimal          `json:"actual_atg"`
	CorrectionATG                 ledger.Decimal          `json:"correction_atg"`
	ShortfallATG                  ledger.Decimal          `json:"shortfall_atg"`
	OwnRetailFulfillmentCostMoney ledger.Decimal          `json:"own_retail_fulfillment_cost_money"`
	OwnRetailGiftCostMoney        ledger.Decimal          `json:"own_retail_gift_cost_money"`
	SupplierCosts                 []SupplierCurrencyCosts `json:"supplier_costs"`
	PendingSupplierCosts          int64                   `json:"pending_supplier_costs"`
}

// FinancialFacts recognizes retail at completion and corrections when posted.
// Platform supplier costs include every verified attempt across all merchants;
// lower operators only receive their own settled procurement cost. Gift cost is
// a subset of retail fulfillment cost, never an additional expense.
func (s *Service) FinancialFacts(ctx context.Context, tx *gorm.DB, operator string, from, to time.Time) (FinancialSnapshot, error) {
	var out FinancialSnapshot
	if _, e := uuid.Parse(operator); e != nil {
		return out, ErrConflict
	}
	if from.IsZero() || !to.After(from) || to.Sub(from) > 366*24*time.Hour {
		return out, ErrConflict
	}
	e := s.within(ctx, tx, func(t *gorm.DB) error {
		// Aggregate gift cost as rational values before a single display rounding.
		giftCost := new(big.Rat)
		var batch []Request
		q := t.Where("operator_id=? AND status='completed' AND completed_at>=? AND completed_at<?", operator, from, to)
		if e := q.FindInBatches(&batch, 500, func(batchTx *gorm.DB, _ int) error {
			for _, r := range batch {
				var settlement Settlement
				if e := json.Unmarshal(r.Settlement, &settlement); e != nil {
					return e
				}
				out.CompletedRequests++
				out.PaidInvocationMoney = out.PaidInvocationMoney.Add(settlement.Customer.PaidMoney)
				out.ChargedATG = out.ChargedATG.Add(r.ChargedATG)
				out.ActualATG = out.ActualATG.Add(r.ActualATG)
				out.ShortfallATG = out.ShortfallATG.Add(r.MerchantShortfall)
				cost := ledger.Zero()
				if operator != s.Catalog.PlatformOperatorID {
					for _, p := range settlement.Procurement {
						if p.BuyerID == operator {
							cost = cost.Add(p.CostMoney)
						}
					}
				}
				out.OwnRetailFulfillmentCostMoney = out.OwnRetailFulfillmentCostMoney.Add(cost)
				gift := ledger.Zero()
				for _, e := range settlement.Customer.Events {
					var sources []ledger.GrantSource
					if err := json.Unmarshal(e.Sources, &sources); err != nil {
						return err
					}
					for _, source := range sources {
						if source.Kind != "paid" {
							gift = gift.Add(source.Amount)
						}
					}
				}
				if r.ActualATG.Sign() > 0 {
					giftCost.Add(giftCost, new(big.Rat).Quo(new(big.Rat).Mul(cost.Rat(), gift.Rat()), r.ActualATG.Rat()))
				}
			}
			return nil
		}).Error; e != nil {
			return e
		}
		out.OwnRetailGiftCostMoney, _ = ledger.Quantize(giftCost)
		var correction struct{ Paid, Amount, Returned ledger.Decimal }
		if e := t.Table("usage_charge_corrections").Where("operator_id=? AND created_at>=? AND created_at<?", operator, from, to).Select("COALESCE(sum(paid_money),0)::text AS paid,COALESCE(sum(amount_atg),0)::text AS amount,COALESCE(sum(returned_atg),0)::text AS returned").Scan(&correction).Error; e != nil {
			return e
		}
		out.CorrectionPaidMoney = correction.Paid
		out.CorrectionATG = correction.Amount
		// The forgiven over-estimate is corrected before any actual customer debit.
		out.ShortfallATG = out.ShortfallATG.Sub(correction.Amount.Sub(correction.Returned))
		if operator == s.Catalog.PlatformOperatorID {
			var err error
			out.SupplierCosts, err = supplierCurrencyCosts(ctx, t, from, to)
			if err != nil {
				return err
			}
			if e := t.Model(&AttemptRecord{}).Where("cost_status='pending'").Count(&out.PendingSupplierCosts).Error; e != nil {
				return e
			}
		}
		return nil
	})
	return out, e
}

// SupplierCurrencyCosts keeps independent cost bases side by side. These
// observations are not expenses and are never summed across currencies.
type SupplierCurrencyCosts struct {
	Currency      string         `json:"currency"`
	ReferenceCost ledger.Decimal `json:"reference_cost"`
	ReportedCost  ledger.Decimal `json:"reported_cost"`
}

func supplierCurrencyCosts(ctx context.Context, tx *gorm.DB, from, to time.Time) ([]SupplierCurrencyCosts, error) {
	out := []SupplierCurrencyCosts{}
	err := tx.WithContext(ctx).Raw(`SELECT currency,ROUND(sum(reference_cost),18)::text AS reference_cost,ROUND(sum(reported_cost),18)::text AS reported_cost FROM (
 SELECT reference_cost_currency AS currency,reference_cost_amount AS reference_cost,0::numeric AS reported_cost FROM usage_attempts WHERE reference_cost_amount IS NOT NULL AND cost_recorded_at>=? AND cost_recorded_at<?
 UNION ALL SELECT reported_cost_currency,0::numeric,reported_cost_amount FROM usage_attempts WHERE reported_cost_amount IS NOT NULL AND cost_recorded_at>=? AND cost_recorded_at<?
 ) costs GROUP BY currency ORDER BY currency`, from, to, from, to).Scan(&out).Error
	return out, err
}
