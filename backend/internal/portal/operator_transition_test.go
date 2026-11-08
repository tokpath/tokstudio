package portal

import (
	"context"
	"errors"
	"testing"

	"github.com/tokpath/tokstudio/backend/internal/organization"
)

func TestChannelTransitionCommandBoundary(t *testing.T) {
	for _, state := range []string{"preparing", "awaiting_approval", "completed"} {
		for _, path := range []string{"plans", "pricing", "marketing", "gifts", "business-status", "exits", "procurement", "procurement/quote/purchase", "procurement/order/checkout", "payments", "notifications"} {
			if channelCommandAllowed(state, path) {
				t.Errorf("%s allowed new business %s", state, path)
			}
		}
		for _, path := range []string{"invoices/invoice/register", "invoices/invoice/files", "procurement-invoices", "payments/receipt/fee", "refunds/case/review", "evidence", "exports", "notifications/read-all", "notifications/notice/read", "staff"} {
			if !channelCommandAllowed(state, path) {
				t.Errorf("%s blocked historical work %s", state, path)
			}
		}
	}
	if !channelCommandAllowed("preparing", "procurement/order/cancel") || !channelCommandAllowed("preparing", "procurement/order/query_payment") || channelCommandAllowed("completed", "procurement/order/cancel") {
		t.Fatal("outstanding preparation boundary")
	}
}

func TestPostgresCancelledTakeoverRestoresActionsWithoutReopeningBusiness(t *testing.T) {
	s := setupExitAcceptance(t)
	f := s.f
	f.login(t, "staff", "owner@example.test", "owner-password-test")
	base := "/v1/operators/" + s.oem + "/takeovers"
	status, out, _ := f.call(t, "POST", base, map[string]any{"channel_id": s.channel}, "")
	mustOK(t, status, out)
	status, out, _ = f.call(t, "POST", base+"/"+out["id"].(string)+"/cancel", map[string]any{}, "")
	mustOK(t, status, out)
	op, e := f.a.Organization.GetOperator(context.Background(), nil, s.channel)
	if e != nil {
		t.Fatal(e)
	}
	if transition, e := f.a.operatorTransition(context.Background(), op); e != nil || transition != nil {
		t.Fatal("cancelled workspace still locked", transition, e)
	}
	business, e := f.a.Organization.GetBusinessStatus(context.Background(), nil, s.channel)
	if e != nil || business.NewPurchase || business.Registration {
		t.Fatal("cancel reopened business", business, e)
	}
	status, out, _ = f.call(t, "POST", "/v1/operators/"+s.channel+"/exits", map[string]any{}, "")
	mustOK(t, status, out)
}

func assertTakeoverWorkspaceState(t *testing.T, f *testApp, channel, state string) {
	t.Helper()
	base := "/v1/operators/" + channel
	status, out, _ := f.call(t, "GET", "/v1/auth/me", nil, "")
	mustOK(t, status, out)
	found := false
	for _, v := range out["workspaces"].([]any) {
		w := v.(map[string]any)
		if w["operator"].(map[string]any)["id"] != channel {
			continue
		}
		tr := w["transition"].(map[string]any)
		if tr["state"] != state || tr["brand_name"] != "星河品牌" {
			t.Fatal("workspace transition", w)
		}
		found = true
	}
	if !found {
		t.Fatal("channel workspace missing")
	}
	status, out, _ = f.call(t, "GET", base+"/takeovers", nil, "")
	mustOK(t, status, out)
	if out["can_notify"] != false || len(out["items"].([]any)) != 1 {
		t.Fatal("direct takeover not visible without notice", out)
	}
	if _, e := f.a.Organization.ReportExit(context.Background(), nil, f.a.Installation.OwnerStaffID, channel, "stale page"); !errors.Is(e, organization.ErrBusinessClosed) {
		t.Fatal("domain accepted late exit", e)
	}
	for _, resource := range []string{"exits", "plans", "pricing", "marketing", "business-status", "procurement"} {
		status, out, _ = f.call(t, "POST", base+"/"+resource, map[string]any{}, "")
		if status != 409 || out["error"].(map[string]any)["code"] != "channel_takeover_restricted" {
			t.Fatalf("%s stale command: %d %#v", resource, status, out)
		}
	}
	for _, resource := range []string{"plans", "procurement", "ledger", "invoices", "payments", "exits", "business-status"} {
		status, out, _ = f.call(t, "GET", base+"/"+resource, nil, "")
		mustOK(t, status, out)
		if resource == "plans" || resource == "exits" || resource == "business-status" {
			if len(out["operations"].([]any)) != 0 {
				t.Fatal("unavailable action advertised", resource, out)
			}
		}
	}
}
