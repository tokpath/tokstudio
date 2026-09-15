package payment

import (
	"errors"
	"time"
)

var (
	ErrNotFound           = errors.New("payment order not found")
	ErrInvalidAdapter     = errors.New("unsupported payment adapter")
	ErrInvalidSignature   = errors.New("invalid payment signature")
	ErrInvalidEvent       = errors.New("invalid payment event")
	ErrInvalidAmount      = errors.New("invalid payment amount")
	ErrChargeFailed       = errors.New("renewal charge failed")
	ErrNoAutoRenew        = errors.New("adapter cannot auto-renew")
	ErrOrderNotPending    = errors.New("payment order is not pending")
	ErrInstanceNotFound   = errors.New("payment instance not found")
	ErrInstanceIncomplete = errors.New("payment instance is incomplete")
	ErrAdapterDisabled    = errors.New("payment adapter is disabled")
	ErrOnlineDisabled     = errors.New("channel online payments are disabled")
	ErrNotTested          = errors.New("payment instance has not passed connectivity test")
	ErrMethodUnavailable  = errors.New("payment method is not available for this channel")
	ErrRefundDisabled     = errors.New("refunds are disabled for this instance")
	ErrProviderFailed     = errors.New("payment provider request failed")
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

	ModeSandbox = "sandbox"
	ModeLive    = "live"

	LaneNone        = "none"
	LaneConfiguring = "configuring"
	LaneSandbox     = "sandbox"
	LaneLive        = "live"
	LaneDisabled    = "disabled"
)

type CreateOrderInput struct {
	UserID        string
	ChannelOrgID  string
	Adapter       string
	Purpose       string
	ReferenceType string
	ReferenceID   string
	AmountMinor   int64
	Currency      string
	PayMajor      int64
}

type ListOrdersFilter struct {
	Status       string
	ChannelOrgID string
	Adapter      string
	Query        string
}

type OrderView struct {
	ID            string     `json:"id"`
	UserID        string     `json:"user_id"`
	ChannelOrgID  string     `json:"channel_org_id,omitempty"`
	Adapter       string     `json:"adapter"`
	Purpose       string     `json:"purpose"`
	ReferenceType string     `json:"reference_type,omitempty"`
	ReferenceID   string     `json:"reference_id,omitempty"`
	AmountMinor   int64      `json:"amount_minor"`
	CreditMinor   int64      `json:"credit_minor"`
	Currency      string     `json:"currency"`
	Status        string     `json:"status"`
	TradeID       string     `json:"provider_trade_id,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	FulfilledAt   *time.Time `json:"fulfilled_at,omitempty"`
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
	Order          *OrderView       `json:"order"`
	Adapter        string           `json:"adapter"`
	Sandbox        bool             `json:"sandbox"`
	WebhookURL     string           `json:"webhook_url"`
	AutoRenew      bool             `json:"auto_renew_supported"`
	Mode           CheckoutMode     `json:"mode,omitempty"`
	QRCode         string           `json:"qr_code,omitempty"`
	RedirectURL    string           `json:"redirect_url,omitempty"`
	ClientSecret   string           `json:"client_secret,omitempty"`
	PublishableKey string           `json:"publishable_key,omitempty"`
	Payload        map[string]any   `json:"payload,omitempty"`
	Session        *CheckoutSession `json:"session,omitempty"`
}

type CredentialFieldView struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Secret   bool   `json:"secret"`
	Required bool   `json:"required"`
}

type InstanceInput struct {
	Adapter         string            `json:"adapter"`
	Name            string            `json:"name"`
	Mode            string            `json:"mode"`
	Enabled         *bool             `json:"enabled"`
	RefundEnabled   *bool             `json:"refund_enabled"`
	MinAmountMinor  *int64            `json:"min_amount_minor"`
	MaxAmountMinor  *int64            `json:"max_amount_minor"`
	DailyLimitMinor *int64            `json:"daily_limit_minor"`
	Credentials     map[string]string `json:"credentials"`
}

type InstanceView struct {
	ID              string            `json:"id"`
	ChannelOrgID    string            `json:"channel_org_id"`
	Adapter         string            `json:"adapter"`
	Name            string            `json:"name"`
	Mode            string            `json:"mode"`
	State           string            `json:"state"`
	Enabled         bool              `json:"enabled"`
	RefundEnabled   bool              `json:"refund_enabled"`
	SortOrder       int               `json:"sort_order"`
	MinAmountMinor  *int64            `json:"min_amount_minor,omitempty"`
	MaxAmountMinor  *int64            `json:"max_amount_minor,omitempty"`
	DailyLimitMinor *int64            `json:"daily_limit_minor,omitempty"`
	MissingFields   []string          `json:"missing_fields,omitempty"`
	PublicFields    map[string]string `json:"public_fields,omitempty"`
	FieldsSet       []string          `json:"fields_set,omitempty"`
	WebhookURL      string            `json:"webhook_url,omitempty"`
	LastTestedAt    *time.Time        `json:"last_tested_at,omitempty"`
	LastTestOK      *bool             `json:"last_test_ok,omitempty"`
	LastPaidAt      *time.Time        `json:"last_paid_at,omitempty"`
	CreatedAt       time.Time         `json:"created_at"`
}

type SettingsInput struct {
	QuickAmounts     []int64 `json:"quick_amounts"`
	MinPayMajor      *int64  `json:"min_pay_major"`
	MaxPayMajor      *int64  `json:"max_pay_major"`
	TimeoutMinutes   *int    `json:"timeout_minutes"`
	HelpText         *string `json:"help_text"`
	HelpImageURL     *string `json:"help_image_url"`
	ProductPrefix    *string `json:"product_prefix"`
	ProductSuffix    *string `json:"product_suffix"`
	MaxPendingOrders *int    `json:"max_pending_orders"`
	CancelRateLimit  *int    `json:"cancel_rate_limit"`
	FeeBPS           *int    `json:"fee_bps"`
	OnlineDisabled   *bool   `json:"online_disabled"`
}

type SettingsView struct {
	ChannelOrgID     string  `json:"channel_org_id"`
	QuickAmounts     []int64 `json:"quick_amounts"`
	MinPayMajor      int64   `json:"min_pay_major"`
	MaxPayMajor      int64   `json:"max_pay_major"`
	TimeoutMinutes   int     `json:"timeout_minutes"`
	HelpText         string  `json:"help_text"`
	HelpImageURL     string  `json:"help_image_url"`
	ProductPrefix    string  `json:"product_prefix"`
	ProductSuffix    string  `json:"product_suffix"`
	MaxPendingOrders int     `json:"max_pending_orders"`
	CancelRateLimit  int     `json:"cancel_rate_limit"`
	FeeBPS           int     `json:"fee_bps"`
	OnlineDisabled   bool    `json:"online_disabled"`
	IssueRatioBPS    int64   `json:"issue_ratio_bps"`
	FenPerUSD        int64   `json:"fen_per_usd"`
}

type LaneView struct {
	Adapter       string                `json:"adapter"`
	DisplayName   string                `json:"display_name"`
	Kind          AdapterKind           `json:"kind"`
	BrandColor    string                `json:"brand_color,omitempty"`
	CheckoutMode  CheckoutMode          `json:"checkout_mode"`
	PayCurrency   string                `json:"pay_currency"`
	State         string                `json:"state"`
	InstanceCount int                   `json:"instance_count"`
	MissingFields []string              `json:"missing_fields,omitempty"`
	LastPaidAt    *time.Time            `json:"last_paid_at,omitempty"`
	AutoRenew     bool                  `json:"auto_renew_supported"`
	Schema        []CredentialFieldView `json:"schema"`
}

type OverviewView struct {
	ChannelOrgID   string     `json:"channel_org_id"`
	OnlineDisabled bool       `json:"online_disabled"`
	IssueRatioBPS  int64      `json:"issue_ratio_bps"`
	FenPerUSD      int64      `json:"fen_per_usd"`
	CallbackOrigin string     `json:"callback_origin"`
	Lanes          []LaneView `json:"lanes"`
}

type CheckoutMethodView struct {
	Adapter      string       `json:"adapter"`
	DisplayName  string       `json:"display_name"`
	Name         string       `json:"name"`
	Sandbox      bool         `json:"sandbox"`
	AutoRenew    bool         `json:"auto_renew_supported"`
	BrandColor   string       `json:"brand_color,omitempty"`
	CheckoutMode CheckoutMode `json:"checkout_mode"`
	PayCurrency  string       `json:"pay_currency"`
}

type UserCheckoutView struct {
	ChannelOrgID   string               `json:"channel_org_id"`
	OnlineDisabled bool                 `json:"online_disabled"`
	EmptyReason    string               `json:"empty_reason,omitempty"`
	Methods        []CheckoutMethodView `json:"methods"`
	Settings       *SettingsView        `json:"settings"`
	HelpText       string               `json:"help_text,omitempty"`
	HelpImageURL   string               `json:"help_image_url,omitempty"`
}

type QuoteView struct {
	Adapter       string `json:"adapter"`
	PayMajor      int64  `json:"pay_major"`
	PayCurrency   string `json:"pay_currency"`
	PayMinor      int64  `json:"pay_minor"`
	FeeMinor      int64  `json:"fee_minor"`
	FeeBPS        int64  `json:"fee_bps"`
	WalletMinor   int64  `json:"wallet_minor"`
	CreditMinor   int64  `json:"credit_minor"`
	IssueRatioBPS int64  `json:"issue_ratio_bps"`
	FenPerUSD     int64  `json:"fen_per_usd"`
	AutoRenew     bool   `json:"auto_renew_supported"`
}

type AdapterFlagView struct {
	Adapter string `json:"adapter"`
	Enabled bool   `json:"enabled"`
}
