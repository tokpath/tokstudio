package organization

import (
	"context"
	"sort"
	"strings"

	"gorm.io/gorm"
)

type StaffWorkspace struct {
	Membership  Membership `json:"membership"`
	Operator    Operator   `json:"operator"`
	Brand       Brand      `json:"brand"`
	Permissions []string   `json:"permissions"`
}

// MembershipsForStaff is an authenticated staff self-query. Callers must use
// the staff ID from their verified session, never a request-selected identity.
func (s *Service) MembershipsForStaff(ctx context.Context, tx *gorm.DB, staffID string) ([]StaffWorkspace, error) {
	if !validID(staffID) {
		return nil, ErrInvalidInput
	}
	var rows []membershipRow
	if err := s.dbFor(ctx, tx).Where("staff_id=? AND status='active'", staffID).Order("created_at,id").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]StaffWorkspace, 0, len(rows))
	for _, r := range rows {
		operator, err := s.GetOperator(ctx, tx, r.OperatorID)
		if err != nil {
			return nil, err
		}
		brand, err := s.GetBrand(ctx, tx, operator.BrandID)
		if err != nil {
			return nil, err
		}
		permissions := map[string]bool{}
		for _, a := range rolePermissions[r.Role] {
			permissions[a] = true
		}
		var grants []struct{ Action string }
		if err = s.dbFor(ctx, tx).Table("organization_staff_grants").Select("action").Where("membership_id=?", r.ID).Find(&grants).Error; err != nil {
			return nil, err
		}
		for _, g := range grants {
			permissions[g.Action] = true
		}
		actions := make([]string, 0, len(permissions))
		for a := range permissions {
			actions = append(actions, a)
		}
		sort.Strings(actions)
		out = append(out, StaffWorkspace{Membership: Membership(r), Operator: operator, Brand: brand, Permissions: actions})
	}
	return out, nil
}
func (s *Service) CustomerEmail(ctx context.Context, tx *gorm.DB, customerID string) (string, error) {
	if !validID(customerID) {
		return "", ErrInvalidInput
	}
	var row struct{ EmailNormalized string }
	err := s.dbFor(ctx, tx).Table("organization_customer_identities").Select("email_normalized").Where("customer_id=? AND is_current=true", customerID).Take(&row).Error
	return row.EmailNormalized, classify(err)
}
func (s *Service) CustomerInvite(ctx context.Context, tx *gorm.DB, customerID string) (string, error) {
	if !validID(customerID) {
		return "", ErrInvalidInput
	}
	var row codeRow
	err := s.dbFor(ctx, tx).Where("customer_id=? AND kind='user'", customerID).Take(&row).Error
	return row.Code, classify(err)
}

type ScopedCustomer struct {
	Customer  Customer  `json:"customer"`
	Email     string    `json:"email"`
	Ownership Ownership `json:"ownership"`
}

func (s *Service) ListCustomersInScope(ctx context.Context, tx *gorm.DB, actor, operator string, limit int, cursor string, search ...string) ([]ScopedCustomer, error) {
	if limit < 1 || limit > 100 || (cursor != "" && !validID(cursor)) {
		return nil, ErrInvalidInput
	}
	if err := s.RequirePermission(ctx, tx, actor, operator, "customers.read"); err != nil {
		return nil, err
	}
	var rows []ownershipRow
	q := s.dbFor(ctx, tx).Where("operator_id=? AND ended_at IS NULL", operator).Order("customer_id").Limit(limit)
	if len(search) > 0 && strings.TrimSpace(search[0]) != "" {
		term := "%" + strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(strings.TrimSpace(search[0])) + "%"
		q = q.Where(`customer_id IN (SELECT c.id FROM organization_customers c JOIN organization_customer_identities i ON i.customer_id=c.id AND i.is_current=true WHERE c.display_name ILIKE ? OR i.email_normalized ILIKE ? OR c.id::text=?)`, term, term, strings.TrimSpace(search[0]))
	}
	if cursor != "" {
		q = q.Where("customer_id>?", cursor)
	}
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]ScopedCustomer, 0, len(rows))
	for _, r := range rows {
		customer, err := s.GetCustomer(ctx, tx, r.CustomerID)
		if err != nil {
			return nil, err
		}
		email, err := s.CustomerEmail(ctx, tx, r.CustomerID)
		if err != nil {
			return nil, err
		}
		out = append(out, ScopedCustomer{Customer: customer, Email: email, Ownership: Ownership(r)})
	}
	return out, nil
}
func (s *Service) ListSupplies(ctx context.Context, tx *gorm.DB, actor, operator string) ([]SupplyAgreement, error) {
	if err := s.RequirePermission(ctx, tx, actor, operator, "organization.read"); err != nil {
		return nil, err
	}
	var rows []supplyRow
	if err := s.dbFor(ctx, tx).Where("supplier_id=? OR buyer_id=?", operator, operator).Order("created_at,id").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]SupplyAgreement, len(rows))
	for i, r := range rows {
		out[i] = SupplyAgreement(r)
	}
	return out, nil
}
