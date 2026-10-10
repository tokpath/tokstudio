package catalog

import (
	"context"
	"errors"
	"gorm.io/gorm"
	"time"
)

// ModelServiceReadiness is a management-only projection. Public discovery never
// attaches it: upstream IDs, routes and account state belong to the platform.
type ModelServiceReadiness struct {
	ConfigurationReady bool                `json:"configuration_ready"`
	Callable           bool                `json:"callable"`
	RuntimeState       string              `json:"runtime_state"`
	RouteIDs           []string            `json:"route_ids"`
	Missing            []string            `json:"missing"`
	Providers          []ProviderReadiness `json:"providers"`
}
type ProviderReadiness struct {
	ProviderID      string     `json:"provider_id"`
	RouteID         string     `json:"route_id"`
	UpstreamModelID string     `json:"upstream_model_id"`
	Missing         []string   `json:"missing"`
	RuntimeState    string     `json:"runtime_state"`
	CheckedAt       *time.Time `json:"checked_at,omitempty"`
}

func (s *Service) ModelReadiness(ctx context.Context, publicID string) (*ModelServiceReadiness, error) {
	model, err := s.loadModel(ctx, publicID)
	if err != nil {
		return nil, err
	}
	view := &ModelServiceReadiness{RuntimeState: "unknown", RouteIDs: []string{}, Missing: []string{}, Providers: []ProviderReadiness{}}
	configurationErr := s.validateModelReady(ctx, *model)
	if configurationErr != nil && !errors.Is(configurationErr, ErrModelIncomplete) {
		return nil, configurationErr
	}
	view.ConfigurationReady = configurationErr == nil
	if !view.ConfigurationReady {
		view.Missing = append(view.Missing, "model_or_price")
	}
	if model.Status != SyncPublished {
		view.Missing = append(view.Missing, "publish")
	}
	var routes []routeGroupRow
	if err := s.db.WithContext(ctx).Where("public_model_id = ?", model.ID).Order("id").Find(&routes).Error; err != nil {
		return nil, err
	}
	viable, healthy, degraded := 0, 0, 0
	now := time.Now().UTC()
	for _, route := range routes {
		view.RouteIDs = append(view.RouteIDs, route.ID)
		if route.Status != "active" {
			continue
		}
		var candidates []candidateRow
		if err := s.db.WithContext(ctx).Where("route_group_id = ?", route.ID).Order("priority, provider_id").Find(&candidates).Error; err != nil {
			return nil, err
		}
		for _, candidate := range candidates {
			item := ProviderReadiness{ProviderID: candidate.ProviderID, RouteID: route.ID, Missing: []string{}, RuntimeState: "unknown"}
			var provider providerRow
			if err := s.db.WithContext(ctx).Where("id = ?", candidate.ProviderID).First(&provider).Error; err != nil {
				return nil, err
			}
			item.CheckedAt = provider.HealthCheckedAt
			if provider.Status != "active" {
				item.Missing = append(item.Missing, "provider_disabled")
			}
			var mapping mappingRow
			err := s.db.WithContext(ctx).Where("public_model_id = ? AND provider_id = ? AND status = ?", model.ID, provider.ID, "active").First(&mapping).Error
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, err
			}
			if err != nil || mapping.UpstreamModelID == "" {
				item.Missing = append(item.Missing, "mapping")
			} else {
				item.UpstreamModelID = mapping.UpstreamModelID
				if _, err := s.PricedProviderModel(ctx, provider.ID, mapping.UpstreamModelID, modelKind(*model)); err != nil {
					if !errors.Is(err, ErrProviderModelUnpriced) && !errors.Is(err, gorm.ErrRecordNotFound) && !errors.Is(err, ErrInvalidInput) {
						return nil, err
					}
					item.Missing = append(item.Missing, "upstream_cost")
				}
			}
			var accounts []accountRow
			if err := s.db.WithContext(ctx).Where("provider_id = ?", provider.ID).Find(&accounts).Error; err != nil {
				return nil, err
			}
			usable := provider.Adapter == "test"
			for _, account := range accounts {
				if accountUsable(account, now, publicID) {
					usable = true
				}
			}
			if !usable {
				item.Missing = append(item.Missing, "account")
			}
			if provider.Health == "unavailable" || provider.Health == "maintenance" {
				item.RuntimeState = "unavailable"
			} else if provider.HealthCheckedAt != nil && now.Sub(*provider.HealthCheckedAt) <= 24*time.Hour {
				if provider.Health == "available" {
					item.RuntimeState = "healthy"
				} else if provider.Health == "degraded" {
					item.RuntimeState = "degraded"
				}
			}
			if len(item.Missing) == 0 {
				viable++
				if item.RuntimeState == "healthy" {
					healthy++
				}
				if item.RuntimeState == "degraded" {
					degraded++
				}
			}
			view.Providers = append(view.Providers, item)
		}
	}
	if len(view.RouteIDs) == 0 {
		view.Missing = append(view.Missing, "route")
	}
	if viable == 0 && len(view.RouteIDs) > 0 {
		view.Missing = append(view.Missing, "route_candidates")
	}
	view.Callable = view.ConfigurationReady && model.Status == SyncPublished && viable > 0
	if !view.Callable {
		view.RuntimeState = "not_configured"
	} else if healthy > 0 {
		view.RuntimeState = "healthy"
	} else if degraded > 0 {
		view.RuntimeState = "degraded"
	} else {
		allDown := true
		for _, item := range view.Providers {
			if len(item.Missing) == 0 && item.RuntimeState != "unavailable" {
				allDown = false
			}
		}
		if allDown {
			view.RuntimeState = "unavailable"
			view.Callable = false
		}
	}
	return view, nil
}

// ProviderHealthSummary never treats defaults, empty pools, or stale checks as healthy.
func (s *Service) ProviderHealthSummary(ctx context.Context) (map[string]any, error) {
	var providers []providerRow
	if err := s.db.WithContext(ctx).Where("status = ?", "active").Find(&providers).Error; err != nil {
		return nil, err
	}
	known, problems := 0, 0
	now := time.Now().UTC()
	for _, provider := range providers {
		if provider.HealthCheckedAt != nil && now.Sub(*provider.HealthCheckedAt) <= 24*time.Hour {
			known++
			if provider.Health != "available" {
				problems++
			}
		}
	}
	state := "unknown"
	if len(providers) == 0 {
		state = "not_configured"
	} else if problems > 0 {
		state = "degraded"
	} else if known == len(providers) {
		state = "healthy"
	}
	return map[string]any{"state": state, "total": len(providers), "checked": known, "problem_count": problems}, nil
}
