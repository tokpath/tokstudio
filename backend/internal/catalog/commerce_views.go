package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"strings"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/modelruntime"
)

type PublicCapability struct {
	Operation            modelruntime.Operation                      `json:"operation"`
	Streaming            bool                                        `json:"streaming"`
	InputTypes           []string                                    `json:"input_types"`
	Parameters           []string                                    `json:"parameters"`
	RequiredParameters   []string                                    `json:"required_parameters"`
	MaxInputBytes        int                                         `json:"max_input_bytes"`
	MaxOutput            int64                                       `json:"max_output"`
	OutputLimitParameter string                                      `json:"output_limit_parameter,omitempty"`
	ParameterConstraints map[string]modelruntime.ParameterConstraint `json:"parameter_constraints,omitempty"`
}
type PublicPriceComponent struct {
	Component string      `json:"component"`
	AmountATG string      `json:"amount_atg"`
	Per       string      `json:"per"`
	Unit      string      `json:"unit"`
	Tiers     []PriceTier `json:"tiers,omitempty"`
}
type PublicCatalogModel struct {
	ID              string                 `json:"id"`
	Name            string                 `json:"name"`
	Status          string                 `json:"status"`
	Capabilities    []PublicCapability     `json:"capabilities"`
	Prices          []PublicPriceComponent `json:"prices"`
	UpcomingChanges []PublicCatalogChange  `json:"upcoming_changes,omitempty"`
}

// PublicCatalog contains retail prices and public limits only. Provider routes,
// keys, supplier costs and verification evidence are never included.
func (s CommerceCatalog) PublicCatalog(ctx context.Context, operator string, at time.Time) ([]PublicCatalogModel, error) {
	models, e := s.publicDirectory(ctx, operator)
	if e != nil {
		return nil, e
	}
	out := []PublicCatalogModel{}
	for _, m := range models {
		view := PublicCatalogModel{ID: m.ID, Name: m.DisplayName, Status: "available", Capabilities: []PublicCapability{}, Prices: []PublicPriceComponent{}}
		if !m.Enabled {
			view.Status = "disabled"
		}
		view.UpcomingChanges, e = s.PublicChanges(ctx, operator, m.ID, at)
		if e != nil {
			return nil, e
		}
		for _, c := range m.Capabilities {
			if c.Verified() {
				view.Capabilities = append(view.Capabilities, PublicCapability{c.Operation, c.Streaming, c.InputTypes, c.Parameters, c.RequiredParameters, c.MaxInputBytes, c.MaxOutput, c.OutputLimitParameter, c.ParameterConstraints})
			}
		}
		price, e := (PriceResolver{Source: s}).Resolve(ctx, PriceScope{Kind: "retail", SellerID: operator}, m.ID, at)
		if e != nil {
			if m.Enabled {
				view.Status = "pricing_unavailable"
			}
			out = append(out, view)
			continue
		}
		factor, e := price.Factor()
		if e != nil {
			return nil, e
		}
		for _, c := range price.Components {
			rate, e := PriceDecimal(c.Rate)
			if e != nil {
				return nil, e
			}
			amount, e := exactPublicDecimal(new(big.Rat).Mul(rate, factor))
			if e != nil {
				return nil, e
			}
			component := PublicPriceComponent{Component: c.Key, AmountATG: amount, Per: c.Per, Unit: c.Unit, Tiers: append([]PriceTier(nil), c.Tiers...)}
			for i := range component.Tiers {
				rate, e := PriceDecimal(component.Tiers[i].Rate)
				if e != nil {
					return nil, e
				}
				component.Tiers[i].Rate, e = exactPublicDecimal(new(big.Rat).Mul(rate, factor))
				if e != nil {
					return nil, e
				}
			}
			view.Prices = append(view.Prices, component)
		}
		out = append(out, view)
	}
	return out, nil
}

func (s CommerceCatalog) publicDirectory(ctx context.Context, operator string) ([]PublicModel, error) {
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
		if e := s.requireAuthorization(ctx, operator, m.ID, false); e != nil {
			if errors.Is(e, ErrModelNotVisible) {
				continue
			}
			return nil, e
		}
		var listed int64
		if e := s.DB.WithContext(ctx).Table("tokenhub_model_listings").Where("operator_id=? AND public_model_id=? AND enabled=true", operator, m.ID).Count(&listed).Error; e != nil {
			return nil, e
		}
		if listed > 0 {
			out = append(out, m)
		}
	}
	return out, nil
}
func exactPublicDecimal(value *big.Rat) (string, error) {
	den := new(big.Int).Set(value.Denom())
	two, five := big.NewInt(2), big.NewInt(5)
	twos, fives := 0, 0
	for new(big.Int).Mod(den, two).Sign() == 0 {
		den.Quo(den, two)
		twos++
	}
	for new(big.Int).Mod(den, five).Sign() == 0 {
		den.Quo(den, five)
		fives++
	}
	if den.Cmp(big.NewInt(1)) != 0 {
		return "", ErrInvalidInput
	}
	digits := twos
	if fives > digits {
		digits = fives
	}
	if digits > 512 {
		return "", ErrInvalidInput
	}
	text := value.FloatString(digits)
	if strings.Contains(text, ".") {
		text = strings.TrimRight(strings.TrimRight(text, "0"), ".")
	}
	return text, nil
}
func (s CommerceCatalog) AdminModels(ctx context.Context, actor string) ([]PublicModel, error) {
	if !s.platform(actor) {
		return nil, ErrCatalogForbidden
	}
	var rows []catalogJSONRow
	if e := s.DB.WithContext(ctx).Table("tokenhub_public_models").Order("id").Find(&rows).Error; e != nil {
		return nil, e
	}
	out := []PublicModel{}
	for _, row := range rows {
		m, e := s.PublicModel(ctx, row.ID)
		if e != nil {
			return nil, e
		}
		out = append(out, m)
	}
	return out, nil
}
func (s CommerceCatalog) AdminUpstreams(ctx context.Context, actor, model string) ([]UpstreamModel, error) {
	if !s.platform(actor) {
		return nil, ErrCatalogForbidden
	}
	return s.upstreams(ctx, model)
}

type AuthorizationView struct {
	SellerID      string `json:"seller_id"`
	BuyerID       string `json:"buyer_id"`
	PublicModelID string `json:"public_model_id"`
	Enabled       bool   `json:"enabled"`
	Version       uint64 `json:"version"`
}

func (s CommerceCatalog) Authorizations(ctx context.Context, actor, operator string) ([]AuthorizationView, error) {
	if actor != operator {
		if s.Operators == nil {
			return nil, ErrCatalogForbidden
		}
		child, e := s.Operators.CatalogOperator(ctx, operator)
		if e != nil || child.ParentID != actor {
			return nil, ErrCatalogForbidden
		}
	}
	out := []AuthorizationView{}
	e := s.DB.WithContext(ctx).Table("tokenhub_model_authorizations").Where("seller_id=? OR buyer_id=?", operator, operator).Order("seller_id,buyer_id,public_model_id").Find(&out).Error
	return out, e
}

func (s CommerceCatalog) AdminCredentials(ctx context.Context, platformActor string) ([]CredentialView, error) {
	if platformActor != s.PlatformOperatorID {
		return nil, ErrCatalogForbidden
	}
	out := []CredentialView{}
	e := s.DB.WithContext(ctx).Table("tokenhub_provider_credentials").Select("id,provider_id,enabled,version").Order("provider_id,id").Scan(&out).Error
	return out, e
}

type PublicCatalogChange struct {
	ID          string                 `json:"id"`
	Kind        string                 `json:"kind"`
	ModelID     string                 `json:"model_id"`
	EffectiveAt time.Time              `json:"effective_at"`
	Status      string                 `json:"status"`
	Prices      []PublicPriceComponent `json:"prices,omitempty"`
}
type plannedPriceSource struct {
	Base PriceVersionSource
	Plan *PriceVersion
}

func (p plannedPriceSource) PriceVersions(ctx context.Context, scope PriceScope, model string) ([]PriceVersion, error) {
	rows, e := p.Base.PriceVersions(ctx, scope, model)
	if e != nil {
		return nil, e
	}
	if p.Plan != nil && p.Plan.Scope == scope && (p.Plan.ModelID == model || p.Plan.ModelID == "*") {
		exists := false
		for _, v := range rows {
			if v.ID == p.Plan.ID {
				exists = true
			}
		}
		if !exists {
			rows = append(rows, *p.Plan)
		}
	}
	return rows, nil
}
func publicPriceComponents(price ResolvedPrice) ([]PublicPriceComponent, error) {
	out := []PublicPriceComponent{}
	factor, e := price.Factor()
	if e != nil {
		return nil, e
	}
	for _, c := range price.Components {
		rate, e := PriceDecimal(c.Rate)
		if e != nil {
			return nil, e
		}
		amount, e := exactPublicDecimal(new(big.Rat).Mul(rate, factor))
		if e != nil {
			return nil, e
		}
		component := PublicPriceComponent{Component: c.Key, AmountATG: amount, Per: c.Per, Unit: c.Unit, Tiers: append([]PriceTier(nil), c.Tiers...)}
		for i := range component.Tiers {
			rate, e := PriceDecimal(component.Tiers[i].Rate)
			if e != nil {
				return nil, e
			}
			component.Tiers[i].Rate, e = exactPublicDecimal(new(big.Rat).Mul(rate, factor))
			if e != nil {
				return nil, e
			}
		}
		out = append(out, component)
	}
	return out, nil
}

// PublicChanges exposes effective retail terms only. Supply rates, routing,
// provider identities and technical evidence never enter the customer preview.
func (s CommerceCatalog) PublicChanges(ctx context.Context, operator, model string, at time.Time) ([]PublicCatalogChange, error) {
	rows := []CatalogChange{}
	e := s.DB.WithContext(ctx).Table("tokenhub_catalog_changes c").Select("c.*").Joins("JOIN tokenhub_catalog_change_audiences a ON a.change_id=c.id AND a.operator_id=?", operator).Where("c.resource_id IN ? AND (c.effective_at>? OR c.kind='emergency_model_disable' AND c.created_at>?)", []string{model, "*"}, at, at.Add(-24*time.Hour)).Order("c.effective_at,c.id").Limit(100).Find(&rows).Error
	if e != nil {
		return nil, e
	}
	out := []PublicCatalogChange{}
	for _, c := range rows {
		v := PublicCatalogChange{ID: c.ID, Kind: c.Kind, ModelID: model, EffectiveAt: c.EffectiveAt, Status: c.State}
		if c.State == "applied" && c.EffectiveAt.After(at) {
			v.Status = "scheduled"
		}
		if c.Kind == "price" && (c.State == "announcing" || c.State == "announced" || c.State == "applied") {
			var price PriceVersion
			if json.Unmarshal(c.Payload, &price) != nil {
				return nil, ErrInvalidInput
			}
			if price.Scope.Kind == "supplier" {
				continue
			}
			price.Published = true
			resolved, e := (PriceResolver{Source: plannedPriceSource{Base: s, Plan: &price}}).Resolve(ctx, PriceScope{Kind: "retail", SellerID: operator}, model, c.EffectiveAt)
			if e == nil {
				v.Prices, e = publicPriceComponents(resolved)
				if e != nil {
					return nil, e
				}
			}
		}
		out = append(out, v)
	}
	return out, nil
}
