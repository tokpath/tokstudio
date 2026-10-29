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
		defaultEndpoint := a.brandAPIEndpoint(c.Request.Context(), target.BrandID)
		for _, d := range rows {
			row := entity(d.ID, d.Status, d.Version, d)
			v := row["values"].(gin.H)
			v["operator_name"] = target.Name
			v["txt_record_name"] = "_tokenhub." + d.Hostname
			v["txt_record_value"] = "tokenhub-verification=" + d.VerificationToken
			v["api_entry_note"] = "待完成验证并启用"
			if d.Status == "disabled" {
				v["api_entry_note"] = "已停用"
			}
			if d.Status == "active" && d.VerifiedAt != nil && d.TLSReady && d.ProviderReference != "" {
				v["api_base_url"] = "https://" + d.Hostname + "/v1"
				v["api_entry_note"] = "可用备用入口"
				if v["api_base_url"] == defaultEndpoint {
					v["api_entry_note"] = "客户默认入口"
				}
			}
			actions := []string{"verify"}
			if d.Status == "active" {
				actions = append(actions, "disable")
			} else if d.VerifiedAt != nil && d.TLSReady && d.ProviderReference != "" {
				actions = append(actions, "enable")
			}
			row["allowed_actions"] = actions
			items = append(items, row)
		}
	}
	p["title"] = "品牌域名与TLS验证"
	p["description"] = "品牌API入口由已验证且已启用的域名自动生成，地址固定为 https://域名/v1；渠道沿用品牌入口。多个可用域名按最早添加顺序选定客户默认入口，停用后顺延。未完成DNS与TLS验证时显示待配置。检查已停用域名只更新验证结果，需另行启用。"
	p["items"] = items
	p["columns"] = []gin.H{col("operator_name", "品牌经营方", "text"), col("hostname", "域名", "text"), col("status", "状态", "status"), col("api_base_url", "API入口", "text"), col("api_entry_note", "入口用途", "text"), col("tls_ready", "有效TLS", "boolean"), col("txt_record_name", "TXT记录名称", "text"), col("txt_record_value", "TXT记录完整值", "text"), col("provider_status", "域名供应商状态", "text"), col("tls_status", "证书状态", "text"), col("provider_instructions", "实际DNS步骤", "details"), col("failure_reason", "失败原因", "text"), col("checked_at", "检查时间", "datetime")}
	// An OEM has no child brand, so domain and Google login actions always use that OEM.
	// A platform still chooses among its own brand and direct OEM children.
	target := f("target_operator_id", "目标品牌经营方", "text")
	if op.Kind == organization.OEM {
		target = defaultField("target_operator_id", "目标品牌经营方", "hidden", op.ID)
	}
	p["operations"] = []operation{{Name: "request", Label: "申请域名验证", Method: "POST", Path: base, Fields: []field{target, f("hostname", "品牌域名", "text")}}, {Name: "google", Label: "配置 Google 登录", Method: "POST", Path: base, Fields: []field{defaultField("action", "办理类型", "hidden", "google"), target, f("client_id", "Google 客户端编号", "text"), optional("client_secret", "Google 客户端密钥（已保存可留空）", "password"), defaultField("enabled", "测试通过后启用", "boolean", false), defaultField("config_version", "当前配置版本（首次为 0）", "integer", 0)}, Description: "每个品牌使用自己的 Google 客户端。回调地址是该品牌域名下的 /login/oauth/google。更换编号或密钥后需要重新测试。"}, {Name: "google_test", Label: "生成 Google 回调测试链接", Method: "POST", Path: base, Fields: []field{defaultField("action", "办理类型", "hidden", "google_test"), target, optional("hostname", "要测试的登录域名（官方品牌可留空）", "text")}, Description: "在对应品牌域名打开链接。测试只核对 Google 能否回到这个域名，不会创建客户或登录。"}}
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
	if in.Version != domain.Version {
		handleError(c, branding.ErrConflict)
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
