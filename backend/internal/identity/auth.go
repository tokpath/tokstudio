package identity

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"github.com/tokpath/tokstudio/backend/internal/platform/crypto"
	"github.com/tokpath/tokstudio/backend/internal/platform/id"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrEmailTaken         = errors.New("email already registered")
	ErrWeakPassword       = errors.New("password too short")
	ErrChannelImmutable   = errors.New("channel attribution cannot be changed by the user")
	ErrChannelDisabled    = errors.New("channel is disabled")
	ErrPromotionInvalid   = errors.New("promotion code is invalid")
	ErrOTPInvalid         = errors.New("verification code is invalid")
	ErrInvalidProfile     = errors.New("profile is invalid")
	ErrInvalidLocale      = errors.New("locale is not supported")
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
)

func mapNotFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	return err
}

type otpRow struct {
	ID         string     `gorm:"column:id;primaryKey"`
	Email      string     `gorm:"column:email"`
	Purpose    string     `gorm:"column:purpose"`
	CodeHash   string     `gorm:"column:code_hash"`
	ExpiresAt  time.Time  `gorm:"column:expires_at"`
	ConsumedAt *time.Time `gorm:"column:consumed_at"`
	CreatedAt  time.Time  `gorm:"column:created_at"`
}

func (otpRow) TableName() string { return "identity_email_otps" }

func HashPassword(password string) (string, error) {
	if len(password) < 8 {
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
	resolved, err := s.resolvePromotion(ctx, in.PromotionCode)
	if err != nil {
		return nil, err
	}
	if in.OTP != "" {
		if err := s.consumeOTP(ctx, email, "register", in.OTP); err != nil {
			return nil, err
		}
	}

	var count int64
	if err := s.db.WithContext(ctx).Model(&userRow{}).Where("email = ?", email).Count(&count).Error; err != nil {
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
	if in.OTP != "" {
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
		return tx.Create(&attributionRow{
			UserID:            user.ID,
			ChannelOrgID:      resolved.ChannelID,
			AcquisitionRoleID: resolved.AcquisitionRoleID,
			SourceCode:        resolved.SourceCode,
			AttributedAt:      now,
		}).Error
	}); err != nil {
		return nil, err
	}
	return s.issueSession(ctx, user)
}

func (s *Service) Login(ctx context.Context, in LoginInput) (*Session, error) {
	email := normalizeEmail(in.Email)
	var user userRow
	if err := s.db.WithContext(ctx).Where("email = ? AND status = ?", email, "active").First(&user).Error; err != nil {
		return nil, ErrInvalidCredentials
	}
	if user.PasswordHash == nil || !checkPassword(*user.PasswordHash, in.Password) {
		return nil, ErrInvalidCredentials
	}
	return s.issueSession(ctx, user)
}

func (s *Service) RequestOTP(ctx context.Context, email, purpose string) (string, error) {
	email = normalizeEmail(email)
	if email == "" {
		return "", ErrInvalidCredentials
	}
	if purpose == "" {
		purpose = "login"
	}
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return "", err
	}
	code := fmt.Sprintf("%06d", n.Int64())
	row := otpRow{
		ID:        id.New("otp"),
		Email:     email,
		Purpose:   purpose,
		CodeHash:  crypto.HashToken(code),
		ExpiresAt: time.Now().UTC().Add(10 * time.Minute),
		CreatedAt: time.Now().UTC(),
	}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return "", err
	}
	return code, nil
}

func (s *Service) LoginWithOTP(ctx context.Context, email, code string) (*Session, error) {
	email = normalizeEmail(email)
	if err := s.consumeOTP(ctx, email, "login", code); err != nil {
		return nil, err
	}
	var user userRow
	if err := s.db.WithContext(ctx).Where("email = ? AND status = ?", email, "active").First(&user).Error; err != nil {
		return nil, ErrInvalidCredentials
	}
	now := time.Now().UTC()
	_ = s.db.WithContext(ctx).Model(&userRow{}).Where("id = ?", user.ID).Update("email_verified_at", now).Error
	return s.issueSession(ctx, user)
}

func (s *Service) consumeOTP(ctx context.Context, email, purpose, code string) error {
	var row otpRow
	err := s.db.WithContext(ctx).
		Where("email = ? AND purpose = ? AND consumed_at IS NULL AND expires_at > ?", email, purpose, time.Now().UTC()).
		Order("created_at DESC").
		First(&row).Error
	if err != nil {
		return ErrOTPInvalid
	}
	if row.CodeHash != crypto.HashToken(code) {
		return ErrOTPInvalid
	}
	now := time.Now().UTC()
	return s.db.WithContext(ctx).Model(&otpRow{}).Where("id = ?", row.ID).Update("consumed_at", now).Error
}

func (s *Service) SwitchChannel(_ context.Context, _ Principal, _ string) error {
	return ErrChannelImmutable
}

func (s *Service) issueSession(ctx context.Context, user userRow) (*Session, error) {
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
