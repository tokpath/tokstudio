package gateway

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"io/fs"
	"strings"
	"sync/atomic"
	"time"

	"gorm.io/gorm"

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
	ErrChannelDisabled     = errors.New("channel is disabled")
)

// ChannelGuard 由 identity 实现。网关只问渠道能不能接新消费，不读身份表。
type ChannelGuard interface {
	AssertChannelConsumable(ctx context.Context, channelOrgID string) error
}

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
	ID               string     `gorm:"column:id;primaryKey"`
	RequestPK        string     `gorm:"column:request_pk"`
	ProviderID       string     `gorm:"column:provider_id"`
	UpstreamModelID  string     `gorm:"column:upstream_model_id"`
	AttemptNo        int        `gorm:"column:attempt_no"`
	Status           string     `gorm:"column:status"`
	HTTPStatus       *int       `gorm:"column:http_status"`
	ErrorCode        *string    `gorm:"column:error_code"`
	LatencyMS        int        `gorm:"column:latency_ms"`
	FactSource       *string    `gorm:"column:fact_source"`
	PromptTokens     *int       `gorm:"column:prompt_tokens"`
	CompletionTokens *int       `gorm:"column:completion_tokens"`
	TotalTokens      *int       `gorm:"column:total_tokens"`
	MetadataJSON     []byte     `gorm:"column:metadata_json"`
	StartedAt        time.Time  `gorm:"column:started_at"`
	EndedAt          *time.Time `gorm:"column:ended_at"`
}

func (attemptRow) TableName() string { return "gateway_attempts" }

type AttemptView struct {
	ID               string            `json:"id"`
	RequestID        string            `json:"request_id,omitempty"`
	ProviderID       string            `json:"provider_id"`
	UpstreamModelID  string            `json:"upstream_model_id,omitempty"`
	AttemptNo        int               `json:"attempt_no"`
	Status           string            `json:"status"`
	HTTPStatus       int               `json:"http_status"`
	ErrorCode        string            `json:"error_code,omitempty"`
	LatencyMS        int               `json:"latency_ms,omitempty"`
	FactSource       string            `json:"fact_source,omitempty"`
	PromptTokens     *int              `json:"prompt_tokens,omitempty"`
	CompletionTokens *int              `json:"completion_tokens,omitempty"`
	TotalTokens      *int              `json:"total_tokens,omitempty"`
	Metadata         map[string]string `json:"metadata,omitempty"`
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
	channels     ChannelGuard
	adapters     map[string]Adapter
	adapterCalls int32
	runtime      *Runtime
	encKey       string
}

func (s *Service) SetBreaker(b Breaker) {
	s.breaker = b
}

func (s *Service) SetChannelGuard(g ChannelGuard) {
	s.channels = g
}

func New(db *gorm.DB, cat *catalog.Service, booker Booker, rt *Runtime, encKey string) *Service {
	return &Service{
		db:      db,
		catalog: cat,
		booker:  booker,
		runtime: rt,
		encKey:  encKey,
		adapters: map[string]Adapter{
			"test":    UnavailableAdapter{AdapterName: "test"},
			"gemini":  GeminiAdapter{Runtime: rt},
			"bifrost": BifrostAdapter{Runtime: rt},
		},
	}
}

func (s *Service) Close() {
	if s == nil {
		return
	}
	s.runtime.Close()
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
	Caller      identity.APIKeyPrincipal
	RequestID   string
	Protocol    string
	Hint        catalog.RouteHint
	ForceFail   string
	OmitUsage   bool
	SandboxMode string
	CanarySlug  string
	Chat        ChatRequest
}

type ExecuteOutput struct {
	Response ChatResponse
	Stream   []string
	Attempts []AttemptView
}

func (s *Service) Execute(ctx context.Context, in ExecuteInput) (*ExecuteOutput, error) {
	if s.channels != nil {
		if err := s.channels.AssertChannelConsumable(ctx, in.Caller.ChannelOrgID); err != nil {
			if errors.Is(err, identity.ErrChannelDisabled) {
				return nil, ErrChannelDisabled
			}
			return nil, err
		}
	}
	model, err := s.catalog.GetVisibleModel(ctx, in.Caller.ChannelOrgID, in.Chat.Model, in.Caller.Allowlist)
	if err != nil {
		if errors.Is(err, catalog.ErrUnknownModel) {
			return nil, catalog.ErrUnknownModel
		}
		return nil, ErrModelNotAllowed
	}
	if err := ValidateChat(in.Chat, model.Capabilities); err != nil {
		return nil, err
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
	if in.Chat.ReasoningEffort != "" || presentRaw(in.Chat.Reasoning) {
		maxTokens += 64
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
		adapter := s.adapterFor(cand.Adapter)
		if adapter == nil {
			adapter = s.adapters["test"]
		}
		start := time.Now()
		atomic.AddInt32(&s.adapterCalls, 1)
		callReq := in.Chat
		if s.usesUpstreamModel(cand.Adapter) && cand.UpstreamModelID != "" {
			callReq.Model = cand.UpstreamModelID
		}
		attemptID := id.New("atm")
		callCtx, cancel := context.WithTimeout(ctx, candidateTimeout(cand.TimeoutMS))
		callCtx = ContextWithMeta(callCtx, map[string]string{
			"request_id":      in.RequestID,
			"attempt_id":      attemptID,
			"api_key_id":      in.Caller.APIKeyID,
			"user_id":         in.Caller.UserID,
			"channel_org_id":  in.Caller.ChannelOrgID,
			"public_model_id": model.ID,
		})
		if cand.AccountID != "" && s.encKey != "" {
			if secret, err := s.catalog.RevealAccount(callCtx, cand.AccountID, s.encKey); err == nil && secret != "" {
				callCtx = context.WithValue(callCtx, ctxAccountSecretKey, secret)
			}
		}
		result, err := adapter.Chat(callCtx, cand.ProviderSlug, behavior, callReq)
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(callCtx.Err(), context.DeadlineExceeded) {
			if result.HTTPStatus < 400 {
				result.HTTPStatus = 408
			}
			if result.ErrorClass == "" {
				result.ErrorClass = "timeout"
			}
		}
		cancel()
		if cand.AccountID != "" {
			_ = s.catalog.RecordAccountOutcome(ctx, cand.AccountID, result.HTTPStatus)
		}
		end := time.Now().UTC()
		latency := int(time.Since(start).Milliseconds())
		attempt := attemptRow{
			ID: attemptID, RequestPK: req.ID, ProviderID: cand.ProviderID,
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
			factSource := attemptFactSource(cand.Adapter, result, s.runtime)
			applyAttemptFacts(&attempt, nil, factSource, passthroughMeta(in, attemptID, model.ID, result, factSource))
			_ = s.db.WithContext(ctx).Create(&attempt).Error
			if s.breaker != nil {
				s.breaker.RecordAttempt(ctx, cand.ProviderID, false)
			}
			out.Attempts = append(out.Attempts, AttemptView{ID: attempt.ID, ProviderID: cand.ProviderID, AttemptNo: i + 1, Status: "failed", HTTPStatus: result.HTTPStatus, ErrorCode: code})
			if streamStarted {
				break
			}
			if result.HTTPStatus == 408 || result.HTTPStatus == 429 || result.HTTPStatus >= 500 || result.ErrorClass == "timeout" {
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
		result.Body.Model = in.Chat.Model
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
		mode := NormalizeUsageMode(in.SandboxMode, in.OmitUsage)
		usage = resolveAttemptUsage(usageAdapterName(cand.Adapter, s.runtime), mode, in.Chat, completionText(result.Body), usage)
		result.Body.Usage = usage
		out.Response.Usage = usage
		missing := mode == UsageOmit || len(usage) == 0
		factSource := attemptFactSource(cand.Adapter, result, s.runtime)
		applyAttemptFacts(&attempt, usage, factSource, passthroughMeta(in, attemptID, model.ID, result, factSource))
		_ = s.db.WithContext(ctx).Model(&attemptRow{}).Where("id = ?", attempt.ID).Updates(map[string]any{
			"fact_source":       attempt.FactSource,
			"prompt_tokens":     attempt.PromptTokens,
			"completion_tokens": attempt.CompletionTokens,
			"total_tokens":      attempt.TotalTokens,
			"metadata_json":     attempt.MetadataJSON,
		})
		_, _ = s.booker.Settle(ctx, billing.SettleInput{
			RequestID: in.RequestID, AttemptID: attempt.ID, UserID: in.Caller.UserID,
			APIKeyID: in.Caller.APIKeyID, ChannelOrgID: in.Caller.ChannelOrgID,
			PublicModelID: model.ID, ProviderID: cand.ProviderID, UpstreamModelID: cand.UpstreamModelID,
			Usage: usage, PriceVersionID: snapshot.VersionID, UnitPrices: snapshot.Raw,
			MissingUsage: missing, FactSource: factSource, IdempotencyKey: "usage:" + in.RequestID,
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
		view := AttemptView{
			ID: row.ID, RequestID: req.RequestID, ProviderID: row.ProviderID,
			UpstreamModelID: row.UpstreamModelID, AttemptNo: row.AttemptNo, Status: row.Status,
			LatencyMS: row.LatencyMS, PromptTokens: row.PromptTokens,
			CompletionTokens: row.CompletionTokens, TotalTokens: row.TotalTokens,
		}
		if row.HTTPStatus != nil {
			view.HTTPStatus = *row.HTTPStatus
		}
		if row.ErrorCode != nil {
			view.ErrorCode = *row.ErrorCode
		}
		if row.FactSource != nil {
			view.FactSource = *row.FactSource
		}
		if len(row.MetadataJSON) > 0 {
			var meta map[string]string
			if json.Unmarshal(row.MetadataJSON, &meta) == nil {
				view.Metadata = meta
			}
		}
		out = append(out, view)
	}
	return out, nil
}

func geminiGoesLive(rt *Runtime) bool {
	return GeminiLiveEnabled(rt) || (rt != nil && !rt.Sandbox && rt.Client != nil)
}

func attemptFactSource(adapter string, result AdapterResult, rt *Runtime) string {
	liveGemini := adapter == "gemini" && geminiGoesLive(rt)
	if src := normalizeFactSource(result.FactSource, false); src != "" {
		return src
	}
	liveUpstream := liveGemini || ((adapter == "bifrost" || useUpstreamModel(adapter, rt)) && rt != nil && rt.Client != nil)
	if liveUpstream && result.HTTPStatus == 200 {
		return FactSourceLive
	}
	return ""
}

func useUpstreamModel(adapter string, rt *Runtime) bool {
	if adapter == "bifrost" || (adapter == "gemini" && geminiGoesLive(rt)) {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(adapter)) {
	case "openai", "anthropic", "openrouter", "google":
		return true
	default:
		return false
	}
}

func usageAdapterName(adapter string, rt *Runtime) string {
	if adapter == "gemini" && geminiGoesLive(rt) {
		return "bifrost"
	}
	return adapter
}

func passthroughMeta(in ExecuteInput, attemptID, publicModelID string, result AdapterResult, factSource string) map[string]string {
	meta := PassthroughMeta{
		RequestID: in.RequestID, AttemptID: attemptID, UserID: in.Caller.UserID,
		APIKeyID: in.Caller.APIKeyID, ChannelOrgID: in.Caller.ChannelOrgID,
		PublicModelID: publicModelID, FactSource: factSource,
	}.Map()
	for key, value := range result.EchoedMeta {
		if strings.TrimSpace(value) == "" {
			continue
		}
		if _, exists := meta[key]; !exists {
			meta[key] = value
		}
	}
	return meta
}

func applyAttemptFacts(row *attemptRow, usage map[string]int, factSource string, meta map[string]string) {
	if factSource != "" {
		src := factSource
		row.FactSource = &src
	}
	row.PromptTokens, row.CompletionTokens, row.TotalTokens = usageTokenPtrs(usage)
	row.MetadataJSON = metadataJSON(meta)
}

func candidateTimeout(ms int) time.Duration {
	if ms <= 0 {
		ms = 30000
	}
	return time.Duration(ms) * time.Millisecond
}

func (s *Service) adapterFor(name string) Adapter {
	name = strings.ToLower(strings.TrimSpace(name))
	live := s.runtime != nil && !s.runtime.Sandbox && s.runtime.Client != nil
	switch name {
	case "test", "":
		if a := s.adapters["test"]; a != nil {
			return a
		}
		return UnavailableAdapter{AdapterName: "test"}
	case "gemini":
		if live {
			return s.adapters["bifrost"]
		}
		return GeminiAdapter{Runtime: s.runtime}
	case "bifrost", "openai", "anthropic", "openrouter", "google":
		if s.runtime != nil && s.runtime.Client != nil {
			return s.adapters["bifrost"]
		}
		// 仅测试 harness 可在无 Client 时顶上；生产 BifrostAdapter 不得伪装成功。
		if a, ok := s.adapters["bifrost"].(HarnessAdapter); ok {
			return a
		}
		return UnavailableAdapter{AdapterName: name}
	default:
		if live {
			return s.adapters["bifrost"]
		}
		if a := s.adapters["test"]; a != nil {
			return a
		}
		return UnavailableAdapter{AdapterName: name}
	}
}

func (s *Service) usesUpstreamModel(adapter string) bool {
	if useUpstreamModel(adapter, s.runtime) {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(adapter)) {
	case "bifrost", "openai", "anthropic", "openrouter", "gemini", "google":
		return true
	default:
		return false
	}
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
