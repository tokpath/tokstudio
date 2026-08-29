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
	ID           string    `gorm:"column:id;primaryKey"`
	Email        string    `gorm:"column:email"`
	PasswordHash *string   `gorm:"column:password_hash"`
	Status       string    `gorm:"column:status"`
	CreatedAt    time.Time `gorm:"column:created_at"`
	UpdatedAt    time.Time `gorm:"column:updated_at"`
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

// Principal 是跨模块可安全传递的身份视图，不是 ORM Model。
type Principal struct {
	UserID string
	Email  string
	Roles  []string
}

func (p Principal) HasRole(codes ...string) bool {
	wanted := map[string]struct{}{}
	for _, code := range codes {
		wanted[code] = struct{}{}
	}
	for _, role := range p.Roles {
		if _, ok := wanted[role]; ok {
			return true
		}
	}
	return false
}

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

// Bootstrap 幂等写入开发管理员和普通用户，并刷新引导 token 哈希。
func (s *Service) Bootstrap(ctx context.Context, adminToken, userToken string) error {
	if adminToken == "" || userToken == "" {
		return nil
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := upsertBootUser(tx, "admin@tokenhub.local", "platform_admin", adminToken, "thadm_"); err != nil {
			return err
		}
		return upsertBootUser(tx, "user@tokenhub.local", "end_user", userToken, "thusr_")
	})
}

func upsertBootUser(tx *gorm.DB, email, roleCode, token, prefix string) error {
	now := time.Now().UTC()
	var user userRow
	err := tx.Where("email = ?", email).First(&user).Error
	if err == gorm.ErrRecordNotFound {
		user = userRow{
			ID:        id.New("usr"),
			Email:     email,
			Status:    "active",
			CreatedAt: now,
			UpdatedAt: now,
		}
		if err := tx.Create(&user).Error; err != nil {
			return err
		}
	} else if err != nil {
		return err
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
			ScopeType: "platform",
			ScopeID:   "*",
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
	if err := s.db.WithContext(ctx).Where("id = ? AND status = ?", row.UserID, "active").First(&user).Error; err != nil {
		return nil, err
	}
	var roles []string
	if err := s.db.WithContext(ctx).
		Table("identity_user_roles ur").
		Select("r.code").
		Joins("JOIN identity_roles r ON r.id = ur.role_id").
		Where("ur.user_id = ?", user.ID).
		Scan(&roles).Error; err != nil {
		return nil, err
	}
	return &Principal{UserID: user.ID, Email: user.Email, Roles: roles}, nil
}
