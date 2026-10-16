package portal

import (
	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/platform/mail"
)

func (a *Application) mailResource(c *gin.Context, p gin.H, base string) {
	settings, e := a.Mail.Settings(c.Request.Context())
	if e != nil {
		handleError(c, e)
		return
	}
	p["title"] = "验证与通知邮件服务"
	p["description"] = "SMTP配置仅平台维护，密码加密保存且不回显。已接收表示SMTP服务器接受投递，不代表邮箱最终送达。"
	p["items"] = []any{entity("platform", "configured", 0, settings)}
	p["columns"] = []gin.H{col("host", "SMTP服务器", "text"), col("port", "端口", "text"), col("from", "发信地址", "text"), col("security", "传输安全", "text"), col("password_set", "密码已配置", "boolean"), col("available", "通过配置预检", "boolean"), col("source", "配置来源", "text")}
	p["operations"] = []operation{{Name: "configure", Label: "保存邮件服务配置", Method: "POST", Path: base, Fields: []field{f("enabled", "启用SMTP", "boolean"), f("host", "SMTP主机", "text"), f("port", "端口", "integer"), optional("username", "SMTP认证用户名", "text"), optional("password", "新密码（留空保留）", "password"), f("from", "已授权发信地址", "email"), selectField("security", "传输方式", "starttls|STARTTLS", "tls|TLS", "plain|明文（仅隔离测试）")}}}
	a.resourceResponse(c, p)
}
func (a *Application) mailCommand(c *gin.Context) {
	var in mail.SettingsInput
	if !body(c, &in) {
		return
	}
	settings, e := a.Mail.Update(c.Request.Context(), in)
	if e != nil {
		handleError(c, e)
		return
	}
	state := "pending_configuration"
	if settings.Available {
		state = "configured"
	}
	c.JSON(200, commandResult("platform", state))
}
