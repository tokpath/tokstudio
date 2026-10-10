package identity

import (
	"context"
	"crypto/subtle"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/tokpath/tokstudio/backend/internal/platform/crypto"
	"github.com/tokpath/tokstudio/backend/internal/platform/id"
)

type oauthStateRow struct {
	BrowserHash   *string    `gorm:"column:browser_hash"`
	ConsumedAt    *time.Time `gorm:"column:consumed_at"`
	BrandID       *string    `gorm:"column:brand_id"`
	ReturnPath    *string    `gorm:"column:return_path"`
	ID            string     `gorm:"column:id;primaryKey"`
	StateHash     string     `gorm:"column:state_hash"`
	PromotionCode *string    `gorm:"column:promotion_code"`
	ExpiresAt     time.Time  `gorm:"column:expires_at"`
	CreatedAt     time.Time  `gorm:"column:created_at"`
}

func (oauthStateRow) TableName() string { return "identity_oauth_states" }

// GoogleProfile 是 Google 用户信息。生产路径由真实 token/profile 交换得到；测试可注入假 exchanger。
type GoogleProfile struct {
	Subject string
	Email   string
}

type GoogleExchanger func(ctx context.Context, code string) (GoogleProfile, error)

type OAuthIntent struct {
	BrowserToken  string
	PromotionCode string
	BrandID       string
	NextPath      string
}

func (s *Service) GoogleBrowserMatches(ctx context.Context, state, challenge string) bool {
	if state == "" || challenge == "" {
		return false
	}
	var row oauthStateRow
	if s.db.WithContext(ctx).Where("state_hash = ? AND expires_at > ?", crypto.HashToken(state), time.Now().UTC()).First(&row).Error != nil || row.BrowserHash == nil {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(*row.BrowserHash), []byte(crypto.HashToken(challenge))) == 1
}

func (s *Service) GoogleIntent(ctx context.Context, state string) OAuthIntent {
	var row oauthStateRow
	if state == "" || s.db.WithContext(ctx).Where("state_hash = ? AND expires_at > ?", crypto.HashToken(state), time.Now().UTC()).First(&row).Error != nil {
		return OAuthIntent{}
	}
	return OAuthIntent{PromotionCode: deref(row.PromotionCode), BrandID: deref(row.BrandID), NextPath: SafeReturnPath(deref(row.ReturnPath))}
}

func (s *Service) StartGoogle(ctx context.Context, promotionCode string) (string, error) {
	return s.StartGoogleWithIntent(ctx, OAuthIntent{PromotionCode: promotionCode})
}

func (s *Service) StartGoogleWithIntent(ctx context.Context, intent OAuthIntent) (state string, err error) {
	promotionCode := intent.PromotionCode
	if _, err := s.resolveRegistration(ctx, promotionCode, intent.BrandID); err != nil {
		return "", err
	}
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
		BrowserHash: stringPointer(crypto.HashToken(intent.BrowserToken)),
		ID:          id.New("oas"),
		BrandID:     &intent.BrandID, ReturnPath: stringPointer(SafeReturnPath(intent.NextPath)),
		StateHash:     crypto.HashToken(state),
		PromotionCode: promo,
		ExpiresAt:     time.Now().UTC().Add(15 * time.Minute),
		CreatedAt:     time.Now().UTC(),
	}
	if err := s.db.WithContext(ctx).Where("expires_at <= ?", time.Now().UTC()).Delete(&oauthStateRow{}).Error; err != nil {
		return "", err
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
	// 先原子领取 state，避免 React 双请求把同一授权码兑换两次（第二次 invalid_grant）。
	var row oauthStateRow
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("state_hash = ? AND expires_at > ?", crypto.HashToken(state), time.Now().UTC()).
			First(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrOAuthStateConsumed
			}
			return err
		}
		if row.ConsumedAt != nil {
			return ErrOAuthStateConsumed
		}
		if err := tx.Model(&oauthStateRow{}).Where("id = ?", row.ID).Update("consumed_at", time.Now().UTC()).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	profile, err := exchange(ctx, code)
	if err != nil {
		return nil, err
	}
	promo := ""
	if row.PromotionCode != nil && *row.PromotionCode != "" {
		promo = *row.PromotionCode
	}
	resolved, err := s.resolveRegistration(ctx, promo, deref(row.BrandID))
	if err != nil {
		return nil, err
	}

	if strings.TrimSpace(profile.Subject) == "" || !strings.Contains(profile.Email, "@") {
		return nil, NewGoogleExchangeError("empty_profile")
	}
	profile.Email = normalizeEmail(profile.Email)
	var session *Session
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var user userRow
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("google_sub = ?", profile.Subject).First(&user).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("email = ?", profile.Email).First(&user).Error
			if err == nil && deref(user.GoogleSub) != "" && deref(user.GoogleSub) != profile.Subject {
				return NewGoogleExchangeError("account_conflict")
			}
		}
		scoped := *s
		scoped.db = tx
		if errors.Is(err, gorm.ErrRecordNotFound) {
			password, passwordErr := crypto.RandomToken("oauth_")
			if passwordErr != nil {
				return passwordErr
			}
			session, err = scoped.Register(ctx, RegisterInput{Email: profile.Email, Password: password, PromotionCode: promo, BrandID: deref(row.BrandID), googleSubject: profile.Subject})
			return err
		}
		if err != nil {
			return err
		}
		if user.Status != "active" {
			return ErrInvalidCredentials
		}
		updates := map[string]any{"google_sub": profile.Subject, "email_verified_at": time.Now().UTC()}
		if deref(user.ChannelOrgID) == "" {
			updates["channel_org_id"], updates["brand_id"] = resolved.ChannelID, resolved.BrandID
			user.ChannelOrgID, user.BrandID = &resolved.ChannelID, &resolved.BrandID
		}
		if err := tx.Model(&userRow{}).Where("id = ?", user.ID).Updates(updates).Error; err != nil {
			return err
		}
		user.GoogleSub = &profile.Subject
		session, err = scoped.issueSession(ctx, user)
		return err
	})
	if err == nil {
		session.NextPath = SafeReturnPath(deref(row.ReturnPath))
	}
	return session, err
}

func stringPointer(value string) *string { return &value }
