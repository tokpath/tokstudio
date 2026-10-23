package portal

import (
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"strings"
)

func (a *Application) resourceResponse(c *gin.Context, p gin.H) {
	if err := a.projectResourceLabels(c, p, c.Param("resource")); err != nil {
		handleError(c, err)
		return
	}
	a.prepareResourcePage(c, p, c.Param("resource"))
	c.JSON(200, p)
}
func (a *Application) prepareResourcePage(c *gin.Context, p gin.H, resource string) {
	prepareResourceFilters(c, p, resource)
	scope := "/api/v1/me"
	if c.Param("operatorId") != "" {
		scope = "/api/v1/operators/" + c.Param("operatorId")
	}
	version := int64(0)
	if rows, ok := p["items"].([]any); ok && len(rows) > 0 {
		if row, ok := rows[0].(gin.H); ok {
			if v, ok := row["version"].(int64); ok {
				version = v
			}
		}
	}
	for _, key := range []string{"operations", "row_operations"} {
		ops, ok := p[key].([]operation)
		if !ok {
			continue
		}
		for i := range ops {
			if c.Param("operatorId") != "" && ops[i].Permission == "" {
				ops[i].Permission = operationPermission(resource, ops[i].Name)
			}
			ops[i].Fields = businessFields(ops[i])
			if resource == "payment-methods" && a.Config.TestLoopback {
				ops[i].Fields = append(ops[i].Fields, optional("api_base_url", "独立协议测试地址（仅隔离测试）", "text"))
			}
			for j := range ops[i].Fields {
				f := &ops[i].Fields[j]
				if f.Name == "action" && f.Kind == "text" && f.Default != nil {
					f.Kind = "hidden"
				}
				if f.Name == "expected_version" || f.Name == "version" || strings.HasSuffix(f.Name, "_version") {
					f.Kind = "hidden"
					if f.Default == nil {
						f.Default = 0
					}
					// Nested configuration fields already carry their own policy CAS;
					// the first business record is not that configuration's version.
					if (f.Name == "expected_version" || f.Name == "version") && key == "operations" && contains([]string{"commissions", "business-status", "branding", "channel-themes", "coverage"}, resource) {
						f.Default = version
					}
				}
				lookup := ""
				switch f.Name {
				case "owner_id":
					if resource == "exports" {
						lookup = "export-subjects"
					}
				case "recipient_id":
					lookup = "recipients"
				case "approval_id":
					if resource == "exits" {
						lookup = "supply-failure-approvals"
						f.VisibleWhen = &fieldVisibility{Field: "mode", Values: []string{"supply_failure"}}
					}
				case "contract_attachment_id":
					lookup = "contracts"
				case "target_operator_id":
					lookup = "branding-brands"
				case "asset_network_version_id":
					lookup = "payment-assets"
				case "package_id", "plan_id":
					lookup = "plans"
				case "scope_id":
					lookup = "scope-subjects"
				case "customer_id":
					lookup = "customers"
				case "order_id":
					lookup = "orders"
				case "receipt_id", "lines.0.receipt_id", "proof.receipt_id":
					lookup = "receipts"
				case "buyer_id", "channel_id":
					lookup = "buyers"
				case "supplier_id":
					lookup = "suppliers"
				case "quote_id":
					lookup = "purchase-quotes"
				case "campaign_id", "merchant_campaign_id", "kol_campaign_id":
					lookup = "campaigns"
				case "merchant_id":
					lookup = "merchants"
					if resource == "procurement" {
						lookup = "supplier-methods"
					}
				case "id":
					if resource == "payment-methods" {
						lookup = "merchants"
					} else if resource == "providers" {
						lookup = "credentials"
					}
				}
				if lookup != "" && f.Kind != "hidden" {
					f.LookupURL = scope + "/references/" + lookup
					if resource == "procurement" && lookup == "receipts" {
						f.LookupURL += "?whole=true"
					}
					f.Label = strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(f.Label, " ID", ""), "ID", ""), "编号", "")
				}
				if resource == "payment-methods" && strings.HasPrefix(f.Name, "credentials.") {
					name := strings.TrimPrefix(f.Name, "credentials.")
					for _, material := range paymentMaterials {
						if material.Key == name {
							f.Label = material.Label
							f.VisibleWhen = &fieldVisibility{Field: "driver", Values: material.Drivers}
							break
						}
					}
				}
			}
		}
		p[key] = ops
	}
}

func operationPermission(resource, action string) string {
	switch resource {
	case "operators":
		return "organization.create"
	case "staff":
		return "staff.manage"
	// Appearance targets and row actions are filtered using the exact target
	// authority; ownership of a child workspace is deliberately not required.
	case "branding", "channel-themes", "domains":
		return "organization.read"
	}
	return permissionFor(resource)
}
func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

type fieldVisibility struct {
	Not            bool     `json:"not,omitempty"`
	FallbackFields []string `json:"fallback_fields,omitempty"`
	Field          string   `json:"field"`
	Values         []string `json:"values"`
}
type paymentMaterial struct {
	Key, Label string
	Drivers    []string
}

var paymentMaterials = []paymentMaterial{
	{"currency", "收款币种（大写，例如USD/CNY）", []string{"stripe", "manual"}}, {"recipient_reference", "实际收款银行账号/凭证", []string{"manual"}}, {"instructions", "对付款人展示的付款说明", []string{"manual"}},
	{"secret_key", "Stripe服务端密钥", []string{"stripe"}}, {"publishable_key", "Stripe可发布密钥", []string{"stripe"}}, {"webhook_secret", "Stripe回调签名密钥", []string{"stripe"}},
	{"app_id", "商户应用App ID", []string{"alipay", "wechat"}}, {"app_private_key", "支付宝应用RSA2私钥PEM", []string{"alipay"}}, {"alipay_public_key", "支付宝验签公钥PEM", []string{"alipay"}}, {"seller_id", "支付宝签约卖家ID", []string{"alipay"}},
	{"mch_id", "微信支付商户号", []string{"wechat"}}, {"merchant_api_private_key", "微信商户API私钥PEM", []string{"wechat"}}, {"cert_serial", "微信商户证书序列号", []string{"wechat"}}, {"api_v3_key", "微信API v3加密密钥", []string{"wechat"}}, {"wechat_public_key", "微信平台验签公钥PEM", []string{"wechat"}}, {"public_key_id", "微信平台公钥ID", []string{"wechat"}},
	{"api_key", "NOWPayments API密钥", []string{"nowpayments"}}, {"ipn_secret", "NOWPayments IPN签名密钥", []string{"nowpayments"}}, {"pay_currency", "NOWPayments收款资产代码", []string{"nowpayments"}},
	{"kvoucher_api_key", "KVoucher API Key", []string{"kvoucher"}}, {"kvoucher_api_secret", "KVoucher API Secret", []string{"kvoucher"}}, {"refund_channels", "线下退款方式（bank,alipay,wechat,tng,duitnow，逗号分隔）", []string{"kvoucher"}},
	{"pay_network", "NOWPayments准确收款网络（须已核实）", []string{"nowpayments"}}, {"pay_token_contract", "NOWPayments该网络代币合约", []string{"nowpayments"}}, {"pay_asset_verification_reference", "NOWPayments资产网络合约核实证据", []string{"nowpayments"}},
	{"rpc_url", "EVM只读节点RPC URL", []string{"evm_wallet"}}, {"chain_id", "EVM Chain ID", []string{"evm_wallet"}}, {"token_contract", "代币合约地址", []string{"evm_wallet"}}, {"token_decimals", "代币小数位数", []string{"evm_wallet"}}, {"wallet_address", "本商户收款钱包地址", []string{"evm_wallet"}}, {"asset", "收款资产符号", []string{"evm_wallet"}}, {"min_confirmations", "最少链上确认数", []string{"evm_wallet"}},
}
var _ fmt.Stringer

// These sources load their complete authorized scope. Filtering them here is
// equivalent to a database predicate before pagination. Bounded or paged sources
// must declare their own capabilities only after applying predicates in SQL.
func prepareResourceFilters(c *gin.Context, p gin.H, resource string) {
	complete := c.Param("operatorId") != "" && contains([]string{"plans", "payment-methods", "operators", "marketing", "commissions", "business-status", "staff", "catalog", "providers", "routes", "pricing", "coverage", "mail", "branding", "channel-themes", "domains", "risk", "alerts", "overview"}, resource)
	if _, exists := p["search_supported"]; !exists {
		p["search_supported"] = complete
	}
	if _, exists := p["status_supported"]; !exists {
		p["status_supported"] = complete
	}
	if !complete {
		return
	}
	rows, ok := p["items"].([]any)
	if !ok {
		return
	}
	query, status := strings.ToLower(strings.TrimSpace(c.Query("q"))), strings.TrimSpace(c.Query("status"))
	if query == "" && status == "" {
		return
	}
	filtered := make([]any, 0, len(rows))
	for _, raw := range rows {
		row, ok := raw.(gin.H)
		if !ok {
			continue
		}
		if status != "" && fmt.Sprint(row["status"]) != status {
			continue
		}
		encoded, err := json.Marshal(row)
		if query != "" && (err != nil || !strings.Contains(strings.ToLower(string(encoded)), query)) {
			continue
		}
		filtered = append(filtered, raw)
	}
	p["items"] = filtered
}
