package portal

import (
	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/organization"
)

// These are domain permissions, never synthetic navigation read flags. The
// selected actor's actual membership limits every offered grant.
var staffPermissionLabels = [][2]string{
	{"organization.read", "查看合作主体"}, {"organization.create", "开通合作主体"}, {"cooperation.manage", "管理采购与合作"}, {"business.manage", "调整营业设置"},
	{"staff.read", "查看人员"}, {"staff.manage", "邀请及调整人员"}, {"branding.manage", "管理本品牌外观"}, {"branding.manage_oem", "配置直属OEM外观"}, {"branding.manage_channels", "配置直属渠道外观"},
	{"customers.read", "查看客户服务"}, {"sales.manage", "管理套餐与模型定价"}, {"marketing.manage", "管理活动与赠送"}, {"support.manage", "客户支持"},
	{"operations.manage", "管理风险与通知"}, {"reports.summary", "查看经营汇总"}, {"reports.view", "查看经营报表"}, {"reports.financial", "查看财务报表"}, {"reports.export", "导出私有经营记录"},
	{"ledger.read", "查看资产和账本"}, {"payments.manage", "管理收款"}, {"refunds.manage", "管理退款及复核"}, {"withdrawals.manage", "管理提现"}, {"invoices.manage", "管理发票"}, {"security.manage", "管理安全限制"},
	{"catalog.manage", "维护平台模型与路由"}, {"providers.manage", "维护支付资产网络"}, {"platform.manage", "平台技术运营"},
}

func (a *Application) staffGrantField(c *gin.Context, actor string, op organization.Operator) (field, error) {
	field := optional("grants", "额外权限（可选）", "text")
	field.Help = "勾选当前经营范围明确授予的权限。角色自带权限仍按所选角色生效；这里只显示本人可授予的权限。"
	workspaces, err := a.Organization.MembershipsForStaff(c.Request.Context(), nil, actor)
	if err != nil {
		return field, err
	}
	permissions := map[string]bool{}
	for _, ws := range workspaces {
		if ws.Operator.ID == op.ID {
			for _, permission := range ws.Permissions {
				permissions[permission] = true
			}
		}
	}
	field.Options = []gin.H{}
	for _, permission := range staffPermissionLabels {
		if op.Kind != organization.Platform && contains([]string{"catalog.manage", "providers.manage", "platform.manage"}, permission[0]) {
			continue
		}
		if permissions["*"] || permissions[permission[0]] {
			field.Options = append(field.Options, gin.H{"value": permission[0], "label": permission[1]})
		}
	}
	return field, nil
}
