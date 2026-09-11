package media

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"time"

	"gorm.io/gorm"

	"github.com/tokpath/tokstudio/backend/internal/billing"
	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/outbox"
	"github.com/tokpath/tokstudio/backend/internal/platform/id"
)

//go:embed migrate/*.sql
var migrationFS embed.FS

const (
	StatusQueued     = "queued"
	StatusInProgress = "in_progress"
	StatusCompleted  = "completed"
	StatusFailed     = "failed"
	StatusCancelled  = "cancelled"
	StatusExpired    = "expired"
	KindVideo        = "video"
	KindImage        = "image"
	AssetRetention   = 7 * 24 * time.Hour
	SignTTL          = 15 * time.Minute
)

var (
	ErrNotFound       = errors.New("media job not found")
	ErrNotReady       = errors.New("media job not ready")
	ErrInvalidRequest = errors.New("invalid media request")
)

type jobRow struct {
	ID                string     `gorm:"column:id;primaryKey"`
	UserID            string     `gorm:"column:user_id"`
	APIKeyID          *string    `gorm:"column:api_key_id"`
	ChannelOrgID      *string    `gorm:"column:channel_org_id"`
	RequestID         string     `gorm:"column:request_id"`
	PublicModelID     string     `gorm:"column:public_model_id"`
	ProviderID        *string    `gorm:"column:provider_id"`
	UpstreamJobID     *string    `gorm:"column:upstream_job_id"`
	JobKind           string     `gorm:"column:job_kind"`
	Status            string     `gorm:"column:status"`
	Progress          int        `gorm:"column:progress"`
	Prompt            string     `gorm:"column:prompt"`
	DurationSeconds   int        `gorm:"column:duration_seconds"`
	Resolution        string     `gorm:"column:resolution"`
	AspectRatio       string     `gorm:"column:aspect_ratio"`
	FPS               int        `gorm:"column:fps"`
	GenerateAudio     bool       `gorm:"column:generate_audio"`
	TaskType          string     `gorm:"column:task_type"`
	FirstFrame        string     `gorm:"column:first_frame"`
	LastFrame         string     `gorm:"column:last_frame"`
	ReferenceVideo    string     `gorm:"column:reference_video"`
	ReferenceAudio    string     `gorm:"column:reference_audio"`
	SourceJobID       string     `gorm:"column:source_job_id"`
	ImagesJSON        []byte     `gorm:"column:images_json"`
	CallbackURL       string     `gorm:"column:callback_url"`
	CallbackEventID   string     `gorm:"column:callback_event_id"`
	CallbackStatus    string     `gorm:"column:callback_status"`
	CallbackAttempts  int        `gorm:"column:callback_attempts"`
	CallbackNextAt    *time.Time `gorm:"column:callback_next_at"`
	CallbackLastError string     `gorm:"column:callback_last_error"`
	IdempotencyKey    *string    `gorm:"column:idempotency_key"`
	ErrorCode         *string    `gorm:"column:error_code"`
	UsageJSON         []byte     `gorm:"column:usage_json"`
	ExpiresAt         *time.Time `gorm:"column:expires_at"`
	CreatedAt         time.Time  `gorm:"column:created_at"`
	UpdatedAt         time.Time  `gorm:"column:updated_at"`
	CompletedAt       *time.Time `gorm:"column:completed_at"`
}

func (jobRow) TableName() string { return "media_jobs" }

type assetRow struct {
	ID          string    `gorm:"column:id;primaryKey"`
	MediaJobID  string    `gorm:"column:media_job_id"`
	Kind        string    `gorm:"column:kind"`
	ObjectKey   string    `gorm:"column:object_key"`
	ContentType string    `gorm:"column:content_type"`
	SizeBytes   int64     `gorm:"column:size_bytes"`
	SHA256      string    `gorm:"column:sha256"`
	ExpiresAt   time.Time `gorm:"column:expires_at"`
	CreatedAt   time.Time `gorm:"column:created_at"`
}

func (assetRow) TableName() string { return "media_assets" }

type callbackRow struct {
	ID             string    `gorm:"column:id;primaryKey"`
	EventID        string    `gorm:"column:event_id"`
	MediaJobID     *string   `gorm:"column:media_job_id"`
	SignatureValid bool      `gorm:"column:signature_valid"`
	PayloadJSON    []byte    `gorm:"column:payload_json"`
	ProcessedAt    time.Time `gorm:"column:processed_at"`
}

func (callbackRow) TableName() string { return "media_callback_events" }

type CreateInput struct {
	Caller         identity.APIKeyPrincipal
	RequestID      string
	IdempotencyKey string
	Kind           string
	Model          string
	Prompt         string
	Duration       int
	Resolution     string
	AspectRatio    string
	FPS            int
	Audio          bool
	CallbackURL    string
	Images         []string
	TaskType       string
	FirstFrame     string
	LastFrame      string
	ReferenceVideo string
	ReferenceAudio string
	SourceJobID    string
	ForceFail      string
}

type JobView struct {
	ID             string         `json:"id"`
	Object         string         `json:"object"`
	Kind           string         `json:"kind,omitempty"`
	TaskType       string         `json:"task_type,omitempty"`
	Status         string         `json:"status"`
	Model          string         `json:"model"`
	CreatedAt      int64          `json:"created_at"`
	StatusURL      string         `json:"status_url"`
	Progress       int            `json:"progress"`
	Provider       string         `json:"provider,omitempty"`
	UpstreamID     string         `json:"upstream_job_id,omitempty"`
	Error          string         `json:"error,omitempty"`
	Usage          map[string]int `json:"usage,omitempty"`
	ExpiresAt      int64          `json:"expires_at,omitempty"`
	Duration       int            `json:"duration,omitempty"`
	Resolution     string         `json:"resolution,omitempty"`
	AspectRatio    string         `json:"aspect_ratio,omitempty"`
	FPS            int            `json:"fps,omitempty"`
	GenerateAudio  bool           `json:"generate_audio,omitempty"`
	Images         []string       `json:"images,omitempty"`
	FirstFrame     string         `json:"first_frame,omitempty"`
	LastFrame      string         `json:"last_frame,omitempty"`
	ReferenceVideo string         `json:"reference_video,omitempty"`
	ReferenceAudio string         `json:"reference_audio,omitempty"`
	SourceJobID    string         `json:"source_job_id,omitempty"`
}

type ContentView struct {
	URL       string `json:"url"`
	ExpiresAt int64  `json:"expires_at"`
	Storage   Status `json:"storage"`
}

type Service struct {
	db      *gorm.DB
	catalog *catalog.Service
	billing *billing.Service
	outbox  *outbox.Service
	store   ObjectStore
	ark     RemoteAdapter
	or      RemoteAdapter
	harness *HarnessAdapter
}

func New(db *gorm.DB, cat *catalog.Service, bill *billing.Service, pub *outbox.Service, store ObjectStore, arkURL, arkKey, orURL, orKey string) *Service {
	return &Service{
		db: db, catalog: cat, billing: bill, outbox: pub, store: store,
		ark: RemoteAdapter{NameValue: "ark", BaseURL: arkURL, APIKey: arkKey},
		or:  RemoteAdapter{NameValue: "openrouter", BaseURL: orURL, APIKey: orKey},
	}
}

func Migrations() (string, fs.FS) {
	sub, err := fs.Sub(migrationFS, "migrate")
	if err != nil {
		panic(err)
	}
	return "media", sub
}

func (s *Service) TestCreates() int32 {
	if s == nil || s.harness == nil {
		return 0
	}
	return s.harness.Creates()
}

func (s *Service) Create(ctx context.Context, in CreateInput) (*JobView, error) {
	if in.Kind == "" {
		in.Kind = KindVideo
	}
	if in.Model == "" {
		if in.Kind == KindImage {
			in.Model = catalog.ImageModelID
		} else {
			in.Model = catalog.SeedanceModelID
		}
	}
	if in.Duration <= 0 {
		in.Duration = 5
	}
	if in.Resolution == "" {
		in.Resolution = "720p"
	}
	task, err := NormalizeTaskType(in.Kind, in.TaskType)
	if err != nil {
		return nil, err
	}
	in.TaskType = task
	if err := ValidateCreate(in); err != nil {
		return nil, err
	}
	if in.Kind == KindVideo && (in.TaskType == TaskExtend || in.TaskType == TaskEdit) {
		if err := s.requireSourceJob(ctx, in.Caller.UserID, in.SourceJobID, KindVideo); err != nil {
			return nil, err
		}
	}
	model, err := s.catalog.GetVisibleModel(ctx, in.Caller.ChannelOrgID, in.Model, in.Caller.Allowlist)
	if err != nil {
		return nil, err
	}
	if in.IdempotencyKey != "" {
		var existing jobRow
		if err := s.db.WithContext(ctx).Where("user_id = ? AND idempotency_key = ?", in.Caller.UserID, in.IdempotencyKey).First(&existing).Error; err == nil {
			return s.view(ctx, existing), nil
		}
	}
	snapshot, err := s.catalog.PriceSnapshot(ctx, model.ID)
	if err != nil {
		return nil, err
	}
	quote, err := billing.ParseQuote(snapshot.VersionID, snapshot.Raw)
	if err != nil {
		return nil, err
	}
	images := 0
	if in.Kind == KindImage {
		images = 1
		if len(in.Images) > 0 {
			images = len(in.Images)
		}
	}
	seconds := 0
	if in.Kind == KindVideo {
		seconds = in.Duration
	}
	reserve := billing.EstimateMediaReserveMinor(quote, seconds, images, in.Resolution, in.Audio)
	if _, err := s.billing.Reserve(ctx, billing.ReserveInput{
		UserID: in.Caller.UserID, ChannelOrgID: in.Caller.ChannelOrgID, APIKeyID: in.Caller.APIKeyID,
		RequestID: in.RequestID, PublicModelID: model.ID, PriceVersionID: snapshot.VersionID,
		UnitPrices: snapshot.Raw, ReserveMinor: reserve,
	}); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	exp := now.Add(AssetRetention)
	imagesJSON, _ := json.Marshal(in.Images)
	if len(imagesJSON) == 0 {
		imagesJSON = []byte("[]")
	}
	job := jobRow{
		ID: id.New("vid"), UserID: in.Caller.UserID, RequestID: in.RequestID,
		PublicModelID: model.ID, JobKind: in.Kind, Status: StatusQueued, Prompt: in.Prompt,
		DurationSeconds: in.Duration, Resolution: in.Resolution, AspectRatio: in.AspectRatio,
		FPS: in.FPS, GenerateAudio: in.Audio, CallbackURL: in.CallbackURL,
		TaskType: in.TaskType, FirstFrame: in.FirstFrame, LastFrame: in.LastFrame,
		ReferenceVideo: in.ReferenceVideo, ReferenceAudio: in.ReferenceAudio,
		SourceJobID: in.SourceJobID, ImagesJSON: imagesJSON,
		UsageJSON: []byte(`{}`), ExpiresAt: &exp, CreatedAt: now, UpdatedAt: now,
	}
	if in.Kind == KindImage {
		job.ID = id.New("img")
	}
	if in.Caller.APIKeyID != "" {
		job.APIKeyID = &in.Caller.APIKeyID
	}
	if in.Caller.ChannelOrgID != "" {
		job.ChannelOrgID = &in.Caller.ChannelOrgID
	}
	if in.IdempotencyKey != "" {
		job.IdempotencyKey = &in.IdempotencyKey
	}
	if err := s.db.WithContext(ctx).Create(&job).Error; err != nil {
		_ = s.billing.Release(ctx, in.RequestID)
		return nil, err
	}
	if _, err := s.outbox.Enqueue(ctx, "media.job.created", "media_job", job.ID, map[string]any{"request_id": in.RequestID}); err != nil {
		return nil, err
	}

	if job.UpstreamJobID != nil && *job.UpstreamJobID != "" {
		return s.view(ctx, job), nil
	}

	cands, err := s.catalog.ResolveRoute(ctx, model.ID, catalog.RouteHint{})
	if err != nil || len(cands) == 0 {
		return s.fail(ctx, job, "provider_unavailable")
	}
	var last error
	for _, cand := range cands {
		if in.ForceFail != "" && in.ForceFail == cand.ProviderSlug {
			last = errors.New("forced fail")
			continue
		}
		adapter := s.adapterFor(cand.Adapter)
		result, err := adapter.Create(ctx, SubmitInput{
			JobID: job.ID, Kind: in.Kind, Model: firstNonEmpty(cand.UpstreamModelID, in.Model), Prompt: in.Prompt,
			Duration: in.Duration, Resolution: in.Resolution, AspectRatio: in.AspectRatio,
			FPS: in.FPS, Audio: in.Audio, Images: adapterImages(in),
			TaskType: in.TaskType, FirstFrame: in.FirstFrame, LastFrame: in.LastFrame,
			ReferenceVideo: in.ReferenceVideo, ReferenceAudio: in.ReferenceAudio,
			SourceJobID: in.SourceJobID,
		})
		if err != nil {
			last = err
			continue
		}
		job.ProviderID = &cand.ProviderID
		job.UpstreamJobID = &result.UpstreamID
		job.Status = StatusInProgress
		job.Progress = result.Progress
		job.UpdatedAt = time.Now().UTC()
		if err := s.db.WithContext(ctx).Save(&job).Error; err != nil {
			return nil, err
		}
		if result.Status == StatusCompleted {
			if err := s.finish(ctx, job, result, snapshot); err != nil {
				return nil, err
			}
		}
		return s.Get(ctx, job.ID, in.Caller.UserID)
	}
	if last != nil {
		return s.fail(ctx, job, "upstream_error")
	}
	return s.fail(ctx, job, "provider_unavailable")
}

func (s *Service) List(ctx context.Context, userID, kind, status string, limit int) ([]JobView, error) {
	q := s.db.WithContext(ctx).Order("created_at DESC")
	if userID != "" {
		q = q.Where("user_id = ?", userID)
	}
	if kind != "" {
		q = q.Where("job_kind = ?", kind)
	}
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	var rows []jobRow
	if err := q.Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]JobView, 0, len(rows))
	for _, row := range rows {
		out = append(out, *s.view(ctx, row))
	}
	return out, nil
}

func (s *Service) Get(ctx context.Context, jobID, userID string) (*JobView, error) {
	var job jobRow
	q := s.db.WithContext(ctx).Where("id = ?", jobID)
	if userID != "" {
		q = q.Where("user_id = ?", userID)
	}
	if err := q.First(&job).Error; err != nil {
		return nil, ErrNotFound
	}
	if job.Status == StatusQueued || job.Status == StatusInProgress {
		_ = s.refreshJob(ctx, job)
		reload := s.db.WithContext(ctx).Where("id = ?", jobID)
		if userID != "" {
			reload = reload.Where("user_id = ?", userID)
		}
		if err := reload.First(&job).Error; err != nil {
			return nil, ErrNotFound
		}
	}
	return s.view(ctx, job), nil
}

func (s *Service) Content(ctx context.Context, jobID, userID string) (*ContentView, error) {
	var job jobRow
	q := s.db.WithContext(ctx).Where("id = ?", jobID)
	if userID != "" {
		q = q.Where("user_id = ?", userID)
	}
	if err := q.First(&job).Error; err != nil {
		return nil, ErrNotFound
	}
	if job.Status != StatusCompleted {
		return nil, ErrNotReady
	}
	var asset assetRow
	if err := s.db.WithContext(ctx).Where("media_job_id = ?", job.ID).Order("created_at DESC").First(&asset).Error; err != nil {
		return nil, ErrNotReady
	}
	if time.Now().UTC().After(asset.ExpiresAt) {
		return nil, ErrNotReady
	}
	url, exp, err := s.store.Sign(asset.ObjectKey, SignTTL)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrStoreUnavailable, err)
	}
	return &ContentView{URL: url, ExpiresAt: exp.Unix(), Storage: s.StoreStatus(ctx)}, nil
}

func (s *Service) StoreStatus(ctx context.Context) Status {
	if s == nil || s.store == nil {
		return unavailableStatus("store not configured")
	}
	return s.store.Status(ctx)
}

func (s *Service) Cancel(ctx context.Context, jobID, userID string) (*JobView, error) {
	var job jobRow
	if err := s.db.WithContext(ctx).Where("id = ? AND user_id = ?", jobID, userID).First(&job).Error; err != nil {
		return nil, ErrNotFound
	}
	if job.Status == StatusCompleted || job.Status == StatusCancelled || job.Status == StatusFailed {
		return s.view(ctx, job), nil
	}
	if job.UpstreamJobID != nil && *job.UpstreamJobID != "" {
		_ = s.adapterForJob(ctx, job).Cancel(ctx, *job.UpstreamJobID)
	}
	job.Status = StatusCancelled
	job.UpdatedAt = time.Now().UTC()
	if err := s.db.WithContext(ctx).Save(&job).Error; err != nil {
		return nil, err
	}
	_ = s.billing.Release(ctx, job.RequestID)
	s.queueCustomerCallback(ctx, job)
	s.DeliverCustomerCallbacks(ctx)
	return s.view(ctx, job), nil
}

func (s *Service) HandleCallback(ctx context.Context, eventID, jobID, sig string, payload []byte, usage map[string]int) error {
	valid := s.store.CallbackValid(eventID, jobID, sig)
	if err := s.recordCallbackEvent(ctx, eventID, jobID, payload, valid); err != nil {
		return err
	}
	if !valid {
		return errors.New("invalid callback signature")
	}
	var job jobRow
	if err := s.db.WithContext(ctx).Where("id = ?", jobID).First(&job).Error; err != nil {
		return ErrNotFound
	}
	if job.Status == StatusCancelled {
		return nil
	}
	snapshot, err := s.catalog.PriceSnapshot(ctx, job.PublicModelID)
	if err != nil {
		return err
	}
	if usage == nil {
		usage = map[string]int{}
	}
	result := SubmitResult{Status: StatusCompleted, Progress: 100, Usage: usage, Content: []byte("callback-media:" + job.ID), ContentType: "video/mp4"}
	if job.JobKind == KindImage {
		result.ContentType = "image/png"
	}
	return s.finish(ctx, job, result, snapshot)
}

func (s *Service) Object(key, sig string, exp int64) ([]byte, error) {
	if !s.store.Verify(key, sig, exp) {
		return nil, ErrNotFound
	}
	return s.store.Read(key)
}

func (s *Service) Cleanup(ctx context.Context) (int, error) {
	var assets []assetRow
	if err := s.db.WithContext(ctx).Where("expires_at < ?", time.Now().UTC()).Limit(100).Find(&assets).Error; err != nil {
		return 0, err
	}
	n := 0
	for _, asset := range assets {
		_ = s.store.Delete(asset.ObjectKey)
		if err := s.db.WithContext(ctx).Delete(&asset).Error; err == nil {
			n++
		}
	}
	return n, nil
}

func (s *Service) Run(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	s.Tick(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.Tick(ctx)
		}
	}
}

func (s *Service) Tick(ctx context.Context) {
	s.PollUpstream(ctx)
	s.DeliverCustomerCallbacks(ctx)
	_, _ = s.Cleanup(ctx)
}

func (s *Service) CompleteTestJob(upstreamID string, body []byte) {
	if s == nil || s.harness == nil {
		return
	}
	s.harness.Complete(upstreamID, body)
}

func (s *Service) PollUpstream(ctx context.Context) {
	var jobs []jobRow
	if err := s.db.WithContext(ctx).
		Where("status IN ? AND upstream_job_id IS NOT NULL AND upstream_job_id <> ''", []string{StatusQueued, StatusInProgress}).
		Order("updated_at ASC").Limit(20).Find(&jobs).Error; err != nil {
		return
	}
	for _, job := range jobs {
		_ = s.refreshJob(ctx, job)
	}
}

func (s *Service) refreshJob(ctx context.Context, job jobRow) error {
	if job.UpstreamJobID == nil || *job.UpstreamJobID == "" {
		return nil
	}
	if job.Status != StatusQueued && job.Status != StatusInProgress {
		return nil
	}
	result, err := s.adapterForJob(ctx, job).Get(ctx, *job.UpstreamJobID)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	if result.Status == StatusInProgress || result.Status == StatusQueued {
		progress := result.Progress
		if progress == 0 {
			progress = job.Progress
		}
		_ = s.db.WithContext(ctx).Model(&jobRow{}).
			Where("id = ? AND status IN ?", job.ID, []string{StatusQueued, StatusInProgress}).
			Updates(map[string]any{"status": StatusInProgress, "progress": progress, "updated_at": now}).Error
		return nil
	}
	if result.Status == StatusCompleted {
		if result.Usage == nil {
			result.Usage = map[string]int{}
		}
		if job.JobKind == KindVideo && result.Usage["video_seconds"] == 0 {
			result.Usage["video_seconds"] = job.DurationSeconds
		}
		if job.JobKind == KindImage && result.Usage["image_count"] == 0 {
			result.Usage["image_count"] = 1
		}
		snapshot, err := s.catalog.PriceSnapshot(ctx, job.PublicModelID)
		if err != nil {
			return err
		}
		return s.finish(ctx, job, result, snapshot)
	}
	if result.Status == StatusCancelled {
		job.Status = StatusCancelled
		job.UpdatedAt = now
		_ = s.db.WithContext(ctx).Save(&job).Error
		_ = s.billing.Release(ctx, job.RequestID)
		s.queueCustomerCallback(ctx, job)
		s.DeliverCustomerCallbacks(ctx)
		return nil
	}
	_, _ = s.fail(ctx, job, "upstream_error")
	return nil
}

func (s *Service) finish(ctx context.Context, job jobRow, result SubmitResult, snapshot *catalog.PriceSnapshot) error {
	usage := result.Usage
	if usage == nil {
		usage = map[string]int{}
	}
	if job.JobKind == KindVideo && usage["video_seconds"] == 0 {
		usage["video_seconds"] = job.DurationSeconds
	}
	if job.JobKind == KindImage && usage["image_count"] == 0 {
		usage["image_count"] = 1
	}
	if job.GenerateAudio && usage["audio_seconds"] == 0 && job.JobKind == KindVideo {
		usage["audio_seconds"] = job.DurationSeconds
	}
	wrote := false
	var usageJSON []byte
	if job.Status != StatusCompleted {
		key := job.ID + "/output.bin"
		if err := s.store.Put(key, result.ContentType, result.Content); err != nil {
			return fmt.Errorf("%w: %v", ErrStoreUnavailable, err)
		}
		sum := sha256.Sum256(result.Content)
		now := time.Now().UTC()
		exp := now.Add(AssetRetention)
		asset := assetRow{
			ID: id.New("ast"), MediaJobID: job.ID, Kind: "output", ObjectKey: key,
			ContentType: result.ContentType, SizeBytes: int64(len(result.Content)),
			SHA256: hex.EncodeToString(sum[:]), ExpiresAt: exp, CreatedAt: now,
		}
		if err := s.ensureAsset(ctx, asset); err != nil {
			return err
		}
		body, _ := json.Marshal(usage)
		usageJSON = body
		res := s.db.WithContext(ctx).Model(&jobRow{}).
			Where("id = ? AND status NOT IN ?", job.ID, []string{StatusCompleted, StatusCancelled, StatusFailed}).
			Updates(map[string]any{
				"status":       StatusCompleted,
				"progress":     100,
				"usage_json":   body,
				"updated_at":   now,
				"completed_at": now,
				"expires_at":   exp,
			})
		if res.Error != nil {
			return res.Error
		}
		wrote = res.RowsAffected > 0
	}
	if wrote {
		job.Status = StatusCompleted
		job.UsageJSON = usageJSON
		s.queueCustomerCallback(ctx, job)
		s.DeliverCustomerCallbacks(ctx)
	}
	// 已完成的任务再收到回调时只做幂等结算，不重复写资产。
	_, err := s.billing.Settle(ctx, billing.SettleInput{
		RequestID: job.RequestID, UserID: job.UserID, PublicModelID: job.PublicModelID,
		Usage: usage, PriceVersionID: snapshot.VersionID, UnitPrices: snapshot.Raw,
		IdempotencyKey: "usage:" + job.RequestID, Resolution: job.Resolution,
		MissingUsage: len(usage) == 0,
	})
	return err
}

func (s *Service) fail(ctx context.Context, job jobRow, code string) (*JobView, error) {
	job.Status = StatusFailed
	job.ErrorCode = &code
	job.UpdatedAt = time.Now().UTC()
	_ = s.db.WithContext(ctx).Save(&job).Error
	_ = s.billing.Release(ctx, job.RequestID)
	s.queueCustomerCallback(ctx, job)
	s.DeliverCustomerCallbacks(ctx)
	return s.view(ctx, job), nil
}

func (s *Service) adapterFor(name string) Adapter {
	switch name {
	case "ark":
		if s.ark.Ready() {
			return s.ark
		}
	case "openrouter":
		if s.or.Ready() {
			return s.or
		}
	}
	if s.harness != nil {
		return s.harness
	}
	return UnavailableAdapter{AdapterName: firstNonEmpty(name, "test")}
}

func (s *Service) adapterForJob(ctx context.Context, job jobRow) Adapter {
	if job.ProviderID != nil && *job.ProviderID != "" {
		p, err := s.catalog.GetProvider(ctx, *job.ProviderID)
		if err == nil {
			return s.adapterFor(p.Adapter)
		}
	}
	if s.harness != nil {
		return s.harness
	}
	return UnavailableAdapter{AdapterName: "test"}
}

func (s *Service) view(_ context.Context, job jobRow) *JobView {
	usage := map[string]int{}
	_ = json.Unmarshal(job.UsageJSON, &usage)
	object := "video"
	if job.JobKind == KindImage {
		object = "image"
	}
	images := []string{}
	_ = json.Unmarshal(job.ImagesJSON, &images)
	view := &JobView{
		ID: job.ID, Object: object, Kind: job.JobKind, TaskType: job.TaskType,
		Status: job.Status, Model: job.PublicModelID,
		CreatedAt: job.CreatedAt.Unix(), StatusURL: "/v1/videos/" + job.ID,
		Progress: job.Progress, Usage: usage,
		Duration: job.DurationSeconds, Resolution: job.Resolution, AspectRatio: job.AspectRatio,
		FPS: job.FPS, GenerateAudio: job.GenerateAudio, Images: images,
		FirstFrame: job.FirstFrame, LastFrame: job.LastFrame,
		ReferenceVideo: job.ReferenceVideo, ReferenceAudio: job.ReferenceAudio,
		SourceJobID: job.SourceJobID,
	}
	if job.JobKind == KindImage {
		view.StatusURL = "/v1/images/" + job.ID
	}
	if job.ProviderID != nil {
		view.Provider = *job.ProviderID
	}
	if job.UpstreamJobID != nil {
		view.UpstreamID = *job.UpstreamJobID
	}
	if job.ErrorCode != nil {
		view.Error = *job.ErrorCode
	}
	if job.ExpiresAt != nil {
		view.ExpiresAt = job.ExpiresAt.Unix()
	}
	return view
}

func (s *Service) SignCallback(eventID, jobID string) string {
	return s.store.CallbackSign(eventID, jobID)
}

func (s *Service) requireSourceJob(ctx context.Context, userID, sourceID, wantKind string) error {
	if sourceID == "" {
		return Invalid("延长/编辑需要已完成的本用户视频任务 source_job_id")
	}
	var src jobRow
	if err := s.db.WithContext(ctx).Where("id = ? AND user_id = ?", sourceID, userID).First(&src).Error; err != nil {
		return Invalid("源任务不存在或不属于当前用户")
	}
	if src.JobKind != wantKind {
		return Invalid("源任务类型不匹配")
	}
	if src.Status != StatusCompleted {
		return Invalid("源任务必须已完成才能延长或编辑")
	}
	return nil
}

// recordCallbackEvent 只按 event_id 查找。不要用 FirstOrCreate 带新主键，GORM 会把 id 拼进 WHERE，旧事件会当成“不存在”再插入。
func (s *Service) recordCallbackEvent(ctx context.Context, eventID, jobID string, payload []byte, valid bool) error {
	var existing callbackRow
	err := s.db.WithContext(ctx).Where("event_id = ?", eventID).First(&existing).Error
	if err == nil {
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	row := callbackRow{
		ID: id.New("cbe"), EventID: eventID, MediaJobID: &jobID,
		SignatureValid: valid, PayloadJSON: payload, ProcessedAt: time.Now().UTC(),
	}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		var again callbackRow
		if s.db.WithContext(ctx).Where("event_id = ?", eventID).First(&again).Error == nil {
			return nil
		}
		return err
	}
	return nil
}

func (s *Service) ensureAsset(ctx context.Context, asset assetRow) error {
	var existing assetRow
	err := s.db.WithContext(ctx).Where("media_job_id = ? AND object_key = ?", asset.MediaJobID, asset.ObjectKey).First(&existing).Error
	if err == nil {
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return s.db.WithContext(ctx).Create(&asset).Error
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
