package portal

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/cooperation"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"net/url"
	"testing"
	"time"
)

func TestPostgresProcurementFiltersEntireScopeBeforePaging(t *testing.T) {
	f := setupPortal(t)
	ctx := context.Background()
	op, owner := f.a.Installation.OperatorID, f.a.Installation.OwnerStaffID
	child, e := f.a.Organization.CreateOperator(ctx, nil, organization.CreateOperatorInput{ActorStaffID: owner, ParentOperatorID: op, Kind: organization.Channel, Name: "采购方名字"})
	if e != nil {
		t.Fatal(e)
	}
	now := time.Now().UTC()
	target := ""
	for i := 0; i < 112; i++ {
		q := cooperation.PurchaseQuote{ID: uuid.NewString(), SupplierID: op, BuyerID: child.Operator.ID, Name: fmt.Sprintf("ordinary %d", i), GiftTiers: []cooperation.PurchaseGiftTier{{}}, ExitTerms: json.RawMessage(`{}`), TermsReference: "synthetic", CreatedBy: owner, CreatedAt: now.Add(-time.Duration(i) * time.Minute), Version: int64(i + 1), Enabled: true}
		if i == 110 {
			q.Name = "needle purchase quote"
			q.Enabled = false
			target = q.ID
		}
		if i == 111 {
			q.SupplierID = child.Operator.ID
			q.Name = "needle purchase quote"
			q.Enabled = false
		}
		if e = f.a.DB.Create(&q).Error; e != nil {
			t.Fatal(e)
		}
	}
	f.login(t, "staff", "owner@example.test", "owner-password-test")
	path := "/v1/operators/" + op + "/procurement?view=supplier&section=quotes"
	status, out, _ := f.call(t, "GET", path, nil, "")
	mustOK(t, status, out)
	if len(out["items"].([]any)) != 50 || out["next_cursor"] == "" || out["search_supported"] != true || out["status_supported"] != true {
		t.Fatal(out)
	}
	seen := map[string]bool{}
	for {
		for _, item := range out["items"].([]any) {
			id := item.(map[string]any)["id"].(string)
			if seen[id] {
				t.Fatal("duplicate page record", id)
			}
			seen[id] = true
		}
		cursor, _ := out["next_cursor"].(string)
		if cursor == "" {
			break
		}
		status, out, _ = f.call(t, "GET", path+"&cursor="+url.QueryEscape(cursor), nil, "")
		mustOK(t, status, out)
	}
	if len(seen) != 111 {
		t.Fatal("missing or foreign page", len(seen))
	}
	status, out, _ = f.call(t, "GET", path+"&q=needle&status=disabled", nil, "")
	mustOK(t, status, out)
	rows := out["items"].([]any)
	if len(rows) != 1 || rows[0].(map[string]any)["id"] != target {
		t.Fatal("search truncated or leaked", out)
	}
	status, out, _ = f.call(t, "GET", path+"&q="+url.QueryEscape("采购方名字")+"&status=disabled", nil, "")
	mustOK(t, status, out)
	if len(out["items"].([]any)) != 1 {
		t.Fatal("party name search failed", out)
	}
	status, out, _ = f.call(t, "GET", path+"&status=quote", nil, "")
	mustOK(t, status, out)
	if len(out["items"].([]any)) != 50 {
		t.Fatal("active status filter", out)
	}
	// The unfiltered summary covers all open orders, independently of the page.
	for i := 0; i < 52; i++ {
		o := cooperation.PurchaseOrder{ID: uuid.NewString(), SupplierID: op, BuyerID: child.Operator.ID, QuoteID: target, BusinessKey: fmt.Sprintf("summary-%d", i), Fingerprint: "synthetic", DueMoney: ledger.FromInt64(1), PaidATG: ledger.FromInt64(1), ExitTerms: json.RawMessage(`{}`), TermsReference: "synthetic", Name: "summary order", QuoteVersion: 1, Status: "awaiting_payment", ExpiresAt: now.Add(time.Hour), CreatedAt: now}
		if e = f.a.DB.Create(&o).Error; e != nil {
			t.Fatal(e)
		}
	}
	status, out, _ = f.call(t, "GET", "/v1/operators/"+op+"/procurement?view=supplier&section=orders&q=not-found", nil, "")
	mustOK(t, status, out)
	if len(out["items"].([]any)) != 0 || out["summary"].([]any)[0].(map[string]any)["value"] != "52" {
		t.Fatal("summary depends on filtered page", out)
	}
}

func TestPostgresProcurementOfflineReceiptCreatesAndDeliversOneOrder(t *testing.T) {
	f := setupPortal(t)
	ctx := context.Background()
	parent, owner := f.a.Installation.OperatorID, f.a.Installation.OwnerStaffID
	child, e := f.a.Organization.CreateOperator(ctx, nil, organization.CreateOperatorInput{ActorStaffID: owner, ParentOperatorID: parent, Kind: organization.Channel, Name: "offline buyer"})
	if e != nil {
		t.Fatal(e)
	}
	q, e := f.a.Cooperation.PublishPurchaseQuote(ctx, nil, owner, cooperation.PurchaseQuote{SupplierID: parent, BuyerID: child.Operator.ID, GiftTiers: []cooperation.PurchaseGiftTier{{}, {FromMoney: ledger.FromInt64(1000), GiftPercent: ledger.FromInt64(1)}, {FromMoney: ledger.FromInt64(2000), GiftPercent: ledger.FromInt64(2)}}, Enabled: true})
	if e != nil {
		t.Fatal(e)
	}
	now := time.Now().UTC()
	receipt, e := f.a.Commerce.RecordManualReceipt(ctx, nil, owner, commerce.VerifiedReceipt{OperatorID: parent, ProviderReceiptID: "offline-purchase-bank", GrossMoney: ledger.FromInt64(2000), FeeStatus: "pending", Currency: "USD", OriginalAmount: "2000", RecipientReference: "supplier bank", PayerReference: "buyer bank", ValuationEvidence: "USD", VerificationEvidence: "synthetic transfer verified independently", PaidAt: &now, ObservedAt: now})
	if e != nil {
		t.Fatal(e)
	}
	f.login(t, "staff", "owner@example.test", "owner-password-test")
	path := "/v1/operators/" + parent + "/procurement/" + q.ID + "/offline_purchase"
	status, out, _ := f.call(t, "POST", path, map[string]any{"receipt_id": receipt.ID}, uuid.NewString())
	mustOK(t, status, out)
	if out["status"] != "completed" {
		t.Fatal("offline flow still needs a separate delivery step", out)
	}
	status, again, _ := f.call(t, "POST", path, map[string]any{"receipt_id": receipt.ID}, uuid.NewString())
	mustOK(t, status, again)
	if out["id"] != again["id"] {
		t.Fatal("one payment created multiple orders", out, again)
	}
	position, e := f.a.Cooperation.Position(ctx, nil, child.Operator.ID)
	if e != nil || position.Quantity.String() != "2010" || position.Cost.String() != "2000" {
		t.Fatal(position, e)
	}
	status, page, _ := f.call(t, "GET", "/v1/operators/"+parent+"/procurement?view=supplier&section=quotes", nil, "")
	mustOK(t, status, page)
	found := false
	for _, value := range page["row_operations"].([]any) {
		operation := value.(map[string]any)
		if operation["name"] == "fund" {
			fields := operation["fields"].([]any)
			if len(fields) != 1 || fields[0].(map[string]any)["name"] != "receipt_id" {
				t.Fatal("partial allocation still exposed", operation)
			}
		}
		if operation["name"] == "offline_purchase" {
			found = true
		}
	}
	if !found {
		t.Fatal("offline purchase missing from authorized form")
	}
	status, whole, _ := f.call(t, "GET", "/v1/operators/"+parent+"/references/receipts?whole=true", nil, "")
	mustOK(t, status, whole)
	if len(whole["items"].([]any)) != 0 {
		t.Fatal("used payments still offered for procurement", whole)
	}
	status, all, _ := f.call(t, "GET", "/v1/operators/"+parent+"/references/receipts", nil, "")
	mustOK(t, status, all)
	if len(all["items"].([]any)) != 1 {
		t.Fatal("normal receipt lookup was changed", all)
	}
}
