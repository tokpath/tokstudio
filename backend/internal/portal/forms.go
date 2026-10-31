package portal

import "strings"

// Ordinary business commands expose explicit fields. JSON editors are reserved
// for verified technical model capabilities and component price rules.
func businessFields(o operation) []field {
	if len(o.Fields) != 1 || o.Fields[0].Name != "configuration" {
		return o.Fields
	}
	parts := strings.Split(strings.SplitN(o.Path, "?", 2)[0], "/")
	resource := ""
	for i, p := range parts {
		if p == "operators" && len(parts) > i+2 {
			resource = parts[i+2]
		}
		if p == "me" && len(parts) > i+1 {
			resource = parts[i+1]
		}
	}
	version := defaultField("expected_version", "当前版本", "hidden", 0)
	switch resource {
	case "plans":
		period := selectField("period", "有效期", "lifetime|不限期", "month|一个日历月", "quarter|三个日历月", "year|十二个日历月")
		period.Help = "套餐期限从每次成功交付时开始，按经营时区计算一个、三个或十二个日历月，到期后剩余额度不可用；不限期额度不过期。是否续费由客户授权决定，与 API Key 的自然周期预算分别管理。"
		period.Default = "lifetime"
		renewal := defaultField("renewal_enabled", "允许到期续费", "boolean", false)
		renewal.VisibleWhen = &fieldVisibility{Field: "period", Values: []string{"month", "quarter", "year"}}
		renewal.Help = "仅允许客户自行授权续费，不会替客户开启。停止新购与停止续费分别控制，已购权益保持原条款。"
		return []field{optional("package_id", "套餐", "hidden"), f("name", "套餐名称", "text"), f("price_money", "售价 结算币种", "decimal"), f("paid_atg", "购买额度 ATG", "decimal"), defaultField("gift_atg", "赠送额度 ATG", "decimal", "0"), period, packageTermsField(), defaultField("new_purchase_enabled", "允许新购", "boolean", true), renewal, version}
	case "operators":
		return []field{selectField("kind", "合作类型", "oem|独立OEM品牌", "channel|同品牌渠道"), f("name", "经营方名称", "text"), f("legal_name", "实际经营主体", "text"), cooperationBrandNameField(), optional("supply_terms", "合作约定说明（可选）", "textarea"), optional("contract_attachment_id", "已上传合作合同（可选）", "text")}
	case "commissions":
		confirmation := defaultField("confirmation_days", "消费确认等待天数", "integer", 30)
		confirmation.Help = "按整天填写，默认 30 天。等待期满且相关消费没有未结争议后，才计入资格，对应佣金才能使用或提现。已经开始等待的记录仍按原来的到期时间，不会因为后来改政策而延长。"
		reward := defaultField("reward_validity_days", "注册奖励有效天数", "integer", 30)
		reward.Help = "注册赠送额度从注册成功起按整天计算，默认 30 天。到期后尚未使用的注册奖励不再可用。"
		minimum := f("minimum_money", "佣金最低提现 结算币种", "decimal")
		minimum.Help = "单笔佣金提现不能低于这个额度，且不能小于 1 结算币种。提现手续费由申请人承担，不在这里另设。审批固定为单人。"
		return []field{f("threshold_money", "分佣资格门槛 结算币种", "decimal"), f("direct_percent", "直接邀请佣金 %", "decimal"), f("indirect_percent", "间接邀请佣金 %", "decimal"), confirmation, f("invitee_atg", "受邀人注册赠送 ATG", "decimal"), f("inviter_atg", "邀请人注册赠送 ATG", "decimal"), reward, defaultField("kol_cross_activity_limit", "同一 KOL 跨活动每人限次", "integer", 0), minimum, version}
	case "activities":
		return campaignFields(true, version)
	case "marketing":
		return campaignFields(false, version)
	case "business-status":
		return []field{selectField("feature", "营业项目", "registration|注册", "new_purchase|新购", "renewal|续费", "model_usage|模型调用", "activity|营销活动", "settlement|结算"), f("enabled", "通过真实依赖检查后开放", "boolean"), version}
	case "branding", "channel-themes":
		return []field{version, optional("values.primary_color", "主色（留空继承，#RRGGBB）", "text"), optional("values.background_color", "背景色（留空继承）", "text"), optional("values.text_color", "文字色（留空继承）", "text"), optional("values.logo_url", "已上传Logo地址（留空继承）", "text"), optional("values.support_url", "客户支持URL（留空继承）", "text")}
	case "notifications":
		return []field{f("recipient_id", "本经营范围收件人编号", "text"), selectField("category", "通知类别", "mandatory|必要业务通知", "usage|用量提醒", "marketing|已获授权营销"), notificationSeverityField(), f("title", "标题", "text"), f("body", "通知正文", "textarea"), optional("reference", "关联事项（可选）", "text")}
	case "exports":
		return []field{selectField("kind", "导出内容", "financial|经营报表", "orders|客户订单", "payments|收款记录", "requests|调用明细", "procurement|采购与供货", "ledger|账务明细"), f("from", "开始时间", "datetime"), f("to", "结束时间", "datetime")}
	case "alerts":
		return []field{optional("name", "规则名称（留空按指标命名）", "text"), optional("id", "规则编号", "hidden"), selectField("metric", "真实指标", "sales_space_atg|剩余销售空间ATG", "procurement_available_atg|采购可用余额 ATG", "oversell_used_atg|已用超卖额度 ATG", "pending_usage|在途及待结算请求数", "unverified_costs|待核实上游成本数"), f("warning", "预警阈值", "decimal"), f("critical", "严重阈值", "decimal"), selectField("direction", "触发方向", "below|达到或低于阈值", "above|达到或高于阈值"), f("recipients", "通知负责人", "csv"), optional("escalation_recipients", "升级负责人", "csv"), defaultField("reminder_seconds", "持续风险提醒间隔秒数", "integer", 3600), defaultField("escalate_seconds", "升级等待秒数", "integer", 86400), defaultField("enabled", "启用规则", "boolean", true), version}
	case "payments":
		return manualReceiptFields()
	case "payment-methods":
		fields := []field{optional("id", "现有商户配置ID（首次留空）", "text"), selectField("driver", "真实通道协议", "stripe|Stripe", "alipay|支付宝", "wechat|微信支付", "nowpayments|NOWPayments", "evm_wallet|自有EVM钱包只读验证", "manual|线下人工核实", "kvoucher|KVoucher"), defaultField("enabled", "材料齐全后启用", "boolean", false), version}
		asset := optional("asset_network_version_id", "平台已发布的准确资产与网络", "text")
		asset.VisibleWhen = &fieldVisibility{Field: "driver", Values: []string{"evm_wallet", "nowpayments"}}
		fields = append(fields, asset)
		for _, key := range []string{"currency", "recipient_reference", "instructions", "secret_key", "publishable_key", "webhook_secret", "app_id", "mch_id", "merchant_api_private_key", "cert_serial", "api_v3_key", "wechat_public_key", "public_key_id", "app_private_key", "alipay_public_key", "seller_id", "api_key", "kvoucher_api_key", "kvoucher_api_secret", "refund_channels", "ipn_secret", "rpc_url", "wallet_address"} {
			v := optional("credentials."+key, "通道材料 · "+key, "text")
			if strings.Contains(key, "private_key") || strings.Contains(key, "public_key") {
				v.Kind = "textarea"
			} else if strings.Contains(key, "secret") || key == "api_key" || key == "api_v3_key" {
				v.Kind = "password"
			}
			fields = append(fields, v)
		}
		return fields
	case "invoices":
		return []field{optional("issuer_id", "原始开票经营方", "hidden"), f("lines.0.receipt_id", "原始收款编号", "text"), f("lines.0.amount_money", "本次申请 结算币种", "decimal"), f("details.title", "发票抬头", "text"), f("details.tax_id", "纳税识别号", "text"), f("details.email", "接收邮箱", "email"), optional("details.address", "发票地址", "text")}
	case "refunds":
		switch o.Name {
		case "package_exception":
			return []field{defaultField("action", "办理类型", "hidden", "package_exception"), f("customer_id", "客户编号", "text"), f("order_id", "原成交订单", "text"), selectField("reason", "真实异常原因", "transaction_error|交易错误", "unable_to_fulfill|无法履行服务"), f("evidence", "事实证据", "textarea")}
		case "unallocated":
			return []field{defaultField("action", "办理类型", "hidden", "unallocated"), f("receipt_id", "原收款编号", "text"), f("amount_money", "退回 结算币种", "decimal"), f("reason", "退款原因", "text"), f("evidence", "核实依据", "textarea")}
		case "policy":
			return []field{defaultField("action", "办理类型", "hidden", "policy"), optional("second_review_above", "需独立复核的结算币种阈值（留空关闭）", "decimal"), version}
		case "verify_manual":
			return []field{optional("reference", "退款流水或收据号（可选）", "text"), f("currency", "实际付款币种", "text"), f("amount", "实际币种金额", "decimal"), f("actual_money", "已核实本次清偿金额（结算币种）", "decimal"), optional("fee_money", "付款方已核实清算手续费（可稍后核实）", "decimal"), f("occurred_at", "实际付款时间", "datetime"), f("evidence", "本笔付款与清偿金额的核实说明", "textarea")}
		case "execute_api":
			fields := []field{f("money.currency", "原付款币种", "text"), f("money.amount", "本次实际执行币种金额", "decimal")}
			for _, v := range executionValuationFields("money.currency") {
				v.Name = strings.Replace(v.Name, "valuation.", "refund_valuation.", 1)
				fields = append(fields, v)
			}
			return fields
		}
	}
	return o.Fields
}

func campaignFields(kol bool, version field) []field {
	fields := []field{optional("campaign_id", "活动", "hidden"), optional("name", "活动名称（留空使用套餐名称）", "text"), f("package_id", "适用套餐", "text")}
	if kol {
		fields = append(fields, f("concession_rate", "本人佣金让利比例", "decimal"))
	} else {
		price := optional("final_price_money", "活动售价 结算币种（留空使用套餐售价）", "decimal")
		price.Help = "可以只降价、只加赠或同时使用。活动售价不得高于下单时的套餐售价；不会修改套餐基础额度。"
		fields = append(fields, price, defaultField("gift_atg", "额外赠送 ATG", "decimal", "0"))
	}
	audience := selectField("eligibility", "适用客户", "any|所有符合条件的客户", "first_order|首次购买客户", "existing|已有购买记录客户")
	audience.Default = "any"
	total := defaultField("total_use_limit", "活动总名额", "integer", 0)
	total.Help = "0 表示不单独限制总名额。待付款订单先占用名额，成交后计入已用，关闭订单后释放。每位客户限次和实际履约能力仍同时生效。"
	return append(fields, audience, f("starts_at", "开始时间", "datetime"), f("ends_at", "结束时间", "datetime"), defaultField("per_customer_limit", "每位客户限次", "integer", 1), total, defaultField("enabled", "启用活动", "boolean", true), version)
}
func supplyExitPolicyField() field {
	v := selectField("exit_policy", "主动退出时的余额处理", "non_refundable|剩余采购余额不退", "remaining_paid_proportional|清算存量责任后，剩余购买额度按原实付比例退；赠送不退")
	v.Default = "non_refundable"
	v.Help = "报价和订单会固定本次退出规则。选择可退不自动退出或打款；仍须清算存量责任，再办理退出及实际退款。"
	return v
}

func packageTermsField() field {
	v := optional("terms_summary", "对客户公示的服务约定（可选）", "textarea")
	v.Help = "留空采用默认规则：通用额度按调用时报价使用，按成交期限有效；到期余量不结转，套餐不支持一般性退款，可停止未来续费；交易错误、未正确交付或无法履约走异常流程。新条款会固定到本次报价，变更须按续费规则重新确认。"
	return v
}

func cooperationBrandNameField() field {
	v := f("brand_name", "OEM品牌名称", "text")
	v.VisibleWhen = &fieldVisibility{Field: "kind", Values: []string{"oem"}}
	return v
}

func policySecondsFromDays(days, min, max int64) (int64, bool) {
	if days < min || days > max {
		return 0, false
	}
	seconds := days * 86400
	if seconds/86400 != days {
		return 0, false
	}
	return seconds, true
}

// policyDays shows a stored duration as whole days. A leftover partial day is
// rounded up so the form never asks the operator to type seconds.
func policyDays(seconds int64) int64 {
	if seconds <= 0 {
		return 0
	}
	days := seconds / 86400
	if seconds%86400 != 0 {
		days++
	}
	return days
}

func notificationSeverityField() field {
	v := selectField("severity", "级别", "info|一般", "warning|预警", "critical|紧急")
	v.Default = "info"
	return v
}
