package ops

import "testing"

func TestDashboardFailedModulesAreNotZero(t *testing.T) {
	d := &Dashboard{Totals: MoneyView{RevenueMinor: 0, GrossProfitMinor: 0, SuccessRate: 0.9, CallbackP95MS: 10}, ModuleErrors: map[string]string{"finance": "read_error", "callbacks": "read_error"}}
	totals := d.SafeTotals()
	if _, ok := totals["revenue_minor"]; ok {
		t.Fatal("failed finance serialized fake zero")
	}
	if _, ok := totals["callback_latency_p95_ms"]; ok {
		t.Fatal("failed callbacks serialized a stale value")
	}
	if totals["success_rate"] != 0.9 {
		t.Fatalf("healthy traffic lost: %+v", totals)
	}
	d.ModuleErrors = map[string]string{"traffic": "read_error"}
	totals = d.SafeTotals()
	if _, ok := totals["success_rate"]; ok {
		t.Fatal("failed traffic serialized fake zero")
	}
	if totals["revenue_minor"] != float64(0) {
		t.Fatal("successfully read real zero lost")
	}
}
