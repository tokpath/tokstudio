package portal

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/ledger"
)

func TestMarketingPercentagePrecision(t *testing.T) {
	for _, points := range []string{"0", "15", "33.3333333333333333", "100"} {
		rate, err := percentageRate(ledger.MustDecimal(points))
		if err != nil || ratePercentage(rate).Cmp(ledger.MustDecimal(points)) != 0 {
			t.Fatalf("percentage round trip %s: %s %v", points, rate, err)
		}
	}
	for _, points := range []string{"-1", "100.01", "0.000000000000000001"} {
		if _, err := percentageRate(ledger.MustDecimal(points)); err == nil {
			t.Fatalf("invalid or unrepresentable percentage accepted: %s", points)
		}
	}
}

func TestMarketingWorkspacePublishesNamedActivityWithoutBudget(t *testing.T) {
	f := setupPortal(t)
	f.login(t, "staff", "owner@example.test", "owner-password-test")
	base := "/v1/operators/" + f.a.Installation.OperatorID
	status, out, _ := f.call(t, "POST", base+"/commissions", map[string]any{
		"threshold_money": "100", "direct_percent": "15", "indirect_percent": "5", "confirmation_days": 30,
		"reward_validity_days": 30, "invitee_atg": "0", "inviter_atg": "0", "kol_cross_activity_limit": 0, "expected_version": 0,
	}, "")
	mustOK(t, status, out)
	status, out, _ = f.call(t, "GET", base+"/commissions", nil, "")
	mustOK(t, status, out)
	v := out["items"].([]any)[0].(map[string]any)["values"].(map[string]any)
	if v["direct_percent"] != "15" || v["direct_rate"] != nil {
		t.Fatalf("percentage conversion: %+v", v)
	}
	status, out, _ = f.call(t, "POST", base+"/plans", map[string]any{"name": "Starter", "price_money": "100", "paid_atg": "100", "gift_atg": "20", "period": "lifetime", "new_purchase_enabled": true, "renewal_enabled": false, "expected_version": 0}, "")
	mustOK(t, status, out)
	packageID := out["id"].(string)
	status, out, _ = f.call(t, "POST", base+"/marketing", map[string]any{
		"name": "Summer offer", "package_id": packageID, "eligibility": "any", "final_price_money": "90", "gift_atg": "10",
		"starts_at": time.Now().Add(-time.Hour), "ends_at": time.Now().Add(time.Hour), "per_customer_limit": 1, "total_use_limit": 0, "enabled": true,
	}, "")
	mustOK(t, status, out)
	status, out, _ = f.call(t, "GET", base+"/marketing", nil, "")
	mustOK(t, status, out)
	rows := out["items"].([]any)
	if len(rows) != 1 || rows[0].(map[string]any)["status"] != "active" {
		t.Fatalf("campaign state: %+v", rows)
	}
	encoded, _ := json.Marshal(out)
	if strings.Contains(string(encoded), "budget_atg") || strings.Contains(string(encoded), "discount_budget") || !strings.Contains(string(encoded), "Summer offer") {
		t.Fatal("campaign still exposes redundant budgets or omits name")
	}
	ops := out["operations"].([]any)
	if len(ops) != 1 || ops[0].(map[string]any)["name"] != "publish" {
		t.Fatalf("activity toolbar contains unrelated workflows: %+v", ops)
	}
}
