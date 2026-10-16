package catalog

// This file defines versioned TokenHub retail, supply and supplier pricing.

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"sort"
	"time"
)

var (
	ErrPriceUnavailable  = errors.New("effective price unavailable")
	ErrUnboundedCoverage = errors.New("positive procurement price requires positive retail price")
	priceDecimalPattern  = regexp.MustCompile(`^(0|[1-9][0-9]*)(\.[0-9]{1,24})?$`)
)

// PriceDecimal accepts unsigned decimal strings, at most 24 fractional digits.
// It never rounds: callers pass the result to ledger.Quantize at settlement.
func PriceDecimal(value string) (*big.Rat, error) {
	if !priceDecimalPattern.MatchString(value) {
		return nil, fmt.Errorf("invalid price decimal %q", value)
	}
	r, ok := new(big.Rat).SetString(value)
	if !ok {
		return nil, ErrInvalidInput
	}
	return r, nil
}

// PriceScope identifies either an operator's retail book, or a directed supply
// edge. ATG and procurement ATG have separate books even when numbers coincide.
type PriceScope struct {
	Kind     string `json:"kind"` // retail | supply | supplier (USD per upstream)
	SellerID string `json:"seller_id"`
	BuyerID  string `json:"buyer_id,omitempty"`
}

func (s PriceScope) Key() string { return s.Kind + ":" + s.SellerID + ":" + s.BuyerID }
func (s PriceScope) Validate() error {
	if s.SellerID == "" || (s.Kind != "retail" && s.Kind != "supply" && s.Kind != "supplier") || ((s.Kind == "supply" || s.Kind == "supplier") && (s.BuyerID == "" || s.BuyerID == s.SellerID)) || (s.Kind == "retail" && s.BuyerID != "") {
		return ErrInvalidInput
	}
	return nil
}
func (s PriceScope) AssetUnit() string {
	switch s.Kind {
	case "supplier":
		return "USD"
	case "supply":
		return "procurement_ATG"
	default:
		return "ATG"
	}
}

// ComponentRate describes one billable unit and one exact specification. Keys
// include specification/condition identity, e.g. image:1024x1024:high. Tiers are
// progressive: [From,To); an omitted To covers the remainder. A zero quantity
// must never incur a fixed fee; per-request fees use a separate request unit.
type ComponentRate struct {
	Key   string      `json:"key"`
	Unit  string      `json:"unit"`
	Per   string      `json:"per"`
	Rate  string      `json:"rate"`
	Tiers []PriceTier `json:"tiers,omitempty"`
}
type PriceTier struct {
	From string `json:"from"`
	To   string `json:"to,omitempty"`
	Rate string `json:"rate"`
}
type PriceRule struct {
	Mode             string            `json:"mode"` // manual | follow
	Components       []ComponentRate   `json:"components,omitempty"`
	Follow           *PriceScope       `json:"follow,omitempty"`
	Multiplier       string            `json:"multiplier,omitempty"`
	ModelMultipliers map[string]string `json:"model_multipliers,omitempty"`
}
type PriceVersion struct {
	ID          string     `json:"id"`
	Scope       PriceScope `json:"scope"`
	ModelID     string     `json:"model_id"`
	Layer       string     `json:"layer"` // regular | promotion
	EffectiveAt time.Time  `json:"effective_at"`
	EndsAt      *time.Time `json:"ends_at,omitempty"`
	Rule        PriceRule  `json:"rule"`
	NoticeID    string     `json:"notice_id"`
	Published   bool       `json:"published"`
}
type PriceVersionSource interface {
	PriceVersions(context.Context, PriceScope, string) ([]PriceVersion, error)
}
type PriceLineage struct {
	VersionID  string     `json:"version_id"`
	Scope      PriceScope `json:"scope"`
	Layer      string     `json:"layer"`
	Multiplier string     `json:"multiplier,omitempty"`
}

// ResolvedPrice is immutable request evidence, including the whole follow chain.
// Rational factors preserve exact follow multiplication without decimal rounding.
type ResolvedPrice struct {
	AssetUnit         string          `json:"asset_unit"`
	Scope             PriceScope      `json:"scope"`
	ModelID           string          `json:"model_id"`
	AcceptedAt        time.Time       `json:"accepted_at"`
	Components        []ComponentRate `json:"components"`
	FactorNumerator   string          `json:"factor_numerator"`
	FactorDenominator string          `json:"factor_denominator"`
	Lineage           []PriceLineage  `json:"lineage"`
}
type PriceResolver struct{ Source PriceVersionSource }

func (r PriceResolver) Resolve(ctx context.Context, scope PriceScope, model string, at time.Time) (ResolvedPrice, error) {
	if r.Source == nil || scope.Validate() != nil || model == "" || at.IsZero() {
		return ResolvedPrice{}, ErrInvalidInput
	}
	return r.resolve(ctx, scope, model, at.UTC(), map[string]bool{})
}
func (r PriceResolver) resolve(ctx context.Context, scope PriceScope, model string, at time.Time, seen map[string]bool) (ResolvedPrice, error) {
	key := scope.Key() + ":" + model
	if seen[key] || len(seen) >= 16 {
		return ResolvedPrice{}, errors.New("price follow cycle or excessive depth")
	}
	seen[key] = true
	defer delete(seen, key)
	versions, err := r.Source.PriceVersions(ctx, scope, model)
	if err != nil {
		return ResolvedPrice{}, err
	}
	var regular, promo *PriceVersion
	for i := range versions {
		v := &versions[i]
		if !v.Published || v.Scope != scope || (v.ModelID != model && v.ModelID != "*") || v.EffectiveAt.After(at) || (v.EndsAt != nil && !at.Before(*v.EndsAt)) {
			continue
		}
		if err := ValidatePriceVersion(*v); err != nil {
			return ResolvedPrice{}, err
		}
		switch v.Layer {
		case "regular":
			if regular == nil || (v.ModelID == model && regular.ModelID == "*") || (v.ModelID == regular.ModelID && v.EffectiveAt.After(regular.EffectiveAt)) {
				regular = v
			} else if v.ModelID == regular.ModelID && v.EffectiveAt.Equal(regular.EffectiveAt) {
				return ResolvedPrice{}, errors.New("ambiguous regular price")
			}
		case "promotion":
			if promo != nil {
				return ResolvedPrice{}, errors.New("overlapping model promotions")
			}
			promo = v
		}
	}
	v := regular
	if promo != nil {
		v = promo
	}
	if v == nil {
		return ResolvedPrice{}, ErrPriceUnavailable
	}
	multiplier := v.Rule.Multiplier
	if override, ok := v.Rule.ModelMultipliers[model]; ok {
		multiplier = override
	}
	line := PriceLineage{VersionID: v.ID, Scope: scope, Layer: v.Layer, Multiplier: multiplier}
	if v.Rule.Mode == "manual" {
		// Deep copy by value, including the tier slices, so later configuration
		// edits cannot change an accepted request's in-memory snapshot.
		components := append([]ComponentRate(nil), v.Rule.Components...)
		for i := range components {
			components[i].Tiers = append([]PriceTier(nil), components[i].Tiers...)
		}
		return ResolvedPrice{AssetUnit: scope.AssetUnit(), Scope: scope, ModelID: model, AcceptedAt: at, Components: components, FactorNumerator: "1", FactorDenominator: "1", Lineage: []PriceLineage{line}}, nil
	}
	base, err := r.resolve(ctx, *v.Rule.Follow, model, at, seen)
	if err != nil {
		return ResolvedPrice{}, err
	}
	factor, err := base.Factor()
	if err != nil {
		return ResolvedPrice{}, err
	}
	mult, _ := PriceDecimal(multiplier)
	factor.Mul(factor, mult)
	base.Scope = scope
	base.AssetUnit = scope.AssetUnit()
	base.FactorNumerator = factor.Num().String()
	base.FactorDenominator = factor.Denom().String()
	base.Lineage = append([]PriceLineage{line}, base.Lineage...)
	return base, nil
}
func ValidatePriceVersion(v PriceVersion) error {
	if v.ID == "" || v.ModelID == "" || v.Scope.Validate() != nil || v.EffectiveAt.IsZero() || (v.Layer != "regular" && v.Layer != "promotion") || (v.EndsAt != nil && !v.EndsAt.After(v.EffectiveAt)) || (v.Layer == "promotion" && v.EndsAt == nil) {
		return ErrInvalidInput
	}
	switch v.Rule.Mode {
	case "follow":
		if v.Scope.Kind == "supplier" {
			return ErrInvalidInput
		}
		if v.Rule.Follow == nil || v.Rule.Follow.Validate() != nil || *v.Rule.Follow == v.Scope || len(v.Rule.Components) > 0 {
			return ErrInvalidInput
		}
		m, e := PriceDecimal(v.Rule.Multiplier)
		if e != nil || m.Sign() <= 0 {
			return ErrInvalidInput
		}
		for model, value := range v.Rule.ModelMultipliers {
			m, e := PriceDecimal(value)
			if model == "" || model == "*" || e != nil || m.Sign() <= 0 {
				return ErrInvalidInput
			}
		}
	case "manual":
		if v.ModelID == "*" || len(v.Rule.Components) == 0 || v.Rule.Follow != nil || v.Rule.Multiplier != "" || len(v.Rule.ModelMultipliers) > 0 {
			return ErrInvalidInput
		}
		seen := map[string]bool{}
		for _, c := range v.Rule.Components {
			if c.Key == "" || c.Unit == "" || seen[c.Key] {
				return ErrInvalidInput
			}
			seen[c.Key] = true
			per, e := PriceDecimal(c.Per)
			if e != nil || per.Sign() <= 0 {
				return ErrInvalidInput
			}
			if _, e = PriceDecimal(c.Rate); e != nil {
				return e
			}
			last := new(big.Rat)
			for i, t := range c.Tiers {
				from, e := PriceDecimal(t.From)
				if e != nil || from.Cmp(last) != 0 {
					return errors.New("price tiers must be contiguous and start at zero")
				}
				if _, e = PriceDecimal(t.Rate); e != nil {
					return e
				}
				if t.To == "" {
					if i != len(c.Tiers)-1 {
						return ErrInvalidInput
					}
				} else {
					to, e := PriceDecimal(t.To)
					if e != nil || to.Cmp(from) <= 0 {
						return ErrInvalidInput
					}
					last = to
				}
			}
			if len(c.Tiers) > 0 && c.Tiers[len(c.Tiers)-1].To != "" {
				return errors.New("last price tier must be unbounded")
			}
		}
	default:
		return ErrInvalidInput
	}
	if v.ModelID == "*" && v.Layer != "regular" {
		return ErrInvalidInput
	}
	return nil
}
func (p ResolvedPrice) Factor() (*big.Rat, error) {
	n, ok := new(big.Int).SetString(p.FactorNumerator, 10)
	if !ok || n.Sign() < 0 {
		return nil, ErrInvalidInput
	}
	d, ok := new(big.Int).SetString(p.FactorDenominator, 10)
	if !ok || d.Sign() <= 0 {
		return nil, ErrInvalidInput
	}
	return new(big.Rat).SetFrac(n, d), nil
}

// Charge requires a quantity (including explicit zero) for every component.
// Omitted evidence cannot accidentally settle as a free request.
func (p ResolvedPrice) Charge(quantities map[string]string) (*big.Rat, error) {
	factor, err := p.Factor()
	if err != nil {
		return nil, err
	}
	total := new(big.Rat)
	known := map[string]bool{}
	for _, c := range p.Components {
		known[c.Key] = true
		raw, ok := quantities[c.Key]
		if !ok {
			return nil, fmt.Errorf("missing usage %s", c.Key)
		}
		q, e := PriceDecimal(raw)
		if e != nil {
			return nil, e
		}
		per, e := PriceDecimal(c.Per)
		if e != nil || per.Sign() <= 0 {
			return nil, ErrInvalidInput
		}
		cost := new(big.Rat)
		if len(c.Tiers) == 0 {
			rate, e := PriceDecimal(c.Rate)
			if e != nil {
				return nil, e
			}
			cost.Mul(q, rate)
		} else {
			for _, t := range c.Tiers {
				from, _ := PriceDecimal(t.From)
				if q.Cmp(from) <= 0 {
					break
				}
				end := new(big.Rat).Set(q)
				if t.To != "" {
					to, _ := PriceDecimal(t.To)
					if end.Cmp(to) > 0 {
						end = to
					}
				}
				rate, _ := PriceDecimal(t.Rate)
				cost.Add(cost, new(big.Rat).Mul(new(big.Rat).Sub(end, from), rate))
			}
		}
		total.Add(total, cost.Quo(cost, per))
	}
	for k := range quantities {
		if !known[k] {
			return nil, fmt.Errorf("unknown usage component %s", k)
		}
	}
	return total.Mul(total, factor), nil
}

// CoverageFactor is a conservative upper bound over every component and tier.
// Ratios use the lowest retail marginal rate and highest procurement rate.
// A fixed per-request fee is represented by a request component on both books.
func CoverageFactor(retail, procurement ResolvedPrice) (*big.Rat, error) {
	if retail.ModelID != procurement.ModelID {
		return nil, ErrInvalidInput
	}
	rf, e := retail.Factor()
	if e != nil {
		return nil, e
	}
	pf, e := procurement.Factor()
	if e != nil {
		return nil, e
	}
	rates := map[string]ComponentRate{}
	for _, c := range retail.Components {
		rates[c.Key] = c
	}
	max := new(big.Rat)
	for _, p := range procurement.Components {
		r, ok := rates[p.Key]
		if !ok || r.Unit != p.Unit {
			return nil, fmt.Errorf("incomparable component %s", p.Key)
		}
		pr, e := extremeRate(p, false)
		if e != nil {
			return nil, e
		}
		pr.Mul(pr, pf)
		if pr.Sign() == 0 {
			continue
		}
		rr, e := extremeRate(r, true)
		if e != nil {
			return nil, e
		}
		rr.Mul(rr, rf)
		if rr.Sign() == 0 {
			return nil, fmt.Errorf("%w: %s", ErrUnboundedCoverage, p.Key)
		}
		k := new(big.Rat).Quo(pr, rr)
		if k.Cmp(max) > 0 {
			max = k
		}
	}
	return max, nil
}
func extremeRate(c ComponentRate, min bool) (*big.Rat, error) {
	per, e := PriceDecimal(c.Per)
	if e != nil || per.Sign() <= 0 {
		return nil, ErrInvalidInput
	}
	values := []string{c.Rate}
	if len(c.Tiers) > 0 {
		values = nil
		for _, t := range c.Tiers {
			values = append(values, t.Rate)
		}
	}
	var out *big.Rat
	for _, v := range values {
		r, e := PriceDecimal(v)
		if e != nil {
			return nil, e
		}
		r.Quo(r, per)
		if out == nil || (min && r.Cmp(out) < 0) || (!min && r.Cmp(out) > 0) {
			out = r
		}
	}
	if out == nil {
		return nil, ErrInvalidInput
	}
	return out, nil
}

// MaximumCharge is exact for progressive nonnegative tiers, including fixed
// request units. Bounds must come from the verified route's admitted limits.
func (p ResolvedPrice) MaximumCharge(bounds map[string]string) (*big.Rat, error) {
	return p.Charge(bounds)
}

func SortPriceVersions(v []PriceVersion) {
	sort.Slice(v, func(i, j int) bool { return v[i].EffectiveAt.Before(v[j].EffectiveAt) })
}
