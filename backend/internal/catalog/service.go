package catalog

import (
	"context"
	"embed"
	"encoding/json"
	"io/fs"
	"strings"

	"gorm.io/gorm"

	"github.com/tokpath/tokstudio/backend/internal/identity"
)

//go:embed migrate/*.sql
var migrationFS embed.FS

const (
	EchoModelID     = "tokenhub/echo-1"
	OEMModelID      = "tokenhub/oem-demo"
	PrimaryProvider = "echo-primary"
	BackupProvider  = "echo-backup"
)

type providerRow struct {
	ID           string `gorm:"column:id;primaryKey"`
	Name         string `gorm:"column:name"`
	Slug         string `gorm:"column:slug"`
	Kind         string `gorm:"column:kind"`
	Adapter      string `gorm:"column:adapter"`
	BaseURL      string `gorm:"column:base_url"`
	Region       string `gorm:"column:region"`
	Status       string `gorm:"column:status"`
	Health       string `gorm:"column:health"`
	TestBehavior string `gorm:"column:test_behavior"`
}

func (providerRow) TableName() string { return "catalog_providers" }

type publicModelRow struct {
	ID           string `gorm:"column:id;primaryKey"`
	PublicID     string `gorm:"column:public_id"`
	Vendor       string `gorm:"column:vendor"`
	DisplayName  string `gorm:"column:display_name"`
	Capabilities []byte `gorm:"column:capabilities_json"`
	Status       string `gorm:"column:status"`
}

func (publicModelRow) TableName() string { return "catalog_public_models" }

type mappingRow struct {
	ID              string `gorm:"column:id;primaryKey"`
	PublicModelID   string `gorm:"column:public_model_id"`
	ProviderID      string `gorm:"column:provider_id"`
	UpstreamModelID string `gorm:"column:upstream_model_id"`
	Status          string `gorm:"column:status"`
}

func (mappingRow) TableName() string { return "catalog_provider_model_mappings" }

type priceRow struct {
	ID            string `gorm:"column:id;primaryKey"`
	PublicModelID string `gorm:"column:public_model_id"`
	ProviderID    *string `gorm:"column:provider_id"`
	UnitPrices    []byte `gorm:"column:unit_prices_json"`
	Status        string `gorm:"column:status"`
}

func (priceRow) TableName() string { return "catalog_price_versions" }

type routeGroupRow struct {
	ID            string `gorm:"column:id;primaryKey"`
	PublicModelID string `gorm:"column:public_model_id"`
	Strategy      string `gorm:"column:strategy"`
	Status        string `gorm:"column:status"`
}

func (routeGroupRow) TableName() string { return "catalog_route_groups" }

type candidateRow struct {
	RouteGroupID string `gorm:"column:route_group_id;primaryKey"`
	ProviderID   string `gorm:"column:provider_id;primaryKey"`
	Priority     int    `gorm:"column:priority"`
}

func (candidateRow) TableName() string { return "catalog_route_candidates" }

type channelPolicyRow struct {
	ChannelOrgID  string `gorm:"column:channel_org_id;primaryKey"`
	PublicModelID string `gorm:"column:public_model_id;primaryKey"`
	Enabled       bool   `gorm:"column:enabled"`
}

func (channelPolicyRow) TableName() string { return "catalog_channel_model_policies" }

type ModelView struct {
	ID            string         `json:"id"`
	Vendor        string         `json:"vendor"`
	DisplayName   string         `json:"display_name"`
	Capabilities  map[string]any `json:"capabilities"`
	SellPrice     map[string]any `json:"sell_price,omitempty"`
	Providers     []string       `json:"providers"`
	Status        string         `json:"status"`
}

type RouteCandidate struct {
	ProviderID      string
	ProviderSlug    string
	Adapter         string
	UpstreamModelID string
	TestBehavior    string
	Health          string
}

type RouteHint struct {
	Only  []string
	Ignore []string
	Order []string
}

type Service struct {
	db *gorm.DB
}

func New(db *gorm.DB) *Service { return &Service{db: db} }

func Migrations() (string, fs.FS) {
	sub, err := fs.Sub(migrationFS, "migrate")
	if err != nil {
		panic(err)
	}
	return "catalog", sub
}

func (s *Service) Seed(ctx context.Context) error {
	caps, _ := json.Marshal(map[string]any{
		"supported_parameters": []string{"stream", "temperature", "max_tokens", "messages", "model", "system", "tools"},
		"unsupported_parameters": []string{"logit_bias"},
	})
	oemCaps, _ := json.Marshal(map[string]any{
		"supported_parameters": []string{"stream", "messages", "model"},
	})
	price, _ := json.Marshal(map[string]any{"input": "0.000001", "output": "0.000002", "currency": "USD"})
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		providers := []providerRow{
			{ID: "prd_echo_primary", Name: "Echo Primary", Slug: PrimaryProvider, Kind: "direct", Adapter: "test", Status: "active", Health: "available", TestBehavior: "ok"},
			{ID: "prd_echo_backup", Name: "Echo Backup", Slug: BackupProvider, Kind: "direct", Adapter: "test", Status: "active", Health: "available", TestBehavior: "ok"},
		}
		for i := range providers {
			if err := tx.Where("slug = ?", providers[i].Slug).FirstOrCreate(&providers[i]).Error; err != nil {
				return err
			}
		}
		models := []publicModelRow{
			{ID: "mdl_echo", PublicID: EchoModelID, Vendor: "tokenhub", DisplayName: "Echo", Capabilities: caps, Status: "published"},
			{ID: "mdl_oem", PublicID: OEMModelID, Vendor: "tokenhub", DisplayName: "OEM Demo", Capabilities: oemCaps, Status: "published"},
		}
		for i := range models {
			if err := tx.Where("public_id = ?", models[i].PublicID).FirstOrCreate(&models[i]).Error; err != nil {
				return err
			}
		}
		mappings := []mappingRow{
			{ID: "map_echo_p", PublicModelID: "mdl_echo", ProviderID: "prd_echo_primary", UpstreamModelID: "echo-upstream", Status: "active"},
			{ID: "map_echo_b", PublicModelID: "mdl_echo", ProviderID: "prd_echo_backup", UpstreamModelID: "echo-upstream", Status: "active"},
			{ID: "map_oem_p", PublicModelID: "mdl_oem", ProviderID: "prd_echo_primary", UpstreamModelID: "oem-upstream", Status: "active"},
		}
		for i := range mappings {
			if err := tx.Where("id = ?", mappings[i].ID).FirstOrCreate(&mappings[i]).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("id = ?", "price_echo").FirstOrCreate(&priceRow{ID: "price_echo", PublicModelID: "mdl_echo", UnitPrices: price, Status: "published"}).Error; err != nil {
			return err
		}
		if err := tx.Where("id = ?", "rg_echo").FirstOrCreate(&routeGroupRow{ID: "rg_echo", PublicModelID: "mdl_echo", Strategy: "priority", Status: "active"}).Error; err != nil {
			return err
		}
		cands := []candidateRow{
			{RouteGroupID: "rg_echo", ProviderID: "prd_echo_primary", Priority: 1},
			{RouteGroupID: "rg_echo", ProviderID: "prd_echo_backup", Priority: 2},
		}
		for i := range cands {
			if err := tx.Where("route_group_id = ? AND provider_id = ?", cands[i].RouteGroupID, cands[i].ProviderID).FirstOrCreate(&cands[i]).Error; err != nil {
				return err
			}
		}
		policies := []channelPolicyRow{
			{ChannelOrgID: identity.OfficialChannelID, PublicModelID: "mdl_echo", Enabled: true},
			{ChannelOrgID: identity.ResellerChannelID, PublicModelID: "mdl_echo", Enabled: true},
			{ChannelOrgID: identity.OEMChannelID, PublicModelID: "mdl_echo", Enabled: true},
			{ChannelOrgID: identity.OEMChannelID, PublicModelID: "mdl_oem", Enabled: true},
		}
		for i := range policies {
			if err := tx.Where("channel_org_id = ? AND public_model_id = ?", policies[i].ChannelOrgID, policies[i].PublicModelID).FirstOrCreate(&policies[i]).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Service) ListVisibleModels(ctx context.Context, channelOrgID string, allowlist []string) ([]ModelView, error) {
	var models []publicModelRow
	q := s.db.WithContext(ctx).Table("catalog_public_models m").
		Select("m.*").
		Joins("JOIN catalog_channel_model_policies p ON p.public_model_id = m.id AND p.enabled = true").
		Where("m.status = ? AND p.channel_org_id = ?", "published", channelOrgID)
	if len(allowlist) > 0 {
		q = q.Where("m.public_id IN ?", allowlist)
	}
	if err := q.Find(&models).Error; err != nil {
		return nil, err
	}
	out := make([]ModelView, 0, len(models))
	for _, model := range models {
		view, err := s.modelView(ctx, model)
		if err != nil {
			return nil, err
		}
		out = append(out, *view)
	}
	return out, nil
}

func (s *Service) GetVisibleModel(ctx context.Context, channelOrgID, publicID string, allowlist []string) (*ModelView, error) {
	models, err := s.ListVisibleModels(ctx, channelOrgID, allowlist)
	if err != nil {
		return nil, err
	}
	for i := range models {
		if models[i].ID == publicID {
			return &models[i], nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func (s *Service) ResolveRoute(ctx context.Context, publicID string, hint RouteHint) ([]RouteCandidate, error) {
	var model publicModelRow
	if err := s.db.WithContext(ctx).Where("public_id = ?", publicID).First(&model).Error; err != nil {
		return nil, err
	}
	var group routeGroupRow
	if err := s.db.WithContext(ctx).Where("public_model_id = ? AND status = ?", model.ID, "active").First(&group).Error; err != nil {
		return nil, err
	}
	var cands []candidateRow
	if err := s.db.WithContext(ctx).Where("route_group_id = ?", group.ID).Order("priority ASC").Find(&cands).Error; err != nil {
		return nil, err
	}
	out := make([]RouteCandidate, 0, len(cands))
	for _, cand := range cands {
		var provider providerRow
		if err := s.db.WithContext(ctx).Where("id = ?", cand.ProviderID).First(&provider).Error; err != nil {
			continue
		}
		if provider.Status != "active" || provider.Health == "unavailable" {
			continue
		}
		if ignored(hint.Ignore, provider.Slug) {
			continue
		}
		if len(hint.Only) > 0 && !contains(hint.Only, provider.Slug) {
			continue
		}
		var mapping mappingRow
		if err := s.db.WithContext(ctx).Where("public_model_id = ? AND provider_id = ? AND status = ?", model.ID, provider.ID, "active").First(&mapping).Error; err != nil {
			continue
		}
		out = append(out, RouteCandidate{
			ProviderID: provider.ID, ProviderSlug: provider.Slug, Adapter: provider.Adapter,
			UpstreamModelID: mapping.UpstreamModelID, TestBehavior: provider.TestBehavior, Health: provider.Health,
		})
	}
	if len(hint.Order) > 0 {
		out = orderBy(out, hint.Order)
	}
	return out, nil
}

func (s *Service) MarkHealth(ctx context.Context, providerID, health string) error {
	return s.db.WithContext(ctx).Model(&providerRow{}).Where("id = ?", providerID).Update("health", health).Error
}

type ProviderView struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Slug   string `json:"slug"`
	Health string `json:"health"`
	Status string `json:"status"`
}

func (s *Service) ListProviders(ctx context.Context) ([]ProviderView, error) {
	var rows []providerRow
	if err := s.db.WithContext(ctx).Order("slug").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]ProviderView, 0, len(rows))
	for _, row := range rows {
		out = append(out, ProviderView{ID: row.ID, Name: row.Name, Slug: row.Slug, Health: row.Health, Status: row.Status})
	}
	return out, nil
}

func (s *Service) Probe(ctx context.Context, providerID string) (string, error) {
	var provider providerRow
	if err := s.db.WithContext(ctx).Where("id = ?", providerID).First(&provider).Error; err != nil {
		return "", err
	}
	health := "available"
	if provider.TestBehavior == "down" {
		health = "unavailable"
	}
	_ = s.MarkHealth(ctx, provider.ID, health)
	return health, nil
}

func (s *Service) modelView(ctx context.Context, model publicModelRow) (*ModelView, error) {
	caps := map[string]any{}
	_ = json.Unmarshal(model.Capabilities, &caps)
	var price priceRow
	sell := map[string]any{}
	if err := s.db.WithContext(ctx).Where("public_model_id = ? AND status = ?", model.ID, "published").First(&price).Error; err == nil {
		_ = json.Unmarshal(price.UnitPrices, &sell)
	}
	var slugs []string
	_ = s.db.WithContext(ctx).Table("catalog_provider_model_mappings m").
		Select("p.slug").
		Joins("JOIN catalog_providers p ON p.id = m.provider_id").
		Where("m.public_model_id = ? AND m.status = ?", model.ID, "active").
		Scan(&slugs).Error
	return &ModelView{ID: model.PublicID, Vendor: model.Vendor, DisplayName: model.DisplayName, Capabilities: caps, SellPrice: sell, Providers: slugs, Status: model.Status}, nil
}

func ignored(list []string, slug string) bool {
	return contains(list, slug)
}

func contains(list []string, slug string) bool {
	for _, item := range list {
		if strings.EqualFold(strings.TrimSpace(item), slug) {
			return true
		}
	}
	return false
}

func orderBy(cands []RouteCandidate, order []string) []RouteCandidate {
	ranked := make([]RouteCandidate, 0, len(cands))
	used := map[string]bool{}
	for _, slug := range order {
		for _, cand := range cands {
			if cand.ProviderSlug == slug && !used[slug] {
				ranked = append(ranked, cand)
				used[slug] = true
			}
		}
	}
	for _, cand := range cands {
		if !used[cand.ProviderSlug] {
			ranked = append(ranked, cand)
		}
	}
	return ranked
}
