package portal

import (
	"github.com/tokpath/tokstudio/backend/internal/operations"
	"testing"
)

func TestFinanceCanReadOwnNotificationsWithoutManagingOtherRecipients(t *testing.T) {
	f := setupPortal(t)
	op, owner := f.a.Installation.OperatorID, f.a.Installation.OwnerStaffID
	n, err := f.a.Operations.Emit(t.Context(), nil, operations.Notification{OperatorID: op, RecipientID: owner, BusinessKey: "finance-own-notice", Category: "mandatory", Severity: "info", Title: "Receipt review", Body: "Verify the original receipt"})
	if err != nil {
		t.Fatal(err)
	}
	if err = f.a.DB.Exec("UPDATE organization_staff_memberships SET role='finance' WHERE operator_id=? AND staff_id=?", op, owner).Error; err != nil {
		t.Fatal(err)
	}
	f.login(t, "staff", "owner@example.test", "owner-password-test")
	base := "/v1/operators/" + op + "/notifications"
	for _, path := range []string{base, base + "/unread-count", base + "/notices?group=personal"} {
		status, out, _ := f.call(t, "GET", path, nil, "")
		mustOK(t, status, out)
	}
	status, out, _ := f.call(t, "POST", base+"/"+n.ID+"/read", map[string]any{}, "")
	mustOK(t, status, out)
	status, _, _ = f.call(t, "GET", base+"/notices?group=finance-own-notice", nil, "")
	if status != 403 {
		t.Fatal("finance can inspect other recipients", status)
	}
	status, _, _ = f.call(t, "POST", base+"/"+n.ID+"/retry", map[string]any{}, "")
	if status != 403 {
		t.Fatal("finance can manage notification delivery", status)
	}
}
