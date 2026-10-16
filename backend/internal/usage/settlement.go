package usage

import (
	"context"
	"encoding/json"
	"math/big"

	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"github.com/tokpath/tokstudio/backend/internal/cooperation"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/modelruntime"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Settlement struct {
	Customer    ledger.CreditResult                 `json:"customer"`
	Procurement []cooperation.ProcurementSettlement `json:"procurement"`
	Usage       modelruntime.Usage                  `json:"usage"`
}

func quantities(u modelruntime.Usage) (map[string]string, error) {
	if !u.Reliable || u.Source == "" || len(u.Measurements) == 0 {
		return nil, modelruntime.ErrUsageUnknown
	}
	out := map[string]string{}
	for _, m := range u.Measurements {
		if _, exists := out[m.Component]; exists {
			return nil, modelruntime.ErrUsageUnknown
		}
		if _, e := catalog.PriceDecimal(m.Quantity); e != nil {
			return nil, e
		}
		out[m.Component] = m.Quantity
	}
	return out, nil
}
func (s *Service) rounded(ctx context.Context, t *gorm.DB, operator, owner, scope, request string, exact *big.Rat, unit string) (ledger.Decimal, error) {
	account, e := s.Ledger.EnsureAccount(ctx, t, ledger.Account{OperatorID: operator, OwnerID: owner, Purpose: "control:rounding:" + scope, Unit: unit, AllowNegative: true})
	if e != nil {
		return ledger.Zero(), e
	}
	return s.Ledger.AccumulateCharge(ctx, t, account.ID, scope, request, exact)
}
func (s *Service) observe(ctx context.Context, t *gorm.DB, events []ledger.GrantEvent) error {
	for _, event := range events {
		g, e := s.Ledger.GetGrant(ctx, t, event.GrantID)
		if e != nil {
			return e
		}
		if e = s.GrantObserver.ObserveGrantEvent(ctx, t, g, event); e != nil {
			return e
		}
	}
	return nil
}

// Settle uses the accepted price snapshot even after a price change or key
// reset. All customer/procurement debits and incentive facts share one commit.
func (s *Service) Settle(ctx context.Context, tx *gorm.DB, requestID string, usage modelruntime.Usage) (Request, error) {
	ctx, span := otel.Tracer("tokenhub/usage").Start(ctx, "usage.settle")
	defer span.End()
	var committedWork trace.Span
	defer func() {
		if committedWork != nil {
			committedWork.End()
		}
	}()
	var out Request
	q, e := quantities(usage)
	if e != nil {
		return out, e
	}
	e = s.within(ctx, tx, func(t *gorm.DB) error {
		var observed Request
		if e := t.First(&observed, "id=?", requestID).Error; e != nil {
			return e
		}
		if _, e = s.Organization.LockOwnership(ctx, t, observed.CustomerID, observed.OperatorID, observed.OwnershipVersion); e != nil {
			return e
		}
		if e = s.Cooperation.Lock(ctx, t, observed.OperatorID); e != nil {
			return e
		}
		if e = s.GrantObserver.LockBusiness(ctx, t, observed.OperatorID); e != nil {
			return e
		}
		var key Key
		if e = t.Clauses(clause.Locking{Strength: "UPDATE"}).First(&key, "id=?", observed.KeyID).Error; e != nil {
			return e
		}
		if e = t.Clauses(clause.Locking{Strength: "UPDATE"}).First(&out, "id=?", requestID).Error; e != nil {
			return e
		}
		_, work := otel.Tracer("tokenhub/usage").Start(ctx, "usage.settle.after_locks")
		defer work.End()
		// A caller-owned transaction commits after this method returns, so
		// only report committed work when this method owns the transaction.
		if tx == nil {
			_, committedWork = otel.Tracer("tokenhub/usage").Start(ctx, "usage.settle.committed_work")
		}
		if terminal(out.Status) {
			return nil
		}
		now, clockErr := s.businessTime(t)
		if clockErr != nil {
			return clockErr
		}
		if !now.Before(out.UsageDeadline) {
			return s.release(t, ctx, &out, "usage_unverified_before_deadline", "free")
		}
		var quote catalog.ModelQuote
		if e = json.Unmarshal(out.PriceSnapshot, &quote); e != nil {
			return e
		}
		for _, rate := range quote.Retail.Components {
			found := false
			for _, m := range usage.Measurements {
				if m.Component == rate.Key && m.Unit == rate.Unit {
					found = true
				}
			}
			if !found {
				return modelruntime.ErrUsageUnknown
			}
		}
		exact, e := quote.Retail.Charge(q)
		if e != nil {
			return e
		}
		actual, e := s.rounded(ctx, t, out.OperatorID, out.CustomerID, "retail", out.ID, exact, "ATG")
		if e != nil {
			return e
		}
		result := Settlement{Procurement: []cooperation.ProcurementSettlement{}, Usage: usage}
		if out.CustomerHoldID != nil {
			result.Customer, e = s.Ledger.SettleCredits(ctx, t, out.OperatorID, out.CustomerID, *out.CustomerHoldID, "usage:"+out.ID, actual, now)
			if e != nil {
				return e
			}
		} else {
			result.Customer = ledger.CreditResult{Requested: actual, Shortfall: actual}
		}
		if e = s.observe(ctx, t, result.Customer.Events); e != nil {
			return e
		}
		var holds []SupplyHold
		if e = json.Unmarshal(out.SupplyHolds, &holds); e != nil {
			return e
		}
		for _, price := range quote.Supply {
			var hold *SupplyHold
			for i := range holds {
				if holds[i].BuyerID == price.Scope.BuyerID {
					hold = &holds[i]
				}
			}
			if hold == nil {
				return ErrConflict
			}
			exact, e := price.Charge(q)
			if e != nil {
				return e
			}
			amount, e := s.rounded(ctx, t, price.Scope.BuyerID, price.Scope.BuyerID, "procurement", out.ID, exact, "ATG")
			if e != nil {
				return e
			}
			if hold.HoldID == "" {
				if amount.Sign() > 0 {
					return modelruntime.ErrBoundExceeded
				}
				continue
			}
			settled, e := s.Cooperation.SettleProcurement(ctx, t, hold.BuyerID, out.ID, hold.HoldID, amount, now)
			if e != nil {
				return e
			}
			result.Procurement = append(result.Procurement, settled)
		}
		out.ActualATG = actual
		out.ChargedATG = result.Customer.Debited
		out.MerchantShortfall = result.Customer.Shortfall
		// Use the original window, not the window at completion time. Other settled
		// requests are counted once while this request still has zero actual charge.
		if out.BudgetLimit != nil {
			var spent ledger.Decimal
			query := t.Model(&Request{}).Where("key_id=? AND accepted_at>=?", key.ID, out.WindowStart)
			if out.WindowEnd != nil {
				query = query.Where("accepted_at<?", *out.WindowEnd)
			}
			if e = query.Select("COALESCE(sum(actual_atg-corrected_atg),0)::text").Row().Scan(&spent); e != nil {
				return e
			}
			over := spent.Add(actual).Sub(*out.BudgetLimit)
			previous := spent.Sub(*out.BudgetLimit)
			if previous.Sign() < 0 {
				previous = ledger.Zero()
			}
			if over.Sign() > 0 {
				out.BudgetOverage = over.Sub(previous)
			}
		}
		out.Settlement, _ = json.Marshal(result)
		out.Status = "completed"
		out.CompletedAt = &now
		if e = t.Save(&out).Error; e != nil {
			return e
		}
		expired, e := s.Ledger.ExpireCredits(ctx, t, out.OperatorID, out.CustomerID, now)
		if e != nil {
			return e
		}
		return s.observe(ctx, t, expired)
	})
	return out, e
}
func (s *Service) release(t *gorm.DB, ctx context.Context, out *Request, reason, status string) error {
	if terminal(out.Status) {
		return nil
	}
	if out.CustomerHoldID != nil {
		if _, e := s.Ledger.Release(ctx, t, out.OperatorID, *out.CustomerHoldID, "usage-free:"+out.ID); e != nil {
			return e
		}
	}
	var holds []SupplyHold
	if e := json.Unmarshal(out.SupplyHolds, &holds); e != nil {
		return e
	}
	for _, h := range holds {
		if h.HoldID != "" {
			if _, e := s.Ledger.Release(ctx, t, h.BuyerID, h.HoldID, "usage-free:"+out.ID); e != nil {
				return e
			}
		}
	}
	now, clockErr := s.businessTime(t)
	if clockErr != nil {
		return clockErr
	}
	out.Status = status
	out.Reason = reason
	out.CompletedAt = &now
	if e := t.Save(out).Error; e != nil {
		return e
	}
	expired, e := s.Ledger.ExpireCredits(ctx, t, out.OperatorID, out.CustomerID, now)
	if e != nil {
		return e
	}
	return s.observe(ctx, t, expired)
}
func (s *Service) ResolveTimeout(ctx context.Context, id string) (Request, error) {
	var out Request
	e := s.within(ctx, nil, func(t *gorm.DB) error {
		if e := t.First(&out, "id=?", id).Error; e != nil {
			return e
		}
		if terminal(out.Status) {
			return nil
		}
		now, clockErr := s.businessTime(t)
		if clockErr != nil {
			return clockErr
		}
		if now.Before(out.UsageDeadline) {
			return ErrInProgress
		}
		if _, e := s.Organization.LockOwnership(ctx, t, out.CustomerID, out.OperatorID, out.OwnershipVersion); e != nil {
			return e
		}
		if e := s.Cooperation.Lock(ctx, t, out.OperatorID); e != nil {
			return e
		}
		if e := s.GrantObserver.LockBusiness(ctx, t, out.OperatorID); e != nil {
			return e
		}
		var k Key
		if e := t.Clauses(clause.Locking{Strength: "UPDATE"}).First(&k, "id=?", out.KeyID).Error; e != nil {
			return e
		}
		if e := t.Clauses(clause.Locking{Strength: "UPDATE"}).First(&out, "id=?", id).Error; e != nil {
			return e
		}
		if e := s.release(t, ctx, &out, "usage_unverified_before_deadline", "free"); e != nil {
			return e
		}
		var attempts []AttemptRecord
		if e := t.Where("request_id=?", id).Find(&attempts).Error; e != nil {
			return e
		}
		for _, attempt := range attempts {
			if e := s.Catalog.WithDB(t).ObserveRoute(ctx, attempt.ID, modelruntime.Result{State: "outcome_unknown", ErrorCode: "usage_unverified_before_deadline"}); e != nil {
				return e
			}
		}
		return nil
	})
	return out, e
}

// Every candidate can lose capacity between quote and dispatch. Only absence
// of all durable dispatch intents proves that releasing immediately is safe.
func (s *Service) failUndispatched(ctx context.Context, id string) (Request, error) {
	var out Request
	e := s.within(ctx, nil, func(t *gorm.DB) error {
		if e := t.First(&out, "id=?", id).Error; e != nil {
			return e
		}
		if terminal(out.Status) {
			return nil
		}
		if _, e := s.Organization.LockOwnership(ctx, t, out.CustomerID, out.OperatorID, out.OwnershipVersion); e != nil {
			return e
		}
		if e := s.Cooperation.Lock(ctx, t, out.OperatorID); e != nil {
			return e
		}
		if e := s.GrantObserver.LockBusiness(ctx, t, out.OperatorID); e != nil {
			return e
		}
		var k Key
		if e := t.Clauses(clause.Locking{Strength: "UPDATE"}).First(&k, "id=?", out.KeyID).Error; e != nil {
			return e
		}
		if e := t.Clauses(clause.Locking{Strength: "UPDATE"}).First(&out, "id=?", id).Error; e != nil {
			return e
		}
		var count int64
		if e := t.Model(&AttemptRecord{}).Where("request_id=?", id).Count(&count).Error; e != nil {
			return e
		}
		if count != 0 {
			return ErrInProgress
		}
		return s.release(t, ctx, &out, "route_capacity_or_health_unavailable", "failed")
	})
	return out, e
}
