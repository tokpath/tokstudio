package media

import (
	"context"
	"fmt"
)

type SubmitInput struct {
	JobID          string
	Kind           string
	Model          string
	Prompt         string
	Duration       int
	Resolution     string
	AspectRatio    string
	FPS            int
	Audio          bool
	Images         []string
	TaskType       string
	FirstFrame     string
	LastFrame      string
	ReferenceVideo string
	ReferenceAudio string
	SourceJobID    string
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

// UnavailableAdapter 缺真实媒体上游时诚实失败，禁止沙箱假内容。
type UnavailableAdapter struct {
	AdapterName string
}

func (a UnavailableAdapter) Name() string {
	if a.AdapterName == "" {
		return "unavailable"
	}
	return a.AdapterName
}

func (a UnavailableAdapter) Create(context.Context, SubmitInput) (SubmitResult, error) {
	return SubmitResult{}, fmt.Errorf("%s media provider unavailable", a.Name())
}

func (a UnavailableAdapter) Cancel(context.Context, string) error {
	return fmt.Errorf("%s media provider unavailable", a.Name())
}

func (a UnavailableAdapter) Get(context.Context, string) (SubmitResult, error) {
	return SubmitResult{}, fmt.Errorf("%s media provider unavailable", a.Name())
}
