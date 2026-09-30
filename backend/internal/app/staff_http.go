package app

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/audit"
	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/platform/httpx"
	"gorm.io/gorm"
)

func (a *App) registerStaffRoutes(r *gin.Engine) {
	r.GET("/admin/staff", a.requireRoles("platform_admin"), a.listStaff(false))
	r.POST("/admin/staff", a.requireRoles("platform_admin"), a.createStaff(false))
	r.PATCH("/admin/staff/:id", a.requireRoles("platform_admin"), a.updateStaff(false))
	r.GET("/admin/staff/:id/history", a.requireRoles("platform_admin"), a.staffHistory(false))
	r.GET("/channel/staff", a.requireRoles("channel_admin"), a.listStaff(true))
	r.POST("/channel/staff", a.requireRoles("channel_admin"), a.createStaff(true))
	r.PATCH("/channel/staff/:id", a.requireRoles("channel_admin"), a.updateStaff(true))
	r.GET("/channel/staff/:id/history", a.requireRoles("channel_admin"), a.staffHistory(true))
}
func (a *App) staffScope(c *gin.Context, oem bool) (identity.StaffScope, bool) {
	scope, err := a.Identity.StaffScope(c.Request.Context(), *a.currentPrincipal(c), oem)
	if err != nil {
		a.abortStaffError(c, err)
		return scope, false
	}
	return scope, true
}
func (a *App) listStaff(oem bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		scope, ok := a.staffScope(c, oem)
		if !ok {
			return
		}
		items, err := a.Identity.ListStaff(c.Request.Context(), scope)
		if a.abortStaffError(c, err) {
			return
		}
		httpx.OK(c, gin.H{"items": items, "roles": identity.StaffRoleOptions(scope), "scope": scope})
	}
}
func (a *App) createStaff(oem bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		scope, ok := a.staffScope(c, oem)
		if !ok || !a.requireConfirm(c) {
			return
		}
		var in identity.StaffInput
		if c.ShouldBindJSON(&in) != nil {
			a.abortStaffError(c, identity.ErrStaffInput)
			return
		}
		var item *identity.StaffView
		err := a.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
			var err error
			item, err = a.Identity.CreateStaffTx(tx, *a.currentPrincipal(c), scope, in)
			if err != nil {
				return err
			}
			_, err = a.Audit.RecordTx(tx, audit.RecordInput{ActorUserID: a.currentPrincipal(c).UserID, Action: "staff.create", ResourceType: "staff", ResourceID: item.UserID, After: item, IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID)})
			return err
		})
		if a.abortStaffError(c, err) {
			return
		}
		httpx.Created(c, gin.H{"item": item})
	}
}
func (a *App) updateStaff(oem bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		scope, ok := a.staffScope(c, oem)
		if !ok || !a.requireConfirm(c) {
			return
		}
		var in identity.StaffInput
		if c.ShouldBindJSON(&in) != nil || (in.Roles == nil && in.Status == "") {
			a.abortStaffError(c, identity.ErrStaffInput)
			return
		}
		var item *identity.StaffView
		err := a.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
			before, after, err := a.Identity.UpdateStaffTx(tx, *a.currentPrincipal(c), scope, c.Param("id"), in)
			if err != nil {
				return err
			}
			item = after
			action := "staff.roles.update"
			if before.Status != after.Status {
				if after.Status == "disabled" {
					action = "staff.disable"
				} else {
					action = "staff.enable"
				}
			}
			_, err = a.Audit.RecordTx(tx, audit.RecordInput{ActorUserID: a.currentPrincipal(c).UserID, Action: action, ResourceType: "staff", ResourceID: item.UserID, Before: before, After: after, IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID)})
			return err
		})
		if a.abortStaffError(c, err) {
			return
		}
		httpx.OK(c, gin.H{"item": item})
	}
}
func (a *App) staffHistory(oem bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		scope, ok := a.staffScope(c, oem)
		if !ok {
			return
		}
		if _, err := a.Identity.GetStaff(c.Request.Context(), scope, c.Param("id")); a.abortStaffError(c, err) {
			return
		}
		items, err := a.Audit.Search(c.Request.Context(), audit.SearchQuery{ResourceType: "staff", ResourceID: c.Param("id"), Limit: 100})
		if a.abortStaffError(c, err) {
			return
		}
		staff, err := a.Identity.ListStaff(c.Request.Context(), scope)
		if a.abortStaffError(c, err) {
			return
		}
		actors := map[string]string{}
		for _, member := range staff {
			actors[member.UserID] = member.Email
		}
		httpx.OK(c, gin.H{"items": items, "actors": actors})
	}
}
func (a *App) abortStaffError(c *gin.Context, err error) bool {
	if err == nil {
		return false
	}
	status, code, message := http.StatusBadRequest, "invalid_request", "员工信息或岗位无效"
	switch {
	case errors.Is(err, identity.ErrChannelImmutable):
		status, code, message = 403, "permission_denied", "只有本公司管理员可管理员工"
	case errors.Is(err, identity.ErrStaffTarget):
		status, code, message = 409, "staff_target", "账号须正常且属于公司本部，不能加入其他公司或渠道的账号"
	case errors.Is(err, identity.ErrStaffExists):
		status, code, message = 409, "staff_exists", "该账号已在员工列表中，请编辑岗位或恢复账号"
	case errors.Is(err, identity.ErrEmailTaken):
		status, code, message = 409, "email_taken", "该邮箱已注册，请清空初始密码后再添加；已有密码不会被修改"
	case errors.Is(err, identity.ErrWeakPassword):
		code, message = "weak_password", "新账号的初始密码至少 8 位"
	case errors.Is(err, identity.ErrSelfAction):
		status, code, message = 403, "self_action", "不能修改自己的岗位或停用自己的账号"
	case errors.Is(err, identity.ErrStaffLastAdmin):
		status, code, message = 409, "last_admin", "公司必须保留至少一位有效管理员"
	case errors.Is(err, identity.ErrNotFound):
		status, code, message = 404, "not_found", "员工不存在或不属于本公司"
	case errors.Is(err, identity.ErrStaffInput):
	default:
		status, code, message = 500, "internal_error", "员工权限保存失败，请重试"
	}
	httpx.Abort(c, status, code, message, status == 500)
	return true
}
