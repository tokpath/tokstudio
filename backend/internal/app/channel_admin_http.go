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
	items, err := a.Identity.ChannelAdmins(c.Request.Context(), c.Param("id"))
	if err != nil {
		httpx.Abort(c, 500, "internal_error", "读取渠道管理员失败", true)
		return
	}
	httpx.OK(c, gin.H{"items": items})
}
func (a *App) setChannelAdmin(c *gin.Context) {
	if !a.requireConfirm(c) {
		return
	}
	var body struct {
		Email   string `json:"email"`
		Enabled *bool  `json:"enabled"`
		Reason  string `json:"reason"`
	}
	if c.ShouldBindJSON(&body) != nil || strings.TrimSpace(body.Email) == "" || body.Enabled == nil || strings.TrimSpace(body.Reason) == "" {
		httpx.Abort(c, 400, "invalid_request", "请填写已注册的本渠道用户邮箱、操作和原因", false)
		return
	}
	changed := false
	err := a.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		var err error
		changed, err = a.Identity.SetChannelAdminTx(tx, c.Param("id"), body.Email, *body.Enabled)
		if err != nil || !changed {
			return err
		}
		_, err = a.Audit.RecordTx(tx, audit.RecordInput{ActorUserID: a.currentPrincipal(c).UserID, Action: "channel.admin.change", ResourceType: "channel", ResourceID: c.Param("id"), After: map[string]any{"email": strings.ToLower(strings.TrimSpace(body.Email)), "enabled": *body.Enabled, "reason": strings.TrimSpace(body.Reason)}, IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID)})
		return err
	})
	if errors.Is(err, identity.ErrChannelAdminTarget) {
		httpx.Abort(c, http.StatusConflict, "invalid_channel_admin", "目标必须是该 B/C 渠道已注册的用户；授权时账户和渠道须正常，且不能持有其他管理角色。未修改权限。", false)
		return
	}
	if err != nil {
		httpx.Abort(c, 500, "internal_error", "权限保存失败，已回滚，请重试。", true)
		return
	}
	httpx.OK(c, gin.H{"changed": changed})
}
