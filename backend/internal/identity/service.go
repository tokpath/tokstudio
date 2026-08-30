// Package identity 管理用户、角色和访问令牌。其他模块只使用 Principal 和 Authenticator。
package identity

import (
	"context"
	"embed"
	"io/fs"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/tokpath/tokstudio/backend/internal/platform/crypto"
	"github.com/tokpath/tokstudio/backend/internal/platform/id"
)

//go:embed migrate/*.sql
var migrationFS embed.FS

type userRow struct {
	ID              string     `gorm:"column:id;primaryKey"`
	Email           string     `gorm:"column:email"`
	PasswordHash    *string    `gorm:"column:password_hash"`
	Status          string     `gorm:"column:status"`
	ChannelOrgID    *string    `gorm:"column:channel_org_id"`
	BrandID         *string    `gorm:"column:brand_id"`
	DisplayName     string     `gorm:"column:display_name"`
	Locale          string     `gorm:"column:locale"`
	EmailVerifiedAt *time.Time `gorm:"column:email_verified_at"`
	GoogleSub       *string    `gorm:"column:google_sub"`
	CreatedAt       time.Time  `gorm:"column:created_at"`
	UpdatedAt       time.Time  `gorm:"column:updated_at"`
}

func (userRow) TableName() string { return "identity_users" }

type roleRow struct {
	ID   string `gorm:"column:id;primaryKey"`
	Code string `gorm:"column:code"`
}

func (roleRow) TableName() string { return "identity_roles" }

type userRoleRow struct {
	UserID    string `gorm:"column:user_id;primaryKey"`
	RoleID    string `gorm:"column:role_id;primaryKey"`
	ScopeType string `gorm:"column:scope_type;primaryKey"`
	ScopeID   string `gorm:"column:scope_id;primaryKey"`
}

func (userRoleRow) TableName() string { return "identity_user_roles" }

type tokenRow struct {
	ID        string    `gorm:"column:id;primaryKey"`
	UserID    string    `gorm:"column:user_id"`
	TokenHash string    `gorm:"column:token_hash"`
	Prefix    string    `gorm:"column:prefix"`
	Status    string    `gorm:"column:status"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

func (tokenRow) TableName() string { return "identity_access_tokens" }

type Service struct {
	db *gorm.DB
}

func New(db *gorm.DB) *Service {
	return &Service{db: db}
}

func Migrations() (string, fs.FS) {
	sub, err := fs.Sub(migrationFS, "migrate")
	if err != nil {
		panic(err)
	}
	return "identity", sub
}

func upsertBootUser(tx *gorm.DB, email, roleCode, token, prefix, channelID, brandID, scopeType, scopeID string) error {
	now := time.Now().UTC()
	var user userRow
	err := tx.Where("email = ?", email).First(&user).Error
	if err == gorm.ErrRecordNotFound {
		pwd, err := HashPassword(BootstrapPassword)
		if err != nil {
			return err
		}
		user = userRow{
			ID:           id.New("usr"),
			Email:        email,
			PasswordHash: &pwd,
			Status:       "active",
			ChannelOrgID: &channelID,
			BrandID:      &brandID,
			CreatedAt:    now,
			UpdatedAt:    now,
		}
		if err := tx.Create(&user).Error; err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else {
		_ = tx.Model(&userRow{}).Where("id = ?", user.ID).Updates(map[string]any{
			"channel_org_id": channelID,
			"brand_id":       brandID,
		}).Error
	}
	if user.PasswordHash == nil {
		pwd, err := HashPassword(BootstrapPassword)
		if err != nil {
			return err
		}
		if err := tx.Model(&userRow{}).Where("id = ?", user.ID).Update("password_hash", pwd).Error; err != nil {
			return err
		}
		user.PasswordHash = &pwd
	}

	var role roleRow
	if err := tx.Where("code = ?", roleCode).First(&role).Error; err != nil {
		return err
	}
	if err := tx.Where("user_id = ? AND role_id = ?", user.ID, role.ID).First(&userRoleRow{}).Error; err != nil {
		if err != gorm.ErrRecordNotFound {
			return err
		}
		if err := tx.Create(&userRoleRow{
			UserID:    user.ID,
			RoleID:    role.ID,
			ScopeType: scopeType,
			ScopeID:   scopeID,
		}).Error; err != nil {
			return err
		}
	}

	hash := crypto.HashToken(token)
	var existing tokenRow
	err = tx.Where("user_id = ? AND prefix = ?", user.ID, prefix).First(&existing).Error
	if err == gorm.ErrRecordNotFound {
		return tx.Create(&tokenRow{
			ID:        id.New("tok"),
			UserID:    user.ID,
			TokenHash: hash,
			Prefix:    prefix,
			Status:    "active",
			CreatedAt: now,
		}).Error
	}
	if err != nil {
		return err
	}
	return tx.Model(&tokenRow{}).Where("id = ?", existing.ID).Updates(map[string]any{
		"token_hash": hash,
		"status":     "active",
	}).Error
}

// Authenticate 用 Bearer token 解析身份。失败返回 nil。
func (s *Service) Authenticate(ctx context.Context, bearer string) (*Principal, error) {
	token := strings.TrimSpace(strings.TrimPrefix(bearer, "Bearer "))
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, nil
	}
	var row tokenRow
	if err := s.db.WithContext(ctx).
		Where("token_hash = ? AND status = ?", crypto.HashToken(token), "active").
		First(&row).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	var user userRow
	if err := s.db.WithContext(ctx).Where("id = ? AND status = ?", row.UserID, UserStatusActive).First(&user).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	principal, err := s.loadPrincipal(ctx, user)
	if err != nil {
		return nil, err
	}
	return principal, nil
}

func (s *Service) loadPrincipal(ctx context.Context, user userRow) (*Principal, error) {
	type roleScope struct {
		Code      string
		ScopeType string
		ScopeID   string
	}
	var rows []roleScope
	if err := s.db.WithContext(ctx).
		Table("identity_user_roles ur").
		Select("r.code, ur.scope_type, ur.scope_id").
		Joins("JOIN identity_roles r ON r.id = ur.role_id").
		Where("ur.user_id = ?", user.ID).
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	roles := make([]string, 0, len(rows))
	p := principalFromUser(user, nil)
	for _, row := range rows {
		roles = append(roles, row.Code)
		if row.Code == "channel_admin" {
			p.ScopeType = row.ScopeType
			p.ScopeID = row.ScopeID
			if p.ChannelOrgID == "" {
				p.ChannelOrgID = row.ScopeID
			}
		}
	}
	p.Roles = roles
	return p, nil
}

func principalFromUser(user userRow, roles []string) *Principal {
	p := &Principal{UserID: user.ID, Email: user.Email, Roles: roles, ScopeType: "platform", ScopeID: "*"}
	if user.ChannelOrgID != nil {
		p.ChannelOrgID = *user.ChannelOrgID
	}
	if user.BrandID != nil {
		p.BrandID = *user.BrandID
	}
	return p
}

func deref(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
