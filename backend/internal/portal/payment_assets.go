package portal

import (
	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"github.com/tokpath/tokstudio/backend/internal/payments"
	"gorm.io/gorm"
)

func (a *Application) publicPaymentAssets(c *gin.Context) {
	rows, e := a.Payments.ListSupportedAssets(c.Request.Context())
	if e != nil {
		handleError(c, e)
		return
	}
	c.JSON(200, gin.H{"items": rows})
}
func (a *Application) paymentAssetsResource(c *gin.Context, op organization.Operator, actor string) {
	rows, e := a.Payments.AdminAssetNetworks(c.Request.Context(), actor, op.ID)
	if e != nil {
		handleError(c, e)
		return
	}
	p := page("支付资产与网络")
	items := []any{}
	for _, r := range rows {
		row := entity(r.VersionID, map[bool]string{true: "enabled", false: "disabled"}[r.Enabled], r.Version, r)
		row["allowed_actions"] = []string{"publish_version"}
		v := row["values"].(gin.H)
		v["asset_id"] = r.ID
		items = append(items, row)
	}
	p["description"] = "平台逐一核实资产、网络、合约和确认阈值后发布。经营方只能选择已生效组合；停用阻止新付款，原交易仍按历史快照核实。"
	p["items"] = items
	p["columns"] = []gin.H{col("driver", "协议", "text"), col("asset", "准确资产代码", "text"), col("network", "网络/Chain ID", "text"), col("token_contract", "合约", "text"), col("token_decimals", "小数位", "text"), col("minimum_confirmations", "最少确认数", "text"), col("effective_at", "生效时间", "datetime"), col("evidence_reference", "核实依据", "text")}
	fields := []field{optional("asset_id", "已有组合（首次留空）", "text"), selectField("driver", "收款协议", "evm_wallet|自有EVM钱包", "nowpayments|NOWPayments"), f("asset", "准确资产代码（NOW须用完整provider代码）", "text"), f("network", "明确网络（EVM为十进制Chain ID）", "text"), f("token_contract", "已核实代币合约", "text"), f("token_decimals", "代币小数位数", "integer"), f("minimum_confirmations", "最少链上确认数", "integer"), defaultField("enabled", "允许新的收款", "boolean", true), f("effective_at", "本版本生效时间", "datetime"), f("evidence_reference", "资产/网络/合约官方核实依据", "textarea"), defaultField("expected_version", "当前版本", "hidden", 0)}
	p["operations"] = []operation{{Name: "publish", Label: "发布支持的资产与网络", Method: "POST", Path: "/api/v1/operators/" + op.ID + "/payment-assets", Fields: fields}}
	rowFields := append([]field(nil), fields...)
	rowFields[0].Kind = "hidden"
	p["row_operations"] = []operation{{Name: "publish_version", Label: "发布此组合新版本", Method: "POST", Path: "/api/v1/operators/" + op.ID + "/payment-assets", IncludeVersion: true, Fields: rowFields}}
	a.resourceResponse(c, p)
}
func (a *Application) paymentAssetsCommand(c *gin.Context, op organization.Operator, actor string) {
	var in struct {
		payments.AssetNetwork
		AssetID         string `json:"asset_id"`
		ExpectedVersion int64  `json:"expected_version"`
	}
	if !body(c, &in) {
		return
	}
	raw, e := a.command(c, "payment-assets:"+op.ID, in, func(tx *gorm.DB) (any, error) {
		svc := *a.Payments
		svc.DB = tx
		in.AssetNetwork.ID = in.AssetID
		out, e := svc.PublishAssetNetwork(c.Request.Context(), actor, op.ID, in.AssetNetwork, in.ExpectedVersion)
		return gin.H{"id": out.VersionID, "status": "published", "version": out.Version}, e
	})
	jsonResponse(c, raw, e)
}
