package identity

import (
	"context"
	"net/url"
	"strings"
	"unicode"
)

// SafeReturnPath validates a site-local path before it can be used as a redirect.
func SafeReturnPath(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" || !strings.HasPrefix(value, "/") || strings.HasPrefix(value, "//") || strings.Contains(value, "\\") {
		return ""
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return ""
		}
	}
	u, err := url.Parse(value)
	if err != nil || u.IsAbs() || u.Host != "" {
		return ""
	}
	decoded := u.Path
	for i := 0; i < 3; i++ {
		if strings.HasPrefix(decoded, "//") || strings.Contains(decoded, "\\") {
			return ""
		}
		for _, r := range decoded {
			if unicode.IsControl(r) {
				return ""
			}
		}
		next, err := url.PathUnescape(decoded)
		if err != nil {
			return ""
		}
		if next == decoded {
			break
		}
		decoded = next
	}
	return value
}

// A brand selected by the public host constrains signup, never existing accounts.
func (s *Service) resolveRegistration(ctx context.Context, code, brandID string) (resolvedPromotion, error) {
	if strings.TrimSpace(code) != "" || brandID == "" || brandID == OfficialBrandID {
		resolved, err := s.resolvePromotion(ctx, code)
		if err != nil {
			return resolvedPromotion{}, err
		}
		if brandID != "" && resolved.BrandID != brandID {
			return resolvedPromotion{}, ErrPromotionInvalid
		}
		return resolved, nil
	}
	var channel channelRow
	if err := s.db.WithContext(ctx).Where("brand_id = ? AND type = ? AND status = ?", brandID, ChannelTypeC, "active").First(&channel).Error; err != nil {
		return resolvedPromotion{}, ErrPromotionInvalid
	}
	return resolvedPromotion{ChannelID: channel.ID, BrandID: brandID}, nil
}
