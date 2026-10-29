package cooperation

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"github.com/tokpath/tokstudio/backend/internal/payments"
	"gorm.io/gorm"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

type testReplenishmentObserver struct{}

func (testReplenishmentObserver) RecordReplenishmentNotice(ctx context.Context, tx *gorm.DB, n ReplenishmentNotice) error {
	return tx.Exec(`INSERT INTO test_replenishment_notices(id,kind) VALUES(?,?) ON CONFLICT DO NOTHING`, n.AttemptID+":"+n.Kind, n.Kind).Error
}
func replenishmentFixture(t *testing.T, base string) (fixture, *Replenisher, payments.Merchant, PurchaseQuote) {
	t.Helper()
	f := setup(t)
	inv, e := f.org.InviteStaff(f.ctx, nil, f.owner, f.channel, "buyer-owner@example.test", "owner")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.org.AcceptStaffInvitation(f.ctx, nil, inv.Token, organization.VerifiedStaff{ID: f.owner, Email: "buyer-owner@example.test", EmailVerified: true}); e != nil {
		t.Fatal(e)
	}
	if e = f.db.Exec(payments.Schema()).Error; e != nil {
		t.Fatal(e)
	}
	if e = f.db.Exec(`CREATE TABLE test_replenishment_notices(id text PRIMARY KEY,kind text NOT NULL)`).Error; e != nil {
		t.Fatal(e)
	}
	collector := commerce.New(f.db)
	pay := payments.NewService(f.db, "synthetic-encryption", f.org, payments.NewTransport(true), payments.CommerceSink{Commerce: collector})
	m := payments.Merchant{ID: uuid.NewString(), OperatorID: f.platform, Driver: "stripe", Enabled: true, APIBaseURL: base, Credentials: map[string]string{"secret_key": "sk_test_lab", "publishable_key": "pk_test_lab", "webhook_secret": "whsec_integration_test", "currency": "USD"}}
	if _, e = pay.SaveMerchant(f.ctx, f.owner, m, 0); e != nil {
		t.Fatal(e)
	}
	quote, e := f.s.PublishPurchaseQuote(f.ctx, nil, f.owner, PurchaseQuote{SupplierID: f.platform, BuyerID: f.channel, GiftTiers: []PurchaseGiftTier{{FromMoney: amount("0"), GiftPercent: amount("900")}}, ExitTerms: json.RawMessage(`{"version":1,"voluntary_refund":true,"method":"remaining_paid_proportional","terms_reference":"accepted-exit-v1"}`), TermsReference: "quote-v1", Enabled: true})
	if e != nil {
		t.Fatal(e)
	}
	r := &Replenisher{Cooperation: f.s, Payments: pay, Collector: collector, EncryptionKey: "synthetic-encryption", Observer: testReplenishmentObserver{}}
	return f, r, m, quote
}
func authorizeReplenishment(t *testing.T, f fixture, r *Replenisher, m payments.Merchant, q PurchaseQuote, mode string) ReplenishmentAuthorization {
	t.Helper()
	merchant := m.ID
	a, e := r.SaveAuthorization(f.ctx, f.owner, ReplenishmentAuthorizationInput{ReplenishmentAuthorization: ReplenishmentAuthorization{BuyerID: f.channel, SupplierID: f.platform, QuoteID: q.ID, AmountMoney: amount("10"), TriggerAvailableATG: amount("200"), PeriodLimitMoney: amount("10"), Period: "month", Mode: mode, MerchantID: &merchant, ConsentReference: "explicit-consent:one", Enabled: true}, Payment: ReplenishmentPayment{PaymentMethod: "pm_synthetic", CustomerRef: "cus_synthetic"}})
	if e != nil {
		t.Fatal(e)
	}
	return a
}
func TestPostgresOfflineReplenishmentNeverGrantsUnverifiedCredit(t *testing.T) {
	f, r, m, q := replenishmentFixture(t, "")
	authorizeReplenishment(t, f, r, m, q, "offline")
	jobs, e := r.Run(f.ctx, 20)
	if e != nil || len(jobs) != 1 || jobs[0].State != "awaiting_manual_payment" {
		t.Fatalf("offline procurement %+v %v", jobs, e)
	}
	_, _ = r.Run(f.ctx, 20)
	var count int64
	_ = f.db.Model(&PurchaseOrder{}).Count(&count).Error
	if count != 1 {
		t.Fatal("in-flight offline procurement was duplicated")
	}
	if automatic, err := r.UsesAutomaticPayment(f.ctx, nil, jobs[0].PurchaseOrderID); err != nil || automatic {
		t.Fatal("offline order cannot require an automatic payment", err)
	}
	if _, e = f.s.Position(f.ctx, nil, f.channel); !errors.Is(e, gorm.ErrRecordNotFound) {
		t.Fatal("offline order created procurement credit before payment")
	}
}
func labControl(t *testing.T, base, path string, v any) map[string]any {
	t.Helper()
	raw, _ := json.Marshal(v)
	res, e := http.Post(base+path, "application/json", strings.NewReader(string(raw)))
	if e != nil {
		t.Fatal(e)
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("external control %d %s", res.StatusCode, b)
	}
	out := map[string]any{}
	if e = json.NewDecoder(res.Body).Decode(&out); e != nil {
		t.Fatal(e)
	}
	return out
}
func TestExternalAutomaticProcurementUnknownThenOriginalSignedCallback(t *testing.T) {
	base := os.Getenv("TOKENHUB_PAYMENT_MOCK_URL")
	if base == "" {
		t.Skip("standalone payment integration lab required")
	}
	f, r, m, q := replenishmentFixture(t, base)
	a := authorizeReplenishment(t, f, r, m, q, "automatic")
	labControl(t, base, "/__control/faults", map[string]any{"path": "/v1/payment_intents", "mode": "timeout_after", "count": 1})
	jobs, e := r.Run(f.ctx, 20)
	if e != nil || len(jobs) != 1 || jobs[0].State != "outcome_unknown" {
		t.Fatalf("unknown original procurement %+v %v", jobs, e)
	}
	orderID := jobs[0].PurchaseOrderID
	if automatic, err := r.UsesAutomaticPayment(f.ctx, nil, orderID); err != nil || !automatic {
		t.Fatal("automatic payment must keep its original method", err)
	}
	if e = r.CancelAuthorization(f.ctx, f.owner, f.channel, a.Version); e != nil {
		t.Fatal(e)
	}
	_, _ = r.Run(f.ctx, 20)
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
	if e = json.NewDecoder(response.Body).Decode(&state); e != nil {
		t.Fatal(e)
	}
	_ = response.Body.Close()
	provider := ""
	count := 0
	for id, payment := range state.Payments {
		if payment.Metadata.OrderID == orderID {
			provider = id
			count++
		}
	}
	if count != 1 {
		t.Fatalf("ambiguous procurement made %d external collections", count)
	}
	if _, e = f.s.Position(f.ctx, nil, f.channel); !errors.Is(e, gorm.ErrRecordNotFound) {
		t.Fatal("unknown external payment granted credit")
	}
	callbacks := make(chan error, 2)
	server := externalCallbackServer(t, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, _ := io.ReadAll(req.Body)
		_, e := r.Payments.ReceiveWebhook(req.Context(), m.ID, payments.Webhook{Headers: req.Header, Body: body, ReceivedAt: time.Now().UTC()})
		callbacks <- e
		if e != nil {
			w.WriteHeader(400)
		}
	}))
	defer server.Close()
	settled := labControl(t, base, "/__control/payments/"+provider+"/settle", map[string]any{})
	labControl(t, base, "/__control/webhooks", map[string]any{"url": server.URL, "event_id": settled["event_id"], "repeat": 2})
	for range 2 {
		select {
		case e = <-callbacks:
			if e != nil {
				t.Fatal(e)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("independent payment service could not reach the callback listener")
		}
	}
	inboxAttempts, inboxErr := r.Payments.RunPending(f.ctx, 20)
	if inboxErr != nil {
		t.Fatal(inboxErr)
	}
	if len(inboxAttempts) != 1 || inboxAttempts[0].State != "processed" {
		t.Fatalf("receipt worker: %+v", inboxAttempts)
	}
	jobs, e = r.Run(f.ctx, 20)
	if e != nil || len(jobs) != 1 || jobs[0].State != "completed" {
		t.Fatalf("original receipt delivery state=%s error=%s %v", jobs[0].State, jobs[0].ErrorCode, e)
	}
	position, e := f.s.Position(f.ctx, nil, f.channel)
	if e != nil || position.Quantity.String() != "100" || position.Cost.String() != "10" {
		t.Fatalf("verified prepaid procurement %+v %v", position, e)
	}
	_, _ = r.Run(f.ctx, 20)
	var lots int64
	_ = f.db.Model(&ProcurementLot{}).Count(&lots).Error
	if lots != 1 {
		t.Fatal("duplicate callbacks repeated procurement delivery")
	}
}
