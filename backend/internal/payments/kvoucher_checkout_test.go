package payments

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"gorm.io/gorm"
)

func TestKVoucherCheckoutFormIsClaimedOnce(t *testing.T) {
	db := paymentDB(t)
	s := NewService(db, "kvoucher-form-test", testPaymentAuth{}, NewTransport(true), &sqlReceiptSink{})
	var orders, forms atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /api/exchange-rates":
			_, _ = w.Write([]byte(`{"rates":{"USD":0.25}}`))
		case "POST /api/kv/merchant/v1/orders":
			orders.Add(1)
			_, _ = w.Write([]byte(`{"ok":true,"data":{"order":{"id":30,"status":"pending_payment"}}}`))
		case "POST /api/kv/merchant/v1/orders/30/gkash-pay":
			forms.Add(1)
			_, _ = w.Write([]byte(`{"ok":true,"data":{"payment":{"action":"https://pay.example.test/form","method":"POST","fields":{"v_cartid":"synthetic-cart","signature":"fixture-signature","v_amount":"40.25","v_currency":"MYR"}}}}`))
		case "GET /api/kv/merchant/v1/orders/31":
			_, _ = w.Write([]byte(`{"ok":true,"data":{"order":{"id":31,"status":"pending_payment","items":[{"denomination":40,"qty":1}]}}}`))
		case "POST /api/kv/merchant/v1/orders/31/gkash-pay":
			forms.Add(1)
			_, _ = w.Write([]byte(`{"ok":true,"data":{"payment":{"action":"https://pay.example.test/form","method":"POST","fields":{"v_cartid":"synthetic-replacement","signature":"new-fixture-signature","v_amount":"40.25","v_currency":"MYR"}}}}`))
		default:
			t.Errorf("unexpected checkout request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	m := Merchant{ID: uuid.NewString(), OperatorID: uuid.NewString(), Driver: "kvoucher", Enabled: true, APIBaseURL: server.URL + "/api/kv/merchant/v1", Credentials: map[string]string{"kvoucher_api_key": "fixture", "kvoucher_api_secret": "fixture"}}
	ctx := t.Context()
	if _, err := s.SaveMerchant(ctx, "finance", m, 0); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	o := Order{ID: uuid.NewString(), OperatorID: m.OperatorID, MerchantID: m.ID, Money: Money{Currency: "MYR"}, SettlementMoney: Money{Currency: "USD", Amount: ledger.FromInt64(10)}, AcceptedAt: now, ExpiresAt: now.Add(time.Hour)}
	for range 2 {
		ch, err := s.Create(ctx, m.OperatorID, m.ID, o)
		if err != nil || ch.URL != "" || len(ch.FormFields) != 0 || ch.PaymentQuote == nil || ch.PaymentQuote.PayMoney.Amount.String() != "40" || ch.PayMoney == nil || ch.PayMoney.Amount.String() != "40.25" {
			t.Fatal("order creation should return the original amounts without the form", err)
		}
	}
	assertView := func(id, state string, generation int64) {
		t.Helper()
		view, err := s.PaymentIntent(ctx, m.OperatorID, m.ID, id)
		if err != nil || view.KVoucherFormState != state || view.KVoucherFormGeneration != generation || view.Checkout == nil || view.Checkout.URL != "" || len(view.Checkout.FormFields) != 0 {
			t.Fatalf("expected form state %q at generation %d with fields withheld, got %q at %d (%v)", state, generation, view.KVoucherFormState, view.KVoucherFormGeneration, err)
		}
	}
	assertView(o.ID, KVoucherFormReady, 0)

	scope := func(context.Context, *gorm.DB, string) (string, string, error) { return m.OperatorID, m.ID, nil }
	wrongOperator := func(context.Context, *gorm.DB, string) (string, string, error) { return "wrong-operator", m.ID, nil }
	_, _, err := s.OpenKVoucherCheckout(ctx, o.ID, 0, uuid.NewString(), wrongOperator)
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatal("unrelated operator was able to claim the form", err)
	}

	type claim struct {
		checkout   Checkout
		generation int64
		key        string
		err        error
	}
	start := make(chan struct{})
	results := make(chan claim, 2)
	for range 2 {
		go func() {
			<-start
			key := uuid.NewString()
			checkout, generation, err := s.OpenKVoucherCheckout(ctx, o.ID, 0, key, scope)
			results <- claim{checkout, generation, key, err}
		}()
	}
	close(start)
	var issued Checkout
	var issuedKey string
	var issuedCount int
	for range 2 {
		result := <-results
		if result.generation != 1 {
			t.Fatal("customer should receive the current checkout generation", result.generation, result.err)
		}
		if result.err == nil {
			issued = result.checkout
			issuedKey = result.key
			issuedCount++
		} else if !errors.Is(result.err, ErrCheckoutUnavailable) || result.checkout.URL != "" || len(result.checkout.FormFields) != 0 {
			t.Fatal("the losing claim should receive no form", result.err)
		}
	}
	if issuedCount != 1 || issued.ProviderID != "30" || issued.URL != "https://pay.example.test/form" || issued.FormFields["v_cartid"] != "synthetic-cart" || issued.FormFields["signature"] != "fixture-signature" {
		t.Fatal("exactly one claim should receive the original signed fields", issuedCount)
	}
	assertView(o.ID, KVoucherFormClaimed, 1)
	replayed, generation, err := s.OpenKVoucherCheckout(ctx, o.ID, 1, issuedKey, scope)
	if !errors.Is(err, ErrCheckoutUnavailable) || generation != 1 || replayed.URL != "" || len(replayed.FormFields) > 0 {
		t.Fatal("the same action key should not issue another form", generation, err)
	}
	if orders.Load() != 1 || forms.Load() != 1 {
		t.Fatal("lookup and claim should never request new gateway payment forms", orders.Load(), forms.Load())
	}

	legacy := o
	legacy.ID = uuid.NewString()
	legacy.PaymentDriver = "kvoucher"
	legacy.Money.Amount = ledger.FromInt64(40)
	legacyOrder, err := s.seal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	issued.ProviderID = "31"
	legacyCheckout, err := s.seal(issued)
	if err != nil {
		t.Fatal(err)
	}
	err = db.Table("payments_intents").Create(map[string]any{
		"merchant_id": m.ID, "order_id": legacy.ID, "operator_id": m.OperatorID, "fingerprint": "legacy",
		"state": "created", "order_ciphertext": legacyOrder, "checkout_ciphertext": legacyCheckout,
		"provider_id": issued.ProviderID, "created_at": now, "updated_at": now,
	}).Error
	if err != nil {
		t.Fatal(err)
	}
	assertView(legacy.ID, KVoucherFormUnavailable, 0)
	replacement, generation, err := s.OpenKVoucherCheckout(ctx, legacy.ID, 0, uuid.NewString(), scope)
	if err != nil || generation != 1 || replacement.ProviderID != "31" || replacement.FormFields["v_cartid"] != "synthetic-replacement" || replacement.PayMoney.Amount.String() != "40.25" {
		t.Fatal("the legacy order should yield only a newly signed form", generation, err)
	}
	assertView(legacy.ID, KVoucherFormUnavailable, 1)
}

func TestKVoucherRetriesCheckOriginalOrderAndRejectChangedForms(t *testing.T) {
	db := paymentDB(t)
	s := NewService(db, "kvoucher-retry-test", testPaymentAuth{}, NewTransport(true), &sqlReceiptSink{})
	type reply struct {
		status  int
		body    string
		entered chan struct{}
		release chan struct{}
	}
	var lookup, signed atomic.Value
	var signCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var response reply
		switch r.Method + " " + r.URL.Path {
		case "GET /api/kv/merchant/v1/orders/31":
			response = lookup.Load().(reply)
		case "POST /api/kv/merchant/v1/orders/31/gkash-pay":
			signCalls.Add(1)
			response = signed.Load().(reply)
		default:
			t.Errorf("unexpected retry request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		if response.entered != nil {
			close(response.entered)
			select {
			case <-response.release:
			case <-r.Context().Done():
				return
			}
		}
		w.WriteHeader(response.status)
		_, _ = w.Write([]byte(response.body))
	}))
	t.Cleanup(server.Close)
	m := Merchant{ID: uuid.NewString(), OperatorID: uuid.NewString(), Driver: "kvoucher", Enabled: true, APIBaseURL: server.URL + "/api/kv/merchant/v1", Credentials: map[string]string{"kvoucher_api_key": "fixture", "kvoucher_api_secret": "fixture"}}
	ctx := t.Context()
	if _, err := s.SaveMerchant(ctx, "finance", m, 0); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	order := Order{
		ID: uuid.NewString(), MerchantID: m.ID, OperatorID: m.OperatorID, PaymentDriver: "kvoucher",
		Money: Money{Currency: "MYR", Amount: ledger.FromInt64(40)}, AcceptedAt: now, ExpiresAt: now.Add(time.Hour),
	}
	orderCipher, err := s.seal(order)
	if err != nil {
		t.Fatal(err)
	}
	checkoutCipher, err := s.seal(Checkout{
		ProviderID: "31", URL: "https://pay.example.test/form", FormFields: map[string]string{"v_cartid": "original-cart", "signature": "fixture"},
		PayMoney: &Money{Currency: "MYR", Amount: ledger.MustDecimal("40.25")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Table("payments_intents").Create(map[string]any{
		"merchant_id": m.ID, "order_id": order.ID, "operator_id": m.OperatorID, "fingerprint": "fixture",
		"state": "created", "order_ciphertext": orderCipher, "checkout_ciphertext": checkoutCipher, "provider_id": "31",
		"created_at": now, "updated_at": now, "checkout_issue_until": now.Add(-time.Second),
	}).Error; err != nil {
		t.Fatal(err)
	}
	scope := func(context.Context, *gorm.DB, string) (string, string, error) { return m.OperatorID, m.ID, nil }
	orderReply := func(id, status, face string) reply {
		return reply{status: 200, body: `{"ok":true,"data":{"order":{"id":` + id + `,"status":"` + status + `","items":[{"denomination":` + face + `,"qty":1}]}}}`}
	}
	pending := orderReply("31", "pending_payment", "40")
	formReply := func(cart, amount, currency string) reply {
		return reply{status: 200, body: `{"ok":true,"data":{"payment":{"action":"https://pay.example.test/form","method":"POST","fields":{"v_cartid":"` + cart + `","signature":"new-fixture-signature","v_amount":"` + amount + `","v_currency":"` + currency + `"}}}}`}
	}
	var generation int64
	for _, tc := range []struct {
		name          string
		lookup        reply
		cart, amount  string
		currency      string
		want          error
		wantSignCalls int32
	}{
		{name: "submitted payment", lookup: orderReply("31", "payment_submitted", "40"), want: ErrNotFinal},
		{name: "unknown lookup", lookup: reply{status: 504}, want: ErrOutcomeUnknown},
		{name: "wrong order", lookup: orderReply("30", "pending_payment", "40"), want: ErrConflict},
		{name: "wrong denomination", lookup: orderReply("31", "pending_payment", "40.25"), want: ErrConflict},
		{name: "original cart", lookup: pending, cart: "ORIGINAL-CART", amount: "40.25", currency: "MYR", want: ErrCheckoutCartReused, wantSignCalls: 1},
		{name: "changed payer amount", lookup: pending, cart: "second-cart", amount: "40.30", currency: "MYR", want: ErrCheckoutMismatch, wantSignCalls: 1},
		{name: "changed currency", lookup: pending, cart: "second-cart", amount: "40.25", currency: "USD", want: ErrCheckoutMismatch, wantSignCalls: 1},
		{name: "first replacement", lookup: pending, cart: "second-cart", amount: "40.25", currency: "MYR", wantSignCalls: 1},
		{name: "second replacement", lookup: pending, cart: "third-cart", amount: "40.25", currency: "MYR", wantSignCalls: 1},
		{name: "previous replacement cart", lookup: pending, cart: "SECOND-CART", amount: "40.25", currency: "MYR", want: ErrCheckoutCartReused, wantSignCalls: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lookup.Store(tc.lookup)
			signed.Store(formReply(tc.cart, tc.amount, tc.currency))
			before := signCalls.Load()
			form, nextGeneration, err := s.OpenKVoucherCheckout(ctx, order.ID, generation, uuid.NewString(), scope)
			if !errors.Is(err, tc.want) || nextGeneration != generation+1 || signCalls.Load()-before != tc.wantSignCalls {
				t.Fatalf("expected decision %v, a fresh generation and %d signing calls, got %v, %d and %d", tc.want, tc.wantSignCalls, err, nextGeneration, signCalls.Load()-before)
			}
			generation = nextGeneration
			if tc.want == nil {
				if form.FormFields["v_cartid"] != tc.cart || form.PayMoney == nil || form.PayMoney.Amount.String() != "40.25" {
					t.Fatal("retry did not preserve the original payer amount and use the newly signed cart")
				}
			} else if form.URL != "" || len(form.FormFields) != 0 {
				t.Fatal("blocked checkout exposed a signed form")
			}
		})
	}

	for _, phase := range []string{"checking", "signing"} {
		t.Run("receipt while "+phase, func(t *testing.T) {
			entered, release := make(chan struct{}), make(chan struct{})
			defer func() {
				select {
				case <-release:
				default:
					close(release)
				}
			}()
			check, sign := pending, formReply("new-receipt-race-cart", "40.25", "MYR")
			if phase == "checking" {
				check.entered, check.release = entered, release
			} else {
				sign.entered, sign.release = entered, release
			}
			lookup.Store(check)
			signed.Store(sign)
			before := signCalls.Load()
			completed := make(chan error, 1)
			expected := generation
			go func() {
				form, next, err := s.OpenKVoucherCheckout(ctx, order.ID, expected, uuid.NewString(), scope)
				if next != expected+1 || form.URL != "" || len(form.FormFields) != 0 {
					completed <- errors.New("the superseded operation returned a form or lost its generation")
					return
				}
				completed <- err
			}()
			select {
			case <-entered:
			case err := <-completed:
				t.Fatal("retry stopped before the simulated remote wait", err)
			case <-time.After(3 * time.Second):
				t.Fatal("retry did not reach the expected remote wait")
			}

			receiptCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			defer cancel()
			view, err := s.PaymentIntent(receiptCtx, m.OperatorID, m.ID, order.ID)
			if err != nil || view.KVoucherFormState != KVoucherFormPreparing {
				t.Fatal("in-flight checkout was not visible for concurrency control", view.KVoucherFormState, err)
			}
			form, current, err := s.OpenKVoucherCheckout(receiptCtx, order.ID, expected+1, uuid.NewString(), scope)
			if !errors.Is(err, ErrCheckoutUnavailable) || current != expected+1 || form.URL != "" {
				t.Fatal("a concurrent request could reserve another form", current, err)
			}
			receiptID := uuid.NewString()
			err = db.WithContext(receiptCtx).Transaction(func(tx *gorm.DB) error {
				if err := LockOrderPayment(tx, order.ID); err != nil {
					return err
				}
				return tx.Table("payments_verified_facts").Create(map[string]any{
					"id": receiptID, "merchant_id": m.ID, "operator_id": m.OperatorID, "order_id": order.ID, "kind": "receipt",
					"fingerprint": "receipt", "fact": "{}", "valuation": "{}", "state": "recorded", "observed_at": time.Now(),
				}).Error
			})
			if err != nil {
				t.Fatal("remote wait blocked receipt recording", err)
			}
			close(release)
			if err := <-completed; !errors.Is(err, ErrConflict) {
				t.Fatal("a verified receipt should block delivery of the form", err)
			}
			generation++
			wantSignCalls := int32(0)
			if phase == "signing" {
				wantSignCalls = 1
			}
			if signCalls.Load()-before != wantSignCalls {
				t.Fatal("retry contacted signing at the wrong stage")
			}
			if err := db.Exec("DELETE FROM payments_verified_facts WHERE id=?", receiptID).Error; err != nil {
				t.Fatal(err)
			}
		})
	}
}
