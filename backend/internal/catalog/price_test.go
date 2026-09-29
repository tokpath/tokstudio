package catalog

import (
	"encoding/json"
	"testing"
)

func TestNormalizeUnitPrices(t *testing.T) {
	got, err := NormalizeUnitPrices(map[string]any{
		"model":            "tokenhub/echo-1",
		"customer_sell":    map[string]any{"input": "0.000003", "output": "0.000006"},
		"wholesale":        map[string]any{"input": "0.000002", "output": "0.000004"},
		"upstream_cost":    map[string]any{"input": "0.000001", "output": "0.000002"},
		"channel_override": map[string]any{"input": "0.0000035", "output": "0.000007"},
		"currency":         "USD",
	})
	if err != nil {
		t.Fatal(err)
	}
	assertStr(t, got["input"], "0.000003")
	assertStr(t, got["output"], "0.000006")
	assertStr(t, got["customer_sell_input"], "0.000003")
	for _, key := range []string{"wholesale_input", "upstream_cost_output", "channel_customer_input"} {
		if got[key] != nil {
			t.Fatalf("%s must be configured outside model price: %+v", key, got)
		}
	}
	if _, ok := got["customer_sell"].(map[string]any); !ok {
		t.Fatalf("customer_sell nested missing: %+v", got)
	}
	if _, err := NormalizeUnitPrices(map[string]any{"currency": "USD"}); err != ErrEmptyUnitPrices {
		t.Fatalf("empty prices should fail, got %v", err)
	}

	legacy, err := NormalizeUnitPrices(map[string]any{"input": "0.01", "output": "0.02"})
	if err != nil {
		t.Fatal(err)
	}
	assertStr(t, legacy["customer_sell_input"], "0.01")
	assertStr(t, legacy["input"], "0.01")
}

func TestPriceBookFromRowSellOnly(t *testing.T) {
	raw, _ := json.Marshal(map[string]any{
		"input": "0.000003", "output": "0.000006",
	})
	view := priceBookFromRow(priceRow{ID: "prc_1", Status: "published", UnitPrices: raw}, "tokenhub/echo-1")
	if view.Sell != "0.000003/0.000006" {
		t.Fatalf("sell=%q", view.Sell)
	}
	if view.PublicID != "tokenhub/echo-1" || view.Status != "published" {
		t.Fatalf("identity: %+v", view)
	}
}

func TestMergeUnitPricesDropsPriorInternalDims(t *testing.T) {
	prev, err := NormalizeUnitPrices(map[string]any{
		"input": "0.000001", "output": "0.000002",
		"upstream_cost_input": "0.0000004", "wholesale_input": "0.0000007",
	})
	if err != nil {
		t.Fatal(err)
	}
	next, err := NormalizeUnitPrices(map[string]any{"input": "0.01", "output": "0.02"})
	if err != nil {
		t.Fatal(err)
	}
	merged := mergeUnitPrices(prev, next)
	assertStr(t, merged["input"], "0.01")
	if merged["upstream_cost_input"] != nil || merged["wholesale_input"] != nil {
		t.Fatalf("internal price dimensions must not survive: %+v", merged)
	}
}

func assertStr(t *testing.T, got any, want string) {
	t.Helper()
	if stringifyPrice(got) != want {
		t.Fatalf("got %v want %s", got, want)
	}
}
