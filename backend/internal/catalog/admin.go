package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/tokpath/tokstudio/backend/internal/platform/id"
)

var (
	ErrInvalidInput    = errors.New("invalid catalog input")
	ErrUnknownModel    = errors.New("model not in platform catalog")
	ErrModelNotVisible = errors.New("model not visible to tenant")
	ErrModelIncomplete = errors.New("model configuration incomplete")
	ErrModelExists     = errors.New("model already exists")
	ErrRouteIncomplete = errors.New("route configuration incomplete")
	ErrRouteExists     = errors.New("route already exists for model")
)

type ChannelModelGrant struct {
	PublicID         string            `json:"public_id"`
	Enabled          bool              `json:"enabled"`
	Wholesale        map[string]string `json:"wholesale,omitempty"`
	CustomerOverride map[string]string `json:"customer_override,omitempty"`
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
	Health           string `json:"health"`
}

type ModelInput struct {
	PublicID     string         `json:"public_id"`
	Vendor       string         `json:"vendor"`
	DisplayName  string         `json:"display_name"`
	Status       string         `json:"status"`
	Capabilities map[string]any `json:"capabilities"`
	InitialPrice map[string]any `json:"initial_price,omitempty"`
}

type RouteInput struct {
	PublicModelID string             `json:"public_model_id"`
	Strategy      string             `json:"strategy"`
	Status        string             `json:"status"`
	Candidates    []RouteCandidateIn `json:"candidates"`
}

type RouteCandidateIn struct {
	ProviderID      string `json:"provider_id"`
	UpstreamModelID string `json:"upstream_model_id"`
	Priority        int    `json:"priority"`
	Weight          int    `json:"weight"`
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
	if in.Name == "" {
		return nil, ErrInvalidInput
	}
	if err := ValidateUpstreamURL(in.BaseURL, s.production, s.allowHosts); err != nil {
		return nil, err
	}
	providerID := id.New("prd")
	if in.Slug == "" {
		in.Slug = providerID
	}
	row := providerRow{
		ID: providerID, Name: in.Name, Slug: in.Slug, Kind: in.Kind, Adapter: in.Adapter,
		BaseURL: in.BaseURL, Region: in.Region, Status: in.Status, Health: "available",
		TestBehavior: in.TestBehavior, Priority: in.Priority, Weight: in.Weight,
		TimeoutMS: in.TimeoutMS, RetryMax: in.RetryMax, RPMLimit: in.RPMLimit,
		ConcurrencyLimit: in.ConcurrencyLimit, CapabilityTags: in.CapabilityTags,
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

func (s *Service) CreateModel(ctx context.Context, in ModelInput, actorUserID string) (*ModelView, *PriceSnapshot, error) {
	in.PublicID = strings.TrimSpace(in.PublicID)
	in.DisplayName = strings.TrimSpace(in.DisplayName)
	in.Vendor = strings.TrimSpace(in.Vendor)
	if in.Vendor == "" || (in.PublicID == "" && in.DisplayName == "") {
		return nil, nil, ErrInvalidInput
	}
	if in.PublicID == "" {
		in.PublicID = generatedModelPublicID(in.Vendor, in.DisplayName)
	}
	if in.DisplayName == "" {
		in.DisplayName = in.PublicID
	}
	var existing int64
	if err := s.db.WithContext(ctx).Model(&publicModelRow{}).Where("public_id = ?", in.PublicID).Count(&existing).Error; err != nil {
		return nil, nil, err
	}
	if existing > 0 {
		return nil, nil, ErrModelExists
	}
	caps, _ := json.Marshal(in.Capabilities)
	if in.Capabilities == nil {
		caps = []byte(`{}`)
	}
	row := publicModelRow{
		ID: id.New("mdl"), PublicID: in.PublicID, Vendor: in.Vendor, DisplayName: in.DisplayName,
		Capabilities: caps, Status: SyncDraft, SyncState: SyncDraft, CreatedByUserID: strings.TrimSpace(actorUserID),
	}
	var units map[string]any
	if in.InitialPrice != nil {
		var err error
		units, err = NormalizeUnitPrices(in.InitialPrice)
		if err != nil || !modelConfigurationReady(row, in.Capabilities, units) {
			return nil, nil, ErrModelIncomplete
		}
	}
	var price *PriceSnapshot
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		if units == nil {
			return nil
		}
		body, err := json.Marshal(units)
		if err != nil {
			return err
		}
		created := priceRow{
			ID: id.New("prc"), PublicModelID: row.ID, UnitPrices: body,
			Status: SyncPublished, EffectiveAt: time.Now().UTC(),
		}
		if err := tx.Create(&created).Error; err != nil {
			return err
		}
		price = &PriceSnapshot{VersionID: created.ID, PublicID: row.PublicID, Raw: created.UnitPrices, EffectiveAt: created.EffectiveAt}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	view, err := s.modelView(ctx, row)
	return view, price, err
}

func generatedModelPublicID(vendor, name string) string {
	vendorPart, namePart := modelIDSegment(vendor), modelIDSegment(name)
	if vendorPart == "" || namePart == "" {
		return id.New("mdl")
	}
	return vendorPart + "/" + namePart
}

func modelIDSegment(raw string) string {
	var out strings.Builder
	var separator rune
	for _, char := range strings.ToLower(strings.TrimSpace(raw)) {
		if unicode.IsLetter(char) || unicode.IsDigit(char) {
			if separator != 0 && out.Len() > 0 {
				out.WriteRune(separator)
			}
			out.WriteRune(char)
			separator = 0
		} else if char == '.' {
			separator = '.'
		} else {
			separator = '-'
		}
	}
	return out.String()
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
		if row.Status == SyncPublished {
			proposed := row
			proposed.Capabilities = body
			if in.DisplayName != "" {
				proposed.DisplayName = in.DisplayName
			}
			if in.Vendor != "" {
				proposed.Vendor = in.Vendor
			}
			if err := s.validateModelReady(ctx, proposed); err != nil {
				return nil, err
			}
		}
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
			var mapping mappingRow
			_ = s.db.WithContext(ctx).Where("public_model_id = ? AND provider_id = ?", group.PublicModelID, cand.ProviderID).First(&mapping).Error
			items = append(items, map[string]any{
				"provider_id": cand.ProviderID, "provider_slug": provider.Slug,
				"upstream_model_id": mapping.UpstreamModelID, "priority": cand.Priority, "weight": cand.Weight,
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

func (s *Service) GetRoute(ctx context.Context, routeID string) (*RouteView, error) {
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
	if _, err := pricedProviderModelTx(s.db.WithContext(ctx), provider.ID, upstream, modelKind(model)); err != nil {
		return ErrProviderModelUnpriced
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
	if model.Status != SyncPublished {
		return nil, ErrRouteIncomplete
	}
	if err := s.validateModelReady(ctx, model); err != nil {
		return nil, ErrModelIncomplete
	}
	if in.Strategy == "" {
		in.Strategy = "priority"
	}
	if in.Status == "" {
		in.Status = "inactive"
	}
	if err := validateRouteInput(in); err != nil {
		return nil, err
	}
	group := routeGroupRow{ID: id.New("rg"), PublicModelID: model.ID, Strategy: in.Strategy, Status: in.Status}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var locked publicModelRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", model.ID).First(&locked).Error; err != nil {
			return err
		}
		if locked.Status != SyncPublished {
			return ErrRouteIncomplete
		}
		if err := s.validateModelReady(ctx, locked); err != nil {
			return ErrModelIncomplete
		}
		var count int64
		if err := tx.Model(&routeGroupRow{}).Where("public_model_id = ?", model.ID).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return ErrRouteExists
		}
		if err := tx.Create(&group).Error; err != nil {
			return err
		}
		return replaceRouteCandidates(tx, group, in.Candidates)
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
	var model publicModelRow
	if err := s.db.WithContext(ctx).Where("id = ?", group.PublicModelID).First(&model).Error; err != nil {
		return nil, err
	}
	final := in
	if final.Strategy == "" {
		final.Strategy = group.Strategy
	}
	if final.Status == "" {
		final.Status = group.Status
	}
	if model.Status != SyncPublished && final.Status == "active" {
		return nil, ErrRouteIncomplete
	}
	if final.Status == "active" && s.validateModelReady(ctx, model) != nil {
		return nil, ErrModelIncomplete
	}
	if in.Candidates == nil {
		var current []candidateRow
		if err := s.db.WithContext(ctx).Where("route_group_id = ?", routeID).Order("priority").Find(&current).Error; err != nil {
			return nil, err
		}
		for _, item := range current {
			var mapping mappingRow
			_ = s.db.WithContext(ctx).Where("public_model_id = ? AND provider_id = ?", group.PublicModelID, item.ProviderID).First(&mapping).Error
			final.Candidates = append(final.Candidates, RouteCandidateIn{ProviderID: item.ProviderID, UpstreamModelID: mapping.UpstreamModelID, Priority: item.Priority, Weight: item.Weight})
		}
	}
	if err := validateRouteInput(final); err != nil {
		return nil, err
	}
	if final.Status == "active" {
		for _, cand := range final.Candidates {
			var provider providerRow
			if err := s.db.WithContext(ctx).Where("id = ? OR slug = ?", cand.ProviderID, cand.ProviderID).First(&provider).Error; err != nil {
				return nil, err
			}
			if provider.Status != "active" {
				return nil, ErrRouteIncomplete
			}
		}
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
			group.Status = final.Status
			if err := replaceRouteCandidates(tx, group, in.Candidates); err != nil {
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
		if routes[i].ID == routeID {
			return &routes[i], nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func validateRouteInput(in RouteInput) error {
	switch in.Strategy {
	case StrategyPriority, "weight", "price", "health":
	default:
		return ErrInvalidInput
	}
	if in.Status != "active" && in.Status != "inactive" {
		return ErrInvalidInput
	}
	if in.Status == "active" && len(in.Candidates) == 0 {
		return ErrRouteIncomplete
	}
	seen := map[string]bool{}
	for _, cand := range in.Candidates {
		provider := strings.TrimSpace(cand.ProviderID)
		if provider == "" || strings.TrimSpace(cand.UpstreamModelID) == "" || cand.Priority < 0 || cand.Weight < 0 || seen[provider] {
			return ErrRouteIncomplete
		}
		seen[provider] = true
	}
	return nil
}

func replaceRouteCandidates(tx *gorm.DB, group routeGroupRow, input []RouteCandidateIn) error {
	var model publicModelRow
	if err := tx.Where("id = ?", group.PublicModelID).First(&model).Error; err != nil {
		return err
	}
	if err := tx.Where("route_group_id = ?", group.ID).Delete(&candidateRow{}).Error; err != nil {
		return err
	}
	if err := tx.Model(&mappingRow{}).Where("public_model_id = ?", group.PublicModelID).Update("status", "inactive").Error; err != nil {
		return err
	}
	seen := map[string]bool{}
	for i, cand := range input {
		var provider providerRow
		if err := tx.Where("id = ? OR slug = ?", cand.ProviderID, cand.ProviderID).First(&provider).Error; err != nil {
			return err
		}
		if seen[provider.ID] {
			return ErrRouteIncomplete
		}
		seen[provider.ID] = true
		if group.Status == "active" && provider.Status != "active" {
			return ErrRouteIncomplete
		}
		if _, err := pricedProviderModelTx(tx, provider.ID, strings.TrimSpace(cand.UpstreamModelID), modelKind(model)); err != nil {
			return ErrProviderModelUnpriced
		}
		var mapping mappingRow
		err := tx.Where("public_model_id = ? AND provider_id = ?", group.PublicModelID, provider.ID).First(&mapping).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			mapping = mappingRow{ID: id.New("map"), PublicModelID: group.PublicModelID, ProviderID: provider.ID,
				UpstreamModelID: strings.TrimSpace(cand.UpstreamModelID), Status: "active", SyncState: SyncPublished}
			if err := tx.Create(&mapping).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		} else if err := tx.Model(&mappingRow{}).Where("id = ?", mapping.ID).Updates(map[string]any{
			"upstream_model_id": strings.TrimSpace(cand.UpstreamModelID), "status": "active", "sync_state": SyncPublished,
		}).Error; err != nil {
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
		TestBehavior: row.TestBehavior, Models: []MappedModelView{},
	}
}

func (s *Service) loadMappedModels(ctx context.Context, providerIDs ...string) ([]mappedModelScan, error) {
	q := s.db.WithContext(ctx).Table("catalog_provider_model_mappings AS m").
		Select("m.provider_id, pm.public_id, pm.vendor, pm.display_name, m.upstream_model_id, m.status").
		Joins("JOIN catalog_public_models AS pm ON pm.id = m.public_model_id").
		Joins("JOIN catalog_route_groups AS rg ON rg.public_model_id = pm.id").
		Joins("JOIN catalog_route_candidates AS rc ON rc.route_group_id = rg.id AND rc.provider_id = m.provider_id").
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
