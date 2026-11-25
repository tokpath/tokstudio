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

func (s CommerceCatalog) prepareSubscriptionDrafts(ctx context.Context, actor string, pool CredentialView, discovered []string) error {
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
		ids = append(ids, priceID(model))
	}
	var prices []string
	db := s.DB.WithContext(ctx)
	if err := db.Table("model_pricing_rows").Distinct("model").
		Where("provider=? AND model IN ? AND mode IN ('chat','responses') AND COALESCE(definition->>'is_deprecated','false')='false'", provider, ids).
		Pluck("model", &prices).Error; err != nil {
		return err
	}
	priced := make(map[string]bool, len(prices))
	for _, model := range prices {
		priced[model] = true
	}
	var saved []string
	if err := db.Raw("SELECT definition->>'model' FROM tokenhub_supplier_models WHERE provider_id=?", pool.ID).Scan(&saved).Error; err != nil {
		return err
	}
	existing := make(map[string]bool, len(saved))
	for _, model := range saved {
		existing[model] = true
	}
	textOp := modelruntime.Operation(subscriptionauth.Operation(kind))
	for _, model := range discovered {
		if existing[model] || !priced[priceID(model)] {
			continue
		}
		draft := SupplierModel{
			ID: uuid.NewString(), CredentialID: pool.ID, Provider: subscriptionauth.Protocol(kind), Model: model,
			Capabilities: []modelruntime.Capability{{Operation: textOp, Streaming: true}},
		}
		err := s.saveSupplierModel(ctx, actor, draft, 0)
		if errors.Is(err, ErrSubscriptionModelLimits) || errors.Is(err, ErrInvalidInput) {
			continue
		}
		if err != nil {
			return err
		}
		existing[model] = true
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
