package identity

import (
	"context"
	"encoding/json"
	"errors"
	"net/mail"
	"slices"
	"strings"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/platform/id"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrStaffInput     = errors.New("invalid staff input")
	ErrStaffTarget    = errors.New("staff account belongs to another company or is unavailable")
	ErrStaffExists    = errors.New("staff member already exists")
	ErrStaffLastAdmin = errors.New("at least one active administrator must remain")
)

var platformStaffRoles = []string{"platform_admin", "ops_admin", "finance_admin", "tech_admin", "audit_readonly"}
var oemStaffRoles = []string{"channel_admin", "oem_ops", "oem_finance", "oem_audit"}

func (p Principal) IsOEMStaff() bool     { return p.HasRole("oem_ops", "oem_finance", "oem_audit") }
func (p Principal) IsChannelStaff() bool { return p.HasRole("channel_admin") || p.IsOEMStaff() }

type StaffScope struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}
type staffRow struct {
	UserID    string `gorm:"primaryKey"`
	ScopeType string
	ScopeID   string
	RolesJSON json.RawMessage `gorm:"column:roles_json"`
	Status    string
	CreatedBy string
	UpdatedBy string
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (staffRow) TableName() string { return "identity_staff_members" }

type StaffView struct {
	UserID         string    `json:"user_id"`
	Email          string    `json:"email"`
	DisplayName    string    `json:"display_name"`
	Roles          []string  `json:"roles"`
	Status         string    `json:"status"`
	CreatedBy      string    `json:"created_by"`
	UpdatedBy      string    `json:"updated_by"`
	CreatedByEmail string    `json:"created_by_email"`
	UpdatedByEmail string    `json:"updated_by_email"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}
type StaffInput struct {
	Email       string   `json:"email"`
	DisplayName string   `json:"display_name"`
	Password    string   `json:"password"`
	Roles       []string `json:"roles"`
	Status      string   `json:"status"`
}

// Scope comes from the authenticated account, never from request fields.
func (s *Service) StaffScope(ctx context.Context, p Principal, oem bool) (StaffScope, error) {
	if !oem {
		if !p.IsPlatformAdmin() {
			return StaffScope{}, ErrChannelImmutable
		}
		return StaffScope{"platform", "*"}, nil
	}
	if !p.HasRole("channel_admin") {
		return StaffScope{}, ErrChannelImmutable
	}
	ch, err := s.lookupChannel(ctx, p.ChannelOrgID)
	if err != nil || ch.Type != ChannelTypeC {
		return StaffScope{}, ErrChannelImmutable
	}
	return StaffScope{"channel", ch.ID}, nil
}
func StaffRoleOptions(scope StaffScope) []string {
	if scope.Type == "platform" {
		return slices.Clone(platformStaffRoles)
	}
	return slices.Clone(oemStaffRoles)
}
func validateStaffRoles(scope StaffScope, roles []string) ([]string, error) {
	allowed := StaffRoleOptions(scope)
	result := []string{}
	for _, role := range roles {
		if !slices.Contains(allowed, role) {
			return nil, ErrStaffInput
		}
		if !slices.Contains(result, role) {
			result = append(result, role)
		}
	}
	if len(result) == 0 {
		return nil, ErrStaffInput
	}
	slices.Sort(result)
	return result, nil
}
func (s *Service) ListStaff(ctx context.Context, scope StaffScope) ([]StaffView, error) {
	items := make([]StaffView, 0)
	err := staffQuery(s.db.WithContext(ctx), scope).Order("m.created_at, m.user_id").Scan(&items).Error
	// Roles are decoded independently of the display joins.
	for i := range items {
		var row staffRow
		if err == nil {
			err = s.db.WithContext(ctx).Where("user_id = ?", items[i].UserID).First(&row).Error
		}
		if err != nil {
			return nil, err
		}
		if err = json.Unmarshal(row.RolesJSON, &items[i].Roles); err != nil {
			return nil, err
		}
	}
	return items, err
}
func staffQuery(db *gorm.DB, scope StaffScope) *gorm.DB {
	return db.Table("identity_staff_members m").Select("m.user_id, u.email, u.display_name, m.status, m.created_by, m.updated_by, c.email AS created_by_email, e.email AS updated_by_email, m.created_at, m.updated_at").
		Joins("JOIN identity_users u ON u.id=m.user_id").Joins("JOIN identity_users c ON c.id=m.created_by").Joins("JOIN identity_users e ON e.id=m.updated_by").Where("m.scope_type = ? AND m.scope_id = ?", scope.Type, scope.ID)
}
func (s *Service) GetStaff(ctx context.Context, scope StaffScope, userID string) (*StaffView, error) {
	var view StaffView
	if err := staffQuery(s.db.WithContext(ctx), scope).Where("m.user_id = ?", userID).Take(&view).Error; err != nil {
		return nil, mapNotFound(err)
	}
	var row staffRow
	if err := s.db.WithContext(ctx).Where("user_id = ?", userID).First(&row).Error; err != nil {
		return nil, err
	}
	if err := json.Unmarshal(row.RolesJSON, &view.Roles); err != nil {
		return nil, err
	}
	return &view, nil
}

// Serialize company mutations, including administrator removal, and recheck the
// actor inside the transaction so a concurrent revocation cannot grant access.
func (s *Service) lockStaffScope(tx *gorm.DB, actor Principal, scope StaffScope) (channelRow, error) {
	channelID := scope.ID
	if scope.Type == "platform" {
		channelID = OfficialChannelID
	}
	var ch channelRow
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", channelID).First(&ch).Error; err != nil {
		return ch, err
	}
	var user userRow
	if err := tx.Where("id = ? AND status = 'active'", actor.UserID).First(&user).Error; err != nil {
		return ch, ErrChannelImmutable
	}
	scoped := *s
	scoped.db = tx
	current, err := scoped.loadPrincipal(tx.Statement.Context, user)
	if err != nil {
		return ch, err
	}
	derived, err := scoped.StaffScope(tx.Statement.Context, *current, scope.Type == "channel")
	if err != nil || derived != scope {
		return ch, ErrChannelImmutable
	}
	return ch, nil
}
func (s *Service) CreateStaffTx(tx *gorm.DB, actor Principal, scope StaffScope, in StaffInput) (*StaffView, error) {
	ch, err := s.lockStaffScope(tx, actor, scope)
	if err != nil {
		return nil, err
	}
	roles, err := validateStaffRoles(scope, in.Roles)
	if err != nil {
		return nil, err
	}
	email := normalizeEmail(in.Email)
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Address != email || len(email) > 254 || len(in.DisplayName) > 120 || len(in.Password) > 72 {
		return nil, ErrStaffInput
	}
	var user userRow
	err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("LOWER(email) = ?", email).First(&user).Error
	now := time.Now().UTC()
	if errors.Is(err, gorm.ErrRecordNotFound) {
		hash, hashErr := HashPassword(in.Password)
		if hashErr != nil {
			return nil, hashErr
		}
		user = userRow{ID: id.New("usr"), Email: email, DisplayName: strings.TrimSpace(in.DisplayName), PasswordHash: &hash, Status: UserStatusActive, ChannelOrgID: &ch.ID, BrandID: &ch.BrandID, Locale: DefaultLocale, CreatedAt: now, UpdatedAt: now}
		if err = tx.Create(&user).Error; err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	} else {
		if user.Status != UserStatusActive || deref(user.ChannelOrgID) != ch.ID {
			return nil, ErrStaffTarget
		}
		if in.Password != "" {
			return nil, ErrEmailTaken
		}
	}
	var existing int64
	if err := tx.Model(&staffRow{}).Where("user_id = ?", user.ID).Count(&existing).Error; err != nil {
		return nil, err
	}
	if existing > 0 {
		return nil, ErrStaffExists
	}
	var conflicts int64
	if err := tx.Table("identity_user_roles ur").Joins("JOIN identity_roles r ON r.id=ur.role_id").Where("ur.user_id = ? AND r.code <> 'end_user'", user.ID).Count(&conflicts).Error; err != nil {
		return nil, err
	}
	if conflicts > 0 {
		return nil, ErrStaffTarget
	}
	raw, _ := json.Marshal(roles)
	row := staffRow{UserID: user.ID, ScopeType: scope.Type, ScopeID: scope.ID, RolesJSON: raw, Status: "active", CreatedBy: actor.UserID, UpdatedBy: actor.UserID, CreatedAt: now, UpdatedAt: now}
	if err := tx.Create(&row).Error; err != nil {
		return nil, err
	}
	if err := applyStaffRolesTx(tx, row); err != nil {
		return nil, err
	}
	scoped := *s
	scoped.db = tx
	return scoped.GetStaff(tx.Statement.Context, scope, user.ID)
}
func (s *Service) UpdateStaffTx(tx *gorm.DB, actor Principal, scope StaffScope, userID string, in StaffInput) (*StaffView, *StaffView, error) {
	if _, err := s.lockStaffScope(tx, actor, scope); err != nil {
		return nil, nil, err
	}
	if userID == actor.UserID {
		return nil, nil, ErrSelfAction
	}
	scoped := *s
	scoped.db = tx
	before, err := scoped.GetStaff(tx.Statement.Context, scope, userID)
	if err != nil {
		return nil, nil, err
	}
	var row staffRow
	if err := tx.Where("user_id = ?", userID).First(&row).Error; err != nil {
		return nil, nil, err
	}
	roles := before.Roles
	if in.Roles != nil {
		roles, err = validateStaffRoles(scope, in.Roles)
		if err != nil {
			return nil, nil, err
		}
	}
	status := row.Status
	if in.Status != "" {
		status = in.Status
	}
	if status != "active" && status != "disabled" {
		return nil, nil, ErrStaffInput
	}
	adminRole := "platform_admin"
	if scope.Type == "channel" {
		adminRole = "channel_admin"
	}
	if before.Status == "active" && slices.Contains(before.Roles, adminRole) && (status != "active" || !slices.Contains(roles, adminRole)) {
		var count int64
		if err := tx.Table("identity_staff_members m").Joins("JOIN identity_users u ON u.id=m.user_id").Where("m.scope_type = ? AND m.scope_id = ? AND m.status = 'active' AND u.status = 'active' AND m.roles_json @> ?::jsonb", scope.Type, scope.ID, `["`+adminRole+`"]`).Count(&count).Error; err != nil {
			return nil, nil, err
		}
		if count <= 1 {
			return nil, nil, ErrStaffLastAdmin
		}
	}
	row.Status = status
	row.RolesJSON, _ = json.Marshal(roles)
	row.UpdatedAt = time.Now().UTC()
	row.UpdatedBy = actor.UserID
	if err := tx.Save(&row).Error; err != nil {
		return nil, nil, err
	}
	if err := applyStaffRolesTx(tx, row); err != nil {
		return nil, nil, err
	}
	// A disabled account cannot keep its old login sessions after reactivation.
	if status == "disabled" {
		if err := tx.Model(&apiKeyRow{}).Where("user_id = ? AND status = 'active'", userID).Update("status", "disabled").Error; err != nil {
			return nil, nil, err
		}
		if err := tx.Model(&tokenRow{}).Where("user_id = ? AND status = 'active'", userID).Update("status", "revoked").Error; err != nil {
			return nil, nil, err
		}
	}
	after, err := scoped.GetStaff(tx.Statement.Context, scope, userID)
	return before, after, err
}
func applyStaffRolesTx(tx *gorm.DB, row staffRow) error {
	var roles []string
	if err := json.Unmarshal(row.RolesJSON, &roles); err != nil {
		return err
	}
	var ids []string
	if err := tx.Model(&roleRow{}).Where("code IN ?", StaffRoleOptions(StaffScope{row.ScopeType, row.ScopeID})).Pluck("id", &ids).Error; err != nil {
		return err
	}
	if err := tx.Where("user_id = ? AND role_id IN ? AND scope_type = ? AND scope_id = ?", row.UserID, ids, row.ScopeType, row.ScopeID).Delete(&userRoleRow{}).Error; err != nil {
		return err
	}
	if row.Status != "active" {
		return nil
	}
	for _, code := range roles {
		var role roleRow
		if err := tx.Where("code = ?", code).First(&role).Error; err != nil {
			return err
		}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&userRoleRow{UserID: row.UserID, RoleID: role.ID, ScopeType: row.ScopeType, ScopeID: row.ScopeID}).Error; err != nil {
			return err
		}
	}
	return nil
}
func (s *Service) staffDisabled(ctx context.Context, userID string) (bool, error) {
	var count int64
	err := s.db.WithContext(ctx).Model(&staffRow{}).Where("user_id = ? AND status = 'disabled'", userID).Count(&count).Error
	return count > 0, err
}

func backfillStaffTx(tx *gorm.DB) error {
	return tx.Exec(`INSERT INTO identity_staff_members (user_id, scope_type, scope_id, roles_json, created_by, updated_by)
 SELECT ur.user_id, ur.scope_type, ur.scope_id, jsonb_agg(r.code ORDER BY r.code), ur.user_id, ur.user_id
 FROM identity_user_roles ur JOIN identity_roles r ON r.id=ur.role_id
 WHERE (ur.scope_type='platform' AND ur.scope_id='*' AND r.code IN ('platform_admin','ops_admin','finance_admin','tech_admin','audit_readonly'))
 OR (ur.scope_type='channel' AND r.code='channel_admin' AND ur.scope_id IN (SELECT id FROM identity_channel_orgs WHERE type='C'))
 GROUP BY ur.user_id, ur.scope_type, ur.scope_id ON CONFLICT DO NOTHING`).Error
}

func (s *Service) ListStaffTx(tx *gorm.DB, scope StaffScope) ([]StaffView, error) {
	scoped := *s
	scoped.db = tx
	return scoped.ListStaff(tx.Statement.Context, scope)
}
