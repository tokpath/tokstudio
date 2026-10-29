package payments

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type testPaymentAuth struct{}

func (testPaymentAuth) LockOperatorResponsibility(context.Context, *gorm.DB, string) error {
	return nil
}

type testRefundApproval struct{ calls atomic.Int32 }

type testRefundSink struct{}

func (testRefundSink) RecordVerifiedRefund(_ context.Context, tx *gorm.DB, m Merchant, o Order, r Refund, f Fact, v MoneyValue) error {
	if !f.Verified() || f.Kind != "refund" || f.OrderID != o.ID || f.MerchantID != m.ID || r.MoneyAmount.Cmp(v.Amount) != 0 {
		return ErrInvalid
	}
	return nil
}

func (a *testRefundApproval) RequireApprovedRefund(context.Context, *gorm.DB, string, Refund) error {
	a.calls.Add(1)
	return nil
}

func (testPaymentAuth) RequirePermission(_ context.Context, _ *gorm.DB, actor, operator, permission string) error {
	if actor != "finance" || operator == "" || permission != "payments.manage" {
		return ErrForbidden
	}
	return nil
}

type sqlReceiptSink struct{ fail atomic.Bool }

func (s *sqlReceiptSink) RecordVerifiedReceipt(ctx context.Context, tx *gorm.DB, m Merchant, o Order, f Fact, v MoneyValue) (string, error) {
	if e := tx.WithContext(ctx).Table("payment_test_receipts").Create(map[string]any{"id": f.ID, "operator_id": m.OperatorID, "amount": v.Amount.String()}).Error; e != nil {
		return "", e
	}
	if s.fail.Load() {
		return "", ErrProvider
	}
	return f.ID, nil
}
func paymentDB(t *testing.T, currencies ...string) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("TOKENHUB_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TOKENHUB_TEST_DATABASE_URL required for payment transaction integration tests")
	}
	admin, e := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if e != nil {
		t.Fatal(e)
	}
	schema := "payments_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if e = admin.Exec("CREATE SCHEMA " + schema).Error; e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		_ = admin.Exec("DROP SCHEMA " + schema + " CASCADE").Error
		sqlDB, _ := admin.DB()
		_ = sqlDB.Close()
	})
	u, e := url.Parse(dsn)
	if e != nil {
		t.Fatal(e)
	}
	query := u.Query()
	query.Set("search_path", schema)
	u.RawQuery = query.Encode()
	db, e := gorm.Open(postgres.Open(u.String()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { sqlDB, _ := db.DB(); _ = sqlDB.Close() })
	if e = db.Exec(Schema()).Error; e != nil {
		t.Fatal(e)
	}
	if e = db.Exec("CREATE TABLE payment_test_receipts(id text PRIMARY KEY,operator_id text NOT NULL,amount numeric(38,18) NOT NULL)").Error; e != nil {
		t.Fatal(e)
	}
	if e = db.AutoMigrate(&ledger.MonetarySettings{}); e != nil {
		t.Fatal(e)
	}
	effective := time.Now().UTC().Add(-time.Hour)
	currency := "USD"
	if len(currencies) > 0 {
		currency = currencies[0]
	}
	if e = db.Create(&ledger.MonetarySettings{ID: "platform", Currency: currency, AtgPerUnit: ledger.FromInt64(1), EffectiveAt: &effective, Version: 1}).Error; e != nil {
		t.Fatal(e)
	}
	return db
}
func TestPaymentMerchantEncryptionManualFactsAndAtomicSink(t *testing.T) {
	db := paymentDB(t)
	sink := &sqlReceiptSink{}
	s := NewService(db, "integration-encryption-key", testPaymentAuth{}, NewTransport(true), sink)
	m := Merchant{ID: uuid.NewString(), OperatorID: uuid.NewString(), Driver: "manual", Enabled: true, Credentials: map[string]string{"currency": "USD", "recipient_reference": "bank:synthetic-account", "instructions": "Verify actual transfer"}}
	if _, e := s.SaveMerchant(context.Background(), "customer", m, 0); !errors.Is(e, ErrForbidden) {
		t.Fatal("customer saved merchant credentials")
	}
	view, e := s.SaveMerchant(context.Background(), "finance", m, 0)
	if e != nil {
		t.Fatal(e)
	}
	raw, _ := json.Marshal(view)
	if strings.Contains(string(raw), "synthetic-account") {
		t.Fatal("merchant response leaked credential values")
	}
	var ciphertext string
	_ = db.Table("payments_merchants").Select("credentials_ciphertext").Where("id=?", m.ID).Scan(&ciphertext).Error
	if strings.Contains(ciphertext, "synthetic-account") {
		t.Fatal("merchant credentials stored plaintext")
	}
	if _, e = s.Merchant(context.Background(), "other-operator", m.ID); e == nil {
		t.Fatal("cross-operator merchant read")
	}
	if _, e = s.SaveMerchant(context.Background(), "finance", m, 7); !errors.Is(e, ErrConflict) {
		t.Fatal("stale merchant overwrite")
	}
	o := paymentTestOrder(m, "USD")
	checkout, e := s.Create(context.Background(), m.OperatorID, m.ID, o)
	if e != nil {
		t.Fatal(e)
	}
	if checkout.PaymentAddress != m.Credentials["recipient_reference"] || checkout.Instructions != m.Credentials["instructions"] || checkout.PayMoney == nil || checkout.PayMoney.Currency != "USD" {
		t.Fatal("manual checkout lost its recipient, instructions or original currency", checkout)
	}
	verification := ManualVerification{ExternalReference: "bank-transfer-1", Money: o.Money, PaidAt: time.Now().UTC(), EvidenceReference: "private-object:bank-statement", PayerReference: "bank:payer-account"}
	if _, e = s.RecordManual(context.Background(), "customer", m.OperatorID, m.ID, o.ID, verification); !errors.Is(e, ErrForbidden) {
		t.Fatal("customer verified own funds")
	}
	sink.fail.Store(true)
	if _, e = s.RecordManual(context.Background(), "finance", m.OperatorID, m.ID, o.ID, verification); e == nil {
		t.Fatal("sink failure ignored")
	}
	var count int64
	_ = db.Table("payments_verified_facts").Count(&count).Error
	if count != 1 {
		t.Fatal("verified original-currency fact was lost when receipt matching failed")
	}
	_ = db.Table("payment_test_receipts").Count(&count).Error
	if count != 0 {
		t.Fatal("receipt survived transaction rollback")
	}
	sink.fail.Store(false)
	var wg sync.WaitGroup
	errs := make(chan error, 16)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, e := s.RecordManual(context.Background(), "finance", m.OperatorID, m.ID, o.ID, verification)
			errs <- e
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	_ = db.Table("payment_test_receipts").Count(&count).Error
	if count != 1 {
		t.Fatalf("duplicate financial receipt count=%d", count)
	}
	verification.Money.Amount = ledger.MustDecimal("11")
	if _, e = s.RecordManual(context.Background(), "finance", m.OperatorID, m.ID, o.ID, verification); !errors.Is(e, ErrConflict) {
		t.Fatal("same actual receipt ID changed amount")
	}
}
func TestExternalUnknownCreateDurableCallbackAndNoDuplicateCollection(t *testing.T) {
	base, config := externalPaymentLab(t)
	db := paymentDB(t)
	transport := NewTransport(true)
	defer transport.Client.CloseIdleConnections()
	sink := &sqlReceiptSink{}
	service := NewService(db, "test-encryption-material", testPaymentAuth{}, transport, sink)
	m := merchantFixture("stripe", base, config)
	if _, e := service.SaveMerchant(context.Background(), "finance", m, 0); e != nil {
		t.Fatal(e)
	}
	o := paymentTestOrder(m, "USD")
	paymentControl(t, base, "/__control/faults", map[string]any{"path": "/v1/payment_intents", "mode": "timeout_after", "count": 1})
	transport.Client.CloseIdleConnections()
	if _, e := service.Create(context.Background(), m.OperatorID, m.ID, o); !errors.Is(e, ErrOutcomeUnknown) {
		t.Fatalf("expected ambiguous external create, got %v", e)
	}
	if _, e := service.Create(context.Background(), m.OperatorID, m.ID, o); !errors.Is(e, ErrOutcomeUnknown) {
		t.Fatal("unknown create was blindly resubmitted")
	}
	response, e := http.Get(base + "/__control/state")
	if e != nil {
		t.Fatal(e)
	}
	var state struct {
		Payments map[string]struct {
			Metadata struct {
				OrderID string `json:"order_id"`
			}
		}
	}
	if json.NewDecoder(response.Body).Decode(&state) != nil {
		t.Fatal("mock state malformed")
	}
	_ = response.Body.Close()
	providerID := ""
	matches := 0
	for id, p := range state.Payments {
		if p.Metadata.OrderID == o.ID {
			providerID = id
			matches++
		}
	}
	if matches != 1 {
		t.Fatalf("unknown create produced %d actual payments", matches)
	}
	settled := paymentControl(t, base, "/__control/payments/"+providerID+"/settle", map[string]any{})
	eventID, _ := settled["event_id"].(string)
	callbacks := make(chan error, 2)
	server := externalCallbackServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_, e := service.ReceiveWebhook(r.Context(), m.ID, Webhook{Headers: r.Header, Body: body, ReceivedAt: time.Now().UTC()})
		callbacks <- e
		if e != nil {
			w.WriteHeader(400)
			return
		}
		w.WriteHeader(200)
	}))
	defer server.Close()
	paymentControl(t, base, "/__control/webhooks", map[string]any{"event_id": eventID, "url": server.URL, "repeat": 2})
	for i := 0; i < 2; i++ {
		select {
		case e := <-callbacks:
			if e != nil {
				t.Fatal(e)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("external callback was not delivered")
		}
	}
	var inboxCount int64
	_ = db.Table("payments_webhook_inbox").Count(&inboxCount).Error
	if inboxCount != 1 {
		t.Fatalf("duplicate callback inbox count=%d", inboxCount)
	}
	paymentControl(t, base, "/__control/faults", map[string]any{"path": "/v1/payment_intents/" + providerID, "mode": "error", "status": 503, "count": 1})
	attempts, e := service.RunPending(context.Background(), 10)
	if e != nil || len(attempts) != 1 || attempts[0].State != "pending" {
		var inbox struct {
			State         string
			NextAttemptAt time.Time
			DatabaseNow   time.Time
		}
		_ = db.Table("payments_webhook_inbox").Select("state,next_attempt_at,now() AS database_now").Take(&inbox).Error
		t.Fatalf("durable retry: %+v %v; inbox=%+v local_now=%s", attempts, e, inbox, time.Now().UTC())
	}
	attempts, e = service.RunPending(context.Background(), 10)
	if e != nil || len(attempts) != 0 {
		t.Fatal("backoff ignored")
	}
	// Simulate a crashed worker lease instead of waiting in the test.
	if e = db.Table("payments_webhook_inbox").Where("merchant_id=?", m.ID).Updates(map[string]any{"state": "processing", "lease_until": time.Now().Add(-time.Minute)}).Error; e != nil {
		t.Fatal(e)
	}
	var facts int64
	_ = db.Table("payments_verified_facts").Count(&facts).Error
	if facts != 0 {
		t.Fatal("unverified webhook became receipt")
	}
	attempts, e = service.RunPending(context.Background(), 10)
	if e != nil || len(attempts) != 1 || attempts[0].State != "processed" {
		t.Fatalf("lease recovery: %+v %v", attempts, e)
	}
	if _, e = service.ProcessInbox(context.Background(), m.ID, eventID); e != nil {
		t.Fatal(e)
	}
	_ = db.Table("payment_test_receipts").Count(&facts).Error
	if facts != 1 {
		t.Fatal("duplicate callback repeated receipt")
	}
}
func TestVerifiedFactWaitsForSinkAndReplaysOriginalEvidence(t *testing.T) {
	db := paymentDB(t)
	s := NewService(db, "test-key", testPaymentAuth{}, NewTransport(true), nil)
	m := Merchant{ID: uuid.NewString(), OperatorID: uuid.NewString(), Driver: "manual", Enabled: true, Credentials: map[string]string{"currency": "USD", "recipient_reference": "bank:account", "instructions": "bank transfer"}}
	if _, e := s.SaveMerchant(context.Background(), "finance", m, 0); e != nil {
		t.Fatal(e)
	}
	o := paymentTestOrder(m, "USD")
	if _, e := s.Create(context.Background(), m.OperatorID, m.ID, o); e != nil {
		t.Fatal(e)
	}
	verification := ManualVerification{ExternalReference: "stable-bank-reference", Money: o.Money, PaidAt: time.Now().UTC(), EvidenceReference: "statement:evidence", PayerReference: "bank:payer"}
	if _, e := s.RecordManual(context.Background(), "finance", m.OperatorID, m.ID, o.ID, verification); e != nil {
		t.Fatal(e)
	}
	var count int64
	_ = db.Table("payment_test_receipts").Count(&count).Error
	if count != 0 {
		t.Fatal("unconfigured sink pretended to record receipt")
	}
	s.Sink = &sqlReceiptSink{}
	if _, e := s.RecordManual(context.Background(), "finance", m.OperatorID, m.ID, o.ID, verification); e != nil {
		t.Fatal(e)
	}
	_ = db.Table("payment_test_receipts").Count(&count).Error
	if count != 1 {
		t.Fatal("verified fact could not resume after sink configured")
	}
}
func TestExternalRefundUnknownRequiresOriginalApprovalAndIdentity(t *testing.T) {
	base, config := externalPaymentLab(t)
	for _, name := range []string{"stripe", "alipay", "wechat"} {
		t.Run(name, func(t *testing.T) {
			db := paymentDB(t)
			transport := NewTransport(true)
			defer transport.Client.CloseIdleConnections()
			s := NewService(db, "refund-test-key", testPaymentAuth{}, transport, &sqlReceiptSink{})
			m := merchantFixture(name, base, config)
			if _, e := s.SaveMerchant(context.Background(), "finance", m, 0); e != nil {
				t.Fatal(e)
			}
			currency := "CNY"
			if name == "stripe" {
				currency = "USD"
			}
			if e := db.Model(&ledger.MonetarySettings{}).Where("id=?", "platform").Update("currency", currency).Error; e != nil {
				t.Fatal(e)
			}
			o := paymentTestOrder(m, currency)
			checkout, e := s.Create(context.Background(), m.OperatorID, m.ID, o)
			if e != nil {
				t.Fatal(e)
			}
			o.ProviderID = checkout.ProviderID
			if name == "stripe" {
				paymentControl(t, base, "/__control/payments/"+o.ProviderID+"/settle", map[string]any{})
			} else {
				paymentControl(t, base, "/__control/protocol/settle", map[string]any{"driver": name, "order_id": o.ID})
			}
			if _, e = s.Reconcile(context.Background(), m.OperatorID, m.ID, o.ID); e != nil {
				t.Fatal(e)
			}
			r := Refund{ID: uuid.NewString(), Order: o, Money: Money{Currency: currency, Amount: ledger.MustDecimal("2")}, MoneyAmount: ledger.MustDecimal("2"), RefundValuation: o.Valuation, ValuedAt: o.AcceptedAt, FXExecutionReference: "synthetic-current-refund-fx"}
			if _, e = s.SubmitRefund(context.Background(), "finance", m.OperatorID, m.ID, r); !errors.Is(e, ErrNotConfigured) {
				t.Fatal("finance permission bypassed business refund approval")
			}
			approval := &testRefundApproval{}
			s.RefundApproval = approval
			s.RefundSink = testRefundSink{}
			path := "/v1/refunds"
			if name == "alipay" {
				path = "/gateway.do"
			}
			if name == "wechat" {
				path = "/v3/refund/domestic/refunds"
			}
			paymentControl(t, base, "/__control/faults", map[string]any{"path": path, "mode": "timeout_after", "count": 1})
			transport.Client.CloseIdleConnections()
			if _, e = s.SubmitRefund(context.Background(), "finance", m.OperatorID, m.ID, r); !errors.Is(e, ErrOutcomeUnknown) {
				t.Fatalf("refund outcome %v", e)
			}
			if _, e = s.SubmitRefund(context.Background(), "finance", m.OperatorID, m.ID, r); !errors.Is(e, ErrOutcomeUnknown) {
				t.Fatal("unknown refund blindly reissued")
			}
			if approval.calls.Load() != 1 {
				t.Fatal("duplicate refund re-reserved business authorization")
			}
			result, e := s.QueryRefund(context.Background(), m.OperatorID, m.ID, r.ID)
			if e != nil {
				t.Fatal(e)
			}
			if name == "stripe" && result.State == "pending" {
				paymentControl(t, base, "/__control/refunds/"+result.ProviderID+"/settle", map[string]any{"status": "succeeded"})
				result, e = s.QueryRefund(context.Background(), m.OperatorID, m.ID, r.ID)
			}
			if e != nil || result.Fact == nil || !result.Fact.Verified() {
				t.Fatalf("refund not recovered %#v %v", result, e)
			}
			if _, e = s.QueryRefund(context.Background(), m.OperatorID, m.ID, r.ID); e != nil {
				t.Fatal(e)
			}
			var count int64
			_ = db.Table("payments_verified_facts").Where("kind='refund'").Count(&count).Error
			if count != 1 {
				t.Fatalf("same actual refund recorded %d times", count)
			}
		})
	}
}
