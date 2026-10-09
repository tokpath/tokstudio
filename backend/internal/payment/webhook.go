package payment

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/platform/id"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func callbackAmount(value any) *int64 {
	n, ok := value.(float64)
	if !ok || n < 0 || math.IsNaN(n) || math.IsInf(n, 0) || n >= math.MaxInt64 || math.Trunc(n) != n {
		return nil
	}
	amount := int64(n)
	return &amount
}

func sameCallbackPayload(a, b []byte) bool {
	var av, bv any
	if json.Unmarshal(a, &av) != nil || json.Unmarshal(b, &bv) != nil {
		return false
	}
	ac, _ := json.Marshal(av)
	bc, _ := json.Marshal(bv)
	return bytes.Equal(ac, bc)
}

// Receiving a valid event does not prove that local credit reversal succeeded.
// Every retry applies the original event again until the local operation commits.
func (s *Service) HandleWebhook(ctx context.Context, adapter string, headers http.Header, body []byte) (*EventView, error) {
	plugin, ok := s.plugin(adapter)
	if !ok {
		return nil, ErrInvalidAdapter
	}
	if headers == nil {
		headers = http.Header{}
	}
	parsed, err := s.parseWebhookEvent(ctx, plugin, adapter, headers, body)
	if err != nil {
		return nil, err
	}
	if parsed == nil || parsed.ExternalEventID == "" {
		return nil, ErrInvalidEvent
	}
	if !parsed.SignatureValid {
		return nil, ErrInvalidSignature
	}
	// Earlier Stripe refunds may have no metadata; the original payment intent
	// is still a safe lookup after verifying this brand's own webhook secret.
	if parsed.OrderID == "" && parsed.TradeID != "" {
		var matches []orderRow
		if err := s.db.WithContext(ctx).Where("adapter = ? AND provider_trade_id = ?", adapter, parsed.TradeID).Limit(2).Find(&matches).Error; err != nil {
			return nil, err
		}
		if len(matches) == 1 {
			parsed.OrderID = matches[0].ID
		}
	}
	var order *OrderView
	if parsed.OrderID != "" {
		order, err = s.GetOrder(ctx, parsed.OrderID, "")
		if err != nil {
			return nil, err
		}
		if order.Adapter != adapter {
			return nil, ErrInvalidEvent
		}
		inst := s.firstReadyInstance(ctx, order.PayeeChannelOrgID, adapter)
		if inst == nil && adapter != AdapterManual {
			return nil, ErrMethodUnavailable
		}
		creds := map[string]string{}
		if inst != nil {
			creds, err = openCredentials(s.signKey, inst.CredentialsCiphertext)
			if err != nil {
				return nil, err
			}
		}
		owned, err := plugin.ParseWebhook(ctx, WebhookRequest{Adapter: adapter, Headers: headers, Body: body, SignKey: s.signKey, Credentials: creds})
		if err != nil || owned == nil || !owned.SignatureValid {
			return nil, ErrInvalidSignature
		}
		if owned.OrderID != order.ID && !(owned.OrderID == "" && owned.TradeID != "" && owned.TradeID == order.TradeID) {
			return nil, ErrInvalidEvent
		}
		if parsed.CheckRefundAmount {
			expected := order.AmountMinor
			if adapter == AdapterStripe {
				expected = stripeCents(expected)
			}
			if strings.ToUpper(order.Currency) != parsed.Currency || parsed.RefundAmountMinor == nil || *parsed.RefundAmountMinor <= 0 || *parsed.RefundAmountMinor > expected || (parsed.OriginalAmountMinor != nil && *parsed.OriginalAmountMinor != expected) {
				parsed.Status = StatusRefundReview
			} else if parsed.Status == StatusRefunded && *parsed.RefundAmountMinor < expected {
				parsed.Status = StatusRefundPartial
			}
		}
	}
	view := &EventView{Adapter: adapter, ExternalEventID: parsed.ExternalEventID, OrderID: parsed.OrderID, SignatureValid: true, Status: parsed.Status}
	var applied bool
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", "payment-event:"+parsed.ExternalEventID).Error; err != nil {
			return err
		}
		var existing eventRow
		err := tx.Where("external_event_id = ?", parsed.ExternalEventID).First(&existing).Error
		if err == nil {
			if existing.Adapter != adapter || !existing.SignatureValid || !sameCallbackPayload(existing.PayloadJSON, body) || (existing.OrderID != nil && *existing.OrderID != parsed.OrderID) {
				return ErrInvalidEvent
			}
			view.ID, view.Duplicate = existing.ID, true
			applied = existing.AppliedAt != nil
			if existing.Status != "" {
				view.Status, parsed.Status = existing.Status, existing.Status
			}
			updates := map[string]any{"status": parsed.Status}
			if existing.ProviderTradeID == "" {
				updates["provider_trade_id"] = parsed.TradeID
			}
			if existing.OrderID == nil && parsed.OrderID != "" {
				updates["order_id"] = parsed.OrderID
			}
			return tx.Model(&existing).Updates(updates).Error
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		row := eventRow{ID: id.New("pev"), Adapter: adapter, ExternalEventID: parsed.ExternalEventID, SignatureValid: true, ProviderTradeID: parsed.TradeID, PayloadJSON: body, Status: parsed.Status, ProcessedAt: time.Now().UTC()}
		if parsed.OrderID != "" {
			row.OrderID = &parsed.OrderID
		}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		view.ID = row.ID
		_, err = s.outbox.EnqueueTx(tx, "payment.webhook.received", "payment_event", row.ID, map[string]any{"adapter": adapter, "external_event_id": parsed.ExternalEventID, "status": parsed.Status})
		return err
	})
	if err != nil || applied {
		return view, err
	}
	if parsed.OrderID == "" {
		err := s.db.WithContext(ctx).Model(&eventRow{}).Where("id = ?", view.ID).Update("processing_error", "order_not_matched").Error
		return view, err
	}
	err = s.applyWebhook(ctx, parsed, order)
	updates := map[string]any{"processing_error": ""}
	if err != nil {
		updates["processing_error"] = "local_application_failed"
	} else {
		updates["applied_at"] = time.Now().UTC()
	}
	if recordErr := s.db.WithContext(ctx).Model(&eventRow{}).Where("id = ?", view.ID).Updates(updates).Error; err == nil {
		err = recordErr
	}
	return view, err
}

func (s *Service) applyWebhook(ctx context.Context, ev *WebhookEvent, order *OrderView) error {
	var applyErr error
	switch ev.Status {
	case StatusPaid:
		return s.markPaid(ctx, ev.OrderID, ev.TradeID)
	case StatusFailed:
		return s.db.WithContext(ctx).Model(&orderRow{}).Where("id = ? AND status = ?", ev.OrderID, StatusPending).Updates(map[string]any{"status": StatusFailed, "updated_at": time.Now().UTC()}).Error
	case StatusRefunded:
		applyErr = s.markRefunded(ctx, ev.OrderID)
	case StatusRefunding, StatusRefundFailed, StatusRefundPartial, StatusRefundReview:
	default:
		return nil
	}
	// Keep actual provider facts even when local wallet/entitlement reversal fails.
	recordErr := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row orderRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", ev.OrderID).First(&row).Error; err != nil {
			return err
		}
		if row.RefundStatus == StatusRefunded && ev.Status != StatusRefunded {
			return nil
		}
		if ev.Status == StatusRefunding && row.RefundStatus != "" && row.RefundStatus != StatusRefunding {
			return nil
		}
		row.RefundStatus = ev.Status
		if ev.Status == StatusRefunded {
			row.RefundAmountMinor = &row.AmountMinor
		}
		if ev.Status == StatusRefundPartial && ev.RefundAmountMinor != nil && order != nil {
			amount := *ev.RefundAmountMinor
			if order.Adapter == AdapterStripe && order.Currency == "USD" {
				amount *= 10000
			}
			row.RefundAmountMinor = &amount
		}
		row.UpdatedAt = time.Now().UTC()
		return tx.Save(&row).Error
	})
	if applyErr != nil {
		return applyErr
	}
	return recordErr
}

// Retry only financial application of an already verified, normalized callback.
// This never initiates another external payment or refund.
func (s *Service) RetryUnappliedEvents(ctx context.Context) (int, error) {
	var rows []eventRow
	if err := s.db.WithContext(ctx).Where("signature_valid = true AND applied_at IS NULL AND order_id IS NOT NULL AND status <> '' AND processing_error = ?", "local_application_failed").Order("processed_at,id").Limit(50).Find(&rows).Error; err != nil {
		return 0, err
	}
	n := 0
	for _, row := range rows {
		order, err := s.GetOrder(ctx, *row.OrderID, "")
		if err != nil {
			continue
		}
		ev := &WebhookEvent{OrderID: order.ID, Status: row.Status, TradeID: row.ProviderTradeID}
		// Partial/failed facts have already been retained separately; only a full
		// refund or payment success can have a failed financial application.
		if ev.Status != StatusPaid && ev.Status != StatusRefunded {
			continue
		}
		if err := s.applyWebhook(ctx, ev, order); err != nil {
			continue
		}
		if err := s.db.WithContext(ctx).Model(&eventRow{}).Where("id = ?", row.ID).Updates(map[string]any{"applied_at": time.Now().UTC(), "processing_error": ""}).Error; err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}
