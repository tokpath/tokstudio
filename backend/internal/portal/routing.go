package portal

import (
	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"gorm.io/gorm"
)

func (a *Application) routingResource(c *gin.Context, op organization.Operator, actor string, p gin.H, base string) {
	ctx := c.Request.Context()
	models, e := a.Catalog.AdminModels(ctx, op.ID)
	if e != nil {
		handleError(c, e)
		return
	}
	items := []any{}
	for _, model := range models {
		view, e := a.Catalog.AdminRouting(ctx, op.ID, model.ID)
		if e != nil {
			handleError(c, e)
			return
		}
		items = append(items, entity(model.ID, "configured", int64(view.Policy.Version), view))
	}
	p["title"] = "路由策略与动态健康"
	p["description"] = "仅在受理时已经验证的候选集合内选择。并发容量、未知用量与故障熔断均使用持久事实；冷却后只允许受控探测恢复。"
	p["items"] = items
	p["columns"] = []gin.H{col("id", "公开模型", "text"), col("policy", "实际策略版本", "details"), col("health", "候选路由健康与占用", "details")}
	p["row_operations"] = []operation{{Name: "policy", Label: "调整选路策略", Method: "POST", Path: base + "/{id}/policy", IncludeVersion: true, Fields: []field{selectField("strategy", "选择策略", "priority|固定优先级", "lowest_latency|已验证最低延迟", "lowest_cost|已验证最低成本"), f("evidence", "实际策略调整依据", "textarea")}}}
	a.resourceResponse(c, p)
}
func (a *Application) routingAction(c *gin.Context, op organization.Operator, actor string) {
	var in struct {
		Strategy string `json:"strategy"`
		Evidence string `json:"evidence"`
		Version  uint64 `json:"version"`
	}
	if !body(c, &in) {
		return
	}
	raw, e := a.command(c, "routing:"+op.ID+":"+c.Param("id"), in, func(tx *gorm.DB) (any, error) {
		cat := a.Catalog.WithDB(tx)
		e := cat.SaveRoutingPolicy(c.Request.Context(), op.ID, catalog.RoutingPolicy{ModelID: c.Param("id"), Strategy: in.Strategy, Evidence: in.Evidence}, in.Version)
		return gin.H{"id": c.Param("id"), "status": "configured", "version": in.Version + 1}, e
	})
	jsonResponse(c, raw, e)
}

func (a *Application) routeLimitsAction(c *gin.Context, op organization.Operator, actor string) {
	var in struct {
		ModelID string              `json:"public_model_id"`
		Routing catalog.RouteLimits `json:"routing"`
		Version uint64              `json:"version"`
	}
	if !body(c, &in) {
		return
	}
	raw, e := a.command(c, "route-limits:"+op.ID+":"+c.Param("id"), in, func(tx *gorm.DB) (any, error) {
		cat := a.Catalog.WithDB(tx)
		rows, e := cat.AdminUpstreams(c.Request.Context(), op.ID, in.ModelID)
		if e != nil {
			return nil, e
		}
		for _, upstream := range rows {
			if upstream.ID == c.Param("id") {
				upstream.Routing = in.Routing
				e = cat.SaveUpstream(c.Request.Context(), op.ID, upstream, in.Version)
				return gin.H{"id": upstream.ID, "status": "configured", "version": in.Version + 1}, e
			}
		}
		return nil, organization.ErrNotFound
	})
	jsonResponse(c, raw, e)
}
