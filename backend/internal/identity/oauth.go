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
	ConfigVersion int64
	RedirectURI   string
	LinkUserID    *string
	TestOnly      bool
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
	ConfigVersion int64
	RedirectURI   string
	LinkUserID    string
	TestOnly      bool
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
	return OAuthIntent{ConfigVersion: row.ConfigVersion, RedirectURI: row.RedirectURI, LinkUserID: deref(row.LinkUserID), TestOnly: row.TestOnly, PromotionCode: deref(row.PromotionCode), BrandID: deref(row.BrandID), NextPath: SafeReturnPath(deref(row.ReturnPath))}
}

func (s *Service) StartGoogle(ctx context.Context, promotionCode string) (string, error) {
	return s.StartGoogleWithIntent(ctx, OAuthIntent{PromotionCode: promotionCode})
}

func (s *Service) StartGoogleWithIntent(ctx context.Context, intent OAuthIntent) (state string, err error) {
	promotionCode := intent.PromotionCode
	if !intent.TestOnly && intent.LinkUserID == "" {
		resolved, err := s.resolveRegistration(ctx, promotionCode, intent.BrandID)
		if err != nil {
			return "", err
		}
		intent.BrandID = resolved.BrandID
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
		ConfigVersion: intent.ConfigVersion, RedirectURI: intent.RedirectURI, TestOnly: intent.TestOnly,
		BrowserHash: stringPointer(crypto.HashToken(intent.BrowserToken)),
		ID:          id.New("oas"),
		BrandID:     &intent.BrandID, ReturnPath: stringPointer(SafeReturnPath(intent.NextPath)),
		StateHash:     crypto.HashToken(state),
		PromotionCode: promo,
		ExpiresAt:     time.Now().UTC().Add(15 * time.Minute),
		CreatedAt:     time.Now().UTC(),
	}
	if intent.LinkUserID != "" {
		row.LinkUserID = &intent.LinkUserID
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
		if row.TestOnly {
			reason := "authentication_error"
			var googleErr *GoogleExchangeError
			if errors.As(err, &googleErr) {
				reason = googleErr.Reason
			}
			_ = s.RecordGoogleVerification(ctx, OAuthIntent{BrandID: deref(row.BrandID), ConfigVersion: row.ConfigVersion, RedirectURI: row.RedirectURI}, reason)
		}
		return nil, err
	}
	if strings.TrimSpace(profile.Subject) == "" || !strings.Contains(profile.Email, "@") {
		return nil, NewGoogleExchangeError("empty_profile")
	}
	if row.TestOnly {
		if err := s.RecordGoogleVerification(ctx, OAuthIntent{BrandID: deref(row.BrandID), ConfigVersion: row.ConfigVersion, RedirectURI: row.RedirectURI}, ""); err != nil {
			return nil, err
		}
		return &Session{TestOnly: true}, nil
	}
	promo := ""
	if row.PromotionCode != nil && *row.PromotionCode != "" {
		promo = *row.PromotionCode
	}
	profile.Email = normalizeEmail(profile.Email)
	var session *Session
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if row.ConfigVersion > 0 {
			var cfg googleConfigRow
			if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).Where("brand_id = ? AND version = ? AND enabled = true", deref(row.BrandID), row.ConfigVersion).First(&cfg).Error; err != nil {
				return NewGoogleExchangeError("configuration_changed")
			}
		}
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", "google-user:"+deref(row.BrandID)).Error; err != nil {
			return err
		}
		var user userRow
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("brand_id = ? AND google_sub = ?", deref(row.BrandID), profile.Subject).First(&user).Error
		if row.LinkUserID != nil {
			if err == nil && user.ID != *row.LinkUserID {
				return NewGoogleExchangeError("account_conflict")
			}
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND brand_id = ?", *row.LinkUserID, deref(row.BrandID)).First(&user).Error
			if err != nil {
				return ErrInvalidCredentials
			}
			if normalizeEmail(user.Email) != profile.Email || (deref(user.GoogleSub) != "" && deref(user.GoogleSub) != profile.Subject) {
				return NewGoogleExchangeError("account_conflict")
			}
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("brand_id = ? AND lower(email) = ?", deref(row.BrandID), profile.Email).First(&user).Error
			if err == nil && deref(user.GoogleSub) != "" && deref(user.GoogleSub) != profile.Subject {
				return NewGoogleExchangeError("account_conflict")
			}
			if err == nil {
				return NewGoogleExchangeError("account_link_required")
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
