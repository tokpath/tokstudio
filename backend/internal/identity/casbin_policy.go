package identity

type policyRule struct {
	role   string
	object string
	action string
}

func p(role, object, action string) policyRule {
	return policyRule{role: role, object: object, action: action}
}

func grant(object, action string, roles ...string) []policyRule {
	out := make([]policyRule, 0, len(roles))
	for _, role := range roles {
		out = append(out, p(role, object, action))
	}
	return out
}

func grantMany(action string, roles []string, objects ...string) []policyRule {
	out := make([]policyRule, 0, len(roles)*len(objects))
	for _, object := range objects {
		out = append(out, grant(object, action, roles...)...)
	}
	return out
}

func casbinPolicy() []policyRule {
	// 平台超管：管理台和渠道台全部功能。其它角色必须显式授权。
	rules := []policyRule{
		p("platform_admin", "/admin/*", "*"),
		p("platform_admin", "/channel/*", "*"),
		p("platform_admin", "/v1/topups/:id/refund", "POST"),
	}
	rules = append(rules, authenticatedRules()...)
	rules = append(rules, financeRules()...)
	rules = append(rules, opsRules()...)
	rules = append(rules, techRules()...)
	rules = append(rules, channelRules()...)
	rules = append(rules, auditRules()...)
	return rules
}

func authenticatedRules() []policyRule {
	return []policyRule{
		p(RoleAuthenticated, "/v1/me", "*"),
		p(RoleAuthenticated, "/v1/me/*", "*"),
		p(RoleAuthenticated, "/v1/partner/*", "GET"),
		p(RoleAuthenticated, "/v1/balance", "GET"),
		p(RoleAuthenticated, "/v1/usage", "GET"),
		p(RoleAuthenticated, "/v1/topups", "POST"),
		p(RoleAuthenticated, "/v1/topups/:id", "GET"),
		p(RoleAuthenticated, "/v1/topups/redeem", "POST"),
		p(RoleAuthenticated, "/v1/payments/checkout", "GET"),
		p(RoleAuthenticated, "/v1/payments/quote", "GET"),
		p(RoleAuthenticated, "/v1/payments/orders", "POST"),
		p(RoleAuthenticated, "/v1/payments/orders/:id", "GET"),
		p(RoleAuthenticated, "/v1/payments/orders/:id/sync", "POST"),
		p(RoleAuthenticated, "/v1/videos", "POST"),
		p(RoleAuthenticated, "/v1/videos/:id", "GET"),
		p(RoleAuthenticated, "/v1/videos/:id/*", "*"),
		p(RoleAuthenticated, "/v1/images/*", "*"),
	}
}

func financeRules() []policyRule {
	roles := []string{"finance_admin"}
	out := []policyRule{}
	out = append(out, grantMany("GET", roles,
		"/admin/me",
		"/admin/me/2fa",
		"/admin/ledger",
		"/admin/usage",
		"/admin/usage/pending",
		"/admin/usage/pending/:id",
		"/admin/billing/report",
		"/admin/billing/export",
		"/admin/margin",
		"/admin/price-books",
		"/admin/payments",
		"/admin/settlements",
		"/admin/commissions",
		"/admin/commission-policy",
		"/admin/eligibility-rules",
		"/admin/supplier-entries",
		"/admin/channels/:id/pnl",
		"/admin/channel-quotas/:channel_id",
		"/admin/channel-quotas/:channel_id/issue-rule",
		"/admin/channels",
		"/admin/channels/:id",
		"/admin/channels/:id/payments",
		"/admin/payment-adapters",
		"/admin/metrics",
		"/admin/metrics/series",
		"/admin/metrics/daily",
		"/admin/ops/dashboard",
		"/channel/payments/orders",
		"/channel/quota",
		"/channel/allocations",
		"/channel/commissions",
		"/channel/usage",
		"/channel/reconciliation",
		"/channel/settlements",
		"/channel/eligibility-rules",
		"/channel/commission-policy",
		"/channel/supplier-entries",
		"/channel/pnl",
	)...)
	out = append(out, grantMany("POST", roles,
		"/admin/me/2fa/setup",
		"/admin/me/2fa/enable",
		"/admin/me/2fa/disable",
		"/admin/topups/:id/confirm",
		"/admin/refunds",
		"/admin/payments/:id/confirm",
		"/admin/payments/:id/refund",
		"/admin/entitlements/bonus",
		"/admin/commissions/recalc",
		"/admin/commissions/unfreeze",
		"/admin/commissions/settle",
		"/admin/settlements/:id/payout",
		"/admin/channel-quotas/grant",
		"/admin/supplier-entries",
		"/admin/supplier-entries/:id/reverse",
		"/channel/supplier-entries",
		"/channel/supplier-entries/:id/reverse",
		"/channel/reconciliation/flag",
		"/admin/price-books",
		"/admin/usage/replay",
		"/admin/usage/pending/resolve",
		"/admin/margin/corrections",
		"/admin/ops/drills/payment",
		"/channel/payments/orders/:id/confirm",
		"/channel/payments/orders/:id/refund",
		"/v1/topups/:id/refund",
	)...)
	out = append(out, grantMany("PATCH", roles,
		"/admin/commission-policy",
		"/admin/eligibility-rules",
		"/admin/channel-quotas/:channel_id/issue-rule",
	)...)
	return out
}

func opsRules() []policyRule {
	roles := []string{"ops_admin"}
	out := []policyRule{}
	out = append(out, grantMany("GET", roles,
		"/admin/me",
		"/admin/me/2fa",
		"/admin/brands",
		"/admin/brands/:id",
		"/admin/channels",
		"/admin/channels/:id",
		"/admin/channels/:id/models",
		"/admin/api-keys",
		"/admin/providers",
		"/admin/providers/:id",
		"/admin/providers/:id/accounts",
		"/admin/models",
		"/admin/models/*",
		"/admin/routes",
		"/admin/plans",
		"/admin/payments",
		"/admin/usage",
		"/admin/usage/pending",
		"/admin/usage/pending/:id",
		"/admin/billing/report",
		"/admin/billing/export",
		"/admin/margin",
		"/admin/price-books",
		"/admin/settlements",
		"/admin/commissions",
		"/admin/commission-policy",
		"/admin/eligibility-rules",
		"/admin/supplier-entries",
		"/admin/channels/:id/pnl",
		"/admin/promotion-codes",
		"/admin/media",
		"/admin/metrics",
		"/admin/metrics/series",
		"/admin/metrics/daily",
		"/admin/ops/dashboard",
		"/admin/ops/alerts",
		"/admin/ops/thresholds",
		"/admin/ops/runbooks",
		"/admin/ops/canary",
		"/admin/channels/:id/payments",
		"/admin/payment-adapters",
		"/channel/plans",
		"/channel/usage",
		"/channel/reconciliation",
		"/channel/pnl",
	)...)
	out = append(out, grantMany("POST", roles,
		"/admin/me/2fa/setup",
		"/admin/me/2fa/enable",
		"/admin/me/2fa/disable",
		"/admin/models",
		"/admin/models/review",
		"/admin/models/publish",
		"/admin/models/deprecate",
		"/admin/models/attach",
		"/admin/providers/:id/sync",
		"/admin/plans",
		"/admin/plans/:id/review",
		"/admin/entitlements/bonus",
		"/admin/price-books",
		"/admin/usage/replay",
		"/admin/usage/pending/resolve",
		"/admin/margin/corrections",
		"/admin/ops/alerts/evaluate",
		"/admin/ops/canary",
		"/admin/channels/:id/payments/disable",
	)...)
	out = append(out, grantMany("PATCH", roles,
		"/admin/channels/:id/models",
		"/admin/models/*",
		"/admin/plans/:id",
		"/admin/ops/thresholds",
	)...)
	return out
}

func techRules() []policyRule {
	roles := []string{"tech_admin"}
	out := []policyRule{}
	out = append(out, grantMany("GET", roles,
		"/admin/me",
		"/admin/me/2fa",
		"/admin/outbox/stats",
		"/admin/brands",
		"/admin/brands/:id",
		"/admin/api-keys",
		"/admin/providers",
		"/admin/providers/:id",
		"/admin/providers/:id/accounts",
		"/admin/models",
		"/admin/models/*",
		"/admin/routes",
		"/admin/media",
		"/admin/metrics",
		"/admin/metrics/series",
		"/admin/metrics/daily",
		"/admin/ops/dashboard",
		"/admin/ops/alerts",
		"/admin/ops/thresholds",
		"/admin/ops/runbooks",
		"/admin/ops/canary",
		"/admin/payment-adapters",
	)...)
	out = append(out, grantMany("POST", roles,
		"/admin/me/2fa/setup",
		"/admin/me/2fa/enable",
		"/admin/me/2fa/disable",
		"/admin/brands/:id/tls/issue",
		"/admin/api-keys/:id/disable",
		"/admin/providers",
		"/admin/providers/:id/credentials",
		"/admin/providers/:id/accounts",
		"/admin/providers/:id/health-check",
		"/admin/providers/:id/sync",
		"/admin/routes",
		"/admin/models/attach",
		"/admin/ops/alerts/evaluate",
		"/admin/ops/backup-drill",
		"/admin/ops/canary",
		"/admin/ops/circuit/:id",
		"/admin/ops/drills/payment",
		"/admin/ops/drills/media",
		"/admin/ops/drills/tls",
	)...)
	out = append(out, grantMany("PATCH", roles,
		"/admin/providers/:id",
		"/admin/providers/:id/accounts/:aid",
		"/admin/routes/:id",
	)...)
	return out
}

func channelRules() []policyRule {
	roles := []string{"channel_admin"}
	out := []policyRule{
		p("channel_admin", "/channel/*", "*"),
		p("channel_admin", "/admin/me", "GET"),
	}
	out = append(out, grantMany("GET", roles,
		"/admin/channels",
		"/admin/channels/:id",
		"/admin/channels/:id/models",
		"/admin/plans",
		"/admin/acquisition-roles",
		"/admin/acquisition-roles/:id",
		"/admin/promotion-codes",
		"/admin/channel-quotas/:channel_id",
		"/admin/channel-quotas/:channel_id/issue-rule",
	)...)
	out = append(out, grantMany("POST", roles,
		"/admin/plans",
		"/admin/acquisition-roles",
		"/admin/promotion-codes",
		"/admin/channels",
	)...)
	out = append(out, grant("/admin/acquisition-roles/:id", "PATCH", roles...)...)
	return out
}

func auditRules() []policyRule {
	roles := []string{"audit_readonly"}
	return grantMany("GET", roles,
		"/admin/me",
		"/admin/audit-logs",
		"/admin/api-keys",
		"/admin/providers",
		"/admin/providers/:id",
		"/admin/providers/:id/accounts",
		"/admin/models",
		"/admin/models/*",
		"/admin/routes",
		"/admin/plans",
		"/admin/payments",
		"/admin/ledger",
		"/admin/usage",
		"/admin/usage/pending",
		"/admin/usage/pending/:id",
		"/admin/billing/report",
		"/admin/billing/export",
		"/admin/margin",
		"/admin/price-books",
		"/admin/settlements",
		"/admin/commissions",
		"/admin/commission-policy",
		"/admin/eligibility-rules",
		"/admin/supplier-entries",
		"/admin/channels/:id/pnl",
		"/admin/promotion-codes",
		"/admin/media",
		"/admin/metrics",
		"/admin/metrics/series",
		"/admin/metrics/daily",
		"/admin/ops/dashboard",
		"/admin/ops/alerts",
		"/admin/ops/thresholds",
		"/admin/ops/runbooks",
		"/admin/channels",
		"/admin/channels/:id",
		"/admin/channels/:id/payments",
	)
}
