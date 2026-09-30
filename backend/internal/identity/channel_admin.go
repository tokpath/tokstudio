package identity

import (
	"context"
	"encoding/json"
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"slices"
	"strings"
	"time"
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
func (s *Service) SetChannelAdminTx(tx *gorm.DB, actor Principal, channelID, email string, enabled bool) (bool, error) {
	var channel channelRow
	if err := tx.Where("id = ? AND type IN ?", channelID, []string{ChannelTypeB, ChannelTypeC}).First(&channel).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, ErrChannelAdminTarget
		}
		return false, err
	}
	if channel.Type == ChannelTypeC {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", channelID).First(&channel).Error; err != nil {
			return false, err
		}
	}
	var user userRow
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("LOWER(email) = ? AND channel_org_id = ?", strings.ToLower(strings.TrimSpace(email)), channelID).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, ErrChannelAdminTarget
		}
		return false, err
	}
	if channel.Type == ChannelTypeC {
		if user.ID == actor.UserID {
			return false, ErrSelfAction
		}
		scope := StaffScope{"channel", channelID}
		var member staffRow
		err := tx.Where("user_id = ?", user.ID).First(&member).Error
		if err == nil {
			if member.ScopeType != scope.Type || member.ScopeID != scope.ID {
				return false, ErrStaffTarget
			}
			var roles []string
			if err := json.Unmarshal(member.RolesJSON, &roles); err != nil {
				return false, err
			}
			has := member.Status == "active" && slices.Contains(roles, "channel_admin")
			if has == enabled {
				return false, nil
			}
			if !enabled {
				var count int64
				if err := tx.Table("identity_staff_members m").Joins("JOIN identity_users u ON u.id=m.user_id").Where(`m.scope_type='channel' AND m.scope_id=? AND m.status='active' AND u.status='active' AND m.roles_json @> '["channel_admin"]'::jsonb`, channelID).Count(&count).Error; err != nil {
					return false, err
				}
				if count <= 1 {
					return false, ErrStaffLastAdmin
				}
				roles = slices.DeleteFunc(roles, func(r string) bool { return r == "channel_admin" })
				if len(roles) == 0 {
					member.Status = "disabled"
					roles = []string{"channel_admin"}
				}
			} else {
				if user.Status != UserStatusActive || channel.Status != "active" {
					return false, ErrStaffTarget
				}
				if !slices.Contains(roles, "channel_admin") {
					roles = append(roles, "channel_admin")
				}
				member.Status = "active"
			}
			member.RolesJSON, _ = json.Marshal(roles)
			member.UpdatedBy = actor.UserID
			member.UpdatedAt = time.Now().UTC()
			if err := tx.Save(&member).Error; err != nil {
				return false, err
			}
			if err := applyStaffRolesTx(tx, member); err != nil {
				return false, err
			}
			if member.Status == "disabled" {
				if err := tx.Model(&apiKeyRow{}).Where("user_id = ? AND status = 'active'", user.ID).Update("status", "disabled").Error; err != nil {
					return false, err
				}
				if err := tx.Model(&tokenRow{}).Where("user_id=?", user.ID).Update("status", "revoked").Error; err != nil {
					return false, err
				}
			}
			return true, nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return false, err
		}
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
	if result.Error != nil {
		return false, result.Error
	}
	if channel.Type == ChannelTypeC {
		raw, _ := json.Marshal([]string{"channel_admin"})
		now := time.Now().UTC()
		member := staffRow{UserID: user.ID, ScopeType: "channel", ScopeID: channelID, RolesJSON: raw, Status: "active", CreatedBy: actor.UserID, UpdatedBy: actor.UserID, CreatedAt: now, UpdatedAt: now}
		if err := tx.Create(&member).Error; err != nil {
			return false, err
		}
	}
	return result.RowsAffected > 0, nil
}
