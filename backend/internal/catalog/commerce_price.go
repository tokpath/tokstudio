package catalog

// This file defines versioned TokenHub retail and supply pricing.

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"sort"
	"strings"
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
	Kind     string `json:"kind"` // retail | supply
	SellerID string `json:"seller_id"`
	BuyerID  string `json:"buyer_id,omitempty"`
}

func (s PriceScope) Key() string { return s.Kind + ":" + s.SellerID + ":" + s.BuyerID }
func (s PriceScope) Validate() error {
	if s.SellerID == "" || (s.Kind != "retail" && s.Kind != "supply") || (s.Kind == "supply" && (s.BuyerID == "" || s.BuyerID == s.SellerID)) || (s.Kind == "retail" && s.BuyerID != "") {
		return ErrInvalidInput
	}
	return nil
}
func (s PriceScope) AssetUnit() string {
	switch s.Kind {
	case "supply":
		return "procurement_ATG"
	default:
		return "ATG"
	}
}

// ComponentRate describes one billable unit and one exact specification. Keys
// include specification/condition identity, e.g. image:1024x1024:high. The same
// rate applies to the whole quantity. A zero quantity must never incur a fixed
// fee; per-request fees use a separate request unit.
type ComponentRate struct {
	Key  string `json:"key"`
	Unit string `json:"unit"`
	Per  string `json:"per"`
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
		// Copy the component list so a later price edit cannot change a request
		// that already accepted this snapshot.
		components := append([]ComponentRate(nil), v.Rule.Components...)
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

// resolutionParent separates a meter key from one specification suffix.
// image:1024x1024:high is not a single suffix and stays one exact key.
func resolutionParent(key string) (string, bool) {
	parent, child, ok := strings.Cut(key, ":")
	if !ok || parent == "" || child == "" || strings.Contains(child, ":") {
		return "", false
	}
	return parent, true
}
func quantityHasChild(quantities map[string]string, parent string) bool {
	prefix := parent + ":"
	for key := range quantities {
		if strings.HasPrefix(key, prefix) && !strings.Contains(key[len(prefix):], ":") {
			return true
		}
	}
	return false
}
func (p ResolvedPrice) matchingRate(key string) (ComponentRate, bool) {
	for _, component := range p.Components {
		if component.Key == key {
			return component, true
		}
	}
	parent, ok := resolutionParent(key)
	if !ok {
		return ComponentRate{}, false
	}
	for _, component := range p.Components {
		if component.Key == parent {
			return component, true
		}
	}
	return ComponentRate{}, false
}
func (p ResolvedPrice) childRate(parent string) (ComponentRate, bool, error) {
	var best *ComponentRate
	var bestUnit *big.Rat
	for i := range p.Components {
		got, ok := resolutionParent(p.Components[i].Key)
		if !ok || got != parent {
			continue
		}
		unit, e := extremeRate(p.Components[i], false)
		if e != nil {
			return ComponentRate{}, false, e
		}
		if best == nil || unit.Cmp(bestUnit) > 0 {
			best = &p.Components[i]
			bestUnit = unit
		}
	}
	if best == nil {
		return ComponentRate{}, false, nil
	}
	return *best, true, nil
}
func applyCost(total *big.Rat, component ComponentRate, raw string) error {
	quantity, e := PriceDecimal(raw)
	if e != nil {
		return e
	}
	per, e := PriceDecimal(component.Per)
	if e != nil || per.Sign() <= 0 {
		return ErrInvalidInput
	}
	rate, e := PriceDecimal(component.Rate)
	if e != nil {
		return e
	}
	cost := new(big.Rat).Mul(quantity, rate)
	total.Add(total, cost.Quo(cost, per))
	return nil
}

// componentCovers accepts a listed resolution price, its base meter, or any
// priced resolution when an older price has no separate base component.
func componentCovers(components []ComponentRate, key, unit string) bool {
	for _, rate := range components {
		if rate.Unit == unit && rate.Key == key {
			return true
		}
	}
	if parent, ok := resolutionParent(key); ok {
		for _, rate := range components {
			if rate.Unit == unit && rate.Key == parent {
				return true
			}
		}
	}
	for _, rate := range components {
		parent, ok := resolutionParent(rate.Key)
		if ok && parent == key && rate.Unit == unit {
			return true
		}
	}
	return false
}

// Charge requires a quantity (including explicit zero) for every component.
// Omitted evidence cannot accidentally settle as a free request. A resolution
// quantity without its own price uses the base meter. A base quantity that is
// absent because only resolutions were measured is zero.
func (p ResolvedPrice) Charge(quantities map[string]string) (*big.Rat, error) {
	factor, err := p.Factor()
	if err != nil {
		return nil, err
	}
	total := new(big.Rat)
	known := map[string]bool{}
	for _, component := range p.Components {
		known[component.Key] = true
		raw, ok := quantities[component.Key]
		if !ok {
			if quantityHasChild(quantities, component.Key) {
				raw = "0"
			} else {
				return nil, fmt.Errorf("missing usage %s", component.Key)
			}
		}
		if e := applyCost(total, component, raw); e != nil {
			return nil, e
		}
	}
	for key, raw := range quantities {
		if known[key] {
			continue
		}
		if rate, ok := p.matchingRate(key); ok {
			if e := applyCost(total, rate, raw); e != nil {
				return nil, e
			}
			continue
		}
		amount, e := PriceDecimal(raw)
		if e != nil {
			return nil, e
		}
		if amount.Sign() > 0 {
			rate, ok, e := p.childRate(key)
			if e != nil {
				return nil, e
			}
			if ok {
				if e = applyCost(total, rate, raw); e != nil {
					return nil, e
				}
				continue
			}
		} else if _, child := resolutionParent(key); child {
			continue
		} else if _, ok, e := p.childRate(key); e != nil {
			return nil, e
		} else if ok {
			continue
		}
		return nil, fmt.Errorf("unknown usage component %s", key)
	}
	return total.Mul(total, factor), nil
}

// CoverageFactor is a conservative upper bound over every component, including a
// resolution that one side prices specifically and the other side prices on the base meter.
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
	keys := map[string]bool{}
	for _, component := range retail.Components {
		keys[component.Key] = true
	}
	for _, component := range procurement.Components {
		keys[component.Key] = true
	}
	max := new(big.Rat)
	for key := range keys {
		retailRate, e := pricedForCoverage(retail, key)
		if e != nil {
			return nil, e
		}
		procurementRate, e := pricedForCoverage(procurement, key)
		if e != nil {
			return nil, e
		}
		if retailRate.Unit != procurementRate.Unit {
			return nil, fmt.Errorf("incomparable component %s", key)
		}
		pr, e := extremeRate(procurementRate, false)
		if e != nil {
			return nil, e
		}
		pr.Mul(pr, pf)
		if pr.Sign() == 0 {
			continue
		}
		rr, e := extremeRate(retailRate, true)
		if e != nil {
			return nil, e
		}
		rr.Mul(rr, rf)
		if rr.Sign() == 0 {
			return nil, fmt.Errorf("%w: %s", ErrUnboundedCoverage, key)
		}
		k := new(big.Rat).Quo(pr, rr)
		if k.Cmp(max) > 0 {
			max = k
		}
	}
	return max, nil
}
func pricedForCoverage(price ResolvedPrice, key string) (ComponentRate, error) {
	if rate, ok := price.matchingRate(key); ok {
		return rate, nil
	}
	rate, ok, e := price.childRate(key)
	if e != nil {
		return ComponentRate{}, e
	}
	if !ok {
		return ComponentRate{}, fmt.Errorf("incomparable component %s", key)
	}
	return rate, nil
}
func extremeRate(c ComponentRate, min bool) (*big.Rat, error) {
	per, e := PriceDecimal(c.Per)
	if e != nil || per.Sign() <= 0 {
		return nil, ErrInvalidInput
	}
	values := []string{c.Rate}
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

// MaximumCharge is exact for a nonnegative unit rate, including a fixed
// per-request component. Bounds must come from the verified route's admitted limits.
func (p ResolvedPrice) MaximumCharge(bounds map[string]string) (*big.Rat, error) {
	return p.Charge(bounds)
}

func SortPriceVersions(v []PriceVersion) {
	sort.Slice(v, func(i, j int) bool { return v[i].EffectiveAt.Before(v[j].EffectiveAt) })
}
