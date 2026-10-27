package portal

import (
	"testing"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/operations"
)

func TestPostgresRiskWorkspaceEditsActualRuleVersionsAndKeepsObservationsReadOnly(t *testing.T) {
	f := setupPortal(t)
	op, owner := f.a.Installation.OperatorID, f.a.Installation.OwnerStaffID
	f.login(t, "staff", "owner@example.test", "owner-password-test")
	base := "/v1/operators/" + op
	input := map[string]any{"name": "采购余额补货提醒", "metric": "procurement_available_atg", "warning": "10.000000000000000001", "critical": "2.000000000000000001", "direction": "below", "recipients": []string{owner}, "escalation_recipients": []string{}, "reminder_seconds": 3600, "escalate_seconds": 7200, "enabled": true, "expected_version": 0}
	creationKey := uuid.NewString()
	var status int
	var out map[string]any
	createdID := ""
	for i := 0; i < 2; i++ {
		status, out, _ = f.call(t, "POST", base+"/alerts", input, creationKey)
		mustOK(t, status, out)
		id, _ := out["id"].(string)
		if _, err := uuid.Parse(id); err != nil || out["version"] != float64(1) || (createdID != "" && id != createdID) {
			t.Fatal("new rule did not generate and replay one stable identity", out)
		}
		createdID = id
	}
	input["id"] = createdID
	read := func(resource string) map[string]any {
		t.Helper()
		status, out, _ := f.call(t, "GET", base+resource, nil, "")
		mustOK(t, status, out)
		return out
	}
	assertRule := func(page map[string]any, version float64, warning string) {
		t.Helper()
		rows := page["items"].([]any)
		if len(rows) != 1 {
			t.Fatal("rule missing", page)
		}
		row := rows[0].(map[string]any)
		value := row["values"].(map[string]any)
		if row["id"] != input["id"] || row["version"] != version || value["warning"] != warning || value["reminder_seconds"] != float64(3600) || value["recipients"].([]any)[0] != owner {
			t.Fatal("saved rule fields/version were not projected", row)
		}
		if value["name"] != input["name"] || value["recipient_names"] != "Owner · owner@example.test" {
			t.Fatal("rule name or responsible person was not readable", value)
		}
		if actions := row["allowed_actions"].([]any); len(actions) != 1 || actions[0] != "revise_rule" {
			t.Fatal("rule not editable", row)
		}
		edit := page["row_operations"].([]any)[0].(map[string]any)
		if edit["name"] != "revise_rule" || edit["permission"] != "operations.manage" {
			t.Fatal("edit operation authority", edit)
		}
		for _, field := range edit["fields"].([]any) {
			f := field.(map[string]any)
			if (f["name"] == "id" || f["name"] == "expected_version") && f["kind"] != "hidden" {
				t.Fatal("rule identity/version is not fixed to selected record", f)
			}
			if f["name"] == "recipients" {
				options, ok := f["options"].([]any)
				if !ok || len(options) != 1 || options[0].(map[string]any)["value"] != owner || options[0].(map[string]any)["label"] != "Owner · owner@example.test" {
					t.Fatal("responsible people cannot be selected by name/email", f)
				}
			}
		}
	}
	assertRule(read("/alerts?view=rules"), 1, input["warning"].(string))
	if len(read("/alerts")["items"].([]any)) != 0 {
		t.Fatal("an enabled rule was presented as an active risk")
	}
	state := operations.RiskState{OperatorID: op, RuleID: input["id"].(string), Status: "warning", Value: ledger.MustDecimal("5.000000000000000001"), Sequence: 7}
	if err := f.a.DB.Create(&state).Error; err != nil {
		t.Fatal(err)
	}
	for _, view := range []string{"/alerts", "/alerts?view=observations"} {
		page := read(view)
		rows := page["items"].([]any)
		if len(rows) != 1 {
			t.Fatal("observation missing", page)
		}
		row := rows[0].(map[string]any)
		if row["values"].(map[string]any)["name"] != input["name"] {
			t.Fatal("risk observation lost the readable rule name", row)
		}
		if row["version"] != float64(0) || row["status"] != "warning" || len(row["allowed_actions"].([]any)) != 0 || len(page["row_operations"].([]any)) != 0 {
			t.Fatal("observation can masquerade as an editable rule", page)
		}
	}
	assertRule(read("/alerts?view=rules"), 1, input["warning"].(string))
	status, _, _ = f.call(t, "GET", base+"/alerts?view=unknown", nil, "")
	if status != 400 {
		t.Fatal("unrecognized view accepted", status)
	}
	input["expected_version"], input["warning"] = 1, "11.000000000000000001"
	key := uuid.NewString()
	for i := 0; i < 2; i++ {
		status, out, _ = f.call(t, "POST", base+"/alerts", input, key)
		mustOK(t, status, out)
		if out["version"] != float64(2) {
			t.Fatal("same original action did not replay the saved rule version", out)
		}
	}
	status, out, _ = f.call(t, "POST", base+"/alerts", input, uuid.NewString())
	if status != 409 || out["error"].(map[string]any)["code"] != "version_conflict" {
		t.Fatal("stale rule did not require review", status, out)
	}
	assertRule(read("/alerts?view=rules"), 2, input["warning"].(string))
	// A custom operator with only the exact rule-management permission must be
	// able to use both views without inheriting financial report permissions.
	var membership string
	if err := f.a.DB.Table("organization_staff_memberships").Select("id").Where("staff_id=? AND operator_id=?", owner, op).Scan(&membership).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.a.DB.Exec("UPDATE organization_staff_memberships SET role='custom' WHERE id=?", membership).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.a.DB.Exec("INSERT INTO organization_staff_grants(membership_id,action) VALUES (?,?)", membership, "operations.manage").Error; err != nil {
		t.Fatal(err)
	}
	assertRule(read("/alerts?view=rules"), 2, input["warning"].(string))
	read("/alerts")
	input["expected_version"], input["enabled"] = 2, false
	status, out, _ = f.call(t, "POST", base+"/alerts", input, uuid.NewString())
	mustOK(t, status, out)
	if out["version"] != float64(3) {
		t.Fatal("custom operations permission could not revise rule", out)
	}
	for _, path := range []string{"/v1/operators/" + uuid.NewString() + "/alerts", base + "/reports/financial?from=2026-10-01T00:00:00Z&to=2026-10-02T00:00:00Z"} {
		status, _, _ = f.call(t, "GET", path, nil, "")
		if status == 200 {
			t.Fatal("rule management granted another scope or financial reports", path)
		}
	}
	var retained operations.RiskState
	if err := f.a.DB.Where("operator_id=? AND rule_id=?", op, state.RuleID).Take(&retained).Error; err != nil {
		t.Fatal(err)
	}
	if retained.Status != state.Status || retained.Sequence != state.Sequence || retained.Value.String() != state.Value.String() {
		t.Fatal("rule revision rewrote historical observation", retained)
	}
}
