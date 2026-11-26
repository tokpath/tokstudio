package usage

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"github.com/tokpath/tokstudio/backend/internal/cooperation"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/modelruntime"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"gorm.io/gorm"
)

func TestPostgresSubscriptionReservationTopUpIsAtomicAcrossProcurementEdges(t *testing.T) {
	f := setup(t, "100", nil, modelruntime.DefaultBilling(modelruntime.Responses).Capability(modelruntime.Responses, false))
	type operator struct{ id, brand, owner, invite string }
	createOperator := func(parent operator, kind organization.OperatorKind, rate string) operator {
		t.Helper()
		created, err := f.org.CreateOperator(f.ctx, nil, organization.CreateOperatorInput{ActorStaffID: parent.owner,
			ParentOperatorID: parent.id, Kind: kind, Name: string(kind), BrandName: string(kind)})
		if err != nil {
			t.Fatal(err)
		}
		createdOwner, email := uuid.NewString(), string(kind)+"@example.test"
		invitation, err := f.org.InviteStaff(f.ctx, nil, parent.owner, created.Operator.ID, email, "owner")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = f.org.AcceptStaffInvitation(f.ctx, nil, invitation.Token, organization.VerifiedStaff{ID: createdOwner, Email: email, EmailVerified: true}); err != nil {
			t.Fatal(err)
		}
		child := operator{created.Operator.ID, created.Brand.ID, createdOwner, created.InviteCode}
		for _, feature := range []organization.BusinessFeature{organization.Registration, organization.ModelUsage} {
			status, err := f.org.GetBusinessStatus(f.ctx, nil, child.id)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = f.org.SetBusinessStatus(f.ctx, nil, organization.SetBusinessStatusInput{ActorStaffID: child.owner, OperatorID: child.id,
				Feature: feature, Enabled: true, ExpectedVersion: status.Version, EvidenceReference: "isolated test dependencies"}, ready{}); err != nil {
				t.Fatal(err)
			}
		}
		if err = f.cat.SetAuthorization(f.ctx, parent.id, child.id, "public-model", true, 0); err != nil {
			t.Fatal(err)
		}
		if err = f.cat.SetListing(f.ctx, child.id, "public-model", true, 0); err != nil {
			t.Fatal(err)
		}
		for _, scope := range []catalog.PriceScope{{Kind: "supply", SellerID: parent.id, BuyerID: child.id}, {Kind: "retail", SellerID: child.id}} {
			priceRate := rate
			if scope.Kind == "retail" {
				priceRate = "0.004"
			}
			version := catalog.PriceVersion{ID: uuid.NewString(), Scope: scope, ModelID: "public-model", Layer: "regular",
				EffectiveAt: time.Now().Add(-time.Hour), Published: true, NoticeID: "isolated-prices",
				Rule: catalog.PriceRule{Mode: "manual", Components: []catalog.ComponentRate{
					{Key: "input", Unit: "token", Per: "1", Rate: priceRate}, {Key: "output", Unit: "token", Per: "1", Rate: priceRate},
					{Key: "cache", Unit: "token", Per: "1", Rate: priceRate},
				}}}
			if err = f.cat.PublishPrice(f.ctx, scope.SellerID, version, func(_ context.Context, t *gorm.DB, _ catalog.PriceVersion) error {
				var count int64
				if err := t.Model(&ledger.Grant{}).Where("operator_id=?", child.id).Count(&count).Error; err != nil {
					return err
				}
				if count != 0 {
					return errors.New("initial pricing fixture already has sold grants")
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
		}
		if _, err = f.coop.DeliverPurchase(f.ctx, nil, cooperation.PurchaseInput{BuyerID: child.id, SupplierID: parent.id,
			OrderID: uuid.NewString(), ReceiptReference: "verified:" + child.id, ActorID: child.owner, PaidMoney: d("100"), PaidATG: d("200"),
			ExitTerms: json.RawMessage(`{"voluntary_refund":false}`), At: time.Now()}); err != nil {
			t.Fatal(err)
		}
		return child
	}
	oem := createOperator(operator{id: f.operator, owner: f.owner}, organization.OEM, "0.001")
	channel := createOperator(oem, organization.Channel, "0.002")
	intent, err := f.org.BeginRegistration(f.ctx, nil, organization.RegistrationIntentRequest{BrandID: channel.brand, InviteCode: channel.invite,
		Email: "pool-customer@example.test", Password: "integration-strong-password"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.org.VerifyEmail(f.ctx, nil, intent.Intent.ID, intent.VerificationToken); err != nil {
		t.Fatal(err)
	}
	reg, err := f.org.CompleteRegistration(f.ctx, nil, organization.CompleteRegistrationInput{IntentID: intent.Intent.ID, VerificationToken: intent.VerificationToken})
	if err != nil || reg.Ownership.OperatorID != channel.id {
		t.Fatal("customer registration for the channel", err)
	}
	grant, err := f.l.IssueGrant(f.ctx, nil, ledger.GrantInput{OperatorID: channel.id, CustomerID: reg.Customer.ID, OrderID: "funded-grant-order",
		BusinessKey: "funded-grant", ActorID: channel.owner, Reason: "test funded credits", EffectiveAt: time.Now().Add(-time.Minute),
		Sources: []ledger.GrantSource{{Kind: "paid", Amount: d("500")}}, PaidMoney: d("500")})
	if err != nil {
		t.Fatal(err)
	}
	key, err := f.s.CreateKey(f.ctx, reg.Customer.ID, "pool-key", KeyInput{Name: "pool", Period: ledger.Month, Timezone: "UTC"})
	if err != nil {
		t.Fatal(err)
	}
	addTestCodexRoute(t, f)
	input, err := modelruntime.DecodeProtocol("incoming", modelruntime.Responses, []byte(`{"model":"public-model","input":"hello","max_output_tokens":8000}`))
	if err != nil {
		t.Fatal(err)
	}
	checkHeld := func(accountID, want string) {
		t.Helper()
		account, err := f.l.AccountInTransaction(f.ctx, nil, accountID)
		if err != nil || account.Held.String() != want {
			t.Fatalf("held %s, want %s: %v", account.Held, want, err)
		}
	}
	channelPosition, err := f.coop.Position(f.ctx, nil, channel.id)
	if err != nil {
		t.Fatal(err)
	}
	oemPosition, err := f.coop.Position(f.ctx, nil, oem.id)
	if err != nil {
		t.Fatal(err)
	}
	var codexCalls int
	f.s.Runtime = subscriptionTestRuntime(func(_ context.Context, a modelruntime.Attempt, _ modelruntime.Request, _ modelruntime.Emit) (modelruntime.Result, error) {
		if a.UpstreamID == "upstream" {
			return modelruntime.Result{State: "failed", ProviderStatus: 503}, &modelruntime.UpstreamFailure{Status: 503}
		}
		codexCalls++
		checkHeld(grant.AccountID, "200.136")
		checkHeld(channelPosition.AccountID, "100.068")
		checkHeld(oemPosition.AccountID, "50.034")
		return modelruntime.Result{State: "completed", Usage: measured("12", "5")}, nil
	})
	result, err := f.s.Execute(f.ctx, key.Key, "whole-chain", input, nil)
	if err != nil || result.Request.Status != "completed" || codexCalls != 1 {
		t.Fatalf("successful growth did not dispatch once and settle: %s, %d, %v", result.Request.Status, codexCalls, err)
	}
	checkHeld(grant.AccountID, "0")
	checkHeld(channelPosition.AccountID, "0")
	checkHeld(oemPosition.AccountID, "0")

	if _, _, err = f.s.Admit(f.ctx, key.Key, "blocked-oem", input); err != nil {
		t.Fatal(err)
	}
	// The ordinary route still has enough ATG to run. The last procurement
	// edge cannot cover the additional reservation needed for the pool.
	if _, err = f.coop.ReserveProcurement(f.ctx, nil, oem.id, "in-flight-order", d("175"), time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	var upstreamErr *modelruntime.UpstreamFailure
	if _, err = f.s.Execute(f.ctx, key.Key, "blocked-oem", input, nil); !errors.As(err, &upstreamErr) || codexCalls != 1 {
		t.Fatalf("underfunded fallback dispatched or lost first error: %d, %v", codexCalls, err)
	}
	checkHeld(grant.AccountID, "32.136")
	checkHeld(channelPosition.AccountID, "16.068")
	checkHeld(oemPosition.AccountID, "183.034")
}
