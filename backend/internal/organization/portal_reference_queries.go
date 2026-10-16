package organization

import (
	"context"
	"gorm.io/gorm"
	"strings"
)

type CustomerReference struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
}

// ReferenceCustomers supplies only the minimal selector identity in the exact
// operator's current service scope. Finance permission does not grant descendants.
func (s *Service) ReferenceCustomers(ctx context.Context, tx *gorm.DB, actor, operator, q, cursor string, limit int) ([]CustomerReference, string, error) {
	if limit < 1 || limit > 50 || len(q) > 200 || !validID(operator) || (cursor != "" && !validID(cursor)) {
		return nil, "", ErrInvalidInput
	}
	allowed := false
	mayEmail := false
	for _, permission := range []string{"customers.read", "refunds.manage", "withdrawals.manage", "marketing.manage", "security.manage"} {
		e := s.RequirePermission(ctx, tx, actor, operator, permission)
		if e == nil {
			allowed = true
			mayEmail = permission == "customers.read" || permission == "refunds.manage" || permission == "withdrawals.manage"
			break
		}
		if e != ErrForbidden {
			return nil, "", e
		}
	}
	if !allowed {
		return nil, "", ErrForbidden
	}
	rows := []CustomerReference{}
	query := s.dbFor(ctx, tx).Table("organization_service_ownerships own").Select("c.id,c.display_name,e.email_normalized AS email").Joins("JOIN organization_customers c ON c.id=own.customer_id").Joins("JOIN organization_customer_identities e ON e.customer_id=c.id AND e.is_current=true").Where("own.operator_id=? AND own.ended_at IS NULL AND c.id::text>?", operator, cursor)
	if text := strings.TrimSpace(q); text != "" {
		pattern := "%" + text + "%"
		if mayEmail {
			query = query.Where("(c.display_name ILIKE ? OR e.email_normalized ILIKE ? OR c.id::text ILIKE ?)", pattern, pattern, pattern)
		} else {
			query = query.Where("(c.display_name ILIKE ? OR c.id::text ILIKE ?)", pattern, pattern)
		}
	}
	if e := query.Order("c.id").Limit(limit + 1).Scan(&rows).Error; e != nil {
		return nil, "", e
	}
	next := ""
	if len(rows) > limit {
		rows = rows[:limit]
		next = rows[limit-1].ID
	}
	if !mayEmail {
		for i := range rows {
			rows[i].Email = ""
		}
	}
	return rows, next, nil
}
