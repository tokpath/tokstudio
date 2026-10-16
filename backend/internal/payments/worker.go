package payments

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type InboxAttempt struct {
	MerchantID string `json:"merchant_id"`
	EventID    string `json:"event_id"`
	State      string `json:"state"`
	ErrorCode  string `json:"error_code,omitempty"`
}

// RunPending is a bounded worker tick with durable leases and exponential
// backoff. A crash can only cause a repeated read/verification; fact recording
// remains unique in PostgreSQL. It never automatically creates another payment.
func (s *Service) RunPending(ctx context.Context, limit int) ([]InboxAttempt, error) {
	if limit < 1 || limit > 100 {
		limit = 20
	}
	var jobs []struct {
		MerchantID, EventID string
		Attempts            int
	}
	e := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Scheduling uses PostgreSQL's clock, as the inbox's initial due time
		// does. Host/DB clock skew must not hide a newly persisted callback.
		if e := tx.Table("payments_webhook_inbox").Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).Where("(state='pending' AND next_attempt_at<=clock_timestamp()) OR (state='processing' AND lease_until<clock_timestamp())").Order("received_at").Limit(limit).Find(&jobs).Error; e != nil {
			return e
		}
		for _, job := range jobs {
			if e := tx.Table("payments_webhook_inbox").Where("merchant_id=? AND event_id=?", job.MerchantID, job.EventID).Updates(map[string]any{"state": "processing", "lease_until": gorm.Expr("clock_timestamp()+interval '30 seconds'")}).Error; e != nil {
				return e
			}
		}
		return nil
	})
	if e != nil {
		return nil, e
	}
	out := []InboxAttempt{}
	for _, job := range jobs {
		work, cancel := context.WithTimeout(ctx, 20*time.Second)
		_, e := s.ProcessInbox(work, job.MerchantID, job.EventID)
		cancel()
		attempt := InboxAttempt{MerchantID: job.MerchantID, EventID: job.EventID, State: "processed"}
		if e != nil {
			code := "verification_pending"
			switch {
			case errors.Is(e, ErrOutcomeUnknown):
				code = "provider_unavailable"
			case errors.Is(e, ErrConflict):
				code = "evidence_conflict"
			case errors.Is(e, ErrSignature):
				code = "signature_invalid"
			case errors.Is(e, ErrUnsupported):
				code = "manual_verification_required"
			}
			attempt.State = "pending"
			attempt.ErrorCode = code
			power := job.Attempts
			if power > 9 {
				power = 9
			}
			backoff := 5 * time.Second * time.Duration(1<<power)
			if backoff > time.Hour {
				backoff = time.Hour
			}
			if dbErr := s.DB.WithContext(ctx).Table("payments_webhook_inbox").Where("merchant_id=? AND event_id=? AND state='processing'", job.MerchantID, job.EventID).Updates(map[string]any{"state": "pending", "attempts": job.Attempts + 1, "next_attempt_at": gorm.Expr("clock_timestamp() + make_interval(secs => ?)", int64(backoff/time.Second)), "lease_until": nil, "error_code": code}).Error; dbErr != nil {
				return out, dbErr
			}
		}
		out = append(out, attempt)
	}
	facts, e := s.RunPendingFacts(ctx, limit)
	if e != nil {
		return out, e
	}
	out = append(out, facts...)
	return out, nil
}
