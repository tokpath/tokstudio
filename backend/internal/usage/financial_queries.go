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
	CompletedRequests           int64          `json:"completed_requests"`
	PaidInvocationUSD           ledger.Decimal `json:"paid_invocation_usd"`
	CorrectionPaidUSD           ledger.Decimal `json:"correction_paid_usd"`
	ChargedATG                  ledger.Decimal `json:"charged_atg"`
	ActualATG                   ledger.Decimal `json:"actual_atg"`
	CorrectionATG               ledger.Decimal `json:"correction_atg"`
	ShortfallATG                ledger.Decimal `json:"shortfall_atg"`
	OwnRetailFulfillmentCostUSD ledger.Decimal `json:"own_retail_fulfillment_cost_usd"`
	OwnRetailGiftCostUSD        ledger.Decimal `json:"own_retail_gift_cost_usd"`
	SupplierCostUSD             ledger.Decimal `json:"supplier_cost_usd"`
	PendingSupplierCosts        int64          `json:"pending_supplier_costs"`
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
			costs := map[string]ledger.Decimal{}
			if operator == s.Catalog.PlatformOperatorID {
				ids := make([]string, 0, len(batch))
				for _, r := range batch {
					ids = append(ids, r.ID)
				}
				var rows []struct {
					RequestID string
					Cost      ledger.Decimal
				}
				if e := t.Model(&AttemptRecord{}).Where("request_id IN ? AND cost_status='confirmed' AND cost_recorded_at<?", ids, to).Select("request_id,COALESCE(sum(supplier_cost_usd),0)::text AS cost").Group("request_id").Scan(&rows).Error; e != nil {
					return e
				}
				for _, r := range rows {
					costs[r.RequestID] = r.Cost
				}
			}
			for _, r := range batch {
				var settlement Settlement
				if e := json.Unmarshal(r.Settlement, &settlement); e != nil {
					return e
				}
				out.CompletedRequests++
				out.PaidInvocationUSD = out.PaidInvocationUSD.Add(settlement.Customer.PaidUSD)
				out.ChargedATG = out.ChargedATG.Add(r.ChargedATG)
				out.ActualATG = out.ActualATG.Add(r.ActualATG)
				out.ShortfallATG = out.ShortfallATG.Add(r.MerchantShortfall)
				cost := ledger.Zero()
				if operator == s.Catalog.PlatformOperatorID {
					cost = costs[r.ID]
				} else {
					for _, p := range settlement.Procurement {
						if p.BuyerID == operator {
							cost = cost.Add(p.CostUSD)
						}
					}
				}
				out.OwnRetailFulfillmentCostUSD = out.OwnRetailFulfillmentCostUSD.Add(cost)
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
		out.OwnRetailGiftCostUSD, _ = ledger.Quantize(giftCost)
		var correction struct{ Paid, Amount, Returned ledger.Decimal }
		if e := t.Table("usage_charge_corrections").Where("operator_id=? AND created_at>=? AND created_at<?", operator, from, to).Select("COALESCE(sum(paid_usd),0)::text AS paid,COALESCE(sum(amount_atg),0)::text AS amount,COALESCE(sum(returned_atg),0)::text AS returned").Scan(&correction).Error; e != nil {
			return e
		}
		out.CorrectionPaidUSD = correction.Paid
		out.CorrectionATG = correction.Amount
		// The forgiven over-estimate is corrected before any actual customer debit.
		out.ShortfallATG = out.ShortfallATG.Sub(correction.Amount.Sub(correction.Returned))
		if operator == s.Catalog.PlatformOperatorID {
			if e := t.Model(&AttemptRecord{}).Where("cost_status='confirmed' AND cost_recorded_at>=? AND cost_recorded_at<?", from, to).Select("COALESCE(sum(supplier_cost_usd),0)::text").Row().Scan(&out.SupplierCostUSD); e != nil {
				return e
			}
			if e := t.Model(&AttemptRecord{}).Where("cost_status='pending'").Count(&out.PendingSupplierCosts).Error; e != nil {
				return e
			}
		}
		return nil
	})
	return out, e
}
