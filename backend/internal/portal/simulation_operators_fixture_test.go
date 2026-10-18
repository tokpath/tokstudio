package portal

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/branding"
	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"github.com/tokpath/tokstudio/backend/internal/cooperation"
	"github.com/tokpath/tokstudio/backend/internal/incentives"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"github.com/tokpath/tokstudio/backend/internal/payments"
)

// Only synthetic opening facts are prepared here. The browser subsequently
// creates quotes, procurement orders, payments, model listings, retail prices,
// packages, replenishment mandates, exits and the takeover itself.
func (f *testApp) seedOperatorSimulation(t *testing.T, lab string) map[string]any {
	t.Helper()
	ctx := context.Background()
	platform, platformOwner := f.a.Installation.OperatorID, f.a.Installation.OwnerStaffID
	models, err := f.a.Catalog.PublicCatalog(ctx, platform, time.Now())
	if err != nil || len(models) == 0 {
		t.Fatalf("operator simulation requires external model: %v", err)
	}
	model := models[0].ID
	password := "simulation-operator-password"
	d := ledger.MustDecimal
	staff := func(actor, operator, email, name, role string) Staff {
		t.Helper()
		hash, e := organization.HashPassword(password)
		if e != nil {
			t.Fatal(e)
		}
		person := Staff{ID: uuid.NewString(), Email: email, DisplayName: name, PasswordHash: hash, Version: 1, Status: "active", EmailVerified: true, CreatedAt: time.Now()}
		if e = f.a.DB.Create(&person).Error; e != nil {
			t.Fatal(e)
		}
		invite, e := f.a.Organization.InviteStaff(ctx, nil, actor, operator, email, role)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = f.a.Organization.AcceptStaffInvitation(ctx, nil, invite.Token, organization.VerifiedStaff{ID: person.ID, Email: email, EmailVerified: true}); e != nil {
			t.Fatal(e)
		}
		return person
	}
	create := func(actor, parent string, kind organization.OperatorKind, name string) organization.OperatorCreated {
		t.Helper()
		out, e := f.a.Organization.CreateOperator(ctx, nil, organization.CreateOperatorInput{ActorStaffID: actor, ParentOperatorID: parent, Kind: kind, Name: name, LegalName: name + " 合成经营主体", BrandName: "山海仿真模型"})
		if e != nil {
			t.Fatal(e)
		}
		return out
	}
	oem := create(platformOwner, platform, organization.OEM, "仿真山海 OEM")
	oemOwner := staff(platformOwner, oem.Operator.ID, "sim-oem-owner@operator.test", "山海品牌负责人", "owner")
	channel := create(oemOwner.ID, oem.Operator.ID, organization.Channel, "仿真星河渠道")
	channelOwner := staff(oemOwner.ID, channel.Operator.ID, "sim-channel-owner@operator.test", "星河渠道负责人", "owner")
	spare := create(oemOwner.ID, oem.Operator.ID, organization.Channel, "仿真独立渠道")
	spareOwner := staff(oemOwner.ID, spare.Operator.ID, "sim-other-owner@operator.test", "独立渠道负责人", "owner")
	finance := staff(channelOwner.ID, channel.Operator.ID, "sim-channel-finance@operator.test", "星河财务", "finance")
	support := staff(channelOwner.ID, channel.Operator.ID, "sim-channel-support@operator.test", "星河客服", "support")
	readonly := staff(oemOwner.ID, oem.Operator.ID, "sim-oem-readonly@operator.test", "山海只读人员", "readonly")
	// The *.localhost host is a preconfigured local-only fixture. It deliberately
	// does not claim that public DNS ownership or a public TLS certificate exists.
	now := time.Now().UTC()
	domain := branding.Domain{ID: uuid.NewString(), BrandID: oem.Brand.ID, Hostname: "operators.localhost", VerificationToken: "synthetic-local-only", Status: "active", TLSReady: true, VerifiedAt: &now, CheckedAt: &now, ProviderReference: "test-fixture:local-host", ProviderStatus: "synthetic_local", TLSStatus: "synthetic_local", Version: 1, CreatedAt: now}
	if err = f.a.DB.Table("branding_domains").Create(&domain).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = f.a.Cooperation.DeliverPurchase(ctx, nil, cooperation.PurchaseInput{BuyerID: oem.Operator.ID, SupplierID: platform, OrderID: uuid.NewString(), ReceiptReference: "synthetic-operator-opening-inventory", ActorID: platformOwner, PaidUSD: d("800"), PaidATG: d("1000"), ExitTerms: json.RawMessage(`{"version":1,"voluntary_refund":false,"method":"non_refundable","terms_reference":"synthetic-opening"}`), At: now}); err != nil {
		t.Fatal(err)
	}
	if err = f.a.Catalog.SetAuthorization(ctx, platform, oem.Operator.ID, model, true, 0); err != nil {
		t.Fatal(err)
	}
	if err = f.a.Catalog.SetListing(ctx, oem.Operator.ID, model, true, 0); err != nil {
		t.Fatal(err)
	}
	for _, scope := range []catalog.PriceScope{{Kind: "supply", SellerID: platform, BuyerID: oem.Operator.ID}, {Kind: "retail", SellerID: oem.Operator.ID}, {Kind: "supply", SellerID: oem.Operator.ID, BuyerID: channel.Operator.ID}, {Kind: "supply", SellerID: oem.Operator.ID, BuyerID: spare.Operator.ID}} {
		rate := "0.002"
		if scope.SellerID == platform {
			rate = "0.001"
		}
		price := catalog.PriceVersion{ID: uuid.NewString(), Scope: scope, ModelID: model, Layer: "regular", EffectiveAt: now.Add(-time.Minute), Published: true, NoticeID: "synthetic opening supply price", Rule: catalog.PriceRule{Mode: "manual", Components: []catalog.ComponentRate{{Key: "input", Unit: "token", Per: "1", Rate: rate}, {Key: "output", Unit: "token", Per: "1", Rate: rate}}}}
		if err = f.a.Catalog.PublishPrice(ctx, scope.SellerID, price, f.a.pricePublished); err != nil {
			t.Fatal(err)
		}
	}
	manual := func(actor, operator, name string) string {
		t.Helper()
		merchant, e := f.a.Payments.SaveMerchant(ctx, actor, payments.Merchant{ID: uuid.NewString(), OperatorID: operator, Driver: "manual", Enabled: true, Credentials: map[string]string{"currency": "USD", "recipient_reference": name + " 合成收款账号", "instructions": "隔离仿真，无真实资金转移"}}, 0)
		if e != nil {
			t.Fatal(e)
		}
		return merchant.ID
	}
	oemMerchant := manual(oemOwner.ID, oem.Operator.ID, "山海 OEM")
	channelMerchant := manual(channelOwner.ID, channel.Operator.ID, "星河渠道")
	for _, pair := range [][2]string{{oem.Operator.ID, oemOwner.ID}, {channel.Operator.ID, channelOwner.ID}, {spare.Operator.ID, spareOwner.ID}} {
		if _, err = f.a.Incentives.SetPolicy(ctx, nil, pair[1], pair[0], incentives.PolicyInput{ThresholdUSD: d("60"), DirectRate: d("0.1"), IndirectRate: d("0.02"), RewardValiditySeconds: 86400, Evidence: "synthetic opening incentive terms"}); err != nil {
			t.Fatal(err)
		}
	}
	state, err := f.a.Organization.GetBusinessStatus(ctx, nil, channel.Operator.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.a.Organization.SetBusinessStatus(ctx, nil, organization.SetBusinessStatusInput{ActorStaffID: channelOwner.ID, OperatorID: channel.Operator.ID, Feature: organization.Registration, Enabled: true, ExpectedVersion: state.Version, EvidenceReference: "synthetic existing customer registration"}, f.a); err != nil {
		t.Fatal(err)
	}
	ch, err := f.a.Organization.BeginRegistration(ctx, nil, organization.RegistrationIntentRequest{BrandID: oem.Brand.ID, Email: "sim-channel-customer@operator.test", Password: "simulation-customer-password", InviteCode: channel.InviteCode})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.a.Organization.VerifyRegistration(ctx, nil, ch.Intent.ID, ch.VerificationToken); err != nil {
		t.Fatal(err)
	}
	customer, err := f.a.Organization.CompleteRegistration(ctx, nil, organization.CompleteRegistrationInput{IntentID: ch.Intent.ID, VerificationToken: ch.VerificationToken, DisplayName: "星河真实流程客户"}, f.a.Incentives)
	if err != nil {
		t.Fatal(err)
	}
	person := func(p Staff) map[string]any {
		return map[string]any{"id": p.ID, "email": p.Email, "password": password}
	}
	return map[string]any{"origin": "http://operators.localhost:13003", "oem_id": oem.Operator.ID, "brand_id": oem.Brand.ID, "channel_id": channel.Operator.ID, "channel_code": channel.InviteCode, "other_channel_id": spare.Operator.ID, "model_id": model, "oem_manual_method_id": oemMerchant, "channel_manual_method_id": channelMerchant, "oem_owner": person(oemOwner), "channel_owner": person(channelOwner), "other_owner": person(spareOwner), "finance": person(finance), "support": person(support), "readonly": person(readonly), "customer": map[string]any{"id": customer.Customer.ID, "email": "sim-channel-customer@operator.test", "password": "simulation-customer-password"}}
}

func TestPostgresIndependentChannelOwnerCannotCreateDescendantsOrExposeParentCustomerAuthority(t *testing.T) {
	f := setupPortal(t)
	ctx := context.Background()
	parent, parentOwner := f.a.Installation.OperatorID, f.a.Installation.OwnerStaffID
	channel, err := f.a.Organization.CreateOperator(ctx, nil, organization.CreateOperatorInput{ActorStaffID: parentOwner, ParentOperatorID: parent, Kind: organization.Channel, Name: "isolated channel"})
	if err != nil {
		t.Fatal(err)
	}
	password := "independent-channel-password"
	hash, err := organization.HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	owner := Staff{ID: uuid.NewString(), Email: "independent-channel@operator.test", DisplayName: "Independent channel owner", PasswordHash: hash, Version: 1, Status: "active", EmailVerified: true, CreatedAt: time.Now()}
	if err = f.a.DB.Create(&owner).Error; err != nil {
		t.Fatal(err)
	}
	invitation, err := f.a.Organization.InviteStaff(ctx, nil, parentOwner, channel.Operator.ID, owner.Email, "owner")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.a.Organization.AcceptStaffInvitation(ctx, nil, invitation.Token, organization.VerifiedStaff{ID: owner.ID, Email: owner.Email, EmailVerified: true}); err != nil {
		t.Fatal(err)
	}
	f.login(t, "staff", "owner@example.test", "owner-password-test")
	status, out, _ := f.call(t, "GET", "/v1/operators/"+channel.Operator.ID+"/customers", nil, "")
	if status != 403 {
		t.Fatalf("parent acquired child customers: %d %#v", status, out)
	}
	f.login(t, "staff", owner.Email, password)
	base := "/v1/operators/" + channel.Operator.ID + "/operators"
	status, out, _ = f.call(t, "GET", base, nil, "")
	mustOK(t, status, out)
	if len(out["operations"].([]any)) != 0 {
		t.Fatal("channel was offered descendant creation", out["operations"])
	}
	status, out, _ = f.call(t, "POST", base, map[string]any{"kind": "channel", "name": "forbidden descendant", "legal_name": "forbidden"}, "")
	if status != 400 || out["error"].(map[string]any)["code"] != "invalid_input" {
		t.Fatalf("invalid hierarchy must be a final user error, not an unknown financial result: %d %#v", status, out)
	}
	children, err := f.a.Organization.Children(ctx, nil, channel.Operator.ID)
	if err != nil || len(children) != 0 {
		t.Fatalf("channel created descendants: %v %#v", err, children)
	}
	status, out, _ = f.call(t, "GET", "/v1/operators/"+parent+"/customers", nil, "")
	if status != 403 {
		t.Fatalf("child acquired parent customers: %d %#v", status, out)
	}
	oem, err := f.a.Organization.CreateOperator(ctx, nil, organization.CreateOperatorInput{ActorStaffID: parentOwner, ParentOperatorID: parent, Kind: organization.OEM, Name: "independent OEM", BrandName: "independent OEM brand"})
	if err != nil {
		t.Fatal(err)
	}
	invitation, err = f.a.Organization.InviteStaff(ctx, nil, parentOwner, oem.Operator.ID, owner.Email, "owner")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.a.Organization.AcceptStaffInvitation(ctx, nil, invitation.Token, organization.VerifiedStaff{ID: owner.ID, Email: owner.Email, EmailVerified: true}); err != nil {
		t.Fatal(err)
	}
	status, out, _ = f.call(t, "GET", "/v1/operators/"+oem.Operator.ID+"/operators", nil, "")
	mustOK(t, status, out)
	operations := out["operations"].([]any)
	if len(operations) != 1 || operations[0].(map[string]any)["label"] != "建立直属渠道" {
		t.Fatal("OEM offered unrelated organization types", operations)
	}
	fixed := false
	for _, raw := range operations[0].(map[string]any)["fields"].([]any) {
		input := raw.(map[string]any)
		if input["name"] == "brand_name" || input["name"] == "api_base_url" {
			t.Fatal("channel creation asks for independent brand material", input)
		}
		if input["name"] == "kind" {
			fixed = input["kind"] == "hidden" && input["default_value"] == "channel"
		}
	}
	if !fixed {
		t.Fatal("OEM channel type was not fixed")
	}
}

func TestRegistrationBoundaryErrorsHaveFinalActionableHTTPResponses(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
		code   string
	}{
		{organization.ErrEmailAlreadyRegistered, 409, "email_already_registered"},
		{organization.ErrInvalidInviteCode, 400, "invalid_invite_code"},
		{organization.ErrInvalidVerification, 400, "invalid_verification"},
		{organization.ErrInvalidRelationship, 400, "invalid_input"},
	} {
		t.Run(tc.code+"/"+tc.err.Error(), func(t *testing.T) {
			response := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(response)
			handleError(c, tc.err)
			var data struct {
				Error struct {
					Code      string `json:"code"`
					Retryable bool   `json:"retryable"`
				} `json:"error"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &data); err != nil {
				t.Fatal(err)
			}
			if response.Code != tc.status || data.Error.Code != tc.code || data.Error.Retryable {
				t.Fatalf("unexpected final boundary error: %d %s", response.Code, response.Body.String())
			}
		})
	}
}
