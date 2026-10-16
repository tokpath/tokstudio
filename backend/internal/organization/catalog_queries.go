package organization

import (
	"context"
	"gorm.io/gorm"
	"time"
)

// CatalogNotificationAudience is trusted infrastructure, never a customer list
// API for a supplier. Only stable IDs are returned for notice fanout.
func (s *Service) CatalogNotificationAudience(ctx context.Context, tx *gorm.DB, operator, cursor string, limit int) ([]string, error) {
	if !validID(operator) || limit < 1 || limit > 1000 {
		return nil, ErrInvalidInput
	}
	if cursor != "" && !validID(cursor) {
		return nil, ErrInvalidInput
	}
	out := []string{}
	e := s.dbFor(ctx, tx).Raw(`SELECT id FROM(SELECT o.customer_id::text AS id FROM organization_service_ownerships o JOIN organization_customers c ON c.id=o.customer_id WHERE o.operator_id=? AND o.ended_at IS NULL AND c.status IN ('active','closing') UNION SELECT staff_id::text AS id FROM organization_staff_memberships WHERE operator_id=? AND status='active')audience WHERE id>? ORDER BY id LIMIT ?`, operator, operator, cursor, limit).Scan(&out).Error
	return out, e
}
func (s *Service) CatalogNotificationOperators(ctx context.Context, tx *gorm.DB, ancestor string) ([]string, error) {
	if !validID(ancestor) {
		return nil, ErrInvalidInput
	}
	out := []string{}
	e := s.dbFor(ctx, tx).Raw(`WITH RECURSIVE operators AS(SELECT id,parent_operator_id,0 AS depth FROM organization_operators WHERE id=? UNION ALL SELECT child.id,child.parent_operator_id,p.depth+1 FROM organization_operators child JOIN operators p ON child.parent_operator_id=p.id WHERE p.depth<2)SELECT id::text FROM operators ORDER BY id`, ancestor).Scan(&out).Error
	return out, e
}

// CatalogNotificationAudienceAt excludes accounts/staff created after plan
// publication; later entrants see the already public planned terms on entry.
func (s *Service) CatalogNotificationAudienceAt(ctx context.Context, tx *gorm.DB, operator, cursor string, at time.Time, limit int) ([]string, error) {
	if !validID(operator) || limit < 1 || limit > 1000 || at.IsZero() {
		return nil, ErrInvalidInput
	}
	if cursor != "" && !validID(cursor) {
		return nil, ErrInvalidInput
	}
	out := []string{}
	e := s.dbFor(ctx, tx).Raw(`SELECT id FROM(SELECT o.customer_id::text AS id FROM organization_service_ownerships o JOIN organization_customers c ON c.id=o.customer_id WHERE o.operator_id=? AND o.ended_at IS NULL AND c.status IN ('active','closing') AND c.registered_at<=? UNION SELECT staff_id::text AS id FROM organization_staff_memberships WHERE operator_id=? AND status='active' AND created_at<=?)audience WHERE id>? ORDER BY id LIMIT ?`, operator, at, operator, at, cursor, limit).Scan(&out).Error
	return out, e
}
