package identity

import (
	"context"
	"errors"
	"net/url"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// PersonalReferral only includes the viewer's own memberships, never their downline.
type ReferralLink struct {
	Code     string `json:"code"`
	ShareURL string `json:"share_url"`
}

type PersonalReferral struct {
	Links        []ReferralLink  `json:"links"`
	Codes        []PromotionView `json:"codes"`
	InvitedCount int64           `json:"invited_count"`
	RoleIDs      []string        `json:"-"`
}

func (s *Service) PersonalReferral(ctx context.Context, userID string) (*PersonalReferral, error) {
	out := &PersonalReferral{Codes: []PromotionView{}, RoleIDs: []string{}, Links: []ReferralLink{}}
	if err := s.db.WithContext(ctx).Model(&roleMemberRow{}).Where("user_id = ?", userID).Pluck("acquisition_role_id", &out.RoleIDs).Error; err != nil {
		return nil, err
	}
	if len(out.RoleIDs) == 0 {
		return out, nil
	}
	var rows []struct {
		Promotion     promotionRow `gorm:"embedded"`
		PrimaryDomain string
	}
	if err := s.db.WithContext(ctx).Table("identity_promotion_codes AS p").Select("p.*, b.primary_domain").
		Joins("JOIN identity_acquisition_roles AS r ON r.id = p.acquisition_role_id AND r.channel_org_id = p.channel_org_id").
		Joins("JOIN identity_channel_orgs AS c ON c.id = p.channel_org_id").
		Joins("JOIN identity_brands AS b ON b.id = c.brand_id").
		Where("p.acquisition_role_id IN ? AND p.status = ? AND r.status = ? AND c.status = ?", out.RoleIDs, "active", "active", "active").Order("p.code").Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		out.Codes = append(out.Codes, *promoView(row.Promotion))
		domain := strings.TrimSpace(row.PrimaryDomain)
		if domain != "" && !strings.ContainsAny(domain, "/\\?#@") {
			scheme := "https"
			if domain == "localhost" || strings.HasPrefix(domain, "localhost:") {
				scheme = "http"
			}
			link := url.URL{Scheme: scheme, Host: domain, Path: "/login"}
			params := url.Values{"promotion_code": {row.Promotion.Code}}
			link.RawQuery = params.Encode()
			out.Links = append(out.Links, ReferralLink{Code: row.Promotion.Code, ShareURL: link.String()})
		}
	}
	if err := s.db.WithContext(ctx).Model(&attributionRow{}).Where("acquisition_role_id IN ?", out.RoleIDs).Count(&out.InvitedCount).Error; err != nil {
		return nil, err
	}
	return out, nil
}

// CreatePersonalReferral repairs old accounts that predate automatic promo creation.
// A user lock makes repeated clicks/concurrent requests idempotent.
func (s *Service) CreatePersonalReferral(ctx context.Context, userID string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var user userRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", userID).First(&user).Error; err != nil {
			return err
		}
		var n int64
		if err := tx.Model(&roleMemberRow{}).Where("user_id = ?", userID).Count(&n).Error; err != nil {
			return err
		}
		// Existing disabled memberships/codes must never be bypassed by making new ones.
		if n > 0 {
			return nil
		}
		var attr attributionRow
		if err := tx.Where("user_id = ?", userID).First(&attr).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if user.ChannelOrgID == nil {
			return ErrPromotionInvalid
		}
		var channel channelRow
		if err := tx.Where("id = ? AND status = ?", *user.ChannelOrgID, "active").First(&channel).Error; err != nil {
			return ErrPromotionInvalid
		}
		scoped := *s
		scoped.db = tx
		return scoped.ensureUserPromo(ctx, userID, channel.ID, attr.AcquisitionRoleID)
	})
}
