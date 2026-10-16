package organization

import (
	"context"
	"gorm.io/gorm"
)

// ListScopeFreezes returns the latest persisted decision for each action,
// including disabled decisions needed for versioned re-enabling. Earlier
// decisions remain in ListAuditEvents; they are not fabricated from this view.
func (s *Service) ListScopeFreezes(ctx context.Context, tx *gorm.DB, actor, operator string) ([]ScopeFreeze, error) {
	if !validID(operator) {
		return nil, ErrInvalidInput
	}
	if e := s.RequirePermission(ctx, tx, actor, operator, "security.manage"); e != nil {
		return nil, e
	}
	op, e := s.GetOperator(ctx, tx, operator)
	if e != nil {
		return nil, e
	}
	brand, e := s.GetBrand(ctx, tx, op.BrandID)
	if e != nil {
		return nil, e
	}
	rows := []ScopeFreeze{}
	q := s.dbFor(ctx, tx).Table("organization_scope_freezes f").Select("f.*").Where("(f.scope_kind='operator' AND f.scope_id=?) OR (f.scope_kind='customer' AND EXISTS(SELECT 1 FROM organization_service_ownerships o WHERE o.customer_id=f.scope_id AND o.operator_id=? AND o.ended_at IS NULL)) OR (f.scope_kind='brand' AND f.scope_id=? AND ?=?)", operator, operator, brand.ID, operator, brand.OwnerOperatorID)
	e = q.Order("f.updated_at DESC,f.scope_kind,f.scope_id,f.action").Limit(100).Scan(&rows).Error
	return rows, e
}
