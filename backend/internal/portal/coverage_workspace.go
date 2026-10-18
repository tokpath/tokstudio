package portal

import (
	"errors"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"github.com/tokpath/tokstudio/backend/internal/cooperation"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"gorm.io/gorm"
)

// Coverage is a supply-risk summary. A parent can review its direct buyers'
// aggregate responsibility without entering their customer or money workspaces.
func (a *Application) coverageResource(c *gin.Context, op organization.Operator, base string, p gin.H) {
	ctx := c.Request.Context()
	targets := []organization.Operator{op}
	if op.Kind != organization.Channel {
		children, err := a.Organization.Children(ctx, nil, op.ID)
		if err != nil {
			handleError(c, err)
			return
		}
		targets = append(targets, children...)
	}
	items := []any{}
	for _, target := range targets {
		var snapshot cooperation.CoverageSnapshot
		var policy cooperation.OversellPolicy
		var coverageUnavailable bool
		// Keep the policy version and its displayed allowance in the same target
		// transaction. Coverage takes the domain's canonical supply-chain locks.
		err := a.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var err error
			snapshot, err = a.Cooperation.Coverage(ctx, tx, target.ID, time.Now())
			if err != nil {
				if !errors.Is(err, cooperation.ErrCoverage) && !errors.Is(err, catalog.ErrPriceUnavailable) && !errors.Is(err, catalog.ErrUnboundedCoverage) {
					return err
				}
				coverageUnavailable = true
				if err = a.Cooperation.Lock(ctx, tx, target.ID); err != nil {
					return err
				}
			}
			err = tx.Where("operator_id=?", target.ID).Take(&policy).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		})
		if err != nil {
			handleError(c, err)
			return
		}
		v := values(snapshot)
		v["name"], v["kind"], v["target_operator_id"] = target.Name, target.Kind, target.ID
		v["allowance"], v["warning"] = policy.Allowance, policy.WarningBuffer
		if coverageUnavailable {
			snapshot.Status = "pending_configuration"
			v["status"], v["catalog_version"] = snapshot.Status, nil
			v["coverage_reason"] = "模型及各层有效价格尚不能核实服务责任的采购覆盖；未知责任和销售空间未按零计算。"
			v["oversell"], v["warning_buffer"] = policy.Allowance, policy.WarningBuffer
			for _, key := range []string{"procurement_available", "required", "pending", "space"} {
				v[key] = nil
			}
		}
		allowed := []string{}
		switch {
		case target.Kind == organization.Platform:
			v["management_scope"] = "平台没有上级采购超卖额度；模型履约仍须供应商与合格路由就绪。"
			for _, key := range []string{"procurement_available", "required", "pending", "oversell", "space", "warning_buffer"} {
				v[key] = nil
			}
		case target.Kind == organization.OEM && target.ID == op.ID:
			v["management_scope"] = "OEM 自主设置销售超卖与预警；真实模型调用仍须足额采购余额。"
			allowed = append(allowed, "oversell")
		case target.Kind == organization.Channel && target.ID != op.ID:
			v["management_scope"] = "直属渠道的销售超卖由本经营方授权；不允许欠款调用。"
			allowed = append(allowed, "oversell")
		case target.Kind == organization.OEM:
			v["management_scope"] = "OEM 自主承担销售风险；平台不为 OEM 设置超卖或采购授信。"
		default:
			v["management_scope"] = "本渠道仅查看；超卖额度与预警由直属上级授权，真实调用须足额采购余额。"
		}
		row := entity(target.ID, snapshot.Status, policy.Version, v)
		row["allowed_actions"] = allowed
		items = append(items, row)
	}
	p["items"] = items
	p["description"] = "本人及直属下级的采购与履约风险汇总。销售超卖不等于采购授信，不能用来欠款调用；不包含下级客户与详细账务。"
	p["columns"] = []gin.H{col("name", "经营方", "text"), col("kind", "类型", "text"), col("procurement_available", "可用采购余额", "atg"), col("required", "已承诺责任", "atg"), col("pending", "待交付责任", "atg"), col("oversell", "销售超卖额度", "atg"), col("space", "剩余销售空间", "atg"), col("warning_buffer", "预警缓冲", "atg"), col("coverage_reason", "覆盖核实说明", "text"), col("management_scope", "办理权限与履约要求", "text")}
	p["operations"] = []operation{}
	p["row_operations"] = []operation{{Name: "oversell", Label: "设置授权超卖与缓冲", Method: "POST", Path: base + "?target_operator_id={id}", Permission: "cooperation.manage", Fields: []field{f("allowance", "销售超卖额度 ATG", "decimal"), f("warning", "预警缓冲 ATG", "decimal"), f("expected_version", "配置版本", "integer")}, Description: "只调整目标经营方的销售承载授权和预警。采购余额不足时不能继续模型调用。"}}
	a.resourceResponse(c, p)
}

func (a *Application) coverageTarget(c *gin.Context, tx *gorm.DB, op organization.Operator) (organization.Operator, error) {
	id := c.Query("target_operator_id")
	if id == "" {
		id = op.ID
	}
	if _, err := uuid.Parse(id); err != nil {
		return organization.Operator{}, organization.ErrInvalidInput
	}
	target, err := a.Organization.GetOperator(c.Request.Context(), tx, id)
	if err != nil {
		return target, err
	}
	if target.ID == op.ID {
		if op.Kind == organization.OEM {
			return target, nil
		}
		return target, organization.ErrForbidden
	}
	if op.Kind == organization.Channel || target.Kind != organization.Channel || target.ParentOperatorID == nil || *target.ParentOperatorID != op.ID {
		return target, organization.ErrForbidden
	}
	return target, nil
}

func (a *Application) coverageCommand(c *gin.Context, op organization.Operator, actor string) {
	var in struct {
		Allowance       ledger.Decimal `json:"allowance"`
		Warning         ledger.Decimal `json:"warning"`
		ExpectedVersion int64          `json:"expected_version"`
	}
	if !body(c, &in) {
		return
	}
	// Include the target in the idempotent command scope. Permission and parent
	// relationship are checked again inside the actual mutation transaction.
	target, err := a.coverageTarget(c, nil, op)
	if err != nil {
		handleError(c, err)
		return
	}
	raw, err := a.command(c, "oversell:"+op.ID+":"+target.ID, in, func(tx *gorm.DB) (any, error) {
		target, err := a.coverageTarget(c, tx, op)
		if err != nil {
			return nil, err
		}
		policy, err := a.Cooperation.SetOversell(c.Request.Context(), tx, actor, target.ID, in.Allowance, in.Warning, in.ExpectedVersion)
		if errors.Is(err, cooperation.ErrConflict) {
			return nil, organization.ErrConflict
		}
		if err != nil {
			return nil, err
		}
		return gin.H{"id": policy.OperatorID, "status": "updated", "version": policy.Version}, nil
	})
	jsonResponse(c, raw, err)
}
