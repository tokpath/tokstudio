package portal

import (
	"context"
	"encoding/json"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/branding"
	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/cooperation"
	"github.com/tokpath/tokstudio/backend/internal/incentives"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"gorm.io/gorm"
)

// Only model readiness/prices are fixed in this fixture. Registration, orders,
// wallet funding, delivery, takeover, notifications and HTTP use the real app/DB.
type exitReadiness struct{}

func (exitReadiness) Check(context.Context, *gorm.DB, organization.Operator, organization.BusinessFeature) ([]organization.Requirement, error) {
	return nil, nil
}

type exitCoverage struct{}

func (exitCoverage) CoverageRates(context.Context, *gorm.DB, string, time.Time) (cooperation.CoverageRates, error) {
	return cooperation.CoverageRates{Version: 1, Direct: big.NewRat(1, 1), Children: map[string]*big.Rat{}}, nil
}

type exitFixture struct {
	f                                                                   *testApp
	oem, channel, platformChannel, customer, order, subscription, email string
}

func setupExitAcceptance(t *testing.T) exitFixture {
	t.Helper()
	f := setupPortalMonetary(t, false)
	ctx := context.Background()
	a := f.a
	owner, platform := a.Installation.OwnerStaffID, a.Installation.OperatorID
	a.Cooperation.Rates = exitCoverage{}
	_, port, _ := net.SplitHostPort(f.server.Listener.Addr().String())
	f.server.URL = "http://exit.localhost:" + port
	f.client.Transport = &http.Transport{DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		host, p, _ := net.SplitHostPort(address)
		if host == "exit.localhost" {
			address = net.JoinHostPort("127.0.0.1", p)
		}
		return (&net.Dialer{}).DialContext(ctx, network, address)
	}}

	// Currency/ratio chosen before any fixture assets; this is not a USD/1:1 test.
	if err := a.DB.Exec("UPDATE ledger_monetary_settings SET currency='EUR',atg_per_unit=7,effective_at=clock_timestamp()-interval '1 minute'").Error; err != nil {
		t.Fatal(err)
	}
	create := func(parent string, kind organization.OperatorKind, name string) organization.OperatorCreated {
		v, e := a.Organization.CreateOperator(ctx, nil, organization.CreateOperatorInput{ActorStaffID: owner, ParentOperatorID: parent, Kind: kind, Name: name, BrandName: name})
		if e != nil {
			t.Fatal(e)
		}
		inv, e := a.Organization.InviteStaff(ctx, nil, owner, v.Operator.ID, "owner@example.test", "owner")
		if e != nil {
			t.Fatal(e)
		}
		if _, e = a.Organization.AcceptStaffInvitation(ctx, nil, inv.Token, organization.VerifiedStaff{ID: owner, Email: inv.Email, EmailVerified: true}); e != nil {
			t.Fatal(e)
		}
		return v
	}
	oem := create(platform, organization.OEM, "星河品牌")

	domainTime := time.Now()
	if e := a.DB.Table("branding_domains").Create(&branding.Domain{ID: uuid.NewString(), BrandID: oem.Brand.ID, Hostname: "exit.localhost", VerificationToken: "local-only", Status: "active", TLSReady: true, RouteReady: true, VerifiedAt: &domainTime, CheckedAt: &domainTime, ProviderReference: "isolated acceptance", Version: 1, CreatedAt: domainTime}).Error; e != nil {
		t.Fatal(e)
	}

	channel := create(oem.Operator.ID, organization.Channel, "星河渠道")
	direct := create(platform, organization.Channel, "官方直属渠道")
	for _, op := range []string{oem.Operator.ID, channel.Operator.ID, direct.Operator.ID} {
		if _, e := a.Incentives.SetPolicy(ctx, nil, owner, op, incentives.PolicyInput{ThresholdMoney: ledger.FromInt64(100), DirectRate: ledger.MustDecimal("0.1"), IndirectRate: ledger.MustDecimal("0.02"), RewardValiditySeconds: 86400}); e != nil {
			t.Fatal(e)
		}
		for _, feature := range []organization.BusinessFeature{organization.Registration, organization.NewPurchase, organization.Renewal, organization.Activity} {
			state, e := a.Organization.GetBusinessStatus(ctx, nil, op)
			if e != nil {
				t.Fatal(e)
			}
			if _, e = a.Organization.SetBusinessStatus(ctx, nil, organization.SetBusinessStatusInput{ActorStaffID: owner, OperatorID: op, Feature: feature, Enabled: true, ExpectedVersion: state.Version, EvidenceReference: "isolated acceptance readiness"}, exitReadiness{}); e != nil {
				t.Fatal(e)
			}
		}
	}
	email := "takeover@example.test"
	intent, e := a.Organization.BeginRegistration(ctx, nil, organization.RegistrationIntentRequest{BrandID: channel.Brand.ID, InviteCode: channel.InviteCode, Email: email, Password: "customer-password-test"})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = a.Organization.VerifyEmail(ctx, nil, intent.Intent.ID, intent.VerificationToken); e != nil {
		t.Fatal(e)
	}
	reg, e := a.Organization.CompleteRegistration(ctx, nil, organization.CompleteRegistrationInput{IntentID: intent.Intent.ID, VerificationToken: intent.VerificationToken})
	if e != nil {
		t.Fatal(e)
	}
	for _, pair := range [][2]string{{oem.Operator.ID, platform}, {channel.Operator.ID, oem.Operator.ID}} {
		if _, e = a.Cooperation.DeliverPurchase(ctx, nil, cooperation.PurchaseInput{BuyerID: pair[0], SupplierID: pair[1], OrderID: uuid.NewString(), ReceiptReference: "synthetic opening stock", ActorID: owner, PaidMoney: ledger.FromInt64(100), PaidATG: ledger.FromInt64(700), ExitTerms: json.RawMessage(`{"voluntary_refund":false}`), At: time.Now()}); e != nil {
			t.Fatal(e)
		}
	}
	topup, e := a.Commerce.CreateTopUpOrder(ctx, nil, commerce.CreateTopUpInput{OperatorID: channel.Operator.ID, CustomerID: reg.Customer.ID, BusinessKey: "opening-wallet", ExpectedOwnershipVersion: 1, AmountMoney: ledger.FromInt64(125)})
	if e != nil {
		t.Fatal(e)
	}
	now := time.Now()
	receipt, e := a.Commerce.RecordManualReceipt(ctx, nil, owner, commerce.VerifiedReceipt{OperatorID: channel.Operator.ID, ProviderReceiptID: "synthetic EUR opening", GrossMoney: ledger.FromInt64(125), FeeStatus: "pending", Currency: "EUR", OriginalAmount: "125", ValuationEvidence: "EUR settlement currency", VerificationEvidence: "isolated receipt", PaidAt: &now})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = a.Commerce.AllocateReceipt(ctx, nil, commerce.AllocateReceiptInput{ActorStaffID: owner, OperatorID: channel.Operator.ID, BusinessKey: "fund-wallet", ReceiptID: receipt.ID, OrderID: topup.ID, AmountMoney: ledger.FromInt64(125)}); e != nil {
		t.Fatal(e)
	}
	if _, e = a.Commerce.Fulfill(ctx, nil, topup.ID, owner); e != nil {
		t.Fatal(e)
	}
	out := exitFixture{f: f, oem: oem.Operator.ID, channel: channel.Operator.ID, platformChannel: direct.Operator.ID, customer: reg.Customer.ID, email: email}
	for _, period := range []ledger.Period{ledger.Month, ledger.Lifetime} {
		name := "渠道月套餐"
		price := int64(80)
		if period == ledger.Lifetime {
			name = "渠道不限时套餐"
			price = 20
		}
		quote, e := a.Commerce.PublishPackage(ctx, nil, commerce.PublishPackageInput{ActorStaffID: owner, OperatorID: channel.Operator.ID, Name: name, PriceMoney: ledger.FromInt64(price), PaidATG: ledger.FromInt64(price), GiftATG: ledger.FromInt64(20), Period: period, Timezone: "Asia/Shanghai", NewPurchaseEnabled: true, RenewalEnabled: period != ledger.Lifetime})
		if e != nil {
			t.Fatal(e)
		}
		order, e := a.Commerce.CreateOrder(ctx, nil, commerce.CreateOrderInput{OperatorID: channel.Operator.ID, CustomerID: reg.Customer.ID, QuoteID: quote.ID, BusinessKey: string(period), ExpectedOwnershipVersion: 1, InternalMoney: ledger.FromInt64(price)})
		if e != nil {
			t.Fatal(e)
		}
		if _, e = a.Commerce.Fulfill(ctx, nil, order.ID, owner); e != nil {
			t.Fatal(e)
		}
		if period == ledger.Month {
			sub, e := a.Commerce.EnableAutoRenewal(ctx, nil, reg.Customer.ID, order.ID, "original customer consent", false)
			if e != nil {
				t.Fatal(e)
			}
			out.order, out.subscription = order.ID, sub.ID
		}
	}
	return out
}

func TestExternalExitAcceptanceServer(t *testing.T) {
	ready := os.Getenv("TOKENHUB_EXIT_ACCEPTANCE_FILE")
	if ready == "" {
		t.Skip("opt-in isolated browser fixture")
	}
	s := setupExitAcceptance(t)
	f := s.f
	f.server.Close()
	f.a.Config.TrustedProxies = []string{"127.0.0.1", "::1"}
	f.a.Config.WebOrigin = "http://127.0.0.1:13003"
	f.a.Config.PublicBaseURL = f.a.Config.WebOrigin
	listener, e := net.Listen("tcp", "127.0.0.1:18081")
	if e != nil {
		t.Fatal(e)
	}
	server := httptest.NewUnstartedServer(f.a.Router())
	server.Listener.Close()
	server.Listener = listener
	server.Start()
	t.Cleanup(server.Close)
	data, _ := json.Marshal(map[string]string{"platform": f.a.Installation.OperatorID, "oem": s.oem, "channel": s.channel, "platform_channel": s.platformChannel, "customer": s.customer, "customer_email": s.email, "owner_email": "owner@example.test", "owner_password": "owner-password-test"})
	if e = os.WriteFile(ready, data, 0600); e != nil {
		t.Fatal(e)
	}
	deadline := time.Now().Add(60 * time.Minute)
	for time.Now().Before(deadline) {
		if _, e = os.Stat(ready); os.IsNotExist(e) {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func TestPostgresTakeoverCopiedPlansAndRenewalConsentHTTP(t *testing.T) {
	s := setupExitAcceptance(t)
	f := s.f
	ctx := context.Background()
	owner := f.a.Installation.OwnerStaffID
	f.login(t, "customer", s.email, "customer-password-test")
	invoiceStatus, invoice, _ := f.call(t, "POST", "/v1/me/invoices", map[string]any{"issuer_id": s.channel, "lines": []map[string]string{{"order_id": s.order}}, "details": map[string]string{"title": "Original channel buyer", "tax_id": "TEST-123", "email": "billing@example.test"}}, "")
	mustOK(t, invoiceStatus, invoice)
	invoiceID := invoice["id"].(string)
	f.login(t, "staff", "owner@example.test", "owner-password-test")
	base := "/v1/operators/" + s.oem + "/takeovers"
	status, p, _ := f.call(t, "POST", base, map[string]any{"channel_id": s.channel}, "")
	mustOK(t, status, p)
	path := base + "/" + p["id"].(string)
	status, p, _ = f.call(t, "POST", path+"/refresh", map[string]any{}, "")
	mustOK(t, status, p)
	snap := p["snapshot"].(map[string]any)
	if snap["cash_money"] != "25" || snap["service_atg"] != "140" || len(snap["packages"].([]any)) != 2 {
		t.Fatal("rights summary", snap)
	}
	assertTakeoverWorkspaceState(t, f, s.channel, "awaiting_approval")
	status, p, _ = f.call(t, "POST", path+"/execute", map[string]any{"expected_snapshot": p["snapshot_fingerprint"]}, "")
	mustOK(t, status, p)
	assertTakeoverWorkspaceState(t, f, s.channel, "completed")
	status, invoice, _ = f.call(t, "POST", "/v1/operators/"+s.channel+"/invoices/"+invoiceID+"/register", map[string]any{"state": "rejected", "reason": "Buyer requested a corrected invoice title"}, "")
	mustOK(t, status, invoice)

	finance := uuid.NewString()
	passwordHash, err := organization.HashPassword("finance-password-test")
	if err != nil {
		t.Fatal(err)
	}
	if err = f.a.DB.Create(&Staff{ID: finance, Email: "finance-takeover@example.test", DisplayName: "Finance", PasswordHash: passwordHash, Status: "active", Version: 1, EmailVerified: true, CreatedAt: time.Now()}).Error; err != nil {
		t.Fatal(err)
	}

	invitation, e := f.a.Organization.InviteStaff(ctx, nil, owner, s.oem, "finance-takeover@example.test", "finance")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.a.Organization.AcceptStaffInvitation(ctx, nil, invitation.Token, organization.VerifiedStaff{ID: finance, Email: invitation.Email, EmailVerified: true}); e != nil {
		t.Fatal(e)
	}
	if rows, e := f.a.Remediation.Takeovers(ctx, finance, s.oem); e != nil || len(rows) != 1 {
		t.Fatal("finance cannot review liabilities", rows, e)
	}
	if _, e := f.a.Remediation.CancelTakeover(ctx, finance, s.oem, p["id"].(string), "forbidden finance mutation"); e == nil {
		t.Fatal("finance changed takeover")
	}
	if rows, e := f.a.Remediation.Takeovers(ctx, owner, f.a.Installation.OperatorID); e != nil || len(rows) != 0 {
		t.Fatal("platform read independent OEM takeover", rows, e)
	}
	f.login(t, "customer", s.email, "customer-password-test")
	status, plans, _ := f.call(t, "GET", "/v1/me/plans", nil, "")
	mustOK(t, status, plans)
	var monthly map[string]any
	for _, raw := range plans["items"].([]any) {
		item := raw.(map[string]any)
		if item["status"] != "active" || item["auto_renew"] != false || item["auto_renew_available"] != false {
			t.Fatal("continued renewal", item)
		}
		if item["id"] == s.subscription {
			monthly = item
		}
	}
	if monthly == nil {
		t.Fatal("transferred subscription missing", plans)
	}
	var copied commerce.Package
	if err := f.a.DB.Table("commerce_packages").Where("operator_id=? AND source_operator_id=? AND name=?", s.oem, s.channel, "渠道月套餐").Take(&copied).Error; err != nil {
		t.Fatal(err)
	}
	status, _, _ = f.call(t, "POST", "/v1/me/orders/quotes", map[string]any{"plan_id": copied.ID}, "")
	if status == 200 {
		t.Fatal("disabled copied plan could be purchased")
	}
	quote, e := f.a.Commerce.GetQuote(ctx, nil, *copied.CurrentQuoteID)
	if e != nil {
		t.Fatal(e)
	}
	reopened, e := f.a.Commerce.PublishPackage(ctx, nil, commerce.PublishPackageInput{ActorStaffID: owner, OperatorID: s.oem, PackageID: copied.ID, ExpectedVersion: quote.Version, Name: copied.Name, PriceMoney: quote.PriceMoney, PaidATG: quote.PaidATG, GiftATG: quote.GiftATG, Period: quote.Period, Timezone: quote.Timezone, RenewalEnabled: true})
	if e != nil {
		t.Fatal(e)
	}
	status, plans, _ = f.call(t, "GET", "/v1/me/plans", nil, "")
	mustOK(t, status, plans)
	for _, raw := range plans["items"].([]any) {
		item := raw.(map[string]any)
		if item["id"] == s.subscription {
			monthly = item
		}
	}
	if monthly["requires_confirmation"] != true || monthly["auto_renew"] != false || monthly["auto_renew_available"] != true {
		t.Fatal("new brand consent absent", monthly)
	}
	input := map[string]any{"auto_renew": true, "version": monthly["version"], "plan_id": copied.ID, "quote_id": reopened.ID}
	status, _, _ = f.call(t, "PATCH", "/v1/me/plans/"+s.subscription+"/renewal", input, "")
	if status == 200 {
		t.Fatal("accepted without new brand consent")
	}
	input["confirmed_changed_terms"] = true
	status, result, _ := f.call(t, "PATCH", "/v1/me/plans/"+s.subscription+"/renewal", input, "")
	mustOK(t, status, result)
	status, plans, _ = f.call(t, "GET", "/v1/me/plans", nil, "")
	mustOK(t, status, plans)
	for _, raw := range plans["items"].([]any) {
		item := raw.(map[string]any)
		if item["id"] == s.subscription && (item["requires_confirmation"] != false || item["auto_renew"] != true) {
			t.Fatal("confirmed renewal not reflected", item)
		}
	}
	for i := 0; i < 2; i++ {
		if _, e = f.a.Remediation.RelayEvents(ctx, nil, 100, f.a.publishRemediationEvent); e != nil {
			t.Fatal(e)
		}
	}
	var notices int64
	if e = f.a.DB.Table("operations_notifications").Where("operator_id=? AND recipient_id=? AND title=?", s.oem, s.customer, "渠道客户权益已承接").Count(&notices).Error; e != nil || notices != 1 {
		t.Fatal("customer completion notice", notices, e)
	}
}
