package organization

import (
	"context"
	"gorm.io/gorm"
)

// OperatorNotificationStaff is a trusted system recipient lookup. It must never
// be exposed as a directory endpoint or used as staff authorization.
func (s *Service) OperatorNotificationStaff(ctx context.Context, tx *gorm.DB, operator string) ([]string, error) {
	if !validID(operator) {
		return nil, ErrInvalidInput
	}
	ids := []string{}
	e := s.dbFor(ctx, tx).Table("organization_staff_memberships").Distinct("staff_id").Where("operator_id=? AND status='active' AND role IN ('owner','finance','operations')", operator).Order("staff_id").Pluck("staff_id", &ids).Error
	return ids, e
}
