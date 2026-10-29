package operations_test

import (
	"context"
	"errors"
	"io/fs"
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

func TestFinancialReportRecordedFactsScopeAndTakeover(t *testing.T) {
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
	apply(usage.Migrations())
	apply(modelpricing.Migrations())
	apply(operations.Migrations())
	apply(payments.Migrations())
	apply(remediation.Migrations())
	ctx := context.Background()
	org := organization.New(db)
	owner := uuid.NewString()
	platform, e := org.BootstrapPlatform(ctx, nil, organization.BootstrapInput{BrandName: "Report Test", OperatorName: "Report Operator", OwnerStaffID: owner})
	if e != nil {
		t.Fatal(e)
	}
	op := platform.Operator.ID
	l := ledger.New(db)
	coop := cooperation.New(db, l, org)
	com := commerce.New(db)
	u := usage.New(db, usage.Dependencies{Ledger: l, Organization: org, Cooperation: coop, Catalog: catalog.CommerceCatalog{DB: db, PlatformOperatorID: op}})
	rem := remediation.New(db, remediation.Dependencies{Ledger: l, Organization: org, Cooperation: coop, Commerce: com, Incentives: incentives.New(db)})
	s := &operations.Service{DB: db, Authorize: org}
	s.Source = operations.DomainSource{Ledger: l, Cooperation: coop, Commerce: com, Usage: u, Payments: payments.NewService(db, "financial-read", org, nil, nil), Refunds: operations.RefundFinancialSourceFunc(func(ctx context.Context, tx *gorm.DB, op string, from, to time.Time) (operations.RefundFinancialFacts, error) {
		r, e := rem.FinancialRefundFacts(ctx, tx, op, from, to)
		return operations.RefundFinancialFacts{PendingFees: r.PendingFees, PendingExecutions: r.PendingExecutions, OutstandingMoney: r.OutstandingMoney, MismatchCount: r.MismatchCount}, e
	})}
	account := func(purpose string, negative bool) ledger.Account {
		t.Helper()
		a, e := l.EnsureAccount(ctx, nil, ledger.Account{OperatorID: op, OwnerID: op, Purpose: purpose, Unit: ledger.MoneyUnit, AllowNegative: negative})
		if e != nil {
			t.Fatal(e)
		}
		return a
	}
	cash := account("control:external_cash", true)
	clearing := account("control:financial_test", true)
	fees := account("control:merchant_fees", true)
	commissions := account("control:commissions", true)
	frozen := account("commission:frozen", false)
	supplier := account("control:supplier:test", true)
	cost := account("cost:supplier:test", false)
	post := func(kind string, lines ...ledger.Line) {
		t.Helper()
		_, e := l.Post(ctx, nil, ledger.Posting{OperatorID: op, BusinessKey: uuid.NewString(), Kind: kind, SourceID: uuid.NewString(), ActorID: owner, Reason: "synthetic verified accounting fact", Lines: lines})
		if e != nil {
			t.Fatal(e)
		}
	}
	post("provider_settlement", ledger.Line{AccountID: cash.ID, Amount: ledger.FromInt64(-100)}, ledger.Line{AccountID: clearing.ID, Amount: ledger.FromInt64(100)})
	post("merchant_fee", ledger.Line{AccountID: cash.ID, Amount: ledger.FromInt64(2)}, ledger.Line{AccountID: fees.ID, Amount: ledger.FromInt64(-2)})
	post("commission_accrual", ledger.Line{AccountID: frozen.ID, Amount: ledger.FromInt64(3)}, ledger.Line{AccountID: commissions.ID, Amount: ledger.FromInt64(-3)})
	post("takeover_cash_settlement", ledger.Line{AccountID: cash.ID, Amount: ledger.FromInt64(7)}, ledger.Line{AccountID: fees.ID, Amount: ledger.FromInt64(-7)})
	post("supplier_cost", ledger.Line{AccountID: supplier.ID, Amount: ledger.FromInt64(-4)}, ledger.Line{AccountID: cost.ID, Amount: ledger.FromInt64(4)})
	if _, e = l.IssueGrant(ctx, nil, ledger.GrantInput{OperatorID: op, CustomerID: uuid.NewString(), BusinessKey: "outstanding", ActorID: owner, Reason: "verified test package", PaidMoney: ledger.FromInt64(10), Sources: []ledger.GrantSource{{Kind: "paid", Amount: ledger.FromInt64(80)}, {Kind: "package_gift", Amount: ledger.FromInt64(20)}}, EffectiveAt: time.Now().Add(-time.Minute)}); e != nil {
		t.Fatal(e)
	}
	var now time.Time
	if e = db.Raw("SELECT clock_timestamp()").Row().Scan(&now); e != nil {
		t.Fatal(e)
	}
	r, e := s.FinancialReport(ctx, owner, op, now.Add(-time.Hour), now.Add(time.Second))
	if e != nil {
		t.Fatal(e)
	}
	if r.Revenue.TotalMoney.String() != "0" || r.Costs.TotalMoney.String() != "5" {
		t.Fatalf("cash/contract settlement inflated operating revenue, or takeover fees became operating costs: %+v", r)
	}
	if r.Cash.RecordedInMoney.String() != "100" || r.Cash.RecordedOutMoney.String() != "9" || r.Cash.TakeoverOutMoney.String() != "7" || len(r.TakeoverFlows) != 2 {
		t.Fatalf("cash and takeover separation: %+v", r.Cash)
	}
	if r.Liabilities.SupplierPayableMoney.String() != "4" || r.Liabilities.OutstandingGrantPaidMoney.String() != "10" || r.Liabilities.OutstandingGrantATG.String() != "100" {
		t.Fatalf("recorded unpaid obligations missing: %+v", r.Liabilities)
	}
	if _, e = s.FinancialReport(ctx, uuid.NewString(), op, now.Add(-time.Hour), now); e == nil {
		t.Fatal("outsider obtained financial report")
	}
	if _, e = s.FinancialReport(ctx, owner, op, now, now); !errors.Is(e, operations.ErrInvalid) {
		t.Fatal("invalid report window accepted", e)
	}
}
