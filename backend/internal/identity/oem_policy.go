package identity

// OEM employees have explicit scoped business permissions, never global roles.
func oemEmployeeRules() []policyRule {
	all := []string{"oem_ops", "oem_finance", "oem_audit"}
	opsAudit := []string{"oem_ops", "oem_audit"}
	out := grantMany("GET", all,
		"/channel/models", "/channel/me", "/channel/metrics", "/channel/alerts", "/channel/quota",
		"/channel/allocations", "/channel/usage", "/channel/reconciliation",
		"/channel/pnl", "/channel/commissions", "/channel/settlements", "/channel/settlements/manage",
		"/channel/commission-context", "/channel/settlements/:id", "/channel/commissions/settlement-preview",
		"/channel/eligibility-rules", "/channel/commission-policy", "/channel/supplier-entries",
		"/channel/payments/overview", "/channel/payments/orders", "/channel/payments/recipients", "/channel/brand", "/channel/me/2fa",
		"/admin/channels", "/admin/channels/:id", "/admin/channels/:id/models",
		"/admin/channel-quotas/:channel_id", "/admin/channel-quotas/:channel_id/issue-rule",
	)
	out = append(out, grantMany("GET", opsAudit,
		"/channel/users", "/channel/subchannels/:id/users", "/channel/models", "/channel/media",
		"/channel/api-keys", "/channel/attribution", "/channel/promotion-codes", "/channel/plans",
		"/admin/plans", "/admin/plans/eligible-channels", "/admin/acquisition-roles", "/admin/acquisition-roles/:id", "/admin/promotion-codes",
	)...)
	out = append(out, grantMany("POST", []string{"oem_ops"},
		"/channel/users/:id/ban", "/channel/users/:id/unban", "/channel/api-keys/:id/disable",
		"/channel/promotion-codes", "/channel/plans", "/admin/plans", "/admin/plans/:id/review",
		"/admin/channels", "/admin/acquisition-roles", "/admin/promotion-codes", "/channel/brand/assets",
	)...)
	out = append(out, grantMany("PATCH", []string{"oem_ops"},
		"/channel/models", "/channel/brand", "/admin/plans/:id", "/admin/channels/:id",
		"/admin/channels/:id/models", "/admin/acquisition-roles/:id",
	)...)
	out = append(out, grantMany("GET", []string{"oem_finance"}, "/channel/payments/settings", "/channel/payments/instances")...)
	out = append(out, grantMany("GET", []string{"oem_finance"}, "/channel/commission-operations/:operation_id", "/channel/commission-recovery-operations/:operation_id")...)
	out = append(out,grantMany("GET",[]string{"oem_finance","oem_audit"},"/channel/commission-recoveries")...)
	out = append(out, grantMany("PATCH", []string{"oem_finance"}, "/channel/payments/settings", "/channel/payments/instances/:id")...)
	out = append(out, grantMany("POST", []string{"oem_finance"}, "/channel/payments/instances", "/channel/payments/instances/:id/test", "/channel/payments/instances/:id/go-live",
		"/channel/commissions/settle", "/channel/commissions/unfreeze", "/channel/settlements/:id/payout", "/channel/payments/offline", "/channel/payments/orders/:id/confirm", "/channel/payments/orders/:id/refund",
		"/channel/reconciliation/flag", "/channel/quotas/grant", "/channel/supplier-entries", "/channel/supplier-entries/:id/reverse",
		"/channel/commission-recoveries/:id/receipts",
	)...)
	out = append(out, grantMany("PATCH", []string{"oem_finance"},
		"/channel/commission-policy", "/channel/eligibility-rules", "/channel/model-prices",
	)...)
	out = append(out, grantMany("GET", []string{"oem_audit"}, "/channel/audit-logs")...)
	// Personal security settings are available to read-only employees too.
	out = append(out, grantMany("POST", all, "/channel/me/2fa/setup", "/channel/me/2fa/enable", "/channel/me/2fa/disable")...)
	return out
}
