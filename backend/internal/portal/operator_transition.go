package portal

import (
	"context"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/organization"
)

// A read projection for every channel workspace, including a brand-initiated
// takeover with no prior exit notice. It grants no access to the brand workspace.
type operatorTransition struct {
	State       string     `json:"state"`
	BrandName   string     `json:"brand_name"`
	TakeoverID  string     `json:"takeover_id"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

func (a *Application) operatorTransition(ctx context.Context, op organization.Operator) (*operatorTransition, error) {
	if op.Kind != organization.Channel {
		return nil, nil
	}
	var out operatorTransition
	q := a.DB.WithContext(ctx).Raw(`SELECT t.id AS takeover_id,t.state,t.completed_at,o.name AS brand_name
 FROM organization_takeover_locks l JOIN remediation_takeovers t ON t.id=l.takeover_id
 JOIN organization_operators o ON o.id=t.brand_operator_id WHERE l.operator_id=?`, op.ID).Scan(&out)
	if q.Error != nil {
		return nil, q.Error
	}
	if q.RowsAffected == 0 {
		return nil, nil
	}
	return &out, nil
}

func channelCommandAllowed(state, path string) bool {
	if state == "" {
		return true
	}
	parts := strings.Split(strings.Trim(path, "/"), "/")
	resource, action := parts[0], ""
	if len(parts) > 1 {
		action = parts[len(parts)-1]
	}
	switch resource {
	case "plans", "pricing", "marketing", "commissions", "gifts", "coverage", "alerts", "branding", "channel-themes", "domains", "payment-methods", "payment-quotes", "mail", "contracts", "exits", "business-status", "takeovers":
		return false
	case "procurement":
		return state != "completed" && contains([]string{"cancel", "cancel_authorization", "query_payment"}, action)
	case "customers", "customer-context", "orders", "order-context", "requests":
		return state != "completed"
	case "payments":
		// A stopped business may still verify a historical receipt or record its fee.
		return action != ""
	case "notifications":
		return action == "read" || action == "read-all"
	}
	// Historical invoices, refunds, evidence, reports and staff access remain
	// subject to their original permissions and exact record ownership checks.
	return true
}

func operatorActionPath(path string) string {
	_, after, found := strings.Cut(path, "/operators/")
	if !found {
		return ""
	}
	_, after, _ = strings.Cut(after, "/")
	return strings.Split(after, "?")[0]
}

func (a *Application) operatorTransitionGuard(c *gin.Context) {
	// Check exact membership before exposing any transition details.
	var n int64
	if e := a.DB.WithContext(c.Request.Context()).Table("organization_staff_memberships").Where("operator_id=? AND staff_id=? AND status='active'", c.Param("operatorId"), subject(c).Staff.ID).Count(&n).Error; e != nil {
		handleError(c, e)
		c.Abort()
		return
	}
	if n == 0 {
		handleError(c, organization.ErrForbidden)
		c.Abort()
		return
	}
	op, e := a.Organization.GetOperator(c.Request.Context(), nil, c.Param("operatorId"))
	if e != nil {
		handleError(c, e)
		c.Abort()
		return
	}
	transition, e := a.operatorTransition(c.Request.Context(), op)
	if e != nil {
		handleError(c, e)
		c.Abort()
		return
	}
	if transition == nil {
		return
	}
	c.Set("operator-transition", transition)
	if c.Request.Method == "GET" || c.Request.Method == "HEAD" {
		return
	}
	if !channelCommandAllowed(transition.State, operatorActionPath(c.Request.URL.Path)) {
		message := "渠道正在接管，新增经营操作已暂停。请在停业与接管页面查看进度；存量事项仍在原业务页面办理。"
		if transition.State == "completed" {
			message = "渠道已被接管并结束经营，不能继续新增经营或申请退出。客户服务由承接品牌办理；历史记录仍可查询。"
		}
		fail(c, 409, "channel_takeover_restricted", message, false)
		c.Abort()
	}
}

func filterTransitionOperations(c *gin.Context, ops []operation) []operation {
	v, ok := c.Get("operator-transition")
	if !ok {
		return ops
	}
	transition := v.(*operatorTransition)
	out := []operation{}
	for _, op := range ops {
		if channelCommandAllowed(transition.State, operatorActionPath(op.Path)) {
			out = append(out, op)
		}
	}
	return out
}
