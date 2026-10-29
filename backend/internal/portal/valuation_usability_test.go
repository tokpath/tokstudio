package portal

import (
	"strings"
	"testing"
)

func TestPostgresSettlementFormsUseOneTransferAmountAndEvidence(t *testing.T) {
	f := setupPortal(t)
	f.login(t, "staff", "owner@example.test", "owner-password-test")
	parent := f.a.Installation.OperatorID
	count := 0
	for _, resource := range []string{"receipt-valuations", "exits", "refunds"} {
		status, out, _ := f.call(t, "GET", "/v1/operators/"+parent+"/"+resource, nil, "")
		mustOK(t, status, out)
		for _, raw := range out["row_operations"].([]any) {
			op := raw.(map[string]any)
			fields := map[string]map[string]any{}
			for _, raw := range op["fields"].([]any) {
				field := raw.(map[string]any)
				name := field["name"].(string)
				fields[name] = field
				if strings.HasPrefix(name, "valuation.") || strings.HasPrefix(name, "refund_valuation.") || name == "fx_execution_reference" {
					t.Fatal("form exposes a reusable exchange rate or internal identity", resource, op["name"], name)
				}
			}
			amount, ok := fields["settlement_amount"]
			if !ok {
				continue
			}
			count++
			evidenceName := "settlement_evidence"
			if resource == "receipt-valuations" {
				evidenceName = "evidence_reference"
				if amount["required"] != true {
					t.Fatal("pending receipt confirmation lost its settlement amount", amount)
				}
			}
			if _, ok := fields[evidenceName]; !ok {
				t.Fatal("settlement amount has no transfer evidence field", resource, op["name"])
			}
		}
	}
	if count < 10 {
		t.Fatal("settlement workflows were not exercised", count)
	}
}
