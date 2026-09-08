package identity

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	DefaultLocale     = "zh"
	MaxDisplayNameLen = 80
)

type UpdateProfileInput struct {
	DisplayName *string
	Locale      *string
}

func NormalizeDisplayName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if utf8.RuneCountInString(name) > MaxDisplayNameLen {
		return "", ErrInvalidProfile
	}
	return name, nil
}

func NormalizeLocale(locale string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(locale)) {
	case "zh", "en", "ja":
		return strings.ToLower(strings.TrimSpace(locale)), nil
	default:
		return "", ErrInvalidLocale
	}
}

func localeOrDefault(locale string) string {
	if locale == "" {
		return DefaultLocale
	}
	return locale
}

func viewFromUser(user userRow, roles []string, source string) UserView {
	return UserView{
		ID:            user.ID,
		Email:         user.Email,
		DisplayName:   user.DisplayName,
		Locale:        localeOrDefault(user.Locale),
		Status:        user.Status,
		ChannelOrgID:  deref(user.ChannelOrgID),
		BrandID:       deref(user.BrandID),
		Roles:         roles,
		SourceCode:    source,
		CanCommission: user.CanCommission,
		CreatedAt:     user.CreatedAt,
	}
}

func (s *Service) UpdateProfile(ctx context.Context, viewer Principal, in UpdateProfileInput) (*UserView, error) {
	updates := map[string]any{"updated_at": time.Now().UTC()}
	if in.DisplayName != nil {
		name, err := NormalizeDisplayName(*in.DisplayName)
		if err != nil {
			return nil, err
		}
		updates["display_name"] = name
	}
	if in.Locale != nil {
		locale, err := NormalizeLocale(*in.Locale)
		if err != nil {
			return nil, err
		}
		updates["locale"] = locale
	}
	if len(updates) == 1 {
		return s.Me(ctx, viewer)
	}
	if err := s.db.WithContext(ctx).Model(&userRow{}).Where("id = ?", viewer.UserID).Updates(updates).Error; err != nil {
		return nil, err
	}
	return s.Me(ctx, viewer)
}

func (s *Service) ChangePassword(ctx context.Context, viewer Principal, current, next string) error {
	var user userRow
	if err := s.db.WithContext(ctx).Where("id = ? AND status = ?", viewer.UserID, "active").First(&user).Error; err != nil {
		return ErrInvalidCredentials
	}
	if user.PasswordHash == nil || !checkPassword(*user.PasswordHash, current) {
		return ErrInvalidCredentials
	}
	hash, err := HashPassword(next)
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).Model(&userRow{}).Where("id = ?", user.ID).Updates(map[string]any{
		"password_hash": hash,
		"updated_at":    time.Now().UTC(),
	}).Error
}
