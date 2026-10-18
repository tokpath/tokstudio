package portal

import (
	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
)

// Password changes invalidate every other browser session. The current session
// keeps its opaque token so a lost response can be retried safely with its key.
func (a *Application) changeStaffPassword(c *gin.Context) {
	id := subject(c)
	if id == nil || id.Session.Kind != "staff" {
		fail(c, 403, "staff_required", "此操作需要经营人员账号", false)
		return
	}
	var in struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if !body(c, &in) {
		return
	}
	if a.throttle(c.Request.Context(), "staff-password:"+id.Staff.ID) != nil {
		fail(c, 429, "auth_rate", "请求过于频繁", false)
		return
	}
	raw, e := a.command(c, "staff-password:"+id.Staff.ID, in, func(tx *gorm.DB) (any, error) {
		var staff Staff
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&staff, "id=? AND status='active'", id.Staff.ID).Error; e != nil {
			return nil, e
		}
		if !organization.CheckPassword(staff.PasswordHash, in.CurrentPassword) {
			return nil, organization.ErrInvalidCredentials
		}
		if in.NewPassword == in.CurrentPassword {
			return nil, organization.ErrInvalidInput
		}
		hash, e := organization.HashPassword(in.NewPassword)
		if e != nil {
			return nil, e
		}
		newVersion := staff.Version + 1
		if e = tx.Model(&staff).Updates(map[string]any{"password_hash": hash, "must_change_password": false, "version": newVersion}).Error; e != nil {
			return nil, e
		}
		if e = tx.Where("kind='staff' AND subject_id=? AND token_hash<>?", staff.ID, id.Session.TokenHash).Delete(&browserSession{}).Error; e != nil {
			return nil, e
		}
		if e = tx.Model(&browserSession{}).Where("token_hash=?", id.Session.TokenHash).Updates(map[string]any{"credential_version": newVersion, "expires_at": time.Now().UTC().Add(12 * time.Hour)}).Error; e != nil {
			return nil, e
		}
		return commandResult(staff.ID, "password_changed"), nil
	})
	jsonResponse(c, raw, e)
}
