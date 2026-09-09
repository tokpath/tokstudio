package identity

import (
	"strings"
	"testing"
)

func TestCasbinAllowMatrix(t *testing.T) {
	svc := New(nil)
	cases := []struct {
		role   string
		method string
		path   string
		want   bool
	}{
		{"end_user", "GET", "/v1/me", true},
		{"end_user", "GET", "/v1/me/reconciliation", true},
		{"end_user", "POST", "/v1/me/reconciliation/flag", true},
		{"end_user", "GET", "/channel/reconciliation", false},
		{"end_user", "GET", "/v1/me/api-keys", true},
		{"end_user", "GET", "/v1/partner/me", true},
		{"end_user", "GET", "/admin/audit-logs", false},
		{"end_user", "POST", "/admin/providers", false},
		{"end_user", "POST", "/v1/topups/abc/refund", false},
		{"platform_admin", "GET", "/admin/audit-logs", true},
		{"platform_admin", "POST", "/admin/audit-probes", true},
		{"platform_admin", "POST", "/admin/providers", true},
		{"platform_admin", "GET", "/channel/me", true},
		{"platform_admin", "POST", "/v1/topups/abc/refund", true},
		{"finance_admin", "POST", "/admin/refunds", true},
		{"finance_admin", "GET", "/admin/usage/pending", true},
		{"finance_admin", "POST", "/admin/usage/pending/resolve", true},
		{"finance_admin", "GET", "/channel/reconciliation", true},
		{"finance_admin", "POST", "/channel/reconciliation/flag", true},
		{"ops_admin", "GET", "/admin/usage/pending", true},
		{"ops_admin", "POST", "/admin/usage/pending/resolve", true},
		{"ops_admin", "GET", "/channel/reconciliation", true},
		{"ops_admin", "POST", "/channel/reconciliation/flag", false},
		{"audit_readonly", "GET", "/admin/usage/pending", true},
		{"audit_readonly", "POST", "/admin/usage/pending/resolve", false},
		{"finance_admin", "GET", "/admin/ledger", true},
		{"finance_admin", "GET", "/admin/channels", true},
		{"finance_admin", "GET", "/admin/channels/chn_x", true},
		{"finance_admin", "PATCH", "/admin/channels/chn_x", false},
		{"finance_admin", "POST", "/admin/providers", false},
		{"finance_admin", "GET", "/admin/audit-logs", false},
		{"finance_admin", "POST", "/v1/topups/abc/refund", true},
		{"finance_admin", "GET", "/admin/eligibility-rules", true},
		{"finance_admin", "PATCH", "/admin/eligibility-rules", true},
		{"finance_admin", "GET", "/admin/supplier-entries", true},
		{"finance_admin", "POST", "/admin/supplier-entries", true},
		{"finance_admin", "GET", "/admin/channels/chn_x/pnl", true},
		{"ops_admin", "GET", "/admin/eligibility-rules", true},
		{"ops_admin", "PATCH", "/admin/eligibility-rules", false},
		{"ops_admin", "POST", "/admin/models/publish", true},
		{"ops_admin", "GET", "/admin/channels", true},
		{"ops_admin", "POST", "/admin/refunds", false},
		{"ops_admin", "GET", "/channel/plans", true},
		{"tech_admin", "POST", "/admin/providers", true},
		{"tech_admin", "GET", "/admin/providers/prd_echo_primary", true},
		{"ops_admin", "GET", "/admin/providers/prd_echo_primary", true},
		{"audit_readonly", "GET", "/admin/providers/prd_echo_primary", true},
		{"finance_admin", "GET", "/admin/providers/prd_echo_primary", false},
		{"tech_admin", "POST", "/admin/refunds", false},
		{"tech_admin", "GET", "/admin/outbox/stats", true},
		{"channel_admin", "GET", "/channel/me", true},
		{"channel_admin", "POST", "/channel/plans", true},
		{"channel_admin", "GET", "/admin/channels", true},
		{"channel_admin", "POST", "/admin/users/u1/ban", false},
		{"channel_admin", "GET", "/admin/audit-logs", false},
		{"audit_readonly", "GET", "/admin/audit-logs", true},
		{"audit_readonly", "GET", "/admin/channels", true},
		{"audit_readonly", "GET", "/admin/billing/report", true},
		{"audit_readonly", "POST", "/admin/audit-probes", false},
		{"audit_readonly", "GET", "/admin/ledger", true},
		{"audit_readonly", "POST", "/admin/refunds", false},
		{"audit_readonly", "PATCH", "/admin/ops/thresholds", false},
	}
	for _, tc := range cases {
		p := &Principal{UserID: "usr_test", Roles: []string{tc.role}}
		got := svc.Allow(p, tc.path, tc.method)
		if got != tc.want {
			t.Errorf("%s %s %s: got %v want %v", tc.role, tc.method, tc.path, got, tc.want)
		}
	}
}

func TestCasbinDefaultDenyUnknownPath(t *testing.T) {
	svc := New(nil)
	p := &Principal{Roles: []string{"platform_admin"}}
	if svc.Allow(p, "/not-a-real-route", "GET") {
		t.Fatal("unknown path must be denied even for platform_admin")
	}
}

func TestCasbinNilPrincipalDenied(t *testing.T) {
	svc := New(nil)
	if svc.Allow(nil, "/v1/me", "GET") {
		t.Fatal("nil principal must be denied")
	}
}

func TestCasbinPolicyUsesKeyMatch(t *testing.T) {
	svc := New(nil)
	ops := &Principal{Roles: []string{"ops_admin"}}
	if !svc.Allow(ops, "/admin/models/openai/gpt-5.6", "GET") {
		t.Fatal("ops should read nested model path")
	}
	if svc.Allow(ops, "/admin/models/openai/gpt-5.6", "DELETE") {
		t.Fatal("ops must not get undeclared methods")
	}
}

func TestCasbinAuthenticatedDoesNotOpenAdmin(t *testing.T) {
	svc := New(nil)
	user := &Principal{Roles: []string{"end_user"}}
	for _, path := range []string{"/admin/me", "/admin/users", "/channel/quota"} {
		if svc.Allow(user, path, "GET") {
			t.Fatalf("end_user must not access %s via authenticated wildcard", path)
		}
	}
}

func TestCasbinPolicyHasNoEmptyRules(t *testing.T) {
	for _, rule := range casbinPolicy() {
		if strings.TrimSpace(rule.role) == "" || rule.object == "" || rule.action == "" {
			t.Fatalf("empty policy: %+v", rule)
		}
	}
}
