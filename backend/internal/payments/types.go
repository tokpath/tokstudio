// Package payments verifies external money facts. It never changes customer
// balances, fulfills an order, or treats a checkout/callback as proof of payment.
package payments

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/ledger"
)

var (
	ErrNotConfigured  = errors.New("payment merchant configuration incomplete")
	ErrInvalid        = errors.New("invalid payment input")
	ErrSignature      = errors.New("payment signature invalid")
	ErrOutcomeUnknown = errors.New("external payment outcome requires verification")
	ErrProvider       = errors.New("payment provider rejected request")
	ErrNotFinal       = errors.New("payment is not verified final")
	ErrConflict       = errors.New("payment fact conflicts with existing evidence")
	ErrForbidden      = errors.New("payment action outside operator authority")
	ErrUnsupported    = errors.New("payment operation unsupported")
)

type Money struct {
	Currency string         `json:"currency"`
	Amount   ledger.Decimal `json:"amount"`
}

func (m Money) Validate() error {
	if m.Currency == "" || m.Currency != strings.ToUpper(m.Currency) || m.Amount.Sign() <= 0 || !m.Amount.Valid() {
		return ErrInvalid
	}
	return nil
}
func (m Money) Minor(decimals uint8) (int64, error) {
	if m.Validate() != nil || decimals > 18 {
		return 0, ErrInvalid
	}
	r := new(big.Rat).Mul(m.Amount.Rat(), new(big.Rat).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil)))
	if !r.IsInt() || !r.Num().IsInt64() {
		return 0, ErrInvalid
	}
	return r.Num().Int64(), nil
}
func fromMinor(currency string, value int64, decimals uint8) (Money, error) {
	if value <= 0 || decimals > 18 {
		return Money{}, ErrInvalid
	}
	r := new(big.Rat).SetFrac(big.NewInt(value), new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil))
	amount, remainder := ledger.Quantize(r)
	if remainder.Sign() != 0 {
		return Money{}, ErrInvalid
	}
	return Money{Currency: strings.ToUpper(currency), Amount: amount}, nil
}

// Valuation records the operator's explicitly authorized USD conversion basis.
// A stablecoin is not assumed to be permanently worth one USD.
type Valuation struct {
	ID          string         `json:"id"`
	Currency    string         `json:"currency"`
	USDPerUnit  ledger.Decimal `json:"usd_per_unit"`
	Source      string         `json:"source"`
	EffectiveAt time.Time      `json:"effective_at"`
	ExpiresAt   time.Time      `json:"expires_at"`
}

func (v Valuation) Validate(currency string, at time.Time) error {
	if v.ID == "" || v.Currency != currency || v.USDPerUnit.Sign() <= 0 || !v.USDPerUnit.Valid() || v.Source == "" || v.EffectiveAt.IsZero() || v.ExpiresAt.IsZero() || at.Before(v.EffectiveAt) || !at.Before(v.ExpiresAt) {
		return ErrInvalid
	}
	return nil
}

type USDValue struct {
	Amount               ledger.Decimal `json:"amount"`
	RemainderNumerator   string         `json:"remainder_numerator"`
	RemainderDenominator string         `json:"remainder_denominator"`
	Valuation            Valuation      `json:"valuation"`
}

func (v Valuation) Convert(m Money, executionAt time.Time) (USDValue, error) {
	if m.Validate() != nil || v.Validate(m.Currency, executionAt) != nil {
		return USDValue{}, ErrInvalid
	}
	amount, remainder := ledger.Quantize(new(big.Rat).Mul(m.Amount.Rat(), v.USDPerUnit.Rat()))
	if !amount.Valid() {
		return USDValue{}, ErrInvalid
	}
	return USDValue{Amount: amount, RemainderNumerator: remainder.Num().String(), RemainderDenominator: remainder.Denom().String(), Valuation: v}, nil
}

type Merchant struct {
	ID                    string            `json:"id"`
	OperatorID            string            `json:"operator_id"`
	Driver                string            `json:"driver"`
	Enabled               bool              `json:"enabled"`
	Version               uint64            `json:"version"`
	AssetNetworkVersionID string            `json:"asset_network_version_id,omitempty"`
	Credentials           map[string]string `json:"-"`
	// APIBaseURL is platform-controlled. Production accepts official endpoints;
	// the transport's explicit test option permits isolated loopback mocks.
	APIBaseURL string `json:"api_base_url,omitempty"`
}
type Order struct {
	ID                    string    `json:"id"`
	ContextKind           string    `json:"context_kind,omitempty"`
	OperatorID            string    `json:"operator_id"`
	MerchantID            string    `json:"merchant_id"`
	Money                 Money     `json:"money"`
	Valuation             Valuation `json:"valuation"`
	AcceptedAt            time.Time `json:"accepted_at"`
	ExpiresAt             time.Time `json:"expires_at"`
	NotifyURL             string    `json:"notify_url"`
	Description           string    `json:"description"`
	ProviderID            string    `json:"provider_id,omitempty"`
	PaymentDriver         string    `json:"payment_driver,omitempty"`
	AssetNetworkVersionID string    `json:"asset_network_version_id,omitempty"`
	Network               string    `json:"network,omitempty"`
	TokenContract         string    `json:"token_contract,omitempty"`
	MinimumConfirmations  string    `json:"minimum_confirmations,omitempty"`
	// Stripe off-session execution additionally requires a separately saved
	// business authorization reference. It is never implied by a saved method.
	PaymentMethod   string `json:"-"`
	CustomerRef     string `json:"-"`
	AuthorizationID string `json:"authorization_id,omitempty"`
}

func (o Order) Validate(m Merchant) error {
	if o.ID == "" || o.OperatorID != m.OperatorID || o.MerchantID != m.ID || o.Money.Validate() != nil || o.AcceptedAt.IsZero() || !o.ExpiresAt.After(o.AcceptedAt) || o.Valuation.Validate(o.Money.Currency, o.AcceptedAt) != nil {
		return ErrInvalid
	}
	return nil
}

type Checkout struct {
	ProviderID     string `json:"provider_id"`
	State          string `json:"state"` // pending | requires_action | processing
	URL            string `json:"url,omitempty"`
	ClientSecret   string `json:"client_secret,omitempty"`
	PublishableKey string `json:"publishable_key,omitempty"`
	PaymentAddress string `json:"payment_address,omitempty"`
	Instructions   string `json:"instructions,omitempty"`
	PayMoney       *Money `json:"pay_money,omitempty"`
}
type Refund struct {
	ID                   string         `json:"id"`
	ExecutionID          string         `json:"execution_id,omitempty"`
	Order                Order          `json:"order"`
	Money                Money          `json:"money"`
	ProviderID           string         `json:"provider_id,omitempty"`
	USDAmount            ledger.Decimal `json:"usd_amount"`
	RefundValuation      Valuation      `json:"refund_valuation"`
	ValuedAt             time.Time      `json:"valued_at"`
	FXExecutionReference string         `json:"fx_execution_reference"`
}

func (r Refund) Validate(m Merchant) error {
	if r.ID == "" || r.Order.Validate(m) != nil || r.Money.Validate() != nil || r.Money.Currency != r.Order.Money.Currency || r.Money.Amount.Cmp(r.Order.Money.Amount) > 0 {
		return ErrInvalid
	}
	if r.USDAmount.Sign() <= 0 || !r.USDAmount.Valid() {
		return ErrInvalid
	}
	if r.Money.Currency == "USD" {
		if r.USDAmount.Cmp(r.Money.Amount) != 0 {
			return ErrInvalid
		}
	} else {
		if r.ValuedAt.IsZero() || r.FXExecutionReference == "" || r.RefundValuation.Validate(r.Money.Currency, r.ValuedAt) != nil {
			return ErrInvalid
		}
	}
	return nil
}

type RefundStatus struct {
	ProviderID string `json:"provider_id"`
	State      string `json:"state"`
	Fact       *Fact  `json:"fact,omitempty"`
}
type Webhook struct {
	Headers    http.Header
	Body       []byte
	ReceivedAt time.Time
}
type Hint struct {
	EventID           string     `json:"event_id"`
	Kind              string     `json:"kind"`
	OrderID           string     `json:"order_id,omitempty"`
	ReferenceID       string     `json:"reference_id,omitempty"`
	ProviderID        string     `json:"provider_id"`
	OccurredAt        *time.Time `json:"occurred_at,omitempty"`
	SignatureVerified bool       `json:"-"`
	Final             bool       `json:"final"`
}

// Fact's private proof prevents a JSON payload from declaring itself verified.
// Public fields are persisted evidence; construct facts only in a real driver or
// the explicitly authorized manual verification service.
type Fact struct {
	ID             string                   `json:"id"`
	Kind           string                   `json:"kind"` // receipt | refund | chargeback
	OperatorID     string                   `json:"operator_id"`
	MerchantID     string                   `json:"merchant_id"`
	Driver         string                   `json:"driver"`
	OrderID        string                   `json:"order_id"`
	ProviderID     string                   `json:"provider_id"`
	Money          Money                    `json:"money"`
	OccurredAt     *time.Time               `json:"occurred_at,omitempty"`
	ObservedAt     time.Time                `json:"observed_at"`
	Evidence       json.RawMessage          `json:"evidence"`
	ExecutionValue *USDValue                `json:"execution_value,omitempty"`
	ExternalRefund *ExternalRefundExecution `json:"external_refund,omitempty"`
	RefundSource   *RefundSource            `json:"refund_source,omitempty"`
	proof          bool
}

func (f Fact) Verified() bool {
	return f.proof && f.ID != "" && f.ProviderID != "" && f.OperatorID != "" && f.MerchantID != "" && f.Money.Validate() == nil && !f.ObservedAt.IsZero() && len(f.Evidence) > 0
}
func receiptFact(m Merchant, o Order, providerID string, money Money, at *time.Time, evidence any) (Fact, error) {
	if providerID == "" || money.Currency != o.Money.Currency || money.Validate() != nil {
		return Fact{}, ErrInvalid
	}
	raw, e := json.Marshal(evidence)
	if e != nil {
		return Fact{}, e
	}
	return Fact{ID: m.Driver + ":" + m.ID + ":receipt:" + providerID, Kind: "receipt", OperatorID: m.OperatorID, MerchantID: m.ID, Driver: m.Driver, OrderID: o.ID, ProviderID: providerID, Money: money, OccurredAt: at, ObservedAt: time.Now().UTC(), Evidence: raw, proof: true}, nil
}
func refundFact(m Merchant, r Refund, providerID string, money Money, at *time.Time, evidence any) (Fact, error) {
	f, e := receiptFact(m, r.Order, providerID, money, at, evidence)
	f.ID = m.Driver + ":" + m.ID + ":refund:" + providerID
	f.Kind = "refund"
	if e != nil {
		return f, e
	}
	var v USDValue
	if r.Money.Currency == "USD" {
		at := f.ObservedAt
		v, e = (Valuation{ID: "usd-native:" + f.ID, Currency: "USD", USDPerUnit: ledger.FromInt64(1), Source: "native_usd_refund", EffectiveAt: at.Add(-time.Second), ExpiresAt: at.Add(time.Second)}).Convert(money, at)
	} else {
		v, e = r.RefundValuation.Convert(money, r.ValuedAt)
	}
	if e != nil {
		return f, e
	}
	f.ExecutionValue = &v
	return f, nil
}

type Driver interface {
	Name() string
	Validate(Merchant) []string
	Create(context.Context, Merchant, Order) (Checkout, error)
	Lookup(context.Context, Merchant, Order) (Fact, error)
	Refund(context.Context, Merchant, Refund) (RefundStatus, error)
	LookupRefund(context.Context, Merchant, Refund) (RefundStatus, error)
	VerifyWebhook(context.Context, Merchant, Webhook) (Hint, error)
}
