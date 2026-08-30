package payment

import (
	"errors"
	"time"
)

var (
	ErrNotFound         = errors.New("payment order not found")
	ErrInvalidAdapter   = errors.New("unsupported payment adapter")
	ErrInvalidSignature = errors.New("invalid payment signature")
	ErrInvalidEvent     = errors.New("invalid payment event")
	ErrInvalidAmount    = errors.New("invalid payment amount")
	ErrChargeFailed     = errors.New("renewal charge failed")
	ErrNoAutoRenew      = errors.New("adapter cannot auto-renew")
	ErrOrderNotPending  = errors.New("payment order is not pending")
)

const (
	AdapterStripe = "stripe"
	AdapterAlipay = "alipay"
	AdapterWechat = "wechat"
	AdapterManual = "manual"

	PurposeSubscription = "subscription"
	PurposeWallet       = "wallet"
	PurposeRenewal      = "renewal"

	StatusPending  = "pending"
	StatusPaid     = "paid"
	StatusFailed   = "failed"
	StatusRefunded = "refunded"
	StatusExpired  = "expired"
)

type CreateOrderInput struct {
	UserID        string
	Adapter       string
	Purpose       string
	ReferenceType string
	ReferenceID   string
	AmountMinor   int64
	Currency      string
}

type OrderView struct {
	ID            string    `json:"id"`
	UserID        string    `json:"user_id"`
	Adapter       string    `json:"adapter"`
	Purpose       string    `json:"purpose"`
	ReferenceType string    `json:"reference_type,omitempty"`
	ReferenceID   string    `json:"reference_id,omitempty"`
	AmountMinor   int64     `json:"amount_minor"`
	Currency      string    `json:"currency"`
	Status        string    `json:"status"`
	TradeID       string    `json:"provider_trade_id,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

type EventView struct {
	ID              string `json:"id"`
	Adapter         string `json:"adapter"`
	ExternalEventID string `json:"external_event_id"`
	OrderID         string `json:"order_id,omitempty"`
	SignatureValid  bool   `json:"signature_valid"`
	Status          string `json:"status,omitempty"`
	Duplicate       bool   `json:"duplicate,omitempty"`
}

type CheckoutView struct {
	Order      *OrderView `json:"order"`
	Adapter    string     `json:"adapter"`
	Sandbox    bool       `json:"sandbox"`
	WebhookURL string     `json:"webhook_url"`
	AutoRenew  bool       `json:"auto_renew_supported"`
}
