package gateway

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"io/fs"
	"time"

	"gorm.io/gorm"

	"sync/atomic"

	"github.com/tokpath/tokstudio/backend/internal/billing"
	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/platform/id"
)

// Booker 是账务模块暴露给网关的预授权接口。网关不得直连 billing 表。
type Booker interface {
	Reserve(ctx context.Context, in billing.ReserveInput) (*billing.Reservation, error)
	Settle(ctx context.Context, in billing.SettleInput) (*billing.Settlement, error)
	Release(ctx context.Context, requestID string) error
}

//go:embed migrate/*.sql
var migrationFS embed.FS

var (
	ErrModelNotAllowed     = errors.New("model not allowed")
	ErrUnsupportedParam    = errors.New("unsupported parameter")
	ErrProviderUnavailable = errors.New("provider unavailable")
	ErrInsufficientBalance = errors.New("insufficient balance")
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

// Breaker 由 ops 实现。网关只问是否跳过、并回报成败，不读 ops 表。
type Breaker interface {
	RecordAttempt(ctx context.Context, providerID string, success bool)
	CircuitOpen(ctx context.Context, providerID string) bool
}

type Service struct {
	db           *gorm.DB
	catalog      *catalog.Service
	booker       Booker
	breaker      Breaker
	adapters     map[string]Adapter
	adapterCalls int32
}

func (s *Service) SetBreaker(b Breaker) {
	s.breaker = b
}

func New(db *gorm.DB, cat *catalog.Service, booker Booker, bifrostURL string) *Service {
	return &Service{
		db:      db,
		catalog: cat,
		booker:  booker,
		adapters: map[string]Adapter{
			"test":    TestAdapter{},
			"gemini":  GeminiAdapter{},
			"bifrost": BifrostAdapter{BaseURL: bifrostURL},
		},
	}
}

func (s *Service) AdapterCalls() int32 {
	return atomic.LoadInt32(&s.adapterCalls)
}

func Migrations() (string, fs.FS) {
	sub, err := fs.Sub(migrationFS, "migrate")
	if err != nil {
		panic(err)
	}
	return "gateway", sub
}

type ExecuteInput struct {
	Caller     identity.APIKeyPrincipal
	RequestID  string
	Protocol   string
	Hint       catalog.RouteHint
	ForceFail  string
	OmitUsage  bool
	CanarySlug string
	Chat       ChatRequest
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
	if in.CanarySlug != "" {
		in.Hint.Order = append([]string{in.CanarySlug}, in.Hint.Order...)
	}
	cands, err := s.catalog.ResolveRoute(ctx, model.ID, in.Hint)
	if err != nil || len(cands) == 0 {
		return nil, ErrProviderUnavailable
	}

	snapshot, err := s.catalog.PriceSnapshot(ctx, model.ID)
	if err != nil {
		return nil, err
	}
	quote, err := billing.ParseQuote(snapshot.VersionID, snapshot.Raw)
	if err != nil {
		return nil, err
	}
	promptHint := 0
	for _, msg := range in.Chat.Messages {
		promptHint += (len(msg.Content) + 3) / 4
	}
	maxTokens := 256
	if in.Chat.MaxTokens != nil && *in.Chat.MaxTokens > 0 {
		maxTokens = *in.Chat.MaxTokens
	}
	if _, err := s.booker.Reserve(ctx, billing.ReserveInput{
		UserID: in.Caller.UserID, ChannelOrgID: in.Caller.ChannelOrgID, APIKeyID: in.Caller.APIKeyID,
		RequestID: in.RequestID, PublicModelID: model.ID, PriceVersionID: snapshot.VersionID,
		UnitPrices: snapshot.Raw, ReserveMinor: billing.EstimateReserveMinor(quote, promptHint, maxTokens),
	}); err != nil {
		if errors.Is(err, billing.ErrInsufficientBalance) || errors.Is(err, billing.ErrInsufficientQuota) {
			return nil, ErrInsufficientBalance
		}
		return nil, err
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
		_ = s.booker.Release(ctx, in.RequestID)
		return nil, err
	}

	out := &ExecuteOutput{}
	streamStarted := false
	for i, cand := range cands {
		if s.breaker != nil && s.breaker.CircuitOpen(ctx, cand.ProviderID) {
			continue
		}
		behavior := cand.TestBehavior
		if in.ForceFail != "" && in.ForceFail == cand.ProviderSlug {
			behavior = "429"
		}
		adapter := s.adapters[cand.Adapter]
		if adapter == nil {
			adapter = s.adapters["test"]
		}
		start := time.Now()
		atomic.AddInt32(&s.adapterCalls, 1)
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
			if s.breaker != nil {
				s.breaker.RecordAttempt(ctx, cand.ProviderID, false)
			}
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
		if s.breaker != nil {
			s.breaker.RecordAttempt(ctx, cand.ProviderID, true)
		}
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
		usage := result.Body.Usage
		missing := in.OmitUsage || len(usage) == 0
		_, _ = s.booker.Settle(ctx, billing.SettleInput{
			RequestID: in.RequestID, AttemptID: attempt.ID, UserID: in.Caller.UserID,
			APIKeyID: in.Caller.APIKeyID, ChannelOrgID: in.Caller.ChannelOrgID,
			PublicModelID: model.ID, ProviderID: cand.ProviderID, UpstreamModelID: cand.UpstreamModelID,
			Usage: usage, PriceVersionID: snapshot.VersionID, UnitPrices: snapshot.Raw,
			MissingUsage: missing, IdempotencyKey: "usage:" + in.RequestID,
		})
		return out, nil
	}
	ended := time.Now().UTC()
	_ = s.db.WithContext(ctx).Model(&requestRow{}).Where("id = ?", req.ID).Updates(map[string]any{"status": "failed", "ended_at": ended})
	_ = s.booker.Release(ctx, in.RequestID)
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
