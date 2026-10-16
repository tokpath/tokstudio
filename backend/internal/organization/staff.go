package organization

import (
	"context"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var rolePermissions = map[string][]string{
	"owner":      {"*"},
	"operations": {"organization.read", "organization.create", "cooperation.manage", "business.manage", "staff.read", "branding.manage", "branding.manage_channels", "branding.manage_oem", "customers.read", "sales.manage", "marketing.manage", "operations.manage", "reports.summary", "reports.view"},
	"finance":    {"organization.read", "staff.read", "ledger.read", "payments.manage", "refunds.manage", "withdrawals.manage", "invoices.manage", "reports.financial", "reports.view", "reports.export"},
	"support":    {"organization.read", "customers.read", "support.manage"},
	"readonly":   {"organization.read", "staff.read", "reports.summary"},
}

// RequirePermission always checks a membership in the exact requested space.
// Owning a platform or supplier space does not grant customer or money access
// to a child. Casbin evaluates actions; the SQL membership supplies data scope.
func (s *Service) RequirePermission(ctx context.Context, tx *gorm.DB, staffID, operatorID, action string) error {
	if !validID(staffID) || !validID(operatorID) || action == "" {
		return ErrForbidden
	}
	var m membershipRow
	err := s.dbFor(ctx, tx).First(&m, "staff_id = ? AND operator_id = ? AND status = 'active'", staffID, operatorID).Error
	if err == gorm.ErrRecordNotFound {
		return ErrForbidden
	}
	if err != nil {
		return err
	}
	allowed, err := s.enforcer.Enforce(m.Role, action)
	if err != nil {
		return err
	}
	if allowed {
		return nil
	}
	var count int64
	if err = s.dbFor(ctx, tx).Table("organization_staff_grants").Where("membership_id = ? AND action = ?", m.ID, action).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	return ErrForbidden
}

func (s *Service) ListMemberships(ctx context.Context, tx *gorm.DB, actorID, operatorID string) ([]Membership, error) {
	if err := s.RequirePermission(ctx, tx, actorID, operatorID, "staff.read"); err != nil {
		return nil, err
	}
	var rows []membershipRow
	if err := s.dbFor(ctx, tx).Where("operator_id = ?", operatorID).Order("created_at,id").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]Membership, len(rows))
	for i, r := range rows {
		out[i] = Membership(r)
	}
	return out, nil
}

func (s *Service) InviteStaff(ctx context.Context, tx *gorm.DB, actorID, operatorID, email, role string) (out StaffInvitation, err error) {
	email, err = NormalizeEmail(email)
	if err != nil {
		return out, err
	}
	if _, ok := rolePermissions[role]; !ok {
		return out, ErrInvalidInput
	}
	err = s.withTx(ctx, tx, func(tx *gorm.DB) error {
		o, err := s.GetOperator(ctx, tx, operatorID)
		if err != nil {
			return err
		}
		if err = s.RequirePermission(ctx, tx, actorID, operatorID, "staff.manage"); err != nil {
			// Opening a child authorizes its first owner invitation only. It
			// never authorizes management of that child's other staff.
			if role != "owner" || o.ParentOperatorID == nil {
				return ErrForbidden
			}
			if err = s.RequirePermission(ctx, tx, actorID, *o.ParentOperatorID, "organization.create"); err != nil {
				return err
			}
			var count int64
			if err = tx.Model(&membershipRow{}).Where("operator_id = ? AND role='owner'", operatorID).Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				return ErrForbidden
			}
		} else {
			for _, action := range rolePermissions[role] {
				if err = s.RequirePermission(ctx, tx, actorID, operatorID, action); err != nil {
					return err
				}
			}
		}
		raw, err := token()
		if err != nil {
			return err
		}
		out = StaffInvitation{ID: newID(), OperatorID: operatorID, Email: email, Role: role, Token: raw, ExpiresAt: s.now().Add(72 * time.Hour)}
		if err = tx.Exec(`INSERT INTO organization_staff_invitations(id,operator_id,email_normalized,role,token_hash,expires_at,created_at) VALUES(?,?,?,?,?,?,?)`, out.ID, operatorID, email, role, tokenHash(raw), out.ExpiresAt, s.now()).Error; err != nil {
			return err
		}
		return s.event(tx, operatorID, actorID, "staff.invited", "", map[string]any{"invitation_id": out.ID, "email": email, "role": role})
	})
	return
}

func (s *Service) AcceptStaffInvitation(ctx context.Context, tx *gorm.DB, rawToken string, staff VerifiedStaff) (out Membership, err error) {
	if !staff.EmailVerified || !validID(staff.ID) {
		return out, ErrForbidden
	}
	email, err := NormalizeEmail(staff.Email)
	if err != nil {
		return out, err
	}
	err = s.withTx(ctx, tx, func(tx *gorm.DB) error {
		var r struct {
			ID, OperatorID, EmailNormalized, Role, TokenHash string
			ExpiresAt                                        time.Time
			AcceptedBy                                       *string
			AcceptedAt                                       *time.Time
		}
		if err := tx.Table("organization_staff_invitations").Clauses(clause.Locking{Strength: "UPDATE"}).First(&r, "token_hash = ?", tokenHash(rawToken)).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return ErrInvalidVerification
			}
			return err
		}
		if r.EmailNormalized != email {
			return ErrForbidden
		}
		if r.AcceptedBy != nil {
			if *r.AcceptedBy != staff.ID {
				return ErrConflict
			}
			var m membershipRow
			if err := tx.First(&m, "operator_id = ? AND staff_id = ?", r.OperatorID, staff.ID).Error; err != nil {
				return err
			}
			out = Membership(m)
			return nil
		}
		if !r.ExpiresAt.After(s.now()) {
			return ErrInvalidVerification
		}
		m := membershipRow{ID: newID(), OperatorID: r.OperatorID, StaffID: staff.ID, Role: r.Role, Status: "active", CreatedAt: s.now()}
		if err = tx.Create(&m).Error; err != nil {
			return err
		}
		if err = tx.Table("organization_staff_invitations").Where("id = ?", r.ID).Updates(map[string]any{"accepted_by": staff.ID, "accepted_at": s.now()}).Error; err != nil {
			return err
		}
		if r.Role == "owner" {
			if err = tx.Model(&operatorRow{}).Where("id = ? AND lifecycle = 'draft'", r.OperatorID).Update("lifecycle", "active").Error; err != nil {
				return err
			}
		}
		out = Membership(m)
		return s.event(tx, r.OperatorID, staff.ID, "staff.invitation_accepted", r.ID, map[string]any{"membership_id": m.ID, "role": m.Role})
	})
	return
}

type SetMembershipInput struct {
	ActorStaffID, OperatorID, StaffID, Role, Status string
	Grants                                          []string
}

func (s *Service) SetMembership(ctx context.Context, tx *gorm.DB, in SetMembershipInput) error {
	if !validID(in.StaffID) || (in.Status != "active" && in.Status != "suspended") {
		return ErrInvalidInput
	}
	if _, ok := rolePermissions[in.Role]; !ok && in.Role != "custom" {
		return ErrInvalidInput
	}
	return s.withTx(ctx, tx, func(tx *gorm.DB) error {
		if err := s.RequirePermission(ctx, tx, in.ActorStaffID, in.OperatorID, "staff.manage"); err != nil {
			return err
		}
		for _, action := range rolePermissions[in.Role] {
			if err := s.RequirePermission(ctx, tx, in.ActorStaffID, in.OperatorID, action); err != nil {
				return err
			}
		}
		// Serialize owner changes for this whole space, including a last-owner check.
		var o operatorRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&o, "id = ?", in.OperatorID).Error; err != nil {
			return err
		}
		var m membershipRow
		if err := tx.First(&m, "operator_id = ? AND staff_id = ?", in.OperatorID, in.StaffID).Error; err != nil {
			return err
		}
		if m.Role == "owner" && m.Status == "active" && (in.Role != "owner" || in.Status != "active") {
			var count int64
			if err := tx.Model(&membershipRow{}).Where("operator_id = ? AND role='owner' AND status='active'", in.OperatorID).Count(&count).Error; err != nil {
				return err
			}
			if count <= 1 {
				return ErrConflict
			}
		}
		for _, a := range in.Grants {
			if strings.TrimSpace(a) == "" || a == "*" {
				return ErrInvalidInput
			}
			if err := s.RequirePermission(ctx, tx, in.ActorStaffID, in.OperatorID, a); err != nil {
				return err
			}
		}
		if err := tx.Model(&m).Updates(map[string]any{"role": in.Role, "status": in.Status}).Error; err != nil {
			return err
		}
		if err := tx.Exec(`DELETE FROM organization_staff_grants WHERE membership_id=?`, m.ID).Error; err != nil {
			return err
		}
		for _, a := range in.Grants {
			if err := tx.Exec(`INSERT INTO organization_staff_grants(membership_id,action) VALUES(?,?) ON CONFLICT DO NOTHING`, m.ID, a).Error; err != nil {
				return err
			}
		}
		return s.event(tx, in.OperatorID, in.ActorStaffID, "staff.membership_changed", "", in)
	})
}

// RequireChannelBranding grants only the channel's immediate supplier access
// to appearance configuration, with no implied financial or customer access.
func (s *Service) RequireChannelBranding(ctx context.Context, tx *gorm.DB, staffID, channelID string) error {
	o, err := s.GetOperator(ctx, tx, channelID)
	if err != nil {
		return err
	}
	if o.Kind != Channel || o.ParentOperatorID == nil {
		return ErrInvalidRelationship
	}
	return s.RequirePermission(ctx, tx, staffID, *o.ParentOperatorID, "branding.manage_channels")
}
func (s *Service) RequireBrandBranding(ctx context.Context, tx *gorm.DB, staffID, brandID string) error {
	b, err := s.GetBrand(ctx, tx, brandID)
	if err != nil {
		return err
	}
	if err = s.RequirePermission(ctx, tx, staffID, b.OwnerOperatorID, "branding.manage"); err == nil {
		return nil
	}
	if b.Kind != "oem" {
		return ErrForbidden
	}
	o, err := s.GetOperator(ctx, tx, b.OwnerOperatorID)
	if err != nil {
		return err
	}
	if o.ParentOperatorID == nil {
		return ErrForbidden
	}
	return s.RequirePermission(ctx, tx, staffID, *o.ParentOperatorID, "branding.manage_oem")
}
