package portal

import (
	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"github.com/tokpath/tokstudio/backend/internal/platform/mail"
)

func (a *Application) mailResource(c *gin.Context, op organization.Operator, p gin.H, base string) {
	settings, e := a.Mail.SettingsForBrand(c.Request.Context(), op.BrandID)
	if e != nil {
		handleError(c, e)
		return
	}
	p["status_supported"], p["search_supported"] = false, false
	p["title"] = "验证与通知邮件服务"
	p["description"] = "平台提供默认邮件服务，OEM 可使用默认服务或配置自己的 SMTP；渠道沿用所属品牌。发信需要外部 SMTP 服务及已授权邮箱，系统负责发送验证与通知邮件，不提供邮箱托管。"
	state := "pending_configuration"
	if settings.Available {
		state = "configured"
	}
	v := values(settings)
	v["mode_label"] = "本品牌独立 SMTP"
	if settings.Inherited {
		v["mode_label"] = "平台默认邮件服务"
	}
	if op.Kind == organization.Platform {
		v["mode_label"] = "平台默认邮件服务"
	}
	v["configuration_note"] = "请先向邮箱或邮件服务商取得 SMTP 主机、端口、认证信息和已授权发信地址，再保存配置。密码加密保存且不回显。配置预检通过只表示格式完整，实际投递还需服务商接受并送达。"
	if settings.Inherited {
		v["configuration_note"] = "使用平台默认邮件服务，发信名称自动使用客户所属品牌名称。平台的 SMTP 账号和密码不会向下级展示。"
		if !settings.Available {
			v["configuration_note"] = "等待平台配置默认邮件服务；OEM 也可以在此配置自己的 SMTP。"
		}
	}
	p["columns"] = []gin.H{col("mode_label", "发送方式", "text"), col("from", "实际发信地址", "text"), col("available", "通过配置预检", "boolean"), col("configuration_note", "配置说明", "text")}
	if op.Kind == organization.Channel {
		v = gin.H{"from": settings.From, "available": settings.Available, "configuration_note": "渠道沿用所属品牌的邮件服务，无需单独配置。"}
		if !settings.Available {
			v["configuration_note"] = "等待所属品牌方配置可用的邮件服务。"
		}
		p["columns"] = []gin.H{col("from", "实际发信地址", "text"), col("available", "通过配置预检", "boolean"), col("configuration_note", "配置说明", "text")}
		p["items"] = []any{entity(op.ID, state, 0, v)}
		a.resourceResponse(c, p)
		return
	}
	if !settings.Inherited {
		p["columns"] = append(p["columns"].([]gin.H), col("host", "SMTP服务器", "text"), col("port", "端口", "text"), col("from_name", "自定义发信名称", "text"), col("security", "传输安全", "text"), col("password_set", "密码已配置", "boolean"))
	}
	p["items"] = []any{entity(op.ID, state, 0, v)}
	security := selectField("security", "传输方式", "starttls|STARTTLS", "tls|TLS")
	security.Default = settings.Security
	if settings.Security == "" {
		security.Default = "starttls"
	}
	port := settings.Port
	if port == 0 {
		port = 587
	}
	username := optional("username", "SMTP认证用户名", "text")
	username.Default = settings.Username
	fromName := optional("from_name", "发信名称（留空使用品牌名称）", "text")
	fromName.Default = settings.FromName
	fields := []field{defaultField("enabled", "启用SMTP", "boolean", settings.Enabled), defaultField("host", "SMTP主机", "text", settings.Host), defaultField("port", "端口", "integer", port), username, optional("password", "新密码（留空保留）", "password"), defaultField("from", "已授权发信地址", "email", settings.From), fromName, security}

	help := map[string]string{
		"host":      "由邮箱或邮件服务商提供，例如 smtp.example.com；不是本网站域名。",
		"port":      "使用邮件服务商提供的端口；常见为 587 配合 STARTTLS，或 465 配合 TLS。",
		"password":  "填写邮件服务商提供的 SMTP 密码或授权码；留空保留本品牌已保存的密码。",
		"from":      "必须是邮件服务商允许发送的邮箱。自有发信域名的 DNS 验证按服务商指引在外部完成。",
		"from_name": "仅改变收件人看到的发信名称，不改变实际发信邮箱。留空自动使用品牌名称。",
	}
	for i := range fields {
		fields[i].Help = help[fields[i].Name]
	}
	if op.Kind == organization.OEM {
		mode := selectField("mode", "邮件发送方式", "platform|使用平台默认服务", "custom|使用本品牌的 SMTP")
		mode.Default = settings.Mode
		for i := range fields {
			fields[i].VisibleWhen = &fieldVisibility{Field: "mode", Values: []string{"custom"}}
		}
		if settings.Inherited {
			fields[0].Default = true
			for i := range fields {
				if fields[i].Name == "from" {
					fields[i].Default = ""
				}
			}
		}
		fields = append([]field{mode}, fields...)
	}
	p["operations"] = []operation{{Name: "configure", Label: "保存邮件服务配置", Method: "POST", Path: base, Fields: fields, Description: "未接入邮件时，可用已有测试账号验收其他业务；注册验证、找回密码及真实邮件送达需在接入 SMTP 后验收。Google 登录为可选功能，不影响密码登录。"}}
	a.resourceResponse(c, p)
}

func (a *Application) mailCommand(c *gin.Context, op organization.Operator) {
	if op.Kind == organization.Channel {
		handleError(c, organization.ErrForbidden)
		return
	}
	var in mail.SettingsInput
	if !body(c, &in) {
		return
	}
	settings, e := a.Mail.UpdateForBrand(c.Request.Context(), in, op.BrandID)
	if e != nil {
		handleError(c, e)
		return
	}
	state := "pending_configuration"
	if settings.Available {
		state = "configured"
	}
	c.JSON(200, commandResult(op.ID, state))
}

// Staff may manage an OEM while browsing the platform domain. Delivery follows
// the selected workspace's brand, rather than the host used to open management.
func (a *Application) operatorMailAvailable(c *gin.Context) bool {
	op, err := a.Organization.GetOperator(c.Request.Context(), nil, c.Param("operatorId"))
	return err == nil && a.Mail.AvailableForBrand(op.BrandID)
}
