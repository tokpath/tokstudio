package operations

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/cooperation"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/platform/mail"
	"github.com/tokpath/tokstudio/backend/internal/usage"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func operationsDB(t *testing.T) *gorm.DB {
	return operationsDBBefore(t, "")
}

func operationsDBBefore(t *testing.T, before string) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("TOKENHUB_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TOKENHUB_TEST_DATABASE_URL required for operations transaction tests")
	}
	admin, e := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if e != nil {
		t.Fatal(e)
	}
	schema := "operations_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if e = admin.Exec("CREATE SCHEMA " + schema).Error; e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		_ = admin.Exec("DROP SCHEMA " + schema + " CASCADE").Error
		sqlDB, _ := admin.DB()
		_ = sqlDB.Close()
	})
	u, _ := url.Parse(dsn)
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	db, e := gorm.Open(postgres.Open(u.String()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { sqlDB, _ := db.DB(); _ = sqlDB.Close() })
	files, _ := fs.ReadDir(migrations, "migrate")
	for _, file := range files {
		if before != "" && file.Name() >= before {
			continue
		}
		data, e := migrations.ReadFile("migrate/" + file.Name())
		if e != nil {
			t.Fatal(e)
		}
		if e = db.Exec(string(data)).Error; e != nil {
			t.Fatal(e)
		}
	}
	return db
}

type operationAuth struct {
	deny  atomic.Bool
	actor string
}

func (a *operationAuth) RequirePermission(_ context.Context, _ *gorm.DB, actor, operator, permission string) error {
	if a.deny.Load() || actor != a.actor || operator == "" {
		return ErrForbidden
	}
	return nil
}

type operationRecipients struct{ operator, customer string }

func (d operationRecipients) ResolveRecipient(_ context.Context, _ *gorm.DB, operator, recipient string) (Recipient, error) {
	if operator != d.operator || recipient != d.customer {
		return Recipient{}, ErrForbidden
	}
	return Recipient{ID: recipient, OperatorID: operator, Email: "customer@example.test", Brand: "Synthetic Brand", EmailVerified: true}, nil
}

type operationSender struct {
	fail  atomic.Bool
	calls atomic.Int32
}

func (m *operationSender) Send(_ context.Context, msg mail.Message) error {
	m.calls.Add(1)
	if m.fail.Load() {
		return errors.New("synthetic SMTP unavailable")
	}
	if msg.To != "customer@example.test" {
		return ErrInvalid
	}
	return nil
}
func opsFixture(t *testing.T) (*Service, *operationAuth, *operationSender, string, string) {
	db := operationsDB(t)
	operator, customer := uuid.NewString(), uuid.NewString()
	a := &operationAuth{actor: uuid.NewString()}
	sender := &operationSender{}
	return &Service{DB: db, EncryptionKey: "synthetic-operations-key", Authorize: a, Recipients: operationRecipients{operator, customer}, Mail: sender, InvoiceStore: &operationBlobs{data: map[string][]byte{}}}, a, sender, operator, customer
}
func TestNotificationQueueRecoveryConsentAndScope(t *testing.T) {
	s, _, sender, operator, customer := opsFixture(t)
	ctx := context.Background()
	n := Notification{OperatorID: operator, RecipientID: customer, BusinessKey: "receipt:stable-id", Category: "mandatory", Severity: "info", Title: "Receipt verified", Body: "Review your order", Reference: "order:one"}
	notice, e := s.Emit(ctx, nil, n)
	if e != nil {
		t.Fatal(e)
	}
	replay, e := s.Emit(ctx, nil, n)
	if e != nil || notice.ID != replay.ID {
		t.Fatal("notification identity changed")
	}
	n.Body = "changed"
	if _, e = s.Emit(ctx, nil, n); !errors.Is(e, ErrConflict) {
		t.Fatal("notice facts overwritten")
	}
	if _, e = s.ListNotifications(ctx, uuid.NewString(), operator, nil, 20); !errors.Is(e, ErrForbidden) {
		t.Fatal("cross-customer notification read")
	}
	sender.fail.Store(true)
	jobs, e := s.RunNotifications(ctx, 20)
	if e != nil || len(jobs) != 1 || jobs[0].State != "pending" {
		t.Fatalf("failed send %+v %v", jobs, e)
	}
	jobs, e = s.RunNotifications(ctx, 20)
	if e != nil || len(jobs) != 0 {
		t.Fatal("SMTP backoff ignored")
	}
	sender.fail.Store(false)
	_ = s.DB.Table("operations_deliveries").Where("notification_id=?", notice.ID).Updates(map[string]any{"state": "processing", "lease_until": time.Now().Add(-time.Second)}).Error
	jobs, e = s.RunNotifications(ctx, 20)
	if e != nil || len(jobs) != 1 || jobs[0].State != "accepted" || jobs[0].AcceptedAt == nil {
		t.Fatal("crashed lease did not resume")
	}
	if _, e = s.SetPreferences(ctx, customer, operator, true, true); e != nil {
		t.Fatal(e)
	}
	n.BusinessKey = "marketing:once"
	n.Category = "marketing"
	_, e = s.Emit(ctx, nil, n)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.SetPreferences(ctx, customer, operator, false, true); e != nil {
		t.Fatal(e)
	}
	before := sender.calls.Load()
	jobs, e = s.RunNotifications(ctx, 20)
	if e != nil || len(jobs) != 1 || jobs[0].State != "suppressed" || sender.calls.Load() != before {
		t.Fatal("queued marketing ignored new optout")
	}
	if e = s.ReadNotification(ctx, customer, operator, notice.ID); e != nil {
		t.Fatal(e)
	}
}

type metricSource struct {
	value    ledger.Decimal
	findings []Finding
}

func (m *metricSource) Reconcile(context.Context, *gorm.DB, string) ([]Finding, error) {
	return m.findings, nil
}
func (m *metricSource) Metrics(context.Context, *gorm.DB, string, time.Time) (map[string]ledger.Decimal, error) {
	return map[string]ledger.Decimal{"sales_space_atg": m.value}, nil
}
func (m *metricSource) Export(context.Context, *gorm.DB, string, ExportQuery) ([]json.RawMessage, error) {
	return []json.RawMessage{json.RawMessage(`{"amount":"2.5"}`)}, nil
}
func (m *metricSource) HasAdjustment(context.Context, *gorm.DB, string, string) (bool, error) {
	return true, nil
}
func TestRiskTransitionsReminderEscalationAndRecovery(t *testing.T) {
	s, a, _, operator, customer := opsFixture(t)
	source := &metricSource{value: ledger.FromInt64(9)}
	s.Source = source
	recipients, _ := json.Marshal([]string{customer})
	r := RiskRule{OperatorID: operator, ID: "procurement-space", Metric: "sales_space_atg", Warning: ledger.FromInt64(10), Critical: ledger.FromInt64(2), Direction: "below", Recipients: recipients, EscalationRecipients: recipients, ReminderSeconds: 60, EscalateSeconds: 60, Enabled: true}
	if _, e := s.SaveRiskRule(context.Background(), a.actor, r, 0); e != nil {
		t.Fatal(e)
	}
	states, e := s.RunRisks(context.Background(), operator)
	if e != nil || len(states) != 1 || states[0].Status != "warning" {
		t.Fatalf("warning %+v %v", states, e)
	}
	_, _ = s.RunRisks(context.Background(), operator)
	var count int64
	_ = s.DB.Model(&Notification{}).Count(&count).Error
	if count != 1 {
		t.Fatal("interval did not suppress duplicate risk")
	}
	_ = s.DB.Model(&RiskState{}).Where("operator_id=?", operator).Update("opened_at", time.Now().Add(-2*time.Minute)).Error
	states, e = s.RunRisks(context.Background(), operator)
	if e != nil || states[0].Status != "critical" {
		t.Fatal("unresolved warning not escalated")
	}
	source.value = ledger.FromInt64(20)
	states, e = s.RunRisks(context.Background(), operator)
	if e != nil || states[0].Status != "normal" {
		t.Fatal("risk did not recover")
	}
	_ = s.DB.Model(&Notification{}).Count(&count).Error
	if count != 3 {
		t.Fatalf("risk lifecycle notifications=%d", count)
	}
}

type invoiceBasis struct {
	value  ledger.Decimal
	review atomic.Bool
}

func (b *invoiceBasis) InvoiceBasis(_ context.Context, _ *gorm.DB, _, _, receipt string) (InvoiceBasis, error) {
	return InvoiceBasis{ReceiptID: receipt, EligibleUSD: b.value, ReviewRequired: b.review.Load()}, nil
}
func TestInvoiceCapacityConcurrentAndRefundReview(t *testing.T) {
	s, a, _, operator, customer := opsFixture(t)
	if _, e := s.SaveInvoicePolicy(context.Background(), a.actor, InvoicePolicy{OperatorID: operator, Enabled: true, Stage: "receipt", MaximumRatio: ledger.FromInt64(1), RulesReference: "operator-approved-invoice-rules"}, 0); e != nil {
		t.Fatal(e)
	}
	basis := &invoiceBasis{value: ledger.FromInt64(10)}
	s.Invoices = basis
	receipt := uuid.NewString()
	var successes atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, e := s.RequestInvoice(context.Background(), customer, operator, uuid.NewString(), []InvoiceLine{{receipt, ledger.FromInt64(6)}}, InvoiceDetails{Title: "Synthetic Buyer"})
			if e == nil {
				successes.Add(1)
			} else if !errors.Is(e, ErrConflict) {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
	if successes.Load() != 1 {
		t.Fatalf("same receipt over-invoiced %d", successes.Load())
	}
	invoices, e := s.ListInvoices(context.Background(), customer, operator, true)
	if e != nil || len(invoices) != 1 {
		t.Fatal(e)
	}
	if _, e = s.UploadInvoiceFile(context.Background(), a.actor, operator, invoices[0].ID, "actual-file", InvoiceUpload{Filename: "invoice.pdf", ContentType: "application/pdf", Data: invoicePDFFixture()}); e != nil {
		t.Fatal(e)
	}
	if _, e = s.RegisterInvoice(context.Background(), a.actor, operator, invoices[0].ID, "issued", "invoice-001", "private-proof:one", "invoice issued"); e != nil {
		t.Fatal(e)
	}
	basis.review.Store(true)
	count, e := s.ReviewInvoices(context.Background(), operator, 50)
	if e != nil || count != 1 {
		t.Fatal("refund did not flag invoice review")
	}
	if _, e = s.RequestInvoice(context.Background(), customer, operator, "after-refund", []InvoiceLine{{receipt, ledger.FromInt64(1)}}, InvoiceDetails{Title: "Buyer"}); !errors.Is(e, ErrConflict) {
		t.Fatal("unresolved refund allowed new invoice")
	}
}
func installLedger(t *testing.T, db *gorm.DB) {
	t.Helper()
	_, fsys := ledger.Migrations()
	entries, e := fs.ReadDir(fsys, ".")
	if e != nil {
		t.Fatal(e)
	}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		b, e := fs.ReadFile(fsys, entry.Name())
		if e != nil {
			t.Fatal(e)
		}
		if e = db.Exec(string(b)).Error; e != nil {
			t.Fatal(e)
		}
	}
}
func TestRealLedgerReconciliationExportAuthorizationAndRetention(t *testing.T) {
	s, a, _, operator, customer := opsFixture(t)
	ctx := context.Background()
	installLedger(t, s.DB)
	l := ledger.New(s.DB)
	account, e := l.EnsureAccount(ctx, nil, ledger.Account{OperatorID: operator, OwnerID: customer, Purpose: "customer:usd", Unit: "USD"})
	if e != nil {
		t.Fatal(e)
	}
	control, e := l.EnsureAccount(ctx, nil, ledger.Account{OperatorID: operator, OwnerID: operator, Purpose: "control:cash", Unit: "USD", AllowNegative: true})
	if e != nil {
		t.Fatal(e)
	}
	_, e = l.Post(ctx, nil, ledger.Posting{OperatorID: operator, BusinessKey: "actual-receipt", Kind: "external_receipt", SourceID: "external:one", ActorID: a.actor, Reason: "verified receipt", Lines: []ledger.Line{{AccountID: account.ID, Amount: ledger.FromInt64(10)}, {AccountID: control.ID, Amount: ledger.FromInt64(-10)}}})
	if e != nil {
		t.Fatal(e)
	}
	// Use actual domain schemas: the result sweeper now deletes private objects
	// before metadata and must retain a configured store even for an empty queue.
	apply := func(_ string, files fs.FS) {
		t.Helper()
		entries, e := fs.ReadDir(files, ".")
		if e != nil {
			t.Fatal(e)
		}
		for _, entry := range entries {
			data, e := fs.ReadFile(files, entry.Name())
			if e != nil {
				t.Fatal(e)
			}
			if e = s.DB.Exec(string(data)).Error; e != nil {
				t.Fatal(e)
			}
		}
	}
	apply(cooperation.Migrations())
	apply(usage.Migrations())
	us := usage.New(s.DB, usage.Dependencies{ResultStore: &operationBlobs{data: map[string][]byte{}}})
	s.Source = DomainSource{Ledger: l, Cooperation: cooperation.New(s.DB, l, nil), Usage: us}
	s.Results = us
	run, e := s.RunReconciliation(ctx, a.actor, operator)
	if e != nil || run.FindingCount != 0 {
		t.Fatalf("balanced domain report %+v %v", run, e)
	}
	if e = s.DB.Model(&ledger.Account{}).Where("id=?", account.ID).Update("balance", "11").Error; e != nil {
		t.Fatal(e)
	}
	run, e = s.RunReconciliation(ctx, a.actor, operator)
	if e != nil || run.FindingCount != 1 {
		t.Fatal("materialized corruption not detected")
	}
	rows, e := s.ListDiscrepancies(ctx, a.actor, operator, 10)
	if e != nil || rows[0].Expected.Cmp(ledger.FromInt64(10)) != 0 {
		t.Fatal("reconciliation lost exact amount")
	}
	if e = s.ResolveDiscrepancy(ctx, a.actor, operator, rows[0].ID, uuid.NewString(), "hide mismatch"); e == nil {
		t.Fatal("mismatch hidden with arbitrary reference")
	}
	export, e := s.RequestExport(ctx, a.actor, operator, ExportQuery{Kind: "ledger", OwnerID: customer, From: time.Now().Add(-time.Hour), To: time.Now().Add(time.Hour)})
	if e != nil {
		t.Fatal(e)
	}
	jobs, e := s.RunExports(ctx, 5)
	if e != nil || len(jobs) != 1 || jobs[0].RowCount != 1 {
		t.Fatalf("real export %+v %v", jobs, e)
	}
	data, e := s.DownloadExport(ctx, a.actor, operator, export.ID)
	if e != nil || !strings.Contains(string(data), `"amount":"10"`) {
		t.Fatalf("export mismatch %s %v", data, e)
	}
	a.deny.Store(true)
	if _, e = s.DownloadExport(ctx, a.actor, operator, export.ID); !errors.Is(e, ErrForbidden) {
		t.Fatal("revoked permission retained download")
	}
	a.deny.Store(false)
	if e = s.DB.Model(&Export{}).Where("id=?", export.ID).Update("expires_at", time.Now().Add(-time.Hour)).Error; e != nil {
		t.Fatal(e)
	}
	result, e := s.RunRetention(ctx, time.Now(), 100)
	if e != nil || result.ExportsPurged != 1 {
		t.Fatalf("expired export retained: %+v %v", result, e)
	}
	if _, e = s.DownloadExport(ctx, a.actor, operator, export.ID); !errors.Is(e, ErrExpired) {
		t.Fatal("expired export remained readable", e)
	}
	var journals int64
	_ = s.DB.Model(&ledger.Journal{}).Count(&journals).Error
	if journals != 1 {
		t.Fatal("retention deleted financial facts")
	}
}

func TestExternalSMTPSubmissionFailureAndDurableRecovery(t *testing.T) {
	addr, base := os.Getenv("TOKENHUB_SMTP_MOCK_ADDR"), os.Getenv("TOKENHUB_PAYMENT_MOCK_URL")
	if addr == "" || base == "" {
		t.Skip("standalone integration-lab SMTP and control endpoints required")
	}
	host, portText, e := net.SplitHostPort(addr)
	if e != nil {
		t.Fatal(e)
	}
	port, e := strconv.Atoi(portText)
	if e != nil {
		t.Fatal(e)
	}
	sender, e := mail.NewSMTP(mail.SMTPOptions{Host: host, Port: port, From: "receipts@example.test", Security: "plain"})
	if e != nil {
		t.Fatal(e)
	}
	s, _, _, operator, customer := opsFixture(t)
	s.Mail = sender
	control := func(payload string) map[string]any {
		t.Helper()
		response, e := http.Post(base+"/__control/protocol/smtp", "application/json", strings.NewReader(payload))
		if e != nil {
			t.Fatal(e)
		}
		defer response.Body.Close()
		if response.StatusCode != 200 {
			t.Fatal("SMTP control unavailable")
		}
		var result map[string]any
		if e = json.NewDecoder(response.Body).Decode(&result); e != nil {
			t.Fatal(e)
		}
		return result
	}
	control(`{"fail_count":1}`)
	n, e := s.Emit(context.Background(), nil, Notification{OperatorID: operator, RecipientID: customer, BusinessKey: "smtp:actual-data", Category: "mandatory", Severity: "info", Title: "Synthetic receipt confirmation", Body: "No real funds moved in this protocol test.", Reference: "receipt:synthetic"})
	if e != nil {
		t.Fatal(e)
	}
	attempts, e := s.RunNotifications(context.Background(), 10)
	if e != nil || len(attempts) != 1 || attempts[0].State != "pending" {
		t.Fatalf("SMTP451 became accepted %+v %v", attempts, e)
	}
	if e = s.DB.Table("operations_deliveries").Where("notification_id=?", n.ID).Updates(map[string]any{"state": "processing", "lease_until": time.Now().Add(-time.Minute)}).Error; e != nil {
		t.Fatal(e)
	}
	attempts, e = s.RunNotifications(context.Background(), 10)
	if e != nil || len(attempts) != 1 || attempts[0].State != "accepted" {
		t.Fatalf("SMTP DATA recovery %+v %v", attempts, e)
	}
	state := control(`{}`)
	raw, _ := json.Marshal(state["accepted"])
	if !strings.Contains(string(raw), n.ID+"@example.test") {
		t.Fatal("standalone SMTP did not accept stable Message-ID")
	}
}
