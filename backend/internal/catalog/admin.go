package catalog

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"errors"

	"gorm.io/gorm"

	"github.com/tokpath/tokstudio/backend/internal/platform/crypto"
	"github.com/tokpath/tokstudio/backend/internal/platform/id"
)

var (
	ErrInvalidInput    = errors.New("invalid catalog input")
	ErrUnknownModel    = errors.New("model not in platform catalog")
	ErrModelNotVisible = errors.New("model not visible to tenant")
	ErrNotReviewed     = errors.New("model not reviewed")
	ErrRejected        = errors.New("model rejected")
	ErrSameActor       = errors.New("creator cannot review or publish")
)

type ChannelModelGrant struct {
	PublicID string `json:"public_id"`
	Enabled  bool   `json:"enabled"`
}

type ProviderInput struct {
	Name             string `json:"name"`
	Slug             string `json:"slug"`
	Kind             string `json:"kind"`
	Adapter          string `json:"adapter"`
	BaseURL          string `json:"base_url"`
	Region           string `json:"region"`
	Status           string `json:"status"`
	TestBehavior     string `json:"test_behavior"`
	Priority         int    `json:"priority"`
	Weight           int    `json:"weight"`
	TimeoutMS        int    `json:"timeout_ms"`
	RetryMax         int    `json:"retry_max"`
	RPMLimit         int    `json:"rpm_limit"`
	ConcurrencyLimit int    `json:"concurrency_limit"`
	CapabilityTags   string `json:"capability_tags"`
	CredentialRef    string `json:"credential_ref"`
	Health           string `json:"health"`
}

type ModelInput struct {
	PublicID     string         `json:"public_id"`
	Vendor       string         `json:"vendor"`
	DisplayName  string         `json:"display_name"`
	Status       string         `json:"status"`
	Capabilities map[string]any `json:"capabilities"`
}

type RouteInput struct {
	PublicModelID string             `json:"public_model_id"`
	Strategy      string             `json:"strategy"`
	Status        string             `json:"status"`
	Candidates    []RouteCandidateIn `json:"candidates"`
}

type RouteCandidateIn struct {
	ProviderID string `json:"provider_id"`
	Priority   int    `json:"priority"`
	Weight     int    `json:"weight"`
}

type RouteView struct {
	ID            string           `json:"id"`
	PublicModelID string           `json:"public_model_id"`
	Vendor        string           `json:"vendor,omitempty"`
	Strategy      string           `json:"strategy"`
	Status        string           `json:"status"`
	Candidates    []map[string]any `json:"candidates"`
}

func (s *Service) GetProvider(ctx context.Context, id string) (*ProviderView, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, gorm.ErrRecordNotFound
	}
	var row providerRow
	if err := s.db.WithContext(ctx).Where("id = ? OR slug = ?", id, id).First(&row).Error; err != nil {
		return nil, err
	}
	view := providerView(row)
	maps, err := s.loadMappedModels(ctx, row.ID)
	if err != nil {
		return nil, err
	}
	attached := attachMappedModels([]ProviderView{*view}, maps)
	s.attachAccountCounts(ctx, attached)
	return &attached[0], nil
}

func (s *Service) CreateProvider(ctx context.Context, in ProviderInput) (*ProviderView, error) {
	in = normalizeProvider(in)
	if err := ValidateUpstreamURL(in.BaseURL, s.production, s.allowHosts); err != nil {
		return nil, err
	}
	row := providerRow{
		ID: id.New("prd"), Name: in.Name, Slug: in.Slug, Kind: in.Kind, Adapter: in.Adapter,
		BaseURL: in.BaseURL, Region: in.Region, Status: in.Status, Health: "available",
		TestBehavior: in.TestBehavior, Priority: in.Priority, Weight: in.Weight,
		TimeoutMS: in.TimeoutMS, RetryMax: in.RetryMax, RPMLimit: in.RPMLimit,
		ConcurrencyLimit: in.ConcurrencyLimit, CapabilityTags: in.CapabilityTags, CredentialRef: in.CredentialRef,
	}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return nil, err
	}
	return providerView(row), nil
}

func (s *Service) PatchProvider(ctx context.Context, id string, in ProviderInput) (*ProviderView, error) {
	var row providerRow
	if err := s.db.WithContext(ctx).Where("id = ?", id).First(&row).Error; err != nil {
		return nil, err
	}
	updates := map[string]any{}
	if in.Name != "" {
		updates["name"] = in.Name
	}
	if in.Kind != "" {
		updates["kind"] = in.Kind
	}
	if in.Adapter != "" {
		updates["adapter"] = in.Adapter
	}
	if in.BaseURL != "" {
		if err := ValidateUpstreamURL(in.BaseURL, s.production, s.allowHosts); err != nil {
			return nil, err
		}
		updates["base_url"] = in.BaseURL
	}
	if in.Region != "" {
		updates["region"] = in.Region
	}
	if in.Status != "" {
		updates["status"] = in.Status
	}
	if in.TestBehavior != "" {
		updates["test_behavior"] = in.TestBehavior
	}
	if in.Priority != 0 {
		updates["priority"] = in.Priority
	}
	if in.Weight != 0 {
		updates["weight"] = in.Weight
	}
	if in.TimeoutMS != 0 {
		updates["timeout_ms"] = in.TimeoutMS
	}
	if in.RetryMax != 0 {
		updates["retry_max"] = in.RetryMax
	}
	if in.RPMLimit != 0 {
		updates["rpm_limit"] = in.RPMLimit
	}
	if in.ConcurrencyLimit != 0 {
		updates["concurrency_limit"] = in.ConcurrencyLimit
	}
	if in.CapabilityTags != "" {
		updates["capability_tags"] = in.CapabilityTags
	}
	if in.CredentialRef != "" {
		updates["credential_ref"] = in.CredentialRef
	}
	if in.Health != "" {
		updates["health"] = in.Health
	}
	if len(updates) > 0 {
		if err := s.db.WithContext(ctx).Model(&providerRow{}).Where("id = ?", id).Updates(updates).Error; err != nil {
			return nil, err
		}
	}
	if err := s.db.WithContext(ctx).Where("id = ?", id).First(&row).Error; err != nil {
		return nil, err
	}
	return providerView(row), nil
}

func (s *Service) RotateCredential(ctx context.Context, providerID, secret, encKey string) (string, error) {
	var row providerRow
	if err := s.db.WithContext(ctx).Where("id = ?", providerID).First(&row).Error; err != nil {
		return "", err
	}
	sealed, err := crypto.Seal(encKey, secret)
	if err != nil {
		return "", err
	}
	credID := id.New("crd")
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`UPDATE catalog_provider_credentials SET status = 'rotated' WHERE provider_id = ?`, providerID).Error; err != nil {
			return err
		}
		if err := tx.Exec(
			`INSERT INTO catalog_provider_credentials(id, provider_id, ciphertext, key_hash, status, created_at) VALUES (?, ?, ?, ?, 'active', ?)`,
			credID, providerID, sealed, crypto.HashToken(secret), time.Now().UTC(),
		).Error; err != nil {
			return err
		}
		return tx.Model(&providerRow{}).Where("id = ?", providerID).Update("credential_ref", credID).Error
	}); err != nil {
		return "", err
	}
	return credID, nil
}

func (s *Service) ListAdminModels(ctx context.Context) ([]ModelView, error) {
	var models []publicModelRow
	if err := s.db.WithContext(ctx).Order("public_id").Find(&models).Error; err != nil {
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

func (s *Service) GetAdminModel(ctx context.Context, publicID string) (*ModelView, error) {
	model, err := s.loadModel(ctx, publicID)
	if err != nil {
		return nil, err
	}
	return s.modelView(ctx, *model)
}

func (s *Service) CreateModel(ctx context.Context, in ModelInput, actorUserID string) (*ModelView, error) {
	if in.PublicID == "" || in.Vendor == "" {
		return nil, ErrInvalidInput
	}
	if in.DisplayName == "" {
		in.DisplayName = in.PublicID
	}
	caps, _ := json.Marshal(in.Capabilities)
	if in.Capabilities == nil {
		caps = []byte(`{}`)
	}
	row := publicModelRow{
		ID: id.New("mdl"), PublicID: in.PublicID, Vendor: in.Vendor, DisplayName: in.DisplayName,
		Capabilities: caps, Status: SyncDraft, SyncState: SyncDraft, CreatedByUserID: strings.TrimSpace(actorUserID),
	}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return nil, err
	}
	return s.modelView(ctx, row)
}

func (s *Service) PatchModel(ctx context.Context, publicID string, in ModelInput) (*ModelView, error) {
	var row publicModelRow
	if err := s.db.WithContext(ctx).Where("id = ? OR public_id = ?", publicID, publicID).First(&row).Error; err != nil {
		return nil, err
	}
	updates := map[string]any{}
	if in.DisplayName != "" {
		updates["display_name"] = in.DisplayName
	}
	if in.Vendor != "" {
		updates["vendor"] = in.Vendor
	}
	if in.Capabilities != nil {
		body, _ := json.Marshal(in.Capabilities)
		updates["capabilities_json"] = body
	}
	if len(updates) > 0 {
		if err := s.db.WithContext(ctx).Model(&publicModelRow{}).Where("id = ?", row.ID).Updates(updates).Error; err != nil {
			return nil, err
		}
	}
	if err := s.db.WithContext(ctx).Where("id = ?", row.ID).First(&row).Error; err != nil {
		return nil, err
	}
	return s.modelView(ctx, row)
}

func (s *Service) ListRoutes(ctx context.Context) ([]RouteView, error) {
	var groups []routeGroupRow
	if err := s.db.WithContext(ctx).Order("id").Find(&groups).Error; err != nil {
		return nil, err
	}
	out := make([]RouteView, 0, len(groups))
	for _, group := range groups {
		var model publicModelRow
		_ = s.db.WithContext(ctx).Where("id = ?", group.PublicModelID).First(&model).Error
		var cands []candidateRow
		_ = s.db.WithContext(ctx).Where("route_group_id = ?", group.ID).Order("priority").Find(&cands).Error
		items := make([]map[string]any, 0, len(cands))
		for _, cand := range cands {
			var provider providerRow
			_ = s.db.WithContext(ctx).Where("id = ?", cand.ProviderID).First(&provider).Error
			items = append(items, map[string]any{
				"provider_id": cand.ProviderID, "provider_slug": provider.Slug,
				"priority": cand.Priority, "weight": cand.Weight,
			})
		}
		publicID := model.PublicID
		if publicID == "" {
			publicID = group.PublicModelID
		}
		vendor := model.Vendor
		if vendor == "" {
			if i := strings.IndexByte(publicID, '/'); i > 0 {
				vendor = publicID[:i]
			}
		}
		out = append(out, RouteView{ID: group.ID, PublicModelID: publicID, Vendor: vendor, Strategy: group.Strategy, Status: group.Status, Candidates: items})
	}
	return out, nil
}

// AttachProvider 把一个 Provider 挂到已有公开模型：写 mapping，并追加到现有路由组。
func (s *Service) AttachProvider(ctx context.Context, publicID, providerID, upstream string) error {
	if publicID == "" || providerID == "" {
		return ErrInvalidInput
	}
	if upstream == "" {
		upstream = publicID
	}
	var model publicModelRow
	if err := s.db.WithContext(ctx).Where("id = ? OR public_id = ?", publicID, publicID).First(&model).Error; err != nil {
		return err
	}
	var provider providerRow
	if err := s.db.WithContext(ctx).Where("id = ? OR slug = ?", providerID, providerID).First(&provider).Error; err != nil {
		return err
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		mapping := mappingRow{
			ID: id.New("map"), PublicModelID: model.ID, ProviderID: provider.ID,
			UpstreamModelID: upstream, Status: "active", SyncState: SyncPublished,
		}
		if err := tx.Where("public_model_id = ? AND provider_id = ?", model.ID, provider.ID).FirstOrCreate(&mapping).Error; err != nil {
			return err
		}
		if mapping.Status != "active" || mapping.UpstreamModelID != upstream {
			if err := tx.Model(&mappingRow{}).Where("id = ?", mapping.ID).Updates(map[string]any{
				"status": "active", "upstream_model_id": upstream,
			}).Error; err != nil {
				return err
			}
		}
		var group routeGroupRow
		if err := tx.Where("public_model_id = ? AND status = ?", model.ID, "active").First(&group).Error; err != nil {
			return nil
		}
		var existing candidateRow
		if err := tx.Where("route_group_id = ? AND provider_id = ?", group.ID, provider.ID).First(&existing).Error; err == nil {
			return nil
		}
		var max struct{ Priority int }
		_ = tx.Model(&candidateRow{}).Where("route_group_id = ?", group.ID).Select("COALESCE(MAX(priority),0) AS priority").Scan(&max).Error
		return tx.Create(&candidateRow{RouteGroupID: group.ID, ProviderID: provider.ID, Priority: max.Priority + 1}).Error
	})
}

func (s *Service) CreateRoute(ctx context.Context, in RouteInput) (*RouteView, error) {
	var model publicModelRow
	if err := s.db.WithContext(ctx).Where("id = ? OR public_id = ?", in.PublicModelID, in.PublicModelID).First(&model).Error; err != nil {
		return nil, err
	}
	if in.Strategy == "" {
		in.Strategy = "priority"
	}
	if in.Status == "" {
		in.Status = "active"
	}
	group := routeGroupRow{ID: id.New("rg"), PublicModelID: model.ID, Strategy: in.Strategy, Status: in.Status}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&group).Error; err != nil {
			return err
		}
		for i, cand := range in.Candidates {
			var provider providerRow
			if err := tx.Where("id = ? OR slug = ?", cand.ProviderID, cand.ProviderID).First(&provider).Error; err != nil {
				return err
			}
			priority := cand.Priority
			if priority == 0 {
				priority = i + 1
			}
			weight := cand.Weight
			if weight == 0 {
				weight = 1
			}
			if err := tx.Create(&candidateRow{RouteGroupID: group.ID, ProviderID: provider.ID, Priority: priority, Weight: weight}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	routes, err := s.ListRoutes(ctx)
	if err != nil {
		return nil, err
	}
	for i := range routes {
		if routes[i].ID == group.ID {
			return &routes[i], nil
		}
	}
	return &RouteView{ID: group.ID, PublicModelID: model.PublicID, Strategy: group.Strategy, Status: group.Status}, nil
}

func (s *Service) PatchRoute(ctx context.Context, routeID string, in RouteInput) (*RouteView, error) {
	var group routeGroupRow
	if err := s.db.WithContext(ctx).Where("id = ?", routeID).First(&group).Error; err != nil {
		return nil, err
	}
	updates := map[string]any{}
	if in.Strategy != "" {
		updates["strategy"] = in.Strategy
	}
	if in.Status != "" {
		updates["status"] = in.Status
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if len(updates) > 0 {
			if err := tx.Model(&routeGroupRow{}).Where("id = ?", routeID).Updates(updates).Error; err != nil {
				return err
			}
		}
		if in.Candidates != nil {
			if err := tx.Where("route_group_id = ?", routeID).Delete(&candidateRow{}).Error; err != nil {
				return err
			}
			for i, cand := range in.Candidates {
				var provider providerRow
				if err := tx.Where("id = ? OR slug = ?", cand.ProviderID, cand.ProviderID).First(&provider).Error; err != nil {
					return err
				}
				priority := cand.Priority
				if priority == 0 {
					priority = i + 1
				}
				weight := cand.Weight
				if weight == 0 {
					weight = 1
				}
				if err := tx.Create(&candidateRow{RouteGroupID: routeID, ProviderID: provider.ID, Priority: priority, Weight: weight}).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	routes, err := s.ListRoutes(ctx)
	if err != nil {
		return nil, err
	}
	for i := range routes {
		if routes[i].ID == routeID {
			return &routes[i], nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func normalizeProvider(in ProviderInput) ProviderInput {
	in.Slug = strings.TrimSpace(in.Slug)
	in.Name = strings.TrimSpace(in.Name)
	if in.Kind == "" {
		in.Kind = "direct"
	}
	if in.Adapter == "" {
		in.Adapter = "test"
	}
	if in.Status == "" {
		in.Status = "active"
	}
	if in.TestBehavior == "" {
		in.TestBehavior = "ok"
	}
	if in.Priority == 0 {
		in.Priority = 100
	}
	if in.Weight == 0 {
		in.Weight = 1
	}
	if in.TimeoutMS == 0 {
		in.TimeoutMS = 30000
	}
	if in.RetryMax == 0 {
		in.RetryMax = 1
	}
	return in
}

func providerView(row providerRow) *ProviderView {
	return &ProviderView{
		ID: row.ID, Name: row.Name, Slug: row.Slug, Kind: row.Kind, Adapter: row.Adapter,
		BaseURL: row.BaseURL, Region: row.Region, Health: row.Health, Status: row.Status,
		Priority: row.Priority, Weight: row.Weight, TimeoutMS: row.TimeoutMS, RetryMax: row.RetryMax,
		RPMLimit: row.RPMLimit, ConcurrencyLimit: row.ConcurrencyLimit, CapabilityTags: row.CapabilityTags,
		CredentialRef: row.CredentialRef, TestBehavior: row.TestBehavior, Models: []MappedModelView{},
	}
}

func (s *Service) loadMappedModels(ctx context.Context, providerIDs ...string) ([]mappedModelScan, error) {
	q := s.db.WithContext(ctx).Table("catalog_provider_model_mappings AS m").
		Select("m.provider_id, pm.public_id, pm.vendor, pm.display_name, m.upstream_model_id, m.status").
		Joins("JOIN catalog_public_models AS pm ON pm.id = m.public_model_id").
		Order("pm.public_id ASC")
	if len(providerIDs) > 0 {
		q = q.Where("m.provider_id IN ?", providerIDs)
	}
	var rows []mappedModelScan
	if err := q.Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}
