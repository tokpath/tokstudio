package portal

import (
	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"github.com/tokpath/tokstudio/backend/internal/remediation"
)

// Takeover cases and their settlement remain separate from customer entitlements.
// The complete reviewed snapshot is returned only to the brand's precise scope.
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
	for _, child := range children {
		if child.Kind == organization.Channel {
			channels = append(channels, gin.H{"id": child.ID, "name": child.Name, "legal_name": child.LegalName, "lifecycle": child.Lifecycle})
		}
	}
	if rows == nil {
		rows = []remediation.Takeover{}
	}
	c.JSON(200, gin.H{"items": rows, "channels": channels})
}
func (a *Application) takeoverCommand(c *gin.Context, op organization.Operator, actor string) {
	var in remediation.TakeoverInput
	if !body(c, &in) {
		return
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
		out, e = a.Remediation.CancelTakeover(ctx, actor, op.ID, id, in.Evidence)
	case "settle":
		var in remediation.ExecutionProof
		if !body(c, &in) {
			return
		}
		in.BusinessKey, _ = key(c)
		settings, err := ledger.GetMonetary(ctx, a.DB)
		if err != nil {
			handleError(c, err)
			return
		}
		in, e = normalizeExecutionProof(in, settings.Currency)
		if e != nil {
			handleError(c, e)
			return
		}
		out, e = a.Remediation.RecordTakeoverSettlement(ctx, actor, op.ID, id, in)
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
