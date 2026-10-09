package identity

func usageWorkflowRules() []policyRule {
	out := grantMany("GET", []string{"finance_admin", "ops_admin", "audit_readonly"}, "/admin/usage/summary", "/admin/requests", "/admin/requests/:id")
	out = append(out, grantMany("GET", []string{"channel_admin", "oem_ops", "oem_finance", "oem_audit", "finance_admin", "ops_admin", "audit_readonly"}, "/channel/usage/summary", "/channel/requests", "/channel/requests/:id")...)
	out = append(out, grantMany("POST", []string{"finance_admin", "ops_admin"}, "/admin/requests/:id/reconcile")...)
	out = append(out, grantMany("GET", []string{"channel_admin", "oem_ops", "oem_finance", "oem_audit", "finance_admin", "ops_admin", "audit_readonly"}, "/channel/usage/pending", "/channel/usage/pending/:id")...)
	return out
}
