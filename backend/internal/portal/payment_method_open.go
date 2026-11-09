package portal

import (
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"github.com/tokpath/tokstudio/backend/internal/paymentmethods"
	"github.com/tokpath/tokstudio/backend/internal/payments"
	"gorm.io/gorm"
	"strings"
)

func paymentCatalog() []gin.H {
	out := []gin.H{}
	labels := map[string]string{"manual": "线下办理", "stripe": "Stripe", "alipay": "支付宝线上收款", "wechat": "微信线上收款", "kvoucher": "KVoucher", "nowpayments": "NOWPayments", "evm_wallet": "自有链上钱包"}
	for _, m := range paymentmethods.Catalog() {
		drivers := []gin.H{}
		for _, driver := range m.Drivers {
			fields := []gin.H{}
			for _, material := range paymentMaterials {
				if contains(material.Drivers, driver) && !contains([]string{"chain_id", "token_contract", "token_decimals", "asset", "min_confirmations", "pay_currency", "pay_network", "pay_token_contract", "pay_asset_verification_reference"}, material.Key) {
					fields = append(fields, gin.H{"name": material.Key, "label": material.Label, "secret": contains([]string{"secret_key", "webhook_secret", "app_private_key", "merchant_api_private_key", "api_v3_key", "api_key", "ipn_secret", "kvoucher_api_key", "kvoucher_api_secret"}, material.Key)})
				}
			}
			drivers = append(drivers, gin.H{"value": driver, "label": labels[driver], "fields": fields})
		}
		out = append(out, gin.H{"kind": m.Kind, "label": m.Label, "payout": m.PayoutKind != "", "drivers": drivers})
	}
	return out
}
func methodOpenFields() []field {
	kind := f("method_kind", "支付方式", "select")
	for _, m := range paymentmethods.Catalog() {
		kind.Options = append(kind.Options, gin.H{"value": m.Kind, "label": m.Label})
	}
	return []field{defaultField("action", "操作", "hidden", "open_method"), kind, defaultField("enabled", "启用支付方式", "boolean", true), defaultField("collection", "用于收款", "boolean", true), defaultField("payout_enabled", "用于余额和佣金提现", "boolean", false), optional("driver", "收款办理方式", "text"), optional("credentials", "接入资料", "json"), optional("asset_network_version_id", "收款资产与网络", "text"), selectField("fee_mode", "手续费规则", "none|不收费", "fixed|固定金额", "percent|按比例"), defaultField("flat_fee_money", "固定手续费 结算币种", "decimal", "0"), defaultField("fee_percent", "手续费率 %", "decimal", "0"), defaultField("expected_version", "当前版本", "hidden", 0)}
}
func (a *Application) openPaymentMethod(c *gin.Context, op organization.Operator, actor, id string, raw json.RawMessage, tx *gorm.DB) (any, error) {
	builtin := id == "offline"
	var in struct {
		AssetNetworkVersionID string            `json:"asset_network_version_id"`
		Kind                  string            `json:"method_kind"`
		Enabled               bool              `json:"enabled"`
		Collection            bool              `json:"collection"`
		Payout                bool              `json:"payout_enabled"`
		Driver                string            `json:"driver"`
		Credentials           map[string]string `json:"credentials"`
		Mode                  string            `json:"fee_mode"`
		Fixed                 ledger.Decimal    `json:"flat_fee_money"`
		Percent               ledger.Decimal    `json:"fee_percent"`
		Expected              int64             `json:"expected_version"`
	}
	if json.Unmarshal(raw, &in) != nil {
		return nil, paymentmethods.ErrInvalid
	}
	if builtin {
		if in.Kind != "offline" {
			return nil, paymentmethods.ErrInvalid
		}
		id = ""
	}
	method, ok := paymentmethods.Find(in.Kind)
	if in.Enabled && !in.Collection && !in.Payout {
		return nil, paymentmethods.ErrInvalid
	}
	if !ok || in.Driver != "" && !method.Supports(in.Driver) || in.Payout && method.PayoutKind == "" {
		return nil, paymentmethods.ErrInvalid
	}
	if e := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", "method-kind:"+op.ID+":"+in.Kind).Error; e != nil {
		return nil, e
	}
	var identity paymentmethods.Identity
	found := tx.Where("operator_id=? AND kind=?", op.ID, in.Kind).Limit(1).Find(&identity)
	if found.Error != nil {
		return nil, found.Error
	}
	if id == "" {
		if identity.MethodID != "" {
			return nil, paymentmethods.ErrChanged
		}
		id = uuid.NewString()
	}
	old, e := paymentmethods.Current(c.Request.Context(), tx, op.ID, id)
	if e != nil && e != gorm.ErrRecordNotFound {
		return nil, e
	}
	if old.Version != in.Expected || old.Kind != "" && old.Kind != in.Kind {
		return nil, paymentmethods.ErrChanged
	}
	if in.Payout || old.PayoutEnabled {
		if e = a.Organization.RequirePermission(c.Request.Context(), tx, actor, op.ID, "withdrawals.manage"); e != nil {
			return nil, e
		}
	}
	rule := paymentmethods.Rule{MethodID: id, OperatorID: op.ID, Kind: in.Kind, Name: method.Label, Paused: !in.Enabled, Collection: in.Collection, PayoutEnabled: in.Payout, PayoutKind: method.PayoutKind, CreatedBy: actor}
	switch in.Mode {
	case "none":
	case "fixed":
		rule.FlatFeeMoney = in.Fixed
	case "percent":
		rule.FeeRate, e = percentageRate(in.Percent)
		if e != nil {
			return nil, e
		}
	default:
		return nil, paymentmethods.ErrInvalid
	}
	if _, e = paymentmethods.Save(c.Request.Context(), tx, rule, in.Expected); e != nil {
		return nil, e
	}
	if in.Collection && in.Kind == "offline" {
		in.Driver = "manual"
	}
	if in.Collection && len(method.Drivers) == 1 && method.Drivers[0] == "manual" {
		in.Driver = "manual"
	}
	merchantID := identity.MerchantID
	if in.Collection {
		merchantID = ""
	}
	if in.Collection && in.Driver != "" {
		svc := *a.Payments
		svc.DB = tx
		m := payments.Merchant{ID: uuid.NewString(), OperatorID: op.ID, PaymentMethodID: id, Driver: in.Driver, Credentials: map[string]string{}}
		expected := uint64(0)
		if identity.MerchantID == "" && old.MethodID != "" {
			if prior, err := svc.Merchant(c.Request.Context(), op.ID, old.MethodID); err == nil && prior.Driver == in.Driver {
				identity.MerchantID = prior.ID
			}
		}
		if identity.MerchantID != "" {
			previous, err := svc.Merchant(c.Request.Context(), op.ID, identity.MerchantID)
			if err != nil {
				return nil, err
			}
			if previous.Driver == in.Driver {
				m = previous
				m.PaymentMethodID = id
				expected = previous.Version
			}
		}
		if in.AssetNetworkVersionID != "" {
			m.AssetNetworkVersionID = in.AssetNetworkVersionID
		}
		for key, value := range in.Credentials {
			if strings.TrimSpace(value) != "" || key == "instructions" || key == "recipient_reference" {
				m.Credentials[key] = strings.TrimSpace(value)
			}
		}
		if in.Kind == "offline" {
			delete(m.Credentials, "recipient_reference")
			m.Credentials["instructions"] = "请联系经营方，按约定完成线下付款。"
		}
		delete(m.Credentials, "refund_channels")
		if m.Driver == "manual" {
			if in.Kind == "alipay" || in.Kind == "wechat" {
				m.Credentials["currency"] = "CNY"
			}
			if in.Kind == "usdt" {
				m.Credentials["currency"] = "USDT"
			}
		}
		if m.Driver == "manual" && m.Credentials["currency"] == "" {
			settings, err := ledger.GetMonetary(c.Request.Context(), tx)
			if err != nil {
				return nil, err
			}
			m.Credentials["currency"] = settings.Currency
		}
		if m.Driver == "manual" && m.Credentials["instructions"] == "" {
			m.Credentials["instructions"] = "请联系经营方，按约定完成线下付款。"
		}
		if err := svc.PrepareMerchantAsset(c.Request.Context(), &m, in.Kind); err != nil {
			return nil, err
		}
		m.Enabled = len(svc.Drivers[m.Driver].Validate(m)) == 0
		// Switching a payee creates a new connection; accepted payments retain the
		// old encrypted connection snapshot and the business fee identity is shared.
		if expected > 0 {
			previous, err := svc.Merchant(c.Request.Context(), op.ID, m.ID)
			if err != nil {
				return nil, err
			}
			for _, key := range []string{"app_id", "seller_id", "mch_id", "account_id", "recipient_reference", "wallet_address", "chain_id", "token_contract", "asset", "pay_currency", "pay_network", "pay_token_contract"} {
				if previous.Credentials[key] != m.Credentials[key] {
					m.ID = uuid.NewString()
					expected = 0
					break
				}
			}
		}
		view, err := svc.SaveMerchant(c.Request.Context(), actor, m, expected)
		if err != nil {
			return nil, err
		}
		if m.Driver == "kvoucher" && view.Merchant.Enabled {
			updated, err := svc.EnsureKVoucherWebhook(c.Request.Context(), view.Merchant, a.Config.PublicBaseURL)
			if err != nil {
				return nil, err
			}
			if updated.Credentials["webhook_id"] != view.Merchant.Credentials["webhook_id"] || updated.Credentials["webhook_secret"] != view.Merchant.Credentials["webhook_secret"] {
				if _, err = svc.SaveMerchant(c.Request.Context(), actor, updated, view.Version); err != nil {
					return nil, err
				}
			}
		}
		merchantID = m.ID
	}
	if e = tx.Model(&paymentmethods.Identity{}).Where("operator_id=? AND method_id=?", op.ID, id).Update("merchant_id", merchantID).Error; e != nil {
		return nil, e
	}
	return commandResult(id, "saved"), nil
}

func paymentMethodName(_ *gin.Context, m payments.PublicMethod) string { return m.Name }
