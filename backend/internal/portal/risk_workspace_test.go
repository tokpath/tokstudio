package portal

import (
	"context"
	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/operations"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"testing"
)

func riskOperators(t *testing.T, f *testApp) map[organization.OperatorKind]string {
	t.Helper()
	owner := f.a.Installation.OwnerStaffID
	operators := map[organization.OperatorKind]string{organization.Platform: f.a.Installation.OperatorID}
	parent := f.a.Installation.OperatorID
	for _, kind := range []organization.OperatorKind{organization.OEM, organization.Channel} {
		child, err := f.a.Organization.CreateOperator(context.Background(), nil, organization.CreateOperatorInput{ActorStaffID: owner, ParentOperatorID: parent, Kind: kind, Name: "Risk " + string(kind), BrandName: "Risk OEM"})
		if err != nil {
			t.Fatal(err)
		}
		inv, err := f.a.Organization.InviteStaff(context.Background(), nil, owner, child.Operator.ID, "owner@example.test", "owner")
		if err != nil {
			t.Fatal(err)
		}
		_, err = f.a.Organization.AcceptStaffInvitation(context.Background(), nil, inv.Token, organization.VerifiedStaff{ID: owner, Email: "owner@example.test", EmailVerified: true})
		if err != nil {
			t.Fatal(err)
		}
		operators[kind], parent = child.Operator.ID, child.Operator.ID
	}
	return operators
}
func TestPostgresFixedRiskCardsScopeDefaultsAndSave(t *testing.T) {
	f := setupPortal(t)
	operators := riskOperators(t, f)
	f.login(t, "staff", "owner@example.test", "owner-password-test")
	for kind, operator := range operators {
		base := "/v1/operators/" + operator + "/alerts"
		if kind == organization.Platform {
			for _, method := range []string{"GET", "POST"} {
				status, _, _ := f.call(t, method, base, map[string]any{}, uuid.NewString())
				if status != 404 {
					t.Fatal("platform alerts still available", method, status)
				}
			}
			continue
		}
		// The background evaluator initializes defaults even if nobody visits the page.
		if _, err := f.a.Operations.RunRisks(t.Context(), operator); err != nil {
			t.Fatal(err)
		}
		var count int64
		f.a.DB.Model(&operations.RiskRule{}).Where("operator_id=?", operator).Count(&count)
		if count != 2 {
			t.Fatal(count)
		}
		status, page, _ := f.call(t, "GET", base, nil, "")
		mustOK(t, status, page)
		if len(page["items"].([]any)) != 2 || len(page["operations"].([]any)) != 0 || page["views"] != nil || page["search_supported"] != false {
			t.Fatal(page)
		}
		var originalID string
		for _, raw := range page["items"].([]any) {
			row := raw.(map[string]any)
			v := row["values"].(map[string]any)
			if v["warning"] != "100" || v["reminder_minutes"] != "60" || v["enabled"] != true || len(v["recipients"].([]any)) != 0 || len(v["default_owner_ids"].([]any)) != 1 {
				t.Fatal(v)
			}
			if v["metric"] == "sales_space_atg" {
				originalID = row["id"].(string)
			}
		}
		input := map[string]any{"metric": "sales_space_atg", "warning": "12.000000000000000001", "recipients": []string{}, "reminder_minutes": "1.5", "enabled": true, "expected_version": 1}
		key := uuid.NewString()
		for i := 0; i < 2; i++ {
			status, out, _ := f.call(t, "POST", base, input, key)
			mustOK(t, status, out)
			if out["id"] != originalID || out["version"] != float64(2) {
				t.Fatal(out)
			}
		}
		status, _, _ = f.call(t, "POST", base, input, uuid.NewString())
		if status != 409 {
			t.Fatal("stale settings accepted", status)
		}
		input["expected_version"] = 2
		for _, metric := range []string{"pending_usage", "unverified_costs", "oversell_used_atg"} {
			input["metric"] = metric
			status, _, _ = f.call(t, "POST", base, input, uuid.NewString())
			if status != 400 {
				t.Fatal(metric, status)
			}
		}
		input["metric"] = "sales_space_atg"
		input["recipients"] = []string{uuid.NewString()}
		status, _, _ = f.call(t, "POST", base, input, uuid.NewString())
		if status != 400 {
			t.Fatal("foreign recipient", status)
		}
		input["recipients"] = []string{}
		input["warning"] = "0"
		status, _, _ = f.call(t, "POST", base, input, uuid.NewString())
		if status != 400 {
			t.Fatal("zero threshold", status)
		}
		input["warning"] = "20"
		input["enabled"] = false
		delete(input, "recipients") // Omitting optional recipients follows the owners.
		status, out, _ := f.call(t, "POST", base, input, uuid.NewString())
		mustOK(t, status, out)
		status, page, _ = f.call(t, "GET", base+"?view=rules", nil, "")
		mustOK(t, status, page)
		for _, raw := range page["items"].([]any) {
			row := raw.(map[string]any)
			if row["id"] == originalID && row["status"] != "disabled" {
				t.Fatal(row)
			}
		}
		f.a.DB.Model(&operations.RiskRule{}).Where("operator_id=?", operator).Count(&count)
		if count != 2 {
			t.Fatal("save duplicated fixed settings", count)
		}
	}
}

func TestRiskMinutesPreserveSavedSecondsAndRejectOutOfRangeDurations(t *testing.T) {
	for _, seconds := range []int64{60, 61, 90, 3599, 3600, 86400, 30 * 86400} {
		minutes := ledger.MustDecimal(riskMinutes(seconds))
		got, ok := riskSeconds(minutes)
		if !ok || got != seconds {
			t.Fatalf("duration changed during editing: %d seconds -> %s minutes -> %d seconds", seconds, minutes, got)
		}
	}
	for _, minutes := range []string{"0", "-1", "0.999", "43200.001", "99999999999999999999"} {
		if _, ok := riskSeconds(ledger.MustDecimal(minutes)); ok {
			t.Fatal("invalid minute duration accepted", minutes)
		}
	}
}
