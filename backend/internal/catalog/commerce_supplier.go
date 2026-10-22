package catalog

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/modelruntime"
)

// SupplierModel is one model a provider sells. Its metering and USD price are
// the platform's cost. Binding it to a public model happens on the route page.
type SupplierModel struct {
	ID           string                            `json:"id"`
	ProviderID   string                            `json:"provider_id"`
	CredentialID string                            `json:"credential_id"`
	Provider     string                            `json:"provider"`
	Model        string                            `json:"model"`
	Endpoints    map[modelruntime.Operation]string `json:"endpoints,omitempty"`
	Capabilities []modelruntime.Capability         `json:"capabilities"`
	Prices       []ComponentRate                   `json:"prices"`
	Enabled      bool                              `json:"enabled"`
	Version      uint64                            `json:"version"`
}

// RouteBinding attaches one supplier model to a public model.
type RouteBinding struct {
	ID              string      `json:"id"`
	SupplierModelID string      `json:"supplier_model_id"`
	Priority        int         `json:"priority"`
	Enabled         bool        `json:"enabled"`
	Routing         RouteLimits `json:"routing"`
}

// RouteConfiguration is the route page: several supplier models for one public
// model, plus the strategy used to order the ones that are currently healthy.
type RouteConfiguration struct {
	PublicModelID string         `json:"public_model_id"`
	Strategy      string         `json:"strategy"`
	Bindings      []RouteBinding `json:"bindings"`
}

func publicOperation(op modelruntime.Operation) bool {
	switch op {
	case modelruntime.Chat, modelruntime.Responses, modelruntime.Embedding, modelruntime.Image, modelruntime.Speech, modelruntime.Transcription, modelruntime.Video:
		return true
	default:
		return false
	}
}
func validPublicCapabilities(capabilities []modelruntime.Capability) bool {
	for _, c := range capabilities {
		if !publicOperation(c.Operation) {
			return false
		}
	}
	// The same operation may be stored twice, once for a streamed call and once
	// without streaming. The form only checks the operation; the upstream copy
	// still uses these records to decide which request shape it can carry.
	return true
}
func operationOffered(capabilities []modelruntime.Capability, r modelruntime.Request) bool {
	op := r.Operation
	if op == modelruntime.VideoRetrieve || op == modelruntime.VideoDownload {
		op = modelruntime.Video
	}
	for _, c := range capabilities {
		if c.Operation == op {
			return true
		}
	}
	return false
}
func upstreamOffers(capabilities []modelruntime.Capability, op modelruntime.Operation) bool {
	for _, c := range capabilities {
		if c.Verified() && c.Operation == op {
			return true
		}
	}
	return false
}

func (s CommerceCatalog) SupplierModel(ctx context.Context, id string) (SupplierModel, error) {
	var row catalogJSONRow
	var result SupplierModel
	e := s.DB.WithContext(ctx).Table("tokenhub_supplier_models").Where("id=?", id).Take(&row).Error
	if e == nil {
		e = json.Unmarshal(row.Definition, &result)
	}
	return result, e
}
func (s CommerceCatalog) AdminSupplierModels(ctx context.Context, actor string) ([]SupplierModel, error) {
	if !s.platform(actor) {
		return nil, ErrCatalogForbidden
	}
	var rows []catalogJSONRow
	if e := s.DB.WithContext(ctx).Table("tokenhub_supplier_models").Order("id").Find(&rows).Error; e != nil {
		return nil, e
	}
	out := make([]SupplierModel, 0, len(rows))
	for _, row := range rows {
		var model SupplierModel
		if e := json.Unmarshal(row.Definition, &model); e != nil {
			return nil, e
		}
		out = append(out, model)
	}
	return out, nil
}
func (s CommerceCatalog) SaveSupplierModel(ctx context.Context, actor string, in SupplierModel, expected uint64) error {
	return s.mutate(ctx, func(tx CommerceCatalog) error { return tx.saveSupplierModel(ctx, actor, in, expected) })
}
func (s CommerceCatalog) saveSupplierModel(ctx context.Context, actor string, in SupplierModel, expected uint64) error {
	if !s.platform(actor) {
		return ErrCatalogForbidden
	}
	var credential struct{ ProviderID string }
	if in.ID == "" || in.CredentialID == "" || in.Provider == "" || in.Model == "" {
		return ErrInvalidInput
	}
	if e := s.DB.WithContext(ctx).Table("tokenhub_provider_credentials").Select("provider_id").Where("id=?", in.CredentialID).Take(&credential).Error; e != nil {
		return ErrInvalidInput
	}
	in.ProviderID = credential.ProviderID
	if !supplierModelReady(in) {
		return ErrInvalidInput
	}
	in.Version = expected + 1
	if e := s.saveDefinition(ctx, "tokenhub_supplier_models", in.ID, in, expected, map[string]any{"provider_id": in.ProviderID}); e != nil {
		return e
	}
	return s.refreshSupplierBindings(ctx, actor, in)
}
func supplierModelReady(in SupplierModel) bool {
	if len(in.Capabilities) == 0 || len(in.Prices) == 0 {
		return false
	}
	priced := map[string]string{}
	for _, price := range in.Prices {
		if price.Key == "" || price.Unit == "" || priced[price.Key] != "" {
			return false
		}
		per, e := PriceDecimal(price.Per)
		rate, rateErr := PriceDecimal(price.Rate)
		if e != nil || rateErr != nil || per.Sign() <= 0 || rate.Sign() < 0 {
			return false
		}
		priced[price.Key] = price.Unit
	}
	for _, c := range in.Capabilities {
		if !c.Verified() {
			return false
		}
		if c.Operation == modelruntime.Video {
			for _, operation := range []modelruntime.Operation{modelruntime.VideoRetrieve, modelruntime.VideoDownload} {
				if _, e := modelruntime.ResolveTaskEndpoint(in.Endpoints[operation], "validation-task"); e != nil {
					return false
				}
			}
		}
		for _, rule := range c.UsageRules {
			if priced[rule.Component] != rule.Unit {
				return false
			}
		}
	}
	return true
}
func (s CommerceCatalog) refreshSupplierBindings(ctx context.Context, actor string, model SupplierModel) error {
	var rows []catalogJSONRow
	if e := s.DB.WithContext(ctx).Table("tokenhub_upstreams").Where("definition->>'supplier_model_id' = ?", model.ID).Find(&rows).Error; e != nil {
		return e
	}
	for _, row := range rows {
		var upstream UpstreamModel
		if e := json.Unmarshal(row.Definition, &upstream); e != nil {
			return e
		}
		publicModel, e := s.PublicModel(ctx, upstream.PublicModelID)
		if e != nil {
			return e
		}
		applySupplierModel(&upstream, model, publicModel.Identity)
		if !model.Enabled {
			upstream.Enabled = false
			upstream.Healthy = false
		}
		if e = s.saveUpstream(ctx, actor, upstream, row.Version); e != nil {
			return e
		}
	}
	return nil
}
func applySupplierModel(upstream *UpstreamModel, model SupplierModel, identity string) {
	upstream.Identity = identity
	upstream.ProviderID = model.ProviderID
	upstream.Provider = model.Provider
	upstream.Model = model.Model
	upstream.CredentialID = model.CredentialID
	upstream.Endpoints = model.Endpoints
	upstream.Capabilities = model.Capabilities
	upstream.SupplierModelID = model.ID
}
func (s CommerceCatalog) SaveRouteConfiguration(ctx context.Context, actor string, in RouteConfiguration, expected uint64) error {
	return s.mutate(ctx, func(tx CommerceCatalog) error { return tx.saveRouteConfiguration(ctx, actor, in, expected) })
}
func (s CommerceCatalog) saveRouteConfiguration(ctx context.Context, actor string, in RouteConfiguration, expected uint64) error {
	if !s.platform(actor) {
		return ErrCatalogForbidden
	}
	publicModel, e := s.PublicModel(ctx, in.PublicModelID)
	if e != nil {
		return e
	}
	if in.Strategy == "" {
		in.Strategy = "priority"
	}
	if !validRouteStrategy(in.Strategy) {
		return ErrInvalidInput
	}
	current, e := s.upstreams(ctx, in.PublicModelID)
	if e != nil {
		return e
	}
	byID := map[string]UpstreamModel{}
	bySupplier := map[string]UpstreamModel{}
	for _, upstream := range current {
		byID[upstream.ID] = upstream
		if upstream.SupplierModelID != "" {
			bySupplier[upstream.SupplierModelID] = upstream
		}
	}
	kept := map[string]bool{}
	seen := map[string]bool{}
	for _, binding := range in.Bindings {
		if binding.SupplierModelID == "" || seen[binding.SupplierModelID] || !binding.Routing.valid() {
			return ErrInvalidInput
		}
		seen[binding.SupplierModelID] = true
		supplierModel, e := s.SupplierModel(ctx, binding.SupplierModelID)
		if e != nil || (binding.Enabled && !supplierModel.Enabled) {
			return ErrInvalidInput
		}
		upstream, version := UpstreamModel{}, uint64(0)
		if binding.ID != "" {
			if previous, ok := byID[binding.ID]; ok {
				upstream, version = previous, previous.Version
			} else {
				upstream.ID = binding.ID
			}
		}
		if upstream.ID == "" {
			if previous, ok := bySupplier[binding.SupplierModelID]; ok {
				upstream, version = previous, previous.Version
			}
		}
		if upstream.ID == "" {
			upstream.ID = uuid.NewString()
		}
		applySupplierModel(&upstream, supplierModel, publicModel.Identity)
		upstream.PublicModelID = publicModel.ID
		upstream.Priority = binding.Priority
		upstream.Enabled = binding.Enabled
		upstream.Healthy = binding.Enabled
		upstream.Routing = binding.Routing
		if e = s.saveUpstream(ctx, actor, upstream, version); e != nil {
			return e
		}
		kept[upstream.ID] = true
	}
	for _, upstream := range current {
		if kept[upstream.ID] || (!upstream.Enabled && !upstream.Healthy) {
			continue
		}
		upstream.Enabled = false
		upstream.Healthy = false
		if e = s.saveUpstream(ctx, actor, upstream, upstream.Version); e != nil {
			return e
		}
	}
	return s.writeRoutingPolicy(ctx, RoutingPolicy{ModelID: publicModel.ID, Strategy: in.Strategy, Evidence: "路由配置"}, expected)
}
func supplierModelPrice(model SupplierModel, upstreamID, publicModel string, at time.Time) (ResolvedPrice, error) {
	components := append([]ComponentRate(nil), model.Prices...)
	for i := range components {
		components[i].Tiers = append([]PriceTier(nil), components[i].Tiers...)
	}
	return ResolvedPrice{
		AssetUnit:         "USD",
		Scope:             PriceScope{Kind: "supplier", SellerID: model.ProviderID, BuyerID: upstreamID},
		ModelID:           publicModel,
		AcceptedAt:        at,
		Components:        components,
		FactorNumerator:   "1",
		FactorDenominator: "1",
		Lineage:           []PriceLineage{{VersionID: model.ID, Scope: PriceScope{Kind: "supplier", SellerID: model.ProviderID, BuyerID: model.ID}, Layer: "regular"}},
	}, nil
}
