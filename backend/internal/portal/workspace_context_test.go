package portal

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/incentives"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/operations"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"github.com/tokpath/tokstudio/backend/internal/remediation"
	"github.com/tokpath/tokstudio/backend/internal/usage"
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
	oldest := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	latest := oldest.Add(24 * time.Hour)
	var order commerce.Order
	var ownOrders []commerce.Order
	for i := 0; i < 52; i++ {
		cid := customer
		if i == 51 {
			cid = other
		}
		own, err := f.a.Organization.CurrentOwnership(ctx, nil, cid)
		if err != nil {
			t.Fatal(err)
		}
		row, err := f.a.Commerce.CreateTopUpOrder(ctx, nil, commerce.CreateTopUpInput{CustomerID: cid, OperatorID: op, BusinessKey: fmt.Sprintf("context-%d", i), ExpectedOwnershipVersion: own.Version, AmountMoney: ledger.MustDecimal("10.123456789123456789"), Validity: time.Hour})
		if err != nil {
			t.Fatal(err)
		}
		row.CreatedAt = latest
		if i == 0 || i == 50 {
			row.CreatedAt = oldest // Equal timestamps split across the two pages.
		}
		if err = f.a.DB.Exec("UPDATE commerce_orders SET created_at=? WHERE id=?", row.CreatedAt, row.ID).Error; err != nil {
			t.Fatal(err)
		}
		if cid == customer {
			ownOrders = append(ownOrders, row)
		}
		if i == 0 {
			order = row
		}
	}
	sort.Slice(ownOrders, func(i, j int) bool {
		if ownOrders[i].CreatedAt.Equal(ownOrders[j].CreatedAt) {
			return ownOrders[i].ID > ownOrders[j].ID
		}
		return ownOrders[i].CreatedAt.After(ownOrders[j].CreatedAt)
	})
	wantIDs := make([]string, len(ownOrders))
	for i := range ownOrders {
		wantIDs[i] = ownOrders[i].ID
	}
	fixtures := seedCustomerContextFilters(t, f, order, other, oldest)
	f.login(t, "staff", "owner@example.test", "owner-password-test")
	scope := "/v1/operators/" + op
	endpoint := scope + "/customer-context/" + customer
	status, out, _ := f.call(t, "GET", endpoint, nil, "")
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
	ids := func(p map[string]any) []string {
		var found []string
		for _, item := range p["items"].([]any) {
			found = append(found, item.(map[string]any)["id"].(string))
		}
		return found
	}
	if got := ids(orders); !slices.Equal(got, wantIDs[:50]) {
		t.Fatalf("first page lost newest-first order or customer scope: %v", got)
	}
	for _, item := range find(out, "assets")["items"].([]any) {
		purpose := item.(map[string]any)["values"].(map[string]any)["purpose"].(string)
		if purpose == "control:rounding:retail" {
			t.Fatal("customer assets include an internal accounting account")
		}
	}
	for _, resource := range []string{"withdrawals", "balance-withdrawals"} {
		for _, raw := range find(out, resource)["row_operations"].([]any) {
			action := raw.(map[string]any)
			prefix := "/api" + scope + "/" + resource + "/{id}/"
			if !strings.HasPrefix(action["path"].(string), prefix) {
				t.Fatalf("%s uses the wrong payout workflow: %v", resource, action["path"])
			}
		}
	}
	wantInvoiceActions := map[string][]any{"issued": {"void"}, "rejected": {}, "requested": {"register", "reject"}}
	for _, raw := range find(out, "invoices")["items"].([]any) {
		row := raw.(map[string]any)
		state := row["status"].(string)
		actions, ok := row["allowed_actions"].([]any)
		if !ok || !slices.Equal(actions, wantInvoiceActions[state]) {
			t.Fatalf("%s invoice has incorrect actions: %v", state, row["allowed_actions"])
		}
	}
	cursor := orders["next_cursor"].(string)
	status, out, _ = f.call(t, "GET", endpoint+"?section=orders&cursor="+cursor, nil, "")
	mustOK(t, status, out)
	if len(out["sections"].([]any)) != 1 || !slices.Equal(ids(find(out, "orders")), wantIDs[50:]) {
		t.Fatal("timestamp tie lost or repeated a row across pages", out)
	}
	dateFilters := url.Values{"section": {"orders"}, "q": {strings.ToUpper(wantIDs[50])}, "status": {"pending"}, "from": {oldest.Format(time.RFC3339)}, "to": {latest.Format(time.RFC3339)}}
	status, out, _ = f.call(t, "GET", endpoint+"?"+dateFilters.Encode(), nil, "")
	mustOK(t, status, out)
	if !slices.Equal(ids(find(out, "orders")), wantIDs[50:]) {
		t.Fatal("combined search did not find a record beyond the first page", out)
	}
	dateFilters.Set("q", "余额充值")
	status, out, _ = f.call(t, "GET", endpoint+"?"+dateFilters.Encode(), nil, "")
	mustOK(t, status, out)
	if !slices.Equal(ids(find(out, "orders")), wantIDs[49:]) {
		t.Fatal("date bounds did not include the start and exclude the end", out)
	}
	for _, fixture := range fixtures {
		t.Run(fixture.query.Get("section"), func(t *testing.T) {
			status, out, _ := f.call(t, "GET", endpoint+"?"+fixture.query.Encode(), nil, "")
			mustOK(t, status, out)
			if len(out["sections"].([]any)) != 1 || !slices.Equal(ids(find(out, fixture.query.Get("section"))), fixture.ids) {
				t.Fatalf("expected matching records %v, got %v", fixture.ids, out["sections"])
			}
		})
	}
	for _, query := range []string{"q=unscoped", "section=commissions&q=no", "section=orders&status=issued", "section=assets&unit=unsupported", "section=requests&to=invalid", "section=invoices&cursor=" + uuid.NewString(), "section=orders&from=2026-10-02T00:00:00Z&to=2026-10-01T00:00:00Z"} {
		if status, _, _ := f.call(t, "GET", endpoint+"?"+query, nil, ""); status != 400 {
			t.Fatalf("invalid filter accepted: %s: %d", query, status)
		}
	}
	status, out, _ = f.call(t, "GET", scope+"/customers?q=context-b", nil, "")
	mustOK(t, status, out)
	if len(out["items"].([]any)) != 1 || out["items"].([]any)[0].(map[string]any)["id"] != other {
		t.Fatal("customer search is not applied", out)
	}
	status, out, _ = f.call(t, "GET", scope+"/order-context/"+order.ID, nil, "")
	mustOK(t, status, out)
	if out["subject"].(map[string]any)["values"].(map[string]any)["due_money"] != "10.123456789123456789" {
		t.Fatal("lost decimal precision")
	}
	child, err := f.a.Organization.CreateOperator(ctx, nil, organization.CreateOperatorInput{ActorStaffID: f.a.Installation.OwnerStaffID, ParentOperatorID: op, Kind: organization.Channel, Name: "Context channel", LegalName: "Context channel legal"})
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
	status, out, _ = f.call(t, "GET", endpoint+"?section=invoices&q=doc_100", nil, "")
	mustOK(t, status, out)
	if len(out["sections"].([]any)) != 0 {
		t.Fatal("filtered request widened invoice read permission", out)
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
	if len(find(out, "payments")["operations"].([]any)) != 2 {
		t.Fatal("finance cannot allocate original receipt", out)
	}
	amount := ledger.MustDecimal("10.123456789123456789")
	receipt, err := f.a.Commerce.RecordManualReceipt(ctx, nil, f.a.Installation.OwnerStaffID, commerce.VerifiedReceipt{OperatorID: op, ProviderReceiptID: uuid.NewString(), GrossMoney: amount, FeeStatus: "pending", Currency: "USD", OriginalAmount: amount.String(), RecipientReference: "synthetic recipient", PayerReference: "synthetic payer", ValuationEvidence: "synthetic USD", VerificationEvidence: "synthetic isolated fixture"})
	if err != nil {
		t.Fatal(err)
	}
	input := map[string]any{"receipt_id": receipt.ID}
	commandKey := uuid.NewString()
	for range 2 {
		status, out, _ = f.call(t, "POST", scope+"/order-context/"+order.ID+"/allocate", input, commandKey)
		mustOK(t, status, out)
	}
	status, out, _ = f.call(t, "GET", scope+"/order-context/"+order.ID, nil, "")
	mustOK(t, status, out)
	allocations := find(out, "payments")["items"].([]any)
	if len(allocations) != 1 || allocations[0].(map[string]any)["values"].(map[string]any)["order_allocated_money"] != amount.String() {
		t.Fatal("original receipt allocation duplicated or lost precision", allocations)
	}
	status, _, _ = f.call(t, "POST", scope+"/orders/"+order.ID+"/fulfill", map[string]any{}, uuid.NewString())
	if status != 403 {
		t.Fatal("finance acquired sales mutation authority", status)
	}
}

type customerContextFilterFixture struct {
	query url.Values
	ids   []string
}

// These rows are read-only fixtures in the disposable test schema. The
// workbench test is about exact relationships and filters, not creating funds.
func seedCustomerContextFilters(t *testing.T, f *testApp, order commerce.Order, other string, at time.Time) []customerContextFilterFixture {
	t.Helper()
	db := f.a.DB
	op, customer, brand := order.OperatorID, order.CustomerID, order.BrandID
	insert := func(row any) {
		t.Helper()
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	asset := ledger.Account{ID: uuid.NewString(), OperatorID: op, OwnerID: customer, Purpose: "customer:atg:synthetic-filter", Unit: "ATG", CreatedAt: at}
	insert(&asset)
	if _, err := f.a.Ledger.EnsureAccount(t.Context(), nil, ledger.Account{OperatorID: op, OwnerID: customer, Purpose: "customer:money", Unit: ledger.MoneyUnit}); err != nil {
		t.Fatal(err)
	}
	insert(&ledger.Account{ID: uuid.NewString(), OperatorID: op, OwnerID: customer, Purpose: "control:rounding:retail", Unit: "ATG", AllowNegative: true, CreatedAt: at})

	key := usage.Key{ID: uuid.NewString(), CustomerID: customer, BrandID: brand, Kind: "api", Name: "synthetic fixture", Prefix: "synthetic", TokenHash: uuid.NewString(), Status: "active", Period: "lifetime", Timezone: "UTC", Version: 1, CreatedAt: at, UpdatedAt: at}
	insert(&key)
	request := usage.Request{ID: uuid.NewString(), CustomerID: customer, OperatorID: op, BrandID: brand, OwnershipVersion: order.OwnershipVersion, KeyID: key.ID, BusinessKey: "request-filter", Fingerprint: "synthetic", ModelID: "public-chat-filter", Operation: "chat", Status: "accepted", AcceptedAt: at, UsageDeadline: at.Add(time.Hour), WindowStart: at, SupplyHolds: json.RawMessage(`[]`), PriceSnapshot: json.RawMessage(`{}`), Candidates: json.RawMessage(`[]`)}
	insert(&request)
	oldRequest := request
	oldRequest.ID, oldRequest.BusinessKey, oldRequest.AcceptedAt = uuid.NewString(), "older-request-filter", at.Add(-time.Minute)
	insert(&oldRequest)

	amount := ledger.FromInt64(1)
	policy, err := f.a.Incentives.SetWithdrawalPolicy(t.Context(), nil, f.a.Installation.OwnerStaffID, op, incentives.WithdrawalPolicy{PayoutKind: "bank_cn", MinimumMoney: amount, MethodReference: "synthetic bank rules"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	hold := ledger.Hold{ID: uuid.NewString(), OperatorID: op, BusinessKey: "withdrawal-filter", Fingerprint: "synthetic", Purpose: "commission_withdrawal", Status: "active", ExpiresAt: at.Add(time.Hour), CreatedAt: at}
	insert(&hold)
	withdrawal := incentives.Withdrawal{ID: uuid.NewString(), OperatorID: op, CustomerID: customer, Wallet: "commission", MethodID: policy.MethodID, MethodName: policy.MethodName, PolicyID: policy.ID, BusinessKey: "withdrawal-filter", Fingerprint: "synthetic", AmountMoney: amount, NetMoney: amount, FeeRemainder: "0", RecipientReference: "synthetic recipient filter", HoldID: hold.ID, Status: "requested", CreatedAt: at}
	// Both wallets match the keyword. Order the balance UUIDs opposite to their
	// timestamps so the default section must use the application date.
	newBalance, oldBalance := withdrawal, withdrawal
	newBalance.ID, newBalance.Wallet, newBalance.BusinessKey, newBalance.CreatedAt = "00000000-0000-4000-8000-000000000001", "balance", "balance-filter-new", at.Add(time.Hour)
	oldBalance.ID, oldBalance.Wallet, oldBalance.BusinessKey, oldBalance.Status = "00000000-0000-4000-8000-000000000002", "balance", "balance-filter-old", "failed"
	if err := db.Table("incentives_withdrawals").Create([]incentives.Withdrawal{withdrawal, newBalance, oldBalance}).Error; err != nil {
		t.Fatal(err)
	}

	invoice := operations.Invoice{ID: uuid.NewString(), OperatorID: op, CustomerID: customer, PartyKind: "customer", BusinessKey: "invoice-filter", Fingerprint: "synthetic", AmountMoney: amount, State: "issued", DetailsCiphertext: "synthetic encrypted details", InvoiceReference: "Doc_100%", CreatedAt: at}
	insert(&invoice)
	decoyInvoice := invoice
	decoyInvoice.ID, decoyInvoice.BusinessKey, decoyInvoice.InvoiceReference = uuid.NewString(), "invoice-decoy", "DocX1000"
	insert(&decoyInvoice)
	for _, state := range []string{"rejected", "requested"} {
		row := invoice
		row.ID, row.BusinessKey, row.State, row.InvoiceReference = uuid.NewString(), "invoice-"+state, state, ""
		insert(&row)
	}
	otherInvoice := invoice
	otherInvoice.ID, otherInvoice.CustomerID = uuid.NewString(), other
	insert(&otherInvoice)
	if err := db.Table("operations_invoice_orders").Create(map[string]any{"invoice_id": invoice.ID, "order_id": order.ID, "amount_money": "1"}).Error; err != nil {
		t.Fatal(err)
	}

	refund := remediation.Case{ID: uuid.NewString(), OperatorID: op, CustomerID: &customer, BusinessKey: "refund-filter", Fingerprint: "synthetic", Kind: "wallet", OrderID: &order.ID, AmountMoney: amount, State: "paying", Reason: "synthetic refund reason", Evidence: "synthetic", RequestedBy: "test:fixture", CreatedAt: at}
	insert(&refund)

	return []customerContextFilterFixture{
		{url.Values{"section": {"assets"}, "q": {"customer"}, "unit": {"ATG"}}, []string{asset.ID}},
		{url.Values{"section": {"assets"}, "q": {"control:rounding:retail"}}, nil},
		{url.Values{"section": {"requests"}, "q": {"public-chat"}, "status": {"accepted"}, "from": {at.Format(time.RFC3339)}}, []string{request.ID}},
		{url.Values{"section": {"withdrawals"}, "q": {"recipient filter"}, "status": {"requested"}}, []string{withdrawal.ID}},
		{url.Values{"section": {"balance-withdrawals"}}, []string{newBalance.ID, oldBalance.ID}},
		{url.Values{"section": {"balance-withdrawals"}, "q": {"recipient filter"}, "status": {"requested"}, "from": {at.Format(time.RFC3339)}}, []string{newBalance.ID}},
		{url.Values{"section": {"invoices"}, "q": {"DOC_100%"}, "status": {"issued"}}, []string{invoice.ID}},
		{url.Values{"section": {"invoices"}, "q": {order.ID}}, []string{invoice.ID}},
		{url.Values{"section": {"refunds"}, "q": {"refund reason"}, "status": {"approved"}}, []string{refund.ID}},
	}
}

func TestPostgresExpiredGrantIsUnavailableAcrossCustomerAndOperatorViews(t *testing.T) {
	f := setupPortal(t)
	ctx := t.Context()
	op, actor := f.a.Installation.OperatorID, f.a.Installation.OwnerStaffID
	f.openRegistration(t)
	f.registered(t, "assets@audit.test")
	f.login(t, "customer", "assets@audit.test", "customer-password-test")
	status, session, _ := f.call(t, "GET", "/v1/auth/me", nil, "")
	mustOK(t, status, session)
	customer := session["customer"].(map[string]any)["id"].(string)
	var now time.Time
	if err := f.a.DB.Raw("SELECT clock_timestamp()").Row().Scan(&now); err != nil {
		t.Fatal(err)
	}
	expires := now.Add(-time.Second)
	grant, err := f.a.Ledger.IssueGrant(ctx, nil, ledger.GrantInput{OperatorID: op, CustomerID: customer, OrderID: uuid.NewString(), BusinessKey: "audit-expired-grant", ActorID: actor, Reason: "isolated expired grant before expiry worker", Sources: []ledger.GrantSource{{Kind: "manual_gift", Amount: ledger.MustDecimal("20")}}, EffectiveAt: now.Add(-time.Minute), ExpiresAt: &expires})
	if err != nil {
		t.Fatal(err)
	}
	status, credits, _ := f.call(t, "GET", "/v1/me/credits", nil, "")
	mustOK(t, status, credits)
	f.login(t, "staff", "owner@example.test", "owner-password-test")
	base := "/v1/operators/" + op
	status, workbench, _ := f.call(t, "GET", base+"/customer-context/"+customer+"?section=assets", nil, "")
	mustOK(t, status, workbench)
	status, accounts, _ := f.call(t, "GET", base+"/ledger?view=accounts", nil, "")
	mustOK(t, status, accounts)
	var staffWorkbench, staffLedger any
	for _, section := range workbench["sections"].([]any) {
		page := section.(map[string]any)["page"].(map[string]any)
		if section.(map[string]any)["resource"] != "assets" {
			continue
		}
		for _, raw := range page["items"].([]any) {
			row := raw.(map[string]any)
			if row["id"] == grant.AccountID {
				staffWorkbench = row["values"].(map[string]any)["available"]
			}
		}
	}
	for _, raw := range accounts["items"].([]any) {
		row := raw.(map[string]any)
		if row["id"] == grant.AccountID {
			staffLedger = row["values"].(map[string]any)["display"].(map[string]any)["available"]
		}
	}
	if customerAvailable := credits["customer_atg"].(map[string]any)["available"]; customerAvailable != "0" || staffWorkbench != "0" || staffLedger != "0" {
		t.Fatalf("expired grant availability: customer=%v customer context=%v operator ledger=%v", customerAvailable, staffWorkbench, staffLedger)
	}
}
