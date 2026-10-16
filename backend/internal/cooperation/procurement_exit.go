package cooperation

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"math/big"
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
	if json.Unmarshal(raw, &t) != nil || t.Version != 1 || t.TermsReference == "" || (t.Method != "remaining_paid_proportional" && t.Method != "non_refundable") || (t.VoluntaryRefund && t.Method != "remaining_paid_proportional") {
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
	LotID           string          `json:"lot_id"`
	OrderID         string          `json:"order_id"`
	PaidATG         ledger.Decimal  `json:"paid_atg"`
	GiftATG         ledger.Decimal  `json:"gift_atg"`
	OriginalPaidUSD ledger.Decimal  `json:"original_paid_usd"`
	OriginalPaidATG ledger.Decimal  `json:"original_paid_atg"`
	ExitTerms       json.RawMessage `json:"exit_terms"`
}
type ProcurementRemoval struct {
	ID, BuyerID, BusinessKey, Fingerprint, Mode      string
	Quantity, BookCostUSD                            ledger.Decimal
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
	if tx == nil || in.Guard == nil || in.Quantity.Sign() <= 0 || !in.Quantity.Valid() || in.BusinessKey == "" || in.ActorID == "" || in.Reason == "" || in.Evidence == "" || in.At.IsZero() || (in.Mode != "voluntary_exit" && in.Mode != "supply_failure" && in.Mode != "takeover") {
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
			sources = append(sources, RemovedSource{lot.ID, lot.OrderID, parts[0], parts[1], lot.PaidUSD, lot.PaidATG, lot.ExitTerms})
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
		out = ProcurementRemoval{ID: uuid.NewString(), BuyerID: in.BuyerID, BusinessKey: in.BusinessKey, Fingerprint: fp, Mode: in.Mode, Quantity: in.Quantity, BookCostUSD: cost, CostRemainderNumerator: remainder.Num().String(), CostRemainderDenominator: remainder.Denom().String(), Sources: raw, JournalID: journal.ID, ActorID: in.ActorID, Reason: in.Reason, Evidence: in.Evidence, CreatedAt: in.At.UTC()}
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

type ProcurementExit struct {
	ID                           string         `json:"id"`
	BuyerID                      string         `json:"buyer_id"`
	SupplierID                   string         `json:"supplier_id"`
	BusinessKey                  string         `json:"business_key"`
	Fingerprint                  string         `json:"-"`
	Mode                         string         `json:"mode"`
	RemovalID                    string         `json:"removal_id"`
	ContractUSD                  ledger.Decimal `json:"contract_usd"`
	BookCostUSD                  ledger.Decimal `json:"book_cost_usd"`
	CostDifferenceUSD            ledger.Decimal `json:"cost_difference_usd"`
	RefundedUSD                  ledger.Decimal `json:"refunded_usd"`
	ContractRemainderNumerator   string         `json:"contract_remainder_numerator"`
	ContractRemainderDenominator string         `json:"contract_remainder_denominator"`
	State                        string         `json:"state"`
	Evidence                     string         `json:"evidence"`
	CreatedBy                    string         `json:"created_by"`
	CreatedAt                    time.Time      `json:"created_at"`
}

func (ProcurementExit) TableName() string { return "cooperation_procurement_exits" }
func (s *Service) RequestProcurementExit(ctx context.Context, tx *gorm.DB, actor, buyer, key, mode, evidence string, quantity ledger.Decimal, guard RemovalGuard) (ProcurementExit, error) {
	out := ProcurementExit{}
	if mode != "voluntary_exit" && mode != "supply_failure" {
		return out, ErrExitTerms
	}
	if key == "" || evidence == "" {
		return out, ErrConflict
	}
	e := s.within(ctx, tx, func(t *gorm.DB) error {
		if e := s.Organization.RequirePermission(ctx, t, actor, buyer, "cooperation.manage"); e != nil {
			return e
		}
		if e := s.Lock(ctx, t, buyer); e != nil {
			return e
		}
		fp := hash([]string{buyer, key, mode, evidence, quantity.String()})
		q := t.Where("buyer_id=? AND business_key=?", buyer, key).Limit(1).Find(&out)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected > 0 {
			if out.Fingerprint != fp {
				return ErrConflict
			}
			return nil
		}
		agreement, e := s.Organization.GetSupplyAgreement(ctx, t, buyer)
		if e != nil {
			return e
		}
		now, e := businessTime(ctx, t)
		if e != nil {
			return e
		}

		removal, e := s.RemoveProcurement(ctx, t, RemovalInput{BuyerID: buyer, BusinessKey: "exit:" + key, ActorID: actor, Mode: mode, Reason: "procurement exit settlement", Evidence: evidence, Quantity: quantity, At: now, Guard: guard})
		if e != nil {
			return e
		}
		var sources []RemovedSource
		if json.Unmarshal(removal.Sources, &sources) != nil {
			return ErrConflict
		}
		exact := new(big.Rat)
		for _, src := range sources {
			if mode == "voluntary_exit" {
				terms, e := ParseProcurementExitTerms(src.ExitTerms)
				if e != nil || !terms.VoluntaryRefund {
					return ErrExitTerms
				}
			}
			if src.PaidATG.Sign() > 0 && src.OriginalPaidATG.Sign() > 0 {
				exact.Add(exact, new(big.Rat).Mul(src.OriginalPaidUSD.Rat(), new(big.Rat).Quo(src.PaidATG.Rat(), src.OriginalPaidATG.Rat())))
			}
		}
		cash, tail := ledger.Quantize(exact)
		if !cash.Valid() {
			return ledger.ErrAmount
		}
		out = ProcurementExit{ID: uuid.NewString(), BuyerID: buyer, SupplierID: agreement.SupplierID, BusinessKey: key, Fingerprint: fp, Mode: mode, RemovalID: removal.ID, ContractUSD: cash, BookCostUSD: removal.BookCostUSD, CostDifferenceUSD: cash.Sub(removal.BookCostUSD), ContractRemainderNumerator: tail.Num().String(), ContractRemainderDenominator: tail.Denom().String(), State: "awaiting_refund", Evidence: evidence, CreatedBy: actor, CreatedAt: now}
		if cash.IsZero() {
			out.State = "settled_no_cash"
		}
		if e := t.Create(&out).Error; e != nil {
			return e
		}
		return s.bookExitObligation(ctx, t, out)
	})
	return out, e
}

type SupplyFailureApproval struct {
	ID         string    `json:"id"`
	BuyerID    string    `json:"buyer_id"`
	SupplierID string    `json:"supplier_id"`
	Evidence   string    `json:"evidence"`
	ApprovedBy string    `json:"approved_by"`
	ApprovedAt time.Time `json:"approved_at"`
}

func (SupplyFailureApproval) TableName() string { return "cooperation_supply_failure_approvals" }
func (s *Service) ApproveSupplyFailure(ctx context.Context, tx *gorm.DB, actor, buyer, evidence string) (SupplyFailureApproval, error) {
	out := SupplyFailureApproval{}
	if evidence == "" {
		return out, ErrConflict
	}
	e := s.within(ctx, tx, func(t *gorm.DB) error {
		a, e := s.Organization.GetSupplyAgreement(ctx, t, buyer)
		if e != nil {
			return e
		}
		if e = s.Organization.RequirePermission(ctx, t, actor, a.SupplierID, "refunds.manage"); e != nil {
			return e
		}
		now, e := businessTime(ctx, t)
		if e != nil {
			return e
		}
		out = SupplyFailureApproval{ID: uuid.NewString(), BuyerID: buyer, SupplierID: a.SupplierID, Evidence: evidence, ApprovedBy: actor, ApprovedAt: now}
		return t.Create(&out).Error
	})
	return out, e
}

type ApprovedSupplyFailureGuard struct {
	Cooperation  *Service
	ApprovalID   string
	PaymentFacts PaymentObligationReader
}

func (g ApprovedSupplyFailureGuard) RequireProcurementRemoval(ctx context.Context, tx *gorm.DB, buyer string, q ledger.Decimal, mode, evidence string) error {
	if mode != "supply_failure" || g.Cooperation == nil {
		return ErrExitTerms
	}
	var a SupplyFailureApproval
	if e := tx.First(&a, "id=? AND buyer_id=?", g.ApprovalID, buyer).Error; e != nil {
		return e
	}
	agreement, e := g.Cooperation.Organization.GetSupplyAgreement(ctx, tx, buyer)
	if e != nil {
		return e
	}
	if a.SupplierID != agreement.SupplierID {
		return ErrConflict
	}
	return (CoverageRemovalGuard{Cooperation: g.Cooperation, PaymentFacts: g.PaymentFacts}).RequireProcurementRemoval(ctx, tx, buyer, q, "voluntary_exit", evidence)
}
