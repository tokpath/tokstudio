package organization

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// scopeFreezeSpace follows current responsibility, never the supplier tree.
// Brand decisions belong only to the operator that owns that brand.
func scopeFreezeSpace(q *gorm.DB, operator string) *gorm.DB {
	return q.Where(`((f.scope_kind='operator' AND f.scope_id=?) OR
 (f.scope_kind='customer' AND EXISTS(SELECT 1 FROM organization_service_ownerships o WHERE o.customer_id=f.scope_id AND o.operator_id=? AND o.ended_at IS NULL)) OR
 (f.scope_kind='brand' AND EXISTS(SELECT 1 FROM organization_brands b WHERE b.id=f.scope_id AND b.owner_operator_id=?)))`, operator, operator, operator)
}

// ScopeFreezesDueForReview is a trusted worker query, not a directory endpoint.
// Lock the actual decisions until notifications commit, so a concurrent release
// or reschedule cannot be followed by a notice based on a stale decision.
func (s *Service) ScopeFreezesDueForReview(ctx context.Context, tx *gorm.DB, operator string, at time.Time) ([]ScopeFreeze, error) {
	if !validID(operator) || tx == nil {
		return nil, ErrInvalidInput
	}
	rows := []ScopeFreeze{}
	e := scopeFreezeSpace(tx.WithContext(ctx).Table("organization_scope_freezes f").Select("f.*"), operator).
		Where("f.enabled AND f.review_due_at<=?", at).
		Clauses(clause.Locking{Strength: "UPDATE", Table: clause.Table{Name: "f"}}).
		Order("f.scope_kind,f.scope_id,f.action").Find(&rows).Error
	return rows, e
}

// ScopeFreezeReviewStaff returns the original reviewer if still authorized;
// otherwise local owners take responsibility. Escalations also reach all local
// owners. A parent owner, finance role or former reviewer is not implied here.
func (s *Service) ScopeFreezeReviewStaff(ctx context.Context, tx *gorm.DB, operator, actor string, escalated bool) ([]string, error) {
	if !validID(operator) {
		return nil, ErrInvalidInput
	}
	ids := []string{}
	e := s.RequirePermission(ctx, tx, actor, operator, "security.manage")
	if e == nil {
		ids = append(ids, actor)
	} else if !errors.Is(e, ErrForbidden) {
		return nil, e
	}
	if len(ids) == 0 || escalated {
		owners := []string{}
		if e = s.dbFor(ctx, tx).Table("organization_staff_memberships").Where("operator_id=? AND role='owner' AND status='active'", operator).Order("staff_id").Pluck("staff_id", &owners).Error; e != nil {
			return nil, e
		}
		for _, id := range owners {
			if id != actor || len(ids) == 0 {
				ids = append(ids, id)
			}
		}
	}
	return ids, nil
}

// ScopeFreezeReviewCurrent rechecks current authority and the exact version at
// mail submission, including decisions resolved while an email was queued.
func (s *Service) ScopeFreezeReviewCurrent(ctx context.Context, tx *gorm.DB, operator, recipient, kind, id, action string, version int64) (bool, error) {
	if !validID(operator) || !validID(id) || !freezeAction(action) || version < 1 {
		return false, ErrInvalidInput
	}
	if e := s.RequirePermission(ctx, tx, recipient, operator, "security.manage"); e != nil {
		if errors.Is(e, ErrForbidden) {
			return false, nil
		}
		return false, e
	}
	var count int64
	e := scopeFreezeSpace(s.dbFor(ctx, tx).Table("organization_scope_freezes f"), operator).
		Where("f.scope_kind=? AND f.scope_id=? AND f.action=? AND f.version=? AND f.enabled AND f.review_due_at<=clock_timestamp()", kind, id, action, version).Count(&count).Error
	return count == 1, e
}

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
