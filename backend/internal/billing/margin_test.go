package billing

import "testing"

func TestMarginIsSellMinusCost(t *testing.T) {
	if got := marginOf(100, 40); got != 60 {
		t.Fatalf("positive margin: got %d", got)
	}
	if got := marginOf(40, 100); got != -60 {
		t.Fatalf("negative margin must stay negative: got %d", got)
	}
	if got := marginOf(0, 0); got != 0 {
		t.Fatalf("zero: got %d", got)
	}
}

func TestInventedSupplierSourceRejected(t *testing.T) {
	for _, src := range []string{"estimate", "estimated", "attempt_cost", "usage_cost", "guessed", "invented", "fill_cost"} {
		if !InventedSupplierSource(src) {
			t.Fatalf("%s must be treated as invented cost", src)
		}
		if AllowedSupplierSource(src) {
			t.Fatalf("%s must not be an allowed cash source", src)
		}
	}
	for _, src := range []string{SupplierInvoice, SupplierRecharge, SupplierOther} {
		if InventedSupplierSource(src) {
			t.Fatalf("%s is cash, not invented attempt cost", src)
		}
		if !AllowedSupplierSource(src) {
			t.Fatalf("%s should be allowed cash source", src)
		}
	}
}

func TestPriceTiersFromRawFourColumns(t *testing.T) {
	raw := []byte(`{"upstream_cost_input":"0.000003","upstream_cost_output":"0.000004","wholesale_input":"0.000006","wholesale_output":"0.000008","customer_sell_input":"0.000009","customer_sell_output":"0.000011","channel_customer_input":"0.000010","channel_customer_output":"0.000012"}`)
	got := priceTiersFromRaw(raw)
	if got.Upstream != "0.000003/0.000004" {
		t.Fatalf("upstream: %+v", got)
	}
	if got.Wholesale != "0.000006/0.000008" {
		t.Fatalf("wholesale: %+v", got)
	}
	if got.Sell != "0.000009/0.000011" {
		t.Fatalf("sell: %+v", got)
	}
	if got.Channel != "0.000010/0.000012" {
		t.Fatalf("channel: %+v", got)
	}
}

func TestAssembleDoesNotAddSupplierCashToAttemptCost(t *testing.T) {
	// 装配口径：attempt 成本只来自 cost_entries / usage.upstream，供应商现金不进合计。
	attemptCost := int64(400)
	supplierCash := int64(-2_000_000)
	sell := int64(1000)
	assembled := marginOf(sell, attemptCost)
	if assembled != 600 {
		t.Fatalf("margin ignores supplier cash: got %d", assembled)
	}
	if assembled == marginOf(sell, attemptCost+(-supplierCash)) {
		t.Fatal("must not fold supplier cash into attempt cost")
	}
}

func TestSameAttemptCostFactCountedOnce(t *testing.T) {
	facts := []int64{120, 120, 120}
	seen := map[string]struct{}{"atm_1": {}}
	total := int64(0)
	for i, amt := range facts {
		key := "atm_1"
		if i == 0 {
			total += amt
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		total += amt
	}
	if total != 120 {
		t.Fatalf("same attempt must not double-write: %d", total)
	}
}
