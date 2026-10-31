package portal

import (
	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"github.com/tokpath/tokstudio/backend/internal/payments"
	"gorm.io/gorm"
)

func (a *Application) paymentQuoteSettingsResource(c *gin.Context, op organization.Operator, p gin.H, base string) {
	settings, err := a.Payments.FXSettings(c.Request.Context())
	if err != nil {
		handleError(c, err)
		return
	}
	v := values(settings)
	v["source_name"] = "Frankfurter"
	if settings.RateDate != nil {
		v["rate_date"] = settings.RateDate.Format("2006-01-02")
	}
	v["source_url"] = "https://frankfurter.dev/"
	p["title"] = "付款报价来源"
	p["description"] = "优先使用支付通道报价，通道无报价时使用 Frankfurter 参考汇率。首次外币付款时获取并缓存，之后自动更新。报价只确定本单付款金额，不换算资产、供应商成本或利润。"
	p["items"] = []any{entity("fx-settings", settings.Status, settings.Version, v)}
	p["columns"] = []gin.H{col("source_name", "参考汇率来源", "text"), col("source_url", "来源说明", "link"), col("enabled", "启用参考报价", "boolean"), col("last_success_at", "最近成功同步", "datetime"), col("rate_date", "汇率数据日期", "text")}
	p["operations"] = []operation{}
	p["row_operations"] = []operation{}
	if op.ID == a.Installation.OperatorID {
		source := selectField("source", "参考汇率来源", "frankfurter|Frankfurter")
		source.Default = "frankfurter"
		enabled := f("enabled", "启用参考报价", "boolean")
		enabled.Default = settings.Enabled
		enabled.Help = "无需人工填写汇率。没有有效报价时暂停该方式的跨币种付款；通道自己的报价仍优先使用。"
		p["operations"] = []operation{{Name: "configure", Label: "设置报价来源", Method: "POST", Path: base, Fields: []field{source, enabled, defaultField("expected_version", "配置版本", "hidden", settings.Version)}}}
	} else if !settings.Enabled {
		p["description"] = "请等待平台启用外币付款报价来源。通道自己的报价不受影响。"
	}
	a.resourceResponse(c, p)
}
func (a *Application) paymentQuoteSettingsCommand(c *gin.Context, op organization.Operator, actor string) {
	if op.ID != a.Installation.OperatorID {
		fail(c, 403, "platform_required", "请由平台配置付款报价来源", false)
		return
	}
	var in struct {
		Source   string `json:"source"`
		Enabled  bool   `json:"enabled"`
		Expected int64  `json:"expected_version"`
	}
	if !body(c, &in) {
		return
	}
	raw, err := a.command(c, "payment-quote-settings", in, func(tx *gorm.DB) (any, error) {
		svc := *a.Payments
		svc.DB = tx
		out, err := svc.SetFXSettings(c.Request.Context(), actor, op.ID, payments.FXSettings{Source: in.Source, Enabled: in.Enabled}, in.Expected)
		return commandResult("fx-settings", out.Status), err
	})
	jsonResponse(c, raw, err)
}
