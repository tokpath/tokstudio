package media

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/platform/id"
)

const (
	callbackMaxAttempts = 8
	callbackTimeout     = 10 * time.Second
	CallbackPending     = "pending"
	CallbackDelivered   = "delivered"
	CallbackFailed      = "failed"
)

var callbackHTTP = &http.Client{Timeout: callbackTimeout}

func (s *Service) queueCustomerCallback(ctx context.Context, job jobRow) {
	if strings.TrimSpace(job.CallbackURL) == "" {
		return
	}
	eventID := strings.TrimSpace(job.CallbackEventID)
	if eventID == "" {
		eventID = id.New("evt")
	}
	now := time.Now().UTC()
	_ = s.db.WithContext(ctx).Model(&jobRow{}).
		Where("id = ? AND (callback_status IS NULL OR callback_status = '' OR callback_status = ?)", job.ID, CallbackPending).
		Updates(map[string]any{
			"callback_event_id": eventID,
			"callback_status":   CallbackPending,
			"callback_next_at":  now,
			"updated_at":        now,
		}).Error
}

func (s *Service) DeliverCustomerCallbacks(ctx context.Context) {
	now := time.Now().UTC()
	var jobs []jobRow
	if err := s.db.WithContext(ctx).
		Where("callback_status = ? AND callback_url <> '' AND (callback_next_at IS NULL OR callback_next_at <= ?)", CallbackPending, now).
		Order("callback_next_at ASC").Limit(20).Find(&jobs).Error; err != nil {
		return
	}
	for _, job := range jobs {
		s.deliverOne(ctx, job)
	}
}

func (s *Service) deliverOne(ctx context.Context, job jobRow) {
	claim := s.db.WithContext(ctx).Model(&jobRow{}).
		Where("id = ? AND callback_status = ?", job.ID, CallbackPending).
		Updates(map[string]any{"callback_attempts": job.CallbackAttempts + 1, "updated_at": time.Now().UTC()})
	if claim.Error != nil || claim.RowsAffected == 0 {
		return
	}
	job.CallbackAttempts++
	if job.CallbackEventID == "" {
		job.CallbackEventID = id.New("evt")
		_ = s.db.WithContext(ctx).Model(&jobRow{}).Where("id = ?", job.ID).Update("callback_event_id", job.CallbackEventID).Error
	}
	err := s.postCustomerCallback(ctx, job)
	now := time.Now().UTC()
	if err == nil {
		_ = s.db.WithContext(ctx).Model(&jobRow{}).Where("id = ?", job.ID).Updates(map[string]any{
			"callback_status":     CallbackDelivered,
			"callback_last_error": "",
			"callback_next_at":    nil,
			"updated_at":          now,
		}).Error
		return
	}
	updates := map[string]any{
		"callback_last_error": truncate([]byte(err.Error()), 240),
		"updated_at":          now,
	}
	if job.CallbackAttempts >= callbackMaxAttempts {
		updates["callback_status"] = CallbackFailed
		updates["callback_next_at"] = nil
	} else {
		delay := callbackTimeout * time.Duration(1<<min(job.CallbackAttempts-1, 6))
		if delay < 15*time.Second {
			delay = 15 * time.Second
		}
		next := now.Add(delay)
		updates["callback_status"] = CallbackPending
		updates["callback_next_at"] = next
	}
	_ = s.db.WithContext(ctx).Model(&jobRow{}).Where("id = ?", job.ID).Updates(updates).Error
}

func (s *Service) postCustomerCallback(ctx context.Context, job jobRow) error {
	usage := map[string]int{}
	_ = json.Unmarshal(job.UsageJSON, &usage)
	payload, err := json.Marshal(map[string]any{
		"event_id": job.CallbackEventID,
		"job_id":   job.ID,
		"status":   job.Status,
		"usage":    usage,
		"error":    deref(job.ErrorCode),
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, job.CallbackURL, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tokenhub-Signature", s.store.CallbackSign(job.CallbackEventID, job.ID))
	resp, err := callbackHTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode >= 300 {
		return fmt.Errorf("callback http %d", resp.StatusCode)
	}
	return nil
}

func deref(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
