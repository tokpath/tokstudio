package identity

func customerRules() []policyRule {
	// Channels may bind only their existing customers; the service validates
	// membership/ownership and keeps all qualification and fee facts unchanged.
	out := grant("/channel/subchannels", "GET", "channel_admin", "oem_ops", "oem_audit")
	out = append(out, grantMany("GET", []string{"finance_admin", "ops_admin", "audit_readonly"}, "/admin/customers", "/admin/customers/:id", "/admin/customer-scopes")...)
	out = append(out, grant("/admin/professional-customers", "POST", "channel_admin")...)
	out = append(out, grant("/admin/professional-customers", "GET", "channel_admin")...)
	out = append(out, grant("/admin/channels/:id/onboarding", "GET", "channel_admin", "ops_admin", "audit_readonly", "oem_ops", "oem_audit")...)
	out = append(out, grant("/admin/professional-customers/search", "GET", "channel_admin", "ops_admin", "audit_readonly", "oem_ops", "oem_audit")...)
	out = append(out, grant("/admin/professional-customers/:id", "GET", "channel_admin", "ops_admin", "audit_readonly", "oem_ops", "oem_audit")...)
	return append(out, grantMany("GET", []string{"channel_admin", "oem_ops", "oem_finance", "oem_audit"}, "/channel/customers", "/channel/customers/:id", "/channel/customer-scopes")...)
}
