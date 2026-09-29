package catalog

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
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
}

func (providerRow) TableName() string { return "catalog_providers" }

type publicModelRow struct {
	ID               string `gorm:"column:id;primaryKey"`
	PublicID         string `gorm:"column:public_id"`
	Vendor           string `gorm:"column:vendor"`
	DisplayName      string `gorm:"column:display_name"`
	Capabilities     []byte `gorm:"column:capabilities_json"`
	Status           string `gorm:"column:status"`
	SyncState        string `gorm:"column:sync_state"`
	CreatedByUserID  string `gorm:"column:created_by_user_id"`
	ReviewedByUserID string `gorm:"column:reviewed_by_user_id"`
}

func (publicModelRow) TableName() string { return "catalog_public_models" }

type mappingRow struct {
	ID              string `gorm:"column:id;primaryKey"`
	PublicModelID   string `gorm:"column:public_model_id"`
	ProviderID      string `gorm:"column:provider_id"`
	UpstreamModelID string `gorm:"column:upstream_model_id"`
	Status          string `gorm:"column:status"`
	SyncState       string `gorm:"column:sync_state"`
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
	Wholesale     []byte `gorm:"column:wholesale_json"`
	Override      []byte `gorm:"column:customer_override_json"`
}

func (channelPolicyRow) TableName() string { return "catalog_channel_model_policies" }

type ModelView struct {
	ID                  string         `json:"id"`
	Vendor              string         `json:"vendor"`
	DisplayName         string         `json:"display_name"`
	Capabilities        map[string]any `json:"capabilities"`
	SellPrice           map[string]any `json:"sell_price,omitempty"`
	Providers           []string       `json:"providers"`
	Status              string         `json:"status"`
	ConfigReady         bool           `json:"config_ready"`
	SyncState           string         `json:"sync_state,omitempty"`
	CreatedByUserID     string         `json:"created_by_user_id,omitempty"`
	ReviewedByUserID    string         `json:"reviewed_by_user_id,omitempty"`
	Description         string         `json:"description,omitempty"`
	Kind                string         `json:"kind,omitempty"`
	ContextLength       int            `json:"context_length,omitempty"`
	MaxCompletionTokens int            `json:"max_completion_tokens,omitempty"`
}

// ChannelModelView 是租户可见的平台目录切片，不含上游凭据。租户不能自建提供商或模型。
type ChannelModelView struct {
	PublicID         string            `json:"public_id"`
	DisplayName      string            `json:"display_name"`
	Vendor           string            `json:"vendor"`
	Kind             string            `json:"kind"`
	Status           string            `json:"status"`
	Enabled          bool              `json:"enabled"`
	Wholesale        map[string]string `json:"wholesale,omitempty"`
	CustomerOverride map[string]string `json:"customer_override,omitempty"`
}

type RouteCandidate struct {
	ProviderID      string
	ProviderSlug    string
	Adapter         string
	BaseURL         string
	UpstreamModelID string
	TestBehavior    string
	Health          string
	Priority        int
	Weight          int
	CostMinor       int64
	UnitCosts       json.RawMessage
	AccountID       string
	TimeoutMS       int
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
		"kind":                   "text",
		"supported_parameters":   []string{"stream", "temperature", "max_tokens", "messages", "model", "system", "tools", "vision", "json", "reasoning", "response_format", "tool_choice"},
		"unsupported_parameters": []string{"logit_bias"},
	})
	oemCaps, _ := json.Marshal(map[string]any{
		"kind":                 "text",
		"supported_parameters": []string{"stream", "messages", "model"},
	})
	price, _ := json.Marshal(map[string]any{
		"input": "0.000001", "output": "0.000002", "currency": "USD",
	})
	textCost, _ := json.Marshal(map[string]string{"input": "0.0000004", "output": "0.0000008"})
	textWholesale, _ := json.Marshal(map[string]string{"input": "0.0000007", "output": "0.0000014"})
	mediaCaps, _ := json.Marshal(map[string]any{
		"supported_parameters": []string{
			"prompt", "duration", "resolution", "aspect_ratio", "fps", "generate_audio",
			"callback_url", "images", "task_type", "first_frame", "last_frame",
			"reference_video", "reference_audio", "source_job_id",
		},
		"task_types":       []string{"t2v", "i2v", "first_frame", "first_last_frame", "reference", "extend", "edit", "generate"},
		"media":            []string{"video", "image"},
		"output_modality":  []string{"video", "image"},
		"input_modalities": []string{"text", "image", "video", "audio"},
	})
	mediaPrice, _ := json.Marshal(map[string]any{
		"currency": "USD", "video_second": "0.01", "image_count": "0.02", "audio_second": "0.002",
	})
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
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
			{ID: "mdl_echo", PublicID: EchoModelID, Vendor: "tokenhub", DisplayName: "Echo", Capabilities: caps, Status: "published", SyncState: SyncPublished},
			{ID: "mdl_oem", PublicID: OEMModelID, Vendor: "tokenhub", DisplayName: "OEM Demo", Capabilities: oemCaps, Status: "published", SyncState: SyncPublished},
		}
		for i := range models {
			if err := tx.Where("public_id = ?", models[i].PublicID).FirstOrCreate(&models[i]).Error; err != nil {
				return err
			}
		}
		if err := tx.Model(&publicModelRow{}).Where("public_id = ?", EchoModelID).Update("capabilities_json", caps).Error; err != nil {
			return err
		}
		if err := tx.Model(&publicModelRow{}).Where("public_id = ?", OEMModelID).Update("capabilities_json", oemCaps).Error; err != nil {
			return err
		}
		mappings := []mappingRow{
			{ID: "map_echo_p", PublicModelID: "mdl_echo", ProviderID: "prd_echo_primary", UpstreamModelID: "echo-upstream", Status: "active", SyncState: SyncPublished},
			{ID: "map_echo_b", PublicModelID: "mdl_echo", ProviderID: "prd_echo_backup", UpstreamModelID: "echo-upstream", Status: "active", SyncState: SyncPublished},
			{ID: "map_oem_p", PublicModelID: "mdl_oem", ProviderID: "prd_echo_primary", UpstreamModelID: "oem-upstream", Status: "active", SyncState: SyncPublished},
		}
		for i := range mappings {
			if err := tx.Where("id = ?", mappings[i].ID).FirstOrCreate(&mappings[i]).Error; err != nil {
				return err
			}
		}
		if err := seedProviderModels(tx, []providerModelRow{
			{ProviderID: "prd_echo_primary", UpstreamModelID: "echo-upstream", DisplayName: "Echo", UnitCosts: textCost},
			{ProviderID: "prd_echo_backup", UpstreamModelID: "echo-upstream", DisplayName: "Echo", UnitCosts: textCost},
			{ProviderID: "prd_echo_primary", UpstreamModelID: "oem-upstream", DisplayName: "OEM Demo", UnitCosts: textCost},
		}); err != nil {
			return err
		}
		if err := tx.Where("id = ?", "price_echo").FirstOrCreate(&priceRow{
			ID: "price_echo", PublicModelID: "mdl_echo", UnitPrices: price, Status: "published", EffectiveAt: time.Now().UTC(),
		}).Error; err != nil {
			return err
		}
		if err := tx.Where("id = ?", "price_oem").FirstOrCreate(&priceRow{
			ID: "price_oem", PublicModelID: "mdl_oem", UnitPrices: price, Status: "published", EffectiveAt: time.Now().UTC(),
		}).Error; err != nil {
			return err
		}
		if err := tx.Model(&priceRow{}).Where("effective_at = ?", time.Time{}).Update("effective_at", time.Now().UTC()).Error; err != nil {
			return err
		}
		if err := tx.Where("id = ?", "rg_echo").FirstOrCreate(&routeGroupRow{ID: "rg_echo", PublicModelID: "mdl_echo", Strategy: "priority", Status: "active"}).Error; err != nil {
			return err
		}
		if err := tx.Where("id = ?", "rg_oem").FirstOrCreate(&routeGroupRow{ID: "rg_oem", PublicModelID: "mdl_oem", Strategy: "priority", Status: "active"}).Error; err != nil {
			return err
		}
		cands := []candidateRow{
			{RouteGroupID: "rg_echo", ProviderID: "prd_echo_primary", Priority: 1},
			{RouteGroupID: "rg_echo", ProviderID: "prd_echo_backup", Priority: 2},
			{RouteGroupID: "rg_oem", ProviderID: "prd_echo_primary", Priority: 1},
		}
		for i := range cands {
			if err := tx.Where("route_group_id = ? AND provider_id = ?", cands[i].RouteGroupID, cands[i].ProviderID).FirstOrCreate(&cands[i]).Error; err != nil {
				return err
			}
		}
		policies := []channelPolicyRow{
			{ChannelOrgID: identity.OfficialChannelID, PublicModelID: "mdl_echo", Enabled: true, Wholesale: textWholesale},
			{ChannelOrgID: identity.ResellerChannelID, PublicModelID: "mdl_echo", Enabled: true, Wholesale: textWholesale},
			{ChannelOrgID: identity.OEMChannelID, PublicModelID: "mdl_echo", Enabled: true, Wholesale: textWholesale},
			{ChannelOrgID: identity.OEMChannelID, PublicModelID: "mdl_oem", Enabled: true, Wholesale: textWholesale},
		}
		if err := seedChannelPolicies(tx, policies); err != nil {
			return err
		}
		if err := seedMediaCatalog(tx, mediaCaps, mediaPrice); err != nil {
			return err
		}
		if err := seedGeminiCatalog(tx, caps, price); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return err
	}
	_, err := s.ImportOfoxSnapshot(ctx)
	return err
}

func seedProviderModels(tx *gorm.DB, rows []providerModelRow) error {
	for _, row := range rows {
		row.PriceSource = "manual"
		if row.Status == "" {
			row.Status = "active"
		}
		row.UpdatedAt = time.Now().UTC()
		if err := tx.Where("provider_id = ? AND upstream_model_id = ?", row.ProviderID, row.UpstreamModelID).FirstOrCreate(&row).Error; err != nil {
			return err
		}
	}
	return nil
}

func seedChannelPolicies(tx *gorm.DB, rows []channelPolicyRow) error {
	for _, row := range rows {
		var existing channelPolicyRow
		err := tx.Where("channel_org_id = ? AND public_model_id = ?", row.ChannelOrgID, row.PublicModelID).First(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		} else if len(decodeCosts(existing.Wholesale)) == 0 {
			if err := tx.Model(&existing).Update("wholesale_json", row.Wholesale).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func seedMediaCatalog(tx *gorm.DB, caps, price []byte) error {
	videoCost, _ := json.Marshal(map[string]string{"video_second": "0.004"})
	imageCost, _ := json.Marshal(map[string]string{"image_count": "0.008"})
	videoWholesale, _ := json.Marshal(map[string]string{"video_second": "0.007"})
	imageWholesale, _ := json.Marshal(map[string]string{"image_count": "0.014"})
	var baseCaps map[string]any
	if err := json.Unmarshal(caps, &baseCaps); err != nil {
		return err
	}
	videoCaps := map[string]any{}
	imageCaps := map[string]any{}
	for key, value := range baseCaps {
		videoCaps[key] = value
		imageCaps[key] = value
	}
	videoCaps["kind"] = "video"
	imageCaps["kind"] = "image"
	videoJSON, _ := json.Marshal(videoCaps)
	imageJSON, _ := json.Marshal(imageCaps)
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
		{ID: "mdl_seedance", PublicID: SeedanceModelID, Vendor: "bytedance", DisplayName: "Seedance", Capabilities: videoJSON, Status: "published", SyncState: SyncPublished},
		{ID: "mdl_image", PublicID: ImageModelID, Vendor: "tokenhub", DisplayName: "Image Demo", Capabilities: imageJSON, Status: "published", SyncState: SyncPublished},
	}
	for i := range models {
		if err := tx.Where("public_id = ?", models[i].PublicID).FirstOrCreate(&models[i]).Error; err != nil {
			return err
		}
	}
	if err := tx.Model(&publicModelRow{}).Where("public_id = ?", SeedanceModelID).Update("capabilities_json", videoJSON).Error; err != nil {
		return err
	}
	if err := tx.Model(&publicModelRow{}).Where("public_id = ?", ImageModelID).Update("capabilities_json", imageJSON).Error; err != nil {
		return err
	}
	mappings := []mappingRow{
		{ID: "map_sd_ark", PublicModelID: "mdl_seedance", ProviderID: "prd_ark", UpstreamModelID: "seedance-1-0-ark", Status: "active", SyncState: SyncPublished},
		{ID: "map_sd_or", PublicModelID: "mdl_seedance", ProviderID: "prd_or", UpstreamModelID: "bytedance/seedance-1.0", Status: "active", SyncState: SyncPublished},
		{ID: "map_img_ark", PublicModelID: "mdl_image", ProviderID: "prd_ark", UpstreamModelID: "image-demo-ark", Status: "active", SyncState: SyncPublished},
	}
	for i := range mappings {
		if err := tx.Where("id = ?", mappings[i].ID).FirstOrCreate(&mappings[i]).Error; err != nil {
			return err
		}
	}
	if err := seedProviderModels(tx, []providerModelRow{
		{ProviderID: "prd_ark", UpstreamModelID: "seedance-1-0-ark", DisplayName: "Seedance", UnitCosts: videoCost},
		{ProviderID: "prd_or", UpstreamModelID: "bytedance/seedance-1.0", DisplayName: "Seedance", UnitCosts: videoCost},
		{ProviderID: "prd_ark", UpstreamModelID: "image-demo-ark", DisplayName: "Image Demo", UnitCosts: imageCost},
	}); err != nil {
		return err
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
		{ChannelOrgID: identity.OfficialChannelID, PublicModelID: "mdl_seedance", Enabled: true, Wholesale: videoWholesale},
		{ChannelOrgID: identity.ResellerChannelID, PublicModelID: "mdl_seedance", Enabled: true, Wholesale: videoWholesale},
		{ChannelOrgID: identity.OEMChannelID, PublicModelID: "mdl_seedance", Enabled: true, Wholesale: videoWholesale},
		{ChannelOrgID: identity.OfficialChannelID, PublicModelID: "mdl_image", Enabled: true, Wholesale: imageWholesale},
		{ChannelOrgID: identity.ResellerChannelID, PublicModelID: "mdl_image", Enabled: true, Wholesale: imageWholesale},
		{ChannelOrgID: identity.OEMChannelID, PublicModelID: "mdl_image", Enabled: true, Wholesale: imageWholesale},
	}
	return seedChannelPolicies(tx, policies)
}

func seedGeminiCatalog(tx *gorm.DB, caps, price []byte) error {
	textCost, _ := json.Marshal(map[string]string{"input": "0.0000004", "output": "0.0000008"})
	textWholesale, _ := json.Marshal(map[string]string{"input": "0.0000007", "output": "0.0000014"})
	if err := tx.Where("slug = ?", GeminiProvider).FirstOrCreate(&providerRow{
		ID: "prd_gemini", Name: "Google Gemini Flash", Slug: GeminiProvider, Kind: "direct",
		Adapter: "gemini", Status: "active", Health: "available", TestBehavior: "ok",
		Priority: 80, Weight: 1, TimeoutMS: 30000, RetryMax: 1, CapabilityTags: "text,gemini",
	}).Error; err != nil {
		return err
	}
	if err := tx.Where("public_id = ?", GeminiModelID).FirstOrCreate(&publicModelRow{
		ID: "mdl_gemini", PublicID: GeminiModelID, Vendor: "google", DisplayName: "Gemini Flash",
		Capabilities: caps, Status: "published", SyncState: SyncPublished,
	}).Error; err != nil {
		return err
	}
	if err := tx.Model(&publicModelRow{}).Where("public_id = ?", GeminiModelID).Update("capabilities_json", caps).Error; err != nil {
		return err
	}
	if err := tx.Where("id = ?", "map_gemini").FirstOrCreate(&mappingRow{
		ID: "map_gemini", PublicModelID: "mdl_gemini", ProviderID: "prd_gemini",
		UpstreamModelID: "gemini-2.0-flash", Status: "active", SyncState: SyncPublished,
	}).Error; err != nil {
		return err
	}
	if err := seedProviderModels(tx, []providerModelRow{{ProviderID: "prd_gemini", UpstreamModelID: "gemini-2.0-flash", DisplayName: "Gemini Flash", UnitCosts: textCost}}); err != nil {
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
	policies := []channelPolicyRow{}
	for _, channelID := range []string{identity.OfficialChannelID, identity.ResellerChannelID, identity.OEMChannelID} {
		policies = append(policies, channelPolicyRow{ChannelOrgID: channelID, PublicModelID: "mdl_gemini", Enabled: true, Wholesale: textWholesale})
	}
	return seedChannelPolicies(tx, policies)
}

// GrantDefaultModels 给新渠道复制官方已启用的模型白名单，用户才能聊天/做媒体。
func (s *Service) GrantDefaultModels(ctx context.Context, channelOrgID string) error {
	return s.GrantModelsFrom(ctx, channelOrgID, identity.OfficialChannelID)
}

func (s *Service) GrantModelsFrom(ctx context.Context, channelOrgID, sourceChannelID string) error {
	if channelOrgID == "" || channelOrgID == identity.OfficialChannelID {
		return nil
	}
	var src []channelPolicyRow
	if err := s.db.WithContext(ctx).Where("channel_org_id = ? AND enabled = true", sourceChannelID).Find(&src).Error; err != nil {
		return err
	}
	for _, policy := range src {
		row := channelPolicyRow{ChannelOrgID: channelOrgID, PublicModelID: policy.PublicModelID, Enabled: true, Wholesale: policy.Wholesale, Override: policy.Override}
		if err := s.db.WithContext(ctx).
			Where("channel_org_id = ? AND public_model_id = ?", row.ChannelOrgID, row.PublicModelID).
			FirstOrCreate(&row).Error; err != nil {
			return err
		}
	}
	return nil
}

// ListChannelModels 返回该租户的模型授权。includeCatalog 时带上平台目录里尚未授权的模型，供平台勾选；租户视角只看已有白名单。不含提供商凭据。
func (s *Service) ListChannelModels(ctx context.Context, channelOrgID string, includeCatalog bool) ([]ChannelModelView, error) {
	if channelOrgID == "" {
		return []ChannelModelView{}, nil
	}
	type row struct {
		PublicID    string `gorm:"column:public_id"`
		DisplayName string `gorm:"column:display_name"`
		Vendor      string `gorm:"column:vendor"`
		Kind        string `gorm:"column:kind"`
		Status      string `gorm:"column:status"`
		Enabled     bool   `gorm:"column:enabled"`
		HasPolicy   bool   `gorm:"column:has_policy"`
		Wholesale   []byte `gorm:"column:wholesale_json"`
		Override    []byte `gorm:"column:customer_override_json"`
	}
	var rows []row
	q := s.db.WithContext(ctx)
	if includeCatalog {
		q = q.Table("catalog_public_models m").
			Select("m.public_id, m.display_name, m.vendor, m.capabilities_json->>'kind' AS kind, m.status, COALESCE(p.enabled, false) AS enabled, (p.public_model_id IS NOT NULL) AS has_policy, p.wholesale_json, p.customer_override_json").
			Joins("LEFT JOIN catalog_channel_model_policies p ON p.public_model_id = m.id AND p.channel_org_id = ?", channelOrgID).
			Where("(m.status = 'published' AND " + routeReadySQL + ") OR p.public_model_id IS NOT NULL")
	} else {
		q = q.Table("catalog_channel_model_policies p").
			Select("m.public_id, m.display_name, m.vendor, m.capabilities_json->>'kind' AS kind, m.status, p.enabled, p.wholesale_json, p.customer_override_json").
			Joins("JOIN catalog_public_models m ON m.id = p.public_model_id").
			Where("p.channel_org_id = ?", channelOrgID)
	}
	if err := q.Order("m.public_id").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]ChannelModelView, 0, len(rows))
	for _, item := range rows {
		if includeCatalog && !item.HasPolicy {
			var model publicModelRow
			if err := s.db.WithContext(ctx).Where("public_id = ?", item.PublicID).First(&model).Error; err != nil {
				return nil, err
			}
			if s.validateModelReady(ctx, model) != nil {
				continue
			}
		}
		out = append(out, ChannelModelView{
			PublicID: item.PublicID, DisplayName: item.DisplayName, Vendor: item.Vendor, Kind: item.Kind, Status: item.Status, Enabled: item.Enabled,
			Wholesale: decodeCosts(item.Wholesale), CustomerOverride: decodeCosts(item.Override),
		})
	}
	return out, nil
}

// SetChannelModels 只授权平台目录里已有的公开模型。不会创建提供商或新模型。
func (s *Service) SetChannelModels(ctx context.Context, channelOrgID string, grants []ChannelModelGrant) error {
	if channelOrgID == "" || len(grants) == 0 {
		return ErrInvalidInput
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, grant := range grants {
			publicID := strings.TrimSpace(grant.PublicID)
			if publicID == "" {
				return ErrInvalidInput
			}
			var model publicModelRow
			if err := tx.Where("public_id = ?", publicID).First(&model).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return ErrUnknownModel
				}
				return err
			}
			var existing channelPolicyRow
			err := tx.Where("channel_org_id = ? AND public_model_id = ?", channelOrgID, model.ID).First(&existing).Error
			lookupErr := err
			wholesale := decodeCosts(existing.Wholesale)
			if grant.Wholesale != nil {
				var priceErr error
				wholesale, priceErr = validateUnitCosts(grant.Wholesale)
				if priceErr != nil {
					return priceErr
				}
			}
			override := decodeCosts(existing.Override)
			if grant.CustomerOverride != nil {
				var priceErr error
				override, priceErr = validateUnitCosts(grant.CustomerOverride)
				if priceErr != nil || (len(override) > 0 && !pricedForKind(override, modelKind(model))) {
					return ErrInvalidInput
				}
			}
			if grant.Enabled && !pricedForKind(wholesale, modelKind(model)) {
				return ErrInvalidInput
			}
			wholeJSON, _ := json.Marshal(wholesale)
			overrideJSON, _ := json.Marshal(override)
			if grant.Enabled && (errors.Is(lookupErr, gorm.ErrRecordNotFound) || !existing.Enabled) {
				if model.Status != SyncPublished {
					return ErrModelNotVisible
				}
				if s.validateModelReady(ctx, model) != nil {
					return ErrModelNotVisible
				}
				ready, readyErr := s.routeReady(ctx, model.ID)
				if readyErr != nil {
					return readyErr
				}
				if !ready {
					return ErrModelNotVisible
				}
			}
			if errors.Is(lookupErr, gorm.ErrRecordNotFound) {
				if err := tx.Create(&channelPolicyRow{ChannelOrgID: channelOrgID, PublicModelID: model.ID, Enabled: grant.Enabled, Wholesale: wholeJSON, Override: overrideJSON}).Error; err != nil {
					return err
				}
				continue
			}
			if lookupErr != nil {
				return lookupErr
			}
			if err := tx.Model(&existing).Updates(map[string]any{"enabled": grant.Enabled, "wholesale_json": wholeJSON, "customer_override_json": overrideJSON}).Error; err != nil {
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
		Where("m.status = ? AND p.channel_org_id = ?", "published", channelOrgID).
		Where(routeReadySQL)
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
		if !view.ConfigReady {
			continue
		}
		out = append(out, *view)
	}
	return out, nil
}

func (s *Service) GetVisibleModel(ctx context.Context, channelOrgID, publicID string, allowlist []string) (*ModelView, error) {
	if _, err := s.loadModel(ctx, publicID); err != nil {
		return nil, err
	}
	models, err := s.ListVisibleModels(ctx, channelOrgID, allowlist)
	if err != nil {
		return nil, err
	}
	for i := range models {
		if models[i].ID == publicID {
			return &models[i], nil
		}
	}
	return nil, ErrModelNotVisible
}

func (s *Service) ResolveRoute(ctx context.Context, publicID string, hint RouteHint) ([]RouteCandidate, error) {
	var model publicModelRow
	if err := s.db.WithContext(ctx).Where("public_id = ?", publicID).First(&model).Error; err != nil {
		return nil, err
	}
	if model.Status != SyncPublished || s.validateModelReady(ctx, model) != nil {
		return nil, ErrModelNotVisible
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
		accountID, skipAccount := s.pickAccount(ctx, provider.ID, model.PublicID)
		if skipAccount {
			continue
		}
		costs, costErr := pricedProviderModelTx(s.db.WithContext(ctx), provider.ID, mapping.UpstreamModelID, modelKind(model))
		if costErr != nil {
			continue
		}
		out = append(out, RouteCandidate{
			ProviderID: provider.ID, ProviderSlug: provider.Slug, Adapter: provider.Adapter,
			BaseURL: provider.BaseURL, UpstreamModelID: mapping.UpstreamModelID, TestBehavior: provider.TestBehavior, Health: provider.Health,
			Priority: cand.Priority, Weight: weight, CostMinor: costMinor(costs), UnitCosts: costs,
			AccountID: accountID, TimeoutMS: provider.TimeoutMS,
		})
	}
	applyStrategy(out, group.Strategy)
	if len(hint.Order) > 0 {
		out = orderBy(out, hint.Order)
	}
	return out, nil
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
	return &PriceSnapshot{VersionID: price.ID, PublicID: model.PublicID, Raw: price.UnitPrices, EffectiveAt: price.EffectiveAt}, nil
}

func (s *Service) ListPriceBooks(ctx context.Context) ([]PriceBookView, error) {
	var rows []priceRow
	if err := s.db.WithContext(ctx).Order("effective_at DESC").Limit(200).Find(&rows).Error; err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(rows))
	seen := map[string]bool{}
	for _, row := range rows {
		if !seen[row.PublicModelID] {
			seen[row.PublicModelID] = true
			ids = append(ids, row.PublicModelID)
		}
	}
	names := map[string]string{}
	if len(ids) > 0 {
		var models []publicModelRow
		if err := s.db.WithContext(ctx).Where("id IN ?", ids).Find(&models).Error; err != nil {
			return nil, err
		}
		for _, model := range models {
			names[model.ID] = model.PublicID
		}
	}
	out := make([]PriceBookView, 0, len(rows))
	for _, row := range rows {
		out = append(out, priceBookFromRow(row, names[row.PublicModelID]))
	}
	return out, nil
}

func (s *Service) PublishPrice(ctx context.Context, publicID string, unitPrices map[string]any) (*PriceSnapshot, error) {
	incoming, err := NormalizeUnitPrices(unitPrices)
	if err != nil {
		return nil, err
	}
	var model publicModelRow
	if err := s.db.WithContext(ctx).Where("public_id = ?", publicID).First(&model).Error; err != nil {
		return nil, err
	}
	var created priceRow
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		merged := incoming
		var current priceRow
		if err := tx.Where("public_model_id = ? AND provider_id IS NULL AND status = ?", model.ID, "published").
			Order("effective_at DESC").First(&current).Error; err == nil {
			prev := map[string]any{}
			_ = json.Unmarshal(current.UnitPrices, &prev)
			merged = mergeUnitPrices(prev, incoming)
		}
		body, err := json.Marshal(merged)
		if err != nil {
			return err
		}
		if err := tx.Model(&priceRow{}).Where("public_model_id = ? AND provider_id IS NULL AND status = ?", model.ID, "published").
			Update("status", "superseded").Error; err != nil {
			return err
		}
		created = priceRow{
			ID: id.New("prc"), PublicModelID: model.ID, UnitPrices: body,
			Status: "published", EffectiveAt: time.Now().UTC(),
		}
		return tx.Create(&created).Error
	})
	if err != nil {
		return nil, err
	}
	return &PriceSnapshot{VersionID: created.ID, PublicID: model.PublicID, Raw: created.UnitPrices, EffectiveAt: created.EffectiveAt}, nil
}

func (s *Service) MarkHealth(ctx context.Context, providerID, health string) error {
	return s.db.WithContext(ctx).Model(&providerRow{}).Where("id = ?", providerID).Update("health", health).Error
}

type MappedModelView struct {
	PublicID        string `json:"public_id"`
	Vendor          string `json:"vendor"`
	DisplayName     string `json:"display_name"`
	UpstreamModelID string `json:"upstream_model_id"`
	Status          string `json:"status"`
}

type ProviderView struct {
	ID               string            `json:"id"`
	Name             string            `json:"name"`
	Slug             string            `json:"slug"`
	Kind             string            `json:"kind"`
	Adapter          string            `json:"adapter"`
	BaseURL          string            `json:"base_url,omitempty"`
	Region           string            `json:"region,omitempty"`
	Health           string            `json:"health"`
	Status           string            `json:"status"`
	Priority         int               `json:"priority"`
	Weight           int               `json:"weight"`
	TimeoutMS        int               `json:"timeout_ms"`
	RetryMax         int               `json:"retry_max"`
	RPMLimit         int               `json:"rpm_limit"`
	ConcurrencyLimit int               `json:"concurrency_limit"`
	CapabilityTags   string            `json:"capability_tags,omitempty"`
	TestBehavior     string            `json:"test_behavior,omitempty"`
	AccountCount     int               `json:"account_count"`
	Models           []MappedModelView `json:"models"`
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
	maps, err := s.loadMappedModels(ctx)
	if err != nil {
		return nil, err
	}
	out = attachMappedModels(out, maps)
	s.attachAccountCounts(ctx, out)
	return out, nil
}

func (s *Service) attachAccountCounts(ctx context.Context, items []ProviderView) {
	if len(items) == 0 {
		return
	}
	type countRow struct {
		ProviderID string `gorm:"column:provider_id"`
		N          int64  `gorm:"column:n"`
	}
	var counts []countRow
	_ = s.db.WithContext(ctx).Model(&accountRow{}).
		Select("provider_id, COUNT(*) AS n").
		Where("status <> ?", AccountRotated).
		Group("provider_id").
		Scan(&counts).Error
	byID := map[string]int{}
	for _, row := range counts {
		byID[row.ProviderID] = int(row.N)
	}
	for i := range items {
		items[i].AccountCount = byID[items[i].ID]
	}
}

var ErrProbeUnsupported = errors.New("active upstream probe not implemented")

func (s *Service) Probe(ctx context.Context, providerID string) (string, error) {
	var provider providerRow
	if err := s.db.WithContext(ctx).Where("id = ?", providerID).First(&provider).Error; err != nil {
		return "", err
	}
	if provider.Adapter != "test" {
		return "", ErrProbeUnsupported
	}
	health := "available"
	switch provider.TestBehavior {
	case "down":
		health = "unavailable"
	case "429", "degraded":
		health = "degraded"
	}
	if err := s.MarkHealth(ctx, provider.ID, health); err != nil {
		return "", err
	}
	return health, nil
}

func (s *Service) modelView(ctx context.Context, model publicModelRow) (*ModelView, error) {
	caps := map[string]any{}
	_ = json.Unmarshal(model.Capabilities, &caps)
	var price priceRow
	sell := map[string]any{}
	if err := s.db.WithContext(ctx).Where("public_model_id = ? AND provider_id IS NULL AND status = ?", model.ID, "published").
		Order("effective_at DESC").First(&price).Error; err == nil {
		_ = json.Unmarshal(price.UnitPrices, &sell)
	}
	var slugs []string
	_ = s.db.WithContext(ctx).Table("catalog_provider_model_mappings m").
		Select("p.slug").
		Joins("JOIN catalog_providers p ON p.id = m.provider_id").
		Where("m.public_model_id = ? AND m.status = ?", model.ID, "active").
		Scan(&slugs).Error
	syncState := EffectiveSyncState(model.Status, model.SyncState)
	desc, kind, ctxLen, maxTok := extraFromCaps(caps)
	view := &ModelView{
		ID: model.PublicID, Vendor: model.Vendor, DisplayName: model.DisplayName, Capabilities: caps,
		SellPrice: publicSell(sell), Providers: slugs, Status: model.Status, ConfigReady: modelConfigurationReady(model, caps, sell), SyncState: syncState,
		CreatedByUserID: model.CreatedByUserID, ReviewedByUserID: model.ReviewedByUserID,
		Description: desc, Kind: kind, ContextLength: ctxLen, MaxCompletionTokens: maxTok,
	}
	view.Kind = InferKind(*view)
	return view, nil
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
