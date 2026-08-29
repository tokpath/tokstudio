package identity

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/tokpath/tokstudio/backend/internal/platform/id"
)

type brandRow struct {
	ID            string    `gorm:"column:id;primaryKey"`
	Name          string    `gorm:"column:name"`
	LogoURL       *string   `gorm:"column:logo_url"`
	PrimaryDomain string    `gorm:"column:primary_domain"`
	APIDomain     string    `gorm:"column:api_domain"`
	AdminDomain   string    `gorm:"column:admin_domain"`
	ThemeJSON     []byte    `gorm:"column:theme_json"`
	CreatedAt     time.Time `gorm:"column:created_at"`
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
	if !viewer.IsPlatformAdmin() {
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
	row := channelRow{ID: id.New("chn"), Code: in.Code, Type: in.Type, Status: in.Status, BrandID: in.BrandID, CreatedAt: time.Now().UTC()}
	if in.ParentID != "" {
		row.ParentID = &in.ParentID
	}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return nil, err
	}
	return &ChannelView{ID: row.ID, Code: row.Code, Type: row.Type, Status: row.Status, BrandID: row.BrandID}, nil
}

func (s *Service) PatchChannel(ctx context.Context, viewer Principal, channelID string, in ChannelInput) (*ChannelView, error) {
	if !viewer.IsPlatformAdmin() {
		return nil, ErrChannelImmutable
	}
	var row channelRow
	if err := s.db.WithContext(ctx).Where("id = ?", channelID).First(&row).Error; err != nil {
		return nil, err
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
		return nil, err
	}
	return &ChannelView{ID: row.ID, Code: row.Code, Type: row.Type, Status: row.Status, BrandID: row.BrandID}, nil
}

func (s *Service) ListChannels(ctx context.Context, viewer Principal) ([]ChannelView, error) {
	q := s.db.WithContext(ctx).Model(&channelRow{})
	if channelID := viewer.VisibleChannelID(); channelID != "" {
		q = q.Where("id = ?", channelID)
	}
	var rows []channelRow
	if err := q.Order("code").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]ChannelView, 0, len(rows))
	for _, row := range rows {
		out = append(out, ChannelView{ID: row.ID, Code: row.Code, Type: row.Type, Status: row.Status, BrandID: row.BrandID})
	}
	return out, nil
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
		view := UserView{
			ID:           row.ID,
			Email:        row.Email,
			Status:       row.Status,
			ChannelOrgID: deref(row.ChannelOrgID),
			BrandID:      deref(row.BrandID),
			CreatedAt:    row.CreatedAt,
		}
		var attr attributionRow
		if err := s.db.WithContext(ctx).Where("user_id = ?", row.ID).First(&attr).Error; err == nil {
			view.SourceCode = attr.SourceCode
		}
		out = append(out, view)
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
	return &UserView{
		ID:           user.ID,
		Email:        user.Email,
		Status:       user.Status,
		ChannelOrgID: deref(user.ChannelOrgID),
		BrandID:      deref(user.BrandID),
		Roles:        viewer.Roles,
		SourceCode:   source,
		CreatedAt:    user.CreatedAt,
	}, nil
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
		Theme:         theme,
	}
	if row.LogoURL != nil {
		view.LogoURL = *row.LogoURL
	}
	return view
}
