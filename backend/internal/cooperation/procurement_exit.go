package cooperation

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
)

var ErrExitTerms = errors.New("procurement exit is not permitted by the accepted terms")
var ErrLiability = errors.New("outstanding responsibilities must be settled or explicitly transferred")

type ProcurementExitTerms struct {
	Version         int    `json:"version"`
	VoluntaryRefund bool   `json:"voluntary_refund"`
	Method          string `json:"method"`
	TermsReference  string `json:"terms_reference"`
	Notes           string `json:"notes,omitempty"`
}

func ParseProcurementExitTerms(raw json.RawMessage) (ProcurementExitTerms, error) {
	var t ProcurementExitTerms
	if json.Unmarshal(raw, &t) != nil || t.Version != 1 || t.TermsReference == "" || t.Method != "non_refundable" || t.VoluntaryRefund {
		return t, ErrExitTerms
	}
	return t, nil
}

// RemovalGuard must evaluate current responsibilities or a real, approved
// transfer/settlement. Text evidence alone cannot authorize withdrawing cover.
type RemovalGuard interface {
	RequireProcurementRemoval(context.Context, *gorm.DB, string, ledger.Decimal, string, string) error
}
type RemovalInput struct {
	BuyerID, BusinessKey, ActorID, Mode, Reason, Evidence string
	Quantity                                              ledger.Decimal
	At                                                    time.Time
	Guard                                                 RemovalGuard
}
type RemovedSource struct {
	LotID             string          `json:"lot_id"`
	OrderID           string          `json:"order_id"`
	PaidATG           ledger.Decimal  `json:"paid_atg"`
	GiftATG           ledger.Decimal  `json:"gift_atg"`
	OriginalPaidMoney ledger.Decimal  `json:"original_paid_money"`
	OriginalPaidATG   ledger.Decimal  `json:"original_paid_atg"`
	ExitTerms         json.RawMessage `json:"exit_terms"`
}
type ProcurementRemoval struct {
	ID, BuyerID, BusinessKey, Fingerprint, Mode      string
	Quantity, BookCostMoney                          ledger.Decimal
	CostRemainderNumerator, CostRemainderDenominator string
	Sources                                          json.RawMessage `gorm:"type:jsonb"`
	JournalID, ActorID, Reason, Evidence             string
	CreatedAt                                        time.Time
}

func (ProcurementRemoval) TableName() string { return "cooperation_procurement_removals" }

// RemoveProcurement is a trusted transactional primitive for exits and takeover.
// It moves only unheld inventory and current average book cost. Normal historical
// consumption, customer entitlements and previous cost settlements are unchanged.
func (s *Service) RemoveProcurement(ctx context.Context, tx *gorm.DB, in RemovalInput) (ProcurementRemoval, error) {
	out := ProcurementRemoval{}
	if tx == nil || in.Guard == nil || in.Quantity.Sign() <= 0 || !in.Quantity.Valid() || in.BusinessKey == "" || in.ActorID == "" || in.Reason == "" || in.Evidence == "" || in.At.IsZero() || in.Mode != "takeover" {
		return out, ErrConflict
	}
	fp := hash([]string{in.BuyerID, in.BusinessKey, in.ActorID, in.Mode, in.Reason, in.Evidence, in.Quantity.String()})
	e := s.within(ctx, tx, func(t *gorm.DB) error {
		if e := s.Lock(ctx, t, in.BuyerID); e != nil {
			return e
		}
		q := t.Where("buyer_id=? AND business_key=?", in.BuyerID, in.BusinessKey).Limit(1).Find(&out)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected > 0 {
			if out.Fingerprint != fp {
				return ErrConflict
			}
			return nil
		}
		if e := in.Guard.RequireProcurementRemoval(ctx, t, in.BuyerID, in.Quantity, in.Mode, in.Evidence); e != nil {
			return e
		}
		p, e := s.Position(ctx, t, in.BuyerID)
		if e != nil {
			return e
		}
		a, e := s.Ledger.AccountInTransaction(ctx, t, p.AccountID)
		if e != nil {
			return e
		}
		if a.Available().Cmp(in.Quantity) < 0 {
			return ErrCoverage
		}
		remaining, cost, remainder, e := (ledger.Position{Quantity: p.Quantity, Cost: p.Cost}).Remove(in.Quantity)
		if e != nil {
			return e
		}
		var lots []ProcurementLot
		if e = t.Clauses(clause.Locking{Strength: "UPDATE"}).Where("buyer_id=? AND remaining>0", in.BuyerID).Order("delivered_at,id").Find(&lots).Error; e != nil {
			return e
		}
		left := in.Quantity
		sources := []RemovedSource{}
		for _, lot := range lots {
			if left.IsZero() {
				break
			}
			take := lot.Remaining
			if take.Cmp(left) > 0 {
				take = left
			}
			parts, e := ledger.Proportional(take, []ledger.Decimal{lot.PaidATG.Sub(lot.PaidConsumed).Sub(lot.PaidRemoved), lot.GiftATG.Sub(lot.GiftConsumed).Sub(lot.GiftRemoved)})
			if e != nil {
				return e
			}
			sources = append(sources, RemovedSource{lot.ID, lot.OrderID, parts[0], parts[1], lot.PaidMoney, lot.PaidATG, lot.ExitTerms})
			if e = t.Model(&ProcurementLot{}).Where("id=?", lot.ID).Updates(map[string]any{"remaining": lot.Remaining.Sub(take), "paid_removed": lot.PaidRemoved.Add(parts[0]), "gift_removed": lot.GiftRemoved.Add(parts[1])}).Error; e != nil {
				return e
			}
			left = left.Sub(take)
		}
		if !left.IsZero() {
			return ErrConflict
		}
		control, e := s.Ledger.EnsureAccount(ctx, t, ledger.Account{OperatorID: in.BuyerID, OwnerID: in.BuyerID, Purpose: "control:procurement_removal", Unit: "ATG", AllowNegative: true})
		if e != nil {
			return e
		}
		journal, e := s.Ledger.Post(ctx, t, ledger.Posting{OperatorID: in.BuyerID, BusinessKey: "procurement-removal:" + in.BusinessKey, Kind: "procurement_removal", SourceID: in.BusinessKey, ActorID: in.ActorID, Reason: in.Reason, Lines: []ledger.Line{{AccountID: p.AccountID, Amount: in.Quantity.Neg()}, {AccountID: control.ID, Amount: in.Quantity}}})
		if e != nil {
			return e
		}
		if e = t.Model(&ProcurementPosition{}).Where("buyer_id=?", in.BuyerID).Updates(map[string]any{"quantity": remaining.Quantity, "cost": remaining.Cost, "version": gorm.Expr("version+1")}).Error; e != nil {
			return e
		}
		raw, _ := json.Marshal(sources)
		out = ProcurementRemoval{ID: uuid.NewString(), BuyerID: in.BuyerID, BusinessKey: in.BusinessKey, Fingerprint: fp, Mode: in.Mode, Quantity: in.Quantity, BookCostMoney: cost, CostRemainderNumerator: remainder.Num().String(), CostRemainderDenominator: remainder.Denom().String(), Sources: raw, JournalID: journal.ID, ActorID: in.ActorID, Reason: in.Reason, Evidence: in.Evidence, CreatedAt: in.At.UTC()}
		return t.Create(&out).Error
	})
	return out, e
}

// CoverageRemovalGuard permits only a withdrawal that preserves current real
// liability cover, excluding the discretionary oversell allowance.
type CoverageRemovalGuard struct {
	Cooperation  *Service
	PaymentFacts PaymentObligationReader
}

func (g CoverageRemovalGuard) RequireProcurementRemoval(ctx context.Context, tx *gorm.DB, buyer string, q ledger.Decimal, mode, _ string) error {
	if g.Cooperation == nil {
		return ErrLiability
	}
	if mode == "supply_failure" {
		return ErrExitTerms
	}
	if e := g.Cooperation.RequireNoPendingProcurementReceipts(ctx, tx, buyer, g.PaymentFacts); e != nil {
		return e
	}
	now, e := businessTime(ctx, tx)
	if e != nil {
		return e
	}
	c, e := g.Cooperation.Coverage(ctx, tx, buyer, now)
	if e != nil {
		return e
	}
	if c.ProcurementAvailable.Sub(q).Cmp(c.Required.Add(c.Pending)) < 0 {
		return ErrLiability
	}
	return nil
}
