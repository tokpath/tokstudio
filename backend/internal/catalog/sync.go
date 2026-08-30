package catalog

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/platform/id"
)

const (
	SyncDraft     = "draft"
	SyncReviewed  = "reviewed"
	SyncRejected  = "rejected"
	SyncPublished = "published"
)

type DiscoveredModel struct {
	PublicID     string
	UpstreamID   string
	Vendor       string
	DisplayName  string
	Capabilities map[string]any
	UnitPrices   map[string]any
}

type SyncResult struct {
	ProviderID string      `json:"provider_id"`
	Items      []ModelView `json:"items"`
}

func sandboxDiscovery(provider providerRow) []DiscoveredModel {
	slug := strings.TrimSpace(provider.Slug)
	if slug == "" {
		slug = provider.ID
	}
	caps := map[string]any{
		"supported_parameters": []string{"stream", "messages", "model", "tools", "response_format"},
		"source":               "provider_sync",
	}
	prices := map[string]any{
		"input": "0.000001", "output": "0.000002", "currency": "USD",
		"upstream_cost_input": "0.0000003", "upstream_cost_output": "0.0000006",
	}
	return []DiscoveredModel{{
		PublicID: "tokenhub/sync-" + slug, UpstreamID: "sandbox-" + slug,
		Vendor: "tokenhub", DisplayName: "Synced " + provider.Name,
		Capabilities: caps, UnitPrices: prices,
	}}
}

func (s *Service) SyncProvider(ctx context.Context, providerID string) (*SyncResult, error) {
	var provider providerRow
	if err := s.db.WithContext(ctx).Where("id = ? OR slug = ?", providerID, providerID).First(&provider).Error; err != nil {
		return nil, err
	}
	out := &SyncResult{ProviderID: provider.ID}
	for _, found := range sandboxDiscovery(provider) {
		view, err := s.upsertDraft(ctx, provider, found)
		if err != nil {
			return nil, err
		}
		out.Items = append(out.Items, *view)
	}
	return out, nil
}

func (s *Service) upsertDraft(ctx context.Context, provider providerRow, found DiscoveredModel) (*ModelView, error) {
	caps, _ := json.Marshal(found.Capabilities)
	prices, _ := json.Marshal(found.UnitPrices)
	var model publicModelRow
	err := s.db.WithContext(ctx).Where("public_id = ?", found.PublicID).First(&model).Error
	if err != nil {
		model = publicModelRow{
			ID: id.New("mdl"), PublicID: found.PublicID, Vendor: found.Vendor,
			DisplayName: found.DisplayName, Capabilities: caps, Status: SyncDraft,
		}
		if err := s.db.WithContext(ctx).Create(&model).Error; err != nil {
			return nil, err
		}
	} else if model.Status != "published" && model.Status != "deprecated" {
		_ = s.db.WithContext(ctx).Model(&publicModelRow{}).Where("id = ?", model.ID).Updates(map[string]any{
			"display_name": found.DisplayName, "vendor": found.Vendor, "capabilities_json": caps, "status": SyncDraft,
		}).Error
		model.Status = SyncDraft
	}

	var mapping mappingRow
	if err := s.db.WithContext(ctx).Where("public_model_id = ? AND provider_id = ?", model.ID, provider.ID).First(&mapping).Error; err != nil {
		mapping = mappingRow{
			ID: id.New("map"), PublicModelID: model.ID, ProviderID: provider.ID,
			UpstreamModelID: found.UpstreamID, Status: "active", SyncState: SyncDraft,
		}
		if err := s.db.WithContext(ctx).Create(&mapping).Error; err != nil {
			return nil, err
		}
	} else {
		_ = s.db.WithContext(ctx).Model(&mappingRow{}).Where("id = ?", mapping.ID).Updates(map[string]any{
			"upstream_model_id": found.UpstreamID, "sync_state": SyncDraft,
		}).Error
	}

	price := priceRow{
		ID: id.New("prc"), PublicModelID: model.ID, ProviderID: &provider.ID,
		UnitPrices: prices, Status: SyncDraft, EffectiveAt: time.Now().UTC(),
	}
	if err := s.db.WithContext(ctx).Create(&price).Error; err != nil {
		return nil, err
	}
	return s.modelView(ctx, model)
}

func (s *Service) ReviewModel(ctx context.Context, publicID, action string) (*ModelView, error) {
	model, err := s.loadModel(ctx, publicID)
	if err != nil {
		return nil, err
	}
	state := SyncReviewed
	if strings.EqualFold(strings.TrimSpace(action), "reject") {
		state = SyncRejected
	}
	if err := s.db.WithContext(ctx).Model(&mappingRow{}).Where("public_model_id = ?", model.ID).
		Update("sync_state", state).Error; err != nil {
		return nil, err
	}
	return s.modelView(ctx, *model)
}

func (s *Service) PublishModel(ctx context.Context, publicID string) (*ModelView, error) {
	model, err := s.loadModel(ctx, publicID)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	if err := s.db.WithContext(ctx).Model(&publicModelRow{}).Where("id = ?", model.ID).
		Update("status", SyncPublished).Error; err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Model(&mappingRow{}).Where("public_model_id = ?", model.ID).
		Updates(map[string]any{"status": "active", "sync_state": SyncPublished}).Error; err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Model(&priceRow{}).
		Where("public_model_id = ? AND provider_id IS NULL AND status = ?", model.ID, SyncPublished).
		Update("status", "superseded").Error; err != nil {
		return nil, err
	}
	var draft priceRow
	if err := s.db.WithContext(ctx).Where("public_model_id = ? AND status = ?", model.ID, SyncDraft).
		Order("effective_at DESC").First(&draft).Error; err == nil {
		platform := priceRow{
			ID: id.New("prc"), PublicModelID: model.ID, UnitPrices: draft.UnitPrices,
			Status: SyncPublished, EffectiveAt: now,
		}
		if err := s.db.WithContext(ctx).Create(&platform).Error; err != nil {
			return nil, err
		}
		_ = s.db.WithContext(ctx).Model(&priceRow{}).Where("id = ?", draft.ID).Update("status", SyncPublished).Error
	}
	var group routeGroupRow
	if err := s.db.WithContext(ctx).Where("public_model_id = ?", model.ID).First(&group).Error; err != nil {
		group = routeGroupRow{ID: id.New("rg"), PublicModelID: model.ID, Strategy: StrategyPriority, Status: "active"}
		if err := s.db.WithContext(ctx).Create(&group).Error; err != nil {
			return nil, err
		}
	}
	var mappings []mappingRow
	_ = s.db.WithContext(ctx).Where("public_model_id = ?", model.ID).Find(&mappings).Error
	for _, mapping := range mappings {
		_ = s.db.WithContext(ctx).Where("route_group_id = ? AND provider_id = ?", group.ID, mapping.ProviderID).
			FirstOrCreate(&candidateRow{RouteGroupID: group.ID, ProviderID: mapping.ProviderID, Priority: 1, Weight: 1}).Error
	}
	for _, channelID := range []string{identity.OfficialChannelID, identity.ResellerChannelID, identity.OEMChannelID} {
		_ = s.db.WithContext(ctx).Where("channel_org_id = ? AND public_model_id = ?", channelID, model.ID).
			FirstOrCreate(&channelPolicyRow{ChannelOrgID: channelID, PublicModelID: model.ID, Enabled: true}).Error
	}
	model.Status = SyncPublished
	return s.modelView(ctx, *model)
}

func (s *Service) DeprecateModel(ctx context.Context, publicID string) (*ModelView, error) {
	model, err := s.loadModel(ctx, publicID)
	if err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Model(&publicModelRow{}).Where("id = ?", model.ID).
		Update("status", "deprecated").Error; err != nil {
		return nil, err
	}
	var prices int64
	_ = s.db.WithContext(ctx).Model(&priceRow{}).Where("public_model_id = ?", model.ID).Count(&prices).Error
	var maps int64
	_ = s.db.WithContext(ctx).Model(&mappingRow{}).Where("public_model_id = ?", model.ID).Count(&maps).Error
	if prices == 0 || maps == 0 {
		return nil, ErrInvalidInput
	}
	model.Status = "deprecated"
	return s.modelView(ctx, *model)
}

func (s *Service) loadModel(ctx context.Context, publicID string) (*publicModelRow, error) {
	var model publicModelRow
	if err := s.db.WithContext(ctx).Where("id = ? OR public_id = ?", publicID, publicID).First(&model).Error; err != nil {
		return nil, err
	}
	return &model, nil
}
