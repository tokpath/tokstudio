package usage

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"github.com/tokpath/tokstudio/backend/internal/cooperation"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/modelruntime"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func ceiling(v *big.Rat) (ledger.Decimal, error) {
	d, rem := ledger.Quantize(v)
	if rem.Sign() > 0 {
		d = d.Add(ledger.MustDecimal("0.000000000000000001"))
	}
	if !d.Valid() || d.Sign() < 0 {
		return ledger.Zero(), ledger.ErrAmount
	}
	return d, nil
}

// Estimate is the customer reservation for this request. Visible quantities
// come from the original body; unknown quantities use the registered bound.
func Estimate(q catalog.ModelQuote, r modelruntime.Request) (ledger.Decimal, error) {
	exact, e := q.Retail.Charge(reservationQuantities(q, r))
	if e != nil {
		return ledger.Zero(), e
	}
	return ceiling(exact)
}

// Admit atomically checks the current key and ownership, captures all prices,
// and reserves the customer estimate and every upstream procurement maximum.
// No network model request happens until this transaction commits.
func (s *Service) Admit(ctx context.Context, authenticated Key, businessKey string, r modelruntime.Request) (Request, bool, error) {
	ctx, span := otel.Tracer("tokenhub/usage").Start(ctx, "usage.admit")
	defer span.End()
	var committedWork trace.Span
	defer func() {
		if committedWork != nil {
			committedWork.End()
		}
	}()
	var out Request
	created := false
	if businessKey == "" || len(businessKey) > 160 || s.Ledger == nil || s.Organization == nil || s.Cooperation == nil || s.GrantObserver == nil || s.EncryptionKey == "" {
		return out, false, ErrNotReady
	}
	originalID := r.ID
	r.ID = ""
	fp := s.fingerprint(r)
	r.ID = originalID
	e := s.within(ctx, nil, func(t *gorm.DB) error {
		ownership, e := s.Organization.CurrentOwnership(ctx, t, authenticated.CustomerID)
		if e != nil {
			return e
		}
		if _, e = s.Organization.LockOwnership(ctx, t, authenticated.CustomerID, ownership.OperatorID, ownership.Version); e != nil {
			return e
		}
		cat := s.Catalog.WithDB(t)
		cat.Operators = cooperation.CatalogDirectory{Organization: s.Organization, Tx: t}
		if _, e = cat.LockRevision(ctx); e != nil {
			return e
		}
		if e = s.Cooperation.Lock(ctx, t, ownership.OperatorID); e != nil {
			return e
		}
		if e = s.GrantObserver.LockBusiness(ctx, t, ownership.OperatorID); e != nil {
			return e
		}
		var k Key
		if e = t.Clauses(clause.Locking{Strength: "UPDATE"}).First(&k, "id=? AND customer_id=?", authenticated.ID, authenticated.CustomerID).Error; e != nil {
			return ErrKey
		}
		// Separate shared-resource queueing from the work performed after the
		// business fences are acquired. This child span excludes commit time.
		_, work := otel.Tracer("tokenhub/usage").Start(ctx, "usage.admit.after_locks")
		defer work.End()
		// End outside the transaction callback so this measurement includes
		// its commit or rollback, without the initial business-lock queue.
		_, committedWork = otel.Tracer("tokenhub/usage").Start(ctx, "usage.admit.committed_work")
		if k.Status != "active" || k.TokenHash != authenticated.TokenHash || k.BrandID != authenticated.BrandID {
			return ErrKey
		}
		e = t.First(&out, "key_id=? AND business_key=?", k.ID, businessKey).Error
		if e == nil {
			if out.Fingerprint != fp {
				return ErrConflict
			}
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		c, e := s.Organization.GetCustomer(ctx, t, k.CustomerID)
		if e != nil {
			return e
		}
		if e = s.Organization.RequireCustomerAction(ctx, t, k.CustomerID, ownership.OperatorID, "model_usage"); e != nil {
			return e
		}
		if c.Status != "active" || c.BrandID != k.BrandID {
			return ErrKey
		}
		status, e := s.Organization.GetBusinessStatus(ctx, t, ownership.OperatorID)
		if e != nil {
			return e
		}
		if !status.ModelUsage {
			return organization.ErrBusinessClosed
		}
		var modelIDs []string
		if len(k.ModelIDs) > 0 && json.Unmarshal(k.ModelIDs, &modelIDs) != nil {
			return ErrKey
		}
		if modelIDs != nil {
			allowed := false
			for _, id := range modelIDs {
				if id == r.PublicModel {
					allowed = true
				}
			}
			if !allowed {
				return ErrKey
			}
		}
		now, e := s.businessTime(t)
		if e != nil {
			return e
		}
		var count int64
		var durableRateCount int64
		if k.RateLimitRPM != nil {
			if e = t.Model(&Request{}).Where("key_id=? AND accepted_at>?", k.ID, now.Add(-time.Minute)).Count(&count).Error; e != nil {
				return e
			}
			if count >= *k.RateLimitRPM {
				return ErrRate
			}
			durableRateCount = count
		}
		if k.ConcurrencyLimit != nil {
			if e = t.Model(&Request{}).Where("key_id=? AND status IN ('accepted','executing','submitted','awaiting_usage')", k.ID).Count(&count).Error; e != nil {
				return e
			}
			if count >= *k.ConcurrencyLimit {
				return ErrConcurrency
			}
		}
		r.ID = uuid.NewString()
		quote, e := cat.Quote(ctx, k.BrandID, ownership.OperatorID, r, now)
		if e != nil {
			return e
		}
		estimate, e := Estimate(quote, r)
		if e != nil {
			return e
		}
		window, e := ledger.BudgetWindow(now, k.CreatedAt, k.Period, k.Timezone)
		if e != nil {
			return e
		}
		if k.BudgetLimit != nil {
			budget, e := s.KeyBudget(ctx, t, k, now)
			if e != nil {
				return e
			}
			if budget.SpentATG.Add(budget.HeldATG).Add(estimate).Cmp(*k.BudgetLimit) > 0 {
				return ErrBudget
			}
		}
		out = Request{ID: r.ID, CustomerID: k.CustomerID, OperatorID: ownership.OperatorID, BrandID: k.BrandID, OwnershipVersion: ownership.Version, KeyID: k.ID, BusinessKey: businessKey, Fingerprint: fp, ModelID: r.PublicModel, Operation: r.Operation, Streaming: r.Stream, Status: "accepted", AcceptedAt: now, UsageDeadline: now.Add(s.UsageTimeout), WindowStart: window.Start, WindowEnd: window.End, EstimateATG: estimate}
		out.BudgetLimit = k.BudgetLimit
		if estimate.Sign() > 0 {
			hold, e := s.Ledger.ReserveCredits(ctx, t, ledger.CreditReservation{OperatorID: out.OperatorID, CustomerID: out.CustomerID, BusinessKey: "usage:" + out.ID, Estimate: estimate, At: now, ExpiresAt: out.UsageDeadline})
			if e != nil {
				return e
			}
			out.CustomerHoldID = &hold.ID
		}
		holds := []SupplyHold{}
		reserved := reservationQuantities(quote, r)
		for _, price := range quote.Supply {
			exact, e := price.Charge(reserved)
			if e != nil {
				return e
			}
			max, e := ceiling(exact)
			if e != nil {
				return e
			}
			row := SupplyHold{BuyerID: price.Scope.BuyerID, Maximum: max}
			if max.Sign() > 0 {
				h, e := s.Cooperation.ReserveProcurement(ctx, t, row.BuyerID, out.ID, max, out.UsageDeadline)
				if e != nil {
					return e
				}
				row.HoldID = h.ID
			}
			holds = append(holds, row)
		}
		out.SupplyHolds, _ = json.Marshal(holds)
		out.PriceSnapshot, _ = json.Marshal(quote)
		out.Candidates, _ = json.Marshal(quote.Candidates)
		// The durable transaction rolls all reservations back when Redis is
		// unavailable. Already accepted requests can still settle without Redis.
		if s.AdmissionLimiter != nil {
			if e = s.AdmissionLimiter.Check(ctx, k.ID, out.ID, k.RateLimitRPM, durableRateCount); e != nil {
				return e
			}
		}
		if e = t.Create(&out).Error; e != nil {
			return e
		}
		created = true
		return nil
	})
	return out, created, e
}
