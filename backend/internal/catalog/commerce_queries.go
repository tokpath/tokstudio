package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"time"
)

// AvailableModels respects every ancestor authorization. A retailer additionally
// requires its own listing; a wholesale obligation includes authorized models
// even when the buyer has temporarily taken them off its retail shelf.
func (s CommerceCatalog) AvailableModels(ctx context.Context, operator string, retail bool) ([]PublicModel, error) {
	return s.availableModels(ctx, operator, retail, true)
}

// PricingModels also includes unlisted and globally disabled models so an
// operator can prepare prices before opening sales or inspect a paused model.
func (s CommerceCatalog) PricingModels(ctx context.Context, operator string) ([]PublicModel, error) {
	return s.availableModels(ctx, operator, false, false)
}

func (s CommerceCatalog) availableModels(ctx context.Context, operator string, retail, requireEnabled bool) ([]PublicModel, error) {
	var rows []catalogJSONRow
	if e := s.DB.WithContext(ctx).Table("tokenhub_public_models").Order("id").Find(&rows).Error; e != nil {
		return nil, e
	}
	out := []PublicModel{}
	for _, row := range rows {
		var m PublicModel
		if e := json.Unmarshal(row.Definition, &m); e != nil {
			return nil, e
		}
		e := s.requireAuthorization(ctx, operator, m.ID, requireEnabled)
		if errors.Is(e, ErrModelNotVisible) {
			continue
		}
		if e != nil {
			return nil, e
		}
		if retail {
			var count int64
			if e = s.DB.WithContext(ctx).Table("tokenhub_model_listings").Where("operator_id=? AND public_model_id=? AND enabled=true", operator, m.ID).Count(&count).Error; e != nil {
				return nil, e
			}
			if count == 0 {
				continue
			}
		}
		out = append(out, m)
	}
	return out, nil
}

// MaximumLiabilityRate returns nil when no model can fulfill the obligation.
// Retail callers must refuse new commitments in that state. Wholesale callers
// can use PreparedLiabilityRate to reserve stock while configuration is pending.
func (s CommerceCatalog) MaximumLiabilityRate(ctx context.Context, operator string, downstream, upstream PriceScope, at time.Time) (*big.Rat, error) {
	maximum, _, err := s.liabilityRate(ctx, operator, downstream, upstream, at, false)
	return maximum, err
}

// PreparedLiabilityRate retains every known component while distinguishing
// absent models/prices from zero cost. Invalid or unbounded prices still fail.
func (s CommerceCatalog) PreparedLiabilityRate(ctx context.Context, operator string, downstream, upstream PriceScope, at time.Time) (*big.Rat, bool, error) {
	return s.liabilityRate(ctx, operator, downstream, upstream, at, true)
}

func (s CommerceCatalog) liabilityRate(ctx context.Context, operator string, downstream, upstream PriceScope, at time.Time, partial bool) (*big.Rat, bool, error) {
	models, e := s.AvailableModels(ctx, operator, downstream.Kind == "retail")
	if e != nil {
		return nil, false, e
	}
	complete := len(models) > 0
	resolver := PriceResolver{Source: s}
	var maximum *big.Rat
	for _, m := range models {
		lower, e := resolver.Resolve(ctx, downstream, m.ID, at)
		if partial && errors.Is(e, ErrPriceUnavailable) {
			complete = false
			continue
		}
		if e != nil {
			return nil, false, e
		}
		higher, e := resolver.Resolve(ctx, upstream, m.ID, at)
		if partial && errors.Is(e, ErrPriceUnavailable) {
			complete = false
			continue
		}
		if e != nil {
			return nil, false, e
		}
		factor, e := CoverageFactor(lower, higher)
		if e != nil {
			return nil, false, e
		}
		if maximum == nil || factor.Cmp(maximum) > 0 {
			maximum = new(big.Rat).Set(factor)
		}
	}
	return maximum, complete, nil
}
