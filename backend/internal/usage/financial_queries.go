package usage

import (
	"context"
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
	NetChargedATG                 ledger.Decimal          `json:"net_charged_atg"`
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
		if e := t.Raw(`SELECT count(*),
 COALESCE(sum((settlement#>>'{customer,paid_money}')::numeric),0)::text,
 COALESCE(sum(charged_atg),0)::text, COALESCE(sum(actual_atg),0)::text, COALESCE(sum(merchant_shortfall),0)::text
 FROM usage_requests WHERE operator_id=? AND status='completed' AND completed_at>=? AND completed_at<?`, operator, from, to).
			Row().Scan(&out.CompletedRequests, &out.PaidInvocationMoney, &out.ChargedATG, &out.ActualATG, &out.ShortfallATG); e != nil {
			return e
		}
		if operator != s.Catalog.PlatformOperatorID {
			if e := retailFulfillmentCosts(t, operator, from, to, &out); e != nil {
				return e
			}
		}
		var correction struct{ Paid, Amount, Returned ledger.Decimal }
		if e := t.Table("usage_charge_corrections").Where("operator_id=? AND created_at>=? AND created_at<?", operator, from, to).Select("COALESCE(sum(paid_money),0)::text AS paid,COALESCE(sum(amount_atg),0)::text AS amount,COALESCE(sum(returned_atg),0)::text AS returned").Scan(&correction).Error; e != nil {
			return e
		}
		out.CorrectionPaidMoney = correction.Paid
		out.CorrectionATG = correction.Amount
		out.NetChargedATG = out.ChargedATG.Sub(correction.Returned)
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

// Let PostgreSQL add exact numerators with the same denominator, then quantize
// the combined rational once. Rounding individual gift shares drops small costs.
func retailFulfillmentCosts(t *gorm.DB, operator string, from, to time.Time, out *FinancialSnapshot) error {
	var groups []struct {
		Actual, Cost      ledger.Decimal
		GiftCostNumerator string
	}
	if e := t.Raw(`SELECT r.actual_atg::text AS actual, sum(p.cost_money)::text AS cost,
 COALESCE(sum(p.cost_money*g.gift_atg),0)::text AS gift_cost_numerator
 FROM usage_requests r
 JOIN cooperation_procurement_settlements p ON p.request_id=r.id::text AND p.buyer_id=r.operator_id
 LEFT JOIN LATERAL (
   SELECT sum((source->>'amount')::numeric) AS gift_atg
   FROM jsonb_array_elements(NULLIF(r.settlement#>'{customer,events}','null')) event
   CROSS JOIN LATERAL jsonb_array_elements(NULLIF(event->'sources','null')) source
   WHERE source->>'kind'<>'paid'
 ) g ON r.actual_atg>0
 WHERE r.operator_id=? AND r.status='completed' AND r.completed_at>=? AND r.completed_at<? AND p.cost_money>0
 GROUP BY r.actual_atg`, operator, from, to).Scan(&groups).Error; e != nil {
		return e
	}
	gift := new(big.Rat)
	for _, group := range groups {
		out.OwnRetailFulfillmentCostMoney = out.OwnRetailFulfillmentCostMoney.Add(group.Cost)
		if group.Actual.Sign() > 0 {
			numerator, ok := new(big.Rat).SetString(group.GiftCostNumerator)
			if !ok {
				return ErrConflict
			}
			gift.Add(gift, numerator.Quo(numerator, group.Actual.Rat()))
		}
	}
	out.OwnRetailGiftCostMoney, _ = ledger.Quantize(gift)
	return nil
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
