package portal

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestPostgresSubscriptionPoolUsesExistingModelsRoutesAndActualCosts(t *testing.T) {
	f := setupPortal(t)
	base := "/v1/operators/" + f.a.Installation.OperatorID
	f.login(t, "staff", "owner@example.test", "owner-password-test")
	post := func(path string, body any) map[string]any {
		t.Helper()
		status, out, _ := f.call(t, "POST", base+path, body, uuid.NewString())
		mustOK(t, status, out)
		return out
	}
	const token = "sk-ant-oat-test-portal-credentials"
	pool := post("/subscription-pools", map[string]any{"action": "save_pool", "pool": map[string]any{
		"name": "Claude subscription", "subscription_kind": "claude", "enabled": true,
		"first_account": map[string]any{"name": "Account A", "credential": token},
	}})["id"].(string)
	status, page, _ := f.call(t, "GET", base+"/subscription-pools", nil, "")
	mustOK(t, status, page)
	encoded, _ := json.Marshal(page)
	if strings.Contains(string(encoded), token) || strings.Contains(string(encoded), "ciphertext") {
		t.Fatal("account page exposed authorization material")
	}
	status, reference, _ := f.call(t, "GET", base+"/references/credentials?q="+pool, nil, "")
	mustOK(t, status, reference)
	if items := reference["items"].([]any); len(items) != 1 || items[0].(map[string]any)["value"] != pool {
		t.Fatal("existing supplier connection picker does not offer the pool")
	}
	// The runtime must have a verified native model bound before procurement.
	if err := f.a.DB.Exec(`INSERT INTO model_pricing_rows (id,provider,model,mode,definition)
		VALUES ('pool-claude','anthropic','pool-claude','chat','{"max_input_tokens":131072,"max_output_tokens":4096}'::jsonb)`).Error; err != nil {
		t.Fatal(err)
	}
	supplierModel := post("/providers", map[string]any{"action": "model", "supplier_model": map[string]any{
		"id": uuid.NewString(), "credential_id": pool, "provider": "anthropic", "model": "pool-claude", "enabled": true,
		"capabilities": []map[string]any{{"operation": "chat", "streaming": true}},
	}})["id"].(string)
	post("/catalog", map[string]any{"model": map[string]any{"id": "subscription-public", "capabilities": []map[string]any{{"operation": "chat"}}}})
	post("/routes", map[string]any{"route": map[string]any{
		"public_model_id": "subscription-public", "strategy": "priority",
		"bindings": []map[string]any{{"supplier_model_id": supplierModel, "enabled": true}},
	}})

	now := time.Now().UTC()
	statement := map[string]any{"provider_id": pool, "statement_reference": "SUB-2026-11", "account_coverage": "Account A",
		"starts_at": now.Add(-time.Hour), "ends_at": now.Add(time.Hour), "currency": "USD", "amount": "40"}
	statementID := post("/supplier-costs", statement)["id"].(string)
	status, out, _ := f.call(t, "POST", base+"/supplier-costs", statement, uuid.NewString())
	if status != 409 {
		t.Fatal("duplicate statement was accepted", status, out)
	}
	status, costs, _ := f.call(t, "GET", base+"/supplier-costs", nil, "")
	mustOK(t, status, costs)
	rows := costs["items"].([]any)
	if len(rows) != 1 {
		t.Fatalf("expected one actual statement, got %d", len(rows))
	}
	values := rows[0].(map[string]any)["values"].(map[string]any)
	if values["subscription_kind"] != "claude" || fmt.Sprint(values["amount"]) != "40" || values["reference_amount"] != nil || values["account_coverage"] != "Account A" {
		t.Fatal("subscription cost was reported as a per-call reference charge", values)
	}
	status, page, _ = f.call(t, "GET", base+"/subscription-pools", nil, "")
	mustOK(t, status, page)
	latest := page["items"].([]any)[0].(map[string]any)["latest_statement"].(map[string]any)
	if latest["id"] != statementID || latest["currency"] != "USD" || latest["statement_reference"] != "SUB-2026-11" {
		t.Fatal("pool did not link to its latest recorded statement", latest)
	}
	status, costs, _ = f.call(t, "GET", base+"/supplier-costs?record="+statementID, nil, "")
	mustOK(t, status, costs)
	if rows := costs["items"].([]any); len(rows) != 1 || rows[0].(map[string]any)["id"] != statementID {
		t.Fatal("linked statement was not found", rows)
	}
}
