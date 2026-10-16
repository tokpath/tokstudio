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
		e := s.authorized(ctx, operator, m.ID)
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
// Callers must refuse new ATG commitments in that state instead of treating
// missing prices as zero procurement cost.
func (s CommerceCatalog) MaximumLiabilityRate(ctx context.Context, operator string, downstream, upstream PriceScope, at time.Time) (*big.Rat, error) {
	models, e := s.AvailableModels(ctx, operator, downstream.Kind == "retail")
	if e != nil {
		return nil, e
	}
	resolver := PriceResolver{Source: s}
	var maximum *big.Rat
	for _, m := range models {
		lower, e := resolver.Resolve(ctx, downstream, m.ID, at)
		if e != nil {
			return nil, e
		}
		higher, e := resolver.Resolve(ctx, upstream, m.ID, at)
		if e != nil {
			return nil, e
		}
		factor, e := CoverageFactor(lower, higher)
		if e != nil {
			return nil, e
		}
		if maximum == nil || factor.Cmp(maximum) > 0 {
			maximum = new(big.Rat).Set(factor)
		}
	}
	return maximum, nil
}
