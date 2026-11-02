package portal

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/operations"
)

func TestFinanceCanReadOwnNotificationsWithoutManagingOtherRecipients(t *testing.T) {
	f := setupPortal(t)
	op, owner := f.a.Installation.OperatorID, f.a.Installation.OwnerStaffID
	n, err := f.a.Operations.Emit(t.Context(), nil, operations.Notification{OperatorID: op, RecipientID: owner, BusinessKey: "finance-own-notice", Category: "mandatory", Severity: "info", Title: "Receipt review", Body: "Verify the original receipt"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := f.a.Operations.Emit(t.Context(), nil, operations.Notification{OperatorID: op, RecipientID: owner, BusinessKey: "finance-own-notice-2", Category: "mandatory", Severity: "info", Title: "Second review", Body: "Still unread"})
	if err != nil {
		t.Fatal(err)
	}
	otherID := uuid.NewString()
	if err = f.a.DB.Create(&operations.Notification{ID: otherID, OperatorID: op, RecipientID: uuid.NewString(), BusinessKey: "finance-other-person", Category: "mandatory", Severity: "info", Title: "Someone else", Body: "Leave unread", Reference: "order", Fingerprint: "synthetic", CreatedAt: time.Now()}).Error; err != nil {
		t.Fatal(err)
	}
	f.login(t, "staff", "owner@example.test", "owner-password-test")
	base := "/v1/operators/" + op + "/notifications"
	status, page, _ := f.call(t, "GET", base, nil, "")
	mustOK(t, status, page)
	if !operationNamed(t, page, "announce") || !operationNamed(t, page, "read_all") {
		t.Fatal(page["operations"])
	}
	if err = f.a.DB.Exec("UPDATE organization_staff_memberships SET role='finance' WHERE operator_id=? AND staff_id=?", op, owner).Error; err != nil {
		t.Fatal(err)
	}
	status, page, _ = f.call(t, "GET", base, nil, "")
	mustOK(t, status, page)
	if operationNamed(t, page, "announce") || !operationNamed(t, page, "read_all") {
		t.Fatal(page["operations"])
	}
	for _, path := range []string{base + "/unread-count", base + "/notices?group=personal"} {
		status, out, _ := f.call(t, "GET", path, nil, "")
		mustOK(t, status, out)
	}
	status, out, _ := f.call(t, "POST", base+"/read-all", map[string]any{}, "")
	mustOK(t, status, out)
	if out["count"] != float64(2) {
		t.Fatal(out)
	}
	var firstNotice, secondNotice, other operations.Notification
	if err = f.a.DB.First(&firstNotice, "id=?", n.ID).Error; err != nil || firstNotice.ReadAt == nil {
		t.Fatal(firstNotice.ReadAt, err)
	}
	if err = f.a.DB.First(&secondNotice, "id=?", second.ID).Error; err != nil || secondNotice.ReadAt == nil {
		t.Fatal(secondNotice.ReadAt, err)
	}
	if err = f.a.DB.First(&other, "id=?", otherID).Error; err != nil || other.ReadAt != nil {
		t.Fatal("finance marked another recipient read", other.ReadAt, err)
	}
	status, out, _ = f.call(t, "GET", base+"/unread-count", nil, "")
	mustOK(t, status, out)
	if out["count"] != float64(0) {
		t.Fatal(out)
	}
	status, out, _ = f.call(t, "POST", base+"/"+n.ID+"/read", map[string]any{}, "")
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

func TestCustomerCanMarkEveryOwnNotificationRead(t *testing.T) {
	f := setupPortal(t)
	f.openRegistration(t)
	f.registered(t, "notices@example.test")
	f.login(t, "customer", "notices@example.test", "customer-password-test")
	status, session, _ := f.call(t, "GET", "/v1/auth/me", nil, "")
	mustOK(t, status, session)
	customerID := session["customer"].(map[string]any)["id"].(string)
	operatorID := session["operator"].(map[string]any)["id"].(string)
	first, err := f.a.Operations.Emit(t.Context(), nil, operations.Notification{OperatorID: operatorID, RecipientID: customerID, BusinessKey: "customer-read-all:1", Category: "mandatory", Severity: "info", Title: "First", Body: "One", Reference: "order"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := f.a.Operations.Emit(t.Context(), nil, operations.Notification{OperatorID: operatorID, RecipientID: customerID, BusinessKey: "customer-read-all:2", Category: "mandatory", Severity: "info", Title: "Second", Body: "Two", Reference: "order"})
	if err != nil {
		t.Fatal(err)
	}
	otherID := uuid.NewString()
	if err = f.a.DB.Create(&operations.Notification{ID: otherID, OperatorID: operatorID, RecipientID: uuid.NewString(), BusinessKey: "customer-read-all:other", Category: "mandatory", Severity: "info", Title: "Other", Body: "Leave unread", Reference: "order", Fingerprint: "synthetic", CreatedAt: time.Now()}).Error; err != nil {
		t.Fatal(err)
	}
	status, out, _ := f.call(t, "POST", "/v1/me/notifications/read-all", map[string]any{}, "")
	mustOK(t, status, out)
	if out["count"] != float64(2) {
		t.Fatal(out)
	}
	var firstNotice, secondNotice, other operations.Notification
	if err = f.a.DB.First(&firstNotice, "id=?", first.ID).Error; err != nil || firstNotice.ReadAt == nil {
		t.Fatal(firstNotice.ReadAt, err)
	}
	if err = f.a.DB.First(&secondNotice, "id=?", second.ID).Error; err != nil || secondNotice.ReadAt == nil {
		t.Fatal(secondNotice.ReadAt, err)
	}
	if err = f.a.DB.First(&other, "id=?", otherID).Error; err != nil || other.ReadAt != nil {
		t.Fatal("customer marked another recipient read", other.ReadAt, err)
	}
}

func operationNamed(t *testing.T, page map[string]any, name string) bool {
	t.Helper()
	ops, _ := page["operations"].([]any)
	for _, item := range ops {
		op, _ := item.(map[string]any)
		if op["name"] != name {
			continue
		}
		if name == "read_all" && (!strings.HasSuffix(op["path"].(string), "/notifications/read-all") || op["permission"] != "") {
			t.Fatal(op["path"], op["permission"])
		}
		return true
	}
	return false
}
