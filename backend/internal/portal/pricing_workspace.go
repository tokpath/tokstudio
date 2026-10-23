package portal

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"github.com/tokpath/tokstudio/backend/internal/organization"
)

// The workspace shows the operator's own retail and direct supply books. It
// never exposes the parent's other prices, supplier costs or credentials.
func (a *Application) pricingBooks(ctx context.Context, operator string, at time.Time) ([]catalog.PriceVersion, error) {
	var rows []struct{ Definition json.RawMessage }
	e := a.DB.WithContext(ctx).Raw(`SELECT definition FROM tokenhub_model_price_versions
WHERE definition->'scope'->>'seller_id'=? AND (effective_at>? OR id IN (
 SELECT DISTINCT ON (scope_key,public_model_id,layer) id FROM tokenhub_model_price_versions
 WHERE definition->'scope'->>'seller_id'=? AND effective_at<=?
 ORDER BY scope_key,public_model_id,layer,effective_at DESC,id DESC))
ORDER BY effective_at DESC,id DESC`, operator, at, operator, at).Scan(&rows).Error
	if e != nil {
		return nil, e
	}
	out := []catalog.PriceVersion{}
	for _, row := range rows {
		var price catalog.PriceVersion
		if e := json.Unmarshal(row.Definition, &price); e != nil {
			return nil, e
		}
		if price.EndsAt != nil && !at.Before(*price.EndsAt) {
			continue
		}
		out = append(out, price)
	}
	return out, nil
}

func (a *Application) pricingWorkspaceItems(ctx context.Context, op organization.Operator, query string) ([]any, error) {
	now := time.Now().UTC()
	views, e := a.Catalog.PublicCatalog(ctx, op.ID, now)
	if e != nil {
		return nil, e
	}
	models, e := a.Catalog.PricingModels(ctx, op.ID)
	if e != nil {
		return nil, e
	}
	byModel := map[string]catalog.PublicCatalogModel{}
	for _, view := range views {
		byModel[view.ID] = view
	}
	books, e := a.pricingBooks(ctx, op.ID, now)
	if e != nil {
		return nil, e
	}
	changes, e := a.Catalog.CatalogChanges(ctx, op.ID)
	if e != nil {
		return nil, e
	}
	children, e := a.Organization.Children(ctx, nil, op.ID)
	if e != nil {
		return nil, e
	}
	names := map[string]string{}
	for _, child := range children {
		names[child.ID] = child.Name
	}
	items := []any{}
	query = strings.ToLower(strings.TrimSpace(query))
	for _, model := range models {
		if query != "" && !strings.Contains(strings.ToLower(model.ID), query) {
			continue
		}
		view, listed := byModel[model.ID]
		if !listed {
			view = catalog.PublicCatalogModel{ID: model.ID, Name: model.ID, Status: "not_listed"}
			if !model.Enabled {
				view.Status = "disabled"
			}
		}
		v := values(view)
		// Expose the pending measuring contract for price preparation. The
		// runtime still uses its accepted/current model snapshot until activation.
		prepared, e := a.preparedPricingModel(ctx, model)
		if e != nil {
			return nil, e
		}
		v["billing_pending"] = prepared.Version != model.Version
		v["billing"], v["pricing_components"] = prepared.Billing, prepared.BillingComponents()
		v["current_pricing_components"] = model.BillingComponents()
		v["model_version"] = model.Version
		v["capabilities"] = model.Capabilities
		entries, pending := []gin.H{}, []gin.H{}
		for _, price := range books {
			if price.ModelID != model.ID && price.ModelID != "*" {
				continue
			}
			name := "客户零售价"
			if price.Scope.Kind == "supply" {
				name = names[price.Scope.BuyerID]
				if name == "" {
					name = "直属下级"
				}
			}
			state := "active"
			if price.EffectiveAt.After(now) {
				state = "scheduled"
			}
			entries = append(entries, gin.H{"name": name, "status": state, "price": price})
		}
		for _, change := range changes {
			if change.ResourceID != model.ID && change.ResourceID != "*" {
				continue
			}
			if change.State != "announcing" && change.State != "announced" {
				continue
			}
			pending = append(pending, gin.H{"kind": change.Kind, "status": change.State, "effective_at": change.EffectiveAt, "needs_attention": change.ErrorCode != ""})
		}
		v["price_books"], v["pending_changes"] = entries, pending
		items = append(items, entity(model.ID, view.Status, int64(model.Version), v))
	}
	return items, nil
}

func (a *Application) preparedPricingModel(ctx context.Context, current catalog.PublicModel) (catalog.PublicModel, error) {
	var changes []catalog.CatalogChange
	if e := a.DB.WithContext(ctx).Where("operator_id=? AND kind='model' AND resource_id=? AND state IN ('announcing','announced')", a.Installation.OperatorID, current.ID).Order("created_at DESC,id DESC").Limit(1).Find(&changes).Error; e != nil {
		return current, e
	}
	if len(changes) == 0 {
		return current, nil
	}
	var pending catalog.PublicModel
	if e := json.Unmarshal(changes[0].Payload, &pending); e != nil {
		return current, e
	}
	if changes[0].ExpectedVersion != current.Version {
		return current, nil
	}
	pending.Version = current.Version + 1
	return pending, nil
}
