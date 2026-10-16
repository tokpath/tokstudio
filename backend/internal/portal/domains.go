package portal

import (
	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/branding"
	"github.com/tokpath/tokstudio/backend/internal/organization"
)

func (a *Application) domainResource(c *gin.Context, op organization.Operator, actor string, p gin.H, base string) {
	rows, e := a.Branding.ListDomains(c.Request.Context(), nil, actor, op.BrandID)
	if e != nil {
		handleError(c, e)
		return
	}
	items := []any{}
	for _, d := range rows {
		row := entity(d.ID, d.Status, d.Version, d)
		actions := []string{"verify"}
		if d.Status == "active" {
			actions = append(actions, "disable")
		} else if d.VerifiedAt != nil && d.TLSReady {
			actions = append(actions, "enable")
		}
		row["allowed_actions"] = actions
		items = append(items, row)
	}
	p["title"] = "品牌域名与TLS验证"
	p["description"] = "域名只有通过真实DNS所有权及有效TLS检查后才会参与品牌路由。渠道可改外观，不能建立独立品牌域名。"
	p["items"] = items
	p["columns"] = []gin.H{col("hostname", "域名", "text"), col("status", "状态", "status"), col("tls_ready", "有效TLS", "boolean"), col("verification_token", "TXT验证值", "text"), col("provider_status", "域名供应商状态", "text"), col("tls_status", "证书状态", "text"), col("provider_instructions", "实际DNS步骤", "text"), col("failure_reason", "失败原因", "text"), col("checked_at", "检查时间", "datetime")}
	p["operations"] = []operation{{Name: "request", Label: "申请域名验证", Method: "POST", Path: base, Fields: []field{f("hostname", "品牌域名", "text")}}}
	p["row_operations"] = []operation{{Name: "verify", IncludeVersion: true, Label: "检查DNS与TLS", Method: "POST", Path: base + "/{id}/verify", Fields: []field{}, Description: "服务器执行真实DNS和TLS验证，不能用按钮直接标记验证成功。"}, {Name: "enable", IncludeVersion: true, Label: "启用已验证域名", Method: "POST", Path: base + "/{id}/enable", Fields: []field{}}, {Name: "disable", IncludeVersion: true, Label: "停用域名", Method: "POST", Path: base + "/{id}/disable", Fields: []field{}}}
	a.resourceResponse(c, p)
}
func (a *Application) domainAction(c *gin.Context, op organization.Operator, actor, action string) {
	var in struct{ Version int64 }
	if !body(c, &in) {
		return
	}
	ctx := c.Request.Context()
	domain, e := a.Branding.GetDomain(ctx, nil, actor, c.Param("id"))
	if e != nil {
		handleError(c, e)
		return
	}
	if domain.BrandID != op.BrandID {
		handleError(c, organization.ErrForbidden)
		return
	}
	switch action {
	case "verify":
		domain, e = a.Branding.VerifyAndActivateDomain(ctx, actor, domain.ID, a.DomainVerifier)
	case "enable", "disable":
		domain, e = a.Branding.SetDomainEnabled(ctx, nil, actor, domain.ID, action == "enable", in.Version)
	default:
		handleError(c, branding.ErrInvalidInput)
		return
	}
	if e != nil {
		handleError(c, e)
		return
	}
	c.JSON(200, gin.H{"id": domain.ID, "status": domain.Status, "version": domain.Version})
}
