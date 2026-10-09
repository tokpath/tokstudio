package identity

import (
	"context"
	"github.com/tokpath/tokstudio/backend/internal/platform/crypto"
	"gorm.io/gorm"
	"net/url"
	"strings"
)

func (s *Service) createChannelInvitationTx(tx *gorm.DB, channel channelRow) error {
	if channel.Type != ChannelTypeB {
		return nil
	}
	code := "THC" + strings.ToUpper(crypto.HashToken(channel.ID)[:12])
	row := promotionRow{ID: "prm_" + crypto.HashToken(channel.ID)[:24], Code: code, ChannelOrgID: channel.ID, Status: "active"}
	return tx.Create(&row).Error
}

// Channel names are already globally unique. Recover an unchanged, owned
// channel after a lost creation response instead of creating a second channel.
func (s *Service) recoverCreatedChannel(ctx context.Context, intended channelRow) (*ChannelView, error) {
	if intended.Type != ChannelTypeB {
		return nil, ErrPromotionInvalid
	}
	var existing channelRow
	if err := s.db.WithContext(ctx).Where("code=?", intended.Code).First(&existing).Error; err != nil {
		return nil, err
	}
	if existing.Type != intended.Type || existing.BrandID != intended.BrandID || deref(existing.ParentID) != deref(intended.ParentID) || existing.Status != intended.Status {
		return nil, ErrChannelImmutable
	}
	view := channelViewFrom(existing)
	return &view, nil
}

type ChannelOnboarding struct {
	Channel         *ChannelView `json:"channel"`
	BrandName       string       `json:"brand_name"`
	RegistrationURL string       `json:"registration_url"`
	AdminCount      int          `json:"admin_count"`
	CanManageAdmins bool         `json:"can_manage_admins"`
	CanManageModels bool         `json:"can_manage_models"`
}

func (s *Service) ChannelOnboarding(ctx context.Context, p Principal, channelID string) (*ChannelOnboarding, error) {
	if _, err := s.CustomerChannels(ctx, p, channelID); err != nil {
		return nil, err
	}
	ch, err := s.GetChannel(ctx, p, channelID)
	if err != nil {
		return nil, err
	}
	if ch.Type != ChannelTypeB {
		return nil, ErrChannelImmutable
	}
	out := &ChannelOnboarding{Channel: ch}
	owner, err := s.ResolvePaymentOwnerID(ctx, ch.ID)
	if err != nil {
		return nil, err
	}
	out.CanManageAdmins = (p.IsPlatformAdmin() && owner == OfficialChannelID) || (p.HasRole("channel_admin") && owner == p.ChannelOrgID)
	out.CanManageModels = (p.IsPlatformAdmin() && owner == OfficialChannelID) || (p.HasRole("channel_admin", "oem_ops") && owner == p.ChannelOrgID)
	var brand brandRow
	if err := s.db.WithContext(ctx).Where("id=?", ch.BrandID).First(&brand).Error; err != nil {
		return nil, err
	}
	out.BrandName = brand.Name
	members, err := s.ChannelAdmins(ctx, ch.ID)
	if err != nil {
		return nil, err
	}
	out.AdminCount = len(members)
	var promo struct{ Code string }
	if err := s.db.WithContext(ctx).Table("identity_promotion_codes p").Select("p.code").Joins("LEFT JOIN identity_acquisition_roles r ON r.id=p.acquisition_role_id").Where("p.channel_org_id=? AND p.status='active' AND (p.acquisition_role_id IS NULL OR (r.status='active' AND r.channel_org_id=p.channel_org_id))", ch.ID).Order("p.acquisition_role_id NULLS FIRST,p.created_at,p.id").Limit(1).Scan(&promo).Error; err != nil {
		return nil, err
	}
	domain := strings.TrimSpace(brand.PrimaryDomain)
	if ch.Status == "active" && promo.Code != "" && domain != "" && !strings.ContainsAny(domain, "/\\?#@") {
		scheme := "https"
		if domain == "localhost" || strings.HasPrefix(domain, "localhost:") {
			scheme = "http"
		}
		link := url.URL{Scheme: scheme, Host: domain, Path: "/login", RawQuery: url.Values{"promotion_code": {promo.Code}}.Encode()}
		out.RegistrationURL = link.String()
	}
	return out, nil
}
