package portal

import (
	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"github.com/tokpath/tokstudio/backend/internal/remediation"
)

// The owning brand and original channel can review the takeover. Only the
// owning brand may prepare or execute it; cash handover happens offline.
func (a *Application) takeoverResource(c *gin.Context, op organization.Operator, actor string) {
	rows, e := a.Remediation.Takeovers(c.Request.Context(), actor, op.ID)
	if e != nil {
		handleError(c, e)
		return
	}
	children, e := a.Organization.Children(c.Request.Context(), nil, op.ID)
	if e != nil {
		handleError(c, e)
		return
	}
	channels := []gin.H{}
	active := map[string]bool{}
	for _, row := range rows {
		if row.State != "cancelled" {
			active[row.ChannelID] = true
		}
	}
	for _, child := range children {
		if child.Kind == organization.Channel && child.Lifecycle != "terminated" && !active[child.ID] {
			channels = append(channels, gin.H{"id": child.ID, "name": child.Name, "legal_name": child.LegalName, "lifecycle": child.Lifecycle})
		}
	}
	if rows == nil {
		rows = []remediation.Takeover{}
	}
	notices := []gin.H{}
	canNotify := false
	if a.Organization.RequirePermission(c.Request.Context(), nil, actor, op.ID, "cooperation.manage") == nil {
		found, e := a.Organization.ExitNotices(c.Request.Context(), actor, op.ID)
		if e != nil {
			handleError(c, e)
			return
		}
		ownNotice := false
		for _, n := range found {
			operator, e := a.Organization.GetOperator(c.Request.Context(), nil, n.OperatorID)
			if e != nil {
				handleError(c, e)
				return
			}
			notices = append(notices, gin.H{"id": n.ID, "operator_id": n.OperatorID, "operator_name": operator.Name, "kind": n.Kind, "note": n.Note, "created_at": n.CreatedAt})
			if n.OperatorID == op.ID {
				ownNotice = true
			}
		}
		canNotify = op.Kind != organization.Platform && op.Lifecycle != "terminated" && !ownNotice && !active[op.ID]
	}
	c.JSON(200, gin.H{"items": rows, "channels": channels, "notices": notices, "can_notify": canNotify})
}
func (a *Application) takeoverCommand(c *gin.Context, op organization.Operator, actor string) {
	var in remediation.TakeoverInput
	if !body(c, &in) {
		return
	}
	if in.Evidence == "" {
		in.Evidence = "品牌确认承接渠道客户"
	}
	in.ActorID = actor
	in.BrandOperatorID = op.ID
	in.BusinessKey, _ = key(c)
	out, e := a.Remediation.BeginTakeover(c.Request.Context(), in)
	if e != nil {
		handleError(c, e)
		return
	}
	c.JSON(200, out)
}
func (a *Application) takeoverAction(c *gin.Context, op organization.Operator, actor, action string) {
	ctx := c.Request.Context()
	id := c.Param("id")
	var out remediation.Takeover
	var e error
	switch action {
	case "refresh":
		var in struct{}
		if !body(c, &in) {
			return
		}
		out, e = a.Remediation.RefreshTakeover(ctx, actor, op.ID, id)
	case "execute":
		var in struct {
			ExpectedSnapshot string `json:"expected_snapshot"`
		}
		if !body(c, &in) {
			return
		}
		out, e = a.Remediation.ExecuteTakeover(ctx, actor, op.ID, id, in.ExpectedSnapshot)
	case "cancel":
		var in struct {
			Evidence string `json:"evidence"`
		}
		if !body(c, &in) {
			return
		}
		if in.Evidence == "" {
			in.Evidence = "经营方取消接管准备"
		}
		out, e = a.Remediation.CancelTakeover(ctx, actor, op.ID, id, in.Evidence)
	default:
		fail(c, 404, "not_found", "接管操作不存在", false)
		return
	}
	if e != nil {
		handleError(c, e)
		return
	}
	c.JSON(200, out)
}
