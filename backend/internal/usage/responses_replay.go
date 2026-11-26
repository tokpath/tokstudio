package usage

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/modelruntime"
	platformcrypto "github.com/tokpath/tokstudio/backend/internal/platform/crypto"
	"github.com/tokpath/tokstudio/backend/internal/platform/objectstore"
)

type responseTurnOutcome struct {
	completedAt time.Time
	stored      bool
}

func responseReplayKey(requestID string) string {
	return "tokenhub:usage:responses-replay:" + requestID
}

func (s *Service) finishResponseTurn(ctx context.Context, request Request, result modelruntime.Result) (Request, error) {
	completedAt, err := s.businessTime(s.DB.WithContext(ctx))
	if err != nil {
		return Request{}, err
	}
	completedAt = completedAt.Truncate(time.Millisecond)
	// Secure the result before taking the settlement locks. Use the same end
	// timestamp for object storage, the fallback cache and the settled turn.
	storeErr := s.storeResponseTurn(ctx, request, result, completedAt)
	outcome := responseTurnOutcome{completedAt: completedAt, stored: storeErr == nil}
	settled, err := s.settle(ctx, nil, request.ID, result.Usage, &outcome)
	if err != nil {
		return settled, err
	}
	return settled, storeErr
}

func (s *Service) storeResponseTurn(ctx context.Context, request Request, result modelruntime.Result, completedAt time.Time) error {
	if !replayableResponseOutput(result.Output) {
		return objectstore.ErrUnavailable
	}
	objectCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	expiresAt := completedAt.Add(shortResultRetention)
	err := s.storeTemporaryResult(objectCtx, request, result, expiresAt)
	cancel()
	if err == nil {
		return nil
	}
	if s.ResponseReplayCache == nil || errors.Is(err, ErrConflict) || !expiresAt.After(s.wallNow()) {
		return objectstore.ErrUnavailable
	}
	// Bind the encrypted fallback to this request, even if a cache key is
	// accidentally reused. The database still gates access and expiration.
	sealed, err := platformcrypto.Seal(s.EncryptionKey, request.ID+":"+string(result.Output))
	if err != nil {
		return objectstore.ErrUnavailable
	}
	cacheCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := s.ResponseReplayCache.Do(cacheCtx, "SET", responseReplayKey(request.ID), sealed, "PXAT", expiresAt.UnixMilli()).Err(); err != nil {
		return objectstore.ErrUnavailable
	}
	return nil
}

func (s *Service) readResponseTurn(ctx context.Context, request Request) (modelruntime.Result, error) {
	if !s.withinResponseReplayPeriod(request) {
		return modelruntime.Result{}, ErrResultUnavailable
	}
	result, err := s.readRequestResult(ctx, request)
	if errors.Is(err, ErrResultUnavailable) || errors.Is(err, ErrNotReady) {
		err = objectstore.ErrUnavailable
	}
	return result, err
}

func (s *Service) withinResponseReplayPeriod(request Request) bool {
	return request.Status == "completed" && request.CompletedAt != nil && request.CompletedAt.Add(shortResultRetention).After(s.wallNow())
}

func (s *Service) canReadResponseReplay(request Request) bool {
	return s.ResponseReplayCache != nil && isResponseTurn(request) && s.withinResponseReplayPeriod(request)
}

func isResponseTurn(request Request) bool {
	return request.Operation == modelruntime.Responses && request.Streaming && strings.HasPrefix(request.BusinessKey, "responses-ws:")
}

func (s *Service) readCachedResponseTurn(ctx context.Context, requestID string) (modelruntime.Result, bool) {
	cacheCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	sealed, err := s.ResponseReplayCache.Get(cacheCtx, responseReplayKey(requestID)).Result()
	if err != nil {
		return modelruntime.Result{}, false
	}
	raw, err := platformcrypto.Open(s.EncryptionKey, sealed)
	output, matches := strings.CutPrefix(raw, requestID+":")
	if err != nil || !matches || !replayableResponseOutput([]byte(output)) {
		return modelruntime.Result{}, false
	}
	return modelruntime.Result{State: "completed", ContentType: "application/json", Output: json.RawMessage(output)}, true
}

func replayableResponseOutput(output []byte) bool {
	var event struct {
		Type     string `json:"type"`
		Response *struct {
			ID string `json:"id"`
		} `json:"response"`
	}
	return len(output) <= 32<<20 && json.Unmarshal(output, &event) == nil && event.Response != nil && event.Response.ID != "" &&
		(event.Type == "response.completed" || event.Type == "response.incomplete")
}
