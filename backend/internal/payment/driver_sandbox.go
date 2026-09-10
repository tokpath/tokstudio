package payment

import (
	"context"
	"encoding/json"
	"strings"
)

// sandboxDriver 是官方通道在 Mode!=live 或缺凭证时的 HMAC 实现。
// 验签格式与现有 M5 测试一致：HMAC(event_id|order_id|status)。
type sandboxDriver struct{}

func (sandboxDriver) Test(_ context.Context, in TestInput) error {
	if len(in.Credentials) == 0 {
		return ErrInstanceIncomplete
	}
	return nil
}

func (sandboxDriver) sandboxCheckout(spec AdapterSpec, in CheckoutRequest) *CheckoutSession {
	base := strings.TrimRight(in.PublicBase, "/")
	mode := spec.CheckoutMode
	if mode == "" {
		mode = CheckoutSandbox
	}
	return &CheckoutSession{
		Mode: mode, Sandbox: in.Mode != ModeLive,
		WebhookURL: WebhookURL(base, spec.ID),
		AutoRenew:  spec.AutoRenew,
		Payload:    map[string]any{"hint": "sandbox"},
	}
}

func (sandboxDriver) ParseWebhook(_ context.Context, in WebhookRequest) (*WebhookEvent, error) {
	var payload map[string]any
	if err := json.Unmarshal(in.Body, &payload); err != nil {
		return nil, ErrInvalidEvent
	}
	eventID := asString(payload["event_id"])
	orderID := asString(payload["order_id"])
	status := asString(payload["status"])
	tradeID := asString(payload["trade_id"])
	if eventID == "" || status == "" {
		return nil, ErrInvalidEvent
	}
	sig := ""
	if in.Headers != nil {
		sig = in.Headers.Get("X-Tokenhub-Payment-Signature")
	}
	valid := VerifyWebhook(in.SignKey, eventID, orderID, status, sig)
	return &WebhookEvent{
		ExternalEventID: eventID, OrderID: orderID, Status: status, TradeID: tradeID,
		MerchantID: asString(payload["app_id"]) + asString(payload["mch_id"]),
		SignatureValid: valid,
	}, nil
}

func (sandboxDriver) QueryOrder(_ context.Context, in QueryRequest) (*QueryResult, error) {
	if in.Order == nil {
		return nil, ErrNotFound
	}
	return &QueryResult{Status: in.Order.Status, TradeID: in.Order.TradeID}, nil
}

func (sandboxDriver) Refund(_ context.Context, in RefundRequest) (*RefundResult, error) {
	if in.Order == nil {
		return nil, ErrNotFound
	}
	return &RefundResult{Status: StatusRefunded, TradeID: in.Order.TradeID}, nil
}

func merchantFrom(creds map[string]string, keys ...string) string {
	for _, k := range keys {
		if v := strings.TrimSpace(creds[k]); v != "" {
			return v
		}
	}
	return ""
}
