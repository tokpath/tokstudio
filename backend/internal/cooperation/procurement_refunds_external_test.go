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
	"math/big"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

type fixedExitRate struct{}

func (fixedExitRate) CoverageRates(context.Context, *gorm.DB, string, time.Time) (cooperation.CoverageRates, error) {
	return cooperation.CoverageRates{Version: 1, Direct: big.NewRat(1, 1), Children: map[string]*big.Rat{}}, nil
}
func TestExternalProcurementExitRefundSharesReceiptCapacity(t *testing.T) {
	base := os.Getenv("TOKENHUB_PAYMENT_MOCK_URL")
	if base == "" {
		t.Skip("independent payment HTTP lab required")
	}
	for _, mode := range []string{"stripe", "manual", "manual_mismatch", "stripe_disputed_before_submission", "manual_disputed_before_instruction", "manual_disputed_after_instruction"} {
		t.Run(mode, func(t *testing.T) { testProcurementExitRefund(t, base, mode) })
	}
}
func testProcurementExitRefund(t *testing.T, base, mode string) {
	isStripe := strings.HasPrefix(mode, "stripe")
	db := dbtest.New(t)
	apply := func(_ string, files fs.FS) {
		t.Helper()
		entries, e := fs.ReadDir(files, ".")
		if e != nil {
			t.Fatal(e)
		}
		for _, entry := range entries {
			b, e := fs.ReadFile(files, entry.Name())
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
	if e := db.Exec(payments.Schema()).Error; e != nil {
		t.Fatal(e)
	}
	apply(operations.Migrations())
	apply(remediation.Migrations())
	ctx := context.Background()
	org := organization.New(db)
	owner := uuid.NewString()
	platform, e := org.BootstrapPlatform(ctx, nil, organization.BootstrapInput{BrandName: "Synthetic", OperatorName: "Supplier", OwnerStaffID: owner})
	if e != nil {
		t.Fatal(e)
	}
	child, e := org.CreateOperator(ctx, nil, organization.CreateOperatorInput{ActorStaffID: owner, ParentOperatorID: platform.Operator.ID, Kind: organization.Channel, Name: "Buyer", QuoteReference: "quote", ExitTermsReference: "exit"})
	if e != nil {
		t.Fatal(e)
	}
	if e = org.SetSupplyStatus(ctx, nil, owner, child.Operator.ID, "active", "signed procurement contract"); e != nil {
		t.Fatal(e)
	}
	invite, e := org.InviteStaff(ctx, nil, owner, child.Operator.ID, "buyer-owner@example.test", "owner")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = org.AcceptStaffInvitation(ctx, nil, invite.Token, organization.VerifiedStaff{ID: owner, Email: "buyer-owner@example.test", EmailVerified: true}); e != nil {
		t.Fatal(e)
	}
	l := ledger.New(db)
	supplier, buyer := platform.Operator.ID, child.Operator.ID
	coop := cooperation.New(db, l, org)
	coop.Rates = fixedExitRate{}
	collector := commerce.New(db)
	transport := payments.NewTransport(true)
	defer transport.Client.CloseIdleConnections()
	pay := payments.NewService(db, "synthetic-key", org, transport, payments.CommerceSink{Commerce: collector})
	m := payments.Merchant{ID: uuid.NewString(), OperatorID: supplier, Driver: "stripe", Enabled: true, APIBaseURL: base, Credentials: map[string]string{"secret_key": "sk_test_lab", "publishable_key": "pk_test_lab", "webhook_secret": "whsec_integration_test", "currency": "USD"}}
	if !isStripe {
		m.Driver = "manual"
		m.APIBaseURL = ""
		m.Credentials = map[string]string{"currency": "USD", "recipient_reference": "bank:supplier-account", "instructions": "approved transfer instructions"}
	}
	if _, e = pay.SaveMerchant(ctx, owner, m, 0); e != nil {
		t.Fatal(e)
	}
	q, e := coop.PublishPurchaseQuote(ctx, nil, owner, cooperation.PurchaseQuote{SupplierID: supplier, BuyerID: buyer, PriceUSD: ledger.FromInt64(10), PaidATG: ledger.FromInt64(80), GiftATG: ledger.FromInt64(20), ExitTerms: json.RawMessage(`{"version":1,"voluntary_refund":true,"method":"remaining_paid_proportional","terms_reference":"signed-refund-v1"}`), TermsReference: "signed-v1", Enabled: true})
	if e != nil {
		t.Fatal(e)
	}
	order, e := coop.CreatePurchaseOrder(ctx, nil, owner, buyer, q.ID, "actual-purchase")
	if e != nil {
		t.Fatal(e)
	}
	now := time.Now().UTC()
	externalOrder := payments.Order{ID: order.ID, OperatorID: supplier, MerchantID: m.ID, Money: payments.Money{Currency: "USD", Amount: order.DueUSD}, Valuation: payments.Valuation{ID: "native-usd", Currency: "USD", USDPerUnit: ledger.FromInt64(1), Source: "native_usd", EffectiveAt: now.Add(-time.Minute), ExpiresAt: now.Add(time.Hour)}, AcceptedAt: now, ExpiresAt: order.ExpiresAt}
	checkout, e := pay.Create(ctx, supplier, m.ID, externalOrder)
	if e != nil {
		t.Fatal(e)
	}
	control := func(path string, v any) {
		t.Helper()
		raw, _ := json.Marshal(v)
		r, e := http.Post(base+path, "application/json", strings.NewReader(string(raw)))
		if e != nil {
			t.Fatal(e)
		}
		defer r.Body.Close()
		if r.StatusCode < 200 || r.StatusCode >= 300 {
			t.Fatalf("external control status %d", r.StatusCode)
		}
	}
	if isStripe {
		control("/__control/payments/"+checkout.ProviderID+"/settle", map[string]any{})
		if _, e = pay.Reconcile(ctx, supplier, m.ID, order.ID); e != nil {
			t.Fatal(e)
		}
	} else {
		if _, e = pay.RecordManual(ctx, owner, supplier, m.ID, order.ID, payments.ManualVerification{ExternalReference: "bank-purchase-" + order.ID, Money: externalOrder.Money, PaidAt: now, EvidenceReference: "bank-statement:verified-inbound", PayerReference: "bank:original-buyer"}); e != nil {
			t.Fatal(e)
		}
	}
	links, e := pay.ReceiptsForOrder(ctx, supplier, m.ID, order.ID)
	if e != nil || len(links) != 1 {
		t.Fatal("verified original receipt missing", e)
	}
	receiptID := links[0].ReceiptID
	e = db.Transaction(func(tx *gorm.DB) error {
		if _, e := coop.ApplyVerifiedPurchaseReceipt(ctx, tx, collector, order.ID, receiptID, "original", order.DueUSD); e != nil {
			return e
		}
		_, e := coop.FulfillPurchase(ctx, tx, order.ID, owner)
		return e
	})
	if e != nil {
		t.Fatal(e)
	}
	capacity := remediation.New(db, remediation.Dependencies{Ledger: l, Organization: org, Commerce: collector, Cooperation: coop, Payments: pay, Incentives: incentives.New(db)})
	invoiceService := &operations.Service{DB: db, EncryptionKey: "synthetic-invoice-key", Authorize: org, ProcurementInvoices: operations.DomainProcurementInvoiceSource{Cooperation: coop, Commerce: collector, Adjustments: capacity}}
	if _, e = invoiceService.SaveInvoicePolicy(ctx, owner, operations.InvoicePolicy{OperatorID: supplier, PartyKind: "procurement", Enabled: true, Stage: "purchase", MaximumRatio: ledger.FromInt64(1), RulesReference: "supplier-wholesale-invoice-terms"}, 0); e != nil {
		t.Fatal(e)
	}
	invoice, e := invoiceService.RequestProcurementInvoice(ctx, owner, buyer, supplier, "purchase-invoice", []operations.InvoiceLine{{ReceiptID: receiptID, AmountUSD: ledger.FromInt64(10)}}, operations.InvoiceDetails{Title: "Actual Procurement Buyer"})
	if e != nil || invoice.PartyKind != "procurement" || invoice.CustomerID != "" || invoice.BuyerOperatorID == nil || *invoice.BuyerOperatorID != buyer {
		t.Fatalf("purchase invoice impersonated customer or wrong buyer: %+v %v", invoice, e)
	}
	if _, e = invoiceService.RequestProcurementInvoice(ctx, owner, buyer, supplier, "duplicate-invoice", []operations.InvoiceLine{{ReceiptID: receiptID, AmountUSD: ledger.FromInt64(1)}}, operations.InvoiceDetails{Title: "Duplicate"}); !errors.Is(e, operations.ErrConflict) {
		t.Fatalf("purchase source invoiced beyond actual payment: %v", e)
	}
	exit, e := coop.RequestProcurementExit(ctx, nil, owner, buyer, "exit-once", "voluntary_exit", "approved orderly exit", ledger.FromInt64(100), cooperation.CoverageRemovalGuard{Cooperation: coop, PaymentFacts: pay})
	if e != nil || exit.ContractUSD.String() != "10" {
		t.Fatal("gift refunded as cash or exit failed", exit, e)
	}
	refunds := &cooperation.ProcurementRefunds{Cooperation: coop, Payments: pay, Collector: collector, Capacity: capacity}
	refunds.ObserveReceiptRefund = func(ctx context.Context, tx *gorm.DB, op, receipt, reference, fact string, amount ledger.Decimal) error {
		_, e := capacity.ObserveReceiptRefund(ctx, tx, op, receipt, reference, fact, amount)
		return e
	}
	leg, e := refunds.PrepareRefund(ctx, owner, exit.ID, receiptID, "one", ledger.FromInt64(10))
	if e != nil {
		t.Fatal(e)
	}
	if e = db.Transaction(func(tx *gorm.DB) error {
		return capacity.ReserveReceiptRefund(ctx, tx, supplier, receiptID, uuid.NewString(), ledger.FromInt64(1))
	}); e == nil {
		t.Fatal("retail path reused receipt reserved by wholesale exit")
	}
	dispute := func() {
		t.Helper()
		if _, e = capacity.RecordChargeback(ctx, remediation.ChargebackInput{ActorID: owner, OperatorID: supplier, ReceiptID: receiptID, Provider: "synthetic-bank", ProviderCaseID: "later-dispute", AmountUSD: ledger.FromInt64(10), Reason: "external collection under dispute", Evidence: "verified-provider-dispute-case"}); e != nil {
			t.Fatal(e)
		}
	}
	if strings.Contains(mode, "disputed_before") {
		dispute()
		if isStripe {
			_, e = refunds.Execute(ctx, owner, leg.ID, payments.Money{Currency: "USD", Amount: ledger.FromInt64(10)}, payments.Valuation{}, time.Time{}, "")
		} else {
			_, e = refunds.BeginManualRefund(ctx, owner, leg.ID, "bank-refund-instruction", "approved-original-route")
		}
		if !errors.Is(e, remediation.ErrReview) {
			t.Fatalf("new refund execution ignored later dispute: %v", e)
		}
		var count int64
		if e = db.Table("payments_refund_intents").Count(&count).Error; e != nil || count != 0 {
			t.Fatalf("disputed source submitted external refund: %d %v", count, e)
		}
		return
	}
	if !isStripe {
		if _, e = refunds.BeginManualRefund(ctx, owner, leg.ID, "approved-bank-transfer", "finance-authorized-original-route"); e != nil {
			t.Fatal(e)
		}
		if mode == "manual_disputed_after_instruction" {
			dispute()
		}
		proof := payments.ManualRefundVerification{ReceiptID: receiptID, ExternalReference: "bank-refund-" + leg.ID, Money: payments.Money{Currency: "USD", Amount: ledger.FromInt64(10)}, ExecutedAt: time.Now().UTC(), EvidenceReference: "bank-statement:verified-refund", RecipientReference: "bank:wrong-payee"}
		if _, e = refunds.RecordManualRefund(ctx, owner, leg.ID, proof, payments.Valuation{}, time.Time{}, ""); e == nil {
			t.Fatal("manual refund to unrelated payee accepted")
		}
		proof.RecipientReference = "bank:original-buyer"
		if mode == "manual_mismatch" {
			proof.Money.Amount = ledger.FromInt64(9)
		}
		result, callErr := refunds.RecordManualRefund(ctx, owner, leg.ID, proof, payments.Valuation{}, time.Time{}, "")
		if mode == "manual_mismatch" {
			if !errors.Is(callErr, cooperation.ErrRefundFX) || result.State != "verified_pending_settlement" {
				t.Fatalf("manual mismatch result=%+v %v", result, callErr)
			}
			var facts int64
			if e = db.Table("payments_verified_facts").Where("kind='refund'").Count(&facts).Error; e != nil || facts != 1 {
				t.Fatalf("actual bank refund fact was lost: %d %v", facts, e)
			}
			current, e := capacity.ReceiptRefundCapacity(ctx, nil, supplier, receiptID)
			if e != nil || current.ReservedUSD.String() != "1" || current.CompletedUSD.String() != "9" {
				t.Fatalf("actual partial transfer not recognized or full obligation falsely discharged: %+v %v", current, e)
			}
			if _, e = refunds.BeginManualRefund(ctx, owner, leg.ID, "explicit-supplement-one", "approved remaining USD obligation only"); e != nil {
				t.Fatal(e)
			}
			proof.ExternalReference += "-supplement"
			proof.Money.Amount = ledger.FromInt64(1)
			completed, e := refunds.RecordManualRefund(ctx, owner, leg.ID, proof, payments.Valuation{}, time.Time{}, "")
			if e != nil || completed.State != "succeeded" {
				t.Fatalf("explicit remaining supplement failed: %+v %v", completed, e)
			}
			current, e = capacity.ReceiptRefundCapacity(ctx, nil, supplier, receiptID)
			if e != nil || !current.ReservedUSD.IsZero() || current.CompletedUSD.String() != "10" {
				t.Fatalf("supplement did not close exact remaining capacity: %+v %v", current, e)
			}
			return
		}
		if callErr != nil || result.State != "succeeded" {
			t.Fatalf("manual refund result=%+v %v", result, callErr)
		}
		if _, e = refunds.RecordManualRefund(ctx, owner, leg.ID, proof, payments.Valuation{}, time.Time{}, ""); e != nil {
			t.Fatal("same manual proof did not replay", e)
		}
		proof.ExternalReference = "different-bank-refund"
		if _, e = refunds.RecordManualRefund(ctx, owner, leg.ID, proof, payments.Valuation{}, time.Time{}, ""); e == nil {
			t.Fatal("completed manual instruction accepted changed proof")
		}
	} else {
		control("/__control/faults", map[string]any{"path": "/v1/refunds", "mode": "timeout_after", "count": 1})
		transport.Client.CloseIdleConnections()
		result, e := refunds.Execute(ctx, owner, leg.ID, payments.Money{Currency: "USD", Amount: ledger.FromInt64(10)}, payments.Valuation{}, time.Time{}, "")
		if !errors.Is(e, payments.ErrOutcomeUnknown) {
			t.Fatalf("external ambiguous refund %s %v", result.State, e)
		}
		copied := *pay
		copied.RefundSink = refunds
		result, e = copied.QueryRefund(ctx, supplier, m.ID, leg.ID)
		if e != nil || result.State != "pending" {
			t.Fatalf("original pending refund query %s %v", result.State, e)
		}
		current, e := capacity.ReceiptRefundCapacity(ctx, nil, supplier, receiptID)
		if e != nil || current.ReservedUSD.String() != "10" || !current.CompletedUSD.IsZero() {
			t.Fatal("pending refund treated as settled", current, e)
		}
		control("/__control/refunds/"+result.ProviderID+"/settle", map[string]any{"status": "succeeded"})
		if _, e = refunds.Run(ctx, 20); e != nil {
			t.Fatal(e)
		}
	}
	current, e := capacity.ReceiptRefundCapacity(ctx, nil, supplier, receiptID)
	if e != nil || current.CompletedUSD.String() != "10" || !current.ReservedUSD.IsZero() {
		t.Fatal("verified refund not settled shared capacity", current, e)
	}
	var final cooperation.ProcurementExit
	if e = db.First(&final, "id=?", exit.ID).Error; e != nil || final.State != "settled" || final.RefundedUSD.String() != "10" {
		t.Fatal("exit obligation left incorrect", final, e)
	}
	var finalLeg cooperation.ProcurementExitRefund
	if e = db.First(&finalLeg, "id=?", leg.ID).Error; e != nil || finalLeg.FeeStatus != "pending" || finalLeg.FeeUSD != nil {
		t.Fatalf("unknown refund fee became a confirmed zero: %+v %v", finalLeg, e)
	}
	accounting := cooperation.ProcurementRefundAccounting{RefundID: leg.ID, BusinessKey: "provider-refund-fee", FeeUSD: ledger.MustDecimal("0.2"), SettledAt: time.Now().UTC(), Evidence: "verified-provider-refund-fee-statement"}
	if _, e = refunds.ConfirmRefundAccounting(ctx, owner, accounting); e != nil {
		t.Fatal(e)
	}
	if _, e = refunds.ConfirmRefundAccounting(ctx, owner, accounting); e != nil {
		t.Fatal("same fee statement repeated", e)
	}
	accounting.FeeUSD = ledger.MustDecimal("0.3")
	if _, e = refunds.ConfirmRefundAccounting(ctx, owner, accounting); e == nil {
		t.Fatal("changed fee rewrote immutable statement")
	}
	var feeJournals int64
	if e = db.Table("ledger_journals").Where("kind='procurement_refund_fee'").Count(&feeJournals).Error; e != nil || feeJournals != 1 {
		t.Fatalf("fee was posted more than once: %d %v", feeJournals, e)
	}
	if _, e = refunds.Run(ctx, 20); e != nil {
		t.Fatal(e)
	}
	adjustments, e := capacity.InvoiceReceiptAdjustments(ctx, nil, supplier, []string{receiptID})
	if e != nil || len(adjustments) != 1 || adjustments[0].RemovedUSD.String() != "10" {
		t.Fatal("wholesale refund missing from original receipt invoice adjustment", adjustments, e)
	}
	if reviewed, e := invoiceService.ReviewInvoices(ctx, supplier, 50); e != nil || reviewed != 1 {
		t.Fatalf("refunded procurement invoice not flagged: %d %v", reviewed, e)
	}
}
