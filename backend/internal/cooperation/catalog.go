package cooperation

import (
	"context"
	"math/big"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type CatalogDirectory struct {
	Organization *organization.Service
	Tx           *gorm.DB
}

func (d CatalogDirectory) CatalogOperator(ctx context.Context, id string) (catalog.CatalogOperator, error) {
	o, e := d.Organization.GetOperator(ctx, d.Tx, id)
	if e != nil {
		return catalog.CatalogOperator{}, e
	}
	parent := ""
	if o.ParentOperatorID != nil {
		parent = *o.ParentOperatorID
	}
	return catalog.CatalogOperator{ID: o.ID, BrandID: o.BrandID, ParentID: parent, Kind: string(o.Kind)}, nil
}

type CatalogRates struct {
	Catalog      catalog.CommerceCatalog
	Organization *organization.Service
}

func (r CatalogRates) CoverageRates(ctx context.Context, tx *gorm.DB, operator string, at time.Time) (CoverageRates, error) {
	out := CoverageRates{Children: map[string]*big.Rat{}, ChildrenProvisional: map[string]bool{}}
	c := r.Catalog.WithDB(tx)
	c.Operators = CatalogDirectory{r.Organization, tx}
	version, e := c.LockRevision(ctx)
	if e != nil {
		return out, e
	}
	out.Version = int64(version)
	now, e := businessTime(ctx, tx)
	if e != nil {
		return out, e
	}
	op, e := r.Organization.GetOperator(ctx, tx, operator)
	if e != nil {
		return out, e
	}
	if op.Kind == organization.Platform {
		return out, nil
	}
	if op.ParentOperatorID == nil {
		return out, organization.ErrInvalidRelationship
	}
	upper := catalog.PriceScope{Kind: "supply", SellerID: *op.ParentOperatorID, BuyerID: op.ID}
	direct, complete, e := c.PreparedLiabilityRate(ctx, operator, catalog.PriceScope{Kind: "retail", SellerID: operator}, upper, at)
	if e != nil {
		return out, e
	}
	if complete {
		out.Direct = direct
	}
	out.DirectProvisional = !complete
	out.DirectLiability, e = observedCoverageRate(tx, operator, operator, direct, complete, at, now)
	if e != nil {
		return out, e
	}
	children, e := r.Organization.Children(ctx, tx, operator)
	if e != nil {
		return out, e
	}
	for _, p := range children {
		factor, complete, e := c.PreparedLiabilityRate(ctx, p.ID, catalog.PriceScope{Kind: "supply", SellerID: operator, BuyerID: p.ID}, upper, at)
		if e != nil {
			return out, e
		}
		out.Children[p.ID], e = observedCoverageRate(tx, operator, p.ID, factor, complete, at, now)
		if e != nil {
			return out, e
		}
		out.ChildrenProvisional[p.ID] = !complete
	}
	return out, nil
}

type coverageRateObservation struct {
	OperatorID             string `gorm:"primaryKey"`
	BeneficiaryID          string `gorm:"primaryKey"`
	Numerator, Denominator string
	UpdatedAt              time.Time
}

func (coverageRateObservation) TableName() string { return "cooperation_coverage_rates" }

// CatalogRates holds the shared catalog revision lock. Complete current prices
// replace the observation; incomplete configuration keeps known responsibility
// and reserves at least one upstream ATG per downstream ATG, never zero.
func observedCoverageRate(tx *gorm.DB, operator, beneficiary string, known *big.Rat, complete bool, at, now time.Time) (*big.Rat, error) {
	var prior coverageRateObservation
	found := tx.Where("operator_id=? AND beneficiary_id=?", operator, beneficiary).Limit(1).Find(&prior)
	if found.Error != nil {
		return nil, found.Error
	}
	var previous *big.Rat
	if found.RowsAffected > 0 {
		var ok bool
		previous, ok = new(big.Rat).SetString(prior.Numerator + "/" + prior.Denominator)
		if !ok || previous.Sign() < 0 {
			return nil, ErrCoverage
		}
	}
	if known != nil && !at.After(now) && (previous == nil || !at.Before(prior.UpdatedAt)) && (complete || previous == nil || known.Cmp(previous) > 0) {
		row := coverageRateObservation{operator, beneficiary, known.Num().String(), known.Denom().String(), at}
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "operator_id"}, {Name: "beneficiary_id"}}, DoUpdates: clause.AssignmentColumns([]string{"numerator", "denominator", "updated_at"})}).Create(&row).Error; err != nil {
			return nil, err
		}
		previous = known
	}
	if complete && known != nil {
		return new(big.Rat).Set(known), nil
	}
	factor := big.NewRat(1, 1)
	if previous != nil && previous.Cmp(factor) > 0 {
		factor.Set(previous)
	}
	if known != nil && known.Cmp(factor) > 0 {
		factor.Set(known)
	}
	return factor, nil
}
