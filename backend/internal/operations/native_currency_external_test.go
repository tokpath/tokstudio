package operations_test

import (
	"context"
	"encoding/csv"
	"errors"
	"io/fs"
	"math/big"
	"strings"
	"sync"
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
	"github.com/tokpath/tokstudio/backend/internal/paymentmethods"
	"github.com/tokpath/tokstudio/backend/internal/payments"
	"github.com/tokpath/tokstudio/backend/internal/platform/db/dbtest"
	"github.com/tokpath/tokstudio/backend/internal/remediation"
	"github.com/tokpath/tokstudio/backend/internal/usage"
	"gorm.io/gorm"
)

type nativeExportStore struct {
	sync.Map
}

func (s *nativeExportStore) Put(_ context.Context, key string, data []byte, _ string, _ time.Time) error {
	s.Store(key, data)
	return nil
}
func (s *nativeExportStore) Get(_ context.Context, key string) ([]byte, error) {
	data, ok := s.Load(key)
	if !ok {
		return nil, errors.New("missing test export")
	}
	return data.([]byte), nil
}
func (s *nativeExportStore) Delete(_ context.Context, key string) error {
	s.Map.Delete(key)
	return nil
}

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
	apply(paymentmethods.Migrations())
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
	f.reports = &operations.Service{DB: f.db, EncryptionKey: "native-financial-export-test", Authorize: f.org, ExportStore: &nativeExportStore{}, Source: operations.DomainSource{Ledger: l, Cooperation: f.coop, Commerce: f.commerce, Usage: u, Payments: f.pay, Refunds: operations.RefundFinancialSourceFunc(func(ctx context.Context, tx *gorm.DB, op string, from, to time.Time) (operations.RefundFinancialFacts, error) {
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
	if !total.Income.IsZero() || !total.Expense.IsZero() || total.PendingReceiptFees != 1 || pending.SettlementCurrency != "CNY" || len(pending.OriginalCurrencyTotals) != 1 {
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
	if total.Income.String() != "12" || !total.Expense.IsZero() || total.ReceiptFees.String() != "0.5" || total.NetReceipts.String() != "12" || total.PendingReceiptFees != 0 || !report.Costs.PaymentFeesMoney.IsZero() || len(report.OriginalCurrencyTotals) != 1 {
		t.Fatalf("receipt was counted twice, original fee converted, or settlement fee invented: %+v", report)
	}
	financialCSV := f.export(t, "financial")
	found := map[string]string{}
	for _, row := range financialCSV[1:] {
		found[row[1]+":"+row[3]] = row[2]
	}
	if found["已核实原币流入:EUR"] != "12" || found["已知原币收款手续费:EUR"] != "0.5" || found["已知原币净到账:EUR"] != "12" {
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
	if cells["实际币种"] != "EUR" || cells["原币收款总额（扣手续费前）"] != "12.5" || cells["原币己方手续费"] != "0.500000000000000000" || cells["实际到账金额"] != "12.000000000000000000" || cells["实际到账币种"] != "EUR" || cells["清算币种"] != "CNY" {
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

func TestFinancialVerifiedSameCurrencyNetSurvivesReceiptPosting(t *testing.T) {
	f := newNativeFinancialFixture(t, "USD")
	m := payments.Merchant{ID: uuid.NewString(), OperatorID: f.operator, Driver: "manual", Enabled: true, Credentials: map[string]string{"currency": "USD", "recipient_reference": "bank:usd", "instructions": "Synthetic transfer"}}
	if _, err := f.pay.SaveMerchant(f.ctx, f.owner, m, 0); err != nil {
		t.Fatal(err)
	}
	factID, paid := uuid.NewString(), f.from.Add(time.Minute)
	if err := f.db.Exec(`INSERT INTO payments_verified_facts(id,merchant_id,operator_id,order_id,kind,fingerprint,fact,valuation,state,observed_at)
 VALUES(?,?,?,?,'receipt','verified fixture',jsonb_build_object('occurred_at',?::timestamptz,'merchant_net','{"currency":"USD","amount":"97"}'::jsonb),
 'null'::jsonb,'awaiting_settlement',now())`, factID, m.ID, f.operator, uuid.NewString(), paid).Error; err != nil {
		t.Fatal(err)
	}
	check := func() {
		t.Helper()
		report := f.report(t, f.operator)
		if report.Cash.RecordedInMoney.String() != "97" || report.Cash.ClosingRecordedNetMoney.String() != "97" || nativeTotal(t, report, "USD").Income.String() != "97" {
			t.Fatalf("actual settlement net missing or duplicate: %+v", report.Cash)
		}
	}
	check()
	gross, net, fee := ledger.FromInt64(100), ledger.FromInt64(97), ledger.FromInt64(3)
	r, err := f.commerce.RecordReceipt(f.ctx, nil, commerce.VerifiedReceipt{OperatorID: f.operator, Provider: "stripe", ProviderReceiptID: "confirmed-usd-receipt",
		GrossMoney: gross, NetMoney: &net, FeeMoney: &fee, FeeStatus: "confirmed", Currency: "USD", OriginalAmount: "100",
		SettlementCurrency: "USD", SettlementNetAmount: &net, ValuationEvidence: "verified USD", PaidAt: &paid,
		RecipientReference: "synthetic merchant", PayerReference: "synthetic payer", VerificationEvidence: "synthetic provider statement", VerifiedBy: "system:payment-verification"})
	if err != nil {
		t.Fatal(err)
	}
	if err = f.db.Table("payments_verified_facts").Where("id=?", factID).Update("receipt_id", r.ID).Error; err != nil {
		t.Fatal(err)
	}
	check()
}

type nativeFinancialRates struct{}

func (nativeFinancialRates) CoverageRates(context.Context, *gorm.DB, string, time.Time) (cooperation.CoverageRates, error) {
	return cooperation.CoverageRates{Version: 1, Direct: big.NewRat(1, 1), Children: map[string]*big.Rat{}}, nil
}
