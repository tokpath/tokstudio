package portal

import (
	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/organization"
)

// A parent may manage appearance through its own workspace. This does not grant
// membership in, or financial/customer access to, the target workspace.
func (a *Application) brandingTarget(c *gin.Context, op organization.Operator, selected ...string) (organization.Operator, error) {
	id := c.Query("target_operator_id")
	if len(selected) > 0 && selected[0] != "" {
		id = selected[0]
	}
	if id == "" || id == op.ID {
		return op, nil
	}
	child, err := a.Organization.GetOperator(c.Request.Context(), nil, id)
	if err != nil {
		return child, err
	}
	if child.ParentOperatorID == nil || *child.ParentOperatorID != op.ID {
		return child, organization.ErrForbidden
	}
	return child, nil
}

func (a *Application) appearanceTargets(c *gin.Context, op organization.Operator, channels bool) ([]organization.Operator, error) {
	out := []organization.Operator{}
	if !channels {
		out = append(out, op)
	}
	if op.Kind == organization.Channel {
		return out, nil
	}
	children, err := a.Organization.Children(c.Request.Context(), nil, op.ID)
	if err != nil {
		return nil, err
	}
	for _, child := range children {
		if (child.Kind == organization.Channel) == channels {
			out = append(out, child)
		}
	}
	return out, nil
}

func (a *Application) brandingResource(c *gin.Context, op organization.Operator, actor, resource, base string, p gin.H) {
	targets, err := a.appearanceTargets(c, op, resource == "channel-themes")
	if err != nil {
		handleError(c, err)
		return
	}
	items := []any{}
	for _, target := range targets {
		scope, id := "brand", target.BrandID
		if target.Kind == organization.Channel {
			scope, id = "channel", target.ID
		}
		config, e := a.Branding.ThemeConfiguration(c.Request.Context(), nil, actor, scope, id)
		if e != nil {
			if e == organization.ErrForbidden {
				continue
			}
			handleError(c, e)
			return
		}
		v := values(config.Values)
		v["name"], v["kind"], v["target_operator_id"] = target.Name, target.Kind, target.ID
		resolved, e := a.Branding.ResolveTheme(c.Request.Context(), nil, target.BrandID, channelThemeID(target))
		if e != nil {
			handleError(c, e)
			return
		}
		v["effective_theme"] = resolved.Theme
		row := entity(target.ID, "active", config.Version, v)
		row["allowed_actions"] = []string{"theme", "upload_logo"}
		items = append(items, row)
	}
	p["items"] = items
	p["description"] = "选择目标经营方调整外观。留空字段继承品牌默认配置；本页面不授予下级客户或账务权限。"
	p["columns"] = []gin.H{col("name", "目标经营方", "text"), col("kind", "类型", "text"), col("primary_color", "独立主色", "text"), col("background_color", "独立背景", "text"), col("logo_url", "独立Logo", "text"), col("support_url", "支持入口", "text")}
	p["row_operations"] = []operation{
		{Name: "theme", Label: "编辑网站外观", Method: "POST", Path: base + "?target_operator_id={id}", Fields: []field{jsonField("主题覆盖配置")}},
		{Name: "upload_logo", Label: "上传目标Logo", Method: "POST", Path: "/api/v1/operators/" + op.ID + "/branding/logo?target_operator_id={id}", Fields: []field{f("file", "Logo PNG/JPEG/WebP（不超过2MiB）", "file")}},
	}
	a.resourceResponse(c, p)
}
