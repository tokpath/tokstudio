package app

import (
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/audit"
	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/platform/httpx"
	"gorm.io/gorm"
	"net/http"
	"strings"
)

func (a *App) listChannelAdmins(c *gin.Context) {
	if !a.canManageChannelAdmins(c) {
		return
	}
	items, err := a.Identity.ChannelAdmins(c.Request.Context(), c.Param("id"))
	if err != nil {
		httpx.Abort(c, 500, "internal_error", "读取渠道管理员失败", true)
		return
	}
	httpx.OK(c, gin.H{"items": items})
}
func (a *App) setChannelAdmin(c *gin.Context) {
	if !a.canManageChannelAdmins(c) {
		return
	}
	if !a.requireConfirm(c) {
		return
	}
	var body struct {
		Email   string `json:"email"`
		Enabled *bool  `json:"enabled"`
		Reason  string `json:"reason"`
	}
	if c.ShouldBindJSON(&body) != nil || strings.TrimSpace(body.Email) == "" || body.Enabled == nil {
		httpx.Abort(c, 400, "invalid_request", "请选择本组织已有用户和操作", false)
		return
	}
	changed := false
	err := a.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		var err error
		target, err := a.Identity.GetChannel(c.Request.Context(), *a.currentPrincipal(c), c.Param("id"))
		if err != nil {
			return err
		}
		changed, err = a.Identity.SetChannelAdminTx(tx, *a.currentPrincipal(c), c.Param("id"), body.Email, *body.Enabled)
		if err != nil || !changed {
			return err
		}
		_, err = a.Audit.RecordTx(tx, audit.RecordInput{ActorUserID: a.currentPrincipal(c).UserID, Action: "channel.admin.change", ResourceType: "channel", ResourceID: c.Param("id"), After: map[string]any{"email": strings.ToLower(strings.TrimSpace(body.Email)), "enabled": *body.Enabled, "reason": strings.TrimSpace(body.Reason)}, IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID)})
		if err != nil {
			return err
		}
		if target.Type == identity.ChannelTypeC {
			// The scoped member includes no password or credential fields.
			members, e := a.Identity.ListStaffTx(tx, identity.StaffScope{Type: "channel", ID: target.ID})
			if e != nil {
				return e
			}
			for _, member := range members {
				if member.Email == strings.ToLower(strings.TrimSpace(body.Email)) {
					_, err = a.Audit.RecordTx(tx, audit.RecordInput{ActorUserID: a.currentPrincipal(c).UserID, Action: "staff.roles.update", ResourceType: "staff", ResourceID: member.UserID, After: member, IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID)})
					break
				}
			}
		}
		return err
	})
	if errors.Is(err, identity.ErrStaffLastAdmin) || errors.Is(err, identity.ErrSelfAction) || errors.Is(err, identity.ErrStaffTarget) {
		a.abortStaffError(c, err)
		return
	}
	if errors.Is(err, identity.ErrChannelAdminTarget) {
		httpx.Abort(c, http.StatusConflict, "invalid_channel_admin", "目标必须是该组织已注册的用户；授权时账户和渠道须正常，且不能持有其他管理角色。未修改权限。", false)
		return
	}
	if err != nil {
		httpx.Abort(c, 500, "internal_error", "权限保存失败，已回滚，请重试。", true)
		return
	}
	httpx.OK(c, gin.H{"changed": changed})
}

func (a *App) canManageChannelAdmins(c *gin.Context) bool {
	p := a.currentPrincipal(c)
	if p == nil {
		httpx.Abort(c, http.StatusForbidden, "permission_denied", "权限不足", false)
		return false
	}
	target, err := a.Identity.GetChannel(c.Request.Context(), *p, c.Param("id"))
	if err != nil || target.Type == identity.ChannelTypeA {
		httpx.Abort(c, http.StatusForbidden, "permission_denied", "无权管理该渠道", false)
		return false
	}
	if p.IsPlatformAdmin() {
		if target.ParentID == identity.OfficialChannelID || target.ParentID == "" {
			return true
		}
	} else if p.HasRole("channel_admin") && target.Type == identity.ChannelTypeB && target.ParentID == p.ChannelOrgID {
		parent, err := a.Identity.GetChannel(c.Request.Context(), *p, p.ChannelOrgID)
		if err == nil && parent.Type == identity.ChannelTypeC {
			return true
		}
	}
	httpx.Abort(c, http.StatusForbidden, "permission_denied", "只能管理直属渠道的管理员", false)
	return false
}
