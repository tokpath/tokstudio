package portal

import "testing"

func TestPostgresValuationFormsHideInternalIDsAndRequireRealNonUSDEvidence(t *testing.T) {
	f := setupPortal(t)
	f.login(t, "staff", "owner@example.test", "owner-password-test")
	parent := f.a.Installation.OperatorID
	count := 0
	for _, resource := range []string{"receipt-valuations", "exits", "refunds"} {
		status, out, _ := f.call(t, "GET", "/v1/operators/"+parent+"/"+resource, nil, "")
		mustOK(t, status, out)
		for _, raw := range out["row_operations"].([]any) {
			op := raw.(map[string]any)
			for _, raw := range op["fields"].([]any) {
				field := raw.(map[string]any)
				name := field["name"].(string)
				if name == "valuation.id" || name == "refund_valuation.id" {
					t.Fatal("form still requires a user-invented fact identifier", resource, op["name"])
				}
				if name != "valuation.usd_per_unit" && name != "refund_valuation.usd_per_unit" && name != "fx_execution_reference" {
					continue
				}
				count++
				if field["required"] != true {
					t.Fatal("real non-USD valuation evidence became optional", resource, op["name"], field)
				}
				condition, ok := field["visible_when"].(map[string]any)
				if !ok || condition["not"] != true {
					t.Fatal("native USD still exposes unnecessary FX fields", field)
				}
				if values := condition["values"].([]any); len(values) != 1 || values[0] != "USD" {
					t.Fatal("stablecoins were treated as native USD", condition)
				}
				if op["name"] == "verify_evm" && condition["field"] != "asset" {
					t.Fatal("EVM valuation visibility depends on an absent money input", condition)
				}
				if resource == "receipt-valuations" && condition["field"] != "currency" {
					t.Fatal("receipt valuation lost its verified original currency", condition)
				}
			}
		}
	}
	if count < 10 {
		t.Fatal("valuation workflows were not exercised", count)
	}
}
