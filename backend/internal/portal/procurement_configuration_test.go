package portal

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/cooperation"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"gorm.io/gorm"
)

func TestPostgresOEMPurchaseExplainsMissingConfigurationAndRecoversWithoutBalanceChanges(t *testing.T) {
	f := setupPortal(t)
	ctx := context.Background()
	platform, owner := f.a.Installation.OperatorID, f.a.Installation.OwnerStaffID
	oem, err := f.a.Organization.CreateOperator(ctx, nil, organization.CreateOperatorInput{ActorStaffID: owner, ParentOperatorID: platform, Kind: organization.OEM, Name: "Third OEM", BrandName: "Third OEM"})
	if err != nil {
		t.Fatal(err)
	}
	join := func(operator string) {
		inv, err := f.a.Organization.InviteStaff(ctx, nil, owner, operator, "owner@example.test", "owner")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = f.a.Organization.AcceptStaffInvitation(ctx, nil, inv.Token, organization.VerifiedStaff{ID: owner, Email: "owner@example.test", EmailVerified: true}); err != nil {
			t.Fatal(err)
		}
	}
	join(oem.Operator.ID)
	channel, err := f.a.Organization.CreateOperator(ctx, nil, organization.CreateOperatorInput{ActorStaffID: owner, ParentOperatorID: oem.Operator.ID, Kind: organization.Channel, Name: "OEM3-1 channel"})
	if err != nil {
		t.Fatal(err)
	}
	join(channel.Operator.ID)
	model := f.configureExternalCatalog(t, "http://127.0.0.1:58090")
	if err = f.a.Catalog.SetAuthorization(ctx, platform, oem.Operator.ID, model, true, 0); err != nil {
		t.Fatal(err)
	}
	if err = f.a.Catalog.SetListing(ctx, oem.Operator.ID, model, true, 0); err != nil {
		t.Fatal(err)
	}
	price := func(scope catalog.PriceScope) {
		v := catalog.PriceVersion{ID: uuid.NewString(), Scope: scope, ModelID: model, Layer: "regular", Published: true, EffectiveAt: time.Now().Add(-time.Hour), Rule: catalog.PriceRule{Mode: "manual", Components: []catalog.ComponentRate{{Key: "input", Unit: "token", Per: "1", Rate: "1"}, {Key: "output", Unit: "token", Per: "1", Rate: "1"}, {Key: "cache", Unit: "token", Per: "1", Rate: "1"}}}}
		if err := f.a.Catalog.PublishPrice(ctx, scope.SellerID, v, func(context.Context, *gorm.DB, catalog.PriceVersion) error { return nil }); err != nil {
			t.Fatal(err)
		}
	}
	price(catalog.PriceScope{Kind: "supply", SellerID: platform, BuyerID: oem.Operator.ID})
	price(catalog.PriceScope{Kind: "retail", SellerID: oem.Operator.ID})
	_, err = f.a.Cooperation.DeliverPurchase(ctx, nil, cooperation.PurchaseInput{BuyerID: oem.Operator.ID, SupplierID: platform, OrderID: uuid.NewString(), ReceiptReference: "verified synthetic receipt", ActorID: owner, PaidUSD: ledger.FromInt64(5000), PaidATG: ledger.FromInt64(5000), GiftATG: ledger.FromInt64(90), ExitTerms: json.RawMessage(`{"voluntary_refund":false}`), At: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	quote, err := f.a.Cooperation.PublishPurchaseQuote(ctx, nil, owner, cooperation.PurchaseQuote{SupplierID: oem.Operator.ID, BuyerID: channel.Operator.ID, GiftTiers: []cooperation.PurchaseGiftTier{{}}, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	f.login(t, "staff", "owner@example.test", "owner-password-test")
	base := "/v1/operators/" + channel.Operator.ID + "/procurement"
	purchase := func(amount int64) (int, map[string]any) {
		status, out, _ := f.call(t, "POST", base+"/"+quote.ID+"/purchase", map[string]any{"amount_usd": ledger.FromInt64(amount)}, "")
		return status, out
	}
	assertBlocked := func(code string) {
		status, out := purchase(1000)
		if status != 409 || out["error"].(map[string]any)["code"] != code {
			t.Fatal(status, out)
		}
		if strings.Contains(out["error"].(map[string]any)["message"].(string), "当前业务状态或资金条件") {
			t.Fatal("generic error survived", out)
		}
		status, page, _ := f.call(t, "GET", base+"?section=quotes", nil, "")
		mustOK(t, status, page)
		if len(page["availability"].([]any)) != 1 || len(page["items"].([]any)[0].(map[string]any)["allowed_actions"].([]any)) != 0 {
			t.Fatal("unready quote offered checkout", page)
		}
		var orders int64
		f.a.DB.Model(&cooperation.PurchaseOrder{}).Where("buyer_id=?", channel.Operator.ID).Count(&orders)
		if orders != 0 {
			t.Fatal("failed configuration check created an order")
		}
	}
	assertBlocked("procurement_models_required")
	if err = f.a.Catalog.SetAuthorization(ctx, oem.Operator.ID, channel.Operator.ID, model, true, 0); err != nil {
		t.Fatal(err)
	}
	assertBlocked("procurement_prices_required")
	price(catalog.PriceScope{Kind: "supply", SellerID: oem.Operator.ID, BuyerID: channel.Operator.ID})
	status, page, _ := f.call(t, "GET", base+"?section=quotes", nil, "")
	mustOK(t, status, page)
	if actions := page["items"].([]any)[0].(map[string]any)["allowed_actions"].([]any); len(actions) != 1 || actions[0] != "purchase" {
		t.Fatal("configured buyer remained blocked", page)
	}
	status, out := purchase(1000)
	mustOK(t, status, out)
	orderID := out["id"].(string)
	status, out = purchase(4091)
	if status != 409 || out["error"].(map[string]any)["code"] != "sales_coverage_insufficient" {
		t.Fatal("real insufficient coverage not protected", status, out)
	}
	now := time.Now()
	receipt, err := f.a.Commerce.RecordManualReceipt(ctx, nil, owner, commerce.VerifiedReceipt{OperatorID: oem.Operator.ID, ProviderReceiptID: "synthetic OEM purchase", GrossUSD: ledger.FromInt64(1000), FeeStatus: "pending", Currency: "USD", OriginalAmount: "1000", RecipientReference: "OEM bank", PayerReference: "channel bank", ValuationEvidence: "USD", VerificationEvidence: "isolated verified receipt", PaidAt: &now, ObservedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	status, out, _ = f.call(t, "POST", "/v1/operators/"+oem.Operator.ID+"/procurement/"+orderID+"/fund", map[string]any{"receipt_id": receipt.ID}, "")
	mustOK(t, status, out)
	position, err := f.a.Cooperation.Position(ctx, nil, channel.Operator.ID)
	if err != nil || position.Quantity.String() != "1000" {
		t.Fatal("paid procurement not delivered", position, err)
	}
	stock, err := f.a.Cooperation.Position(ctx, nil, oem.Operator.ID)
	if err != nil || stock.Quantity.String() != "5090" {
		t.Fatal("ordering altered upstream stock", stock, err)
	}
}
