package usage

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"github.com/tokpath/tokstudio/backend/internal/modelruntime"
	"gorm.io/gorm"
)

// RefreshAsync retrieves only the original provider task. It never resubmits a
// generation when submission, polling or a previous worker outcome is unknown.
func (s *Service) RefreshAsync(ctx context.Context, id string) (Request, error) {
	var request Request
	if e := s.DB.WithContext(ctx).First(&request, "id=?", id).Error; e != nil {
		return request, e
	}
	completed := request.Status == "completed"
	if terminal(request.Status) && !completed {
		return request, nil
	}
	if completed {
		if request.Operation != modelruntime.Video || request.CompletedAt == nil || !request.CompletedAt.Add(s.ResultRetention).After(s.wallNow()) {
			return request, nil
		}
		var n int64
		if e := s.DB.WithContext(ctx).Model(&resultRow{}).Where("request_id=? AND state='ready' AND expires_at>?", id, s.wallNow()).Count(&n).Error; e != nil {
			return request, e
		}
		if n > 0 {
			return request, nil
		}
	}
	if !completed && !s.wallNow().Before(request.UsageDeadline) {
		return s.ResolveTimeout(ctx, id)
	}
	if request.Operation != modelruntime.Video {
		return request, ErrInProgress
	}
	var row AttemptRecord
	if e := s.DB.WithContext(ctx).Where("request_id=? AND task_id<>''", id).Order("started_at DESC").First(&row).Error; e != nil {
		return request, e
	}
	attempt, input, e := s.lifecycleAttempt(request, row, modelruntime.VideoRetrieve)
	if e != nil {
		return request, e
	}
	result, e := s.Runtime.Execute(ctx, attempt, input, nil)
	recordCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	if !completed {
		if recordErr := s.RecordAttempt(recordCtx, id, row.ID, result); recordErr != nil {
			return request, recordErr
		}
	}
	if e != nil {
		return request, e
	}
	if result.Usage.Reliable && result.State == "completed" {
		if !completed {
			request, e = s.Settle(recordCtx, nil, id, result.Usage)
			if e != nil {
				return request, e
			}
		}
		if e = s.StoreResult(recordCtx, request, result); e != nil {
			return request, e
		}
	}
	return request, nil
}
func (s *Service) lifecycleAttempt(request Request, row AttemptRecord, operation modelruntime.Operation) (modelruntime.Attempt, modelruntime.Request, error) {
	var attempt modelruntime.Attempt
	if e := json.Unmarshal(row.Definition, &attempt); e != nil {
		return attempt, modelruntime.Request{}, e
	}
	var candidates []catalog.UpstreamModel
	if e := json.Unmarshal(request.Candidates, &candidates); e != nil {
		return attempt, modelruntime.Request{}, e
	}
	found := false
	for _, u := range candidates {
		if u.ID == row.UpstreamID {
			attempt.BaseURL = u.Endpoints[operation]
			found = true
			break
		}
	}
	if !found || row.TaskID == "" {
		return attempt, modelruntime.Request{}, ErrNotReady
	}
	if attempt.BaseURL != "" {
		endpoint, err := modelruntime.ResolveTaskEndpoint(attempt.BaseURL, row.TaskID)
		if err != nil {
			return attempt, modelruntime.Request{}, err
		}
		attempt.BaseURL = endpoint
	}
	attempt.TaskID = row.TaskID
	attempt.Deadline = s.wallNow().Add(2 * time.Minute)
	return attempt, modelruntime.Request{InputFacts: attempt.InputFacts, ID: request.ID, PublicModel: request.ModelID, Operation: operation, Input: json.RawMessage(`{}`), Params: attempt.BillingParams, InputTypes: []string{"text"}}, nil
}
func (s *Service) DownloadAsync(ctx context.Context, customer, id string) (modelruntime.Result, error) {
	request, e := s.GetRequest(ctx, customer, id)
	if e != nil {
		return modelruntime.Result{}, e
	}
	if request.Operation != modelruntime.Video || request.Status != "completed" {
		return modelruntime.Result{}, ErrInProgress
	}
	// A result record is the retention gate; a vendor retaining a task longer
	// does not extend the retention promise displayed by this service.
	var retained resultRow
	if e = s.DB.WithContext(ctx).First(&retained, "request_id=? AND state='ready' AND expires_at>?", id, s.wallNow()).Error; e != nil {
		return modelruntime.Result{}, e
	}
	var row AttemptRecord
	if e = s.DB.WithContext(ctx).Where("request_id=? AND task_id<>''", id).Order("started_at DESC").First(&row).Error; e != nil {
		return modelruntime.Result{}, e
	}
	a, input, e := s.lifecycleAttempt(request, row, modelruntime.VideoDownload)
	if e != nil {
		return modelruntime.Result{}, e
	}
	return s.Runtime.Execute(ctx, a, input, nil)
}
func (s *Service) RequestForKey(ctx context.Context, key Key, id string) (Request, error) {
	request, e := s.GetRequest(ctx, key.CustomerID, id)
	if e != nil {
		return request, e
	}
	if request.KeyID != key.ID || request.BrandID != key.BrandID {
		return Request{}, ErrKey
	}
	return request, nil
}

type ResultInfo struct {
	RequestID   string    `json:"request_id"`
	ExpiresAt   time.Time `json:"expires_at"`
	ContentType string    `json:"content_type"`
}

func (s *Service) ResultMetadata(ctx context.Context, customer, id string) (*ResultInfo, error) {
	if _, e := s.GetRequest(ctx, customer, id); e != nil {
		return nil, e
	}
	var row resultRow
	e := s.DB.WithContext(ctx).First(&row, "request_id=? AND state='ready' AND expires_at>?", id, s.wallNow()).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	return &ResultInfo{row.RequestID, row.ExpiresAt, row.ContentType}, nil
}

// PendingRequests is a worker read. Reconciliation methods are idempotent; two
// workers may query the same provider task but can never debit it twice.
func (s *Service) PendingRequests(ctx context.Context, limit int) ([]Request, error) {
	if limit < 1 || limit > 100 {
		limit = 50
	}
	var rows []Request
	e := s.DB.WithContext(ctx).Where("status IN ('accepted','executing','submitted','awaiting_usage')").Order("usage_deadline,id").Limit(limit).Find(&rows).Error
	return rows, e
}
