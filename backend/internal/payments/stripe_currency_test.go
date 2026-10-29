package payments

import (
	"context"
	"encoding/json"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestStripeNativeCurrenciesPreserveAmountsAcrossChargeAndRefund(t *testing.T) {
	for _, tc := range []struct {
		currency, amount string
		minor            int64
	}{{"CNY", "70", 7000}, {"EUR", "12.34", 1234}, {"JPY", "1234", 1234}} {
		t.Run(tc.currency, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.Method == "POST" && r.URL.Path == "/v1/payment_intents":
					if e := r.ParseForm(); e != nil {
						t.Error(e)
						return
					}
					if r.Form.Get("currency") != strings.ToLower(tc.currency) || r.Form.Get("amount") != strconv.FormatInt(tc.minor, 10) {
						t.Error("incorrect charge denomination", r.Form)
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"id": "pi_native", "status": "requires_payment_method", "client_secret": "synthetic_client_secret"})
				case r.Method == "GET" && r.URL.Path == "/v1/payment_intents/pi_native":
					_ = json.NewEncoder(w).Encode(map[string]any{"id": "pi_native", "status": "succeeded", "currency": strings.ToLower(tc.currency), "amount_received": tc.minor, "metadata": map[string]string{"order_id": "order", "operator_id": "operator"}})
				case r.Method == "POST" && r.URL.Path == "/v1/refunds":
					if e := r.ParseForm(); e != nil {
						t.Error(e)
						return
					}
					if r.Form.Get("amount") != strconv.FormatInt(tc.minor, 10) {
						t.Error("incorrect refund denomination", r.Form)
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"id": "re_native", "status": "succeeded", "currency": strings.ToLower(tc.currency), "amount": tc.minor, "payment_intent": "pi_native", "metadata": map[string]string{"order_id": "order", "refund_id": "refund"}})
				default:
					t.Error("unexpected protocol request", r.Method, r.URL.Path)
					w.WriteHeader(404)
				}
			}))
			defer server.Close()
			transport := NewTransport(true)
			defer transport.Client.CloseIdleConnections()
			driver := Stripe{HTTP: transport}
			merchant := Merchant{ID: "merchant", OperatorID: "operator", Driver: "stripe", Enabled: true, APIBaseURL: server.URL, Credentials: map[string]string{"secret_key": "synthetic_secret", "publishable_key": "synthetic_publishable", "webhook_secret": "synthetic_webhook", "currency": tc.currency}}
			if ok, _ := SettlementSupport(merchant, tc.currency); !ok {
				t.Fatal("native currency incorrectly blocked")
			}
			money := Money{Currency: tc.currency, Amount: ledger.MustDecimal(tc.amount)}
			now := time.Now().UTC()
			order := Order{ID: "order", OperatorID: merchant.OperatorID, MerchantID: merchant.ID, Money: money, SettlementMoney: money, AcceptedAt: now, ExpiresAt: now.Add(time.Hour)}
			checkout, e := driver.Create(context.Background(), merchant, order)
			if e != nil {
				t.Fatal(e)
			}
			order.ProviderID = checkout.ProviderID
			fact, e := driver.Lookup(context.Background(), merchant, order)
			if e != nil || fact.Money.Currency != tc.currency || fact.Money.Amount.Cmp(money.Amount) != 0 || !fact.Verified() {
				t.Fatal(fact, e)
			}
			refund, e := driver.Refund(context.Background(), merchant, Refund{ID: "refund", Order: order, Money: money, MoneyAmount: money.Amount})
			if e != nil || refund.Fact == nil || refund.Fact.Money.Currency != tc.currency || refund.Fact.Money.Amount.Cmp(money.Amount) != 0 {
				t.Fatal(refund, e)
			}
		})
	}
}
