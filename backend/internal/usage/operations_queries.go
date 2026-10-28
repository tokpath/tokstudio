package usage

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"gorm.io/gorm"
)

type OperationalUsage struct {
	ID         string         `json:"id"`
	CustomerID string         `json:"customer_id"`
	ModelID    string         `json:"model_id"`
	Operation  string         `json:"operation"`
	Status     string         `json:"status"`
	ActualATG  ledger.Decimal `json:"actual_atg"`
	// EstimateATG is the pre-call customer reservation. Only the platform audit
	// export receives it; downstream operators do not use it to judge metering.
	EstimateATG         *ledger.Decimal `json:"estimate_atg,omitempty"`
	ChargedATG          ledger.Decimal  `json:"charged_atg"`
	CorrectedATG        ledger.Decimal  `json:"corrected_atg"`
	CorrectedChargedATG ledger.Decimal  `json:"corrected_charged_atg"`
	NetActualATG        ledger.Decimal  `json:"net_actual_atg"`
	NetChargedATG       ledger.Decimal  `json:"net_charged_atg"`
	MerchantShortfall   ledger.Decimal  `json:"merchant_shortfall"`
	FulfillmentCostUSD  ledger.Decimal  `json:"fulfillment_cost_usd"`
	SupplierCostUSD     *ledger.Decimal `json:"supplier_cost_usd,omitempty"`
	UnverifiedAttempts  *int64          `json:"unverified_attempts,omitempty"`
	AcceptedAt          time.Time       `json:"accepted_at"`
}

func (s *Service) OperationalRequests(ctx context.Context, tx *gorm.DB, operator, customer string, from, to time.Time, limit int) ([]OperationalUsage, error) {
	if _, e := uuid.Parse(operator); e != nil {
		return nil, e
	}
	if customer != "" {
		if _, e := uuid.Parse(customer); e != nil {
			return nil, e
		}
	}
	if !to.After(from) || to.Sub(from) > 366*24*time.Hour || limit < 1 || limit > 100001 {
		return nil, ErrConflict
	}
	out := []OperationalUsage{}
	e := s.within(ctx, tx, func(t *gorm.DB) error {
		fields := `r.id,r.customer_id,r.model_id,r.operation,r.status,r.actual_atg::text,r.charged_atg::text,r.corrected_atg::text,r.corrected_charged_atg::text,(r.actual_atg-r.corrected_atg)::text AS net_actual_atg,(r.charged_atg-r.corrected_charged_atg)::text AS net_charged_atg,r.merchant_shortfall::text,r.accepted_at`
		q := t.Table("usage_requests r").Where("r.operator_id=? AND r.accepted_at>=? AND r.accepted_at<?", operator, from, to)
		if operator == s.Catalog.PlatformOperatorID {
			fields += `,r.estimate_atg::text,COALESCE(a.cost,0)::text AS fulfillment_cost_usd,COALESCE(a.cost,0)::text AS supplier_cost_usd,COALESCE(a.pending,0) AS unverified_attempts`
			q = q.Joins(`LEFT JOIN LATERAL (SELECT sum(supplier_cost_usd)cost,count(*) FILTER(WHERE cost_status='pending')pending FROM usage_attempts WHERE request_id=r.id)a ON true`)
		} else {
			// A downstream merchant can export its own historical procurement
			// cost, never the platform's provider cost or another supply edge.
			// The accepted settlement belongs to this domain and is immutable.
			fields += `,COALESCE((SELECT sum((p->>'cost_usd')::numeric) FROM jsonb_array_elements(r.settlement->'procurement')p WHERE p->>'buyer_id'=r.operator_id::text),0)::text AS fulfillment_cost_usd`
		}
		q = q.Select(fields)
		if customer != "" {
			q = q.Where("r.customer_id=?", customer)
		}
		return q.Order("r.accepted_at,r.id").Limit(limit).Scan(&out).Error
	})
	return out, e
}
func (s *Service) OperationalPending(ctx context.Context, tx *gorm.DB, operator string) (int64, int64, error) {
	if _, e := uuid.Parse(operator); e != nil {
		return 0, 0, e
	}
	var v struct{ Pending, Unverified int64 }
	e := s.within(ctx, tx, func(t *gorm.DB) error {
		if err := t.Model(&Request{}).Where("operator_id=? AND status IN ('accepted','executing','submitted','awaiting_usage')", operator).Count(&v.Pending).Error; err != nil {
			return err
		}
		if operator != s.Catalog.PlatformOperatorID {
			return nil
		}
		return t.Model(&AttemptRecord{}).Where("cost_status='pending'").Count(&v.Unverified).Error
	})
	return v.Pending, v.Unverified, e
}

// PurgeExpiredResults deletes only temporary encrypted output, never metering or
// financial facts. A terminal result remains addressable as expired metadata.
func (s *Service) PurgeExpiredResults(ctx context.Context, at time.Time, limit int) (int64, error) {
	if limit < 1 || limit > 1000 {
		limit = 100
	}
	// Administrative retention must use the same object-first deletion as the
	// normal sweeper; deleting only metadata would leave private content behind.
	if at.After(s.wallNow()) {
		at = s.wallNow()
	}
	return s.purgeExpiredResults(ctx, at, limit)
}
