package portal

import (
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/incentives"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"github.com/tokpath/tokstudio/backend/internal/paymentmethods"
	"gorm.io/gorm"
)

func paymentMethodFields() []field {
	mode := selectField("fee_mode", "手续费规则", "none|不收费", "fixed|固定金额", "percent|按比例")
	mode.Default = "none"
	fixed := defaultField("flat_fee_money", "固定手续费 结算币种", "decimal", "0")
	fixed.VisibleWhen = &fieldVisibility{Field: "fee_mode", Values: []string{"fixed"}}
	rate := defaultField("fee_percent", "手续费率 %", "decimal", "0")
	rate.VisibleWhen = &fieldVisibility{Field: "fee_mode", Values: []string{"percent"}}
	rate.Help = "填写 2 表示收取 2%。收款按外部付款本金计算，提现按申请总额计算，均按清算币种最小支付单位四舍五入。"
	payout := defaultField("payout_enabled", "用于余额和佣金提现", "boolean", false)
	payout.Help = "同时开放两类提现，使用上面的同一套手续费。经营方线下付款后登记完成，系统不会自动转账。"
	kind := f("payout_kind", "提现收款材料", "select")
	kind.VisibleWhen = &fieldVisibility{Field: "payout_enabled", Values: []string{"true"}}
	for _, profile := range incentives.PayoutProfiles() {
		kind.Options = append(kind.Options, gin.H{"value": profile.Kind, "label": profile.OptionLabel})
	}
	return []field{defaultField("action", "操作", "hidden", "fee_rule"), f("name", "支付方式名称", "text"), mode, fixed, rate, payout, kind, defaultField("expected_version", "当前版本", "hidden", 0)}
}
func (a *Application) paymentMethodRules(c *gin.Context, op organization.Operator, p gin.H, base string) {
	rows, e := paymentmethods.List(c.Request.Context(), a.DB, op.ID)
	if e != nil {
		handleError(c, e)
		return
	}
	items := []any{}
	for _, r := range rows {
		v := values(r)
		v["fee_mode"] = "none"
		if r.FlatFeeMoney.Sign() > 0 {
			v["fee_mode"] = "fixed"
		}
		if r.FeeRate.Sign() > 0 {
			v["fee_mode"] = "percent"
		}
		v["fee_percent"] = ratePercentage(r.FeeRate)
		scope := "尚未启用"
		if r.Collection {
			scope = "充值、直接购买套餐、下级采购"
		}
		if r.PayoutEnabled {
			if r.Collection {
				scope = "充值、直接购买套餐、下级采购、余额提现、佣金提现"
			} else {
				scope = "余额提现、佣金提现"
			}
		}
		v["scope_label"] = scope
		row := entity(r.MethodID, "configured", r.Version, v)
		row["allowed_actions"] = []string{"fee_rule"}
		items = append(items, row)
	}
	p["title"] = "支付方式与手续费"
	p["description"] = "每种支付方式只设置一套手续费，适用于它支持的收款和提现。收款额外加收，提现从申请总额中扣除；余额和佣金内部抵扣不收费。修改只影响新确认的交易。"
	p["items"] = items
	p["columns"] = []gin.H{col("name", "支付方式", "text"), col("scope_label", "适用业务", "text"), col("flat_fee_money", "固定手续费", "money"), col("fee_percent", "手续费率 %", "decimal"), col("payout_enabled", "可用于提现", "boolean")}
	p["operations"] = []operation{{Name: "fee_rule", Label: "新增提现方式", Method: "POST", Path: base, Fields: paymentMethodFields(), Description: "新增的方式用于线下提现。收款方式请先在收款连接中配置，系统会自动在此建立同一套费用规则。"}}
	p["row_operations"] = []operation{{Name: "fee_rule", Label: "设置方式与费用", Method: "POST", Path: base + "/{id}/fee_rule", Fields: paymentMethodFields()}}
	a.resourceResponse(c, p)
}
func (a *Application) savePaymentMethodRule(c *gin.Context, op organization.Operator, actor, id string, raw json.RawMessage, tx *gorm.DB) (any, error) {
	var in struct {
		Name     string         `json:"name"`
		Mode     string         `json:"fee_mode"`
		Fixed    ledger.Decimal `json:"flat_fee_money"`
		Percent  ledger.Decimal `json:"fee_percent"`
		Payout   bool           `json:"payout_enabled"`
		Kind     string         `json:"payout_kind"`
		Expected int64          `json:"expected_version"`
	}
	if json.Unmarshal(raw, &in) != nil {
		return nil, paymentmethods.ErrInvalid
	}
	if in.Payout {
		if _, ok := incentives.FindPayoutProfile(in.Kind); !ok {
			return nil, paymentmethods.ErrInvalid
		}
	}
	if id == "" {
		id = uuid.NewString()
	}
	var out paymentmethods.Rule
	e := (func() error {
		r := paymentmethods.Rule{CreatedBy: actor, MethodID: id, OperatorID: op.ID, Name: in.Name, PayoutEnabled: in.Payout, PayoutKind: in.Kind}
		old, err := paymentmethods.Current(c.Request.Context(), tx, op.ID, id)
		if err != nil && err != gorm.ErrRecordNotFound {
			return err
		}
		r.Collection = old.Collection
		if in.Payout || old.PayoutEnabled {
			if err = a.Organization.RequirePermission(c.Request.Context(), tx, actor, op.ID, "withdrawals.manage"); err != nil {
				return err
			}
		}
		switch in.Mode {
		case "none":
		case "fixed":
			r.FlatFeeMoney = in.Fixed
		case "percent":
			r.FeeRate, err = percentageRate(in.Percent)
			if err != nil {
				return err
			}
		default:
			return paymentmethods.ErrInvalid
		}
		out, err = paymentmethods.Save(c.Request.Context(), tx, r, in.Expected)
		return err
	})()
	return commandResult(out.MethodID, "saved"), e
}
