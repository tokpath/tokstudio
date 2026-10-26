package portal

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/modelpricing"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"gorm.io/gorm"
	"runtime/debug"
)

func (a *Application) modelSettingsResource(c *gin.Context, p gin.H, base string) {
	settings, e := a.ModelPricing.Settings(c.Request.Context())
	if e != nil {
		handleError(c, e)
		return
	}
	p["title"] = "Bifrost 设置"
	p["description"] = "同步模型供应商参考价目，用于上游成本核算。客户与下级经营方继续按各自的 ATG 价目计费。"
	p["summary"] = []gin.H{{"label": "Bifrost Core", "value": bifrostDependencyVersion("core")}, {"label": "成本计算组件", "value": bifrostDependencyVersion("framework")}, {"label": "自动同步", "value": "每 24 小时"}}
	p["items"] = []any{entity("platform", "configured", int64(settings.Version), settings)}
	p["columns"] = []gin.H{col("last_success_at", "上次成功同步", "datetime"), col("last_attempt_at", "最近尝试", "datetime"), col("sync_requested", "等待同步", "boolean"), col("last_error", "同步提示", "text")}
	p["operations"] = []operation{{Name: "sync", Label: "立即同步", Method: "POST", Path: base, Fields: []field{defaultField("action", "操作", "hidden", "sync")}, Description: "系统每 24 小时自动同步 Bifrost 官方价目；失败会自动重试并保留上次成功的数据。也可手动提交同步，刷新本页查看结果。"}}
	a.resourceResponse(c, p)
}
func (a *Application) modelSettingsCommand(c *gin.Context, op organization.Operator, actor string) {
	var in struct {
		Action string `json:"action"`
	}
	if !body(c, &in) {
		return
	}
	raw, e := a.command(c, "model-settings:"+op.ID, in, func(tx *gorm.DB) (any, error) {
		service := *a.ModelPricing
		service.DB = tx
		if in.Action == "sync" {
			return commandResult("platform", "pending"), service.RequestSync(c.Request.Context())
		}
		return nil, modelpricing.ErrInput
	})
	jsonResponse(c, raw, e)
}
func (a *Application) actualCostResource(c *gin.Context, p gin.H, base string) {
	rows, e := a.ModelPricing.Actuals(c.Request.Context())
	if e != nil {
		handleError(c, e)
		return
	}
	items := []any{}
	credentials, e := a.Catalog.AdminCredentials(c.Request.Context(), a.Installation.OperatorID)
	if e != nil {
		handleError(c, e)
		return
	}
	names := map[string]string{}
	for _, credential := range credentials {
		if names[credential.ProviderID] == "" {
			names[credential.ProviderID] = credential.Name
		}
	}
	models, e := a.Catalog.AdminSupplierModels(c.Request.Context(), a.Installation.OperatorID)
	if e != nil {
		handleError(c, e)
		return
	}
	modelNames := map[string]string{}
	for _, model := range models {
		modelNames[model.ID] = model.Model
	}

	for _, row := range rows {
		item := entity(row.ID, "recorded", int64(row.Version), row)
		var totals struct {
			Cost    ledger.Decimal
			Pending int64
		}
		q := a.DB.WithContext(c.Request.Context()).Table("usage_attempts").Where("provider_id=? AND started_at>=? AND started_at<?", row.ProviderID, row.StartsAt, row.EndsAt)
		if row.SupplierModelID != "" {
			q = q.Where("supplier_model_id=?", row.SupplierModelID)
		}
		if e = q.Select("COALESCE(sum(calculated_cost_usd),0)::text AS cost,count(*) FILTER (WHERE calculated_cost_usd IS NULL) AS pending").Scan(&totals).Error; e != nil {
			handleError(c, e)
			return
		}
		values := item["values"].(gin.H)
		values["provider_name"] = names[row.ProviderID]
		if row.SupplierModelID == "" {
			values["model_name"] = "供应商整体"
		} else {
			values["model_name"] = modelNames[row.SupplierModelID]
		}
		item["presentation"] = gin.H{"title": humanJoin(names[row.ProviderID], fmt.Sprint(values["model_name"]))}
		values["calculated_usd"] = totals.Cost
		values["difference_usd"] = row.AmountUSD.Sub(totals.Cost)
		values["pending_count"] = totals.Pending
		items = append(items, item)
	}
	p["title"] = "真实上游成本"
	p["description"] = "登记供应商实际费用，与同一供应商、模型和期间的系统核算金额并列对比。登记不会修改调用费用或客户账单。结束时间不包含在统计期间内。"
	p["items"] = items
	p["columns"] = []gin.H{col("provider_name", "供应商", "text"), col("model_name", "供应商模型", "text"), col("starts_at", "期间开始", "datetime"), col("ends_at", "期间结束", "datetime"), col("calculated_usd", "系统核算 USD", "usd"), col("amount_usd", "登记实际 USD", "usd"), col("difference_usd", "差额 USD", "usd"), col("pending_count", "未取得核算金额的调用", "text"), col("note", "备注", "text")}
	supplier := f("provider_id", "供应商", "text")
	supplier.LookupURL = "/api/v1/operators/" + c.Param("operatorId") + "/references/cost-suppliers"
	model := optional("supplier_model_id", "供应商模型（留空表示供应商整体）", "text")
	model.LookupURL = "/api/v1/operators/" + c.Param("operatorId") + "/references/cost-models"
	fields := []field{supplier, model, f("starts_at", "期间开始", "datetime"), f("ends_at", "期间结束", "datetime"), f("amount_usd", "实际费用 USD", "decimal"), optional("note", "备注", "textarea")}
	p["row_operations"] = []operation{{Name: "edit", Label: "修改登记", Method: "POST", Path: base, Fields: append([]field{optional("id", "记录", "hidden"), defaultField("expected_version", "当前版本", "hidden", 0)}, fields...)}}
	p["operations"] = []operation{{Name: "record", Label: "登记实际费用", Method: "POST", Path: base, Fields: fields}}
	a.resourceResponse(c, p)
}
func (a *Application) actualCostCommand(c *gin.Context, op organization.Operator, actor string) {
	var in struct {
		modelpricing.ActualCost
		Expected uint64 `json:"expected_version"`
	}
	if !body(c, &in) {
		return
	}
	raw, e := a.command(c, "model-actual-cost:"+op.ID, in, func(tx *gorm.DB) (any, error) {
		if in.Expected == 0 {
			in.ID = uuid.NewString()
		}
		service := *a.ModelPricing
		service.DB = tx
		return commandResult(in.ID, "recorded"), service.SaveActual(c.Request.Context(), in.ActualCost, in.Expected)
	})
	jsonResponse(c, raw, e)
}

func bifrostDependencyVersion(module string) string {
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, dep := range info.Deps {
			if dep.Path == "github.com/maximhq/bifrost/"+module {
				return dep.Version
			}
		}
	}
	return "unknown"
}
