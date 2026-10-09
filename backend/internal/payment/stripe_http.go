package payment

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var (
	stripeAPIBase = "https://api.stripe.com"
	stripeHTTP    = &http.Client{Timeout: 15 * time.Second}
)

const maxProviderBody = 1 << 20

func stripeDo(ctx context.Context, secret, method, path string, form url.Values) ([]byte, int, error) {
	return stripeDoKeyed(ctx, secret, method, path, form, "")
}

func stripeDoKeyed(ctx context.Context, secret, method, path string, form url.Values, idempotencyKey string) ([]byte, int, error) {
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(stripeAPIBase, "/")+path, body)
	if err != nil {
		return nil, 0, err
	}
	req.SetBasicAuth(secret, "")
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if strings.TrimSpace(idempotencyKey) != "" {
		req.Header.Set("Idempotency-Key", strings.TrimSpace(idempotencyKey))
	}
	resp, err := stripeHTTP.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxProviderBody))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return raw, resp.StatusCode, nil
}

func verifyStripeSignature(secret, header string, body []byte) bool {
	secret = strings.TrimSpace(secret)
	header = strings.TrimSpace(header)
	if secret == "" || header == "" {
		return false
	}
	var ts string
	var v1 []string
	for _, part := range strings.Split(header, ",") {
		k, val, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		switch k {
		case "t":
			ts = val
		case "v1":
			v1 = append(v1, val)
		}
	}
	if ts == "" || len(v1) == 0 {
		return false
	}
	unix, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return false
	}
	if d := time.Now().UTC().Unix() - unix; d > 300 || d < -30 {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = fmt.Fprintf(mac, "%s.%s", ts, body)
	want := hex.EncodeToString(mac.Sum(nil))
	for _, got := range v1 {
		if hmac.Equal([]byte(want), []byte(got)) {
			return true
		}
	}
	return false
}

func stripeWebhookEvent(payload map[string]any) *WebhookEvent {
	ev := &WebhookEvent{ExternalEventID: asString(payload["id"])}
	typ := asString(payload["type"])
	data, _ := payload["data"].(map[string]any)
	obj, _ := data["object"].(map[string]any)
	ev.TradeID = asString(obj["id"])
	if meta, ok := obj["metadata"].(map[string]any); ok {
		ev.OrderID = asString(meta["order_id"])
	}
	switch typ {
	case "payment_intent.succeeded", "charge.succeeded":
		ev.Status = StatusPaid
	case "checkout.session.completed", "checkout.session.async_payment_succeeded":
		if asString(obj["payment_status"]) == "paid" {
			ev.Status = StatusPaid
		}
		if ev.OrderID == "" {
			ev.OrderID = asString(obj["client_reference_id"])
		}
		ev.TradeID = stripeObjectID(obj["payment_intent"])
	case "payment_intent.payment_failed", "checkout.session.async_payment_failed", "charge.failed":
		ev.Status = StatusFailed
	case "refund.created", "refund.updated", "refund.failed", "charge.refund.updated":
		ev.CheckRefundAmount = true
		ev.TradeID = stripeObjectID(obj["payment_intent"])
		ev.RefundAmountMinor = callbackAmount(obj["amount"])
		ev.Currency = strings.ToUpper(asString(obj["currency"]))
		switch asString(obj["status"]) {
		case "succeeded":
			ev.Status = StatusRefunded
		case "pending", "requires_action":
			ev.Status = StatusRefunding
		case "failed", "canceled":
			ev.Status = StatusRefundFailed
		default:
			ev.Status = StatusRefundReview
		}
	case "charge.refunded":
		ev.CheckRefundAmount = true
		ev.TradeID = stripeObjectID(obj["payment_intent"])
		ev.OriginalAmountMinor = callbackAmount(obj["amount"])
		ev.RefundAmountMinor = callbackAmount(obj["amount_refunded"])
		ev.Currency = strings.ToUpper(asString(obj["currency"]))
		ev.Status = StatusRefundPartial
		if obj["refunded"] == true && ev.OriginalAmountMinor != nil && ev.RefundAmountMinor != nil && *ev.OriginalAmountMinor > 0 && *ev.RefundAmountMinor == *ev.OriginalAmountMinor {
			ev.Status = StatusRefunded
		}
	}
	if typ == "charge.succeeded" || typ == "charge.failed" {
		ev.TradeID = stripeObjectID(obj["payment_intent"])
	}
	return ev
}

func stripeObjectID(value any) string {
	if obj, ok := value.(map[string]any); ok {
		return asString(obj["id"])
	}
	return asString(value)
}

func decodeJSONMap(raw []byte) map[string]any {
	var payload map[string]any
	if json.Unmarshal(raw, &payload) != nil {
		return nil
	}
	return payload
}
