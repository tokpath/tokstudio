package remediation

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/payments"
)

func TestPostgresRefundRerouteRequiresTerminalProofAndOnlySettlesRemainingPrincipal(t *testing.T) {
	f := setup(t)
	f.topup(t, "source-for-versioned-refund", "100")
	f.s.Payments = payments.NewService(f.db, "isolated-versioned-refund-key", f.s.Organization, payments.NewTransport(true), payments.CommerceSink{Commerce: f.s.Commerce})
	f.s.Payments.RefundApproval, f.s.Payments.RefundSink = f.s, f.s
	c, e := f.s.RequestWalletRefund(f.ctx, WalletRefundInput{OperatorID: f.operator, CustomerID: f.customer, BusinessKey: "versioned-return", AmountMoney: d("10"), Reason: "original receiving bank unavailable"})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.ReviewRefund(f.ctx, f.owner, f.operator, c.ID, "approve", "actual original source and remaining principal verified"); e != nil {
		t.Fatal(e)
	}
	legs, e := f.s.Legs(f.ctx, nil, f.operator, c.ID)
	if e != nil || len(legs) != 1 {
		t.Fatal(legs, e)
	}
	l := legs[0]
	user, _ := f.s.Organization.GetCustomer(f.ctx, nil, f.customer)
	authorize := func(key, address string) RefundDestinationAuthorization {
		t.Helper()
		p, e := f.s.ProposeRefundDestination(f.ctx, f.customer, user.BrandID, l.ID, "customer-"+key, RefundDestinationProposalInput{Destination: RefundDestination{Network: "bank", Asset: "USD", Address: address}, ResponsibleExecution: true, AuthorizationReference: "customer explicitly consented " + key})
		if e != nil {
			t.Fatal(e)
		}
		a, e := f.s.AuthorizeRefundDestination(f.ctx, f.owner, f.operator, l.ID, "finance-"+key, RefundDestinationAuthorizationInput{ProposalID: p.ID, ResponsibleExecution: true, PayerAuthorizationReference: "independently verified original payer authorization " + key, VerificationEvidence: "independently verified receiving bank " + key, RouteExceptionReference: "previous bank route cannot complete original remaining liability"})
		if e != nil {
			t.Fatal(e)
		}
		return a
	}
	a1 := authorize("first", "bank:original-payer-1")
	if _, e = f.s.BeginManualRefund(f.ctx, f.owner, f.operator, l.ID, "first-instruction", "first approved outgoing instruction"); e != nil {
		t.Fatal(e)
	}
	closeInput := RefundExecutionClosureInput{BusinessKey: "close-first", ExpectedExecutionKey: "first-instruction", Reason: "failed", NoFurtherPaymentReference: "independent bank confirmation that no further transfer will occur", Evidence: "bank failure record fully checked"}
	if _, e = f.s.CloseRefundExecution(f.ctx, f.owner, f.operator, l.ID, closeInput); !errors.Is(e, ErrReview) {
		t.Fatal("mere manual instruction was treated as terminal", e)
	}
	if _, e = f.s.MarkRefundUnknown(f.ctx, f.owner, f.operator, l.ID, "bank response missing"); e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.CloseRefundExecution(f.ctx, f.owner, f.operator, l.ID, closeInput); !errors.Is(e, ErrReview) {
		t.Fatal("unknown payment was reopened", e)
	}
	at, _ := instant(f.db)
	failureInput := ManualRefundFailureInput{BusinessKey: "terminal-bank-result", ExpectedExecutionKey: "first-instruction", Result: "failed", ExternalReference: "bank-rejected-transfer-103", NoFurtherPaymentReference: "bank terminal rejection and no scheduled retry", Evidence: "finance independently verified signed terminal result", VerifiedAt: at}
	failure, e := f.s.RecordManualRefundFailure(f.ctx, f.owner, f.operator, l.ID, failureInput)
	if e != nil {
		t.Fatal(e)
	}
	if repeat, e := f.s.RecordManualRefundFailure(f.ctx, f.owner, f.operator, l.ID, failureInput); e != nil || repeat.ID != failure.ID {
		t.Fatal(repeat, e)
	}
	closed, e := f.s.CloseRefundExecution(f.ctx, f.owner, f.operator, l.ID, closeInput)
	if e != nil || closed.RemainingMoney.Cmp(d("10")) != 0 {
		t.Fatal(closed, e)
	}
	if repeat, e := f.s.CloseRefundExecution(f.ctx, f.owner, f.operator, l.ID, closeInput); e != nil || repeat.ID != closed.ID {
		t.Fatal(repeat, e)
	}
	status, e := f.s.RefundDestinationStatusForStaff(f.ctx, f.owner, f.operator, l.ID)
	if e != nil || status.RouteVersion != 2 || !status.DestinationMutable || status.AuthorizedDestination != nil || len(status.History) != 1 || status.History[0].AuthorizationID != a1.ID {
		t.Fatal(status, e)
	}
	if _, e = f.s.BeginManualRefund(f.ctx, f.owner, f.operator, l.ID, "missing-versioned-authorization", "try original already closed address"); !errors.Is(e, ErrReview) {
		t.Fatal("closed route reused without new customer consent", e)
	}
	a2 := authorize("second", "bank:original-payer-2")
	if a2.RouteVersion != 2 || a2.AmountMoney.Cmp(d("10")) != 0 || !a2.SettledBeforeMoney.IsZero() {
		t.Fatal(a2)
	}
	m := payments.Merchant{ID: uuid.NewString(), OperatorID: f.operator, Driver: "manual", Enabled: true, Credentials: map[string]string{"currency": "USD", "recipient_reference": "bank:current-responsible-operator", "instructions": "independently verify actual outgoing transfer"}}
	if _, e = f.s.Payments.SaveMerchant(f.ctx, f.owner, m, 0); e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.BeginManualRefund(f.ctx, f.owner, f.operator, l.ID, "second-instruction", "second independently authorized instruction"); e != nil {
		t.Fatal(e)
	}
	at, _ = instant(f.db)
	proof := payments.ExternalRefundVerification{ReceiptID: l.ReceiptID, ExternalReference: "actual-second-route-9", Money: payments.Money{Currency: "USD", Amount: d("9")}, ExecutedAt: at, EvidenceReference: "bank completed actual 9 USD", Destination: payments.ExternalRefundDestination{Network: a2.Network, Asset: a2.Asset, Address: a2.Address, AuthorizationReference: a2.ID, VerificationEvidence: "independently checked outgoing bank record"}}
	first, e := f.s.Payments.RecordResponsibleBankRefund(f.ctx, f.owner, f.operator, m.ID, payments.Refund{ID: l.ID}, proof)
	if !errors.Is(e, payments.ErrSettlementReview) || first.Fact == nil {
		t.Fatal(first, e)
	}
	closedPartial, e := f.s.CloseRefundExecution(f.ctx, f.owner, f.operator, l.ID, RefundExecutionClosureInput{BusinessKey: "close-second", ExpectedExecutionKey: "second-instruction", Reason: "settled_partial", NoFurtherPaymentReference: "bank confirmed transfer final, no automatic retry or second transfer", Evidence: "remaining 1 USD will use newly authorized receiving account"})
	if e != nil || closedPartial.SettledMoney.Cmp(d("9")) != 0 || closedPartial.RemainingMoney.Cmp(d("1")) != 0 {
		t.Fatal(closedPartial, e)
	}
	a3 := authorize("third", "bank:original-payer-3")
	if a3.RouteVersion != 3 || a3.AmountMoney.Cmp(d("1")) != 0 || a3.SettledBeforeMoney.Cmp(d("9")) != 0 {
		t.Fatal("new authorization exceeded remaining principal", a3)
	}
	// Replaying the already closed stored execution is read-only and does not
	// add cash, consume another customer's capacity or reissue a transfer.
	executions, e := f.s.Payments.RefundExecutions(f.ctx, f.operator, m.ID, l.ID)
	if e != nil || len(executions) != 1 {
		t.Fatal(executions, e)
	}
	if out, e := f.s.Payments.QueryRefund(f.ctx, f.operator, m.ID, executions[0].ExecutionID); e != nil || out.State != "closed" {
		t.Fatal(out, e)
	}
	if _, e = f.s.BeginManualRefund(f.ctx, f.owner, f.operator, l.ID, "third-instruction", "authorize only remaining 1 USD"); e != nil {
		t.Fatal(e)
	}
	final := proof
	final.ExternalReference, final.Money.Amount = "actual-third-route-1", d("1")
	final.Destination.Address, final.Destination.AuthorizationReference = a3.Address, a3.ID
	if out, e := f.s.Payments.RecordResponsibleBankRefund(f.ctx, f.owner, f.operator, m.ID, payments.Refund{ID: l.ID}, final); e != nil || out.State != "succeeded" {
		t.Fatal(out, e)
	}
	// A late, separately verified real transfer to an old authorized endpoint
	// still records all cash. With the debt settled, it is merchant overpayment,
	// never a second deduction from this or any other customer's principal.
	late := proof
	late.ExternalReference, late.Money.Amount = "late-real-second-route-2", d("2")
	if out, e := f.s.Payments.RecordResponsibleBankRefund(f.ctx, f.owner, f.operator, m.ID, payments.Refund{ID: l.ID}, late); !errors.Is(e, payments.ErrSettlementReview) || out.Fact == nil {
		t.Fatal(out, e)
	}
	if out, e := f.s.Payments.RecordResponsibleBankRefund(f.ctx, f.owner, f.operator, m.ID, payments.Refund{ID: l.ID}, late); !errors.Is(e, payments.ErrSettlementReview) || out.Fact == nil {
		t.Fatal("late fact replay lost review or repeated cash", out, e)
	}
	cap, e := f.s.ReceiptRefundCapacity(f.ctx, nil, f.operator, l.ReceiptID)
	if e != nil || cap.CompletedMoney.Cmp(d("10")) != 0 || !cap.ReservedMoney.IsZero() {
		t.Fatal(cap, e)
	}
	if wallet := f.balance(t); wallet.Balance.Cmp(d("90")) != 0 || !wallet.Held.IsZero() {
		t.Fatal("late real transfer touched settled customer principal", wallet)
	}
	cash, e := f.s.account(f.ctx, f.db, f.operator, f.operator, "control:external_cash")
	if e != nil || cash.Balance.Cmp(d("12")) != 0 { // all three actual outgoing transfers
		t.Fatal(cash, e)
	}
	loss, e := f.s.account(f.ctx, f.db, f.operator, f.operator, "control:refund_overpayment")
	if e != nil || loss.Balance.Cmp(d("-2")) != 0 {
		t.Fatal(loss, e)
	}
	history, e := f.s.RefundRouteHistory(f.ctx, f.owner, f.operator, l.ID)
	if e != nil || len(history) != 3 || history[0].Destination.Address != a1.Address || history[1].Destination.Address != a2.Address || history[2].Destination.Address != a3.Address {
		t.Fatal(history, e)
	}
}
