package portal

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/cooperation"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"github.com/tokpath/tokstudio/backend/internal/payments"
)

func receiptForSalesOrder(order, amount string) map[string]any {
	return map[string]any{"order_id": order, "provider_receipt_id": uuid.NewString(), "gross_usd": amount, "currency": "USD", "recipient_reference": "synthetic OEM bank", "payer_reference": "synthetic channel bank", "paid_at": time.Now().UTC(), "verification_evidence": "isolated verified bank statement"}
}

func TestPostgresWholesaleSalesInboxAndFinanceCollection(t *testing.T) {
	s := stockSetup(t)
	f, ctx := s.f, context.Background()
	status, out := s.buy(t, 1000)
	mustOK(t, status, out)
	id := out["id"].(string)
	base := "/v1/operators/" + s.oem
	// Finance has no cooperation permission and must still be able to collect
	// the supplier's sales. The buyer's workspace remains independently scoped.
	if err := f.a.DB.Exec("UPDATE organization_staff_memberships SET role='finance' WHERE staff_id=? AND operator_id=?", f.a.Installation.OwnerStaffID, s.oem).Error; err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{base + "/orders", base + "/orders?q=" + url.QueryEscape("OEM3-1 channel") + "&status=awaiting_payment"} {
		status, out, _ = f.call(t, "GET", path, nil, "")
		mustOK(t, status, out)
		rows := out["items"].([]any)
		if len(rows) != 1 || rows[0].(map[string]any)["id"] != id {
			t.Fatal("wholesale order absent", out)
		}
		v := rows[0].(map[string]any)["values"].(map[string]any)
		if v["kind"] != "supply" || v["counterparty_name"] != "OEM3-1 channel" {
			t.Fatal("unreadable buyer", v)
		}
	}
	status, out, _ = f.call(t, "GET", base+"/references/orders?purpose=collection&q=OEM3-1", nil, "")
	mustOK(t, status, out)
	choices := out["items"].([]any)
	if len(choices) != 1 || choices[0].(map[string]any)["value"] != id || !strings.Contains(choices[0].(map[string]any)["label"].(string), "1000 USD") {
		t.Fatal("missing collection choice", out)
	}
	status, out, _ = f.call(t, "GET", base+"/references/orders", nil, "")
	mustOK(t, status, out)
	if len(out["items"].([]any)) != 0 {
		t.Fatal("wholesale leaked into retail refund references", out)
	}
	status, out, _ = f.call(t, "GET", base+"/order-context/"+id, nil, "")
	mustOK(t, status, out)
	if out["subject"].(map[string]any)["id"] != id || len(out["sections"].([]any)) != 1 {
		t.Fatal("finance cannot open original supply order", out)
	}
	for _, operator := range []string{f.a.Installation.OperatorID, s.channel} {
		status, out, _ = f.call(t, "GET", "/v1/operators/"+operator+"/orders?q="+id, nil, "")
		mustOK(t, status, out)
		if len(out["items"].([]any)) != 0 {
			t.Fatal("sales scope leaked", operator, out)
		}
		status, _, _ = f.call(t, "GET", "/v1/operators/"+operator+"/order-context/"+id, nil, "")
		if status == 200 {
			t.Fatal("foreign sales context exposed", operator)
		}
	}
	input, key := receiptForSalesOrder(id, "1000"), uuid.NewString()
	status, out, _ = f.call(t, "POST", base+"/payments", input, key)
	mustOK(t, status, out)
	receiptID := out["id"].(string)
	status, again, _ := f.call(t, "POST", base+"/payments", input, key)
	mustOK(t, status, again)
	if again["id"] != receiptID {
		t.Fatal("receipt duplicated", again)
	}
	// Retrying association or delivery uses the same original order and lot.
	for _, path := range []string{base + "/payments/" + receiptID + "/allocate", base + "/order-context/" + id + "/allocate", base + "/orders/" + id + "/fulfill_supply"} {
		body := map[string]any{}
		if strings.HasSuffix(path, "/fulfill_supply") {
			// Delivery has no payment inputs.
		} else if strings.Contains(path, "/order-context/") {
			body["receipt_id"] = receiptID
		} else {
			body["order_id"] = id
		}
		status, out, _ = f.call(t, "POST", path, body, uuid.NewString())
		mustOK(t, status, out)
	}
	var order cooperation.PurchaseOrder
	if err := f.a.DB.First(&order, "id=?", id).Error; err != nil || order.Status != "completed" || order.AllocatedUSD.String() != "1000" {
		t.Fatal("original purchase not delivered", order, err)
	}
	position, err := f.a.Cooperation.Position(ctx, nil, s.channel)
	if err != nil || position.Quantity.String() != "1000" || position.Cost.String() != "1000" {
		t.Fatal("incorrect stock delivery", position, err)
	}
	for table, want := range map[string]int64{"cooperation_purchase_allocations": 1, "cooperation_procurement_lots": 1, "commerce_orders": 0} {
		var n int64
		column := "order_id"
		if table == "commerce_orders" {
			column = "id"
		}
		if err := f.a.DB.Table(table).Where(column+"=?", id).Count(&n).Error; err != nil || n != want {
			t.Fatal("duplicate sales facts", table, n, err)
		}
	}
	status, out, _ = f.call(t, "GET", base+"/order-context/"+id, nil, "")
	mustOK(t, status, out)
	section := out["sections"].([]any)[0].(map[string]any)["page"].(map[string]any)
	if len(section["items"].([]any)) != 1 {
		t.Fatal("paid order lacks receipt", out)
	}
	status, out, _ = f.call(t, "GET", base+"/references/orders?purpose=collection&q="+id, nil, "")
	mustOK(t, status, out)
	if len(out["items"].([]any)) != 0 {
		t.Fatal("paid order still offered for collection", out)
	}
}

func TestPostgresWholesaleCollectionRetainsPaidOrderWhenDeliveryNeedsRetry(t *testing.T) {
	s := stockSetup(t)
	status, out := s.buy(t, 1000)
	mustOK(t, status, out)
	id := out["id"].(string)
	if err := s.f.a.DB.Exec("UPDATE organization_supply_agreements SET status='paused' WHERE buyer_id=?", s.channel).Error; err != nil {
		t.Fatal(err)
	}
	base := "/v1/operators/" + s.oem
	status, out, _ = s.f.call(t, "POST", base+"/payments", receiptForSalesOrder(id, "1000"), uuid.NewString())
	mustOK(t, status, out)
	if !strings.Contains(fmt.Sprint(out["message"]), "待交付") {
		t.Fatal("delivery failure hidden", out)
	}
	receipt, err := s.f.a.Commerce.GetReceipt(context.Background(), nil, s.oem, out["id"].(string))
	if err != nil || receipt.AllocatedUSD.String() != "1000" {
		t.Fatal("receipt allocation rolled back", receipt, err)
	}
	var order cooperation.PurchaseOrder
	if err = s.f.a.DB.First(&order, "id=?", id).Error; err != nil || order.Status != "paid" {
		t.Fatal("paid order lost", order, err)
	}
	if err = s.f.a.DB.Exec("UPDATE organization_supply_agreements SET status='active' WHERE buyer_id=?", s.channel).Error; err != nil {
		t.Fatal(err)
	}
	status, out, _ = s.f.call(t, "POST", base+"/orders/"+id+"/fulfill_supply", map[string]any{}, uuid.NewString())
	mustOK(t, status, out)
	if out["status"] != "completed" {
		t.Fatal("cannot retry original delivery", out)
	}
}

func TestPostgresWholesaleManualCollectionPreservesUnmatchedFunds(t *testing.T) {
	s := stockSetup(t)
	f, ctx := s.f, context.Background()
	base := "/v1/operators/" + s.oem
	status, out := s.buy(t, 1000)
	mustOK(t, status, out)
	id := out["id"].(string)
	status, out, _ = f.call(t, "POST", base+"/payments", receiptForSalesOrder(id, "999"), uuid.NewString())
	mustOK(t, status, out)
	shortID := out["id"].(string)
	if !strings.Contains(fmt.Sprint(out["message"]), "尚未关联") {
		t.Fatal("mismatch hidden", out)
	}
	receipt, err := f.a.Commerce.GetReceipt(ctx, nil, s.oem, shortID)
	if err != nil || !receipt.AllocatedUSD.IsZero() || receipt.GrossUSD.String() != "999" {
		t.Fatal("real receipt lost", receipt, err)
	}
	status, _, _ = f.call(t, "POST", base+"/payments/"+shortID+"/allocate", map[string]any{"order_id": id}, uuid.NewString())
	if status != 409 {
		t.Fatal("partial payment accepted", status)
	}
	// A separately recorded receipt can be matched later, without a new order.
	status, out, _ = f.call(t, "POST", base+"/payments", receiptForSalesOrder("", "1000"), uuid.NewString())
	mustOK(t, status, out)
	status, out, _ = f.call(t, "POST", base+"/payments/"+out["id"].(string)+"/allocate", map[string]any{"order_id": id}, uuid.NewString())
	mustOK(t, status, out)
	status, out = s.buy(t, 100)
	mustOK(t, status, out)
	unsafe := out["id"].(string)
	merchantID := uuid.NewString()
	_, err = f.a.Payments.SaveMerchant(ctx, f.a.Installation.OwnerStaffID, payments.Merchant{ID: merchantID, OperatorID: s.oem, Driver: "stripe", Enabled: true, Credentials: map[string]string{"secret_key": "sk_test_fixture", "publishable_key": "pk_test_fixture", "webhook_secret": "whsec_fixture", "currency": "USD"}}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.a.DB.Create(&procurementCheckout{OrderID: unsafe, BuyerID: s.channel, ActorID: f.a.Installation.OwnerStaffID, MerchantID: merchantID, PaymentOrder: json.RawMessage(`{}`), CreatedAt: time.Now()}).Error; err != nil {
		t.Fatal(err)
	}
	status, out, _ = f.call(t, "POST", base+"/payments", receiptForSalesOrder(unsafe, "100"), uuid.NewString())
	mustOK(t, status, out)
	receipt, err = f.a.Commerce.GetReceipt(ctx, nil, s.oem, out["id"].(string))
	if err != nil || !receipt.AllocatedUSD.IsZero() || !strings.Contains(fmt.Sprint(out["message"]), "尚未关联") {
		t.Fatal("online payment bypassed", out, receipt, err)
	}
}

func TestPostgresSalesInboxFiltersAndPagesAcrossBothOrderTypes(t *testing.T) {
	f := setupPortal(t)
	f.openRegistration(t)
	f.registered(t, "sales-page@example.test")
	f.login(t, "customer", "sales-page@example.test", "customer-password-test")
	status, me, _ := f.call(t, "GET", "/v1/auth/me", nil, "")
	mustOK(t, status, me)
	customer := me["customer"].(map[string]any)["id"].(string)
	op, owner := f.a.Installation.OperatorID, f.a.Installation.OwnerStaffID
	if err := f.a.DB.Exec("UPDATE organization_operator_statuses SET new_purchase=true WHERE operator_id=?", op).Error; err != nil {
		t.Fatal(err)
	}
	child, err := f.a.Organization.CreateOperator(context.Background(), nil, organization.CreateOperatorInput{ActorStaffID: owner, ParentOperatorID: op, Kind: organization.Channel, Name: "Paged supply buyer"})
	if err != nil {
		t.Fatal(err)
	}
	quote, err := f.a.Cooperation.PublishPurchaseQuote(context.Background(), nil, owner, cooperation.PurchaseQuote{SupplierID: op, BuyerID: child.Operator.ID, GiftTiers: []cooperation.PurchaseGiftTier{{}}, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	for i := range 60 {
		order, err := f.a.Commerce.CreateTopUpOrder(context.Background(), nil, commerce.CreateTopUpInput{OperatorID: op, CustomerID: customer, BusinessKey: uuid.NewString(), ExpectedOwnershipVersion: 1, AmountUSD: ledger.FromInt64(1)})
		if err != nil {
			t.Fatal(err)
		}
		if err = f.a.DB.Table("commerce_orders").Where("id=?", order.ID).Update("created_at", now.Add(-time.Duration(i*2)*time.Minute)).Error; err != nil {
			t.Fatal(err)
		}
		order2 := cooperation.PurchaseOrder{ID: uuid.NewString(), SupplierID: op, BuyerID: child.Operator.ID, QuoteID: quote.ID, BusinessKey: uuid.NewString(), Fingerprint: "synthetic", Name: "Paged order", QuoteVersion: 1, DueUSD: ledger.FromInt64(1), PaidATG: ledger.FromInt64(1), ExitTerms: json.RawMessage(`{}`), TermsReference: "synthetic", Status: "awaiting_payment", CreatedAt: now.Add(-time.Duration(i*2+1) * time.Minute), ExpiresAt: now.Add(time.Hour)}
		if i == 59 {
			order2.Name = "needle oldest supply order"
		}
		if err = f.a.DB.Create(&order2).Error; err != nil {
			t.Fatal(err)
		}
	}
	f.login(t, "staff", "owner@example.test", "owner-password-test")
	for _, resource := range []string{"orders", "references/orders?purpose=collection"} {
		seen := map[string]bool{}
		cursor := ""
		for {
			sep := "?"
			if strings.Contains(resource, "?") {
				sep = "&"
			}
			status, out, _ := f.call(t, "GET", "/v1/operators/"+op+"/"+resource+sep+"cursor="+url.QueryEscape(cursor), nil, "")
			mustOK(t, status, out)
			for _, raw := range out["items"].([]any) {
				row := raw.(map[string]any)
				key, _ := row["id"].(string)
				if key == "" {
					key = row["value"].(string)
				}
				if seen[key] {
					t.Fatal("duplicate page", resource, key)
				}
				seen[key] = true
			}
			cursor, _ = out["next_cursor"].(string)
			if cursor == "" {
				break
			}
		}
		if len(seen) != 120 {
			t.Fatal("combined pagination truncated", resource, len(seen))
		}
	}
	status, out, _ := f.call(t, "GET", "/v1/operators/"+op+"/orders?q=needle&status=awaiting_payment", nil, "")
	mustOK(t, status, out)
	if len(out["items"].([]any)) != 1 {
		t.Fatal("search after pagination", out)
	}
}
