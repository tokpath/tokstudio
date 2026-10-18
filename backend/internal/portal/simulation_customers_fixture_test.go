package portal

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/cooperation"
	"github.com/tokpath/tokstudio/backend/internal/incentives"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"github.com/tokpath/tokstudio/backend/internal/payments"
)

// seedCustomerSimulation prepares synthetic existing personas and opening
// balances only. Orders, activity use, calls, renewal consent and withdrawals
// are exercised later through the real browser and real HTTP application.
// Every policy belongs to these dedicated channels, never the shared platform.
func (f *testApp) seedCustomerSimulation(t *testing.T, lab string) map[string]any {
	t.Helper()
	ctx := context.Background()
	platform, owner := f.a.Installation.OperatorID, f.a.Installation.OwnerStaffID
	models, err := f.a.Catalog.PublicCatalog(ctx, platform, time.Now())
	if err != nil || len(models) == 0 {
		t.Fatalf("customer simulation requires prepared external model: %v", err)
	}
	model := models[0].ID
	password := "simulation-customer-password"
	d := ledger.MustDecimal
	type preparedChannel struct {
		created  organization.OperatorCreated
		quote    commerce.PackageQuote
		policy   incentives.Policy
		merchant string
	}
	channel := func(name string) preparedChannel {
		t.Helper()
		created, e := f.a.Organization.CreateOperator(ctx, nil, organization.CreateOperatorInput{ActorStaffID: owner, ParentOperatorID: platform, Kind: organization.Channel, Name: name, LegalName: name + " 合成主体"})
		if e != nil {
			t.Fatal(e)
		}
		op := created.Operator.ID
		invitation, e := f.a.Organization.InviteStaff(ctx, nil, owner, op, "owner@example.test", "owner")
		if e != nil {
			t.Fatal(e)
		}
		if _, e = f.a.Organization.AcceptStaffInvitation(ctx, nil, invitation.Token, organization.VerifiedStaff{ID: owner, Email: "owner@example.test", EmailVerified: true}); e != nil {
			t.Fatal(e)
		}
		if _, e = f.a.Cooperation.DeliverPurchase(ctx, nil, cooperation.PurchaseInput{BuyerID: op, SupplierID: platform, OrderID: uuid.NewString(), ReceiptReference: "synthetic-opening-inventory", ActorID: owner, PaidUSD: d("10000"), PaidATG: d("10000"), ExitTerms: json.RawMessage(`{"version":1,"voluntary_refund":false,"method":"non_refundable","terms_reference":"synthetic-opening"}`), At: time.Now()}); e != nil {
			t.Fatal(e)
		}
		if e = f.a.Catalog.SetAuthorization(ctx, platform, op, model, true, 0); e != nil {
			t.Fatal(e)
		}
		if e = f.a.Catalog.SetListing(ctx, op, model, true, 0); e != nil {
			t.Fatal(e)
		}
		for _, scope := range []catalog.PriceScope{{Kind: "supply", SellerID: platform, BuyerID: op}, {Kind: "retail", SellerID: op}} {
			actor := op
			if scope.Kind == "supply" {
				actor = platform
			}
			p := catalog.PriceVersion{ID: uuid.NewString(), Scope: scope, ModelID: model, Layer: "regular", EffectiveAt: time.Now().Add(-time.Minute), Published: true, NoticeID: "explicit synthetic simulation pricing", Rule: catalog.PriceRule{Mode: "manual", Components: []catalog.ComponentRate{{Key: "input", Unit: "token", Per: "1", Rate: "0.001"}, {Key: "output", Unit: "token", Per: "1", Rate: "0.001"}}}}
			if e = f.a.Catalog.PublishPrice(ctx, actor, p, f.a.pricePublished); e != nil {
				t.Fatal(e)
			}
		}
		quote, e := f.a.Commerce.PublishPackage(ctx, nil, commerce.PublishPackageInput{ActorStaffID: owner, OperatorID: op, Name: name + " 月套餐", TermsReference: "synthetic-customer-package-v1", Timezone: "Asia/Shanghai", PriceUSD: d("80"), PaidATG: d("80"), GiftATG: d("20"), Period: ledger.Month, NewPurchaseEnabled: true, RenewalEnabled: true})
		if e != nil {
			t.Fatal(e)
		}
		policy, e := f.a.Incentives.SetPolicy(ctx, nil, owner, op, incentives.PolicyInput{ThresholdUSD: d("60"), DirectRate: d("0.15"), IndirectRate: d("0.05"), ConfirmationSeconds: 5, InviteeATG: ledger.Zero(), InviterATG: ledger.Zero(), RewardValiditySeconds: 86400, KOLCrossActivityLimit: 3, Evidence: "isolated synthetic customer policy"})
		if e != nil {
			t.Fatal(e)
		}
		merchant, e := f.a.Payments.SaveMerchant(ctx, owner, payments.Merchant{ID: uuid.NewString(), OperatorID: op, Driver: "manual", Enabled: true, Credentials: map[string]string{"currency": "USD", "recipient_reference": name + " 合成收款账号", "instructions": "仅供隔离仿真，无真实资金转移"}}, 0)
		if e != nil {
			t.Fatal(e)
		}
		for _, feature := range []organization.BusinessFeature{organization.Registration, organization.NewPurchase, organization.Renewal, organization.ModelUsage, organization.Activity} {
			state, e := f.a.Organization.GetBusinessStatus(ctx, nil, op)
			if e != nil {
				t.Fatal(e)
			}
			if _, e = f.a.Organization.SetBusinessStatus(ctx, nil, organization.SetBusinessStatusInput{ActorStaffID: owner, OperatorID: op, Feature: feature, Enabled: true, ExpectedVersion: state.Version, EvidenceReference: "synthetic simulation readiness"}, f.a); e != nil {
				t.Fatal(e)
			}
		}
		if _, e = f.a.Incentives.SetWithdrawalPolicy(ctx, nil, owner, op, incentives.WithdrawalPolicy{MinimumUSD: d("0.00000001"), FlatFeeUSD: ledger.Zero(), FeeRate: ledger.Zero(), MethodReference: "隔离仿真人工付款：未支付取消释放全部占用"}, 0); e != nil {
			t.Fatal(e)
		}
		return preparedChannel{created: created, quote: quote, policy: policy, merchant: merchant.ID}
	}
	a, b := channel("客户仿真渠道 A"), channel("客户仿真渠道 B")
	register := func(email, name, code string) organization.RegistrationResult {
		t.Helper()
		challenge, e := f.a.Organization.BeginRegistration(ctx, nil, organization.RegistrationIntentRequest{BrandID: f.a.Installation.BrandID, Email: email, Password: password, InviteCode: code})
		if e != nil {
			t.Fatal(e)
		}
		if _, e = f.a.Incentives.PrepareRegistrationPromise(ctx, nil, challenge.Intent); e != nil {
			t.Fatal(e)
		}
		if _, e = f.a.Organization.VerifyRegistration(ctx, nil, challenge.Intent.ID, challenge.VerificationToken); e != nil {
			t.Fatal(e)
		}
		result, e := f.a.Organization.CompleteRegistration(ctx, nil, organization.CompleteRegistrationInput{IntentID: challenge.Intent.ID, VerificationToken: challenge.VerificationToken, DisplayName: name}, f.a.Incentives)
		if e != nil {
			t.Fatal(e)
		}
		return result
	}
	upstream := register("sim-kol-upstream@customer.test", "合成上级 KOL", a.created.InviteCode)
	kol := register("sim-kol-direct@customer.test", "合成直属 KOL", upstream.InviteCode)
	other := register("sim-kol-other@customer.test", "合成其他 KOL", a.created.InviteCode)
	buyer := register("sim-invited@customer.test", "合成受邀客户", kol.InviteCode)
	ordinary := register("sim-ordinary@customer.test", "合成普通邀请人", a.created.InviteCode)
	ordinaryChild := register("sim-ordinary-child@customer.test", "合成普通受邀客户", ordinary.InviteCode)
	foreign := register("sim-foreign@customer.test", "合成异渠道客户", b.created.InviteCode)
	for _, person := range []organization.RegistrationResult{upstream, kol, other} {
		acquired := time.Now().Add(-time.Hour)
		if e := f.a.DB.Exec("INSERT INTO incentives_qualification_accounts(operator_id,customer_id,confirmed_usd,acquired_at,acquired_policy_id,pending_offset_usd) VALUES(?,?,?,?,?,0) ON CONFLICT(operator_id,customer_id) DO UPDATE SET confirmed_usd=EXCLUDED.confirmed_usd, acquired_at=EXCLUDED.acquired_at,acquired_policy_id=EXCLUDED.acquired_policy_id", a.created.Operator.ID, person.Customer.ID, "100", acquired, a.policy.ID).Error; e != nil {
			t.Fatal(e)
		}
	}
	seedMoney := func(operator, customer, purpose, amount string) {
		t.Helper()
		// USD opening assets retain verified synthetic receipt provenance, just
		// like an existing top-up. A bare ledger balance is intentionally not
		// accepted by production order funding.
		order, e := f.a.Commerce.CreateTopUpOrder(ctx, nil, commerce.CreateTopUpInput{OperatorID: operator, CustomerID: customer, BusinessKey: "synthetic-opening:" + uuid.NewString(), ExpectedOwnershipVersion: 1, AmountUSD: d(amount)})
		if e != nil {
			t.Fatal(e)
		}
		paidAt := order.CreatedAt
		receipt, e := f.a.Commerce.RecordManualReceipt(ctx, nil, owner, commerce.VerifiedReceipt{OperatorID: operator, ProviderReceiptID: "synthetic-opening:" + uuid.NewString(), GrossUSD: d(amount), FeeStatus: "pending", Currency: "USD", OriginalAmount: amount, RecipientReference: "synthetic channel account", PayerReference: "synthetic customer's verified account", ValuationEvidence: "synthetic USD no FX", VerificationEvidence: "explicit synthetic pre-existing wallet funding fixture", PaidAt: &paidAt})
		if e != nil {
			t.Fatal(e)
		}
		if _, e = f.a.Commerce.AllocateReceipt(ctx, nil, commerce.AllocateReceiptInput{ActorStaffID: owner, OperatorID: operator, BusinessKey: "synthetic-opening-allocation:" + uuid.NewString(), ReceiptID: receipt.ID, OrderID: order.ID, AmountUSD: d(amount)}); e != nil {
			t.Fatal(e)
		}
		if _, e = f.a.Commerce.Fulfill(ctx, nil, order.ID, "test:synthetic-existing-wallet"); e != nil {
			t.Fatal(e)
		}
	}
	for _, person := range []organization.RegistrationResult{buyer, ordinaryChild} {
		seedMoney(a.created.Operator.ID, person.Customer.ID, "customer:usd", "240")
	}
	seedMoney(b.created.Operator.ID, foreign.Customer.ID, "customer:usd", "240")
	price := d("80")
	merchant, e := f.a.Incentives.PublishCampaign(ctx, nil, incentives.CampaignInput{ActorStaffID: owner, OperatorID: a.created.Operator.ID, Kind: "merchant", PackageID: a.quote.PackageID, Eligibility: "any", Enabled: true, StartsAt: time.Now().Add(-time.Hour), EndsAt: time.Now().Add(24 * time.Hour), FinalPriceUSD: &price, GiftATG: d("20"), PerCustomerLimit: 10, TotalUseLimit: 100, GiftBudgetATG: d("2000"), DiscountBudgetUSD: ledger.Zero(), Evidence: "synthetic operator gift campaign"})
	if e != nil {
		t.Fatal(e)
	}
	// Existing other-KOL activity is a negative fixture. The direct KOL creates
	// their own activity through the customer UI during browser acceptance.
	otherActivity, e := f.a.Incentives.PublishKOLCampaign(ctx, nil, other.Customer.ID, incentives.CampaignInput{OperatorID: a.created.Operator.ID, PackageID: a.quote.PackageID, Eligibility: "any", Enabled: true, StartsAt: time.Now().Add(-time.Hour), EndsAt: time.Now().Add(24 * time.Hour), ConcessionRate: d("0.1"), PerCustomerLimit: 10, TotalUseLimit: 100, GiftBudgetATG: d("1000"), Evidence: "negative other KOL activity fixture"})
	if e != nil {
		t.Fatal(e)
	}
	person := func(p organization.RegistrationResult) map[string]any {
		var identity struct{ EmailNormalized string }
		if e := f.a.DB.Table("organization_customer_identities").Where("customer_id=? AND is_current=true", p.Customer.ID).Take(&identity).Error; e != nil {
			t.Fatal(e)
		}
		return map[string]any{"id": p.Customer.ID, "email": identity.EmailNormalized, "password": password, "invite_code": p.InviteCode}
	}
	return map[string]any{"operator_id": a.created.Operator.ID, "other_operator_id": b.created.Operator.ID, "channel_code": a.created.InviteCode, "other_channel_code": b.created.InviteCode, "model_id": model, "plan_id": a.quote.PackageID, "other_plan_id": b.quote.PackageID, "manual_method_id": a.merchant, "merchant_campaign_id": merchant.CampaignID, "other_kol_campaign_id": otherActivity.CampaignID, "upstream": person(upstream), "kol": person(kol), "other_kol": person(other), "buyer": person(buyer), "ordinary": person(ordinary), "ordinary_child": person(ordinaryChild), "foreign": person(foreign), "new_email": "sim-browser-registration@customer.test", "new_password": password}
}
