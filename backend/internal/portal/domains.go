package portal

import (
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/branding"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"time"
)

func (a *Application) domainResource(c *gin.Context, op organization.Operator, actor string, p gin.H, base string) {
	if op.Kind == organization.Channel {
		handleError(c, organization.ErrForbidden)
		return
	}
	targets, e := a.appearanceTargets(c, op, false)
	if e != nil {
		handleError(c, e)
		return
	}
	items := []any{}
	if op.Kind == organization.Platform {
		row := entity("official", "active", 1, gin.H{"operator_name": op.Name, "hostname": a.Config.OfficialHost, "state_label": "平台官方入口", "api_base_url": a.brandAPIEndpoint(c.Request.Context(), op.BrandID)})
		row["allowed_actions"] = []string{}
		items = append(items, row)
	}
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
		if target.ID == op.ID && a.Organization.RequireBrandBranding(c.Request.Context(), nil, actor, target.BrandID) == nil {
			canRequest = true
		}
		defaultEndpoint := a.brandAPIEndpoint(c.Request.Context(), target.BrandID)
		for _, d := range rows {
			row := entity(d.ID, d.Status, d.Version, d)
			v := row["values"].(gin.H)
			v["operator_name"] = target.Name
			history, e := a.domainHistory(c, target.BrandID, d.ID)
			if e != nil {
				handleError(c, e)
				return
			}
			v["change_history"] = history
			v["txt_record_name"] = "_tokenhub." + d.Hostname
			v["txt_record_value"] = "tokenhub-verification=" + d.VerificationToken
			v["api_entry_note"] = "待完成验证并启用"
			if d.Status == "disabled" {
				v["api_entry_note"] = "已停用"
			}
			if d.Status == "active" && d.VerifiedAt != nil && d.TLSReady && d.RouteReady && d.ProviderReference != "" {
				v["api_base_url"] = "https://" + d.Hostname + "/v1"
				v["api_entry_note"] = "可用备用入口"
				if v["api_base_url"] == defaultEndpoint {
					v["api_entry_note"] = "客户默认入口"
				}
			}
			actions := []string{"verify"}
			if op.Kind == organization.Platform && d.Status == "active" {
				actions = append(actions, "disable")
			} else if op.Kind == organization.Platform && d.CheckedAt != nil && time.Since(*d.CheckedAt) < 10*time.Minute && d.VerifiedAt != nil && d.TLSReady && d.RouteReady && d.ProviderReference != "" {
				actions = append(actions, "enable")
			}
			v["state_label"] = "等待 DNS 配置"
			if d.VerifiedAt != nil {
				v["state_label"] = "等待 HTTPS 与接入验证"
			}
			if d.TLSReady && d.RouteReady {
				v["state_label"] = "验证通过，等待平台启用"
			}
			if d.Status == "active" {
				v["state_label"] = "已启用"
			}
			if d.Status == "disabled" {
				v["state_label"] = "已停用"
			}
			instructions := []branding.DNSInstruction{{Type: "TXT", Name: "_tokenhub." + d.Hostname, Value: "tokenhub-verification=" + d.VerificationToken}}
			if a.Config.DomainCNAMETarget != "" {
				instructions = append(instructions, branding.DNSInstruction{Type: "CNAME", Name: d.Hostname, Value: a.Config.DomainCNAMETarget})
			}
			var provider []branding.DNSInstruction
			_ = json.Unmarshal(d.ProviderInstructions, &provider)
			for _, extra := range provider {
				duplicate := false
				for _, existing := range instructions {
					if existing.Type == extra.Type && existing.Name == extra.Name && existing.Value == extra.Value {
						duplicate = true
					}
				}
				if !duplicate {
					instructions = append(instructions, extra)
				}
			}
			v["dns_records"] = instructions
			if a.Config.DomainCNAMETarget == "" && !d.RouteReady {
				v["routing_note"] = "等待平台提供域名接入地址。DNS 和 HTTPS 接入由平台统一维护。"
			}
			row["allowed_actions"] = actions
			items = append(items, row)
		}
	}
	p["title"] = "品牌域名"
	p["description"] = "登记自己的域名并按提示配置 DNS，系统自动检查。验证通过后由平台启用。更换域名时，原域名保持可用，直到平台启用新域名；请提前安排客户更新 API 地址。"
	p["items"] = items
	p["search_supported"], p["status_supported"] = false, false
	p["columns"] = []gin.H{col("operator_name", "品牌经营方", "text"), col("hostname", "域名", "text"), col("state_label", "状态", "text"), col("checked_at", "检查时间", "datetime")}
	target := defaultField("target_operator_id", "品牌经营方", "hidden", op.ID)
	google, e := a.Organization.GoogleConfig(c.Request.Context(), nil, op.BrandID)
	if e != nil {
		handleError(c, e)
		return
	}
	p["operations"] = []operation{{Name: "request", Label: "登记品牌域名", Method: "POST", Path: base, Fields: []field{target, f("hostname", "品牌域名", "text")}}, {Name: "google", Label: "配置 Google 登录", Method: "POST", Path: base, Fields: []field{defaultField("action", "办理类型", "hidden", "google"), target, defaultField("client_id", "Google 客户端编号", "text", google.ClientID), optional("client_secret", "Google 客户端密钥（已保存可留空）", "password"), defaultField("enabled", "测试通过后启用", "boolean", google.Enabled), defaultField("config_version", "当前配置版本", "hidden", google.Version)}, Description: "Google 登录为可选功能，不影响邮箱密码登录。先在 Google Cloud 创建 OAuth 客户端，将品牌域名下的 /login/oauth/google 登记为回调地址，再填写客户端编号与密钥；测试通过后启用。"}, {Name: "google_test", Label: "生成 Google 回调测试链接", Method: "POST", Path: base, Fields: []field{defaultField("action", "办理类型", "hidden", "google_test"), target, optional("hostname", "要测试的登录域名（官方品牌可留空）", "text")}, Description: "在对应品牌域名打开链接。测试只核对 Google 能否回到这个域名，不会创建客户或登录。"}}
	if op.Kind == organization.Platform {
		p["operations"] = p["operations"].([]operation)[1:]
		p["description"] = "官方入口由部署配置维护。OEM 自行登记域名和配置 DNS；平台检查验证结果后启用，也可以停用域名。"
	}
	if !canRequest {
		p["operations"] = []operation{}
	}
	p["row_operations"] = []operation{{Name: "verify", IncludeVersion: true, Label: "重新检查", Method: "POST", Path: base + "/{id}/verify", Fields: []field{}, Description: "检查 DNS、HTTPS 和品牌接入，结果会自动更新。验证通过后仍需平台启用。"}, {Name: "enable", IncludeVersion: true, Label: "启用域名", Description: "启用后，该品牌的原域名将停用。客户需要使用新的网址和 API 地址。", Confirmation: "确认启用该品牌域名", Method: "POST", Path: base + "/{id}/enable", Fields: []field{}}, {Name: "disable", IncludeVersion: true, Label: "停用域名", Method: "POST", Path: base + "/{id}/disable", Fields: []field{}}}
	if op.Kind != organization.Platform {
		p["row_operations"] = p["row_operations"].([]operation)[:1]
	}
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
		domain, e = a.Branding.VerifyDomain(ctx, actor, domain.ID, a.DomainVerifier)
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
