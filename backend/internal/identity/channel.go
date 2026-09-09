package identity

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/tokpath/tokstudio/backend/internal/platform/id"
)

type brandRow struct {
	ID            string     `gorm:"column:id;primaryKey"`
	Name          string     `gorm:"column:name"`
	LogoURL       *string    `gorm:"column:logo_url"`
	LogoDarkURL   *string    `gorm:"column:logo_dark_url"`
	FaviconURL    *string    `gorm:"column:favicon_url"`
	PrimaryDomain string     `gorm:"column:primary_domain"`
	APIDomain     string     `gorm:"column:api_domain"`
	AdminDomain   string     `gorm:"column:admin_domain"`
	CNAMETarget   string     `gorm:"column:cname_target"`
	TLSStatus     string     `gorm:"column:tls_status"`
	TLSIssuer     string     `gorm:"column:tls_issuer"`
	TLSDirectory  string     `gorm:"column:tls_directory"`
	TLSExpiresAt  *time.Time `gorm:"column:tls_expires_at"`
	ThemeJSON     []byte     `gorm:"column:theme_json"`
	CreatedAt     time.Time  `gorm:"column:created_at"`
}

func (brandRow) TableName() string { return "identity_brands" }

type channelRow struct {
	ID        string    `gorm:"column:id;primaryKey"`
	Code      string    `gorm:"column:code"`
	Type      string    `gorm:"column:type"`
	ParentID  *string   `gorm:"column:parent_id"`
	Status    string    `gorm:"column:status"`
	BrandID   string    `gorm:"column:brand_id"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

func (channelRow) TableName() string { return "identity_channel_orgs" }

type promotionRow struct {
	ID                string  `gorm:"column:id;primaryKey"`
	Code              string  `gorm:"column:code"`
	ChannelOrgID      string  `gorm:"column:channel_org_id"`
	AcquisitionRoleID *string `gorm:"column:acquisition_role_id"`
	Status            string  `gorm:"column:status"`
}

func (promotionRow) TableName() string { return "identity_promotion_codes" }

type attributionRow struct {
	UserID            string    `gorm:"column:user_id;primaryKey"`
	ChannelOrgID      string    `gorm:"column:channel_org_id"`
	AcquisitionRoleID *string   `gorm:"column:acquisition_role_id"`
	SourceCode        string    `gorm:"column:source_code"`
	AttributedAt      time.Time `gorm:"column:attributed_at"`
}

func (attributionRow) TableName() string { return "identity_attributions" }

type attributionChangeRow struct {
	ID          string          `gorm:"column:id;primaryKey"`
	UserID      string          `gorm:"column:user_id"`
	BeforeJSON  json.RawMessage `gorm:"column:before_json"`
	AfterJSON   json.RawMessage `gorm:"column:after_json"`
	Reason      string          `gorm:"column:reason"`
	ActorUserID string          `gorm:"column:actor_user_id"`
	CreatedAt   time.Time       `gorm:"column:created_at"`
}

func (attributionChangeRow) TableName() string { return "identity_attribution_changes" }

type resolvedPromotion struct {
	ChannelID         string
	BrandID           string
	AcquisitionRoleID *string
	SourceCode        string
}

func (s *Service) resolvePromotion(ctx context.Context, code string) (resolvedPromotion, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	if code == "" {
		code = OfficialPromotionCode
	}
	var promo promotionRow
	if err := s.db.WithContext(ctx).Where("code = ? AND status = ?", code, "active").First(&promo).Error; err != nil {
		return resolvedPromotion{}, ErrPromotionInvalid
	}
	var channel channelRow
	if err := s.db.WithContext(ctx).Where("id = ? AND status = ?", promo.ChannelOrgID, "active").First(&channel).Error; err != nil {
		return resolvedPromotion{}, ErrPromotionInvalid
	}
	return resolvedPromotion{
		ChannelID:         channel.ID,
		BrandID:           channel.BrandID,
		AcquisitionRoleID: promo.AcquisitionRoleID,
		SourceCode:        promo.Code,
	}, nil
}

// AssertChannelConsumable 在渠道停用后拦住新消费（聊天/媒体），余额和历史仍可查。
func (s *Service) AssertChannelConsumable(ctx context.Context, channelOrgID string) error {
	if channelOrgID == "" {
		return nil
	}
	var row channelRow
	if err := s.db.WithContext(ctx).Where("id = ?", channelOrgID).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	if row.Status != "" && row.Status != "active" {
		return ErrChannelDisabled
	}
	return nil
}

func (s *Service) KnownBrandHost(ctx context.Context, host string) bool {
	host = strings.ToLower(strings.TrimSpace(strings.Split(host, ":")[0]))
	if host == "" {
		return false
	}
	var n int64
	_ = s.db.WithContext(ctx).Model(&brandRow{}).
		Where("primary_domain = ? OR api_domain = ? OR admin_domain = ?", host, host, host).
		Count(&n).Error
	return n > 0
}

func (s *Service) ListBrands(ctx context.Context) ([]BrandView, error) {
	var rows []brandRow
	if err := s.db.WithContext(ctx).Order("id").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]BrandView, 0, len(rows))
	for _, row := range rows {
		out = append(out, *brandView(row))
	}
	return out, nil
}

func (s *Service) IssueBrandTLS(ctx context.Context, brandID, cname string) (*BrandView, error) {
	if strings.TrimSpace(cname) == "" {
		cname = "edge.tokenhub.local"
	}
	var row brandRow
	if err := s.db.WithContext(ctx).Where("id = ?", brandID).First(&row).Error; err != nil {
		return nil, err
	}
	issuer := IssuerSandbox
	directory := ""
	var expires *time.Time
	if s.acme.Enabled() && (s.acme.Force || UsePublicACME(row.PrimaryDomain)) {
		result, err := s.acme.Issue(ctx, row.PrimaryDomain)
		if err != nil {
			_ = s.db.WithContext(ctx).Model(&brandRow{}).Where("id = ?", brandID).Updates(map[string]any{
				"cname_target": cname, "tls_status": "failed", "tls_issuer": IssuerACME,
				"tls_directory": s.acme.Directory,
			}).Error
			return nil, err
		}
		issuer = result.Issuer
		directory = result.Directory
		expires = result.ExpiresAt
	}
	updates := map[string]any{
		"cname_target": cname, "tls_status": "issued", "tls_issuer": issuer, "tls_directory": directory,
	}
	if expires != nil {
		updates["tls_expires_at"] = *expires
	}
	if err := s.db.WithContext(ctx).Model(&brandRow{}).Where("id = ?", brandID).Updates(updates).Error; err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Where("id = ?", brandID).First(&row).Error; err != nil {
		return nil, err
	}
	return brandView(row), nil
}

func (s *Service) BrandByHost(ctx context.Context, host string) (*BrandView, error) {
	host = strings.ToLower(strings.TrimSpace(strings.Split(host, ":")[0]))
	var row brandRow
	err := s.db.WithContext(ctx).
		Where("primary_domain = ? OR api_domain = ? OR admin_domain = ?", host, host, host).
		First(&row).Error
	if err == gorm.ErrRecordNotFound {
		err = s.db.WithContext(ctx).Where("primary_domain = ?", OfficialPrimaryDomain).First(&row).Error
	}
	if err != nil {
		return nil, err
	}
	return brandView(row), nil
}

type ChannelInput struct {
	Code     string `json:"code"`
	Type     string `json:"type"`
	Status   string `json:"status"`
	BrandID  string `json:"brand_id"`
	ParentID string `json:"parent_id"`
}

func (s *Service) CreateChannel(ctx context.Context, viewer Principal, in ChannelInput) (*ChannelView, error) {
	if !viewer.IsPlatformAdmin() && !viewer.HasRole("channel_admin") {
		return nil, ErrChannelImmutable
	}
	in.Code = strings.TrimSpace(in.Code)
	if in.Code == "" || in.Type == "" {
		return nil, ErrPromotionInvalid
	}
	if in.Status == "" {
		in.Status = "active"
	}
	if in.BrandID == "" {
		in.BrandID = OfficialBrandID
	}
	parentID := strings.TrimSpace(in.ParentID)
	if viewer.HasRole("channel_admin") && !viewer.IsPlatformAdmin() {
		parentID = viewer.ChannelOrgID
		in.Type = ChannelTypeB
	}
	if parentID == "" {
		parentID = OfficialChannelID
	}
	parent, err := s.lookupChannel(ctx, parentID)
	if err != nil {
		return nil, err
	}
	if err := ValidateChannelParent(parent.Type, in.Type); err != nil {
		return nil, err
	}
	row := channelRow{ID: id.New("chn"), Code: in.Code, Type: in.Type, Status: in.Status, BrandID: in.BrandID, CreatedAt: time.Now().UTC(), ParentID: &parentID}
	if in.Type == ChannelTypeC {
		// C 可后续配自有品牌；默认仍用传入 brand。
	}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return nil, err
	}
	view := channelViewFrom(row)
	return &view, nil
}

func (s *Service) lookupChannel(ctx context.Context, channelID string) (channelRow, error) {
	var row channelRow
	if err := s.db.WithContext(ctx).Where("id = ?", channelID).First(&row).Error; err != nil {
		return channelRow{}, mapNotFound(err)
	}
	return row, nil
}

func (s *Service) ResolvePoolChannelID(ctx context.Context, channelID string) (string, error) {
	if channelID == "" {
		return "", nil
	}
	row, err := s.lookupChannel(ctx, channelID)
	if err != nil {
		return "", err
	}
	parentType := ""
	parentID := ""
	if row.ParentID != nil && *row.ParentID != "" {
		parent, err := s.lookupChannel(ctx, *row.ParentID)
		if err == nil {
			parentType = parent.Type
			parentID = parent.ID
		}
	}
	return PoolChannelID(row.Type, row.ID, parentID, parentType), nil
}

func (s *Service) ResolveMarketChannelID(ctx context.Context, channelID string) (string, error) {
	if channelID == "" {
		return "", nil
	}
	row, err := s.lookupChannel(ctx, channelID)
	if err != nil {
		return "", err
	}
	parentType := ""
	parentID := ""
	if row.ParentID != nil && *row.ParentID != "" {
		parent, err := s.lookupChannel(ctx, *row.ParentID)
		if err == nil {
			parentType = parent.Type
			parentID = parent.ID
		}
	}
	return MarketChannelID(row.Type, row.ID, parentID, parentType), nil
}

func (s *Service) GetChannel(ctx context.Context, viewer Principal, channelID string) (*ChannelView, error) {
	var row channelRow
	if err := s.db.WithContext(ctx).Where("id = ?", channelID).First(&row).Error; err != nil {
		return nil, mapNotFound(err)
	}
	if scoped := viewer.VisibleChannelID(); scoped != "" && scoped != row.ID {
		if row.ParentID == nil || *row.ParentID != scoped {
			return nil, ErrChannelImmutable
		}
	}
	view := channelViewFrom(row)
	return &view, nil
}

func (s *Service) PatchChannel(ctx context.Context, viewer Principal, channelID string, in ChannelInput) (*ChannelView, error) {
	if !viewer.IsPlatformAdmin() {
		return nil, ErrChannelImmutable
	}
	var row channelRow
	if err := s.db.WithContext(ctx).Where("id = ?", channelID).First(&row).Error; err != nil {
		return nil, mapNotFound(err)
	}
	updates := map[string]any{}
	if in.Status != "" {
		updates["status"] = in.Status
	}
	if in.BrandID != "" {
		updates["brand_id"] = in.BrandID
	}
	if in.Type != "" {
		updates["type"] = in.Type
	}
	if len(updates) > 0 {
		if err := s.db.WithContext(ctx).Model(&channelRow{}).Where("id = ?", channelID).Updates(updates).Error; err != nil {
			return nil, err
		}
	}
	if err := s.db.WithContext(ctx).Where("id = ?", channelID).First(&row).Error; err != nil {
		return nil, mapNotFound(err)
	}
	view := channelViewFrom(row)
	return &view, nil
}

func (s *Service) ListChannels(ctx context.Context, viewer Principal) ([]ChannelView, error) {
	q := s.db.WithContext(ctx).Model(&channelRow{})
	if channelID := viewer.VisibleChannelID(); channelID != "" {
		q = q.Where("id = ? OR parent_id = ?", channelID, channelID)
	}
	var rows []channelRow
	if err := q.Order("code").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]ChannelView, 0, len(rows))
	for _, row := range rows {
		out = append(out, channelViewFrom(row))
	}
	return out, nil
}

func channelViewFrom(row channelRow) ChannelView {
	view := ChannelView{ID: row.ID, Code: row.Code, Type: row.Type, Status: row.Status, BrandID: row.BrandID}
	if row.ParentID != nil {
		view.ParentID = *row.ParentID
	}
	return view
}

func (s *Service) ListUsers(ctx context.Context, viewer Principal) ([]UserView, error) {
	q := s.db.WithContext(ctx).Model(&userRow{})
	if channelID := viewer.VisibleChannelID(); channelID != "" {
		q = q.Where("channel_org_id = ?", channelID)
	}
	var rows []userRow
	if err := q.Order("created_at DESC").Limit(100).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]UserView, 0, len(rows))
	for _, row := range rows {
		var attr attributionRow
		source := ""
		if err := s.db.WithContext(ctx).Where("user_id = ?", row.ID).First(&attr).Error; err == nil {
			source = attr.SourceCode
		}
		out = append(out, viewFromUser(row, nil, source))
	}
	return out, nil
}

func (s *Service) Me(ctx context.Context, viewer Principal) (*UserView, error) {
	var user userRow
	if err := s.db.WithContext(ctx).Where("id = ?", viewer.UserID).First(&user).Error; err != nil {
		return nil, err
	}
	var attr attributionRow
	source := ""
	if err := s.db.WithContext(ctx).Where("user_id = ?", user.ID).First(&attr).Error; err == nil {
		source = attr.SourceCode
	}
	view := viewFromUser(user, viewer.Roles, source)
	return &view, nil
}

func (s *Service) AdminReattribute(ctx context.Context, actor Principal, userID, promotionCode, reason string) error {
	if !actor.IsPlatformAdmin() {
		return ErrChannelImmutable
	}
	if strings.TrimSpace(reason) == "" {
		return ErrPromotionInvalid
	}
	resolved, err := s.resolvePromotion(ctx, promotionCode)
	if err != nil {
		return err
	}
	var user userRow
	if err := s.db.WithContext(ctx).Where("id = ?", userID).First(&user).Error; err != nil {
		return err
	}
	before := map[string]string{"channel_org_id": deref(user.ChannelOrgID)}
	after := map[string]string{"channel_org_id": resolved.ChannelID, "source_code": resolved.SourceCode}
	beforeJSON, _ := json.Marshal(before)
	afterJSON, _ := json.Marshal(after)
	now := time.Now().UTC()
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&userRow{}).Where("id = ?", user.ID).Updates(map[string]any{
			"channel_org_id": resolved.ChannelID,
			"brand_id":       resolved.BrandID,
			"updated_at":     now,
		}).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", user.ID).Delete(&attributionRow{}).Error; err != nil {
			return err
		}
		if err := tx.Create(&attributionRow{
			UserID:            user.ID,
			ChannelOrgID:      resolved.ChannelID,
			AcquisitionRoleID: resolved.AcquisitionRoleID,
			SourceCode:        resolved.SourceCode,
			AttributedAt:      now,
		}).Error; err != nil {
			return err
		}
		return tx.Create(&attributionChangeRow{
			ID:          id.New("atr"),
			UserID:      user.ID,
			BeforeJSON:  beforeJSON,
			AfterJSON:   afterJSON,
			Reason:      reason,
			ActorUserID: actor.UserID,
			CreatedAt:   now,
		}).Error
	})
}

func brandView(row brandRow) *BrandView {
	theme := map[string]any{}
	_ = json.Unmarshal(row.ThemeJSON, &theme)
	view := &BrandView{
		ID:            row.ID,
		Name:          row.Name,
		PrimaryDomain: row.PrimaryDomain,
		APIDomain:     row.APIDomain,
		AdminDomain:   row.AdminDomain,
		CNAMETarget:   row.CNAMETarget,
		TLSStatus:     row.TLSStatus,
		TLSIssuer:     row.TLSIssuer,
		TLSDirectory:  row.TLSDirectory,
		Theme:         theme,
	}
	if row.TLSExpiresAt != nil {
		view.TLSExpiresAt = row.TLSExpiresAt.Unix()
	}
	if row.LogoURL != nil {
		view.LogoURL = *row.LogoURL
	}
	if row.LogoDarkURL != nil {
		view.LogoDarkURL = *row.LogoDarkURL
	}
	if row.FaviconURL != nil {
		view.FaviconURL = *row.FaviconURL
	}
	return view
}
