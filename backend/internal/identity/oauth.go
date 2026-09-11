package identity

import (
	"context"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/tokpath/tokstudio/backend/internal/platform/crypto"
	"github.com/tokpath/tokstudio/backend/internal/platform/id"
)

type oauthStateRow struct {
	ID            string    `gorm:"column:id;primaryKey"`
	StateHash     string    `gorm:"column:state_hash"`
	PromotionCode *string   `gorm:"column:promotion_code"`
	ExpiresAt     time.Time `gorm:"column:expires_at"`
	CreatedAt     time.Time `gorm:"column:created_at"`
}

func (oauthStateRow) TableName() string { return "identity_oauth_states" }

// GoogleProfile 是 Google 用户信息。生产路径由真实 token/profile 交换得到；测试可注入假 exchanger。
type GoogleProfile struct {
	Subject string
	Email   string
}

type GoogleExchanger func(ctx context.Context, code string) (GoogleProfile, error)

func (s *Service) StartGoogle(ctx context.Context, promotionCode string) (state string, err error) {
	state, err = crypto.RandomToken("gstate_")
	if err != nil {
		return "", err
	}
	var promo *string
	if strings.TrimSpace(promotionCode) != "" {
		value := strings.ToUpper(strings.TrimSpace(promotionCode))
		promo = &value
	}
	row := oauthStateRow{
		ID:            id.New("oas"),
		StateHash:     crypto.HashToken(state),
		PromotionCode: promo,
		ExpiresAt:     time.Now().UTC().Add(15 * time.Minute),
		CreatedAt:     time.Now().UTC(),
	}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return "", err
	}
	return state, nil
}

func (s *Service) FinishGoogle(ctx context.Context, state, code string, exchange GoogleExchanger) (*Session, error) {
	if exchange == nil {
		return nil, ErrGoogleUnavailable
	}
	var row oauthStateRow
	if err := s.db.WithContext(ctx).
		Where("state_hash = ? AND expires_at > ?", crypto.HashToken(state), time.Now().UTC()).
		First(&row).Error; err != nil {
		return nil, ErrInvalidCredentials
	}
	profile, err := exchange(ctx, code)
	if err != nil {
		return nil, err
	}
	promo := OfficialPromotionCode
	if row.PromotionCode != nil && *row.PromotionCode != "" {
		promo = *row.PromotionCode
	}
	resolved, err := s.resolvePromotion(ctx, promo)
	if err != nil {
		return nil, err
	}

	var user userRow
	err = s.db.WithContext(ctx).Where("google_sub = ? OR email = ?", profile.Subject, profile.Email).First(&user).Error
	if err == gorm.ErrRecordNotFound {
		session, err := s.Register(ctx, RegisterInput{
			Email:         profile.Email,
			Password:      "oauth-" + crypto.HashToken(profile.Subject)[:16] + "Xx",
			PromotionCode: promo,
		})
		if err != nil {
			return nil, err
		}
		_ = s.db.WithContext(ctx).Model(&userRow{}).Where("email = ?", profile.Email).Updates(map[string]any{
			"google_sub":        profile.Subject,
			"email_verified_at": time.Now().UTC(),
		})
		_ = s.db.WithContext(ctx).Where("id = ?", row.ID).Delete(&oauthStateRow{})
		return session, nil
	}
	if err != nil {
		return nil, err
	}
	if deref(user.ChannelOrgID) == "" {
		_ = s.db.WithContext(ctx).Model(&userRow{}).Where("id = ?", user.ID).Updates(map[string]any{
			"channel_org_id": resolved.ChannelID,
			"brand_id":       resolved.BrandID,
			"google_sub":     profile.Subject,
		})
		user.ChannelOrgID = &resolved.ChannelID
		user.BrandID = &resolved.BrandID
	}
	_ = s.db.WithContext(ctx).Where("id = ?", row.ID).Delete(&oauthStateRow{})
	return s.issueSession(ctx, user)
}
