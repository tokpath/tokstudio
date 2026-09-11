package media

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
)

// HarnessAdapter 仅供测试注入的确定性媒体上游。生产 New() 不挂载。
type HarnessAdapter struct {
	mu      sync.Mutex
	jobs    map[string]SubmitResult
	creates int32
}

func NewHarnessAdapter() *HarnessAdapter {
	return &HarnessAdapter{jobs: map[string]SubmitResult{}}
}

func (a *HarnessAdapter) Name() string { return "test" }

func (a *HarnessAdapter) Creates() int32 { return atomic.LoadInt32(&a.creates) }

func (a *HarnessAdapter) Create(_ context.Context, in SubmitInput) (SubmitResult, error) {
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

func (a *HarnessAdapter) Cancel(_ context.Context, upstreamID string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if row, ok := a.jobs[upstreamID]; ok {
		row.Status = StatusCancelled
		a.jobs[upstreamID] = row
	}
	return nil
}

func (a *HarnessAdapter) Get(_ context.Context, upstreamID string) (SubmitResult, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if row, ok := a.jobs[upstreamID]; ok {
		return row, nil
	}
	return SubmitResult{UpstreamID: upstreamID, Status: StatusInProgress}, nil
}

func (a *HarnessAdapter) Complete(upstreamID string, body []byte) {
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

// InstallTestHarness 挂上媒体测试上游。生产路径不得调用。
func (s *Service) InstallTestHarness() {
	if s == nil {
		return
	}
	s.harness = NewHarnessAdapter()
}
