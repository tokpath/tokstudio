package payment

import (
	"context"
	"net/http"
)

// Adapter 是可插拔支付方式。新增 PayNow、本地转账、聚合网关时：
// 1. 新增 driver_xxx.go，实现本接口；
// 2. 在 newBuiltinRegistry() 里 Register 一行。
// Service / HTTP / 渠道台只认注册表，不再写死支付宝/微信/Stripe。
// 交互骨架对照 Sub2API 的 Provider：下单、查单、验签、退款；凭证 schema 由插件自己声明。
type Adapter interface {
	Spec() AdapterSpec
	Test(ctx context.Context, in TestInput) error
	CreateCheckout(ctx context.Context, in CheckoutRequest) (*CheckoutSession, error)
	ParseWebhook(ctx context.Context, in WebhookRequest) (*WebhookEvent, error)
	QueryOrder(ctx context.Context, in QueryRequest) (*QueryResult, error)
	Refund(ctx context.Context, in RefundRequest) (*RefundResult, error)
}

type AdapterKind string

const (
	KindOfficial AdapterKind = "official" // 官方直连（支付宝 / 微信 / Stripe）
	KindManual   AdapterKind = "manual"   // 人工入账
	KindLocal    AdapterKind = "local"    // 本地/地区支付，后续插拔
	KindGateway  AdapterKind = "gateway"  // 聚合网关，后续插拔
)

type CheckoutMode string

const (
	CheckoutQR       CheckoutMode = "qr"
	CheckoutRedirect CheckoutMode = "redirect"
	CheckoutElement  CheckoutMode = "element"
	CheckoutManual   CheckoutMode = "manual"
	CheckoutSandbox  CheckoutMode = "sandbox"
)

// AdapterSpec 是给渠道台/收银台用的静态描述，不含密钥。
type AdapterSpec struct {
	ID            string                `json:"id"`
	DisplayName   string                `json:"display_name"`
	Kind          AdapterKind           `json:"kind"`
	PayCurrency   string                `json:"pay_currency"`
	CheckoutMode  CheckoutMode          `json:"checkout_mode"`
	BrandColor    string                `json:"brand_color,omitempty"`
	AutoRenew     bool                  `json:"auto_renew_supported"`
	UserFacing    bool                  `json:"user_facing"`
	Credentials   []CredentialFieldView `json:"credentials"`
	MerchantKeys  []string              `json:"merchant_keys,omitempty"`
}

type TestInput struct {
	Credentials map[string]string
	Mode        string
}

type CheckoutRequest struct {
	Order       *OrderView
	Credentials map[string]string
	Mode        string
	PublicBase  string
	SignKey     string
}

type CheckoutSession struct {
	Mode           CheckoutMode   `json:"mode"`
	Sandbox        bool           `json:"sandbox"`
	WebhookURL     string         `json:"webhook_url"`
	AutoRenew      bool           `json:"auto_renew_supported"`
	QRCode         string         `json:"qr_code,omitempty"`
	RedirectURL    string         `json:"redirect_url,omitempty"`
	ClientSecret   string         `json:"client_secret,omitempty"`
	PublishableKey string         `json:"publishable_key,omitempty"`
	Payload        map[string]any `json:"payload,omitempty"`
}

type WebhookRequest struct {
	Adapter     string
	Headers     http.Header
	Body        []byte
	SignKey     string
	Credentials map[string]string
}

type WebhookEvent struct {
	ExternalEventID string
	OrderID         string
	Status          string
	TradeID         string
	MerchantID      string
	SignatureValid  bool
}

type QueryRequest struct {
	Order       *OrderView
	Credentials map[string]string
	Mode        string
}

type QueryResult struct {
	Status  string
	TradeID string
}

type RefundRequest struct {
	Order       *OrderView
	Credentials map[string]string
	Mode        string
}

type RefundResult struct {
	Status  string
	TradeID string
}
