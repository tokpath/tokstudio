package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/platform/crypto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrDiscoveryUnavailable = errors.New("upstream model discovery unavailable")
var ErrProviderModelUnpriced = errors.New("upstream model has no configured cost")

type providerModelRow struct {
	ProviderID      string     `gorm:"column:provider_id;primaryKey"`
	UpstreamModelID string     `gorm:"column:upstream_model_id;primaryKey"`
	DisplayName     string     `gorm:"column:display_name"`
	UnitCosts       []byte     `gorm:"column:unit_costs_json"`
	PriceSource     string     `gorm:"column:price_source"`
	Status          string     `gorm:"column:status;default:active"`
	DiscoveredAt    *time.Time `gorm:"column:discovered_at"`
	UpdatedAt       time.Time  `gorm:"column:updated_at"`
}

func (providerModelRow) TableName() string { return "catalog_provider_models" }

type ProviderModelView struct {
	ProviderID      string            `json:"provider_id"`
	UpstreamModelID string            `json:"upstream_model_id"`
	DisplayName     string            `json:"display_name"`
	UnitCosts       map[string]string `json:"unit_costs"`
	PriceSource     string            `json:"price_source"`
	Status          string            `json:"status"`
	DiscoveredAt    *time.Time        `json:"discovered_at,omitempty"`
}

type ProviderModelInput struct {
	UpstreamModelID string            `json:"upstream_model_id"`
	DisplayName     string            `json:"display_name"`
	UnitCosts       map[string]string `json:"unit_costs"`
}

var costKeys = []string{"input", "output", "image_count", "video_second", "audio_second"}

func validateUnitCosts(in map[string]string) (map[string]string, error) {
	out := map[string]string{}
	for _, key := range costKeys {
		value := strings.TrimSpace(in[key])
		if value == "" {
			continue
		}
		n, err := strconv.ParseFloat(value, 64)
		if err != nil || math.IsNaN(n) || math.IsInf(n, 0) || n < 0 || n > 1_000_000 {
			return nil, ErrInvalidInput
		}
		out[key] = value
	}
	return out, nil
}

func decodeCosts(raw []byte) map[string]string {
	out := map[string]string{}
	_ = json.Unmarshal(raw, &out)
	return out
}

func providerModelView(row providerModelRow) ProviderModelView {
	return ProviderModelView{
		ProviderID: row.ProviderID, UpstreamModelID: row.UpstreamModelID,
		DisplayName: row.DisplayName, UnitCosts: decodeCosts(row.UnitCosts),
		PriceSource: row.PriceSource, Status: row.Status, DiscoveredAt: row.DiscoveredAt,
	}
}

func (s *Service) ListProviderModels(ctx context.Context, providerID string) ([]ProviderModelView, error) {
	var provider providerRow
	if err := s.db.WithContext(ctx).Where("id = ? OR slug = ?", providerID, providerID).First(&provider).Error; err != nil {
		return nil, err
	}
	var rows []providerModelRow
	if err := s.db.WithContext(ctx).Where("provider_id = ?", provider.ID).Order("upstream_model_id").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]ProviderModelView, 0, len(rows))
	for _, row := range rows {
		out = append(out, providerModelView(row))
	}
	return out, nil
}

func (s *Service) SaveProviderModel(ctx context.Context, providerID string, in ProviderModelInput) (*ProviderModelView, error) {
	var provider providerRow
	if err := s.db.WithContext(ctx).Where("id = ? OR slug = ?", providerID, providerID).First(&provider).Error; err != nil {
		return nil, err
	}
	in.UpstreamModelID = strings.TrimSpace(in.UpstreamModelID)
	if in.UpstreamModelID == "" || len(in.UpstreamModelID) > 300 {
		return nil, ErrInvalidInput
	}
	costs, err := validateUnitCosts(in.UnitCosts)
	if err != nil || len(costs) == 0 {
		return nil, ErrInvalidInput
	}
	encoded, _ := json.Marshal(costs)
	now := time.Now().UTC()
	requestedName := strings.TrimSpace(in.DisplayName)
	displayName := requestedName
	if displayName == "" {
		displayName = in.UpstreamModelID
	}
	row := providerModelRow{ProviderID: provider.ID, UpstreamModelID: in.UpstreamModelID,
		DisplayName: displayName, UnitCosts: encoded, PriceSource: "manual", Status: "active", UpdatedAt: now}
	updates := map[string]any{"unit_costs_json": encoded, "price_source": "manual", "updated_at": now}
	if requestedName != "" {
		updates["display_name"] = requestedName
	}
	if err := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "provider_id"}, {Name: "upstream_model_id"}},
		DoUpdates: clause.Assignments(updates),
	}).Create(&row).Error; err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Where("provider_id = ? AND upstream_model_id = ?", provider.ID, in.UpstreamModelID).First(&row).Error; err != nil {
		return nil, err
	}
	view := providerModelView(row)
	return &view, nil
}

func (s *Service) SetProviderModelEnabled(ctx context.Context, providerID, upstreamID string, enabled bool) (*ProviderModelView, error) {
	var provider providerRow
	if err := s.db.WithContext(ctx).Where("id = ? OR slug = ?", providerID, providerID).First(&provider).Error; err != nil {
		return nil, err
	}
	upstreamID = strings.TrimSpace(upstreamID)
	if upstreamID == "" {
		return nil, ErrInvalidInput
	}
	var row providerModelRow
	if err := s.db.WithContext(ctx).Where("provider_id = ? AND upstream_model_id = ?", provider.ID, upstreamID).First(&row).Error; err != nil {
		return nil, err
	}
	status := "inactive"
	if enabled {
		status = "active"
	}
	if err := s.db.WithContext(ctx).Model(&row).Updates(map[string]any{"status": status, "updated_at": time.Now().UTC()}).Error; err != nil {
		return nil, err
	}
	row.Status = status
	view := providerModelView(row)
	return &view, nil
}

func pricedForKind(costs map[string]string, kind string) bool {
	has := func(key string) bool { return strings.TrimSpace(costs[key]) != "" }
	switch kind {
	case "text":
		return has("input") && has("output")
	case "embedding":
		return has("input")
	case "image":
		return has("image_count")
	case "video":
		return has("video_second")
	case "audio":
		return has("audio_second")
	default:
		return has("input") && has("output")
	}
}

func modelKind(row publicModelRow) string {
	var caps map[string]any
	_ = json.Unmarshal(row.Capabilities, &caps)
	if kind, ok := caps["kind"].(string); ok {
		return kind
	}
	return "text"
}

func (s *Service) PricedProviderModel(ctx context.Context, providerID, upstreamID, kind string) (json.RawMessage, error) {
	return pricedProviderModelTx(s.db.WithContext(ctx), providerID, upstreamID, kind)
}

func pricedProviderModelTx(tx *gorm.DB, providerID, upstreamID, kind string) (json.RawMessage, error) {
	var row providerModelRow
	if err := tx.Where("provider_id = ? AND upstream_model_id = ? AND status = 'active'", providerID, upstreamID).First(&row).Error; err == nil {
		if pricedForKind(decodeCosts(row.UnitCosts), kind) {
			return row.UnitCosts, nil
		}
	}
	return nil, ErrProviderModelUnpriced
}

func (s *Service) DiscoverProviderModels(ctx context.Context, providerID, encKey string) ([]ProviderModelView, error) {
	var provider providerRow
	if err := s.db.WithContext(ctx).Where("id = ? OR slug = ?", providerID, providerID).First(&provider).Error; err != nil {
		return nil, err
	}
	base := strings.TrimRight(strings.TrimSpace(provider.BaseURL), "/")
	if base == "" || ValidateUpstreamURL(base, s.production, s.allowHosts) != nil {
		return nil, ErrDiscoveryUnavailable
	}
	switch strings.ToLower(provider.Adapter) {
	case "openai", "openrouter", "anthropic", "gemini", "ark":
	default:
		return nil, ErrDiscoveryUnavailable
	}
	var accounts []accountRow
	if err := s.db.WithContext(ctx).Where("provider_id = ?", provider.ID).Order("created_at").Find(&accounts).Error; err != nil {
		return nil, err
	}
	secret := ""
	for _, account := range accounts {
		if accountUsable(account, time.Now().UTC(), "") {
			secret, _ = crypto.Open(encKey, account.Ciphertext)
			if secret != "" {
				break
			}
		}
	}
	if secret == "" && provider.Adapter != "openrouter" {
		return nil, fmt.Errorf("%w: 请先添加可用上游账号", ErrDiscoveryUnavailable)
	}
	client := &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	type modelsPage struct {
		Data []struct {
			ID          string            `json:"id"`
			Name        string            `json:"name"`
			DisplayName string            `json:"display_name"`
			Pricing     map[string]string `json:"pricing"`
		} `json:"data"`
		Models []struct {
			Name        string `json:"name"`
			BaseModelID string `json:"baseModelId"`
			DisplayName string `json:"displayName"`
		} `json:"models"`
		NextPageToken string `json:"nextPageToken"`
		HasMore       bool   `json:"has_more"`
		LastID        string `json:"last_id"`
	}
	now := time.Now().UTC()
	rows := make([]providerModelRow, 0)
	cursor := ""
	for page := 0; page < 20; page++ {
		endpoint := base + "/models"
		if cursor != "" {
			query := url.Values{}
			if provider.Adapter == "gemini" {
				query.Set("pageToken", cursor)
			} else {
				query.Set("after", cursor)
			}
			endpoint += "?" + query.Encode()
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}
		if secret != "" {
			if provider.Adapter == "anthropic" {
				req.Header.Set("x-api-key", secret)
				req.Header.Set("anthropic-version", "2023-06-01")
			} else if provider.Adapter == "gemini" && !strings.Contains(base, "/openai") {
				req.Header.Set("x-goog-api-key", secret)
			} else {
				req.Header.Set("Authorization", "Bearer "+secret)
			}
		}
		response, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrDiscoveryUnavailable, err)
		}
		if response.StatusCode != http.StatusOK {
			response.Body.Close()
			return nil, fmt.Errorf("%w: 上游模型接口返回 %d", ErrDiscoveryUnavailable, response.StatusCode)
		}
		var body modelsPage
		err = json.NewDecoder(io.LimitReader(response.Body, 8<<20)).Decode(&body)
		response.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("%w: 模型列表无效", ErrDiscoveryUnavailable)
		}
		for _, item := range body.Data {
			if strings.TrimSpace(item.ID) == "" {
				continue
			}
			costs := map[string]string{}
			if provider.Adapter == "openrouter" {
				for source, target := range map[string]string{"prompt": "input", "completion": "output", "image": "image_count"} {
					if value := strings.TrimSpace(item.Pricing[source]); value != "" {
						costs[target] = value
					}
				}
			}
			costs, err = validateUnitCosts(costs)
			if err != nil {
				costs = map[string]string{}
			}
			encoded, _ := json.Marshal(costs)
			name := item.DisplayName
			if name == "" {
				name = item.Name
			}
			rows = append(rows, providerModelRow{ProviderID: provider.ID, UpstreamModelID: item.ID, DisplayName: name,
				UnitCosts: encoded, PriceSource: map[bool]string{true: "detected", false: "unknown"}[len(costs) > 0], Status: "active", DiscoveredAt: &now, UpdatedAt: now})
		}
		for _, item := range body.Models {
			id := item.BaseModelID
			if id == "" {
				id = strings.TrimPrefix(item.Name, "models/")
			}
			if id == "" {
				continue
			}
			rows = append(rows, providerModelRow{ProviderID: provider.ID, UpstreamModelID: id, DisplayName: item.DisplayName,
				UnitCosts: []byte(`{}`), PriceSource: "unknown", Status: "active", DiscoveredAt: &now, UpdatedAt: now})
		}
		next := body.NextPageToken
		if provider.Adapter == "anthropic" && body.HasMore {
			next = body.LastID
		}
		if next == "" {
			break
		}
		if next == cursor || page == 19 {
			return nil, fmt.Errorf("%w: 上游分页未结束", ErrDiscoveryUnavailable)
		}
		cursor = next
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("%w: 上游未返回模型", ErrDiscoveryUnavailable)
	}
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, row := range rows {
			var existing providerModelRow
			err := tx.Where("provider_id = ? AND upstream_model_id = ?", row.ProviderID, row.UpstreamModelID).First(&existing).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				if err := tx.Create(&row).Error; err != nil {
					return err
				}
			} else if err != nil {
				return err
			} else {
				changes := map[string]any{"discovered_at": now, "updated_at": now}
				if row.DisplayName != "" {
					changes["display_name"] = row.DisplayName
				}
				if existing.PriceSource != "manual" {
					changes["unit_costs_json"] = row.UnitCosts
					changes["price_source"] = row.PriceSource
				}
				if err := tx.Model(&existing).Updates(changes).Error; err != nil {
					return err
				}
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return s.ListProviderModels(ctx, provider.ID)
}
