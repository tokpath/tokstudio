package catalog

import (
	"context"
	"encoding/json"
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
	var saved []catalogJSONRow
	if err := db.Table("tokenhub_supplier_models").Select("definition").
		Where("provider_id=? AND definition->>'model' IN ?", pool.ID, discovered).Order("id").Find(&saved).Error; err != nil {
		return err
	}
	existing := make(map[string]SupplierModel, len(saved))
	for _, row := range saved {
		var model SupplierModel
		if err := json.Unmarshal(row.Definition, &model); err != nil {
			return err
		}
		if existing[model.Model].ID == "" {
			existing[model.Model] = model
		}
	}
	textOp := modelruntime.Operation(subscriptionauth.Operation(kind))
	for _, model := range discovered {
		if !priced[priceID(model)] {
			continue
		}
		draft, exists := existing[model]
		if !exists {
			draft = SupplierModel{
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
			existing[model] = draft
		}
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
	if err != nil {
		return err
	}
	for _, route := range routes {
		if !route.Detached {
			return nil
		}
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
