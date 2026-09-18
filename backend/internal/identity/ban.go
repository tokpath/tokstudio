package identity

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	UserStatusActive = "active"
	UserStatusBanned = "banned"
)

var (
	ErrInvalidReason  = errors.New("reason is required")
	ErrSelfAction     = errors.New("cannot apply this action to yourself")
	ErrAdminProtected = errors.New("platform admin cannot be banned")
)

// AdminSetUserStatus 封禁或解封终端用户。平台管理员账号不能被封禁，也不能封自己。
func (s *Service) AdminSetUserStatus(ctx context.Context, actor Principal, userID, status, reason string) (*UserView, string, error) {
	var view *UserView
	var before string
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		view, before, err = s.AdminSetUserStatusTx(tx, actor, userID, status, reason)
		return err
	})
	return view, before, err
}

func (s *Service) AdminSetUserStatusTx(tx *gorm.DB, actor Principal, userID, status, reason string) (*UserView, string, error) {
	scoped := *s
	scoped.db = tx
	return scoped.setUserStatus(tx.Statement.Context, actor, userID, status, reason)
}

func (s *Service) setUserStatus(ctx context.Context, actor Principal, userID, status, reason string) (*UserView, string, error) {
	if !actor.IsPlatformAdmin() {
		return nil, "", ErrChannelImmutable
	}
	if strings.TrimSpace(reason) == "" {
		return nil, "", ErrInvalidReason
	}
	if status != UserStatusActive && status != UserStatusBanned {
		return nil, "", ErrInvalidReason
	}
	if userID == actor.UserID {
		return nil, "", ErrSelfAction
	}
	var user userRow
	if err := s.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", userID).First(&user).Error; err != nil {
		return nil, "", err
	}
	before := user.Status
	if before == status {
		view := viewFromUser(user, nil, "")
		return &view, before, nil
	}
	principal, err := s.loadPrincipal(ctx, user)
	if err != nil {
		return nil, "", err
	}
	if principal.IsPlatformAdmin() && status == UserStatusBanned {
		return nil, "", ErrAdminProtected
	}
	now := time.Now().UTC()
	if err := s.db.WithContext(ctx).Model(&userRow{}).Where("id = ?", user.ID).Updates(map[string]any{
		"status":     status,
		"updated_at": now,
	}).Error; err != nil {
		return nil, "", err
	}
	user.Status = status
	user.UpdatedAt = now
	var attr attributionRow
	source := ""
	if err := s.db.WithContext(ctx).Where("user_id = ?", user.ID).First(&attr).Error; err == nil {
		source = attr.SourceCode
	}
	view := viewFromUser(user, principal.Roles, source)
	return &view, before, nil
}
