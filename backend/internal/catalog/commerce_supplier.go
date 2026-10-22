package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/modelruntime"
)

// SupplierModel is a provider connection and supported operations, with an
// optional Bifrost cost identity. Public ATG prices are managed independently.
type SupplierModel struct {
	BaseURL          string                    `json:"base_url,omitempty"`
	PricingReference string                    `json:"pricing_reference,omitempty"`
	CostProvider     string                    `json:"cost_provider"`
	CostModel        string                    `json:"cost_model"`
	ID               string                    `json:"id"`
	ProviderID       string                    `json:"provider_id"`
	CredentialID     string                    `json:"credential_id"`
	Provider         string                    `json:"provider"`
	Model            string                    `json:"model"`
	Capabilities     []modelruntime.Capability `json:"capabilities"`
	Enabled          bool                      `json:"enabled"`
	Version          uint64                    `json:"version"`
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

// A public model records the checked operations; its billing template is
// managed separately. Supplier models record protocol support and cost identity.
func publicCapabilityRecords(capabilities []modelruntime.Capability) []modelruntime.Capability {
	out := make([]modelruntime.Capability, len(capabilities))
	for i, capability := range capabilities {
		out[i] = modelruntime.Capability{Operation: capability.Operation}
	}
	return out
}
func validPublicCapabilities(capabilities []modelruntime.Capability) bool {
	for _, c := range capabilities {
		if !publicOperation(c.Operation) {
			return false
		}
	}
	// Listing the same operation more than once is harmless. Only the operation is
	// kept; streamed and plain calls are distinguished on the supplier model.
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
		if c.Operation == op {
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
	switch in.Provider {
	case "openai", "anthropic", "gemini", "openrouter", "elevenlabs":
	default:
		return ErrInvalidInput
	}
	if in.BaseURL != "" && in.Provider != "openai" && in.Provider != "anthropic" && in.Provider != "openrouter" {
		return ErrInvalidInput
	}
	in.CostProvider, in.CostModel = in.Provider, in.Model
	if in.PricingReference != "" {
		var reference struct{ Provider, Model string }
		if e := s.DB.WithContext(ctx).Table("model_pricing_rows").Select("provider,model").Where("id=?", in.PricingReference).Take(&reference).Error; e != nil {
			return ErrInvalidInput
		}
		in.CostProvider, in.CostModel = reference.Provider, reference.Model
	}
	if in.BaseURL != "" {
		u, e := url.Parse(in.BaseURL)
		if e != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "https" && u.Scheme != "http") {
			return ErrInvalidInput
		}
	}
	modes := []modelruntime.Capability{}
	seen := map[string]bool{}
	for _, c := range in.Capabilities {
		for _, stream := range []bool{false, true} {
			if stream && !c.Streaming {
				continue
			}
			key := string(c.Operation) + fmt.Sprint(stream)
			if !seen[key] {
				seen[key] = true
				modes = append(modes, modelruntime.Capability{Operation: c.Operation, Streaming: stream})
			}
		}
	}
	in.Capabilities = modes
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
	if len(in.Capabilities) == 0 {
		return false
	}
	seen := map[string]bool{}
	for _, c := range in.Capabilities {
		key := string(c.Operation) + fmt.Sprint(c.Streaming)
		if !publicOperation(c.Operation) || seen[key] {
			return false
		}
		seen[key] = true
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
	upstream.CostProvider = model.CostProvider
	upstream.CostModel = model.CostModel
	upstream.Identity = identity
	upstream.ProviderID = model.ProviderID
	upstream.Provider = model.Provider
	upstream.Model = model.Model
	upstream.CredentialID = model.CredentialID
	upstream.Endpoints = supplierEndpoints(model)
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
	return s.writeRoutingPolicy(ctx, RoutingPolicy{ModelID: publicModel.ID, Strategy: in.Strategy}, expected)
}

func supplierEndpoints(model SupplierModel) map[modelruntime.Operation]string {
	out := map[modelruntime.Operation]string{}
	if model.BaseURL == "" {
		return out
	}
	base := strings.TrimRight(model.BaseURL, "/")
	paths := map[modelruntime.Operation]string{modelruntime.Chat: "/chat/completions", modelruntime.Responses: "/responses", modelruntime.Embedding: "/embeddings", modelruntime.Image: "/images/generations", modelruntime.Speech: "/audio/speech", modelruntime.Transcription: "/audio/transcriptions", modelruntime.Video: "/videos", modelruntime.VideoRetrieve: "/videos/{task_id}", modelruntime.VideoDownload: "/videos/{task_id}/content"}
	if model.Provider == "anthropic" {
		paths[modelruntime.Chat] = "/messages"
		paths[modelruntime.Responses] = "/messages"
	}
	for op, path := range paths {
		out[op] = base + path
	}
	return out
}
