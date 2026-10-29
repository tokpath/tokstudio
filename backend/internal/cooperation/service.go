package cooperation

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

//go:embed migrate/*.sql
var migrations embed.FS

func Migrations() (string, fs.FS) { f, _ := fs.Sub(migrations, "migrate"); return "cooperation", f }

var ErrCoverage = errors.New("insufficient sales coverage")
var ErrConflict = errors.New("cooperation operation conflicts with existing facts")

type Service struct {
	DB           *gorm.DB
	Ledger       *ledger.Service
	Organization *organization.Service
	Rates        RateSource
}

func New(db *gorm.DB, l *ledger.Service, o *organization.Service) *Service {
	return &Service{DB: db, Ledger: l, Organization: o}
}
func (s *Service) within(ctx context.Context, tx *gorm.DB, fn func(*gorm.DB) error) error {
	if tx != nil {
		return fn(tx.WithContext(ctx))
	}
	return s.DB.WithContext(ctx).Transaction(fn)
}
func (s *Service) Lock(ctx context.Context, tx *gorm.DB, operator string) error {
	if tx == nil {
		return errors.New("cooperation lock requires transaction")
	}
	// A child procurement change also changes its OEM's outstanding liability.
	// All operations therefore lock the same non-platform supply chain in a
	// canonical order before reading or changing either coverage or inventory.
	ids := []string{}
	seen := map[string]bool{}
	for current := operator; current != ""; {
		if seen[current] {
			return organization.ErrInvalidRelationship
		}
		seen[current] = true
		op, e := s.Organization.GetOperator(ctx, tx, current)
		if e != nil {
			return e
		}
		if op.Kind == organization.Platform {
			if current == operator {
				ids = append(ids, current)
			}
			break
		}
		ids = append(ids, current)
		if op.ParentOperatorID == nil {
			return organization.ErrInvalidRelationship
		}
		current = *op.ParentOperatorID
	}
	sort.Strings(ids)
	for _, id := range ids {
		if e := tx.WithContext(ctx).Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", "cooperation:"+id).Error; e != nil {
			return e
		}
	}
	return nil
}
func hash(v any) string {
	b, _ := json.Marshal(v)
	x := sha256.Sum256(b)
	return hex.EncodeToString(x[:])
}

type ProcurementPosition struct {
	BuyerID    string         `gorm:"primaryKey" json:"buyer_id"`
	SupplierID string         `json:"supplier_id"`
	AccountID  string         `json:"account_id"`
	Quantity   ledger.Decimal `json:"quantity"`
	Cost       ledger.Decimal `json:"cost"`
	Version    int64          `json:"version"`
}

func (ProcurementPosition) TableName() string { return "cooperation_procurement_positions" }

type ProcurementLot struct {
	ID, BuyerID, OrderID, ReceiptReference string
	PaidMoney, PaidATG, GiftATG, Remaining ledger.Decimal
	PaidConsumed, GiftConsumed             ledger.Decimal
	PaidRemoved, GiftRemoved               ledger.Decimal
	Fingerprint                            string
	ExitTerms                              json.RawMessage `gorm:"type:jsonb"`
	DeliveredAt                            time.Time
}

func (ProcurementLot) TableName() string { return "cooperation_procurement_lots" }

type PurchaseInput struct {
	BuyerID, SupplierID, OrderID, ReceiptReference, ActorID string
	PaidMoney, PaidATG, GiftATG                             ledger.Decimal
	ExitTerms                                               json.RawMessage
	At                                                      time.Time
}

func (s *Service) Position(ctx context.Context, tx *gorm.DB, buyer string) (ProcurementPosition, error) {
	var p ProcurementPosition
	err := s.within(ctx, tx, func(t *gorm.DB) error { return t.First(&p, "buyer_id=?", buyer).Error })
	return p, err
}

// DeliverPurchase consumes an already verified full payment fact. The application
// validates its allocation to the immutable wholesale quote in the same transaction.
func (s *Service) DeliverPurchase(ctx context.Context, tx *gorm.DB, in PurchaseInput) (ProcurementLot, error) {
	if in.OrderID == "" || in.ReceiptReference == "" || in.ActorID == "" || in.At.IsZero() || !json.Valid(in.ExitTerms) || in.PaidMoney.Sign() < 0 || in.PaidATG.Sign() < 0 || in.GiftATG.Sign() < 0 || in.PaidATG.Add(in.GiftATG).Sign() <= 0 || (in.PaidMoney.Sign() > 0 && in.PaidATG.Sign() <= 0) {
		return ProcurementLot{}, ledger.ErrAmount
	}
	var result ProcurementLot
	err := s.within(ctx, tx, func(t *gorm.DB) error {
		agreement, e := s.Organization.GetSupplyAgreement(ctx, t, in.BuyerID)
		if e != nil {
			return e
		}
		if agreement.SupplierID != in.SupplierID || agreement.Status != "active" {
			return organization.ErrInvalidRelationship
		}
		if e = s.Lock(ctx, t, in.BuyerID); e != nil {
			return e
		}
		e = t.First(&result, "order_id=?", in.OrderID).Error
		if e == nil {
			if result.Fingerprint != hash(in) {
				return ErrConflict
			}
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		p, e := s.Position(ctx, t, in.BuyerID)
		if errors.Is(e, gorm.ErrRecordNotFound) {
			a, e := s.Ledger.EnsureAccount(ctx, t, ledger.Account{OperatorID: in.BuyerID, OwnerID: in.BuyerID, Purpose: "procurement:atg", Unit: "ATG"})
			if e != nil {
				return e
			}
			p = ProcurementPosition{BuyerID: in.BuyerID, SupplierID: in.SupplierID, AccountID: a.ID}
			if e = t.Create(&p).Error; e != nil {
				return e
			}
		} else if e != nil {
			return e
		}
		if p.SupplierID != in.SupplierID {
			return organization.ErrInvalidRelationship
		}
		control, e := s.Ledger.EnsureAccount(ctx, t, ledger.Account{OperatorID: in.BuyerID, OwnerID: in.BuyerID, Purpose: "control:procurement", Unit: "ATG", AllowNegative: true})
		if e != nil {
			return e
		}
		qty := in.PaidATG.Add(in.GiftATG)
		position, e := (ledger.Position{Quantity: p.Quantity, Cost: p.Cost}).Receive(qty, in.PaidMoney)
		if e != nil {
			return e
		}
		_, e = s.Ledger.Post(ctx, t, ledger.Posting{OperatorID: in.BuyerID, BusinessKey: "procurement:" + in.OrderID, Kind: "procurement", SourceID: in.OrderID, ActorID: in.ActorID, Reason: "verified prepaid wholesale delivery", Lines: []ledger.Line{{AccountID: p.AccountID, Amount: qty}, {AccountID: control.ID, Amount: qty.Neg()}}})
		if e != nil {
			return e
		}
		if e = t.Model(&ProcurementPosition{}).Where("buyer_id=?", in.BuyerID).Updates(map[string]any{"quantity": position.Quantity, "cost": position.Cost, "version": gorm.Expr("version+1")}).Error; e != nil {
			return e
		}
		result = ProcurementLot{ID: uuid.NewString(), BuyerID: in.BuyerID, OrderID: in.OrderID, ReceiptReference: in.ReceiptReference, PaidMoney: in.PaidMoney, PaidATG: in.PaidATG, GiftATG: in.GiftATG, Remaining: qty, Fingerprint: hash(in), ExitTerms: in.ExitTerms, DeliveredAt: in.At}
		return t.Create(&result).Error
	})
	return result, err
}

func (s *Service) ReserveProcurement(ctx context.Context, tx *gorm.DB, buyer, request string, maximum ledger.Decimal, expiry time.Time) (ledger.Hold, error) {
	var out ledger.Hold
	err := s.within(ctx, tx, func(t *gorm.DB) error {
		if e := s.Lock(ctx, t, buyer); e != nil {
			return e
		}
		p, e := s.Position(ctx, t, buyer)
		if e != nil {
			return e
		}
		out, e = s.Ledger.Reserve(ctx, t, ledger.Reservation{OperatorID: buyer, BusinessKey: "procurement:" + request, Purpose: "procurement", ExpiresAt: expiry, Amounts: []ledger.Line{{AccountID: p.AccountID, Amount: maximum}}})
		return e
	})
	return out, err
}

type ProcurementSettlement struct {
	RequestID         string          `json:"request_id"`
	BuyerID           string          `json:"buyer_id"`
	HoldID            string          `json:"hold_id"`
	Actual            ledger.Decimal  `json:"actual"`
	Deducted          ledger.Decimal  `json:"deducted"`
	PlatformShortfall ledger.Decimal  `json:"platform_shortfall"`
	CostMoney         ledger.Decimal  `json:"cost_money"`
	SourceAllocations json.RawMessage `json:"source_allocations" gorm:"type:jsonb"`
	CreatedAt         time.Time       `json:"created_at"`
}

func (ProcurementSettlement) TableName() string { return "cooperation_procurement_settlements" }

type SourceConsumption struct {
	LotID   string         `json:"lot_id"`
	PaidATG ledger.Decimal `json:"paid_atg"`
	GiftATG ledger.Decimal `json:"gift_atg"`
}

func (s *Service) SettleProcurement(ctx context.Context, tx *gorm.DB, buyer, request, holdID string, actual ledger.Decimal, at time.Time) (ProcurementSettlement, error) {
	if actual.Sign() < 0 {
		return ProcurementSettlement{}, ledger.ErrAmount
	}
	var result ProcurementSettlement
	err := s.within(ctx, tx, func(t *gorm.DB) error {
		if e := s.Lock(ctx, t, buyer); e != nil {
			return e
		}
		e := t.First(&result, "request_id=? AND buyer_id=?", request, buyer).Error
		if e == nil {
			if result.HoldID != holdID || result.Actual.Cmp(actual) != 0 {
				return ErrConflict
			}
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		p, e := s.Position(ctx, t, buyer)
		if e != nil {
			return e
		}
		hold, heldAllocations, e := s.Ledger.HoldDetails(ctx, t, buyer, holdID)
		if e != nil {
			return e
		}
		if hold.Status != "active" || hold.BusinessKey != "procurement:"+request || len(heldAllocations) != 1 || heldAllocations[0].AccountID != p.AccountID {
			return ErrConflict
		}
		allocation := heldAllocations[0]
		deduct := actual
		if deduct.Cmp(allocation.Amount) > 0 {
			deduct = allocation.Amount
		}
		result = ProcurementSettlement{RequestID: request, BuyerID: buyer, HoldID: holdID, Actual: actual, Deducted: deduct, PlatformShortfall: actual.Sub(deduct), SourceAllocations: json.RawMessage("[]"), CreatedAt: at}
		if deduct.IsZero() {
			_, e = s.Ledger.Release(ctx, t, buyer, holdID, "usage:"+request)
			if e != nil {
				return e
			}
			return t.Create(&result).Error
		}
		control, e := s.Ledger.EnsureAccount(ctx, t, ledger.Account{OperatorID: buyer, OwnerID: buyer, Purpose: "control:procurement", Unit: "ATG", AllowNegative: true})
		if e != nil {
			return e
		}
		_, e = s.Ledger.Capture(ctx, t, holdID, ledger.Posting{OperatorID: buyer, BusinessKey: "procurement:usage:" + request, Kind: "usage", SourceID: request, ActorID: "system:metering", Reason: "confirmed wholesale usage", Lines: []ledger.Line{{AccountID: p.AccountID, Amount: deduct.Neg()}, {AccountID: control.ID, Amount: deduct}}}, nil)
		if e != nil {
			return e
		}
		remaining, cost, _, e := (ledger.Position{Quantity: p.Quantity, Cost: p.Cost}).Remove(deduct)
		if e != nil {
			return e
		}
		result.CostMoney = cost
		if e = t.Model(&ProcurementPosition{}).Where("buyer_id=?", buyer).Updates(map[string]any{"quantity": remaining.Quantity, "cost": remaining.Cost, "version": gorm.Expr("version+1")}).Error; e != nil {
			return e
		}
		var lots []ProcurementLot
		if e = t.Clauses(clause.Locking{Strength: "UPDATE"}).Where("buyer_id=? AND remaining>0", buyer).Order("delivered_at,id").Find(&lots).Error; e != nil {
			return e
		}
		left := deduct
		allocations := []SourceConsumption{}
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
			allocations = append(allocations, SourceConsumption{lot.ID, parts[0], parts[1]})
			if e = t.Model(&ProcurementLot{}).Where("id=?", lot.ID).Updates(map[string]any{"remaining": lot.Remaining.Sub(take), "paid_consumed": lot.PaidConsumed.Add(parts[0]), "gift_consumed": lot.GiftConsumed.Add(parts[1])}).Error; e != nil {
				return e
			}
			left = left.Sub(take)
		}
		if !left.IsZero() {
			return ErrConflict
		}
		result.SourceAllocations, _ = json.Marshal(allocations)
		return t.Create(&result).Error
	})
	return result, err
}
