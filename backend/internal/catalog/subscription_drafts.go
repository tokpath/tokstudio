package catalog

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/modelruntime"
	"github.com/tokpath/tokstudio/backend/internal/subscriptionauth"
	"gorm.io/gorm"
)

func (s CommerceCatalog) prepareSubscriptionDrafts(ctx context.Context, actor string, pool CredentialView, discovered []subscriptionauth.DiscoveredModel) error {
	if len(discovered) == 0 {
		return nil
	}
	kind := pool.SubscriptionKind
	provider := subscriptionauth.PricingProvider(kind)
	priceID := func(id string) string {
		if provider == "gemini" {
			return strings.TrimPrefix(id, "google/")
		}
		return id
	}
	ids := make([]string, 0, len(discovered))
	for _, model := range discovered {
		ids = append(ids, priceID(model.ID))
	}
	var prices []struct{ Model, Mode string }
	db := s.DB.WithContext(ctx)
	if err := db.Table("model_pricing_rows").Distinct("model", "mode").
		Where("provider=? AND model IN ? AND COALESCE(definition->>'is_deprecated','false')='false'", provider, ids).
		Find(&prices).Error; err != nil {
		return err
	}
	modes := make(map[string][]string)
	for _, row := range prices {
		modes[row.Model] = append(modes[row.Model], row.Mode)
	}
	var saved []string
	if err := db.Raw("SELECT definition->>'model' FROM tokenhub_supplier_models WHERE provider_id=?", pool.ID).Scan(&saved).Error; err != nil {
		return err
	}
	existing := make(map[string]bool, len(saved))
	for _, model := range saved {
		existing[model] = true
	}
	connection := s.SubscriptionConnections[kind]
	textOp := modelruntime.Operation(subscriptionauth.Operation(kind))
	for _, model := range discovered {
		if existing[model.ID] {
			continue
		}
		hasText := false
		media := make(map[string]bool)
		for _, mode := range modes[priceID(model.ID)] {
			if mode == "chat" || mode == "responses" {
				hasText = true
			} else {
				media[mode] = true
			}
		}
		for _, op := range model.Capabilities {
			media[op] = true
		}
		// GPT Image models advertised for generation also expose editing.
		if kind == subscriptionauth.Codex && strings.HasPrefix(model.ID, "gpt-image-") && media[subscriptionauth.ImageGeneration] {
			media[subscriptionauth.ImageEdit] = true
		}
		draft := SupplierModel{
			ID: uuid.NewString(), CredentialID: pool.ID, Provider: subscriptionauth.Protocol(kind), Model: model.ID,
		}
		if hasText {
			draft.Capabilities = append(draft.Capabilities, modelruntime.Capability{Operation: textOp, Streaming: true})
		}
		for _, op := range []string{subscriptionauth.ImageGeneration, subscriptionauth.ImageEdit, subscriptionauth.VideoGeneration} {
			if endpoint, ok := connection.MediaEndpoint(op); ok && media[op] {
				draft.Capabilities = append(draft.Capabilities, modelruntime.Capability{Operation: modelruntime.Operation(op), Streaming: endpoint.Streaming})
			}
		}
		if len(draft.Capabilities) == 0 {
			continue
		}
		err := s.saveSupplierModel(ctx, actor, draft, 0)
		if errors.Is(err, ErrSubscriptionModelLimits) && len(draft.Capabilities) > 1 {
			draft.Capabilities = draft.Capabilities[1:]
			err = s.saveSupplierModel(ctx, actor, draft, 0)
		}
		if errors.Is(err, ErrSubscriptionModelLimits) || errors.Is(err, ErrInvalidInput) {
			continue
		}
		if err != nil {
			return err
		}
		existing[model.ID] = true
		if err := s.prepareInitialSubscriptionRoute(ctx, actor, draft); err != nil {
			return err
		}
	}
	return nil
}

func (s CommerceCatalog) prepareInitialSubscriptionRoute(ctx context.Context, actor string, draft SupplierModel) error {
	model, err := s.PublicModel(ctx, draft.Model)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if model.Identity != draft.Model || !SupplierSupportsModel(draft, model) {
		return nil
	}
	routes, err := s.upstreams(ctx, model.ID)
	if err != nil || len(routes) != 0 {
		return err
	}
	policy, err := s.RoutingPolicy(ctx, model.ID)
	if err != nil {
		return err
	}
	return s.saveRouteConfiguration(ctx, actor, RouteConfiguration{
		PublicModelID: model.ID, Strategy: policy.Strategy,
		Bindings: []RouteBinding{{SupplierModelID: draft.ID}},
	}, policy.Version)
}
