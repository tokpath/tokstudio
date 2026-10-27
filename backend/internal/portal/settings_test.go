package portal

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/operations"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"github.com/tokpath/tokstudio/backend/internal/remediation"
)

func TestPostgresSettingsSimpleCommandsRetainReadinessVersionAndAudit(t *testing.T) {
	f := setupPortal(t)
	f.login(t, "staff", "owner@example.test", "owner-password-test")
	op := f.a.Installation.OperatorID
	base := "/v1/operators/" + op
	for _, removed := range []string{"risk", "retention"} {
		status, out, _ := f.call(t, "GET", base+"/"+removed, nil, "")
		if status != 404 {
			t.Fatal("removed workspace still exposed", removed, status, out)
		}
	}
	status, page, _ := f.call(t, "GET", base+"/business-status", nil, "")
	mustOK(t, status, page)
	row := page["items"].([]any)[0].(map[string]any)
	key := uuid.NewString()
	input := map[string]any{"feature": "settlement", "enabled": false, "expected_version": row["version"]}
	status, out, _ := f.call(t, "POST", base+"/business-status", input, key)
	mustOK(t, status, out)
	status, replay, _ := f.call(t, "POST", base+"/business-status", input, key)
	mustOK(t, status, replay)
	if out["version"] != replay["version"] {
		t.Fatal("toggle replay changed state twice", out, replay)
	}
	status, out, _ = f.call(t, "POST", base+"/business-status", input, "")
	if status != 409 {
		t.Fatal("stale toggle overwrote newer state", status, out)
	}
	state, err := f.a.Organization.GetBusinessStatus(context.Background(), nil, op)
	if err != nil {
		t.Fatal(err)
	}
	status, out, _ = f.call(t, "POST", base+"/business-status", map[string]any{"feature": "new_purchase", "enabled": true, "expected_version": state.Version}, "")
	if status != 409 {
		t.Fatal("readiness bypassed", status, out)
	}
	status, page, _ = f.call(t, "GET", base+"/audit", nil, "")
	mustOK(t, status, page)
	found := false
	for _, raw := range page["items"].([]any) {
		v := raw.(map[string]any)["values"].(map[string]any)
		if v["evidence"] == "business settings: settlement" && v["actor_id"] == f.a.Installation.OwnerStaffID {
			found = true
		}
	}
	if !found {
		t.Fatal("simple toggle lost actor or audit", page)
	}
	f.a.Mail.SetSender(nil)
	status, page, _ = f.call(t, "GET", base+"/staff", nil, "")
	mustOK(t, status, page)
	for _, raw := range page["operations"].([]any) {
		for _, rawField := range raw.(map[string]any)["fields"].([]any) {
			field := rawField.(map[string]any)
			if field["name"] == "delivery_mode" {
				choices := field["options"].([]any)
				if len(choices) != 1 || choices[0].(map[string]any)["value"] != "manual" {
					t.Fatal("unconfigured email offered", field)
				}
			}
		}
	}
	// One human explanation supplies the immutable decision evidence too.
	due := time.Now().Add(time.Hour)
	input = map[string]any{"scope_kind": "operator", "scope_id": op, "action": "activity", "enabled": true, "expected_version": 0, "explanation": "Synthetic scoped review", "review_due_at": due}
	status, out, _ = f.call(t, "POST", base+"/restrictions", input, "")
	mustOK(t, status, out)
	var freeze organization.ScopeFreeze
	if err = f.a.DB.First(&freeze, "scope_kind='operator' AND scope_id=? AND action='activity'", op).Error; err != nil || freeze.Evidence != "Synthetic scoped review" {
		t.Fatal(freeze, err)
	}
	input["enabled"], input["expected_version"] = false, 1
	delete(input, "review_due_at")
	status, out, _ = f.call(t, "POST", base+"/restrictions", input, "")
	mustOK(t, status, out)
	status, page, _ = f.call(t, "GET", base+"/restrictions", nil, "")
	mustOK(t, status, page)
	if rows := page["items"].([]any); len(rows) != 1 || rows[0].(map[string]any)["status"] != "resolved" {
		t.Fatal("released restriction shown as active", page)
	}
}

func TestPostgresSettingsAuditAndReviewRespectActualPermissions(t *testing.T) {
	f := setupPortal(t)
	op, owner := f.a.Installation.OperatorID, f.a.Installation.OwnerStaffID
	reviewer := reviewStaff(t, f, owner, op, "settings-reviewer@example.test", "support")
	var admin Staff
	if err := f.a.DB.First(&admin, "id=?", owner).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.a.DB.Model(&Staff{}).Where("id=?", reviewer.ID).Update("password_hash", admin.PasswordHash).Error; err != nil {
		t.Fatal(err)
	}
	f.login(t, "staff", reviewer.Email, "owner-password-test")
	base := "/v1/operators/" + op
	if err := f.a.DB.Create(&remediation.Case{ID: uuid.NewString(), OperatorID: op, BusinessKey: "settings-review", Fingerprint: "synthetic", Kind: "unallocated", AmountUSD: ledger.MustDecimal("1"), State: "pending_review", Reason: "test", Evidence: "test", RequestedBy: owner, RequiredReviews: 1, CreatedAt: time.Now()}).Error; err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		grants   []string
		resource string
		views    int
	}{{[]string{"withdrawals.manage"}, "approvals", 0}, {[]string{"ledger.read"}, "audit", 3}} {
		if err := f.a.Organization.SetMembership(context.Background(), nil, organization.SetMembershipInput{ActorStaffID: owner, OperatorID: op, StaffID: reviewer.ID, Role: "custom", Status: "active", Grants: test.grants}); err != nil {
			t.Fatal(err)
		}
		status, page, _ := f.call(t, "GET", base+"/"+test.resource, nil, "")
		mustOK(t, status, page)
		if test.resource == "approvals" && len(page["items"].([]any)) != 0 {
			t.Fatal("withdrawal reviewer saw unauthorized refunds", page)
		}
		if test.views > 0 {
			if len(page["views"].([]any)) != test.views {
				t.Fatal("unreadable audit tabs", page)
			}
			status, out, _ := f.call(t, "GET", base+"/audit?view=organization", nil, "")
			if status != 403 {
				t.Fatal("audit escaped permissions", status, out)
			}
		}
	}
}

func TestPostgresSettingsNotificationHasReadableContentAndOptionalReference(t *testing.T) {
	f := setupPortal(t)
	op, owner := f.a.Installation.OperatorID, f.a.Installation.OwnerStaffID
	f.login(t, "staff", "owner@example.test", "owner-password-test")
	path := "/v1/operators/" + op + "/notifications"
	status, out, _ := f.call(t, "POST", path, map[string]any{"recipient_id": owner, "category": "mandatory", "title": "Readable settings notice", "body": "A useful message"}, "")
	mustOK(t, status, out)
	status, page, _ := f.call(t, "GET", path+"?q=Readable%20settings", nil, "")
	mustOK(t, status, page)
	rows := page["items"].([]any)
	if len(rows) != 1 {
		t.Fatal(page)
	}
	v := rows[0].(map[string]any)["values"].(map[string]any)
	if v["title"] != "Readable settings notice" || v["body"] != "A useful message" || v["recipient_id"] != owner || v["severity"] != "info" {
		t.Fatal(v)
	}
	var notice operations.Notification
	if err := f.a.DB.First(&notice, "id=?", fmt.Sprint(out["id"])).Error; err != nil || notice.Reference == "" {
		t.Fatal(notice, err)
	}
}
