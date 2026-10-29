package operations_test

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"io/fs"
	"math/big"
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
	"github.com/tokpath/tokstudio/backend/internal/operations"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"github.com/tokpath/tokstudio/backend/internal/payments"
	"github.com/tokpath/tokstudio/backend/internal/platform/db/dbtest"
	"github.com/tokpath/tokstudio/backend/internal/remediation"
	"github.com/tokpath/tokstudio/backend/internal/usage"
	"gorm.io/gorm"
)

type nativeFinancialFixture struct {
	db       *gorm.DB
	ctx      context.Context
	org      *organization.Service
	commerce *commerce.Service
	coop     *cooperation.Service
	pay      *payments.Service
	rem      *remediation.Service
	reports  *operations.Service
	owner    string
	operator string
	from, to time.Time
}

func newNativeFinancialFixture(t *testing.T, currency string) nativeFinancialFixture {
	t.Helper()
	f := nativeFinancialFixture{db: dbtest.New(t), ctx: context.Background(), owner: uuid.NewString()}
	apply := func(_ string, files fs.FS) {
		t.Helper()
		entries, err := fs.ReadDir(files, ".")
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			raw, err := fs.ReadFile(files, entry.Name())
			if err != nil {
				t.Fatal(err)
			}
			if err = f.db.Exec(string(raw)).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	apply(organization.Migrations())
	apply(ledger.Migrations())
	apply(commerce.Migrations())
	apply(incentives.Migrations())
	apply(cooperation.Migrations())
	apply(usage.Migrations())
	apply(modelpricing.Migrations())
	apply(operations.Migrations())
	apply(payments.Migrations())
	apply(remediation.Migrations())
	if err := f.db.Exec("UPDATE ledger_monetary_settings SET currency=?,effective_at=clock_timestamp()-interval '1 second'", currency).Error; err != nil {
		t.Fatal(err)
	}
	f.org = organization.New(f.db)
	platform, err := f.org.BootstrapPlatform(f.ctx, nil, organization.BootstrapInput{BrandName: "Original currency report", OperatorName: "Supplier", OwnerStaffID: f.owner})
	if err != nil {
		t.Fatal(err)
	}
	f.operator = platform.Operator.ID
	l := ledger.New(f.db)
	f.commerce = commerce.New(f.db)
	f.coop = cooperation.New(f.db, l, f.org)
	f.pay = payments.NewService(f.db, "native-financial-test", f.org, nil, payments.CommerceSink{Commerce: f.commerce})
	f.rem = remediation.New(f.db, remediation.Dependencies{Ledger: l, Organization: f.org, Commerce: f.commerce, Cooperation: f.coop, Payments: f.pay, Incentives: incentives.New(f.db)})
	u := usage.New(f.db, usage.Dependencies{Ledger: l, Organization: f.org, Cooperation: f.coop, Catalog: catalog.CommerceCatalog{DB: f.db, PlatformOperatorID: f.operator}})
	f.reports = &operations.Service{DB: f.db, EncryptionKey: "native-financial-export-test", Authorize: f.org, Source: operations.DomainSource{Ledger: l, Cooperation: f.coop, Commerce: f.commerce, Usage: u, Payments: f.pay, Refunds: operations.RefundFinancialSourceFunc(func(ctx context.Context, tx *gorm.DB, op string, from, to time.Time) (operations.RefundFinancialFacts, error) {
		r, err := f.rem.FinancialRefundFacts(ctx, tx, op, from, to)
		return operations.RefundFinancialFacts{PendingFees: r.PendingFees, PendingExecutions: r.PendingExecutions, OutstandingMoney: r.OutstandingMoney, MismatchCount: r.MismatchCount}, err
	})}}
	f.from, f.to = time.Now().UTC().Add(-time.Hour), time.Now().UTC().Add(time.Hour)
	return f
}

func (f nativeFinancialFixture) report(t *testing.T, operator string) operations.FinancialReport {
	t.Helper()
	r, err := f.reports.FinancialReport(f.ctx, f.owner, operator, f.from, f.to)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func nativeTotal(t *testing.T, r operations.FinancialReport, currency string) operations.OriginalCurrencyTotal {
	t.Helper()
	for _, total := range r.OriginalCurrencyTotals {
		if total.Currency == currency {
			return total
		}
	}
	t.Fatalf("currency %s missing: %+v", currency, r.OriginalCurrencyTotals)
	return operations.OriginalCurrencyTotal{}
}

func (f nativeFinancialFixture) export(t *testing.T, kind string) [][]string {
	t.Helper()
	job, err := f.reports.RequestExport(f.ctx, f.owner, f.operator, operations.ExportQuery{Kind: kind, From: f.from, To: f.to})
	if err != nil {
		t.Fatal(err)
	}
	jobs, err := f.reports.RunExports(f.ctx, 1)
	if err != nil || len(jobs) != 1 || jobs[0].State != "ready" {
		t.Fatalf("export did not finish: %+v %v", jobs, err)
	}
	body, err := f.reports.DownloadExport(f.ctx, f.owner, f.operator, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := csv.NewReader(strings.NewReader(string(body))).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func TestFinancialOriginalReceiptsAndFeesKeepTheirCurrencyAcrossSettlementAndExports(t *testing.T) {
	f := newNativeFinancialFixture(t, "CNY")
	// Configure a different settlement currency in this isolated fixture before
	// any money exists. A EUR payment must never acquire the CNY ledger amount.
	m := payments.Merchant{ID: uuid.NewString(), OperatorID: f.operator, Driver: "manual", Enabled: true, Credentials: map[string]string{"currency": "EUR", "recipient_reference": "bank:eur", "instructions": "Transfer the agreed original amount"}}
	if _, err := f.pay.SaveMerchant(f.ctx, f.owner, m, 0); err != nil {
		t.Fatal(err)
	}
	at := time.Now().UTC().Truncate(time.Microsecond)
	o := payments.Order{ID: uuid.NewString(), OperatorID: f.operator, MerchantID: m.ID, Money: payments.Money{Currency: "EUR", Amount: ledger.MustDecimal("12.5")}, SettlementMoney: payments.Money{Currency: "CNY", Amount: ledger.FromInt64(100)}, AcceptedAt: at.Add(-time.Minute), ExpiresAt: at.Add(time.Hour)}
	if _, err := f.pay.Create(f.ctx, f.operator, m.ID, o); err != nil {
		t.Fatal(err)
	}
	fact, err := f.pay.RecordManual(f.ctx, f.owner, f.operator, m.ID, o.ID, payments.ManualVerification{ExternalReference: "eur-transfer", Money: o.Money, PaidAt: at, EvidenceReference: "verified bank statement", PayerReference: "bank:customer"})
	if !errors.Is(err, payments.ErrAwaitingSettlement) {
		t.Fatalf("foreign receipt should await explicit settlement confirmation: %v", err)
	}
	pending := f.report(t, f.operator)
	total := nativeTotal(t, pending, "EUR")
	if total.Income.String() != "12.5" || !total.Expense.IsZero() || total.PendingReceiptFees != 1 || pending.SettlementCurrency != "CNY" || len(pending.OriginalCurrencyTotals) != 1 {
		t.Fatalf("pending original transfer lost or relabeled: %+v", pending)
	}
	confirmed, err := f.pay.ConfirmReceiptValuation(f.ctx, f.owner, f.operator, fact.ID, payments.ReceiptValuationConfirmation{BusinessKey: "eur-explicit-settlement", SettlementAmount: ledger.FromInt64(100), EvidenceReference: "agreed settlement amount"})
	if err != nil || confirmed.ReceiptID == "" {
		t.Fatal(confirmed, err)
	}
	fee := ledger.MustDecimal("0.5")
	if _, err = f.commerce.RecordReceiptAccounting(f.ctx, nil, commerce.ReceiptAccounting{ReceiptID: confirmed.ReceiptID, OriginalFeeAmount: &fee, Evidence: "EUR bank fee statement", VerifiedBy: "staff:" + f.owner}); err != nil {
		t.Fatal(err)
	}
	report := f.report(t, f.operator)
	total = nativeTotal(t, report, "EUR")
	if total.Income.String() != "12.5" || total.Expense.String() != "0.5" || total.ReceiptFees.String() != "0.5" || total.NetReceipts.String() != "12" || total.PendingReceiptFees != 0 || !report.Costs.PaymentFeesMoney.IsZero() || len(report.OriginalCurrencyTotals) != 1 {
		t.Fatalf("receipt was counted twice, original fee converted, or settlement fee invented: %+v", report)
	}
	financialCSV := f.export(t, "financial")
	found := map[string]string{}
	for _, row := range financialCSV[1:] {
		found[row[1]+":"+row[3]] = row[2]
	}
	if found["已核实原币流入:EUR"] != "12.5" || found["已知原币收款手续费:EUR"] != "0.5" || found["已知原币净到账:EUR"] != "12" {
		t.Fatal("financial CSV lost original money", financialCSV)
	}
	paymentCSV := f.export(t, "payments")
	if len(paymentCSV) != 2 {
		t.Fatal("payment export duplicated raw and applied receipt", paymentCSV)
	}
	cells := map[string]string{}
	for i, key := range paymentCSV[0] {
		cells[key] = paymentCSV[1][i]
	}
	if cells["实际币种"] != "EUR" || cells["原币收款总额（扣手续费前）"] != "12.5" || cells["原币己方手续费"] != "0.500000000000000000" || cells["原币净到账"] != "12.000000000000000000" || cells["己方收款手续费 清算币种"] != "" || cells["清算币种"] != "CNY" {
		t.Fatal("payment CSV mixed original and settlement amounts", cells)
	}
}

func TestFinancialSupplierStatementsUseRegistrationTimeAndRemainIndependentOfCash(t *testing.T) {
	f := newNativeFinancialFixture(t, "USD")
	if err := f.db.Exec(catalog.CommerceSchema()).Error; err != nil {
		t.Fatal(err)
	}
	cat := catalog.CommerceCatalog{DB: f.db, PlatformOperatorID: f.operator, EncryptionKey: "native-supplier-test"}
	if _, err := cat.SaveCredential(f.ctx, f.operator, "supplier-key", "supplier", "Statement supplier", "test-secret", true, 0); err != nil {
		t.Fatal(err)
	}
	pricing := modelpricing.Service{DB: f.db}
	for _, statement := range []modelpricing.ActualCost{
		{ID: "cny-statement", ProviderID: "supplier", Currency: "CNY", Amount: ledger.FromInt64(72)},
		{ID: "usdt-statement", ProviderID: "supplier", Currency: "USDT", Amount: ledger.FromInt64(10)},
	} {
		// An old service period and an untrusted client timestamp must not hide
		// a newly registered statement from the current report.
		statement.StartsAt, statement.EndsAt = f.from.Add(-30*24*time.Hour), f.from.Add(-24*time.Hour)
		statement.CreatedAt = statement.StartsAt
		if err := pricing.SaveActual(f.ctx, statement, 0); err != nil {
			t.Fatal(err)
		}
	}
	report := f.report(t, f.operator)
	cny, usdt := nativeTotal(t, report, "CNY"), nativeTotal(t, report, "USDT")
	if len(report.OriginalCurrencyTotals) != 2 || cny.SupplierActualCost.String() != "72" || usdt.SupplierActualCost.String() != "10" || !cny.Income.IsZero() || !cny.Expense.IsZero() || !usdt.Expense.IsZero() || !report.Costs.TotalMoney.IsZero() || !report.Cash.RecordedOutMoney.IsZero() {
		t.Fatalf("registered statements were converted, posted as cash, or counted in settlement cost: %+v", report)
	}
	var supplierPostings int64
	if err := f.db.Table("ledger_journals").Where("kind='supplier_cost'").Count(&supplierPostings).Error; err != nil || supplierPostings != 0 {
		t.Fatal("statement registration posted automatic supplier expense", supplierPostings, err)
	}
}

func TestFinancialOriginalReceiptKeepsObservationPeriodWhenRecordedLater(t *testing.T) {
	f := newNativeFinancialFixture(t, "CNY")
	f.from, f.to = time.Now().UTC().Add(-2*time.Hour), time.Now().UTC().Add(-time.Hour)
	observed, zero, gross := f.from.Add(30*time.Minute), ledger.Zero(), ledger.FromInt64(100)
	if _, err := f.commerce.RecordManualReceipt(f.ctx, nil, f.owner, commerce.VerifiedReceipt{OperatorID: f.operator, ProviderReceiptID: "delayed-offline-receipt", GrossMoney: gross, NetMoney: &gross, FeeMoney: &zero, OriginalFeeAmount: &zero, Currency: "EUR", OriginalAmount: "12.5", ValuationEvidence: "agreed settlement amount", ObservedAt: observed}); err != nil {
		t.Fatal(err)
	}
	total := nativeTotal(t, f.report(t, f.operator), "EUR")
	if total.Income.String() != "12.5" || total.NetReceipts.String() != "12.5" || !total.Expense.IsZero() {
		t.Fatalf("later data entry moved an original transfer into a different period: %+v", total)
	}
	if rows := f.export(t, "payments"); len(rows) != 2 {
		t.Fatal("CSV and financial report disagree on the original receipt period", rows)
	}
}

type nativeFinancialRates struct{}

func (nativeFinancialRates) CoverageRates(context.Context, *gorm.DB, string, time.Time) (cooperation.CoverageRates, error) {
	return cooperation.CoverageRates{Version: 1, Direct: big.NewRat(1, 1), Children: map[string]*big.Rat{}}, nil
}

func TestFinancialProcurementRefundCountsActualCurrencyBeforeAccountingRecoveryOnce(t *testing.T) {
	for _, source := range []string{"manual_receipt", "payment"} {
		t.Run(source, func(t *testing.T) { testFinancialProcurementRefundRecovery(t, source) })
	}
}

func testFinancialProcurementRefundRecovery(t *testing.T, source string) {
	f := newNativeFinancialFixture(t, "USD")
	f.coop.Rates = nativeFinancialRates{}
	child, err := f.org.CreateOperator(f.ctx, nil, organization.CreateOperatorInput{ActorStaffID: f.owner, ParentOperatorID: f.operator, Kind: organization.Channel, Name: "Refund buyer"})
	if err != nil {
		t.Fatal(err)
	}
	buyer := child.Operator.ID
	invite, err := f.org.InviteStaff(f.ctx, nil, f.owner, buyer, "native-buyer@example.test", "owner")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.org.AcceptStaffInvitation(f.ctx, nil, invite.Token, organization.VerifiedStaff{ID: f.owner, Email: "native-buyer@example.test", EmailVerified: true}); err != nil {
		t.Fatal(err)
	}
	quote, err := f.coop.PublishPurchaseQuote(f.ctx, nil, f.owner, cooperation.PurchaseQuote{SupplierID: f.operator, BuyerID: buyer, GiftTiers: []cooperation.PurchaseGiftTier{{FromMoney: ledger.Zero(), GiftPercent: ledger.Zero()}}, ExitTerms: json.RawMessage(`{"version":1,"voluntary_refund":true,"method":"remaining_paid_proportional","terms_reference":"refund-v1"}`), TermsReference: "purchase-v1", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	order, err := f.coop.CreatePurchaseOrder(f.ctx, nil, f.owner, buyer, quote.ID, "purchase", ledger.FromInt64(10))
	if err != nil {
		t.Fatal(err)
	}
	at, zero, gross := time.Now().UTC().Truncate(time.Microsecond), ledger.Zero(), ledger.FromInt64(10)
	var receipt commerce.Receipt
	if source == "manual_receipt" {
		receipt, err = f.commerce.RecordManualReceipt(f.ctx, nil, f.owner, commerce.VerifiedReceipt{OperatorID: f.operator, ProviderReceiptID: "bank-procurement", GrossMoney: gross, NetMoney: &gross, FeeMoney: &zero, OriginalFeeAmount: &zero, Currency: "USD", OriginalAmount: "10", RecipientReference: "bank:supplier", PayerReference: "bank:buyer", ValuationEvidence: "native settlement", VerificationEvidence: "verified bank statement", PaidAt: &at, ObservedAt: at, SettledAt: &at})
	} else {
		m := payments.Merchant{ID: uuid.NewString(), OperatorID: f.operator, Driver: "manual", Enabled: true, Credentials: map[string]string{"currency": "USD", "recipient_reference": "bank:supplier", "instructions": "Bank transfer"}}
		if _, err = f.pay.SaveMerchant(f.ctx, f.owner, m, 0); err != nil {
			t.Fatal(err)
		}
		o := payments.Order{ID: order.ID, OperatorID: f.operator, MerchantID: m.ID, Money: payments.Money{Currency: "USD", Amount: gross}, AcceptedAt: at.Add(-time.Minute), ExpiresAt: order.ExpiresAt}
		if _, err = f.pay.Create(f.ctx, f.operator, m.ID, o); err != nil {
			t.Fatal(err)
		}
		if _, err = f.pay.RecordManual(f.ctx, f.owner, f.operator, m.ID, o.ID, payments.ManualVerification{ExternalReference: "bank-procurement", Money: o.Money, PaidAt: at, EvidenceReference: "verified bank statement", PayerReference: "bank:buyer"}); err != nil {
			t.Fatal(err)
		}
		links, e := f.pay.ReceiptsForOrder(f.ctx, f.operator, m.ID, order.ID)
		if e != nil || len(links) != 1 {
			t.Fatal("payment receipt missing", links, e)
		}
		receipt, err = f.commerce.GetReceipt(f.ctx, nil, f.operator, links[0].ReceiptID)
	}
	if err != nil {
		t.Fatal(err)
	}
	if err = f.db.Transaction(func(tx *gorm.DB) error {
		if _, err := f.coop.ApplyVerifiedPurchaseReceipt(f.ctx, tx, f.commerce, order.ID, receipt.ID, "purchase-payment", gross); err != nil {
			return err
		}
		_, err := f.coop.FulfillPurchase(f.ctx, tx, order.ID, f.owner)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	before := nativeTotal(t, f.report(t, buyer), "USD")
	if before.Expense.String() != "10" || !before.Income.IsZero() {
		t.Fatalf("buyer original purchase payment missing: %+v", before)
	}
	exit, err := f.coop.RequestProcurementExit(f.ctx, nil, f.owner, buyer, "exit", "voluntary_exit", "verified request", gross, cooperation.CoverageRemovalGuard{Cooperation: f.coop, PaymentFacts: f.pay})
	if err != nil {
		t.Fatal(err)
	}
	refunds := &cooperation.ProcurementRefunds{Cooperation: f.coop, Payments: f.pay, Collector: f.commerce, Capacity: f.rem}
	observe := func(ctx context.Context, tx *gorm.DB, op, receipt, reference, fact string, amount ledger.Decimal) error {
		_, err := f.rem.ObserveReceiptRefund(ctx, tx, op, receipt, reference, fact, amount)
		return err
	}
	refunds.ObserveReceiptRefund = observe
	leg, err := refunds.PrepareRefund(f.ctx, f.owner, exit.ID, receipt.ID, "refund", gross)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = refunds.BeginManualRefund(f.ctx, f.owner, leg.ID, "bank-instruction", "verified original route"); err != nil {
		t.Fatal(err)
	}
	refunds.ObserveReceiptRefund = func(context.Context, *gorm.DB, string, string, string, string, ledger.Decimal) error {
		return errors.New("temporary accounting outage after bank transfer verification")
	}
	proof := payments.ManualRefundVerification{ReceiptID: receipt.ID, ExternalReference: "actual-bank-refund", Money: payments.Money{Currency: "USD", Amount: gross}, ExecutedAt: at, EvidenceReference: "verified outbound bank statement", RecipientReference: "bank:buyer"}
	status, err := refunds.RecordManualRefund(f.ctx, f.owner, leg.ID, proof, payments.Valuation{}, time.Time{}, "")
	if err == nil || status.State != "verified_pending_settlement" {
		t.Fatalf("did not retain actual bank fact: %+v %v", status, err)
	}
	pending := nativeTotal(t, f.report(t, f.operator), "USD")
	if pending.Income.String() != "10" || pending.Expense.String() != "10" {
		t.Fatalf("verified bank refund disappeared during internal recovery: %+v", pending)
	}
	refunds.ObserveReceiptRefund = observe
	if status, err = refunds.RecordManualRefund(f.ctx, f.owner, leg.ID, proof, payments.Valuation{}, time.Time{}, ""); err != nil || status.State != "succeeded" {
		t.Fatal(status, err)
	}
	if _, err = refunds.RecordManualRefund(f.ctx, f.owner, leg.ID, proof, payments.Valuation{}, time.Time{}, ""); err != nil {
		t.Fatal("refund replay failed", err)
	}
	supplierTotal := nativeTotal(t, f.report(t, f.operator), "USD")
	buyerTotal := nativeTotal(t, f.report(t, buyer), "USD")
	if supplierTotal.Income.String() != "10" || supplierTotal.Expense.String() != "10" || buyerTotal.Income.String() != "10" || buyerTotal.Expense.String() != "10" {
		t.Fatalf("refund recovery doubled transfers or buyer receipt missing: supplier=%+v buyer=%+v", supplierTotal, buyerTotal)
	}
}

func TestFinancialTakeoverPaymentsKeepOriginalCurrencyForBothParties(t *testing.T) {
	f := newNativeFinancialFixture(t, "CNY")
	channel, e := f.org.CreateOperator(f.ctx, nil, organization.CreateOperatorInput{ActorStaffID: f.owner, ParentOperatorID: f.operator, Kind: organization.Channel, Name: "Takeover original-currency channel"})
	if e != nil {
		t.Fatal(e)
	}
	id := uuid.NewString()
	e = f.db.Exec(`INSERT INTO remediation_takeovers(id,channel_id,brand_operator_id,business_key,fingerprint,state,method,service_money_per_atg,procurement_money_per_atg,extra_money,contract_reference,evidence,requested_by,previous_status,status_version,settlement_due_money,created_at) VALUES(?,?,?,'actual-original-settlement','fixture','completed','agreed_atg_value',1,1,0,'agreed terms','actual cash settlement',?,'{}',1,70,clock_timestamp())`, id, channel.Operator.ID, f.operator, f.owner).Error
	if e != nil {
		t.Fatal(e)
	}
	e = f.db.Exec(`INSERT INTO remediation_takeover_settlements(id,takeover_id,business_key,fingerprint,amount_money,fee_money,currency,original_amount,reference,evidence,occurred_at,created_at) VALUES(?,?,'actual-eur-receipt','fixture',70,0,'EUR','10','actual bank receipt','both parties verified original transfer',clock_timestamp(),clock_timestamp())`, uuid.NewString(), id).Error
	if e != nil {
		t.Fatal(e)
	}
	report, e := f.reports.FinancialReport(f.ctx, f.owner, f.operator, f.from, f.to)
	if e != nil {
		t.Fatal(e)
	}
	found := false
	for _, row := range report.OriginalCurrencyTotals {
		if row.Currency == "EUR" {
			found = true
			if row.Income.String() != "10" || !row.Expense.IsZero() {
				t.Fatal(row)
			}
		}
	}
	if !found {
		t.Fatal("original takeover transfer missing", report.OriginalCurrencyTotals)
	}
}
