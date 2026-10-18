package branding

import (
	"context"
	"gorm.io/gorm"
)

func (s *Service) ListDomains(ctx context.Context, tx *gorm.DB, actor, brandID string) ([]Domain, error) {
	if !validID(brandID) {
		return nil, ErrInvalidInput
	}
	if err := s.directory.RequireBrandBranding(ctx, tx, actor, brandID); err != nil {
		return nil, err
	}
	var rows []domainRow
	if err := s.dbFor(ctx, tx).Where("brand_id=?", brandID).Order("created_at,id").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]Domain, len(rows))
	for i, r := range rows {
		out[i] = Domain(r)
	}
	return out, nil
}

// ThemeConfiguration returns the saved overrides, preserving inheritance rather
// than turning inherited colors into explicit overrides when the form is saved.
func (s *Service) ThemeConfiguration(ctx context.Context, tx *gorm.DB, actor, scope, target string) (ThemeConfig, error) {
	if scope == "channel" {
		if err := s.directory.RequireChannelBranding(ctx, tx, actor, target); err != nil {
			return ThemeConfig{}, err
		}
	} else if scope == "brand" {
		if err := s.directory.RequireBrandBranding(ctx, tx, actor, target); err != nil {
			return ThemeConfig{}, err
		}
	} else {
		return ThemeConfig{}, ErrInvalidInput
	}
	return s.themeConfig(ctx, tx, scope, target)
}
