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
	c, e := f.s.RequestWalletRefund(f.ctx, WalletRefundInput{OperatorID: f.operator, CustomerID: f.customer, BusinessKey: "return-through-replacement-bank", AmountUSD: d("10"), Reason: "original account cannot execute return"})
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
	proof := payments.ExternalRefundVerification{ReceiptID: leg.ReceiptID, ExternalReference: "replacement-first-9", Money: payments.Money{Currency: "USD", Amount: d("9")}, ExecutedAt: at, EvidenceReference: "actual executed bank payment 9", Destination: payments.ExternalRefundDestination{Network: a.Network, Asset: a.Asset, Address: a.Address, AuthorizationReference: a.ID, VerificationEvidence: "verified completed actual outgoing bank transaction"}}
	if out, e := f.s.Payments.RecordResponsibleBankRefund(f.ctx, f.owner, f.operator, m.ID, payments.Refund{ID: leg.ID}, proof); !errors.Is(e, payments.ErrSettlementReview) || out.State != "verified_pending_settlement" {
		t.Fatal(out, e)
	}
	if _, e = f.s.BeginManualRefund(f.ctx, f.owner, f.operator, leg.ID, "replacement-second-instruction", "first actual9 confirmed; explicitly authorize last1"); e != nil {
		t.Fatal(e)
	}
	proof.ExternalReference = "replacement-final-1"
	proof.Money.Amount = d("1")
	if out, e := f.s.Payments.RecordResponsibleBankRefund(f.ctx, f.owner, f.operator, m.ID, payments.Refund{ID: leg.ID}, proof); e != nil || out.State != "succeeded" {
		t.Fatal(out, e)
	}
	if a := f.balance(t); a.Balance.Cmp(d("90")) != 0 || !a.Held.IsZero() {
		t.Fatal(a)
	}
	cap, e := f.s.ReceiptRefundCapacity(f.ctx, nil, f.operator, leg.ReceiptID)
	if e != nil || cap.CompletedUSD.Cmp(d("10")) != 0 || !cap.ReservedUSD.IsZero() {
		t.Fatal(cap, e)
	}
}

func TestPostgresTakeoverRefundUsesBrandOwnWalletAndBankWithoutFormerMerchantAccess(t *testing.T) {
	brand, channel, _, _ := takeoverFixture(t)
	originalBrandCustomer := brand.customer
	brand.topup(t, "unrelated-brand-customer-source", "10")
	unrelated := brand.receiptNamed(t, "unrelated-brand-customer-source")
	ext := externalRefundSetupFor(t, channel, false, brand)
	takeover := beginTakeover(t, brand, channel)
	snapshot, e := brand.s.RefreshTakeover(brand.ctx, brand.owner, brand.operator, takeover.ID)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = brand.s.ExecuteTakeover(brand.ctx, brand.owner, brand.operator, takeover.ID, snapshot.SnapshotFingerprint); e != nil {
		t.Fatal(e)
	}
	brand.customer = channel.customer
	c, e := brand.s.RequestWalletRefund(brand.ctx, WalletRefundInput{OperatorID: brand.operator, CustomerID: brand.customer, BusinessKey: "assumed-original-sources-return", AmountUSD: d("45"), Reason: "return original25 bank and20 stablecoin after complete takeover"})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = brand.s.ReviewRefund(brand.ctx, brand.owner, brand.operator, c.ID, "approve", "brand now owns complete customer responsibility"); e != nil {
		t.Fatal(e)
	}
	legs, e := brand.s.Legs(brand.ctx, nil, brand.operator, c.ID)
	if e != nil || len(legs) != 2 {
		t.Fatal(legs, e)
	}
	var bank RefundLeg
	ext.fixture = brand
	for _, leg := range legs {
		if leg.ReceiptID == ext.receipt {
			ext.leg = leg
		} else {
			bank = leg
		}
		if leg.SourceOperatorID != channel.operator {
			t.Fatal("original collector lost", leg)
		}
	}
	if ext.leg.AmountUSD.Cmp(d("20")) != 0 || bank.AmountUSD.Cmp(d("25")) != 0 {
		t.Fatal(legs)
	}
	// Simulate a former merchant whose secrets are no longer recoverable. The
	// brand can still settle its own obligation from its own configured wallet.
	if e = brand.db.Table("payments_merchants").Where("id=?", ext.merchant.ID).Update("credentials_ciphertext", "former-merchant-secrets-unavailable").Error; e != nil {
		t.Fatal(e)
	}
	if _, e = brand.s.Payments.Merchant(brand.ctx, brand.operator, ext.merchant.ID); e == nil {
		t.Fatal("brand acquired former merchant access")
	}
	a := ext.authorize(t)
	if !a.ResponsibleExecution {
		t.Fatal("takeover did not select responsible execution", a)
	}
	current := payments.Merchant{ID: uuid.NewString(), OperatorID: brand.operator, Driver: "evm_wallet", Enabled: true, AssetNetworkVersionID: ext.merchant.AssetNetworkVersionID, Credentials: map[string]string{}}
	for k, v := range ext.merchant.Credentials {
		current.Credentials[k] = v
	}
	current.Credentials["wallet_address"] = "0x6666666666666666666666666666666666666666"
	if _, e = brand.s.Payments.SaveMerchant(brand.ctx, brand.owner, current, 0); e != nil {
		t.Fatal(e)
	}
	if _, e = brand.s.BeginManualRefund(brand.ctx, brand.owner, brand.operator, ext.leg.ID, "brand-wallet-first-send", "brand own wallet to independently authorized original payer destination"); e != nil {
		t.Fatal(e)
	}
	r, v := ext.refund(t, a, "6")
	wrong := v
	wrong.ReceiptID = unrelated.ID
	if _, e = brand.s.Payments.RecordResponsibleEVMRefund(brand.ctx, brand.owner, brand.operator, current.ID, r, wrong); !errors.Is(e, payments.ErrConflict) {
		t.Fatal("another customer's source selected", e)
	}
	if _, e = brand.s.Payments.RecordResponsibleEVMRefund(brand.ctx, channel.owner, brand.operator, current.ID, r, v); e == nil {
		t.Fatal("former channel owner spent brand funds")
	}
	if out, e := brand.s.Payments.RecordResponsibleEVMRefund(brand.ctx, brand.owner, brand.operator, current.ID, r, v); !errors.Is(e, payments.ErrSettlementReview) || out.State != "verified_pending_settlement" {
		t.Fatal(out, e)
	}
	cap, e := brand.s.ReceiptRefundCapacity(brand.ctx, nil, channel.operator, ext.receipt)
	if e != nil || cap.CompletedUSD.Cmp(d("19")) != 0 || cap.ReservedUSD.Cmp(d("1")) != 0 {
		t.Fatal(cap, e)
	}
	if _, e = brand.s.BeginManualRefund(brand.ctx, brand.owner, brand.operator, ext.leg.ID, "brand-wallet-supplement", "verified original19; authorize remaining1 from brand"); e != nil {
		t.Fatal(e)
	}
	_, v = ext.refund(t, a, "7")
	if out, e := brand.s.Payments.RecordResponsibleEVMRefund(brand.ctx, brand.owner, brand.operator, current.ID, r, v); e != nil || out.State != "succeeded" {
		t.Fatal(out, e)
	}
	cap, e = brand.s.ReceiptRefundCapacity(brand.ctx, nil, channel.operator, ext.receipt)
	if e != nil || cap.CompletedUSD.Cmp(d("20")) != 0 || !cap.ReservedUSD.IsZero() {
		t.Fatal(cap, e)
	}
	// The original bank source is now fulfilled by the brand's own verified
	// bank account; no legacy payment merchant is needed to identify it.
	bankView := ext
	bankView.leg = bank
	bankView.destination = RefundDestination{Network: "bank", Asset: "USD", Address: "bank:verified-original-payer-account"}
	bankAuth := bankView.authorize(t)
	account := payments.Merchant{ID: uuid.NewString(), OperatorID: brand.operator, Driver: "manual", Enabled: true, Credentials: map[string]string{"currency": "USD", "recipient_reference": "bank:brand-own-account", "instructions": "independent finance verifies actual transfer"}}
	if _, e = brand.s.Payments.SaveMerchant(brand.ctx, brand.owner, account, 0); e != nil {
		t.Fatal(e)
	}
	if _, e = brand.s.BeginManualRefund(brand.ctx, brand.owner, brand.operator, bank.ID, "brand-bank-instruction", "brand own bank to authorized original payer"); e != nil {
		t.Fatal(e)
	}
	at, e := instant(brand.db)
	if e != nil {
		t.Fatal(e)
	}
	bankProof := payments.ExternalRefundVerification{ReceiptID: bank.ReceiptID, ExternalReference: "brand-bank-actual-25", Money: payments.Money{Currency: "USD", Amount: d("25")}, ExecutedAt: at, EvidenceReference: "independently verified executed transfer and recipient account", Destination: payments.ExternalRefundDestination{Network: bankAuth.Network, Asset: bankAuth.Asset, TokenContract: bankAuth.TokenContract, Address: bankAuth.Address, AuthorizationReference: bankAuth.ID, VerificationEvidence: "original payer consent and completed bank transfer"}}
	if out, e := brand.s.Payments.RecordResponsibleBankRefund(brand.ctx, brand.owner, brand.operator, account.ID, payments.Refund{ID: bank.ID}, bankProof); e != nil || out.State != "succeeded" {
		t.Fatal(out, e)
	}
	if _, e = brand.s.Payments.RecordResponsibleBankRefund(brand.ctx, brand.owner, brand.operator, account.ID, payments.Refund{ID: bank.ID}, bankProof); e != nil {
		t.Fatal("responsible bank replay", e)
	}
	if a := brand.balance(t); a.Balance.Cmp(d("80")) != 0 || !a.Held.IsZero() {
		t.Fatal(a)
	}
	other, e := brand.s.account(brand.ctx, brand.db, brand.operator, originalBrandCustomer, "customer:usd")
	if e != nil || other.Balance.Cmp(d("10")) != 0 {
		t.Fatal("unrelated customer asset changed", other, e)
	}
	cash, e := brand.s.account(brand.ctx, brand.db, brand.operator, brand.operator, "control:external_cash")
	if e != nil || cash.Balance.Cmp(d("45")) != 0 {
		t.Fatal("cash attributed to wrong payer", cash, e)
	}
	oldCash, e := brand.s.account(brand.ctx, brand.db, channel.operator, channel.operator, "control:external_cash")
	if e != nil || !oldCash.Balance.IsZero() {
		t.Fatal("new payment rewritten onto former collector", oldCash, e)
	}
	c, e = brand.s.GetCase(brand.ctx, nil, brand.operator, c.ID)
	if e != nil || c.State != "completed" {
		t.Fatal(c, e)
	}
}
