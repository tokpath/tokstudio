package portal

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/cooperation"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"github.com/tokpath/tokstudio/backend/internal/payments"
)

func TestPostgresProcurementCancellationKeepsUncertainOnlinePaymentReserved(t *testing.T) {
	f := setupPortal(t)
	ctx := context.Background()
	parent, owner := f.a.Installation.OperatorID, f.a.Installation.OwnerStaffID
	child, err := f.a.Organization.CreateOperator(ctx, nil, organization.CreateOperatorInput{ActorStaffID: owner, ParentOperatorID: parent, Kind: organization.Channel, Name: "cancellation buyer"})
	if err != nil {
		t.Fatal(err)
	}
	invite, err := f.a.Organization.InviteStaff(ctx, nil, owner, child.Operator.ID, "owner@example.test", "owner")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.a.Organization.AcceptStaffInvitation(ctx, nil, invite.Token, organization.VerifiedStaff{ID: owner, Email: "owner@example.test", EmailVerified: true}); err != nil {
		t.Fatal(err)
	}
	quote, err := f.a.Cooperation.PublishPurchaseQuote(ctx, nil, owner, cooperation.PurchaseQuote{SupplierID: parent, BuyerID: child.Operator.ID, GiftTiers: []cooperation.PurchaseGiftTier{{}}, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	newOrder := func() cooperation.PurchaseOrder {
		t.Helper()
		order, err := f.a.Cooperation.CreatePurchaseOrder(ctx, nil, owner, child.Operator.ID, quote.ID, uuid.NewString(), ledger.FromInt64(10))
		if err != nil {
			t.Fatal(err)
		}
		return order
	}
	f.login(t, "staff", "owner@example.test", "owner-password-test")
	base := "/v1/operators/" + parent + "/procurement/"
	unpaid := newOrder()
	status, out, _ := f.call(t, "POST", base+unpaid.ID+"/cancel", map[string]any{}, uuid.NewString())
	mustOK(t, status, out)
	status, out, _ = f.call(t, "POST", base+unpaid.ID+"/cancel", map[string]any{}, uuid.NewString())
	mustOK(t, status, out)
	pending := newOrder()
	merchantID := uuid.NewString()
	_, err = f.a.Payments.SaveMerchant(ctx, owner, payments.Merchant{ID: merchantID, OperatorID: parent, Driver: "stripe", Enabled: true, Credentials: map[string]string{"secret_key": "sk_test_cancellation", "publishable_key": "pk_test_cancellation", "webhook_secret": "whsec_cancellation", "currency": "USD"}}, 0)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(payments.Order{ID: pending.ID, OperatorID: parent, MerchantID: merchantID})
	if err = f.a.DB.Create(&procurementCheckout{OrderID: pending.ID, BuyerID: child.Operator.ID, ActorID: owner, MerchantID: merchantID, PaymentOrder: raw, CreatedAt: time.Now()}).Error; err != nil {
		t.Fatal(err)
	}
	for _, operator := range []string{parent, child.Operator.ID} {
		status, page, _ := f.call(t, "GET", "/v1/operators/"+operator+"/procurement?section=orders", nil, "")
		mustOK(t, status, page)
		for _, raw := range page["items"].([]any) {
			row := raw.(map[string]any)
			if row["id"] != pending.ID {
				continue
			}
			for _, action := range row["allowed_actions"].([]any) {
				if action == "cancel" || action == "fund" || action == "wallet_receipt" {
					t.Fatal("unavailable payment action offered", operator, row)
				}
			}
		}
	}
	status, out, _ = f.call(t, "POST", base+pending.ID+"/cancel", map[string]any{}, uuid.NewString())
	if status != 409 {
		t.Fatal("uncertain checkout could release its order", status, out)
	}
	status, out, _ = f.call(t, "POST", base+pending.ID+"/fund", map[string]any{"receipt_id": uuid.NewString()}, uuid.NewString())
	if status != 409 {
		t.Fatal("online checkout accepted an unrelated offline payment", status, out)
	}
	var order cooperation.PurchaseOrder
	if err = f.a.DB.Where("id=?", pending.ID).First(&order).Error; err != nil || order.Status != "awaiting_payment" {
		t.Fatal("payment uncertainty lost the reservation", order, err)
	}
	var commitment cooperation.SalesCommitment
	if err = f.a.DB.Where("order_id=?", pending.ID).First(&commitment).Error; err != nil || commitment.Status != "active" {
		t.Fatal("payment uncertainty released supplier inventory", commitment, err)
	}
	manualOrder := newOrder()
	manualID := uuid.NewString()
	if _, err = f.a.Payments.SaveMerchant(ctx, owner, payments.Merchant{ID: manualID, OperatorID: parent, Driver: "manual", Enabled: true, Credentials: map[string]string{"currency": "USD", "recipient_reference": "synthetic supplier bank", "instructions": "synthetic transfer"}}, 0); err != nil {
		t.Fatal(err)
	}
	if err = f.a.DB.Create(&procurementCheckout{OrderID: manualOrder.ID, BuyerID: child.Operator.ID, ActorID: owner, MerchantID: manualID, PaymentOrder: json.RawMessage(`{}`), CreatedAt: time.Now()}).Error; err != nil {
		t.Fatal(err)
	}
	if allowed, err := f.a.purchasePaymentMayClose(ctx, f.a.DB, manualOrder); err != nil || !allowed {
		t.Fatal("unused manual instructions prevented cancellation", allowed, err)
	}
	if err = f.a.DB.Table("payments_verified_facts").Create(map[string]any{"id": uuid.NewString(), "merchant_id": manualID, "operator_id": parent, "order_id": manualOrder.ID, "kind": "receipt", "fingerprint": "synthetic pending bank fact", "fact": `{}`, "valuation": `{}`, "state": "awaiting_valuation", "observed_at": time.Now()}).Error; err != nil {
		t.Fatal(err)
	}
	if allowed, err := f.a.purchasePaymentMayClose(ctx, f.a.DB, manualOrder); err != nil || allowed {
		t.Fatal("recorded manual funds released before valuation or allocation", allowed, err)
	}
}

func TestPostgresTakeoverSingleWorkspaceDerivesUSDSettlementFromActualAmount(t *testing.T) {
	f := setupPortal(t)
	ctx := context.Background()
	parent, owner := f.a.Installation.OperatorID, f.a.Installation.OwnerStaffID
	child, err := f.a.Organization.CreateOperator(ctx, nil, organization.CreateOperatorInput{ActorStaffID: owner, ParentOperatorID: parent, Kind: organization.Channel, Name: "takeover settlement channel"})
	if err != nil {
		t.Fatal(err)
	}
	f.login(t, "staff", "owner@example.test", "owner-password-test")
	base := "/v1/operators/" + parent + "/takeovers"
	status, out, _ := f.call(t, "POST", base, map[string]any{"channel_id": child.Operator.ID, "method": "agreed_atg_value", "service_usd_per_atg": "1", "procurement_usd_per_atg": "1", "extra_usd": "5", "contract_reference": "synthetic agreed settlement", "evidence": "brand approved transfer and fixed USD settlement"}, uuid.NewString())
	mustOK(t, status, out)
	id := out["id"].(string)
	status, out, _ = f.call(t, "POST", base+"/"+id+"/refresh", map[string]any{}, uuid.NewString())
	mustOK(t, status, out)
	status, out, _ = f.call(t, "POST", base+"/"+id+"/execute", map[string]any{"expected_snapshot": out["snapshot_fingerprint"]}, uuid.NewString())
	mustOK(t, status, out)
	key := uuid.NewString()
	proof := map[string]any{"currency": "USD", "amount": "5", "fee_usd": "0", "reference": "bank-confirmed-single-usd", "evidence": "synthetic bank statement confirms USD 5 to the brand", "occurred_at": time.Now().UTC()}
	status, out, _ = f.call(t, "POST", base+"/"+id+"/settle", proof, key)
	mustOK(t, status, out)
	if out["settled_usd"] != "5" {
		t.Fatal("USD amount required duplicate valuation", out)
	}
	status, out, _ = f.call(t, "POST", base+"/"+id+"/settle", proof, key)
	mustOK(t, status, out)
	if out["settled_usd"] != "5" {
		t.Fatal("retry duplicated settlement", out)
	}
}
