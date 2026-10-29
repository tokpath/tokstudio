package incentives

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"gorm.io/gorm"
)

func (f fixture) channel(t *testing.T) (fixture, string) {
	t.Helper()
	c, e := f.org.CreateOperator(f.ctx, nil, organization.CreateOperatorInput{ActorStaffID: f.owner, ParentOperatorID: f.operator, Kind: organization.Channel, Name: "Channel"})
	if e != nil {
		t.Fatal(e)
	}
	owner := uuid.NewString()
	invite, e := f.org.InviteStaff(f.ctx, nil, f.owner, c.Operator.ID, "channel-owner@example.test", "owner")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.org.AcceptStaffInvitation(f.ctx, nil, invite.Token, organization.VerifiedStaff{ID: owner, Email: invite.Email, EmailVerified: true}); e != nil {
		t.Fatal(e)
	}
	for _, feature := range []organization.BusinessFeature{organization.Registration, organization.NewPurchase, organization.Renewal, organization.Activity} {
		st, e := f.org.GetBusinessStatus(f.ctx, nil, c.Operator.ID)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = f.org.SetBusinessStatus(f.ctx, nil, organization.SetBusinessStatusInput{ActorStaffID: owner, OperatorID: c.Operator.ID, Feature: feature, Enabled: true, ExpectedVersion: st.Version, EvidenceReference: "test readiness"}, ready{}); e != nil {
			t.Fatal(e)
		}
	}
	q, e := f.commerce.PublishPackage(f.ctx, nil, commerce.PublishPackageInput{ActorStaffID: owner, OperatorID: c.Operator.ID, Name: "Channel monthly", TermsReference: "channel-terms-v1", Timezone: "UTC", PriceMoney: d("100"), PaidATG: d("80"), GiftATG: d("20"), Period: ledger.Month, NewPurchaseEnabled: true, RenewalEnabled: true})
	if e != nil {
		t.Fatal(e)
	}
	p, e := f.s.SetPolicy(f.ctx, nil, owner, c.Operator.ID, PolicyInput{ThresholdMoney: d("60"), DirectRate: d("0.15"), IndirectRate: d("0.05"), RewardValiditySeconds: 3600})
	if e != nil {
		t.Fatal(e)
	}
	f.operator = c.Operator.ID
	f.owner = owner
	f.quote = q
	f.policy = p
	return f, c.InviteCode
}

func TestPostgresTakeoverTransfersWalletQualificationsCommissionsAndPendingWithdrawalAtomically(t *testing.T) {
	brand := setup(t)
	channel, code := brand.channel(t)
	a := channel.register(t, "kol-channel@example.test", code)
	b := channel.register(t, "customer-channel@example.test", a.InviteCode)
	channel.qualify(t, a.Customer.ID, "qualification")
	order := channel.buy(t, b.Customer.ID, "customer-package", "100", ActivitySelection{})
	first := channel.consume(t, b.Customer.ID, "20")
	if _, e := channel.s.SetPolicy(channel.ctx, nil, channel.owner, channel.operator, PolicyInput{ExpectedVersion: 1, ThresholdMoney: d("60"), DirectRate: d("0.15"), IndirectRate: d("0.05"), ConfirmationSeconds: 3600, RewardValiditySeconds: 3600}); e != nil {
		t.Fatal(e)
	}
	channel.consume(t, b.Customer.ID, "20")
	channel.seed(t, b.Customer.ID, "customer:money", "25")
	sub, e := channel.commerce.EnableAutoRenewal(channel.ctx, nil, b.Customer.ID, order.ID, "customer renewal consent", false)
	if e != nil {
		t.Fatal(e)
	}
	payout, e := channel.s.SetWithdrawalPolicy(channel.ctx, nil, channel.owner, channel.operator, WithdrawalPolicy{MinimumMoney: d("1"), MethodReference: "manual-bank-transfer"}, 0)
	if e != nil {
		t.Fatal(e)
	}
	w, e := channel.s.RequestWithdrawal(channel.ctx, nil, WithdrawalInput{OperatorID: channel.operator, CustomerID: a.Customer.ID, BusinessKey: "pending-withdrawal", PolicyID: payout.ID, AmountMoney: d("1"), RecipientReference: "verified customer bank"})
	if e != nil {
		t.Fatal(e)
	}
	if e = channel.s.ReverseEvent(channel.ctx, nil, CorrectionInput{ActorStaffID: channel.owner, OperatorID: channel.operator, OriginalEventID: first[0].ID, BusinessKey: "wrong-first-base", Evidence: "verified original paid base correction", AmountMoney: d("20")}); e != nil {
		t.Fatal(e)
	}
	if _, e = channel.s.SetFunctionState(channel.ctx, nil, channel.owner, channel.operator, a.Customer.ID, "suspended", "confirmed fraud review", true); e != nil {
		t.Fatal(e)
	}
	qBefore, e := channel.s.GetQualification(channel.ctx, nil, channel.operator, a.Customer.ID)
	if e != nil || qBefore.PendingOffsetMoney.Cmp(d("1")) != 0 {
		t.Fatalf("qualification before=%+v %v", qBefore, e)
	}
	obligations, e := channel.s.ClosureRequirements(channel.ctx, nil, channel.operator, a.Customer.ID)
	if e != nil || len(obligations) != 4 {
		t.Fatalf("closure obligations=%+v %v", obligations, e)
	}
	for _, obligation := range obligations {
		if obligation.Ready {
			t.Fatalf("real unfinished obligation hidden: %+v", obligation)
		}
	}
	unrelated := brand.register(t, "unrelated-brand@example.test", "")
	if _, e = brand.commerce.CreateTopUpOrder(brand.ctx, nil, commerce.CreateTopUpInput{OperatorID: brand.operator, CustomerID: unrelated.Customer.ID, BusinessKey: "customer-package", ExpectedOwnershipVersion: 1, AmountMoney: d("1")}); e != nil {
		t.Fatal(e)
	}
	blocked, e := channel.commerce.CreateOrder(channel.ctx, nil, commerce.CreateOrderInput{OperatorID: channel.operator, CustomerID: b.Customer.ID, QuoteID: channel.quote.ID, BusinessKey: "unfinished-external", ExpectedOwnershipVersion: 1})
	if e != nil {
		t.Fatal(e)
	}
	in := TransferInput{FromOperatorID: channel.operator, ToOperatorID: brand.operator, TakeoverID: uuid.NewString(), ActorID: brand.owner, Evidence: "approved full channel takeover", Customers: map[string]int64{a.Customer.ID: 1, b.Customer.ID: 1}, AccountIDs: map[string]string{}, HoldIDs: map[string]string{}}
	e = channel.db.Transaction(func(tx *gorm.DB) error { return channel.commerce.PreflightTransferResponsibility(channel.ctx, tx, in) })
	if !errors.Is(e, commerce.ErrTransferBlocked) {
		t.Fatalf("unfinished payment silently skipped: %v", e)
	}
	if _, e = channel.commerce.CloseOrder(channel.ctx, nil, b.Customer.ID, blocked.ID, commerce.PaymentClosureProof{VerifiedUnpaid: true, EvidenceReference: "provider closed entry and verified unpaid"}); e != nil {
		t.Fatal(e)
	}
	move := func(tx *gorm.DB) error {
		in.AccountIDs = map[string]string{}
		in.HoldIDs = map[string]string{}
		if e := channel.org.LockResponsibilityTransfer(channel.ctx, tx, in); e != nil {
			return e
		}
		if e := channel.commerce.PreflightTransferResponsibility(channel.ctx, tx, in); e != nil {
			return e
		}
		if e := channel.s.PreflightTransferResponsibility(channel.ctx, tx, in); e != nil {
			return e
		}
		instant, e := channel.s.BusinessInstant(channel.ctx, tx)
		if e != nil {
			return e
		}
		for customer := range in.Customers {
			m, e := channel.ledger.TransferCustomerAssets(channel.ctx, tx, ledger.CustomerTransfer{FromOperatorID: in.FromOperatorID, ToOperatorID: in.ToOperatorID, TakeoverID: in.TakeoverID, CustomerID: customer, ActorID: in.ActorID, Evidence: in.Evidence, At: instant.At})
			if e != nil {
				return e
			}
			for old, id := range m.AccountIDs {
				in.AccountIDs[old] = id
			}
			for old, id := range m.HoldIDs {
				in.HoldIDs[old] = id
			}
		}
		if e = channel.commerce.TransferResponsibility(channel.ctx, tx, in); e != nil {
			return e
		}
		if e = channel.s.TransferResponsibility(channel.ctx, tx, in); e != nil {
			return e
		}
		_, e = channel.org.ApplyChannelTakeoverOwnership(channel.ctx, tx, organization.TakeoverOwnershipInput{ChannelID: in.FromOperatorID, BrandOwnerOperatorID: in.ToOperatorID, EvidenceReference: in.TakeoverID, ExpectedVersions: in.Customers})
		return e
	}
	injected := errors.New("coordinator failed after all domain transfers")
	e = channel.db.Transaction(func(tx *gorm.DB) error {
		if e := move(tx); e != nil {
			return e
		}
		return injected
	})
	if !errors.Is(e, injected) {
		t.Fatal(e)
	}
	old, e := channel.org.CurrentOwnership(channel.ctx, nil, b.Customer.ID)
	if e != nil || old.OperatorID != channel.operator {
		t.Fatalf("partial takeover ownership=%+v %v", old, e)
	}
	if account := channel.balance(t, a.Customer.ID, "commission:available"); account.Held.Cmp(d("1")) != 0 || account.Balance.Cmp(d("1")) != 0 {
		t.Fatalf("rollback changed cash=%+v", account)
	}
	if e = channel.db.Transaction(move); e != nil {
		t.Fatal(e)
	}
	if e = channel.db.Transaction(func(tx *gorm.DB) error {
		if e := channel.commerce.TransferResponsibility(channel.ctx, tx, in); e != nil {
			return e
		}
		return channel.s.TransferResponsibility(channel.ctx, tx, in)
	}); e != nil {
		t.Fatalf("domain replay=%v", e)
	}
	moved, e := brand.commerce.GetOrder(brand.ctx, nil, b.Customer.ID, order.ID)
	if e != nil || moved.OperatorID != brand.operator || moved.OriginalOperatorID != channel.operator || moved.OriginalOwnershipVersion != 1 || moved.OwnershipVersion != 2 {
		t.Fatalf("order origin=%+v %v", moved, e)
	}
	sources, e := brand.commerce.OrderWalletFunding(brand.ctx, nil, brand.operator, b.Customer.ID, order.ID)
	if e != nil || len(sources) != 1 || sources[0].SourceOperatorID != channel.operator {
		t.Fatalf("wallet original receipt=%+v %v", sources, e)
	}
	e = brand.db.Transaction(func(tx *gorm.DB) error {
		allocs, e := brand.commerce.ReserveWalletRefund(brand.ctx, tx, brand.operator, b.Customer.ID, uuid.NewString(), d("25"))
		if e != nil {
			return e
		}
		if len(allocs) != 1 || allocs[0].SourceOperatorID != channel.operator {
			return errors.New("unspent original receipt lost")
		}
		return injected
	})
	if !errors.Is(e, injected) {
		t.Fatal(e)
	}
	wMoved, e := brand.s.GetWithdrawal(brand.ctx, nil, a.Customer.ID, w.ID)
	if e != nil || wMoved.OperatorID != brand.operator || wMoved.OriginalOperatorID != channel.operator || wMoved.HoldID == w.HoldID || wMoved.Status != "requested" {
		t.Fatalf("withdrawal lost hold/origin=%+v %v", wMoved, e)
	}
	if account := brand.balance(t, a.Customer.ID, "commission:available"); account.Held.Cmp(d("1")) != 0 || account.Balance.Cmp(d("1")) != 0 {
		t.Fatalf("mapped withdrawal funds=%+v", account)
	}
	var lots []commissionLot
	if e = brand.db.Where("customer_id=?", a.Customer.ID).Find(&lots).Error; e != nil || len(lots) == 0 {
		t.Fatal("commission provenance missing after takeover", lots, e)
	}
	for _, lot := range lots {
		if lot.OperatorID != brand.operator || lot.OriginalOperatorID != channel.operator {
			t.Fatal("commission source/current responsibility rewritten", lot)
		}
	}
	var command commissionFundingCommand
	if e = brand.db.First(&command, "kind='withdrawal' AND reference=?", w.ID).Error; e != nil || command.OperatorID != brand.operator || command.State != "reserved" {
		t.Fatal("withdrawal provenance reservation not transferred", command, e)
	}
	qAfter, e := brand.s.GetQualification(brand.ctx, nil, brand.operator, a.Customer.ID)
	if e != nil || qAfter.AcquiredAt == nil || !qAfter.AcquiredAt.Equal(*qBefore.AcquiredAt) || qAfter.PendingOffsetMoney.Cmp(d("1")) != 0 {
		t.Fatalf("qualification history=%+v %v", qAfter, e)
	}
	instant, e := brand.s.BusinessInstant(brand.ctx, nil)
	if e != nil {
		t.Fatal(e)
	}
	state, e := brand.s.stateAt(brand.db, brand.operator, a.Customer.ID, instant.At)
	if e != nil || state.Status != "suspended" || !state.ReleaseBlocked {
		t.Fatalf("function scope lost=%+v %v", state, e)
	}
	if _, e = brand.s.SetFunctionState(brand.ctx, nil, brand.owner, brand.operator, a.Customer.ID, "active", "review completed after takeover", false); e != nil {
		t.Fatal(e)
	}
	if _, e = brand.s.Advance(brand.ctx, nil, brand.operator, a.Customer.ID, instant.At.Add(2*time.Hour)); e != nil {
		t.Fatal(e)
	}
	qAfter, e = brand.s.GetQualification(brand.ctx, nil, brand.operator, a.Customer.ID)
	if e != nil || !qAfter.PendingOffsetMoney.IsZero() {
		t.Fatalf("pending recovery not continued=%+v %v", qAfter, e)
	}
	before := brand.balance(t, a.Customer.ID, "commission:available").Balance
	if _, e = brand.s.SetPolicy(brand.ctx, nil, brand.owner, brand.operator, PolicyInput{ExpectedVersion: 1, ThresholdMoney: d("1000"), DirectRate: d("0.01"), IndirectRate: d("0.01"), RewardValiditySeconds: 3600}); e != nil {
		t.Fatal(e)
	}
	brand.consume(t, b.Customer.ID, "10")
	if after := brand.balance(t, a.Customer.ID, "commission:available").Balance; after.Sub(before).Cmp(d("1.5")) != 0 {
		t.Fatalf("original fixed rate lost after takeover: %s", after.Sub(before))
	}
	if e = brand.s.AccrueGrantEvent(brand.ctx, nil, GrantEventInput{EventID: first[0].ID, OperatorID: brand.operator, CustomerID: b.Customer.ID, OrderID: order.ID, Kind: "usage", PaidMoney: first[0].PaidMoney, OccurredAt: first[0].CreatedAt}); e != nil {
		t.Fatalf("historical event replay after takeover=%v", e)
	}
	subMoved, e := brand.commerce.GetSubscription(brand.ctx, nil, b.Customer.ID, sub.ID)
	if e != nil || subMoved.OperatorID != brand.operator || subMoved.Status != "needs_confirmation" {
		t.Fatalf("subscription promise=%+v %v", subMoved, e)
	}
	if result, e := brand.commerce.RenewSubscription(brand.ctx, nil, sub.ID); e != nil || result.Status != "needs_confirmation" {
		t.Fatalf("renewal auto-accepted new terms=%+v %v", result, e)
	}
	replacement, e := brand.commerce.PublishPackage(brand.ctx, nil, commerce.PublishPackageInput{ActorStaffID: brand.owner, OperatorID: brand.operator, PackageID: brand.quote.PackageID, ExpectedVersion: brand.quote.Version, Name: "Brand monthly renewal", PriceMoney: brand.quote.PriceMoney, PaidATG: brand.quote.PaidATG, GiftATG: brand.quote.GiftATG, Period: brand.quote.Period, Timezone: brand.quote.Timezone, TermsSummary: brand.quote.TermsSummary, NewPurchaseEnabled: true, RenewalEnabled: true})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = brand.commerce.ConfirmRenewal(brand.ctx, nil, b.Customer.ID, sub.ID, replacement.ID, "explicit customer consent to brand replacement package", subMoved.Version, false); e != nil {
		t.Fatalf("cannot choose new brand package=%v", e)
	}
}
