package portal

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/incentives"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"github.com/tokpath/tokstudio/backend/internal/paymentmethods"
	"github.com/tokpath/tokstudio/backend/internal/platform/httpx"
	"gorm.io/gorm"
	"strings"
)

func (a *Application) paymentMethodRules(c *gin.Context, op organization.Operator, p gin.H, base string) {
	rows, e := paymentmethods.List(c.Request.Context(), a.DB, op.ID)
	if e != nil {
		handleError(c, e)
		return
	}
	public, e := a.Payments.PublicMethods(c.Request.Context(), op.ID)
	if e != nil {
		handleError(c, e)
		return
	}
	ready := map[string]bool{}
	reasons := map[string]string{}
	for _, m := range public {
		ready[m.ID] = m.SettlementSupported
		reasons[m.ID] = m.UnsupportedReason
	}
	items := []any{}
	for _, r := range rows {
		if r.Kind == "" && r.Paused || r.Kind == "cash" || r.Kind == "bank_card" {
			continue
		}
		v := values(r)
		v["enabled"] = !r.Paused
		v["driver"] = ""
		v["credentials"] = gin.H{}
		state := "awaiting_connection"
		collectionReady := false
		var identity paymentmethods.Identity
		if err := a.DB.Where("operator_id=? AND method_id=?", op.ID, r.MethodID).Limit(1).Find(&identity).Error; err != nil {
			handleError(c, err)
			return
		}
		merchantID := identity.MerchantID
		if r.Kind == "" {
			merchantID = r.MethodID
		}
		if merchantID != "" {
			m, err := a.Payments.Merchant(c.Request.Context(), op.ID, merchantID)
			if err == nil {
				v["driver"] = m.Driver
				if r.Kind == "" {
					v["method_kind"] = inferredMethodKind(m.Driver, r.PayoutKind)
				}
				safe := gin.H{}
				for _, material := range paymentMaterials {
					if contains(material.Drivers, m.Driver) && !contains([]string{"secret_key", "webhook_secret", "app_private_key", "merchant_api_private_key", "api_v3_key", "kvoucher_api_key", "kvoucher_api_secret"}, material.Key) {
						safe[material.Key] = m.Credentials[material.Key]
					}
				}
				v["credentials"] = safe
				state = "pending_configuration"
				collectionReady = ready[m.ID]
				missing := a.Payments.Drivers[m.Driver].Validate(m)
				labels := []string{}
				for _, key := range missing {
					label := key
					for _, material := range paymentMaterials {
						if material.Key == key {
							label = material.Label
							break
						}
					}
					labels = append(labels, label)
				}
				v["readiness_note"] = strings.Join(labels, "、")
				if len(missing) == 0 && !collectionReady {
					v["readiness_note"] = reasons[m.ID]
					if reasons[m.ID] == "" {
						v["readiness_note"] = "等待平台完成收款所需配置"
					}
				}
			} else if err != gorm.ErrRecordNotFound {
				handleError(c, err)
				return
			}
		}
		if r.Kind == "" && v["method_kind"] == nil {
			v["method_kind"] = inferredMethodKind("", r.PayoutKind)
		}
		if m, ok := paymentmethods.Find(fmt.Sprint(v["method_kind"])); ok {
			v["method_label"] = m.Label
			v["name"] = httpx.LocalizedText(httpx.RequestLocale(c.Request), m.Label)
		}
		if !r.Collection || r.Paused {
			v["readiness_note"] = ""
		}
		v["collection_ready"] = r.Collection && collectionReady && !r.Paused
		v["payout_ready"] = r.PayoutEnabled && !r.Paused
		if r.PayoutEnabled || r.Collection && collectionReady {
			state = "ready"
		}
		if r.Paused {
			state = "disabled"
		}

		v["collection_status"] = "未启用"
		v["payout_status"] = "未启用"
		if r.Collection && !r.Paused {
			v["collection_status"] = "待配置"
			if collectionReady {
				v["collection_status"] = "可收款"
			}
		}
		if r.PayoutEnabled && !r.Paused {
			v["payout_status"] = "人工办理"
		}
		if profile, ok := incentives.FindPayoutProfile(r.PayoutKind); ok {
			v["payout_summary"] = profile.Summary
		}
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
		if op.Kind == organization.Channel {
			scope = strings.ReplaceAll(scope, "、下级采购", "")
		}
		v["scope_label"] = scope
		row := entity(r.MethodID, state, r.Version, v)
		row["allowed_actions"] = []string{"open_method"}
		items = append(items, row)
	}
	p["title"] = "支付方式与手续费"
	p["description"] = "每种支付方式只设置一套手续费，适用于它支持的收款和提现。收款额外加收，提现从申请总额中扣除；余额和佣金内部抵扣不收费。修改只影响新确认的交易。"
	p["items"] = items
	p["columns"] = []gin.H{col("scope_label", "适用业务", "text"), col("flat_fee_money", "固定手续费", "money"), col("fee_percent", "手续费率 %", "decimal"), col("collection_status", "收款", "text"), col("payout_status", "提现", "text"), col("readiness_note", "待补充资料", "text")}
	catalog := paymentCatalog()
	for _, m := range catalog {
		for _, r := range rows {
			if r.Kind == m["kind"] {
				m["existing_id"] = r.MethodID
			}
		}
	}
	p["payment_method_catalog"] = catalog
	p["operations"] = []operation{{Name: "open_method", Label: "添加支付方式", Method: "POST", Path: base, Fields: methodOpenFields()}}
	p["row_operations"] = []operation{{Name: "open_method", Label: "管理支付方式", Method: "POST", Path: base + "/{id}/open_method", Fields: methodOpenFields()}}
	a.resourceResponse(c, p)
}
func inferredMethodKind(driver, payout string) string {
	switch driver {
	case "manual":
		return "offline"
	case "stripe":
		return "stripe"
	case "alipay", "wechat", "kvoucher":
		return driver
	}
	if payout == "bank_cn" {
		return "bank_transfer"
	}
	return payout
}
