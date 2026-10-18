package remediation

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"math/big"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/cooperation"
	"github.com/tokpath/tokstudio/backend/internal/incentives"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"github.com/tokpath/tokstudio/backend/internal/usage"
	"gorm.io/gorm"
)

// Only the price ratio is fixed in this accounting integration fixture. Every
// account, receipt, order, grant and responsibility transfer uses real Postgres.
type takeoverRates struct{}

func (takeoverRates) CoverageRates(context.Context, *gorm.DB, string, time.Time) (cooperation.CoverageRates, error) {
	return cooperation.CoverageRates{Version: 1, Direct: big.NewRat(1, 1), Children: map[string]*big.Rat{}}, nil
}
func takeoverFixture(t *testing.T) (fixture, fixture, commerce.Order, usage.KeySecret) {
	t.Helper()
	brand := setup(t)
	_, migrations := usage.Migrations()
	files, e := fs.ReadDir(migrations, ".")
	if e != nil {
		t.Fatal(e)
	}
	for _, file := range files {
		b, e := fs.ReadFile(migrations, file.Name())
		if e != nil {
			t.Fatal(e)
		}
		if e = brand.db.Exec(string(b)).Error; e != nil {
			t.Fatal(e)
		}
	}
	brand.s.Cooperation.Rates = takeoverRates{}
	org := brand.s.Organization
	created, e := org.CreateOperator(brand.ctx, nil, organization.CreateOperatorInput{ActorStaffID: brand.owner, ParentOperatorID: brand.operator, Kind: organization.Channel, Name: "Takeover Channel"})
	if e != nil {
		t.Fatal(e)
	}
	owner := uuid.NewString()
	invite, e := org.InviteStaff(brand.ctx, nil, brand.owner, created.Operator.ID, "takeover-owner@example.test", "owner")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = org.AcceptStaffInvitation(brand.ctx, nil, invite.Token, organization.VerifiedStaff{ID: owner, Email: invite.Email, EmailVerified: true}); e != nil {
		t.Fatal(e)
	}
	for _, feature := range []organization.BusinessFeature{organization.Registration, organization.NewPurchase, organization.Renewal, organization.Activity} {
		status, e := org.GetBusinessStatus(brand.ctx, nil, created.Operator.ID)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = org.SetBusinessStatus(brand.ctx, nil, organization.SetBusinessStatusInput{ActorStaffID: owner, OperatorID: created.Operator.ID, Feature: feature, Enabled: true, ExpectedVersion: status.Version, EvidenceReference: "test dependencies"}, testReadiness{}); e != nil {
			t.Fatal(e)
		}
	}
	intent, e := org.BeginRegistration(brand.ctx, nil, organization.RegistrationIntentRequest{BrandID: created.Brand.ID, InviteCode: created.InviteCode, Email: "takeover-customer@example.test", Password: "test-takeover-password"})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = org.VerifyEmail(brand.ctx, nil, intent.Intent.ID, intent.VerificationToken); e != nil {
		t.Fatal(e)
	}
	reg, e := org.CompleteRegistration(brand.ctx, nil, organization.CompleteRegistrationInput{IntentID: intent.Intent.ID, VerificationToken: intent.VerificationToken})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = brand.s.Incentives.SetPolicy(brand.ctx, nil, owner, created.Operator.ID, incentives.PolicyInput{ThresholdUSD: d("100"), DirectRate: d("0.15"), IndirectRate: d("0.05"), ConfirmationSeconds: 3600, RewardValiditySeconds: 86400, KOLCrossActivityLimit: 10}); e != nil {
		t.Fatal(e)
	}
	// Verified wholesale fact: paid 60 USD for 100 purchased + 100 gift ATG.
	if _, e = brand.s.Cooperation.DeliverPurchase(brand.ctx, nil, cooperation.PurchaseInput{BuyerID: created.Operator.ID, SupplierID: brand.operator, OrderID: uuid.NewString(), ReceiptReference: "verified-wholesale-bank-fact", ActorID: brand.owner, PaidUSD: d("60"), PaidATG: d("100"), GiftATG: d("100"), ExitTerms: json.RawMessage(`{"version":1,"voluntary_refund":true,"method":"remaining_paid_proportional","terms_reference":"signed-wholesale-v1"}`), At: time.Now().UTC()}); e != nil {
		t.Fatal(e)
	}
	quote, e := brand.s.Commerce.PublishPackage(brand.ctx, nil, commerce.PublishPackageInput{ActorStaffID: owner, OperatorID: created.Operator.ID, Name: "Channel monthly", TermsReference: "channel-terms", Timezone: "UTC", PriceUSD: d("80"), PaidATG: d("80"), GiftATG: d("20"), Period: ledger.Month, NewPurchaseEnabled: true, RenewalEnabled: true})
	if e != nil {
		t.Fatal(e)
	}
	channel := fixture{brand.ctx, brand.db, brand.s, owner, created.Operator.ID, reg.Customer.ID, quote}
	channel.topup(t, "channel-deposit", "105")
	order, e := channel.s.Commerce.CreateOrder(channel.ctx, nil, commerce.CreateOrderInput{OperatorID: channel.operator, CustomerID: channel.customer, QuoteID: quote.ID, BusinessKey: "channel-package", ExpectedOwnershipVersion: 1, InternalUSD: d("80")})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = channel.s.Commerce.Fulfill(channel.ctx, nil, order.ID, owner); e != nil {
		t.Fatal(e)
	}
	u := usage.New(brand.db, usage.Dependencies{Organization: org, EncryptionKey: "takeover-test-only-encryption-secret"})
	key, e := u.CreateKey(brand.ctx, channel.customer, "key-for-channel", usage.KeyInput{Name: "existing-agent", Period: ledger.Month, Timezone: "UTC"})
	if e != nil {
		t.Fatal(e)
	}
	return brand, channel, order, key
}
func beginTakeover(t *testing.T, brand, channel fixture) Takeover {
	t.Helper()
	p, e := brand.s.BeginTakeover(brand.ctx, TakeoverInput{ActorID: brand.owner, ChannelID: channel.operator, BrandOperatorID: brand.operator, BusinessKey: "agreed-channel-exit", Method: "agreed_atg_value", ServiceUSDPerATG: d("0.8"), ProcurementUSDPerATG: d("0.8"), ContractReference: "signed-takeover-contract", Evidence: "brand approved complete obligations transfer"})
	if e != nil {
		t.Fatal(e)
	}
	return p
}
func TestPostgresTakeoverKeepsRightsAndSettlesContractValueOnce(t *testing.T) {
	brand, channel, order, key := takeoverFixture(t)
	org := brand.s.Organization
	customerBefore, e := org.GetCustomer(brand.ctx, nil, channel.customer)
	if e != nil {
		t.Fatal(e)
	}
	// A stopped model supply must not prevent the contract's fixed valuation.
	brand.s.Cooperation.Rates = nil
	p := beginTakeover(t, brand, channel)
	status, e := org.GetBusinessStatus(brand.ctx, nil, channel.operator)
	if e != nil || status.NewPurchase || status.Registration || status.ModelUsage {
		t.Fatal(status, e)
	}
	p, e = brand.s.RefreshTakeover(brand.ctx, brand.owner, brand.operator, p.ID)
	if e != nil {
		t.Fatal(e)
	}
	if p.ServiceUSD.Cmp(d("80")) != 0 || p.CashUSD.Cmp(d("25")) != 0 || p.ProcurementOffsetATG.Cmp(d("100")) != 0 || p.RemainingProcurementRefundUSD.Cmp(d("30")) != 0 || p.SettlementDueUSD.Cmp(d("-5")) != 0 {
		t.Fatalf("closing valuation %+v", p)
	}
	done, e := brand.s.ExecuteTakeover(brand.ctx, brand.owner, brand.operator, p.ID, p.SnapshotFingerprint)
	if e != nil {
		t.Fatal(e)
	}
	replay, e := brand.s.ExecuteTakeover(brand.ctx, brand.owner, brand.operator, p.ID, p.SnapshotFingerprint)
	if e != nil || replay.ID != done.ID || done.State != "completed" {
		t.Fatal(done, replay, e)
	}
	own, e := org.CurrentOwnership(brand.ctx, nil, channel.customer)
	if e != nil || own.OperatorID != brand.operator || own.Version != 2 {
		t.Fatal(own, e)
	}
	customerAfter, e := org.GetCustomer(brand.ctx, nil, channel.customer)
	if e != nil || customerAfter != customerBefore {
		t.Fatal("identity changed", customerBefore, customerAfter, e)
	}
	var currentKey usage.Key
	if e = brand.db.First(&currentKey, "id=?", key.Key.ID).Error; e != nil || currentKey.TokenHash != key.Key.TokenHash || currentKey.BrandID != key.Key.BrandID || currentKey.Status != "active" {
		t.Fatal(currentKey, e)
	}
	moved, e := brand.s.Commerce.GetOrder(brand.ctx, nil, channel.customer, order.ID)
	if e != nil || moved.OperatorID != brand.operator || moved.OriginalOperatorID != channel.operator || moved.OwnershipVersion != 2 {
		t.Fatal(moved, e)
	}
	var grants []ledger.Grant
	if e = brand.db.Where("customer_id=?", channel.customer).Find(&grants).Error; e != nil || len(grants) != 1 {
		t.Fatal(grants, e)
	}
	if grants[0].OperatorID != brand.operator || grants[0].OriginalOperatorID != channel.operator || grants[0].Total.Cmp(d("100")) != 0 || !grants[0].Consumed.IsZero() {
		t.Fatal(grants[0])
	}
	wallet, e := brand.s.account(brand.ctx, brand.db, brand.operator, channel.customer, "customer:usd")
	if e != nil || wallet.Balance.Cmp(d("25")) != 0 {
		t.Fatal(wallet, e)
	}
	if old := channel.balance(t); !old.Balance.IsZero() || !old.Held.IsZero() {
		t.Fatal(old)
	}
	pos, e := brand.s.Cooperation.Position(brand.ctx, nil, channel.operator)
	if e != nil || !pos.Quantity.IsZero() || !pos.Cost.IsZero() {
		t.Fatal(pos, e)
	}
	if _, e = brand.s.Cooperation.Position(brand.ctx, nil, brand.operator); !errors.Is(e, gorm.ErrRecordNotFound) {
		t.Fatal("takeover fabricated upstream inventory", e)
	}
	// An outstanding brand/channel debt never blocks customer wallet refund.
	refund, e := brand.s.RequestWalletRefund(brand.ctx, WalletRefundInput{OperatorID: brand.operator, CustomerID: channel.customer, BusinessKey: "refund-after-takeover", AmountUSD: d("5"), Reason: "unused deposit"})
	if e != nil {
		t.Fatal(e)
	}
	legs, e := brand.s.Legs(brand.ctx, nil, brand.operator, refund.ID)
	if e != nil || len(legs) != 1 || legs[0].SourceOperatorID != channel.operator {
		t.Fatal(legs, e)
	}
	proof := ExecutionProof{BusinessKey: "settle-part-1", Reference: "bank-transfer-1", Evidence: "verified actual USD received", ActualUSD: d("2"), FeeUSD: d("0.1"), Currency: "USD", Amount: "2", OccurredAt: time.Now().UTC()}
	paid, e := brand.s.RecordTakeoverSettlement(brand.ctx, brand.owner, brand.operator, p.ID, proof)
	if e != nil || paid.SettledUSD.Cmp(d("2")) != 0 {
		t.Fatal(paid, e)
	}
	if _, e = brand.s.RecordTakeoverSettlement(brand.ctx, brand.owner, brand.operator, p.ID, proof); e != nil {
		t.Fatal(e)
	}
	proof.BusinessKey = "same-bank-fact-new-command"
	if _, e = brand.s.RecordTakeoverSettlement(brand.ctx, brand.owner, brand.operator, p.ID, proof); e == nil {
		t.Fatal("same bank fact settled twice")
	}
	proof.BusinessKey = "settle-part-2"
	proof.Reference = "bank-transfer-2"
	proof.ActualUSD = d("3")
	proof.Amount = "3"
	proof.FeeUSD = d("0")
	paid, e = brand.s.RecordTakeoverSettlement(brand.ctx, brand.owner, brand.operator, p.ID, proof)
	if e != nil || paid.SettledUSD.Cmp(d("5")) != 0 {
		t.Fatal(paid, e)
	}
	payable, e := brand.s.account(brand.ctx, brand.db, brand.operator, p.ID, "counterparty:takeover_payable")
	if e != nil || !payable.Balance.IsZero() {
		t.Fatal(payable, e)
	}
}

func TestPostgresTakeoverRefreshRequiredAndFailureRollsBackEveryAsset(t *testing.T) {
	brand, channel, order, _ := takeoverFixture(t)
	p := beginTakeover(t, brand, channel)
	p, e := brand.s.RefreshTakeover(brand.ctx, brand.owner, brand.operator, p.ID)
	if e != nil {
		t.Fatal(e)
	}
	// A newly verified expiry/reversal/freeze changes the liability snapshot and
	// requires a fresh visible closing statement; an old approval cannot be used.
	a := channel.balance(t)
	held, e := brand.s.Ledger.Reserve(brand.ctx, nil, ledger.Reservation{OperatorID: channel.operator, BusinessKey: "pending-financial-review", Purpose: "review", ExpiresAt: time.Now().Add(time.Hour), Amounts: []ledger.Line{{AccountID: a.ID, Amount: d("1")}}})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = brand.s.ExecuteTakeover(brand.ctx, brand.owner, brand.operator, p.ID, p.SnapshotFingerprint); !errors.Is(e, ErrTakeoverBlocked) {
		t.Fatal(e)
	}
	if _, e = brand.s.Ledger.Release(brand.ctx, nil, channel.operator, held.ID, "review finished"); e != nil {
		t.Fatal(e)
	}
	p, e = brand.s.RefreshTakeover(brand.ctx, brand.owner, brand.operator, p.ID)
	if e != nil {
		t.Fatal(e)
	}
	// Inject a storage failure at the last stage, after all modules transferred.
	if e = brand.db.Exec(`CREATE FUNCTION test_abort_adoption() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.state='completed' THEN RAISE EXCEPTION 'injected closing failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER test_abort_adoption BEFORE UPDATE ON remediation_takeovers FOR EACH ROW EXECUTE FUNCTION test_abort_adoption();`).Error; e != nil {
		t.Fatal(e)
	}
	if _, e = brand.s.ExecuteTakeover(brand.ctx, brand.owner, brand.operator, p.ID, p.SnapshotFingerprint); e == nil {
		t.Fatal("injected failure did not abort")
	}
	own, e := brand.s.Organization.CurrentOwnership(brand.ctx, nil, channel.customer)
	if e != nil || own.OperatorID != channel.operator || own.Version != 1 {
		t.Fatal(own, e)
	}
	old, e := brand.s.Commerce.GetOrder(brand.ctx, nil, channel.customer, order.ID)
	if e != nil || old.OperatorID != channel.operator {
		t.Fatal(old, e)
	}
	if a := channel.balance(t); a.Balance.Cmp(d("25")) != 0 {
		t.Fatal(a)
	}
	pos, e := brand.s.Cooperation.Position(brand.ctx, nil, channel.operator)
	if e != nil || pos.Quantity.Cmp(d("200")) != 0 || pos.Cost.Cmp(d("60")) != 0 {
		t.Fatal(pos, e)
	}
	var n int64
	if e = brand.db.Table("ledger_customer_transfers").Where("takeover_id=?", p.ID).Count(&n).Error; e != nil || n != 0 {
		t.Fatal("partial transfer journal", n, e)
	}
	if e = brand.db.Exec(`DROP TRIGGER test_abort_adoption ON remediation_takeovers; DROP FUNCTION test_abort_adoption();`).Error; e != nil {
		t.Fatal(e)
	}
	if _, e = brand.s.ExecuteTakeover(brand.ctx, brand.owner, brand.operator, p.ID, p.SnapshotFingerprint); e != nil {
		t.Fatal("retry after rollback", e)
	}
}
