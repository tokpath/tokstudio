package usage

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/tokpath/tokstudio/backend/internal/modelruntime"
	platformcrypto "github.com/tokpath/tokstudio/backend/internal/platform/crypto"
	"gorm.io/gorm"
)

func (s *Service) SubmitImage(ctx context.Context, key Key, businessKey string, input modelruntime.Request) (Request, error) {
	if s.Runtime == nil || s.ResultStore == nil {
		return Request{}, ErrNotReady
	}
	if !input.Operation.IsImage() || input.Stream {
		return Request{}, modelruntime.ErrUnsupported
	}
	input.Async = true
	request, _, err := s.Admit(ctx, key, businessKey, input)
	return request, err
}

// The accepted row is the durable task. Claiming execution clears its input
// and fences off retries before any request can reach the supplier.
func (s *Service) RunImageTask(ctx context.Context, request Request) error {
	if !request.AsyncImage || terminal(request.Status) {
		return nil
	}
	if request.Status != "accepted" {
		return s.reconcileImageTask(ctx, request)
	}
	if request.CancelRequested {
		_, err := s.failUndispatched(ctx, request.ID, "request_cancelled_before_dispatch")
		return err
	}
	if !s.wallNow().Before(request.UsageDeadline) {
		_, err := s.ResolveTimeout(ctx, request.ID)
		return err
	}
	var payload Request
	err := s.DB.WithContext(ctx).Select("async_input").Take(&payload, "id=? AND status='accepted'", request.ID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	plain, err := platformcrypto.Open(s.EncryptionKey, payload.AsyncInput)
	var input modelruntime.Request
	if err != nil || json.Unmarshal([]byte(plain), &input) != nil || input.ID != request.ID ||
		input.Operation != request.Operation || input.PublicModel != request.ModelID || !input.Async || input.Stream {
		_, err = s.failUndispatched(ctx, request.ID, "request_not_dispatched")
		return err
	}
	_, err = s.executeAdmitted(ctx, request, input, nil, false)
	if errors.Is(err, ErrInProgress) {
		return nil
	}
	return err
}

func (s *Service) reconcileImageTask(ctx context.Context, request Request) error {
	var attempt AttemptRecord
	err := s.DB.WithContext(ctx).Select("usage").Where("request_id=? AND state='completed' AND usage->>'reliable'='true'", request.ID).
		Order("started_at DESC").Take(&attempt).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	var measured modelruntime.Usage
	if err := json.Unmarshal(attempt.Usage, &measured); err != nil {
		return err
	}
	_, err = s.Settle(ctx, nil, request.ID, measured)
	return err
}
