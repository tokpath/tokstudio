package usage

import (
	"context"
	"encoding/json"

	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// reserveSubscriptionRoute grows the original holds only when this route costs
// more to reserve. The caller commits the growth with the route and account
// leases so a failed claim cannot retain an extra reservation.
func (s *Service) reserveSubscriptionRoute(ctx context.Context, t *gorm.DB, out *Request, quote catalog.ModelQuote, selected catalog.RouteSelection) error {
	if selected.SubscriptionOutputTokens == 0 {
		return nil
	}
	quote.Billing, quote.SubscriptionOutputTokens = selected.Capability, selected.SubscriptionOutputTokens
	estimate, err := Estimate(quote, selected.Request)
	if err != nil {
		return err
	}
	targets, err := estimateSupplyReservations(quote, selected.Request)
	if err != nil {
		return err
	}
	var holds []SupplyHold
	if err = json.Unmarshal(out.SupplyHolds, &holds); err != nil {
		return err
	}
	if len(holds) != len(targets) {
		return ErrConflict
	}
	grows := estimate.Cmp(out.EstimateATG) > 0
	for i, target := range targets {
		if holds[i].BuyerID != target.BuyerID {
			return ErrConflict
		}
		grows = grows || target.Maximum.Cmp(holds[i].Maximum) > 0
	}
	if !grows {
		return nil
	}

	if _, err = s.Organization.LockOwnership(ctx, t, out.CustomerID, out.OperatorID, out.OwnershipVersion); err != nil {
		return err
	}
	if out.OperatorID != s.Catalog.PlatformOperatorID {
		if err = s.Cooperation.Lock(ctx, t, out.OperatorID); err != nil {
			return err
		}
	}
	var key Key
	if err = t.Clauses(clause.Locking{Strength: "UPDATE"}).First(&key, "id=?", out.KeyID).Error; err != nil {
		return err
	}
	if err = t.Clauses(clause.Locking{Strength: "UPDATE"}).First(out, "id=?", out.ID).Error; err != nil {
		return err
	}
	now, err := s.businessTime(t)
	if err != nil {
		return err
	}
	if out.Status != "executing" || out.CancelRequested || !out.UsageDeadline.After(now) {
		return catalog.ErrRouteUnavailable
	}
	if estimate.Cmp(out.EstimateATG) > 0 {
		if out.BudgetLimit != nil {
			windows := []keyWindow{{KeyID: out.KeyID, Start: out.WindowStart, End: out.WindowEnd}}
			totals, err := keyBudgetTotals(t, windows)
			if err != nil {
				return err
			}
			budget := totals[out.KeyID]
			if budget.Spent.Add(budget.Held).Add(estimate.Sub(out.EstimateATG)).Cmp(*out.BudgetLimit) > 0 {
				return ErrBudget
			}
		}
		reservation := ledger.CreditReservation{OperatorID: out.OperatorID, CustomerID: out.CustomerID,
			BusinessKey: "usage:" + out.ID, Estimate: estimate, At: now, ExpiresAt: out.UsageDeadline}
		if out.CustomerHoldID == nil {
			hold, err := s.Ledger.ReserveCredits(ctx, t, reservation)
			if err != nil {
				return err
			}
			out.CustomerHoldID = &hold.ID
		} else if err = s.Ledger.IncreaseCreditReservation(ctx, t, *out.CustomerHoldID, reservation); err != nil {
			return err
		}
		out.EstimateATG = estimate
	}
	for i, target := range targets {
		hold := &holds[i]
		if target.Maximum.Cmp(hold.Maximum) <= 0 {
			continue
		}
		if hold.HoldID == "" {
			created, err := s.Cooperation.ReserveProcurement(ctx, t, hold.BuyerID, out.ID, target.Maximum, out.UsageDeadline)
			if err != nil {
				return err
			}
			hold.HoldID = created.ID
		} else if err = s.Cooperation.IncreaseProcurementReservation(ctx, t, hold.BuyerID, out.ID, hold.HoldID, target.Maximum, out.UsageDeadline); err != nil {
			return err
		}
		hold.Maximum = target.Maximum
	}
	out.SupplyHolds, err = json.Marshal(holds)
	if err != nil {
		return err
	}
	return t.Model(&Request{}).Where("id=?", out.ID).Updates(map[string]any{
		"estimate_atg": out.EstimateATG, "customer_hold_id": out.CustomerHoldID, "supply_holds": out.SupplyHolds,
	}).Error
}
