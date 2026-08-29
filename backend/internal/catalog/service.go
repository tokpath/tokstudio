package catalog

import (
	"context"
	"embed"
	"encoding/json"
	"io/fs"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/tokpath/tokstudio/backend/internal/platform/id"

	"github.com/tokpath/tokstudio/backend/internal/identity"
)

//go:embed migrate/*.sql
var migrationFS embed.FS

const (
	EchoModelID         = "tokenhub/echo-1"
	OEMModelID          = "tokenhub/oem-demo"
	SeedanceModelID     = "bytedance/seedance-1.0"
	ImageModelID        = "tokenhub/image-demo"
	GeminiModelID       = "google/gemini-flash"
	PrimaryProvider     = "echo-primary"
	BackupProvider      = "echo-backup"
	ArkSeedanceProvider = "ark-seedance"
	OpenRouterProvider  = "openrouter-seedance"
	GeminiProvider      = "gemini-flash"
)

type providerRow struct {
	ID               string `gorm:"column:id;primaryKey"`
	Name             string `gorm:"column:name"`
	Slug             string `gorm:"column:slug"`
	Kind             string `gorm:"column:kind"`
	Adapter          string `gorm:"column:adapter"`
	BaseURL          string `gorm:"column:base_url"`
	Region           string `gorm:"column:region"`
	Status           string `gorm:"column:status"`
	Health           string `gorm:"column:health"`
	TestBehavior     string `gorm:"column:test_behavior"`
	Priority         int    `gorm:"column:priority"`
	Weight           int    `gorm:"column:weight"`
	TimeoutMS        int    `gorm:"column:timeout_ms"`
	RetryMax         int    `gorm:"column:retry_max"`
	RPMLimit         int    `gorm:"column:rpm_limit"`
	ConcurrencyLimit int    `gorm:"column:concurrency_limit"`
	CapabilityTags   string `gorm:"column:capability_tags"`
	CredentialRef    string `gorm:"column:credential_ref"`
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
	ID            string    `gorm:"column:id;primaryKey"`
	PublicModelID string    `gorm:"column:public_model_id"`
	ProviderID    *string   `gorm:"column:provider_id"`
	UnitPrices    []byte    `gorm:"column:unit_prices_json"`
	Status        string    `gorm:"column:status"`
	EffectiveAt   time.Time `gorm:"column:effective_at"`
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
	Weight       int    `gorm:"column:weight"`
}

func (candidateRow) TableName() string { return "catalog_route_candidates" }

type channelPolicyRow struct {
	ChannelOrgID  string `gorm:"column:channel_org_id;primaryKey"`
	PublicModelID string `gorm:"column:public_model_id;primaryKey"`
	Enabled       bool   `gorm:"column:enabled"`
}

func (channelPolicyRow) TableName() string { return "catalog_channel_model_policies" }

type ModelView struct {
	ID           string         `json:"id"`
	Vendor       string         `json:"vendor"`
	DisplayName  string         `json:"display_name"`
	Capabilities map[string]any `json:"capabilities"`
	SellPrice    map[string]any `json:"sell_price,omitempty"`
	Providers    []string       `json:"providers"`
	Status       string         `json:"status"`
}

type RouteCandidate struct {
	ProviderID      string
	ProviderSlug    string
	Adapter         string
	UpstreamModelID string
	TestBehavior    string
	Health          string
	Priority        int
	Weight          int
	CostMinor       int64
}

type RouteHint struct {
	Only   []string
	Ignore []string
	Order  []string
}

type Service struct {
	db         *gorm.DB
	production bool
	allowHosts []string
}

func New(db *gorm.DB) *Service { return &Service{db: db} }

// SetURLPolicy 由组装层注入：生产环境禁止私网和不在白名单的上游 Base URL。
func (s *Service) SetURLPolicy(production bool, allowHosts []string) {
	s.production = production
	s.allowHosts = allowHosts
}

func Migrations() (string, fs.FS) {
	sub, err := fs.Sub(migrationFS, "migrate")
	if err != nil {
		panic(err)
	}
	return "catalog", sub
}

func (s *Service) Seed(ctx context.Context) error {
	caps, _ := json.Marshal(map[string]any{
		"supported_parameters":   []string{"stream", "temperature", "max_tokens", "messages", "model", "system", "tools"},
		"unsupported_parameters": []string{"logit_bias"},
	})
	oemCaps, _ := json.Marshal(map[string]any{
		"supported_parameters": []string{"stream", "messages", "model"},
	})
	price, _ := json.Marshal(map[string]any{
		"input": "0.000001", "output": "0.000002", "currency": "USD",
		"upstream_cost_input": "0.0000004", "upstream_cost_output": "0.0000008",
		"wholesale_input": "0.0000007", "wholesale_output": "0.0000014",
	})
	mediaCaps, _ := json.Marshal(map[string]any{
		"supported_parameters": []string{"prompt", "duration", "resolution", "aspect_ratio", "fps", "generate_audio", "callback_url", "images"},
		"media":                []string{"video", "image"},
	})
	mediaPrice, _ := json.Marshal(map[string]any{
		"currency": "USD", "video_second": "0.01", "image_count": "0.02", "audio_second": "0.002",
		"video_second_cost": "0.004", "image_count_cost": "0.008",
	})
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
		if err := seedMediaCatalog(tx, mediaCaps, mediaPrice); err != nil {
			return err
		}
		return seedGeminiCatalog(tx, caps, price)
	})
}

func seedMediaCatalog(tx *gorm.DB, caps, price []byte) error {
	providers := []providerRow{
		{ID: "prd_ark", Name: "Volcengine Ark", Slug: ArkSeedanceProvider, Kind: "direct", Adapter: "ark", Status: "active", Health: "available", TestBehavior: "ok"},
		{ID: "prd_or", Name: "OpenRouter Media", Slug: OpenRouterProvider, Kind: "aggregator", Adapter: "openrouter", Status: "active", Health: "available", TestBehavior: "ok"},
	}
	for i := range providers {
		if err := tx.Where("slug = ?", providers[i].Slug).FirstOrCreate(&providers[i]).Error; err != nil {
			return err
		}
	}
	models := []publicModelRow{
		{ID: "mdl_seedance", PublicID: SeedanceModelID, Vendor: "bytedance", DisplayName: "Seedance", Capabilities: caps, Status: "published"},
		{ID: "mdl_image", PublicID: ImageModelID, Vendor: "tokenhub", DisplayName: "Image Demo", Capabilities: caps, Status: "published"},
	}
	for i := range models {
		if err := tx.Where("public_id = ?", models[i].PublicID).FirstOrCreate(&models[i]).Error; err != nil {
			return err
		}
	}
	mappings := []mappingRow{
		{ID: "map_sd_ark", PublicModelID: "mdl_seedance", ProviderID: "prd_ark", UpstreamModelID: "seedance-1-0-ark", Status: "active"},
		{ID: "map_sd_or", PublicModelID: "mdl_seedance", ProviderID: "prd_or", UpstreamModelID: "bytedance/seedance-1.0", Status: "active"},
		{ID: "map_img_ark", PublicModelID: "mdl_image", ProviderID: "prd_ark", UpstreamModelID: "image-demo-ark", Status: "active"},
	}
	for i := range mappings {
		if err := tx.Where("id = ?", mappings[i].ID).FirstOrCreate(&mappings[i]).Error; err != nil {
			return err
		}
	}
	if err := tx.Where("id = ?", "price_seedance").FirstOrCreate(&priceRow{ID: "price_seedance", PublicModelID: "mdl_seedance", UnitPrices: price, Status: "published"}).Error; err != nil {
		return err
	}
	if err := tx.Where("id = ?", "price_image").FirstOrCreate(&priceRow{ID: "price_image", PublicModelID: "mdl_image", UnitPrices: price, Status: "published"}).Error; err != nil {
		return err
	}
	if err := tx.Where("id = ?", "rg_seedance").FirstOrCreate(&routeGroupRow{ID: "rg_seedance", PublicModelID: "mdl_seedance", Strategy: "priority", Status: "active"}).Error; err != nil {
		return err
	}
	if err := tx.Where("id = ?", "rg_image").FirstOrCreate(&routeGroupRow{ID: "rg_image", PublicModelID: "mdl_image", Strategy: "priority", Status: "active"}).Error; err != nil {
		return err
	}
	cands := []candidateRow{
		{RouteGroupID: "rg_seedance", ProviderID: "prd_ark", Priority: 1},
		{RouteGroupID: "rg_seedance", ProviderID: "prd_or", Priority: 2},
		{RouteGroupID: "rg_image", ProviderID: "prd_ark", Priority: 1},
	}
	for i := range cands {
		if err := tx.Where("route_group_id = ? AND provider_id = ?", cands[i].RouteGroupID, cands[i].ProviderID).FirstOrCreate(&cands[i]).Error; err != nil {
			return err
		}
	}
	policies := []channelPolicyRow{
		{ChannelOrgID: identity.OfficialChannelID, PublicModelID: "mdl_seedance", Enabled: true},
		{ChannelOrgID: identity.ResellerChannelID, PublicModelID: "mdl_seedance", Enabled: true},
		{ChannelOrgID: identity.OEMChannelID, PublicModelID: "mdl_seedance", Enabled: true},
		{ChannelOrgID: identity.OfficialChannelID, PublicModelID: "mdl_image", Enabled: true},
		{ChannelOrgID: identity.ResellerChannelID, PublicModelID: "mdl_image", Enabled: true},
		{ChannelOrgID: identity.OEMChannelID, PublicModelID: "mdl_image", Enabled: true},
	}
	for i := range policies {
		if err := tx.Where("channel_org_id = ? AND public_model_id = ?", policies[i].ChannelOrgID, policies[i].PublicModelID).FirstOrCreate(&policies[i]).Error; err != nil {
			return err
		}
	}
	return nil
}

func seedGeminiCatalog(tx *gorm.DB, caps, price []byte) error {
	if err := tx.Where("slug = ?", GeminiProvider).FirstOrCreate(&providerRow{
		ID: "prd_gemini", Name: "Google Gemini Flash", Slug: GeminiProvider, Kind: "direct",
		Adapter: "gemini", Status: "active", Health: "available", TestBehavior: "ok",
		Priority: 80, Weight: 1, TimeoutMS: 30000, RetryMax: 1, CapabilityTags: "text,gemini",
	}).Error; err != nil {
		return err
	}
	if err := tx.Where("public_id = ?", GeminiModelID).FirstOrCreate(&publicModelRow{
		ID: "mdl_gemini", PublicID: GeminiModelID, Vendor: "google", DisplayName: "Gemini Flash",
		Capabilities: caps, Status: "published",
	}).Error; err != nil {
		return err
	}
	if err := tx.Where("id = ?", "map_gemini").FirstOrCreate(&mappingRow{
		ID: "map_gemini", PublicModelID: "mdl_gemini", ProviderID: "prd_gemini",
		UpstreamModelID: "gemini-2.0-flash", Status: "active",
	}).Error; err != nil {
		return err
	}
	if err := tx.Where("id = ?", "price_gemini").FirstOrCreate(&priceRow{
		ID: "price_gemini", PublicModelID: "mdl_gemini", UnitPrices: price, Status: "published",
	}).Error; err != nil {
		return err
	}
	if err := tx.Where("id = ?", "rg_gemini").FirstOrCreate(&routeGroupRow{
		ID: "rg_gemini", PublicModelID: "mdl_gemini", Strategy: "priority", Status: "active",
	}).Error; err != nil {
		return err
	}
	if err := tx.Where("route_group_id = ? AND provider_id = ?", "rg_gemini", "prd_gemini").
		FirstOrCreate(&candidateRow{RouteGroupID: "rg_gemini", ProviderID: "prd_gemini", Priority: 1}).Error; err != nil {
		return err
	}
	for _, channelID := range []string{identity.OfficialChannelID, identity.ResellerChannelID, identity.OEMChannelID} {
		if err := tx.Where("channel_org_id = ? AND public_model_id = ?", channelID, "mdl_gemini").
			FirstOrCreate(&channelPolicyRow{ChannelOrgID: channelID, PublicModelID: "mdl_gemini", Enabled: true}).Error; err != nil {
			return err
		}
	}
	return nil
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
		if provider.Status != "active" || provider.Health == "unavailable" || provider.Health == "maintenance" {
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
		weight := cand.Weight
		if weight == 0 {
			weight = provider.Weight
		}
		if weight == 0 {
			weight = 1
		}
		out = append(out, RouteCandidate{
			ProviderID: provider.ID, ProviderSlug: provider.Slug, Adapter: provider.Adapter,
			UpstreamModelID: mapping.UpstreamModelID, TestBehavior: provider.TestBehavior, Health: provider.Health,
			Priority: cand.Priority, Weight: weight, CostMinor: s.providerCost(ctx, model.ID, provider.ID),
		})
	}
	applyStrategy(out, group.Strategy)
	if len(hint.Order) > 0 {
		out = orderBy(out, hint.Order)
	}
	return out, nil
}

func (s *Service) providerCost(ctx context.Context, modelID, providerID string) int64 {
	var price priceRow
	if err := s.db.WithContext(ctx).Where("public_model_id = ? AND provider_id = ? AND status = ?", modelID, providerID, "published").
		Order("effective_at DESC").First(&price).Error; err == nil {
		return costMinor(price.UnitPrices)
	}
	if err := s.db.WithContext(ctx).Where("public_model_id = ? AND provider_id IS NULL AND status = ?", modelID, "published").
		Order("effective_at DESC").First(&price).Error; err == nil {
		return costMinor(price.UnitPrices)
	}
	if err := s.db.WithContext(ctx).Where("public_model_id = ? AND status = ?", modelID, "published").
		Order("effective_at DESC").First(&price).Error; err == nil {
		return costMinor(price.UnitPrices)
	}
	return 1 << 62
}

// PriceSnapshot 是账务模块允许看到的公开价格视图，不含内部 ORM。
type PriceSnapshot struct {
	VersionID string
	PublicID  string
	Raw       json.RawMessage
}

func (s *Service) PriceSnapshot(ctx context.Context, publicID string) (*PriceSnapshot, error) {
	var model publicModelRow
	if err := s.db.WithContext(ctx).Where("public_id = ?", publicID).First(&model).Error; err != nil {
		return nil, err
	}
	var price priceRow
	err := s.db.WithContext(ctx).Where("public_model_id = ? AND provider_id IS NULL AND status = ?", model.ID, "published").
		Order("effective_at DESC").First(&price).Error
	if err != nil {
		err = s.db.WithContext(ctx).Where("public_model_id = ? AND status = ?", model.ID, "published").
			Order("effective_at DESC").First(&price).Error
	}
	if err != nil {
		return nil, err
	}
	return &PriceSnapshot{VersionID: price.ID, PublicID: model.PublicID, Raw: price.UnitPrices}, nil
}

func (s *Service) PublishPrice(ctx context.Context, publicID string, unitPrices map[string]any) (*PriceSnapshot, error) {
	var model publicModelRow
	if err := s.db.WithContext(ctx).Where("public_id = ?", publicID).First(&model).Error; err != nil {
		return nil, err
	}
	body, err := json.Marshal(unitPrices)
	if err != nil {
		return nil, err
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&priceRow{}).Where("public_model_id = ? AND status = ?", model.ID, "published").
			Update("status", "superseded").Error; err != nil {
			return err
		}
		row := priceRow{
			ID: id.New("prc"), PublicModelID: model.ID, UnitPrices: body,
			Status: "published", EffectiveAt: time.Now().UTC(),
		}
		return tx.Create(&row).Error
	})
	if err != nil {
		return nil, err
	}
	return s.PriceSnapshot(ctx, publicID)
}

func (s *Service) MarkHealth(ctx context.Context, providerID, health string) error {
	return s.db.WithContext(ctx).Model(&providerRow{}).Where("id = ?", providerID).Update("health", health).Error
}

type ProviderView struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Slug             string `json:"slug"`
	Kind             string `json:"kind"`
	Adapter          string `json:"adapter"`
	BaseURL          string `json:"base_url,omitempty"`
	Region           string `json:"region,omitempty"`
	Health           string `json:"health"`
	Status           string `json:"status"`
	Priority         int    `json:"priority"`
	Weight           int    `json:"weight"`
	TimeoutMS        int    `json:"timeout_ms"`
	RetryMax         int    `json:"retry_max"`
	RPMLimit         int    `json:"rpm_limit"`
	ConcurrencyLimit int    `json:"concurrency_limit"`
	CapabilityTags   string `json:"capability_tags,omitempty"`
	CredentialRef    string `json:"credential_ref,omitempty"`
	TestBehavior     string `json:"test_behavior,omitempty"`
}

func (s *Service) ListProviders(ctx context.Context) ([]ProviderView, error) {
	var rows []providerRow
	if err := s.db.WithContext(ctx).Order("slug").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]ProviderView, 0, len(rows))
	for _, row := range rows {
		out = append(out, *providerView(row))
	}
	return out, nil
}

func (s *Service) Probe(ctx context.Context, providerID string) (string, error) {
	var provider providerRow
	if err := s.db.WithContext(ctx).Where("id = ?", providerID).First(&provider).Error; err != nil {
		return "", err
	}
	health := "available"
	switch provider.TestBehavior {
	case "down":
		health = "unavailable"
	case "429", "degraded":
		health = "degraded"
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
