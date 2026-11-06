package remediation

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/payments"
)

func TestPostgresUnavailableOriginalBankUsesExplicitCustomerAuthorizedReplacement(t *testing.T) {
	f := setup(t)
	f.topup(t, "closed-original-bank-source", "100")
	f.s.Payments = payments.NewService(f.db, "isolated-replacement-bank-key", f.s.Organization, payments.NewTransport(true), payments.CommerceSink{Commerce: f.s.Commerce})
	f.s.Payments.RefundApproval = f.s
	f.s.Payments.RefundSink = f.s
	c, e := f.requestPurchaseRefund(t, "return-through-replacement-bank", "10")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.ReviewRefund(f.ctx, f.owner, f.operator, c.ID, "approve", "original payment verified and refund principal reserved"); e != nil {
		t.Fatal(e)
	}
	legs, e := f.s.Legs(f.ctx, nil, f.operator, c.ID)
	if e != nil || len(legs) != 1 {
		t.Fatal(legs, e)
	}
	leg := legs[0]
	user, _ := f.s.Organization.GetCustomer(f.ctx, nil, f.customer)
	dest := RefundDestination{Network: "bank", Asset: "USD", Address: "bank:independently-authorized-original-payer"}
	p, e := f.s.ProposeRefundDestination(f.ctx, f.customer, user.BrandID, leg.ID, "explicit-replacement-consent", RefundDestinationProposalInput{Destination: dest, ResponsibleExecution: true, AuthorizationReference: "customer confirmed replacement execution to original payer"})
	if e != nil {
		t.Fatal(e)
	}
	input := RefundDestinationAuthorizationInput{ProposalID: p.ID, ResponsibleExecution: true, PayerAuthorizationReference: "original payer authorized destination despite original bank unavailable", VerificationEvidence: "independently verified original payer and receiving account"}
	if _, e = f.s.AuthorizeRefundDestination(f.ctx, f.owner, f.operator, leg.ID, "missing-route-exception", input); !errors.Is(e, ErrReview) {
		t.Fatal("replacement lacked original-route exception evidence", e)
	}
	input.RouteExceptionReference = "original bank account closed; replacement execution explicitly authorized"
	a, e := f.s.AuthorizeRefundDestination(f.ctx, f.owner, f.operator, leg.ID, "verified-replacement", input)
	if e != nil || !a.ResponsibleExecution {
		t.Fatal(a, e)
	}
	m := payments.Merchant{ID: uuid.NewString(), OperatorID: f.operator, Driver: "manual", Enabled: true, Credentials: map[string]string{"currency": "USD", "recipient_reference": "bank:replacement-merchant-account", "instructions": "verify actual outgoing transfer"}}
	if _, e = f.s.Payments.SaveMerchant(f.ctx, f.owner, m, 0); e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.BeginManualRefund(f.ctx, f.owner, f.operator, leg.ID, "replacement-first-instruction", "send original USD liability through verified replacement account"); e != nil {
		t.Fatal(e)
	}
	at, e := instant(f.db)
	if e != nil {
		t.Fatal(e)
	}
	proof := payments.ExternalRefundVerification{ReceiptID: leg.ReceiptID, ExternalReference: "replacement-full-10", Money: payments.Money{Currency: "USD", Amount: d("10")}, ExecutedAt: at, EvidenceReference: "actual full bank payment 10", Destination: payments.ExternalRefundDestination{Network: a.Network, Asset: a.Asset, Address: a.Address, AuthorizationReference: a.ID, VerificationEvidence: "verified completed actual outgoing bank transaction"}}
	for range 2 {
		if out, e := f.s.Payments.RecordResponsibleBankRefund(f.ctx, f.owner, f.operator, m.ID, payments.Refund{ID: leg.ID}, proof); e != nil || out.State != "succeeded" {
			t.Fatal(out, e)
		}
	}
	if a := f.balance(t); a.Balance.Cmp(d("100")) != 0 || !a.Held.IsZero() {
		t.Fatal(a)
	}
	cap, e := f.s.ReceiptRefundCapacity(f.ctx, nil, f.operator, leg.ReceiptID)
	if e != nil || cap.CompletedMoney.Cmp(d("10")) != 0 || !cap.ReservedMoney.IsZero() {
		t.Fatal(cap, e)
	}
}
