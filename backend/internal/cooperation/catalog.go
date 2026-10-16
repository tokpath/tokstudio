package cooperation

import (
	"context"
	"math/big"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"gorm.io/gorm"
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
	out := CoverageRates{Children: map[string]*big.Rat{}}
	c := r.Catalog.WithDB(tx)
	c.Operators = CatalogDirectory{r.Organization, tx}
	version, e := c.LockRevision(ctx)
	if e != nil {
		return out, e
	}
	out.Version = int64(version)
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
	out.Direct, e = c.MaximumLiabilityRate(ctx, operator, catalog.PriceScope{Kind: "retail", SellerID: operator}, upper, at)
	if e != nil {
		return out, e
	}
	// The cooperation module owns the outstanding wholesale positions. Only
	// buyers with delivered liability need a wholesale coverage factor.
	children, e := r.Organization.Children(ctx, tx, operator)
	if e != nil {
		return out, e
	}
	for _, p := range children {
		factor, e := c.MaximumLiabilityRate(ctx, p.ID, catalog.PriceScope{Kind: "supply", SellerID: operator, BuyerID: p.ID}, upper, at)
		if e != nil {
			return out, e
		}
		out.Children[p.ID] = factor
	}
	return out, nil
}
