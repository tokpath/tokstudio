package identity

import (
	"context"
	"errors"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/tokpath/tokstudio/backend/internal/platform/crypto"
	"github.com/tokpath/tokstudio/backend/internal/platform/id"
)

var (
	ErrInvalidCredentials   = errors.New("invalid credentials")
	ErrEmailTaken           = errors.New("email already registered")
	ErrWeakPassword         = errors.New("password too short")
	ErrChannelImmutable     = errors.New("channel attribution cannot be changed by the user")
	ErrChannelDisabled      = errors.New("channel is disabled")
	ErrPromotionInvalid     = errors.New("promotion code is invalid")
	ErrInvalidProfile       = errors.New("profile is invalid")
	ErrInvalidLocale        = errors.New("locale is not supported")
	ErrNotFound             = errors.New("record not found")
	ErrThemeKey             = errors.New("theme key is not allowed")
	ErrThemeHex             = errors.New("theme color must be #RRGGBB")
	ErrThemeContrast        = errors.New("theme contrast is below 4.5:1")
	ErrBrandNotCustomizable = errors.New("brand is not customizable")
	ErrAssetKind            = errors.New("asset kind is not allowed")
	ErrAssetType            = errors.New("asset type is not allowed")
	ErrAssetTooLarge        = errors.New("asset exceeds size limit")
	ErrAssetDimension       = errors.New("asset dimensions are outside the contract")
	ErrAssetSVG             = errors.New("svg asset is unsafe or missing viewBox")
	ErrAssetRateLimited     = errors.New("brand asset upload rate limited")
	ErrBrandDomainTaken     = errors.New("brand domain is already used")
	ErrStoreUnavailable     = errors.New("object store unavailable")
	ErrGoogleUnavailable    = errors.New("google oauth is not configured")
	ErrGoogleExchange       = errors.New("google oauth exchange failed")
	// ErrOAuthStateConsumed：state 已领取/删除（典型是 React 双回调二次请求）。
	ErrOAuthStateConsumed = errors.New("oauth state already consumed")
)

// GoogleExchangeError 携带 Google 返回的安全 error 码（不含 code/token/secret）。
type GoogleExchangeError struct {
	Reason string
}

func (e *GoogleExchangeError) Error() string {
	if e == nil || e.Reason == "" {
		return ErrGoogleExchange.Error()
	}
	return ErrGoogleExchange.Error() + ": " + e.Reason
}

func (e *GoogleExchangeError) Unwrap() error { return ErrGoogleExchange }

func NewGoogleExchangeError(reason string) error {
	reason = sanitizeGoogleReason(reason)
	if reason == "" {
		return ErrGoogleExchange
	}
	return &GoogleExchangeError{Reason: reason}
}

func sanitizeGoogleReason(reason string) string {
	reason = strings.TrimSpace(strings.ToLower(reason))
	switch reason {
	case "invalid_grant", "redirect_uri_mismatch", "invalid_client", "unauthorized_client",
		"access_denied", "invalid_request", "unsupported_grant_type", "network_error",
		"userinfo_error", "empty_token", "empty_profile":
		return reason
	case "email_unverified", "state_mismatch", "account_conflict", "account_link_required", "configuration_changed":
		return reason
	default:
		return ""
	}
}

func mapNotFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	return err
}

func HashPassword(password string) (string, error) {
	if len(password) < 8 || len(password) > 72 {
		return "", ErrWeakPassword
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func checkPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

func (s *Service) Register(ctx context.Context, in RegisterInput) (*Session, error) {
	email := normalizeEmail(in.Email)
	if email == "" || !strings.Contains(email, "@") {
		return nil, ErrInvalidCredentials
	}
	hash, err := HashPassword(in.Password)
	if err != nil {
		return nil, err
	}
	resolved, err := s.resolveRegistration(ctx, in.PromotionCode, in.BrandID)
	if err != nil {
		return nil, err
	}

	var count int64
	if err := s.db.WithContext(ctx).Model(&userRow{}).Where("brand_id = ? AND lower(email) = ?", resolved.BrandID, email).Count(&count).Error; err != nil {
		return nil, err
	}
	if count > 0 {
		return nil, ErrEmailTaken
	}

	now := time.Now().UTC()
	user := userRow{
		ID:           id.New("usr"),
		Email:        email,
		PasswordHash: &hash,
		Status:       "active",
		ChannelOrgID: &resolved.ChannelID,
		BrandID:      &resolved.BrandID,
		Locale:       DefaultLocale,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if in.googleSubject != "" {
		user.GoogleSub = &in.googleSubject
		user.EmailVerifiedAt = &now
	}
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&user).Error; err != nil {
			return err
		}
		var role roleRow
		if err := tx.Where("code = ?", "end_user").First(&role).Error; err != nil {
			return err
		}
		if err := tx.Create(&userRoleRow{
			UserID:    user.ID,
			RoleID:    role.ID,
			ScopeType: "channel",
			ScopeID:   resolved.ChannelID,
		}).Error; err != nil {
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
		scoped := *s
		scoped.db = tx
		if err := scoped.ensureUserPromo(ctx, user.ID, resolved.ChannelID, resolved.AcquisitionRoleID); err != nil {
			return err
		}
		if in.Verification != nil && s.EmailAvailable() && user.EmailVerifiedAt == nil {
			return s.queueEmailAction(tx, user, EmailVerify, *in.Verification)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return s.issueSession(ctx, user)
}

func (s *Service) Login(ctx context.Context, in LoginInput) (*Session, error) {
	email := normalizeEmail(in.Email)
	brandID := in.BrandID
	if brandID == "" {
		brandID = OfficialBrandID
	}
	var session *Session
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var user userRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("brand_id = ? AND lower(email) = ? AND status = ?", brandID, email, "active").First(&user).Error; err != nil {
			return ErrInvalidCredentials
		}
		if user.PasswordHash == nil || !checkPassword(*user.PasswordHash, in.Password) {
			return ErrInvalidCredentials
		}
		// Serialize password login with reset, so an in-flight old-password login
		// cannot create a surviving session after the reset has revoked sessions.
		scoped := *s
		scoped.db = tx
		var err error
		session, err = scoped.issueSession(ctx, user)
		return err
	})
	if err != nil {
		return nil, err
	}
	return session, nil
}

// Logout 吊销当前会话令牌。空令牌是空操作，不发明会话。
func (s *Service) Logout(ctx context.Context, bearer string) error {
	token := strings.TrimSpace(strings.TrimPrefix(bearer, "Bearer "))
	token = strings.TrimSpace(token)
	if token == "" {
		return nil
	}
	return s.db.WithContext(ctx).Model(&tokenRow{}).
		Where("token_hash = ? AND status = ?", crypto.HashToken(token), "active").
		Update("status", "revoked").Error
}

func (s *Service) SwitchChannel(_ context.Context, _ Principal, _ string) error {
	return ErrChannelImmutable
}

func (s *Service) issueSession(ctx context.Context, user userRow) (*Session, error) {
	disabled, err := s.staffDisabled(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	if disabled {
		return nil, ErrInvalidCredentials
	}
	raw, err := crypto.RandomToken("thses_")
	if err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Create(&tokenRow{
		ID:        id.New("tok"),
		UserID:    user.ID,
		TokenHash: crypto.HashToken(raw),
		Prefix:    "thses_",
		Status:    "active",
		CreatedAt: time.Now().UTC(),
	}).Error; err != nil {
		return nil, err
	}
	principal, err := s.loadPrincipal(ctx, user)
	if err != nil {
		return nil, err
	}
	return &Session{
		Token:     raw,
		ExpiresAt: time.Now().UTC().Add(24 * time.Hour),
		User:      viewFromUser(user, principal.Roles, ""),
	}, nil
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}
