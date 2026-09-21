package identity

import (
	"context"
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strings"
)

var ErrChannelAdminTarget = errors.New("administrator must be an active user of this B/C channel without another administrative role")

type ChannelAdminView struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`
}

func (s *Service) ChannelAdmins(ctx context.Context, channelID string) ([]ChannelAdminView, error) {
	rows := make([]ChannelAdminView, 0)
	err := s.db.WithContext(ctx).Table("identity_user_roles ur").Select("u.id AS user_id, u.email").Joins("JOIN identity_users u ON u.id=ur.user_id").Joins("JOIN identity_roles r ON r.id=ur.role_id").Where("r.code = ? AND ur.scope_type = ? AND ur.scope_id = ?", "channel_admin", "channel", channelID).Scan(&rows).Error
	return rows, err
}

// Bind only users already belonging to the channel. Never move attribution or grant platform access.
func (s *Service) SetChannelAdminTx(tx *gorm.DB, channelID, email string, enabled bool) (bool, error) {
	var channel channelRow
	if err := tx.Where("id = ? AND type IN ?", channelID, []string{ChannelTypeB, ChannelTypeC}).First(&channel).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, ErrChannelAdminTarget
		}
		return false, err
	}
	var user userRow
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("LOWER(email) = ? AND channel_org_id = ?", strings.ToLower(strings.TrimSpace(email)), channelID).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, ErrChannelAdminTarget
		}
		return false, err
	}
	var role roleRow
	if err := tx.Where("code = ?", "channel_admin").First(&role).Error; err != nil {
		return false, err
	}
	if !enabled {
		result := tx.Where("user_id = ? AND role_id = ? AND scope_type = ? AND scope_id = ?", user.ID, role.ID, "channel", channelID).Delete(&userRoleRow{})
		return result.RowsAffected > 0, result.Error
	}
	if user.Status != UserStatusActive || channel.Status != "active" {
		return false, ErrChannelAdminTarget
	}
	var conflicts int64
	if err := tx.Table("identity_user_roles ur").Joins("JOIN identity_roles r ON r.id=ur.role_id").Where("ur.user_id = ? AND r.code <> ? AND NOT (r.code = ? AND ur.scope_type = ? AND ur.scope_id = ?)", user.ID, "end_user", "channel_admin", "channel", channelID).Count(&conflicts).Error; err != nil {
		return false, err
	}
	if conflicts > 0 {
		return false, ErrChannelAdminTarget
	}
	row := userRoleRow{UserID: user.ID, RoleID: role.ID, ScopeType: "channel", ScopeID: channelID}
	result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
	return result.RowsAffected > 0, result.Error
}
