package payments

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
)

func TestStaticCrossCurrencyCheckoutNeverInventsPaymentAmount(t *testing.T) {
	now := time.Now().UTC()
	for _, driver := range []Driver{Manual{}, EVMWallet{}} {
		m := Merchant{ID: "merchant", OperatorID: "operator", Driver: driver.Name(), Enabled: true, Credentials: map[string]string{"currency": "CNY", "instructions": "bank transfer", "recipient_reference": "own-bank", "rpc_url": "https://rpc.example.test", "chain_id": "1", "token_contract": "0x" + strings.Repeat("a", 40), "wallet_address": "0x" + strings.Repeat("b", 40), "token_decimals": "6", "min_confirmations": "12", "asset": "USDC"}}
		currency := "CNY"
		if driver.Name() == "evm_wallet" {
			currency = "USDC"
		}
		order := Order{ID: "order", OperatorID: m.OperatorID, MerchantID: m.ID, Money: Money{Currency: currency}, SettlementMoney: Money{Currency: "EUR", Amount: ledger.FromInt64(20)}, AcceptedAt: now, ExpiresAt: now.Add(time.Hour)}
		checkout, e := driver.Create(context.Background(), m, order)
		if e != nil || checkout.PayMoney != nil || checkout.SettlementMoney == nil || checkout.SettlementMoney.Currency != "EUR" || checkout.SettlementMoney.Amount.String() != "20" || checkout.PaymentAddress == "" {
			t.Fatalf("%s: %+v %v", driver.Name(), checkout, e)
		}
		fact, e := receiptFact(m, order, "real-transfer", Money{Currency: currency, Amount: ledger.FromInt64(21)}, nil, map[string]string{"evidence": "verified transfer"})
		if e != nil {
			t.Fatal(e)
		}
		if value, reason := receiptExecutionValue(order, fact); reason != "settlement_amount_unconfirmed" || !value.Amount.IsZero() {
			t.Fatalf("invented a settlement relation: %+v %s", value, reason)
		}
	}
}

func TestNOWPaymentsUsesOrderCurrencyAndVerifiedProviderRelation(t *testing.T) {
	for _, withRelation := range []bool{false, true} {
		t.Run(fmt.Sprint(withRelation), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == http.MethodPost {
					var in map[string]any
					if json.NewDecoder(r.Body).Decode(&in) != nil || in["price_currency"] != "eur" || in["price_amount"] != float64(20) || in["is_fee_paid_by_user"] != false {
						t.Errorf("unexpected payment request: %+v", in)
					}
					fmt.Fprint(w, `{"payment_id":123,"pay_address":"wallet","pay_amount":22,"pay_currency":"usdt"}`)
					return
				}
				relation := ""
				if withRelation {
					relation = `,"price_currency":"eur","price_amount":20,"pay_amount":22`
				}
				fmt.Fprint(w, `{"payment_id":123,"order_id":"order","payment_status":"finished","pay_currency":"usdt","actually_paid":22`+relation+`}`)
			}))
			defer server.Close()
			driver := NOWPayments{HTTP: &Transport{Client: server.Client(), AllowTestLoopback: true}}
			m := Merchant{ID: "merchant", OperatorID: "operator", Driver: "nowpayments", Enabled: true, APIBaseURL: server.URL, Credentials: map[string]string{"api_key": "key", "ipn_secret": "secret", "pay_currency": "usdt"}}
			now := time.Now().UTC()
			order := Order{ID: "order", OperatorID: m.OperatorID, MerchantID: m.ID, Money: Money{Currency: "USDT"}, SettlementMoney: Money{Currency: "EUR", Amount: ledger.FromInt64(20)}, AcceptedAt: now, ExpiresAt: now.Add(time.Hour), NotifyURL: "https://example.test/webhook"}
			checkout, e := driver.Create(context.Background(), m, order)
			if e != nil || checkout.PayMoney == nil || checkout.PayMoney.Amount.String() != "22" {
				t.Fatal(checkout, e)
			}
			order.ProviderID = checkout.ProviderID
			fact, e := driver.Lookup(context.Background(), m, order)
			if e != nil || !fact.Verified() || fact.Money.Currency != "USDT" || fact.Money.Amount.String() != "22" {
				t.Fatal(fact, e)
			}
			value, reason := receiptExecutionValue(order, fact)
			if withRelation {
				if reason != "" || value.Currency != "EUR" || value.Amount.String() != "20" || value.OriginalMoney.Amount.String() != "22" {
					t.Fatal(value, reason)
				}
			} else if reason != "settlement_amount_unconfirmed" {
				t.Fatal("missing provider relation became order payment", value, reason)
			}
		})
	}
}

func TestSameCurrencySettlementCannotChangeActualTransferAmount(t *testing.T) {
	money := Money{Currency: "CNY", Amount: ledger.FromInt64(70)}
	confirmation := Valuation{Currency: "CNY", Amount: ledger.FromInt64(10), Source: "verified transfer"}
	if _, err := confirmation.Convert(money, time.Time{}); err == nil {
		t.Fatal("same-currency confirmation changed the actual transfer amount")
	}
	order := Order{Money: money, SettlementMoney: Money{Currency: "CNY", Amount: ledger.FromInt64(70)}}
	fact := Fact{Money: money, ExecutionValue: &MoneyValue{Currency: "CNY", Amount: ledger.FromInt64(10), OriginalMoney: money, EvidenceReference: "verified transfer"}}
	if _, reason := receiptExecutionValue(order, fact); reason != "settlement_confirmation_invalid" {
		t.Fatal("same-currency execution changed the actual transfer amount", reason)
	}
	order.ID, order.OperatorID, order.MerchantID = "order", "operator", "merchant"
	order.AcceptedAt, order.ExpiresAt = time.Now().UTC(), time.Now().UTC().Add(time.Hour)
	order.Money.Amount = ledger.FromInt64(10)
	if err := order.Validate(Merchant{ID: "merchant", OperatorID: "operator"}); err == nil {
		t.Fatal("same-currency checkout disagreed with the order obligation")
	}
}

func TestConfiguredNonUSDSettlementPreservesForeignUSDReceipt(t *testing.T) {
	db := paymentDB(t, "CNY")
	ctx := context.Background()
	s := NewService(db, "non-usd-settlement", testPaymentAuth{}, NewTransport(true), &sqlReceiptSink{})
	m := Merchant{ID: uuid.NewString(), OperatorID: uuid.NewString(), Driver: "manual", Enabled: true, Credentials: map[string]string{"currency": "USD", "instructions": "verified transfer", "recipient_reference": "usd-bank"}}
	if _, err := s.SaveMerchant(ctx, "finance", m, 0); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	order := Order{ID: uuid.NewString(), OperatorID: m.OperatorID, MerchantID: m.ID, Money: Money{Currency: "USD"}, SettlementMoney: Money{Currency: "CNY", Amount: ledger.FromInt64(70)}, AcceptedAt: now, ExpiresAt: now.Add(time.Hour)}
	if _, err := s.Create(ctx, m.OperatorID, m.ID, order); err != nil {
		t.Fatal(err)
	}
	fact, err := s.RecordManual(ctx, "finance", m.OperatorID, m.ID, order.ID, ManualVerification{ExternalReference: "real-usd-transfer", Money: Money{Currency: "USD", Amount: ledger.FromInt64(10)}, PaidAt: now, EvidenceReference: "verified USD bank statement"})
	if !errors.Is(err, ErrAwaitingSettlement) || fact.Money.Currency != "USD" || fact.Money.Amount.String() != "10" {
		t.Fatal("foreign USD was automatically posted as native money", fact, err)
	}
	out, err := s.ConfirmReceiptValuation(ctx, "finance", m.OperatorID, fact.ID, ReceiptValuationConfirmation{BusinessKey: "one-transfer-settlement", SettlementAmount: ledger.FromInt64(70), EvidenceReference: "this USD transfer settles the CNY 70 obligation"})
	if err != nil || out.Value == nil || out.Value.Currency != "CNY" || out.Value.Amount.String() != "70" || out.Value.OriginalMoney.Currency != "USD" || out.Value.OriginalMoney.Amount.String() != "10" || out.ReceiptID == "" {
		t.Fatal("settlement did not preserve original USD money", out, err)
	}
	m.ID, m.Credentials["currency"], m.Credentials["recipient_reference"] = uuid.NewString(), "CNY", "cny-bank"
	if _, err = s.SaveMerchant(ctx, "finance", m, 0); err != nil {
		t.Fatal(err)
	}
	order.ID, order.MerchantID, order.Money = uuid.NewString(), m.ID, order.SettlementMoney
	if _, err = s.Create(ctx, m.OperatorID, m.ID, order); err != nil {
		t.Fatal(err)
	}
	proof := ManualVerification{ExternalReference: "real-cny-transfer", Money: order.Money, PaidAt: now, EvidenceReference: "verified CNY bank statement", SettlementAmount: ledger.FromInt64(10)}
	if _, err = s.RecordManual(ctx, "finance", m.OperatorID, m.ID, order.ID, proof); !errors.Is(err, ErrInvalid) {
		t.Fatal("same-currency contradictory settlement amount was accepted", err)
	}
	proof.SettlementAmount = ledger.Zero()
	if _, err = s.RecordManual(ctx, "finance", m.OperatorID, m.ID, order.ID, proof); err != nil {
		t.Fatal("native CNY required an exchange-rate confirmation", err)
	}
}
