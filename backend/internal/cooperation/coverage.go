package cooperation

import (
	"context"
	"errors"
	"math/big"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"gorm.io/gorm"
)

type CoverageRates struct {
	Version             int64
	Direct              *big.Rat
	Children            map[string]*big.Rat
	DirectLiability     *big.Rat
	DirectProvisional   bool
	ChildrenProvisional map[string]bool
}

// CoverageRates takes a catalog revision lock in tx. Rates express the maximum
// upstream ATG required for one ATG of each outstanding downstream liability.
type RateSource interface {
	CoverageRates(context.Context, *gorm.DB, string, time.Time) (CoverageRates, error)
}
type OversellPolicy struct {
	OperatorID    string         `gorm:"primaryKey" json:"operator_id"`
	Allowance     ledger.Decimal `json:"allowance"`
	WarningBuffer ledger.Decimal `json:"warning_buffer"`
	Version       int64          `json:"version"`
	UpdatedBy     string         `json:"updated_by"`
	UpdatedAt     time.Time      `json:"updated_at"`
}

func (OversellPolicy) TableName() string { return "cooperation_oversell_policies" }

type CoverageSnapshot struct {
	OperatorID           string         `json:"operator_id"`
	HasUpstream          bool           `json:"has_upstream"`
	ProcurementAvailable ledger.Decimal `json:"procurement_available"`
	Required             ledger.Decimal `json:"required"`
	Pending              ledger.Decimal `json:"pending"`
	Oversell             ledger.Decimal `json:"oversell"`
	Space                ledger.Decimal `json:"space"`
	WarningBuffer        ledger.Decimal `json:"warning_buffer"`
	CatalogVersion       int64          `json:"catalog_version"`
	Status               string         `json:"status"`
	Provisional          bool           `json:"provisional"`
}
type SalesCommitment struct {
	ID, OperatorID, OrderID string
	BuyerID                 *string
	ATG                     ledger.Decimal
	Status                  string
	CatalogVersion          int64
	CreatedAt, UpdatedAt    time.Time
}

func (SalesCommitment) TableName() string { return "cooperation_sales_commitments" }

func (s *Service) SetOversell(ctx context.Context, tx *gorm.DB, actor, operator string, allowance, warning ledger.Decimal, expected int64) (OversellPolicy, error) {
	var out OversellPolicy
	if allowance.Sign() < 0 || warning.Sign() < 0 {
		return out, ledger.ErrAmount
	}
	err := s.within(ctx, tx, func(t *gorm.DB) error {
		op, e := s.Organization.GetOperator(ctx, t, operator)
		if e != nil {
			return e
		}
		manager := op.ID
		if string(op.Kind) == "channel" {
			if op.ParentOperatorID == nil {
				return organization.ErrInvalidRelationship
			}
			manager = *op.ParentOperatorID
		}
		if string(op.Kind) == "platform" {
			return organization.ErrInvalidRelationship
		}
		if e = s.Organization.RequirePermission(ctx, t, actor, manager, "cooperation.manage"); e != nil {
			return e
		}
		if e = s.Lock(ctx, t, operator); e != nil {
			return e
		}
		e = t.First(&out, "operator_id=?", operator).Error
		if errors.Is(e, gorm.ErrRecordNotFound) {
			if expected != 0 {
				return ErrConflict
			}
			out = OversellPolicy{OperatorID: operator, Allowance: allowance, WarningBuffer: warning, Version: 1, UpdatedBy: actor, UpdatedAt: time.Now().UTC()}
			return t.Create(&out).Error
		}
		if e != nil {
			return e
		}
		if out.Version != expected {
			return ErrConflict
		}
		out.Allowance = allowance
		out.WarningBuffer = warning
		out.Version++
		out.UpdatedBy = actor
		out.UpdatedAt = time.Now().UTC()
		return t.Save(&out).Error
	})
	return out, err
}

func conservative(q ledger.Decimal, r *big.Rat) (ledger.Decimal, error) {
	if q.IsZero() {
		return ledger.Zero(), nil
	}
	if r == nil || r.Sign() < 0 {
		return ledger.Zero(), ErrCoverage
	}
	out, tail := ledger.Quantize(new(big.Rat).Mul(q.Rat(), r))
	if tail.Sign() > 0 {
		out = out.Add(ledger.MustDecimal("0.000000000000000001"))
	}
	if !out.Valid() {
		return out, ledger.ErrAmount
	}
	return out, nil
}
func (s *Service) Coverage(ctx context.Context, tx *gorm.DB, operator string, at time.Time) (CoverageSnapshot, error) {
	var out CoverageSnapshot
	err := s.within(ctx, tx, func(t *gorm.DB) error { var e error; out, _, e = s.coverage(ctx, t, operator, at); return e })
	return out, err
}
func (s *Service) coverage(ctx context.Context, t *gorm.DB, operator string, at time.Time) (CoverageSnapshot, CoverageRates, error) {
	out := CoverageSnapshot{OperatorID: operator, Status: "ready", CatalogVersion: 1}
	rates := CoverageRates{}
	op, e := s.Organization.GetOperator(ctx, t, operator)
	if e != nil {
		return out, rates, e
	}
	if string(op.Kind) == "platform" {
		return out, rates, nil
	}
	out.HasUpstream = true
	if s.Rates == nil {
		return out, rates, ErrCoverage
	}
	rates, e = s.Rates.CoverageRates(ctx, t, operator, at)
	if e != nil {
		return out, rates, e
	}
	if e = s.Lock(ctx, t, operator); e != nil {
		return out, rates, e
	}
	out.CatalogVersion = rates.Version
	p, e := s.Position(ctx, t, operator)
	if e == nil {
		a, e := s.Ledger.AccountInTransaction(ctx, t, p.AccountID)
		if e != nil {
			return out, rates, e
		}
		out.ProcurementAvailable = a.Available()
	} else if !errors.Is(e, gorm.ErrRecordNotFound) {
		return out, rates, e
	}
	liability, e := s.Ledger.UnheldLiability(ctx, t, operator, at)
	if e != nil {
		return out, rates, e
	}
	directLiability := rates.DirectLiability
	if directLiability == nil {
		directLiability = rates.Direct
	}
	out.Required, e = conservative(liability, directLiability)
	out.Provisional = liability.Sign() > 0 && rates.DirectProvisional
	if e != nil {
		return out, rates, e
	}
	var children []ProcurementPosition
	if e = t.Where("supplier_id=?", operator).Order("buyer_id").Find(&children).Error; e != nil {
		return out, rates, e
	}
	for _, child := range children {
		a, e := s.Ledger.AccountInTransaction(ctx, t, child.AccountID)
		if e != nil {
			return out, rates, e
		}
		demand, e := conservative(a.Available(), rates.Children[child.BuyerID])
		if e != nil {
			return out, rates, e
		}
		out.Required = out.Required.Add(demand)
		out.Provisional = out.Provisional || a.Available().Sign() > 0 && rates.ChildrenProvisional[child.BuyerID]
	}
	var pending []SalesCommitment
	if e = t.Where("operator_id=? AND status='active'", operator).Find(&pending).Error; e != nil {
		return out, rates, e
	}
	for _, row := range pending {
		factor := directLiability
		if row.BuyerID != nil {
			factor = rates.Children[*row.BuyerID]
			out.Provisional = out.Provisional || rates.ChildrenProvisional[*row.BuyerID]
		} else {
			out.Provisional = out.Provisional || rates.DirectProvisional
		}
		demand, e := conservative(row.ATG, factor)
		if e != nil {
			return out, rates, e
		}
		out.Pending = out.Pending.Add(demand)
	}
	var policy OversellPolicy
	e = t.First(&policy, "operator_id=?", operator).Error
	if e != nil && !errors.Is(e, gorm.ErrRecordNotFound) {
		return out, rates, e
	}
	out.Oversell = policy.Allowance
	out.WarningBuffer = policy.WarningBuffer
	out.Space = out.ProcurementAvailable.Add(out.Oversell).Sub(out.Required).Sub(out.Pending)
	switch {
	case out.ProcurementAvailable.IsZero():
		out.Status = "procurement_empty"
	case out.Space.Sign() < 0:
		out.Status = "uncovered"
	case out.Space.IsZero():
		out.Status = "sold_out"
	case out.Required.Add(out.Pending).Cmp(out.ProcurementAvailable) > 0:
		out.Status = "overselling"
	case out.ProcurementAvailable.Sub(out.Required).Sub(out.Pending).Cmp(out.WarningBuffer) <= 0:
		out.Status = "replenish_early"
	}
	return out, rates, nil
}
func (s *Service) Reserve(ctx context.Context, tx *gorm.DB, o commerce.Order) (commerce.Commitment, error) {
	return s.reserveSale(ctx, tx, o, false)
}
func (s *Service) Restore(ctx context.Context, tx *gorm.DB, o commerce.Order) (commerce.Commitment, error) {
	return s.reserveSale(ctx, tx, o, true)
}
func (s *Service) reserveSale(ctx context.Context, tx *gorm.DB, o commerce.Order, restore bool) (commerce.Commitment, error) {
	var out commerce.Commitment
	err := s.within(ctx, tx, func(t *gorm.DB) error {
		if e := s.Lock(ctx, t, o.OperatorID); e != nil {
			return e
		}
		now, e := businessTime(ctx, t)
		if e != nil {
			return e
		}
		snapshot, rates, e := s.coverage(ctx, t, o.OperatorID, now)
		if e != nil {
			return e
		}
		if e = s.Lock(ctx, t, o.OperatorID); e != nil {
			return e
		}
		qty := o.PaidATG.Add(o.PackageGiftATG).Add(o.MerchantGiftATG).Add(o.KOLGiftATG)
		var old SalesCommitment
		e = t.First(&old, "order_id=?", o.ID).Error
		exists := e == nil
		if e != nil && !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		if exists {
			if old.OperatorID != o.OperatorID || old.ATG.Cmp(qty) != 0 {
				return ErrConflict
			}
			out = commerce.Commitment{Reference: old.ID, Version: old.CatalogVersion}
			if old.Status == "active" || old.Status == "fulfilled" {
				return nil
			}
			if !restore {
				return ErrConflict
			}
		}
		if snapshot.HasUpstream {
			demand, e := conservative(qty, rates.Direct)
			if e != nil {
				return e
			}
			if demand.Cmp(snapshot.Space) > 0 && !(restore && o.ExternalAllocatedMoney.Sign() > 0) {
				return ErrCoverage
			}
		}
		if exists {
			old.Status = "active"
			old.CatalogVersion = snapshot.CatalogVersion
			old.UpdatedAt = now
			if e = t.Model(&SalesCommitment{}).Where("id=?", old.ID).Updates(map[string]any{"status": old.Status, "catalog_version": old.CatalogVersion, "updated_at": now}).Error; e != nil {
				return e
			}
		} else {
			old = SalesCommitment{ID: uuid.NewString(), OperatorID: o.OperatorID, OrderID: o.ID, ATG: qty, Status: "active", CatalogVersion: snapshot.CatalogVersion, CreatedAt: now, UpdatedAt: now}
			if e = t.Create(&old).Error; e != nil {
				return e
			}
		}
		out = commerce.Commitment{Reference: old.ID, Version: old.CatalogVersion}
		return nil
	})
	return out, err
}
func (s *Service) Fulfill(ctx context.Context, tx *gorm.DB, o commerce.Order) error {
	return s.transition(ctx, tx, o, "fulfilled")
}
func (s *Service) Release(ctx context.Context, tx *gorm.DB, o commerce.Order) error {
	return s.transition(ctx, tx, o, "released")
}
func (s *Service) transition(ctx context.Context, tx *gorm.DB, o commerce.Order, status string) error {
	return s.within(ctx, tx, func(t *gorm.DB) error {
		if e := s.Lock(ctx, t, o.OperatorID); e != nil {
			return e
		}
		var row SalesCommitment
		if e := t.First(&row, "order_id=? AND operator_id=?", o.ID, o.OperatorID).Error; e != nil {
			return e
		}
		if row.Status == status {
			return nil
		}
		if row.Status != "active" {
			return ErrConflict
		}
		return t.Model(&SalesCommitment{}).Where("id=?", row.ID).Updates(map[string]any{"status": status, "updated_at": time.Now().UTC()}).Error
	})
}
