package branding

import (
	"context"

	"gorm.io/gorm"
)

// APIEndpoint is derived from verified, enabled domain evidence, never a
// manually entered URL on a brand. If several domains are available, the first
// added available domain wins. Disabling it selects the next available domain.
// No active domain is a normal configuration state and returns an empty URL.
func (s *Service) APIEndpoint(ctx context.Context, tx *gorm.DB, brandID string) (string, error) {
	if !validID(brandID) {
		return "", ErrInvalidInput
	}
	b, err := s.directory.GetBrand(ctx, tx, brandID)
	if err != nil {
		return "", err
	}
	if b.Status != "active" {
		return "", nil
	}
	var rows []domainRow
	err = s.dbFor(ctx, tx).Where("brand_id=? AND status='active' AND verified_at IS NOT NULL AND tls_ready=true AND provider_reference<>''", brandID).Order("created_at,id").Limit(1).Find(&rows).Error
	if err != nil || len(rows) == 0 {
		return "", err
	}
	return "https://" + rows[0].Hostname + "/v1", nil
}
