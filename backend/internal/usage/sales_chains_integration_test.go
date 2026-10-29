package usage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/cooperation"
	"github.com/tokpath/tokstudio/backend/internal/incentives"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/modelpricing"
	"github.com/tokpath/tokstudio/backend/internal/modelruntime"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"gorm.io/gorm"
)

// Real business services share one isolated PostgreSQL schema. Initial wholesale
// inventory represents independently verified purchases; retail funding uses the
// authorized offline-receipt workflow. Model traffic goes through Bifrost to the
// separately deployed HTTP service, and replay reads encrypted private S3 data.
func TestExternalFourSalesChainsKeepPricesFundsSourcesAndCommissionsSeparate(t *testing.T) {
	base := strings.TrimRight(os.Getenv("TOKENHUB_MODEL_MOCK_URL"), "/")
	if base == "" {
		t.Skip("separately deployed model mock required")
	}
	origin, e := url.Parse(base)
	if e != nil || origin.Hostname() != "127.0.0.1" {
		t.Fatal("isolated loopback mock required")
	}
	f := setup(t, "0.01", nil)
	if f.s.ResultStore == nil {
		t.Fatal("private S3 integration required")
	}
	_, migrations := incentives.Migrations()
	files, e := fs.ReadDir(migrations, ".")
	if e != nil {
		t.Fatal(e)
	}
	for _, file := range files {
		b, e := fs.ReadFile(migrations, file.Name())
		if e != nil {
			t.Fatal(e)
		}
		if e = f.db.Exec(string(b)).Error; e != nil {
			t.Fatal(e)
		}
	}
	inc := incentives.New(f.db)
	inc.SetRegistrationCoverage(f.coop)
	c := commerce.New(f.db)
	c.SetSalesCoverage(f.coop)
	c.SetMarketing(inc)
	f.s.GrantObserver = inc
	runtime, e := modelruntime.NewBifrost(f.ctx, modelruntime.BifrostConfig{Providers: []string{"openai"}, AllowTestLoopback: true, Credentials: f.cat, Costs: &modelpricing.Service{}, EndpointGuard: func(_ context.Context, target string) error {
		u, e := url.Parse(target)
		if e != nil || u.Host != origin.Host {
			return modelruntime.ErrInvalidRequest
		}
		return nil
	}})
	if e != nil {
		t.Fatal(e)
	}
	defer runtime.Close()
	f.s.Runtime = runtime
	type chain struct{ op, brand, owner, invite, parent, retail, supply, cost string }
	platform := chain{op: f.operator, brand: f.brand, owner: f.owner, retail: "0.017"}
	newOperator := func(parent chain, kind organization.OperatorKind, name, retail, supply, cost string) chain {
		t.Helper()
		created, e := f.org.CreateOperator(f.ctx, nil, organization.CreateOperatorInput{ActorStaffID: parent.owner, ParentOperatorID: parent.op, Kind: kind, Name: name, BrandName: name})
		if e != nil {
			t.Fatal(e)
		}
		owner := uuid.NewString()
		email := name + "@example.test"
		inv, e := f.org.InviteStaff(f.ctx, nil, parent.owner, created.Operator.ID, email, "owner")
		if e != nil {
			t.Fatal(e)
		}
		if _, e = f.org.AcceptStaffInvitation(f.ctx, nil, inv.Token, organization.VerifiedStaff{ID: owner, Email: email, EmailVerified: true}); e != nil {
			t.Fatal(e)
		}
		if e = f.cat.SetAuthorization(f.ctx, parent.op, created.Operator.ID, "public-model", true, 0); e != nil {
			t.Fatal(e)
		}
		if e = f.cat.SetListing(f.ctx, created.Operator.ID, "public-model", true, 0); e != nil {
			t.Fatal(e)
		}
		return chain{op: created.Operator.ID, brand: created.Brand.ID, owner: owner, invite: created.InviteCode, parent: parent.op, retail: retail, supply: supply, cost: cost}
	}
	direct := newOperator(platform, organization.Channel, "direct-channel", "0.034", "0.001", "700")
	oem := newOperator(platform, organization.OEM, "oem", "0.051", "0.0008", "800")
	nested := newOperator(oem, organization.Channel, "oem-channel", "0.068", "0.002", "600")
	chains := []chain{platform, direct, oem, nested}
	// The initial catalog has no sold downstream responsibilities. Publication
	// verifies that fact; after all required prices exist, real coverage is used
	// for every purchase, grant, registration reward and model call.
	for _, ch := range chains[1:] {
		retailRate := map[string]string{"0.034": "0.002", "0.051": "0.003", "0.068": "0.004"}[ch.retail]
		for _, p := range []struct {
			scope       catalog.PriceScope
			rate, actor string
		}{{catalog.PriceScope{Kind: "retail", SellerID: ch.op}, retailRate, ch.op}, {catalog.PriceScope{Kind: "supply", SellerID: ch.parent, BuyerID: ch.op}, ch.supply, ch.parent}} {
			v := catalog.PriceVersion{ID: uuid.NewString(), Scope: p.scope, ModelID: "public-model", Layer: "regular", EffectiveAt: time.Now().Add(-time.Minute), Published: true, NoticeID: "initial-published-terms", Rule: catalog.PriceRule{Mode: "manual", Components: []catalog.ComponentRate{{Key: "input", Unit: "token", Per: "1", Rate: p.rate}, {Key: "output", Unit: "token", Per: "1", Rate: p.rate}, {Key: "cache", Unit: "token", Per: "1", Rate: p.rate}}}}
			if e = f.cat.PublishPrice(f.ctx, p.actor, v, func(ctx context.Context, tx *gorm.DB, _ catalog.PriceVersion) error {
				var obligations int64
				if e := tx.WithContext(ctx).Model(&ledger.Grant{}).Where("operator_id=?", ch.op).Count(&obligations).Error; e != nil {
					return e
				}
				if obligations != 0 {
					return fmt.Errorf("initial price has existing obligations")
				}
				return nil
			}); e != nil {
				t.Fatal(e)
			}
		}
	}
	for _, ch := range []chain{oem, direct, nested} {
		if _, e = f.coop.DeliverPurchase(f.ctx, nil, cooperation.PurchaseInput{BuyerID: ch.op, SupplierID: ch.parent, OrderID: "funded-stock-" + ch.op, ReceiptReference: "verified-bank-" + ch.op, ActorID: ch.owner, PaidMoney: d(ch.cost), PaidATG: d("800"), GiftATG: d("200"), ExitTerms: json.RawMessage(`{"voluntary_refund":false}`), At: time.Now()}); e != nil {
			t.Fatal(e)
		}
	}
	for _, ch := range chains {
		for _, feature := range []organization.BusinessFeature{organization.Registration, organization.ModelUsage, organization.NewPurchase, organization.Activity} {
			st, e := f.org.GetBusinessStatus(f.ctx, nil, ch.op)
			if e != nil {
				t.Fatal(e)
			}
			if _, e = f.org.SetBusinessStatus(f.ctx, nil, organization.SetBusinessStatusInput{ActorStaffID: ch.owner, OperatorID: ch.op, Feature: feature, Enabled: true, ExpectedVersion: st.Version, EvidenceReference: "configured integration environment"}, ready{}); e != nil {
				t.Fatal(e)
			}
		}
		if _, e = inc.SetPolicy(f.ctx, nil, ch.owner, ch.op, incentives.PolicyInput{ThresholdMoney: ledger.Zero(), DirectRate: d("0.15"), IndirectRate: d("0.05"), RewardValiditySeconds: 3600, Evidence: "test published incentive policy"}); e != nil {
			t.Fatal(e)
		}
	}
	for n, ch := range chains {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			register := func(label, code string) organization.RegistrationResult {
				t.Helper()
				intent, e := f.org.BeginRegistration(f.ctx, nil, organization.RegistrationIntentRequest{BrandID: ch.brand, Email: label + ch.op + "@example.test", Password: "integration-strong-password", InviteCode: code})
				if e != nil {
					t.Fatal(e)
				}
				if _, e = inc.PrepareRegistrationPromise(f.ctx, nil, intent.Intent); e != nil {
					t.Fatal(e)
				}
				if _, e = f.org.VerifyEmail(f.ctx, nil, intent.Intent.ID, intent.VerificationToken); e != nil {
					t.Fatal(e)
				}
				r, e := f.org.CompleteRegistration(f.ctx, nil, organization.CompleteRegistrationInput{IntentID: intent.Intent.ID, VerificationToken: intent.VerificationToken}, inc)
				if e != nil {
					t.Fatal(e)
				}
				if r.Ownership.OperatorID != ch.op {
					t.Fatal("incorrect registration owner")
				}
				return r
			}
			grand := register("grand", ch.invite)
			parent := register("parent", grand.InviteCode)
			customer := register("buyer", parent.InviteCode)
			for _, id := range []string{grand.Customer.ID, parent.Customer.ID} {
				q, e := inc.GetQualification(f.ctx, nil, ch.op, id)
				if e != nil || q.AcquiredAt == nil {
					t.Fatalf("zero threshold KOL qualification %v %+v", e, q)
				}
			}
			q, e := c.PublishPackage(f.ctx, nil, commerce.PublishPackageInput{ActorStaffID: ch.owner, OperatorID: ch.op, Name: "Chain package", TermsReference: "fixed-order-terms", Timezone: "UTC", PriceMoney: d("60"), PaidATG: d("80"), GiftATG: d("20"), Period: ledger.Month, NewPurchaseEnabled: true})
			if e != nil {
				t.Fatal(e)
			}
			o, e := c.CreateOrder(f.ctx, nil, commerce.CreateOrderInput{CustomerID: customer.Customer.ID, OperatorID: ch.op, QuoteID: q.ID, BusinessKey: "paid-retail-order", ExpectedOwnershipVersion: 1})
			if e != nil {
				t.Fatal(e)
			}
			gross, zero := d("60"), ledger.Zero()
			fact := commerce.VerifiedReceipt{OperatorID: ch.op, ProviderReceiptID: "bank-receipt-" + ch.op, GrossMoney: gross, NetMoney: &gross, FeeMoney: &zero, Currency: "USD", OriginalAmount: "60", RecipientReference: "owned-bank-account-" + ch.op, PayerReference: "external-payer", ValuationEvidence: "USD original payment", VerificationEvidence: "synthetic bank statement verified by operator", PaidAt: &o.CreatedAt, SettledAt: &o.CreatedAt}
			receipt, e := c.RecordManualReceipt(f.ctx, nil, ch.owner, fact)
			if e != nil {
				t.Fatal(e)
			}
			duplicate, e := c.RecordManualReceipt(f.ctx, nil, ch.owner, fact)
			if e != nil || duplicate.ID != receipt.ID {
				t.Fatal("receipt replay", e)
			}
			if _, e = c.AllocateReceipt(f.ctx, nil, commerce.AllocateReceiptInput{ActorStaffID: ch.owner, OperatorID: ch.op, BusinessKey: "allocation", ReceiptID: receipt.ID, OrderID: o.ID, AmountMoney: gross}); e != nil {
				t.Fatal(e)
			}
			if _, e = c.Fulfill(f.ctx, nil, o.ID, ch.owner); e != nil {
				t.Fatal(e)
			}
			if _, e = c.Fulfill(f.ctx, nil, o.ID, ch.owner); e != nil {
				t.Fatal("fulfillment replay", e)
			}
			key, e := f.s.CreateKey(f.ctx, customer.Customer.ID, "one-key", KeyInput{Name: "Multi-model key", Period: ledger.Month, Timezone: "UTC"})
			if e != nil {
				t.Fatal(e)
			}
			if _, e = f.s.AuthenticateKey(f.ctx, ch.brand, key.Secret); e != nil {
				t.Fatal(e)
			}
			if ch.brand != platform.brand {
				if _, e = f.s.AuthenticateKey(f.ctx, platform.brand, key.Secret); e == nil {
					t.Fatal("cross-brand key accepted")
				}
			}
			before := map[string]cooperation.ProcurementPosition{}
			var positions []string
			if n == 1 {
				positions = []string{direct.op}
			}
			if n == 2 {
				positions = []string{oem.op}
			}
			if n == 3 {
				positions = []string{oem.op, nested.op}
			}
			for _, buyer := range positions {
				p, e := f.coop.Position(f.ctx, nil, buyer)
				if e != nil {
					t.Fatal(e)
				}
				before[buyer] = p
			}
			ex, e := f.s.Execute(f.ctx, key.Key, "actual-model-call", request(t, false), nil)
			if e != nil {
				t.Fatal(e)
			}
			if ex.Request.Status != "completed" || ex.Request.ChargedATG.String() != ch.retail {
				t.Fatalf("wrong retail price %+v", ex.Request)
			}
			rows, e := f.s.OperationalRequests(f.ctx, nil, ch.op, customer.Customer.ID, time.Now().Add(-time.Hour), time.Now().Add(time.Minute), 100)
			if e != nil || len(rows) != 1 {
				t.Fatal("own request export", rows, e)
			}
			ownCost := map[string]string{"0.017": "0", "0.034": "0.0119", "0.051": "0.01088", "0.068": "0.0204"}[ch.retail]
			if rows[0].FulfillmentCostMoney.String() != ownCost {
				t.Fatal("export did not use own historical cost", rows[0])
			}
			exported, e := json.Marshal(rows[0])
			if e != nil {
				t.Fatal(e)
			}
			if ch.op != f.operator && (len(rows[0].SupplierReferenceCosts) > 0 || len(rows[0].SupplierReportedCosts) > 0 || rows[0].UnverifiedAttempts != nil || rows[0].EstimateATG != nil || strings.Contains(string(exported), "supplier_reference_costs") || strings.Contains(string(exported), "unverified_attempts") || strings.Contains(string(exported), "estimate_atg")) {
				t.Fatal("downstream export leaked platform provider economics", string(exported))
			}
			if ch.op == f.operator && (len(rows[0].SupplierReferenceCosts) != 1 || rows[0].SupplierReferenceCosts[0].Currency != "USD" || rows[0].SupplierReferenceCosts[0].Amount != "0.017" || rows[0].EstimateATG == nil || rows[0].EstimateATG.Sign() <= 0) {
				t.Fatal("platform lost actual supplier cost or reservation", rows[0])
			}
			replay, e := f.s.Execute(f.ctx, key.Key, "actual-model-call", request(t, false), nil)
			if e != nil || !replay.Replay {
				t.Fatal("model replay", e)
			}
			// Make each real procurement edge unavailable in turn, including
			// the OEM's platform stock in the three-level chain. A failed
			// admission must roll back the customer's and every other edge's
			// reservations before any provider dispatch.
			for _, blockedBuyer := range positions {
				p, e := f.coop.Position(f.ctx, nil, blockedBuyer)
				if e != nil {
					t.Fatal(e)
				}
				hold, e := f.coop.ReserveProcurement(f.ctx, nil, blockedBuyer, "block-edge-"+ch.op, p.Quantity, time.Now().Add(time.Minute))
				if e != nil {
					t.Fatal(e)
				}
				_, e = f.s.Execute(f.ctx, key.Key, "unavailable-edge-"+blockedBuyer, request(t, false), nil)
				if !errors.Is(e, ledger.ErrInsufficient) {
					t.Fatal("unfunded supply edge was not rejected", blockedBuyer, e)
				}
				if _, e = f.l.Release(f.ctx, nil, blockedBuyer, hold.ID, "unblock-edge-"+ch.op); e != nil {
					t.Fatal(e)
				}
				var extra int64
				if e = f.db.Model(&Request{}).Where("key_id=? AND business_key LIKE 'unavailable-edge-%'", key.Key.ID).Count(&extra).Error; e != nil || extra != 0 {
					t.Fatal("failed admission persisted a callable request", extra, e)
				}
				for _, buyer := range positions {
					position, e := f.coop.Position(f.ctx, nil, buyer)
					if e != nil {
						t.Fatal(e)
					}
					account, e := f.l.AccountInTransaction(f.ctx, nil, position.AccountID)
					if e != nil || !account.Held.IsZero() {
						t.Fatal("failed chain admission left procurement reserved", account, e)
					}
				}
			}
			for _, buyer := range positions {
				p, e := f.coop.Position(f.ctx, nil, buyer)
				if e != nil {
					t.Fatal(e)
				}
				expected := "0.0136"
				if buyer == direct.op {
					expected = "0.017"
				}
				if buyer == nested.op {
					expected = "0.034"
				}
				if before[buyer].Quantity.Sub(p.Quantity).String() != expected {
					t.Fatalf("wrong wholesale charge %s: %s", buyer, before[buyer].Quantity.Sub(p.Quantity))
				}
			}
			for _, id := range []string{customer.Customer.ID, parent.Customer.ID, grand.Customer.ID} {
				if _, e = inc.GetQualification(f.ctx, nil, ch.op, id); e != nil {
					t.Fatal(e)
				}
			}
			var count int64
			if e = f.db.Table("incentives_commission_entries").Where("operator_id=?", ch.op).Count(&count).Error; e != nil || count != 2 {
				t.Fatalf("two hops %d: %v", count, e)
			}
			var total ledger.Decimal
			if e = f.db.Raw("SELECT sum(amount_money)::text FROM incentives_commission_entries WHERE operator_id=?", ch.op).Row().Scan(&total); e != nil {
				t.Fatal(e)
			}
			// 60USD / 100ATG * actual retail debit * (15% + 5%).
			expected := map[string]string{"0.017": "0.00204", "0.034": "0.00408", "0.051": "0.00612", "0.068": "0.00816"}[ch.retail]
			if total.String() != expected {
				t.Fatalf("commission %s expected %s", total, expected)
			}
			var accounts []ledger.Account
			if e = f.db.Where("operator_id=?", ch.op).Find(&accounts).Error; e != nil {
				t.Fatal(e)
			}
			for _, a := range accounts {
				if e = f.l.Reconcile(f.ctx, nil, a.ID); e != nil {
					t.Fatal(e)
				}
			}
		})
	}
}
