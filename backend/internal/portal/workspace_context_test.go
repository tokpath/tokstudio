package portal

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/organization"
)

func TestPostgresStaffEditorPreservesCurrentCustomPermissions(t *testing.T) {
	f := setupPortal(t)
	f.login(t, "staff", "owner@example.test", "owner-password-test")
	op, actor := f.a.Installation.OperatorID, f.a.Installation.OwnerStaffID
	var membership string
	if err := f.a.DB.Table("organization_staff_memberships").Select("id").Where("staff_id=? AND operator_id=?", actor, op).Scan(&membership).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.a.DB.Exec("UPDATE organization_staff_memberships SET role='custom' WHERE id=?", membership).Error; err != nil {
		t.Fatal(err)
	}
	for _, grant := range []string{"staff.read", "customers.read"} {
		if err := f.a.DB.Exec("INSERT INTO organization_staff_grants(membership_id,action) VALUES (?,?)", membership, grant).Error; err != nil {
			t.Fatal(err)
		}
	}
	status, page, _ := f.call(t, "GET", "/v1/operators/"+op+"/staff", nil, "")
	mustOK(t, status, page)
	items := page["items"].([]any)
	if len(items) != 1 {
		t.Fatal("unexpected membership projection", items)
	}
	row := items[0].(map[string]any)["values"].(map[string]any)
	if row["role"] != "custom" || row["grants"] != "customers.read,staff.read" {
		t.Fatal("opening the editor must preserve existing custom grants", row)
	}
}

func TestPostgresWorkspaceContextScopePaginationAndPermissions(t *testing.T) {
	f := setupPortal(t)
	f.openRegistration(t)
	f.registered(t, "context-a@example.test")
	f.registered(t, "context-b@example.test")
	f.login(t, "customer", "context-a@example.test", "customer-password-test")
	status, me, _ := f.call(t, "GET", "/v1/auth/me", nil, "")
	mustOK(t, status, me)
	customer := me["customer"].(map[string]any)["id"].(string)
	f.login(t, "customer", "context-b@example.test", "customer-password-test")
	status, me, _ = f.call(t, "GET", "/v1/auth/me", nil, "")
	mustOK(t, status, me)
	other := me["customer"].(map[string]any)["id"].(string)
	ctx := context.Background()
	op := f.a.Installation.OperatorID
	if err := f.a.DB.Exec("UPDATE organization_operator_statuses SET new_purchase=true WHERE operator_id=?", op).Error; err != nil {
		t.Fatal(err)
	}
	var order commerce.Order
	for i := 0; i < 52; i++ {
		cid := customer
		if i == 51 {
			cid = other
		}
		own, err := f.a.Organization.CurrentOwnership(ctx, nil, cid)
		if err != nil {
			t.Fatal(err)
		}
		row, err := f.a.Commerce.CreateTopUpOrder(ctx, nil, commerce.CreateTopUpInput{CustomerID: cid, OperatorID: op, BusinessKey: fmt.Sprintf("context-%d", i), ExpectedOwnershipVersion: own.Version, AmountUSD: ledger.MustDecimal("10.123456789123456789"), Validity: time.Hour})
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			order = row
		}
	}
	f.login(t, "staff", "owner@example.test", "owner-password-test")
	scope := "/v1/operators/" + op
	status, out, _ := f.call(t, "GET", scope+"/customer-context/"+customer, nil, "")
	mustOK(t, status, out)
	find := func(out map[string]any, resource string) map[string]any {
		t.Helper()
		for _, s := range out["sections"].([]any) {
			section := s.(map[string]any)
			if section["resource"] == resource {
				return section["page"].(map[string]any)
			}
		}
		t.Fatalf("missing section %s", resource)
		return nil
	}
	orders := find(out, "orders")
	items := orders["items"].([]any)
	if len(items) != 50 {
		t.Fatal("unbounded or short page", len(items))
	}
	for _, item := range items {
		if item.(map[string]any)["values"].(map[string]any)["customer_id"] != customer {
			t.Fatal("other customer record leaked")
		}
	}
	cursor := orders["next_cursor"].(string)
	status, out, _ = f.call(t, "GET", scope+"/customer-context/"+customer+"?section=orders&cursor="+cursor, nil, "")
	mustOK(t, status, out)
	if len(out["sections"].([]any)) != 1 || len(find(out, "orders")["items"].([]any)) != 1 {
		t.Fatal("section pagination lost exact scope", out)
	}
	status, out, _ = f.call(t, "GET", scope+"/customers?q=context-b", nil, "")
	mustOK(t, status, out)
	if len(out["items"].([]any)) != 1 || out["items"].([]any)[0].(map[string]any)["id"] != other {
		t.Fatal("customer search is not applied", out)
	}
	status, out, _ = f.call(t, "GET", scope+"/order-context/"+order.ID, nil, "")
	mustOK(t, status, out)
	if out["subject"].(map[string]any)["values"].(map[string]any)["due_usd"] != "10.123456789123456789" {
		t.Fatal("lost decimal precision")
	}
	child, err := f.a.Organization.CreateOperator(ctx, nil, organization.CreateOperatorInput{ActorStaffID: f.a.Installation.OwnerStaffID, ParentOperatorID: op, Kind: organization.Channel, Name: "Context channel", LegalName: "Context channel legal", QuoteReference: "terms", ExitTermsReference: "exit"})
	if err != nil {
		t.Fatal(err)
	}
	status, _, _ = f.call(t, "GET", "/v1/operators/"+child.Operator.ID+"/customer-context/"+customer, nil, "")
	if status == 200 {
		t.Fatal("parent implicitly entered child scope")
	}
	status, _, _ = f.call(t, "GET", scope+"/customer-context/"+uuid.NewString(), nil, "")
	if status == 200 {
		t.Fatal("unknown customer accepted")
	}
	// Restrict the existing member directly in this isolated fixture to verify
	// projection authorization independently of UI navigation and role editing.
	if err = f.a.DB.Exec("UPDATE organization_staff_memberships SET role='custom' WHERE staff_id=? AND operator_id=?", f.a.Installation.OwnerStaffID, op).Error; err != nil {
		t.Fatal(err)
	}
	var membership string
	if err = f.a.DB.Table("organization_staff_memberships").Select("id").Where("staff_id=? AND operator_id=?", f.a.Installation.OwnerStaffID, op).Scan(&membership).Error; err != nil {
		t.Fatal(err)
	}
	if err = f.a.DB.Exec("INSERT INTO organization_staff_grants(membership_id,action) VALUES (?,?)", membership, "customers.read").Error; err != nil {
		t.Fatal(err)
	}
	status, out, _ = f.call(t, "GET", scope+"/customer-context/"+customer, nil, "")
	mustOK(t, status, out)
	for _, s := range out["sections"].([]any) {
		if s.(map[string]any)["resource"] != "requests" {
			t.Fatal("finance section leaked to support", s)
		}
	}
	status, _, _ = f.call(t, "GET", scope+"/order-context/"+order.ID, nil, "")
	if status == 200 {
		t.Fatal("read-only customer permission opened financial order")
	}
	if err = f.a.DB.Exec("INSERT INTO organization_staff_grants(membership_id,action) VALUES (?,?)", membership, "payments.manage").Error; err != nil {
		t.Fatal(err)
	}
	status, out, _ = f.call(t, "GET", scope+"/customer-context/"+customer, nil, "")
	mustOK(t, status, out)
	if len(find(out, "orders")["items"].([]any)) != 50 {
		t.Fatal("finance read scope differs between customer context and order list", out)
	}
	status, out, _ = f.call(t, "GET", scope+"/order-context/"+order.ID, nil, "")
	mustOK(t, status, out)
	if actions, ok := out["subject_operations"].([]any); ok && len(actions) != 0 {
		t.Fatal("finance received sales delivery action", actions)
	}
	if len(find(out, "payments")["operations"].([]any)) != 1 {
		t.Fatal("finance cannot allocate original receipt", out)
	}
	amount := ledger.MustDecimal("10.123456789123456789")
	receipt, err := f.a.Commerce.RecordManualReceipt(ctx, nil, f.a.Installation.OwnerStaffID, commerce.VerifiedReceipt{OperatorID: op, ProviderReceiptID: uuid.NewString(), GrossUSD: amount, FeeStatus: "pending", Currency: "USD", OriginalAmount: amount.String(), RecipientReference: "synthetic recipient", PayerReference: "synthetic payer", ValuationEvidence: "synthetic USD", VerificationEvidence: "synthetic isolated fixture"})
	if err != nil {
		t.Fatal(err)
	}
	input := map[string]any{"receipt_id": receipt.ID, "amount_usd": amount.String()}
	commandKey := uuid.NewString()
	for range 2 {
		status, out, _ = f.call(t, "POST", scope+"/order-context/"+order.ID+"/allocate", input, commandKey)
		mustOK(t, status, out)
	}
	status, out, _ = f.call(t, "GET", scope+"/order-context/"+order.ID, nil, "")
	mustOK(t, status, out)
	allocations := find(out, "payments")["items"].([]any)
	if len(allocations) != 1 || allocations[0].(map[string]any)["values"].(map[string]any)["order_allocated_usd"] != amount.String() {
		t.Fatal("original receipt allocation duplicated or lost precision", allocations)
	}
	status, _, _ = f.call(t, "POST", scope+"/orders/"+order.ID+"/fulfill", map[string]any{}, uuid.NewString())
	if status != 403 {
		t.Fatal("finance acquired sales mutation authority", status)
	}
}
