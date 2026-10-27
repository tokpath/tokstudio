package portal

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
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

type stockFixture struct {
	f                   *testApp
	oem, channel, model string
	quote               cooperation.PurchaseQuote
}

func stockSetup(t *testing.T) stockFixture {
	t.Helper()
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
	child, err := f.a.Organization.CreateOperator(ctx, nil, organization.CreateOperatorInput{ActorStaffID: owner, ParentOperatorID: oem.Operator.ID, Kind: organization.Channel, Name: "OEM3-1 channel"})
	if err != nil {
		t.Fatal(err)
	}
	join(child.Operator.ID)
	model := f.configureExternalCatalog(t, "http://127.0.0.1:58090")
	if err = f.a.Catalog.SetAuthorization(ctx, platform, oem.Operator.ID, model, true, 0); err != nil {
		t.Fatal(err)
	}
	if err = f.a.Catalog.SetListing(ctx, oem.Operator.ID, model, true, 0); err != nil {
		t.Fatal(err)
	}
	out := stockFixture{f: f, oem: oem.Operator.ID, channel: child.Operator.ID, model: model}
	out.price(t, catalog.PriceScope{Kind: "supply", SellerID: platform, BuyerID: out.oem}, "1")
	out.price(t, catalog.PriceScope{Kind: "retail", SellerID: out.oem}, "1")
	_, err = f.a.Cooperation.DeliverPurchase(ctx, nil, cooperation.PurchaseInput{BuyerID: out.oem, SupplierID: platform, OrderID: uuid.NewString(), ReceiptReference: "verified synthetic receipt", ActorID: owner, PaidUSD: ledger.FromInt64(5000), PaidATG: ledger.FromInt64(5000), GiftATG: ledger.FromInt64(90), ExitTerms: json.RawMessage(`{"voluntary_refund":false}`), At: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	out.quote, err = f.a.Cooperation.PublishPurchaseQuote(ctx, nil, owner, cooperation.PurchaseQuote{SupplierID: out.oem, BuyerID: out.channel, GiftTiers: []cooperation.PurchaseGiftTier{{}, {FromUSD: ledger.FromInt64(2000), GiftPercent: ledger.FromInt64(2)}, {FromUSD: ledger.FromInt64(3000), GiftPercent: ledger.FromInt64(3)}}, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	f.login(t, "staff", "owner@example.test", "owner-password-test")
	return out
}
func (s stockFixture) price(t *testing.T, scope catalog.PriceScope, rate string) {
	t.Helper()
	v := catalog.PriceVersion{ID: uuid.NewString(), Scope: scope, ModelID: s.model, Layer: "regular", Published: true, EffectiveAt: time.Now().Add(-time.Hour), Rule: catalog.PriceRule{Mode: "manual", Components: []catalog.ComponentRate{{Key: "input", Unit: "token", Per: "1", Rate: rate}, {Key: "output", Unit: "token", Per: "1", Rate: rate}, {Key: "cache", Unit: "token", Per: "1", Rate: rate}}}}
	if err := s.f.a.Catalog.PublishPrice(context.Background(), scope.SellerID, v, func(context.Context, *gorm.DB, catalog.PriceVersion) error { return nil }); err != nil {
		t.Fatal(err)
	}
}
func (s stockFixture) buy(t *testing.T, amount int64) (int, map[string]any) {
	t.Helper()
	status, out, _ := s.f.call(t, "POST", "/v1/operators/"+s.channel+"/procurement/"+s.quote.ID+"/purchase", map[string]any{"amount_usd": ledger.FromInt64(amount)}, "")
	return status, out
}
func (s stockFixture) coverage(t *testing.T, required, pending, space string, provisional bool) {
	t.Helper()
	v, err := s.f.a.Cooperation.Coverage(context.Background(), nil, s.oem, time.Now())
	if err != nil || v.Required.String() != required || v.Pending.String() != pending || v.Space.String() != space || v.Provisional != provisional {
		t.Fatal("coverage mismatch", v, err)
	}
}
func (s stockFixture) fund(t *testing.T, id string, amount int64) {
	t.Helper()
	now := time.Now()
	owner := s.f.a.Installation.OwnerStaffID
	receipt, err := s.f.a.Commerce.RecordManualReceipt(context.Background(), nil, owner, commerce.VerifiedReceipt{OperatorID: s.oem, ProviderReceiptID: uuid.NewString(), GrossUSD: ledger.FromInt64(amount), FeeStatus: "pending", Currency: "USD", OriginalAmount: ledger.FromInt64(amount).String(), RecipientReference: "OEM bank", PayerReference: "channel bank", ValuationEvidence: "USD", VerificationEvidence: "isolated receipt", PaidAt: &now, ObservedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	status, out, _ := s.f.call(t, "POST", "/v1/operators/"+s.oem+"/procurement/"+id+"/fund", map[string]any{"receipt_id": receipt.ID}, "")
	mustOK(t, status, out)
}

func (s stockFixture) stopModel(t *testing.T, listing bool) {
	t.Helper()
	ctx := context.Background()
	controller := *s.f.a.CatalogChanges
	now := time.Now().UTC().Truncate(time.Microsecond)
	controller.Now = func() time.Time { return now }
	var change catalog.CatalogChange
	var err error
	if listing {
		change, err = controller.ScheduleListing(ctx, s.oem, s.model, false, 1, now.Add(3*time.Minute), "planned stock configuration change")
	} else {
		change, err = controller.ScheduleAuthorization(ctx, s.oem, s.channel, s.model, false, 1, now.Add(3*time.Minute), "planned stock configuration change")
	}
	if err != nil {
		t.Fatal(err)
	}
	if _, err = controller.Run(ctx, 100); err != nil {
		t.Fatal(err)
	}
	now = change.EffectiveAt
	if _, err = controller.Run(ctx, 100); err != nil {
		t.Fatal(err)
	}
	var stored catalog.CatalogChange
	if err = s.f.a.DB.First(&stored, "id=?", change.ID).Error; err != nil || stored.State != "applied" {
		t.Fatal(stored, err)
	}
}
func TestPostgresOEMStockPurchaseBeforeModelsReservesAndDeliversWithoutCredit(t *testing.T) {
	s := stockSetup(t)
	f := s.f
	ctx := context.Background()
	status, page, _ := f.call(t, "GET", "/v1/operators/"+s.channel+"/procurement?section=quotes", nil, "")
	mustOK(t, status, page)
	row := page["items"].([]any)[0].(map[string]any)
	if row["values"].(map[string]any)["purchase_available"] != true || !strings.Contains(row["values"].(map[string]any)["purchase_notice"].(string), "备货") || row["allowed_actions"].([]any)[0] != "purchase" {
		t.Fatal("stock purchase hidden", page)
	}
	status, out := s.buy(t, 1000)
	mustOK(t, status, out)
	id := out["id"].(string)
	s.coverage(t, "0", "1000", "4090", true)
	if err := f.a.Catalog.SetAuthorization(ctx, s.oem, s.channel, s.model, true, 0); err != nil {
		t.Fatal(err)
	}
	status, extra := s.buy(t, 500) // authorized but still missing call prices
	mustOK(t, status, extra)
	s.coverage(t, "0", "1500", "3590", true)
	status, canceled, _ := f.call(t, "POST", "/v1/operators/"+s.channel+"/procurement/"+extra["id"].(string)+"/cancel", map[string]any{}, "")
	mustOK(t, status, canceled)
	status, out = s.buy(t, 4091)
	if status != 409 || out["error"].(map[string]any)["code"] != "sales_coverage_insufficient" {
		t.Fatal("oversold provisional stock", out)
	}
	s.fund(t, id, 1000)
	s.coverage(t, "1000", "0", "4090", true)
	stock, err := f.a.Cooperation.Position(ctx, nil, s.channel)
	if err != nil || stock.Quantity.String() != "1000" {
		t.Fatal(stock, err)
	}
	if _, err = f.a.Cooperation.ReserveProcurement(ctx, nil, s.channel, "no-credit", ledger.FromInt64(1001), time.Now().Add(time.Minute)); !errors.Is(err, ledger.ErrInsufficient) {
		t.Fatal("stock purchase enabled borrowing", err)
	}
	snapshot, err := f.a.Cooperation.Position(ctx, nil, s.oem)
	if err != nil || snapshot.Quantity.String() != "5090" {
		t.Fatal("purchase spent upper balance before usage", snapshot, err)
	}
}
func TestPostgresUnconfiguredWholesaleConcurrencyGiftsAndCancellation(t *testing.T) {
	s := stockSetup(t)
	ctx := context.Background()
	owner := s.f.a.Installation.OwnerStaffID
	var count atomic.Int32
	var wg sync.WaitGroup
	results := make(chan cooperation.PurchaseOrder, 20)
	failures := make(chan error, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			order, err := s.f.a.Cooperation.CreatePurchaseOrder(ctx, nil, owner, s.channel, s.quote.ID, uuid.NewString(), ledger.FromInt64(1000))
			if err == nil {
				count.Add(1)
				results <- order
			} else if !errors.Is(err, cooperation.ErrCoverage) {
				failures <- err
			}
		}()
	}
	wg.Wait()
	close(results)
	close(failures)
	for err := range failures {
		t.Fatal(err)
	}
	if count.Load() != 5 {
		t.Fatal("same stock promised more than once", count.Load())
	}
	s.coverage(t, "0", "5000", "90", true)
	for order := range results {
		status, out, _ := s.f.call(t, "POST", "/v1/operators/"+s.channel+"/procurement/"+order.ID+"/cancel", map[string]any{}, "")
		mustOK(t, status, out)
	}
	s.coverage(t, "0", "0", "5090", false)
	status, out := s.buy(t, 5000)
	mustOK(t, status, out)
	s.coverage(t, "0", "5080", "10", true) // purchased AND gift stock are reserved
	status, out = s.buy(t, 11)
	if status != 409 {
		t.Fatal("gift stock omitted", out)
	}
}
func TestPostgresPreparedPricesRevalueExistingStockAndKeepKnownCommitments(t *testing.T) {
	s := stockSetup(t)
	ctx := context.Background()
	f := s.f
	status, paid := s.buy(t, 1000)
	mustOK(t, status, paid)
	s.fund(t, paid["id"].(string), 1000)
	status, out := s.buy(t, 2000)
	mustOK(t, status, out)
	s.coverage(t, "1000", "2000", "2090", true)
	s.price(t, catalog.PriceScope{Kind: "supply", SellerID: s.oem, BuyerID: s.channel}, "0.5")
	if err := f.a.Catalog.SetAuthorization(ctx, s.oem, s.channel, s.model, true, 0); err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(map[string]any{"buyer_id": s.channel, "model_id": s.model, "enabled": true})
	change := catalog.CatalogChange{ID: uuid.NewString(), OperatorID: s.oem, Kind: "authorization", ResourceID: s.model, EffectiveAt: time.Now(), Payload: payload}
	if err := f.a.DB.Transaction(func(tx *gorm.DB) error { return f.a.validateCatalogChange(ctx, tx, change) }); err != nil {
		t.Fatal("setup blocked by newly discovered gap", err)
	}
	s.coverage(t, "2000", "4000", "-910", false)
	var check priceCheck
	if err := f.a.DB.Where("price_id=? AND operator_id=?", "catalog:"+change.ID, s.oem).First(&check).Error; err != nil || check.Status != "gap" {
		t.Fatal("gap not recorded", check, err)
	}
	var notices int64
	f.a.DB.Table("operations_notifications").Where("operator_id=? AND title=?", s.oem, "调价后的责任覆盖需要处理").Count(&notices)
	if notices == 0 {
		t.Fatal("responsible operator was not alerted")
	}
	status, out = s.buy(t, 1)
	if status != 409 {
		t.Fatal("new commitments accepted in a gap", out)
	}
	s.stopModel(t, false)
	s.coverage(t, "2000", "4000", "-910", true) // removing config cannot release known responsibility
	stock, err := f.a.Cooperation.Position(ctx, nil, s.channel)
	if err != nil || stock.Quantity.String() != "1000" {
		t.Fatal("revaluation changed paid rights", stock, err)
	}
}
func TestPostgresStockPurchaseIncludesExistingRetailRights(t *testing.T) {
	s := stockSetup(t)
	ctx := context.Background()
	owner := s.f.a.Installation.OwnerStaffID
	_, err := s.f.a.Ledger.IssueGrant(ctx, nil, ledger.GrantInput{OperatorID: s.oem, CustomerID: uuid.NewString(), OrderID: uuid.NewString(), BusinessKey: uuid.NewString(), ActorID: owner, Reason: "existing paid rights", PaidUSD: ledger.FromInt64(4000), Sources: []ledger.GrantSource{{Kind: "paid", Amount: ledger.FromInt64(4000)}}, EffectiveAt: time.Now().Add(-time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	status, out := s.buy(t, 1000)
	mustOK(t, status, out)
	s.coverage(t, "4000", "1000", "90", true)
	s.stopModel(t, true)
	s.coverage(t, "4000", "1000", "90", true)
	status, out = s.buy(t, 91)
	if status != 409 {
		t.Fatal("retail rights were ignored", out)
	}
	if _, err = s.f.a.Cooperation.Reserve(ctx, nil, commerce.Order{ID: uuid.NewString(), OperatorID: s.oem, PaidATG: ledger.FromInt64(1)}); !errors.Is(err, cooperation.ErrCoverage) {
		t.Fatal("unpriced retail sale accepted", err)
	}
}

// Standalone real application for browser verification; no external money or data.
func TestExternalStockPurchaseAcceptanceServer(t *testing.T) {
	ready := os.Getenv("TOKENHUB_STOCK_ACCEPTANCE_FILE")
	if ready == "" {
		t.Skip("explicit stock browser acceptance only")
	}
	s := stockSetup(t)
	s.f.server.Close()
	s.f.a.Config.TrustedProxies = []string{"127.0.0.1", "::1"}
	s.f.a.Config.WebOrigin = "http://127.0.0.1:13003"
	s.f.a.Config.PublicBaseURL = "http://127.0.0.1:13003"
	listener, err := net.Listen("tcp", "0.0.0.0:18081")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(s.f.a.Router())
	server.Listener.Close()
	server.Listener = listener
	server.Start()
	server.URL = "http://127.0.0.1:18081"
	s.f.server = server
	t.Cleanup(server.Close)
	data, _ := json.Marshal(map[string]string{"url": s.f.server.URL, "operator_id": s.channel, "supplier_id": s.oem, "owner_email": "owner@example.test", "owner_password": "owner-password-test"})
	if err := os.WriteFile(ready, data, 0600); err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(ready); os.IsNotExist(err) {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
}
