package gateway

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"io/fs"
	"time"

	"gorm.io/gorm"

	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/platform/id"
)

//go:embed migrate/*.sql
var migrationFS embed.FS

var (
	ErrModelNotAllowed     = errors.New("model not allowed")
	ErrUnsupportedParam    = errors.New("unsupported parameter")
	ErrProviderUnavailable = errors.New("provider unavailable")
)

type requestRow struct {
	ID            string     `gorm:"column:id;primaryKey"`
	RequestID     string     `gorm:"column:request_id"`
	UserID        string     `gorm:"column:user_id"`
	APIKeyID      *string    `gorm:"column:api_key_id"`
	ChannelOrgID  *string    `gorm:"column:channel_org_id"`
	PublicModelID string     `gorm:"column:public_model_id"`
	Protocol      string     `gorm:"column:protocol"`
	Status        string     `gorm:"column:status"`
	StartedAt     time.Time  `gorm:"column:started_at"`
	EndedAt       *time.Time `gorm:"column:ended_at"`
	FinalAttempt  *string    `gorm:"column:final_attempt_id"`
}

func (requestRow) TableName() string { return "gateway_requests" }

type attemptRow struct {
	ID              string     `gorm:"column:id;primaryKey"`
	RequestPK       string     `gorm:"column:request_pk"`
	ProviderID      string     `gorm:"column:provider_id"`
	UpstreamModelID string     `gorm:"column:upstream_model_id"`
	AttemptNo       int        `gorm:"column:attempt_no"`
	Status          string     `gorm:"column:status"`
	HTTPStatus      *int       `gorm:"column:http_status"`
	ErrorCode       *string    `gorm:"column:error_code"`
	LatencyMS       int        `gorm:"column:latency_ms"`
	StartedAt       time.Time  `gorm:"column:started_at"`
	EndedAt         *time.Time `gorm:"column:ended_at"`
}

func (attemptRow) TableName() string { return "gateway_attempts" }

type AttemptView struct {
	ID         string `json:"id"`
	ProviderID string `json:"provider_id"`
	AttemptNo  int    `json:"attempt_no"`
	Status     string `json:"status"`
	HTTPStatus int    `json:"http_status"`
	ErrorCode  string `json:"error_code,omitempty"`
}

type Service struct {
	db       *gorm.DB
	catalog  *catalog.Service
	adapters map[string]Adapter
}

func New(db *gorm.DB, cat *catalog.Service, bifrostURL string) *Service {
	return &Service{
		db:      db,
		catalog: cat,
		adapters: map[string]Adapter{
			"test":    TestAdapter{},
			"bifrost": BifrostAdapter{BaseURL: bifrostURL},
		},
	}
}

func Migrations() (string, fs.FS) {
	sub, err := fs.Sub(migrationFS, "migrate")
	if err != nil {
		panic(err)
	}
	return "gateway", sub
}

type ExecuteInput struct {
	Caller    identity.APIKeyPrincipal
	RequestID string
	Protocol  string
	Hint      catalog.RouteHint
	ForceFail string
	Chat      ChatRequest
}

type ExecuteOutput struct {
	Response ChatResponse
	Stream   []string
	Attempts []AttemptView
}

func (s *Service) Execute(ctx context.Context, in ExecuteInput) (*ExecuteOutput, error) {
	if in.Chat.LogitBias != nil && len(in.Chat.LogitBias) > 0 && string(in.Chat.LogitBias) != "null" {
		return nil, ErrUnsupportedParam
	}
	model, err := s.catalog.GetVisibleModel(ctx, in.Caller.ChannelOrgID, in.Chat.Model, in.Caller.Allowlist)
	if err != nil {
		return nil, ErrModelNotAllowed
	}
	cands, err := s.catalog.ResolveRoute(ctx, model.ID, in.Hint)
	if err != nil || len(cands) == 0 {
		return nil, ErrProviderUnavailable
	}

	now := time.Now().UTC()
	req := requestRow{
		ID: id.New("grq"), RequestID: in.RequestID, UserID: in.Caller.UserID,
		PublicModelID: model.ID, Protocol: in.Protocol, Status: "started", StartedAt: now,
	}
	if in.Caller.APIKeyID != "" {
		req.APIKeyID = &in.Caller.APIKeyID
	}
	if in.Caller.ChannelOrgID != "" {
		req.ChannelOrgID = &in.Caller.ChannelOrgID
	}
	if err := s.db.WithContext(ctx).Create(&req).Error; err != nil {
		return nil, err
	}

	out := &ExecuteOutput{}
	streamStarted := false
	for i, cand := range cands {
		behavior := cand.TestBehavior
		if in.ForceFail != "" && in.ForceFail == cand.ProviderSlug {
			behavior = "429"
		}
		adapter := s.adapters[cand.Adapter]
		if adapter == nil {
			adapter = s.adapters["test"]
		}
		start := time.Now()
		result, err := adapter.Chat(ctx, cand.ProviderSlug, behavior, in.Chat)
		end := time.Now().UTC()
		latency := int(time.Since(start).Milliseconds())
		attempt := attemptRow{
			ID: id.New("atm"), RequestPK: req.ID, ProviderID: cand.ProviderID,
			UpstreamModelID: cand.UpstreamModelID, AttemptNo: i + 1, StartedAt: start.UTC(), EndedAt: &end, LatencyMS: latency,
		}
		status := result.HTTPStatus
		attempt.HTTPStatus = &status
		if err != nil || result.HTTPStatus >= 400 {
			attempt.Status = "failed"
			code := result.ErrorClass
			if code == "" {
				code = "upstream_error"
			}
			attempt.ErrorCode = &code
			_ = s.db.WithContext(ctx).Create(&attempt).Error
			out.Attempts = append(out.Attempts, AttemptView{ID: attempt.ID, ProviderID: cand.ProviderID, AttemptNo: i + 1, Status: "failed", HTTPStatus: result.HTTPStatus, ErrorCode: code})
			if streamStarted {
				break
			}
			if result.HTTPStatus == 429 || result.HTTPStatus >= 500 {
				continue
			}
			break
		}
		attempt.Status = "succeeded"
		_ = s.db.WithContext(ctx).Create(&attempt).Error
		out.Attempts = append(out.Attempts, AttemptView{ID: attempt.ID, ProviderID: cand.ProviderID, AttemptNo: i + 1, Status: "succeeded", HTTPStatus: 200})
		result.Body.Provider = cand.ProviderSlug
		result.Body.RequestID = in.RequestID
		out.Response = result.Body
		out.Stream = result.Stream
		if in.Chat.Stream {
			streamStarted = true
		}
		ended := time.Now().UTC()
		_ = s.db.WithContext(ctx).Model(&requestRow{}).Where("id = ?", req.ID).Updates(map[string]any{
			"status": "succeeded", "ended_at": ended, "final_attempt_id": attempt.ID,
		})
		return out, nil
	}
	ended := time.Now().UTC()
	_ = s.db.WithContext(ctx).Model(&requestRow{}).Where("id = ?", req.ID).Updates(map[string]any{"status": "failed", "ended_at": ended})
	return out, ErrProviderUnavailable
}

func (s *Service) ListAttempts(ctx context.Context, requestID string) ([]AttemptView, error) {
	var req requestRow
	if err := s.db.WithContext(ctx).Where("request_id = ?", requestID).First(&req).Error; err != nil {
		return nil, err
	}
	var rows []attemptRow
	if err := s.db.WithContext(ctx).Where("request_pk = ?", req.ID).Order("attempt_no").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]AttemptView, 0, len(rows))
	for _, row := range rows {
		view := AttemptView{ID: row.ID, ProviderID: row.ProviderID, AttemptNo: row.AttemptNo, Status: row.Status}
		if row.HTTPStatus != nil {
			view.HTTPStatus = *row.HTTPStatus
		}
		if row.ErrorCode != nil {
			view.ErrorCode = *row.ErrorCode
		}
		out = append(out, view)
	}
	return out, nil
}

func ParseHint(only, ignore, order string) catalog.RouteHint {
	split := func(v string) []string {
		if v == "" {
			return nil
		}
		var parts []string
		_ = json.Unmarshal([]byte(v), &parts)
		if len(parts) == 0 {
			parts = []string{v}
		}
		return parts
	}
	return catalog.RouteHint{Only: split(only), Ignore: split(ignore), Order: split(order)}
}
