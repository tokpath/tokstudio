package cooperation_test

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/cooperation"
	"github.com/tokpath/tokstudio/backend/internal/incentives"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/operations"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"github.com/tokpath/tokstudio/backend/internal/payments"
	"github.com/tokpath/tokstudio/backend/internal/platform/db/dbtest"
	"github.com/tokpath/tokstudio/backend/internal/remediation"
	"gorm.io/gorm"
	"io/fs"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func TestExternalStablecoinProcurementRefundAuthorizationAndActualCash(t *testing.T) {
	base := os.Getenv("TOKENHUB_PAYMENT_MOCK_URL")
	if base == "" {
		t.Skip("independent refund HTTP/RPC lab required")
	}
	for _, mode := range []string{"evm", "evm_partial", "evm_overpaid", "evm_misdirected", "now_external", "evm_replacement_bank", "evm_replacement_wallet", "evm_failure_replacement"} {
		t.Run(mode, func(t *testing.T) { testExternalStablecoinProcurement(t, base, mode) })
	}
}
func testExternalStablecoinProcurement(t *testing.T, base, mode string) {
	db := dbtest.New(t)
	apply := func(_ string, f fs.FS) {
		t.Helper()
		files, e := fs.ReadDir(f, ".")
		if e != nil {
			t.Fatal(e)
		}
		for _, file := range files {
			b, e := fs.ReadFile(f, file.Name())
			if e != nil {
				t.Fatal(e)
			}
			if e = db.Exec(string(b)).Error; e != nil {
				t.Fatal(e)
			}
		}
	}
	apply(organization.Migrations())
	apply(ledger.Migrations())
	apply(commerce.Migrations())
	apply(incentives.Migrations())
	apply(cooperation.Migrations())
	apply(operations.Migrations())
	apply(payments.Migrations())
	apply(remediation.Migrations())
	ctx := context.Background()
	org := organization.New(db)
	owner, buyerActor := uuid.NewString(), uuid.NewString()
	platform, e := org.BootstrapPlatform(ctx, nil, organization.BootstrapInput{BrandName: "Stablecoin Test", OperatorName: "Supplier", OwnerStaffID: owner})
	if e != nil {
		t.Fatal(e)
	}
	child, e := org.CreateOperator(ctx, nil, organization.CreateOperatorInput{ActorStaffID: owner, ParentOperatorID: platform.Operator.ID, Kind: organization.Channel, Name: "Procurement buyer"})
	if e != nil {
		t.Fatal(e)
	}
	invite, e := org.InviteStaff(ctx, nil, owner, child.Operator.ID, "independent-buyer@example.test", "owner")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = org.AcceptStaffInvitation(ctx, nil, invite.Token, organization.VerifiedStaff{ID: buyerActor, Email: "independent-buyer@example.test", EmailVerified: true}); e != nil {
		t.Fatal(e)
	}
	supplier, buyer := platform.Operator.ID, child.Operator.ID
	l := ledger.New(db)
	coop := cooperation.New(db, l, org)
	coop.Rates = fixedExitRate{}
	collector := commerce.New(db)
	transport := payments.NewTransport(true)
	defer transport.Client.CloseIdleConnections()
	pay := payments.NewService(db, "stablecoin-test", org, transport, payments.CommerceSink{Commerce: collector})
	contract, wallet, destination := "0x"+strings.Repeat("aa", 20), "0x"+strings.Repeat("bb", 20), "0x"+strings.Repeat("dd", 20)
	m := payments.Merchant{ID: uuid.NewString(), OperatorID: supplier, Driver: "evm_wallet", Enabled: true, APIBaseURL: base, Credentials: map[string]string{"rpc_url": base + "/rpc", "chain_id": "1", "token_contract": contract, "token_decimals": "6", "wallet_address": wallet, "asset": "USDC", "min_confirmations": "12"}}
	currency := "USDC"
	network := "1"
	if mode == "now_external" {
		m.Driver = "nowpayments"
		currency = "USDTERC20"
		network = "ethereum"
		m.Credentials = map[string]string{"api_key": "now_api_lab", "ipn_secret": "now_ipn_lab", "pay_currency": "usdterc20", "pay_network": network, "pay_token_contract": contract, "pay_asset_verification_reference": "verified-provider-full-currencies-record"}
	}
	asset, e := pay.PublishAssetNetwork(ctx, owner, supplier, payments.AssetNetwork{Driver: m.Driver, Asset: currency, Network: network, TokenContract: contract, TokenDecimals: 6, MinimumConfirmations: 12, Enabled: true, EvidenceReference: "independently verified platform supported asset"}, 0)
	if e != nil {
		t.Fatal(e)
	}
	m.AssetNetworkVersionID = asset.VersionID
	if _, e = pay.SaveMerchant(ctx, owner, m, 0); e != nil {
		t.Fatal(e)
	}
	quote, e := coop.PublishPurchaseQuote(ctx, nil, owner, cooperation.PurchaseQuote{SupplierID: supplier, BuyerID: buyer, PriceUSD: ledger.FromInt64(10), PaidATG: ledger.FromInt64(100), ExitTerms: json.RawMessage(`{"version":1,"voluntary_refund":true,"method":"remaining_paid_proportional","terms_reference":"signed-original-source-return"}`), TermsReference: "signed-inclusive-final-price", Enabled: true})
	if e != nil {
		t.Fatal(e)
	}
	order, e := coop.CreatePurchaseOrder(ctx, nil, buyerActor, buyer, quote.ID, "purchase")
	if e != nil {
		t.Fatal(e)
	}
	now := time.Now().UTC()
	valuation := payments.Valuation{ID: "verified-current-stablecoin-price", Currency: currency, USDPerUnit: ledger.FromInt64(1), Source: "independent contemporaneous price fixture", EffectiveAt: now.Add(-10 * time.Minute), ExpiresAt: now.Add(time.Hour)}
	o := payments.Order{ID: order.ID, OperatorID: supplier, MerchantID: m.ID, Money: payments.Money{Currency: currency, Amount: ledger.FromInt64(10)}, Valuation: valuation, AcceptedAt: now, ExpiresAt: order.ExpiresAt, NotifyURL: "https://callback.example.test/verified"}
	checkout, e := pay.Create(ctx, supplier, m.ID, o)
	if e != nil {
		t.Fatal(e)
	}
	control := func(path string, v any) {
		t.Helper()
		raw, _ := json.Marshal(v)
		response, e := http.Post(base+path, "application/json", strings.NewReader(string(raw)))
		if e != nil {
			t.Fatal(e)
		}
		defer response.Body.Close()
		if response.StatusCode != 200 {
			t.Fatal("independent control", response.StatusCode)
		}
	}
	if mode == "now_external" {
		control("/__control/protocol/settle", map[string]any{"driver": "nowpayments", "provider_id": checkout.ProviderID})
		actual, e := pay.Reconcile(ctx, supplier, m.ID, o.ID)
		if !errors.Is(e, payments.ErrAwaitingValuation) {
			t.Fatalf("NOW unknown actual payment time must remain pending: %v", e)
		}
		if e = db.Model(&cooperation.PurchaseOrder{}).Where("id=?", order.ID).Update("status", "closed").Error; e != nil {
			t.Fatal(e)
		}
		if e = db.Transaction(func(tx *gorm.DB) error { return coop.RequireNoPendingProcurementReceipts(ctx, tx, buyer, pay) }); !errors.Is(e, cooperation.ErrLiability) {
			t.Fatalf("closed historical procurement lost actual unvalued receipt: %v", e)
		}
		if _, e = pay.ConfirmReceiptValuation(ctx, owner, supplier, actual.ID, payments.ReceiptValuationConfirmation{BusinessKey: "independent-original-payment-valuation", Valuation: valuation, PaidAt: &now, PaymentTimeEvidence: "independent provider receipt actual transfer time verified", EvidenceReference: "contemporaneous independently verified USD valuation"}); e != nil {
			t.Fatal(e)
		}
		if e = db.Transaction(func(tx *gorm.DB) error { return coop.RequireNoPendingProcurementReceipts(ctx, tx, buyer, pay) }); !errors.Is(e, cooperation.ErrLiability) {
			t.Fatalf("recorded but unallocated procurement money lost from exit obligations: %v", e)
		}
		if e = db.Model(&cooperation.PurchaseOrder{}).Where("id=?", order.ID).Update("status", order.Status).Error; e != nil {
			t.Fatal(e)
		}
	} else {
		tx := "0x" + strings.ReplaceAll(uuid.NewString()+uuid.NewString(), "-", "")
		control("/__control/protocol/evm", map[string]any{"transaction_hash": tx, "raw_units": "10000000", "head": 120, "chain_id": "0x1", "reorg": false})
		if _, e = pay.VerifyWalletReceipt(ctx, supplier, m.ID, o.ID, tx, "0x0"); e != nil {
			t.Fatal(e)
		}
	}
	links, e := pay.ReceiptsForOrder(ctx, supplier, m.ID, o.ID)
	if e != nil || len(links) != 1 {
		t.Fatal(links, e)
	}
	receipt := links[0].ReceiptID
	e = db.Transaction(func(tx *gorm.DB) error {
		if _, e := coop.ApplyVerifiedPurchaseReceipt(ctx, tx, collector, order.ID, receipt, "original", ledger.FromInt64(10)); e != nil {
			return e
		}
		_, e := coop.FulfillPurchase(ctx, tx, order.ID, owner)
		return e
	})
	if e != nil {
		t.Fatal(e)
	}
	capacity := remediation.New(db, remediation.Dependencies{Ledger: l, Organization: org, Commerce: collector, Cooperation: coop, Payments: pay, Incentives: incentives.New(db)})
	refunds := &cooperation.ProcurementRefunds{Cooperation: coop, Payments: pay, Collector: collector, Capacity: capacity, ObserveReceiptRefund: func(ctx context.Context, tx *gorm.DB, op, receipt, reference, fact string, amount ledger.Decimal) error {
		_, e := capacity.ObserveReceiptRefund(ctx, tx, op, receipt, reference, fact, amount)
		return e
	}}
	exit, e := coop.RequestProcurementExit(ctx, nil, buyerActor, buyer, "exit", "voluntary_exit", "buyer explicitly exits unused procurement", ledger.FromInt64(100), cooperation.CoverageRemovalGuard{Cooperation: coop, PaymentFacts: pay})
	if e != nil {
		t.Fatal(e)
	}
	leg, e := refunds.PrepareRefund(ctx, owner, exit.ID, receipt, "original-source", ledger.FromInt64(10))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = refunds.BeginManualRefund(ctx, owner, leg.ID, "premature", "not yet authorized address"); e == nil {
		t.Fatal("original sender guessed as external refund destination")
	}
	d := payments.ExternalRefundDestination{Network: network, Asset: currency, TokenContract: contract, Address: destination}
	proposal, e := refunds.ProposeRefundDestination(ctx, buyerActor, buyer, leg.ID, d, "verified independent original payer signed new destination")
	if e != nil {
		t.Fatal(e)
	}
	auth, e := refunds.VerifyRefundDestination(ctx, owner, supplier, proposal.ID, "independent verified payer identity and address/network control")
	if e != nil {
		t.Fatal(e)
	}
	if auth.ID == "" {
		t.Fatal("missing durable authorization")
	}
	views, e := refunds.BuyerRefundLegs(ctx, buyerActor, buyer)
	if e != nil || len(views) != 1 || views[0].Driver != m.Driver {
		t.Fatal("buyer without supplier payments privilege cannot propose", views, e)
	}
	if _, e = refunds.BeginManualRefund(ctx, owner, leg.ID, "initial-transfer", "finance approved exact original-source obligation"); e != nil {
		t.Fatal(e)
	}
	if mode == "evm_failure_replacement" {
		if _, e = refunds.CloseRefundRoute(ctx, owner, supplier, leg.ID, "initial route unavailable", "unverified timeout cannot close"); e == nil {
			t.Fatal("unknown manual execution allowed a new route")
		}
		failure := cooperation.ManualRefundFailureInput{BusinessKey: "verified-terminal-failure", ExpectedExecutionKey: "initial-transfer", Result: "failed", ExternalReference: "independent-wallet-execution-cancelled", NoFurtherPaymentReference: "provider irrevocable cancellation receipt", Evidence: "finance independently verified no signed/broadcast transaction remains", VerifiedAt: now}
		if _, e = refunds.RecordManualRefundFailure(ctx, owner, supplier, leg.ID, failure); e != nil {
			t.Fatal(e)
		}
		if _, e = refunds.CloseRefundRoute(ctx, owner, supplier, leg.ID, "wallet permanently unavailable", "independent final cancellation proof verified"); e != nil {
			t.Fatal(e)
		}
		testProcurementReplacement(ctx, t, db, pay, refunds, capacity, owner, buyerActor, supplier, buyer, receipt, leg, base, "bank", ledger.FromInt64(10), now, contract, asset.VersionID, valuation, control)
		return
	}
	if mode == "now_external" {
		result, e := refunds.RecordExternalRefund(ctx, owner, leg.ID, payments.ExternalRefundVerification{ExternalReference: "verified-provider-external-return-" + uuid.NewString(), Money: payments.Money{Currency: currency, Amount: ledger.FromInt64(10)}, ExecutedAt: now, EvidenceReference: "provider executed return record independently verified"}, valuation, now, "current execution valuation")
		if e != nil || result.State != "succeeded" || result.Fact == nil || result.Fact.ExternalRefund.VerificationMethod != "finance_verified_external_record" {
			t.Fatal(result, e)
		}
		return
	}
	units := "10000000"
	if mode == "evm_partial" || strings.HasPrefix(mode, "evm_replacement_") {
		units = "9000000"
	}
	if mode == "evm_overpaid" {
		units = "11000000"
	}
	actualRecipient := destination
	if mode == "evm_misdirected" {
		actualRecipient = "0x" + strings.Repeat("ff", 20)
	}
	txHash := "0x" + strings.ReplaceAll(uuid.NewString()+uuid.NewString(), "-", "")
	control("/__control/protocol/evm", map[string]any{"transaction_hash": txHash, "sender": wallet, "recipient": actualRecipient, "raw_units": units, "head": 100, "reorg": false})
	v := payments.EVMRefundVerification{TransactionHash: txHash, LogIndex: "0x0"}
	if _, e = refunds.RecordEVMRefund(ctx, owner, leg.ID, v, valuation, now, "current independent executed value"); !errors.Is(e, payments.ErrNotFinal) {
		t.Fatalf("unconfirmed transfer accepted: %v", e)
	}
	control("/__control/protocol/evm", map[string]any{"transaction_hash": txHash, "sender": wallet, "recipient": actualRecipient, "raw_units": units, "head": 120, "reorg": false})
	result, callErr := refunds.RecordEVMRefund(ctx, owner, leg.ID, v, valuation, now, "current independent executed value")
	current, e := capacity.ReceiptRefundCapacity(ctx, nil, supplier, receipt)
	if e != nil {
		t.Fatal(e)
	}
	if mode == "evm" {
		if callErr != nil || result.State != "succeeded" || current.CompletedUSD.String() != "10" || !current.ReservedUSD.IsZero() {
			t.Fatal(result, current, callErr)
		}
		return
	}
	if !errors.Is(callErr, payments.ErrSettlementReview) || result.State != "verified_pending_settlement" {
		t.Fatalf("actual exception falsely succeeded or disappeared: %+v %v", result, callErr)
	}
	var saved cooperation.ProcurementExitRefund
	if e = db.First(&saved, "id=?", leg.ID).Error; e != nil {
		t.Fatal(e)
	}
	if strings.HasPrefix(mode, "evm_replacement_") {
		if current.CompletedUSD.String() != "9" || current.ReservedUSD.String() != "1" {
			t.Fatal(current)
		}
		closed, e := refunds.CloseRefundRoute(ctx, owner, supplier, leg.ID, "original payment route permanently unavailable", "independently confirmed no remaining outgoing transaction on original route")
		if e != nil || closed.RemainingUSD.String() != "1" {
			t.Fatal(closed, e)
		}
		if result.Fact == nil {
			t.Fatal("missing actual partial fact")
		}
		executions, e := pay.RefundExecutions(ctx, supplier, m.ID, leg.ID)
		if e != nil || len(executions) != 1 {
			t.Fatal(executions, e)
		}
		sealed, e := pay.QueryRefund(ctx, supplier, m.ID, executions[0].ExecutionID)
		if e != nil || sealed.State != "closed" {
			t.Fatal(sealed, e)
		}
		kind := "bank"
		if mode == "evm_replacement_wallet" {
			kind = "wallet"
		}
		testProcurementReplacement(ctx, t, db, pay, refunds, capacity, owner, buyerActor, supplier, buyer, receipt, leg, base, kind, ledger.FromInt64(1), now, contract, asset.VersionID, valuation, control)
		return
	}
	switch mode {
	case "evm_partial":
		if current.CompletedUSD.String() != "9" || current.ReservedUSD.String() != "1" || saved.ActualUSD.String() != "9" || saved.SettledUSD.String() != "9" {
			t.Fatal(current, saved)
		}
		if _, e = refunds.BeginManualRefund(ctx, owner, leg.ID, "explicit-remaining-one", "fresh approval for remaining liability only"); e != nil {
			t.Fatal(e)
		}
		v.TransactionHash = "0x" + strings.ReplaceAll(uuid.NewString()+uuid.NewString(), "-", "")
		control("/__control/protocol/evm", map[string]any{"transaction_hash": v.TransactionHash, "sender": wallet, "recipient": destination, "raw_units": "1000000", "head": 120, "reorg": false})
		result, e = refunds.RecordEVMRefund(ctx, owner, leg.ID, v, valuation, now, "actual supplement execution quote")
		if e != nil || result.State != "succeeded" {
			t.Fatal(result, e)
		}
		current, e = capacity.ReceiptRefundCapacity(ctx, nil, supplier, receipt)
		if e != nil || current.CompletedUSD.String() != "10" || !current.ReservedUSD.IsZero() {
			t.Fatal(current, e)
		}
	case "evm_overpaid":
		if current.CompletedUSD.String() != "10" || saved.ActualUSD.String() != "11" || saved.ExcessUSD.String() != "1" || !current.ReservedUSD.IsZero() {
			t.Fatal(current, saved)
		}
	case "evm_misdirected":
		if !current.CompletedUSD.IsZero() || current.ReservedUSD.String() != "10" || !saved.SettledUSD.IsZero() || saved.ActualUSD.String() != "10" {
			t.Fatal(current, saved)
		}
	}
	var cash ledger.Account
	if e = db.Where("operator_id=? AND purpose='control:external_cash'", supplier).First(&cash).Error; e != nil {
		t.Fatal(e)
	}
	expected := "10"
	if mode == "evm_overpaid" {
		expected = "11"
	}
	if cash.Balance.String() != expected {
		t.Fatalf("real executed cash missing: %s want %s", cash.Balance, expected)
	}
}

func testProcurementReplacement(ctx context.Context, t *testing.T, db *gorm.DB, pay *payments.Service, refunds *cooperation.ProcurementRefunds, capacity *remediation.Service, owner, buyerActor, supplier, buyer, receipt string, leg cooperation.ProcurementExitRefund, base, kind string, remaining ledger.Decimal, now time.Time, contract, assetVersion string, valuation payments.Valuation, control func(string, any)) {
	t.Helper()
	merchant := payments.Merchant{ID: uuid.NewString(), OperatorID: supplier, Driver: "manual", Enabled: true, Credentials: map[string]string{"currency": "USD", "recipient_reference": "independently-verified-new-bank-account", "instructions": "verified bank account"}}
	d := payments.ExternalRefundDestination{Network: "bank", Asset: "USD", Address: "original-payer-independent-bank-destination"}
	if kind == "wallet" {
		merchant.Driver = "evm_wallet"
		merchant.APIBaseURL = base
		merchant.AssetNetworkVersionID = assetVersion
		merchant.Credentials = map[string]string{"rpc_url": base + "/rpc", "wallet_address": "0x" + strings.Repeat("ff", 20)}
		d = payments.ExternalRefundDestination{Network: "1", Asset: "USDC", TokenContract: contract, Address: "0x" + strings.Repeat("ab", 20)}
	}
	if _, e := pay.SaveMerchant(ctx, owner, merchant, 0); e != nil {
		t.Fatal(e)
	}
	proposal, e := refunds.ProposeRefundDestination(ctx, buyerActor, buyer, leg.ID, d, "original payer explicitly authorizes new route for only outstanding USD amount")
	if e != nil || proposal.Version != 2 {
		t.Fatal(proposal, e)
	}
	auth, e := refunds.VerifyReplacementRefundDestination(ctx, owner, supplier, proposal.ID, merchant.ID, "independent verified original payer and new payment destination")
	if e != nil || auth.Version != 2 || auth.ExecutionMerchantID != merchant.ID {
		t.Fatal(auth, e)
	}
	if _, e = refunds.BeginManualRefund(ctx, owner, leg.ID, "explicit-new-route-only-remaining", "new payer account and remaining USD obligation explicitly approved"); e != nil {
		t.Fatal(e)
	}
	external := payments.ExternalRefundVerification{ExternalReference: "bank-actual-" + uuid.NewString(), Money: payments.Money{Currency: "USD", Amount: remaining}, ExecutedAt: now, EvidenceReference: "verified bank executed outgoing transaction"}
	evm := payments.EVMRefundVerification{}
	if kind == "wallet" {
		evm.TransactionHash = "0x" + strings.ReplaceAll(uuid.NewString()+uuid.NewString(), "-", "")
		evm.LogIndex = "0x0"
		control("/__control/protocol/evm", map[string]any{"transaction_hash": evm.TransactionHash, "raw_units": "1000000", "sender": merchant.Credentials["wallet_address"], "recipient": d.Address, "token_contract": contract, "head": 120, "reorg": false})
	}
	result, e := refunds.RecordReplacementRefund(ctx, owner, supplier, leg.ID, evm, external, valuation, now, "independent actual supplemental execution value")
	if e != nil || result.State != "succeeded" {
		t.Fatal(result, e)
	}
	var saved cooperation.ProcurementExitRefund
	if e = db.First(&saved, "id=?", leg.ID).Error; e != nil {
		t.Fatal(e)
	}
	if saved.SettledUSD.String() != "10" || saved.State != "completed" {
		t.Fatal(saved)
	}
	cap, e := capacity.ReceiptRefundCapacity(ctx, nil, supplier, receipt)
	if e != nil || cap.CompletedUSD.String() != "10" || !cap.ReservedUSD.IsZero() {
		t.Fatal(cap, e)
	}
	_, authorizations, e := refunds.RefundDestinations(ctx, buyerActor, buyer, leg.ID)
	if e != nil || len(authorizations) != 2 {
		t.Fatal(authorizations, e)
	}
	status, e := refunds.RefundRouteStatus(ctx, buyerActor, buyer, leg.ID)
	if e != nil || status.Version != 2 || len(status.Closures) != 1 || status.Closures[0].Evidence != "" || status.Closures[0].PaymentFacts != nil {
		t.Fatal(status, e)
	}
	if kind == "wallet" {
		var originalAuth cooperation.RefundDestinationAuthorization
		for _, a := range authorizations {
			if a.Version == 1 {
				originalAuth = a
			}
		}
		late := payments.EVMRefundVerification{TransactionHash: "0x" + strings.ReplaceAll(uuid.NewString()+uuid.NewString(), "-", ""), LogIndex: "0x0", Destination: payments.ExternalRefundDestination{AuthorizationReference: originalAuth.ID}}
		control("/__control/protocol/evm", map[string]any{"transaction_hash": late.TransactionHash, "raw_units": "2000000", "sender": "0x" + strings.Repeat("bb", 20), "recipient": "0x" + strings.Repeat("dd", 20), "token_contract": contract, "head": 120, "reorg": false})
		actual, e := refunds.RecordEVMRefund(ctx, owner, leg.ID, late, valuation, now, "verified late actual original-route execution")
		if !errors.Is(e, payments.ErrSettlementReview) || actual.Fact == nil {
			t.Fatal(actual, e)
		}
		if e = db.First(&saved, "id=?", leg.ID).Error; e != nil {
			t.Fatal(e)
		}
		if saved.ActualUSD.String() != "12" || saved.SettledUSD.String() != "10" || saved.ExcessUSD.String() != "2" {
			t.Fatal(saved)
		}
		cap, e = capacity.ReceiptRefundCapacity(ctx, nil, supplier, receipt)
		if e != nil || cap.CompletedUSD.String() != "10" || !cap.ReservedUSD.IsZero() {
			t.Fatal(cap, e)
		}
		var cash ledger.Account
		if e = db.Where("operator_id=? AND purpose='control:external_cash'", supplier).First(&cash).Error; e != nil || cash.Balance.String() != "12" {
			t.Fatal(cash, e)
		}
	}
}
