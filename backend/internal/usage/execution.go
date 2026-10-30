package usage

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"github.com/tokpath/tokstudio/backend/internal/modelruntime"
	platformcrypto "github.com/tokpath/tokstudio/backend/internal/platform/crypto"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type AttemptRecord struct {
	SupplierModelID                       string
	CostObservation                       json.RawMessage `gorm:"type:jsonb"`
	ReferenceCostCurrency                 string
	ReferenceCostAmount                   *string
	ReportedCostCurrency                  string
	ReportedCostAmount                    *string
	ID, RequestID, UpstreamID, ProviderID string
	Definition                            json.RawMessage `gorm:"type:jsonb"`
	State, ProviderRequestID, TaskID      string
	Usage                                 json.RawMessage `gorm:"type:jsonb"`
	CostStatus, ErrorCode                 string
	ProviderStatus                        int
	CostRecordedAt                        *time.Time
	OutputDelivered                       bool
	StartedAt                             time.Time
	CompletedAt                           *time.Time
}

func (AttemptRecord) TableName() string { return "usage_attempts" }

func failureReason(result modelruntime.Result) string {
	return modelruntime.SafeErrorCode(result.ErrorCode)
}

type resultRow struct {
	RequestID                             string `gorm:"primaryKey"`
	ContentType, ObjectKey, Digest, State string
	ExpiresAt, CreatedAt                  time.Time
}

func (resultRow) TableName() string { return "usage_results" }

type Execution struct {
	Request Request             `json:"request"`
	Result  modelruntime.Result `json:"-"`
	Replay  bool                `json:"replay"`
}

func (s *Service) Execute(ctx context.Context, key Key, businessKey string, input modelruntime.Request, emit modelruntime.Emit) (Execution, error) {
	ctx, span := otel.Tracer("tokenhub/usage").Start(ctx, "model.execute")
	defer span.End()
	if s.Runtime == nil {
		return Execution{}, ErrNotReady
	}
	req, created, e := s.Admit(ctx, key, businessKey, input)
	if e != nil {
		return Execution{}, e
	}
	span.SetAttributes(attribute.String("tokenhub.request_id", req.ID), attribute.String("tokenhub.model_id", req.ModelID), attribute.String("tokenhub.operation", string(req.Operation)), attribute.Bool("tokenhub.replay", !created))
	out := Execution{Request: req, Replay: !created}
	if terminal(req.Status) {
		out.Result, e = s.ReadResult(ctx, key.CustomerID, req.ID)
		return out, e
	}
	// Persist dispatch intent before external work. A process loss after this
	// point never causes an automatic second generation of the same request.
	claim := s.DB.WithContext(ctx).Model(&Request{}).Where("id=? AND status='accepted' AND cancel_requested=false", req.ID).Update("status", "executing")
	if claim.Error != nil {
		return out, claim.Error
	}
	if claim.RowsAffected == 0 {
		return out, ErrInProgress
	}
	input.ID = req.ID
	var candidates []catalog.UpstreamModel
	if e = json.Unmarshal(req.Candidates, &candidates); e != nil {
		return out, e
	}
	executionCtx, cancel := context.WithDeadline(ctx, req.UsageDeadline)
	defer cancel()
	// Cancellation is persisted so a different API instance can cancel the
	// active request. The monitor reads only status, never model content.
	monitorDone := make(chan struct{})
	defer close(monitorDone)
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-monitorDone:
				return
			case <-executionCtx.Done():
				return
			case <-ticker.C:
				var state struct {
					CancelRequested bool
					Status          string
				}
				if s.DB.WithContext(executionCtx).Model(&Request{}).Select("cancel_requested,status").Where("id=?", req.ID).Take(&state).Error == nil && (state.CancelRequested || terminal(state.Status)) {
					cancel()
					return
				}
			}
		}
	}()
	var last modelruntime.Result
	var lastErr error
	dispatched := false
	for _, upstream := range candidates {
		if executionCtx.Err() != nil {
			break
		}
		capability, e := catalog.SelectCapability(upstream, input)
		if e != nil {
			return out, e
		}
		attempt := modelruntime.Attempt{ID: uuid.NewString(), RequestID: req.ID, UpstreamID: upstream.ID, Provider: upstream.Provider, UpstreamModel: upstream.Model, PublicModel: req.ModelID, CredentialID: upstream.CredentialID, BaseURL: upstream.Endpoints[input.Operation], Deadline: req.UsageDeadline, Capability: capability}
		var acceptedQuote catalog.ModelQuote
		if e = json.Unmarshal(req.PriceSnapshot, &acceptedQuote); e != nil {
			return out, e
		}
		for _, price := range acceptedQuote.Supplier {
			if price.UpstreamID == upstream.ID {
				attempt.CostSnapshot = price.Snapshot
			}
		}
		attempt.BillingParams = modelruntime.BillingParameters(input.Params)
		definition, _ := json.Marshal(attempt)
		row := AttemptRecord{SupplierModelID: upstream.SupplierModelID, ID: attempt.ID, RequestID: req.ID, UpstreamID: upstream.ID, ProviderID: upstream.ProviderID, Definition: definition, State: "dispatching", CostStatus: "pending", StartedAt: s.wallNow()}
		e = s.DB.WithContext(executionCtx).Transaction(func(t *gorm.DB) error {
			if e := s.Catalog.WithDB(t).ClaimRoute(executionCtx, upstream.ID, attempt.ID, req.ID, req.UsageDeadline); e != nil {
				return e
			}
			return t.Create(&row).Error
		})
		if errors.Is(e, catalog.ErrRouteUnavailable) {
			lastErr = e
			continue
		}
		if e != nil {
			return out, e
		}
		dispatched = true
		delivered := false
		wrapped := func(c context.Context, event modelruntime.Event) error {
			if emit == nil {
				return modelruntime.ErrInvalidRequest
			}
			if e := emit(c, event); e != nil {
				return e
			}
			if len(event.Data) > 0 {
				delivered = true
			}
			return nil
		}
		attemptCtx, attemptSpan := otel.Tracer("tokenhub/usage").Start(executionCtx, "model.upstream_attempt")
		attemptSpan.SetAttributes(attribute.String("tokenhub.attempt_id", attempt.ID), attribute.String("tokenhub.upstream_id", upstream.ID), attribute.String("tokenhub.provider_id", upstream.ProviderID))
		last, lastErr = s.Runtime.Execute(attemptCtx, attempt, input, wrapped)
		last.ErrorCode = modelruntime.SafeErrorCode(last.ErrorCode)
		last.ProviderStatus = modelruntime.SafeProviderStatus(last.ProviderStatus)
		if last.ErrorCode == "" && lastErr != nil {
			last.ErrorCode = modelruntime.Diagnostic(lastErr)
		}
		attemptSpan.SetAttributes(attribute.String("tokenhub.outcome", last.State), attribute.Bool("tokenhub.usage_reliable", last.Usage.Reliable))
		attemptSpan.End()
		last.OutputDelivered = last.OutputDelivered || delivered
		// The caller may disconnect after consuming output. Settlement has its own
		// short context and cannot be cancelled by the disconnected client.
		recordCtx, done := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		e = s.RecordAttempt(recordCtx, req.ID, attempt.ID, last)
		done()
		if e != nil {
			return out, e
		}
		if last.State == "not_dispatched" {
			finalCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
			defer cancel()
			out.Request, e = s.failUndispatched(finalCtx, req.ID, failureReason(last))
			out.Result = last
			if e != nil {
				return out, e
			}
			return out, lastErr
		}
		if last.State == "submitted" || last.State == "outcome_unknown" || last.OutputDelivered || lastErr == nil {
			break
		}
		// Only a failed, undelivered attempt can move to the next verified route.
		// Its independently observed vendor cost remains a platform expense.
	}
	finishCtx, finishCancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer finishCancel()
	if !dispatched {
		out.Request, e = s.failUndispatched(finishCtx, req.ID)
		if e != nil {
			return out, e
		}
		return out, catalog.ErrRouteUnavailable
	}
	if last.Usage.Reliable && (last.State == "completed" || last.OutputDelivered) {
		out.Request, e = s.Settle(finishCtx, nil, req.ID, last.Usage)
	} else {
		state := "awaiting_usage"
		if last.State == "submitted" {
			state = "submitted"
		}
		reason := failureReason(last)
		if reason == "" {
			reason = "upstream_outcome_pending"
		}
		e = s.DB.WithContext(finishCtx).Model(&Request{}).Where("id=? AND status='executing'", req.ID).Updates(map[string]any{"status": state, "reason": reason}).Error
		if e == nil {
			out.Request, e = s.GetRequest(finishCtx, key.CustomerID, req.ID)
		}
	}
	out.Result = last
	if e != nil {
		return out, e
	}
	if out.Request.Status == "completed" && (len(last.Output) > 0 || len(last.Binary) > 0) {
		if e = s.StoreResult(finishCtx, out.Request, last); e != nil && req.Operation == modelruntime.Video {
			return out, e
		}
	}
	// Reliable usage can be charged on an interrupted stream. The HTTP handler
	// still receives its original runtime error and never sends a replacement.
	return out, lastErr
}

// RecordAttempt books every reliably observed vendor cost independently of
// customer success. Unknown costs remain pending instead of appearing as zero.
func (s *Service) RecordAttempt(ctx context.Context, requestID, attemptID string, result modelruntime.Result) error {
	return s.within(ctx, nil, func(t *gorm.DB) error {
		var request Request
		if e := t.First(&request, "id=?", requestID).Error; e != nil {
			return e
		}
		var row AttemptRecord
		if e := t.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, "id=? AND request_id=?", attemptID, requestID).Error; e != nil {
			return e
		}
		if row.State == "completed" && !result.Usage.Reliable {
			return nil
		}
		if result.TaskID == "" {
			result.TaskID = row.TaskID
		}
		if result.ProviderRequestID == "" {
			result.ProviderRequestID = row.ProviderRequestID
		}
		now := s.wallNow()
		updates := map[string]any{"state": result.State, "provider_request_id": result.ProviderRequestID, "task_id": result.TaskID, "error_code": modelruntime.SafeErrorCode(result.ErrorCode), "provider_status": modelruntime.SafeProviderStatus(result.ProviderStatus), "output_delivered": row.OutputDelivered || result.OutputDelivered, "completed_at": now}
		if result.InputFacts != nil {
			var definition modelruntime.Attempt
			if e := json.Unmarshal(row.Definition, &definition); e != nil {
				return e
			}
			definition.InputFacts = result.InputFacts
			encoded, _ := json.Marshal(definition)
			updates["definition"] = encoded
		}
		if result.Usage.Reliable {
			raw, _ := json.Marshal(result.Usage)
			updates["usage"] = raw
		}
		if result.State == "not_dispatched" {
			result.Cost = &modelruntime.CostObservation{Source: "not_dispatched"}
		}
		// Observations are source facts, never automatic financial postings. A
		// reference and a reported amount may coexist without being added together.
		if row.CostStatus != "confirmed" && result.Cost != nil {
			raw, _ := json.Marshal(result.Cost)
			updates["cost_observation"] = raw
			known := result.State == "not_dispatched"
			for _, item := range []struct {
				Prefix string
				Money  *modelruntime.Money
			}{
				{"reference_cost", result.Cost.ReferenceCost}, {"reported_cost", result.Cost.ReportedCost},
			} {
				if item.Money == nil {
					continue
				}
				if !validObservedMoney(*item.Money) {
					return ErrConflict
				}
				updates[item.Prefix+"_currency"] = item.Money.Currency
				updates[item.Prefix+"_amount"] = item.Money.Amount
				known = true
			}
			if known {
				updates["cost_status"] = "confirmed"
				if row.CostRecordedAt == nil {
					at, err := s.businessTime(t)
					if err != nil {
						return err
					}
					updates["cost_recorded_at"] = at
				}
			}
		}
		if e := t.Model(&AttemptRecord{}).Where("id=?", row.ID).Updates(updates).Error; e != nil {
			return e
		}
		return s.Catalog.WithDB(t).ObserveRoute(ctx, attemptID, result)
	})
}
func (s *Service) StoreResult(ctx context.Context, r Request, result modelruntime.Result) error {
	if s.ResultStore == nil {
		return ErrNotReady
	}
	if r.Status != "completed" || r.CompletedAt == nil {
		return ErrInProgress
	}
	if len(result.Output)+len(result.Binary) > 32<<20 {
		return errors.New("temporary result exceeds storage limit")
	}
	data := struct {
		Output      json.RawMessage `json:"output,omitempty"`
		Binary      string          `json:"binary,omitempty"`
		ContentType string          `json:"content_type"`
		TaskID      string          `json:"task_id,omitempty"`
	}{Output: result.Output, Binary: base64.StdEncoding.EncodeToString(result.Binary), ContentType: result.ContentType, TaskID: result.TaskID}
	b, e := json.Marshal(data)
	if e != nil {
		return e
	}
	sealed, e := platformcrypto.Seal(s.EncryptionKey, string(b))
	if e != nil {
		return e
	}
	ttl := 5 * time.Minute
	if r.Operation == modelruntime.Video {
		ttl = s.ResultRetention
	}
	now := s.wallNow()
	expiresAt := r.CompletedAt.Add(ttl)
	if !expiresAt.After(now) {
		return ErrResultUnavailable
	}
	digest := sha256.Sum256(b)
	hash := hex.EncodeToString(digest[:])
	row := resultRow{RequestID: r.ID, ContentType: result.ContentType, ObjectKey: "temporary-results/" + r.BrandID + "/" + r.CustomerID + "/" + r.ID + "/" + hash, Digest: hash, State: "pending", ExpiresAt: expiresAt, CreatedAt: now}
	// Record upload intent before S3 I/O. Retries never extend the original TTL
	// and never overwrite an earlier, different result under the same request.
	if e = s.DB.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; e != nil {
		return e
	}
	var stored resultRow
	if e = s.DB.WithContext(ctx).First(&stored, "request_id=?", r.ID).Error; e != nil {
		return e
	}
	if stored.Digest != hash {
		return ErrConflict
	}
	if !stored.ExpiresAt.After(now) {
		return ErrResultUnavailable
	}
	if stored.State == "ready" {
		return nil
	}
	if e = s.ResultStore.Put(ctx, stored.ObjectKey, []byte(sealed), "application/octet-stream", stored.ExpiresAt); e != nil {
		return e
	}
	updated := s.DB.WithContext(ctx).Model(&resultRow{}).Where("request_id=? AND digest=? AND expires_at>?", r.ID, hash, s.wallNow()).Update("state", "ready")
	if updated.Error != nil {
		return updated.Error
	}
	if updated.RowsAffected != 1 {
		if e = s.ResultStore.Delete(ctx, stored.ObjectKey); e != nil {
			return e
		}
		return ErrResultUnavailable
	}
	return nil
}
func (s *Service) ReadResult(ctx context.Context, customer, id string) (modelruntime.Result, error) {
	if _, e := s.GetRequest(ctx, customer, id); e != nil {
		return modelruntime.Result{}, e
	}
	var row resultRow
	if e := s.DB.WithContext(ctx).First(&row, "request_id=? AND state='ready' AND expires_at>?", id, s.wallNow()).Error; e != nil {
		if errors.Is(e, gorm.ErrRecordNotFound) {
			return modelruntime.Result{}, ErrResultUnavailable
		}
		return modelruntime.Result{}, e
	}
	if s.ResultStore == nil {
		return modelruntime.Result{}, ErrNotReady
	}
	ciphertext, e := s.ResultStore.Get(ctx, row.ObjectKey)
	if e != nil {
		return modelruntime.Result{}, e
	}
	raw, e := platformcrypto.Open(s.EncryptionKey, string(ciphertext))
	if e != nil {
		return modelruntime.Result{}, e
	}
	var data struct {
		Output                      json.RawMessage `json:"output"`
		Binary, ContentType, TaskID string
	}
	if e = json.Unmarshal([]byte(raw), &data); e != nil {
		return modelruntime.Result{}, e
	}
	binary, e := base64.StdEncoding.DecodeString(data.Binary)
	if e != nil {
		return modelruntime.Result{}, e
	}
	return modelruntime.Result{State: "completed", Output: data.Output, Binary: binary, ContentType: data.ContentType, TaskID: data.TaskID}, nil
}
func (s *Service) GetRequest(ctx context.Context, customer, id string) (Request, error) {
	var row Request
	e := s.DB.WithContext(ctx).First(&row, "id=? AND customer_id=?", id, customer).Error
	return row, e
}
func (s *Service) ListRequests(ctx context.Context, customer string, limit int, before *time.Time) ([]Request, error) {
	if limit < 1 || limit > 100 {
		limit = 50
	}
	q := s.DB.WithContext(ctx).Where("customer_id=?", customer)
	if before != nil {
		q = q.Where("accepted_at<?", *before)
	}
	var rows []Request
	e := q.Order("accepted_at DESC,id DESC").Limit(limit).Find(&rows).Error
	return rows, e
}
func (s *Service) RequestCancellation(ctx context.Context, customer, id string) (Request, error) {
	var row Request
	e := s.within(ctx, nil, func(t *gorm.DB) error {
		if e := t.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, "id=? AND customer_id=?", id, customer).Error; e != nil {
			return e
		}
		if terminal(row.Status) {
			return nil
		}
		row.CancelRequested = true
		return t.Model(&Request{}).Where("id=?", id).Update("cancel_requested", true).Error
	})
	return row, e
}
func (s *Service) DeleteExpiredResults(ctx context.Context) (int64, error) {
	return s.purgeExpiredResults(ctx, s.wallNow(), 100)
}
func (s *Service) purgeExpiredResults(ctx context.Context, at time.Time, limit int) (int64, error) {
	if s.ResultStore == nil {
		return 0, ErrNotReady
	}
	var count int64
	// Delete object first; an S3 outage keeps the durable reference for retry.
	// Each transaction handles one item so a slow object does not lock a batch.
	for count < int64(limit) {
		found := false
		e := s.DB.WithContext(ctx).Transaction(func(t *gorm.DB) error {
			var row resultRow
			e := t.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).Where("expires_at<=?", at).Order("expires_at,request_id").First(&row).Error
			if errors.Is(e, gorm.ErrRecordNotFound) {
				return nil
			}
			if e != nil {
				return e
			}
			found = true
			if e = s.ResultStore.Delete(ctx, row.ObjectKey); e != nil {
				return e
			}
			return t.Delete(&row).Error
		})
		if e != nil {
			return count, e
		}
		if !found {
			break
		}
		count++
	}
	return count, nil
}

func validObservedMoney(m modelruntime.Money) bool {
	if len(m.Currency) < 2 || len(m.Currency) > 12 || len(m.Amount) == 0 || len(m.Amount) > 100 {
		return false
	}
	for _, c := range m.Currency {
		if c < 'A' || c > 'Z' {
			return false
		}
	}
	digits, dots := 0, 0
	for _, c := range m.Amount {
		if c == '.' {
			dots++
			if dots > 1 {
				return false
			}
		} else if c >= '0' && c <= '9' {
			digits++
		} else {
			return false
		}
	}
	return digits > 0 && m.Amount[0] != '.' && m.Amount[len(m.Amount)-1] != '.'
}
