package portal

import (
	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/branding"
	"github.com/tokpath/tokstudio/backend/internal/organization"
)

// Domain administration can inspect a direct OEM through the platform workspace.
// Appearance commands never use this cross-operator target selection.
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
	if (resource == "channel-themes") != (op.Kind == organization.Channel) {
		fail(c, 404, "not_found", "经营资源不存在", false)
		return
	}
	items := []any{}
	for _, target := range []organization.Operator{op} {
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
		v["default_theme"] = branding.DefaultTheme()
		if target.Kind == organization.Channel {
			inherited, err := a.Branding.ResolveTheme(c.Request.Context(), nil, target.BrandID, "")
			if err != nil {
				handleError(c, err)
				return
			}
			v["default_theme"] = inherited.Theme
		}
		row := entity(target.ID, "active", config.Version, v)
		row["allowed_actions"] = []string{"theme"}
		items = append(items, row)
	}
	p["items"] = items
	p["description"] = "管理自己的网站配色、Logo 和客户支持入口。未设置的项目使用默认外观。"
	p["search_supported"], p["status_supported"] = false, false
	p["columns"] = []gin.H{col("name", "经营方", "text"), col("primary_color", "主色", "text"), col("background_color", "背景色", "text"), col("support_url", "支持入口", "text")}
	p["row_operations"] = []operation{
		{Name: "theme", Label: "编辑网站外观", Method: "POST", Path: base, Fields: []field{jsonField("主题覆盖配置")}},
	}
	a.resourceResponse(c, p)
}

func ownAppearanceTarget(c *gin.Context, op organization.Operator) error {
	if selected := c.Query("target_operator_id"); selected != "" && selected != op.ID {
		return organization.ErrForbidden
	}
	return nil
}
