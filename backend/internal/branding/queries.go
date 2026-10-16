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
