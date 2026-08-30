package media

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
)

type SubmitInput struct {
	JobID       string
	Kind        string
	Model       string
	Prompt      string
	Duration    int
	Resolution  string
	AspectRatio string
	FPS         int
	Audio       bool
	Images      []string
}

type SubmitResult struct {
	UpstreamID  string
	Status      string
	Progress    int
	Usage       map[string]int
	Content     []byte
	ContentType string
}

type Adapter interface {
	Name() string
	Create(ctx context.Context, in SubmitInput) (SubmitResult, error)
	Cancel(ctx context.Context, upstreamID string) error
	Get(ctx context.Context, upstreamID string) (SubmitResult, error)
}

// TestAdapter 是可重复跑的沙箱上游：第一次 Create 才发号，之后只查询。
type TestAdapter struct {
	mu      sync.Mutex
	jobs    map[string]SubmitResult
	creates int32
}

func NewTestAdapter() *TestAdapter {
	return &TestAdapter{jobs: map[string]SubmitResult{}}
}

func (a *TestAdapter) Name() string { return "test" }

func (a *TestAdapter) Creates() int32 { return atomic.LoadInt32(&a.creates) }

func (a *TestAdapter) Create(_ context.Context, in SubmitInput) (SubmitResult, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if existing, ok := a.jobs[in.JobID]; ok && existing.UpstreamID != "" {
		return existing, nil
	}
	atomic.AddInt32(&a.creates, 1)
	if in.Prompt == "force-fail" {
		return SubmitResult{}, fmt.Errorf("upstream rejected")
	}
	result := SubmitResult{
		UpstreamID:  fmt.Sprintf("up_%s", in.JobID),
		Status:      StatusInProgress,
		Progress:    10,
		ContentType: "video/mp4",
		Usage:       map[string]int{"video_seconds": in.Duration, "prompt_tokens": 8},
	}
	if in.Kind == KindImage {
		result.ContentType = "image/png"
		result.Usage = map[string]int{"image_count": 1}
	}
	if in.Audio {
		result.Usage["audio_seconds"] = in.Duration
	}
	if in.Prompt != "force-async" {
		result.Status = StatusCompleted
		result.Progress = 100
		result.Content = []byte("tokenhub-sandbox-media:" + in.JobID)
	}
	a.jobs[in.JobID] = result
	a.jobs[result.UpstreamID] = result
	return result, nil
}

func (a *TestAdapter) Cancel(_ context.Context, upstreamID string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if row, ok := a.jobs[upstreamID]; ok {
		row.Status = StatusCancelled
		a.jobs[upstreamID] = row
	}
	return nil
}

func (a *TestAdapter) Get(_ context.Context, upstreamID string) (SubmitResult, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if row, ok := a.jobs[upstreamID]; ok {
		return row, nil
	}
	return SubmitResult{UpstreamID: upstreamID, Status: StatusInProgress}, nil
}

func (a *TestAdapter) Complete(upstreamID string, body []byte) {
	a.mu.Lock()
	defer a.mu.Unlock()
	row := a.jobs[upstreamID]
	row.Status = StatusCompleted
	row.Progress = 100
	if len(body) > 0 {
		row.Content = body
	}
	row.ContentType = "video/mp4"
	if row.Usage == nil {
		row.Usage = map[string]int{"video_seconds": 5}
	}
	a.jobs[upstreamID] = row
}

// RemoteAdapter 预留火山方舟 / OpenRouter。未配置 BaseURL 时不可用，测试走 TestAdapter。
type RemoteAdapter struct {
	NameValue string
	BaseURL   string
}

func (a RemoteAdapter) Name() string { return a.NameValue }

func (a RemoteAdapter) Create(_ context.Context, _ SubmitInput) (SubmitResult, error) {
	if a.BaseURL == "" {
		return SubmitResult{}, fmt.Errorf("%s unavailable", a.NameValue)
	}
	return SubmitResult{}, fmt.Errorf("%s not configured for sandbox", a.NameValue)
}

func (a RemoteAdapter) Cancel(context.Context, string) error { return nil }
func (a RemoteAdapter) Get(context.Context, string) (SubmitResult, error) {
	return SubmitResult{}, fmt.Errorf("unavailable")
}
