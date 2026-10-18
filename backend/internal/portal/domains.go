package portal

import (
	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/branding"
	"github.com/tokpath/tokstudio/backend/internal/organization"
)

func (a *Application) domainResource(c *gin.Context, op organization.Operator, actor string, p gin.H, base string) {
	targets, e := a.appearanceTargets(c, op, false)
	if e != nil {
		handleError(c, e)
		return
	}
	items := []any{}
	canRequest := false
	for _, target := range targets {
		if target.Kind == organization.Channel {
			continue
		}
		rows, e := a.Branding.ListDomains(c.Request.Context(), nil, actor, target.BrandID)
		if e == organization.ErrForbidden {
			continue
		}
		if e != nil {
			handleError(c, e)
			return
		}
		canRequest = true
		for _, d := range rows {
			row := entity(d.ID, d.Status, d.Version, d)
			row["values"].(gin.H)["operator_name"] = target.Name
			actions := []string{"verify"}
			if d.Status == "active" {
				actions = append(actions, "disable")
			} else if d.VerifiedAt != nil && d.TLSReady {
				actions = append(actions, "enable")
			}
			row["allowed_actions"] = actions
			items = append(items, row)
		}
	}
	p["title"] = "品牌域名与TLS验证"
	p["description"] = "为本品牌或有权管理的直属OEM配置域名，通过DNS所有权及有效TLS检查后启用。渠道沿用品牌域名，由直属上级定制外观。"
	p["items"] = items
	p["columns"] = []gin.H{col("operator_name", "品牌经营方", "text"), col("hostname", "域名", "text"), col("status", "状态", "status"), col("tls_ready", "有效TLS", "boolean"), col("verification_token", "TXT验证值", "text"), col("provider_status", "域名供应商状态", "text"), col("tls_status", "证书状态", "text"), col("provider_instructions", "实际DNS步骤", "text"), col("failure_reason", "失败原因", "text"), col("checked_at", "检查时间", "datetime")}
	p["operations"] = []operation{{Name: "request", Label: "申请域名验证", Method: "POST", Path: base, Fields: []field{f("target_operator_id", "目标品牌经营方", "text"), f("hostname", "品牌域名", "text")}}}
	if !canRequest {
		p["operations"] = []operation{}
	}
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
	brand, e := a.Organization.GetBrand(ctx, nil, domain.BrandID)
	if e != nil {
		handleError(c, e)
		return
	}
	_, e = a.brandingTarget(c, op, brand.OwnerOperatorID)
	if e != nil {
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
