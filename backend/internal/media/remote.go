package media

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	maxProviderBody = 1 << 20
	maxAssetBody    = 64 << 20
)

var remoteHTTP = &http.Client{Timeout: 45 * time.Second}

// RemoteAdapter 对接火山方舟 contents/generations 与 OpenRouter /videos。
// BaseURL 与 API Key 都齐才 Ready；否则 Create 仍走 TestAdapter。
type RemoteAdapter struct {
	NameValue string
	BaseURL   string
	APIKey    string
	HTTP      *http.Client
}

func (a RemoteAdapter) Name() string { return a.NameValue }

func (a RemoteAdapter) Ready() bool {
	return strings.TrimSpace(a.BaseURL) != "" && strings.TrimSpace(a.APIKey) != ""
}

func (a RemoteAdapter) client() *http.Client {
	if a.HTTP != nil {
		return a.HTTP
	}
	return remoteHTTP
}

func (a RemoteAdapter) Create(ctx context.Context, in SubmitInput) (SubmitResult, error) {
	if !a.Ready() {
		return SubmitResult{}, fmt.Errorf("%s unavailable", a.NameValue)
	}
	if in.Kind == KindImage {
		return a.createImage(ctx, in)
	}
	switch a.NameValue {
	case "openrouter":
		return a.createOpenRouter(ctx, in)
	default:
		return a.createArk(ctx, in)
	}
}

func (a RemoteAdapter) Cancel(ctx context.Context, upstreamID string) error {
	if !a.Ready() || strings.TrimSpace(upstreamID) == "" {
		return nil
	}
	path := "/contents/generations/tasks/" + url.PathEscape(upstreamID) + "/cancel"
	if a.NameValue == "openrouter" {
		path = "/videos/" + url.PathEscape(upstreamID) + "/cancel"
	}
	_, _, _ = a.doJSON(ctx, http.MethodPost, path, map[string]any{})
	return nil
}

func (a RemoteAdapter) Get(ctx context.Context, upstreamID string) (SubmitResult, error) {
	if !a.Ready() {
		return SubmitResult{}, fmt.Errorf("%s unavailable", a.NameValue)
	}
	path := "/contents/generations/tasks/" + url.PathEscape(upstreamID)
	if a.NameValue == "openrouter" {
		path = "/videos/" + url.PathEscape(upstreamID)
	}
	raw, code, err := a.doJSON(ctx, http.MethodGet, path, nil)
	if err != nil {
		return SubmitResult{}, err
	}
	if code >= 300 {
		return SubmitResult{}, fmt.Errorf("%s get %d: %s", a.NameValue, code, truncate(raw, 240))
	}
	parsed := map[string]any{}
	_ = json.Unmarshal(raw, &parsed)
	result := SubmitResult{
		UpstreamID:  firstString(parsed, "id", "task_id", "generation_id"),
		Status:      mapRemoteStatus(firstString(parsed, "status", "task_status", "state")),
		Progress:    jsonInt(parsed["progress"]),
		Usage:       usageFromRemote(parsed),
		ContentType: "video/mp4",
	}
	if result.UpstreamID == "" {
		result.UpstreamID = upstreamID
	}
	if result.Status == StatusCompleted {
		if result.Progress == 0 {
			result.Progress = 100
		}
		body, ctype, err := a.downloadOutput(ctx, parsed, a.NameValue == "openrouter", upstreamID)
		if err != nil {
			return SubmitResult{}, err
		}
		result.Content = body
		if ctype != "" {
			result.ContentType = ctype
		}
	}
	return result, nil
}

func (a RemoteAdapter) createArk(ctx context.Context, in SubmitInput) (SubmitResult, error) {
	model := strings.TrimSpace(in.Model)
	if model == "" {
		return SubmitResult{}, fmt.Errorf("ark model required")
	}
	content := []map[string]any{{"type": "text", "text": in.Prompt}}
	for _, img := range arkImages(in) {
		item := map[string]any{
			"type":      "image_url",
			"image_url": map[string]string{"url": img.URL},
		}
		if img.Role != "" {
			item["role"] = img.Role
		}
		content = append(content, item)
	}
	if in.ReferenceVideo != "" {
		content = append(content, map[string]any{
			"type":      "video_url",
			"video_url": map[string]string{"url": in.ReferenceVideo},
		})
	}
	if in.ReferenceAudio != "" {
		content = append(content, map[string]any{
			"type":      "audio_url",
			"audio_url": map[string]string{"url": in.ReferenceAudio},
		})
	}
	body := map[string]any{"model": model, "content": content, "watermark": false}
	if in.Resolution != "" {
		body["resolution"] = in.Resolution
	}
	if in.AspectRatio != "" {
		body["ratio"] = in.AspectRatio
	}
	if in.Duration > 0 {
		body["duration"] = in.Duration
	}
	raw, code, err := a.doJSON(ctx, http.MethodPost, "/contents/generations/tasks", body)
	if err != nil {
		return SubmitResult{}, err
	}
	if code >= 300 {
		return SubmitResult{}, fmt.Errorf("ark create %d: %s", code, truncate(raw, 240))
	}
	return parseCreateResult(raw)
}

func (a RemoteAdapter) createOpenRouter(ctx context.Context, in SubmitInput) (SubmitResult, error) {
	model := strings.TrimSpace(in.Model)
	if model == "" {
		return SubmitResult{}, fmt.Errorf("openrouter model required")
	}
	body := map[string]any{"model": model, "prompt": in.Prompt}
	if in.Duration > 0 {
		body["duration"] = in.Duration
	}
	if in.Resolution != "" {
		body["resolution"] = in.Resolution
	}
	if in.AspectRatio != "" {
		body["aspect_ratio"] = in.AspectRatio
	}
	body["generate_audio"] = in.Audio
	if frames := openRouterFrames(in); len(frames) > 0 {
		body["frame_images"] = frames
	}
	raw, code, err := a.doJSON(ctx, http.MethodPost, "/videos", body)
	if err != nil {
		return SubmitResult{}, err
	}
	if code >= 300 {
		return SubmitResult{}, fmt.Errorf("openrouter create %d: %s", code, truncate(raw, 240))
	}
	return parseCreateResult(raw)
}

func (a RemoteAdapter) createImage(ctx context.Context, in SubmitInput) (SubmitResult, error) {
	model := strings.TrimSpace(in.Model)
	body := map[string]any{"model": model, "prompt": in.Prompt, "n": 1}
	raw, code, err := a.doJSON(ctx, http.MethodPost, "/images/generations", body)
	if err != nil {
		return SubmitResult{}, err
	}
	if code >= 300 {
		return SubmitResult{}, fmt.Errorf("%s image %d: %s", a.NameValue, code, truncate(raw, 240))
	}
	parsed := map[string]any{}
	_ = json.Unmarshal(raw, &parsed)
	id := firstString(parsed, "id")
	data, _ := parsed["data"].([]any)
	var fileURL, b64 string
	if len(data) > 0 {
		if row, ok := data[0].(map[string]any); ok {
			fileURL = firstString(row, "url")
			b64 = firstString(row, "b64_json")
		}
	}
	result := SubmitResult{
		UpstreamID:  id,
		Status:      StatusCompleted,
		Progress:    100,
		ContentType: "image/png",
		Usage:       map[string]int{"image_count": 1},
	}
	if result.UpstreamID == "" {
		result.UpstreamID = in.JobID
	}
	if b64 != "" {
		decoded, err := base64.StdEncoding.DecodeString(b64)
		if err != nil {
			return SubmitResult{}, err
		}
		result.Content = decoded
		return result, nil
	}
	if fileURL != "" {
		body, ctype, err := a.getBytes(ctx, fileURL)
		if err != nil {
			return SubmitResult{}, err
		}
		result.Content = body
		if ctype != "" {
			result.ContentType = ctype
		}
		return result, nil
	}
	return SubmitResult{}, fmt.Errorf("%s image missing output", a.NameValue)
}

func parseCreateResult(raw []byte) (SubmitResult, error) {
	parsed := map[string]any{}
	_ = json.Unmarshal(raw, &parsed)
	id := firstString(parsed, "id", "task_id", "generation_id")
	if id == "" {
		if data, ok := parsed["data"].(map[string]any); ok {
			id = firstString(data, "id", "task_id")
		}
	}
	if id == "" {
		return SubmitResult{}, fmt.Errorf("upstream missing task id")
	}
	status := mapRemoteStatus(firstString(parsed, "status", "task_status", "state"))
	if status == "" {
		status = StatusInProgress
	}
	progress := jsonInt(parsed["progress"])
	if status == StatusInProgress && progress == 0 {
		progress = 10
	}
	return SubmitResult{
		UpstreamID:  id,
		Status:      status,
		Progress:    progress,
		Usage:       usageFromRemote(parsed),
		ContentType: "video/mp4",
	}, nil
}

func (a RemoteAdapter) downloadOutput(ctx context.Context, parsed map[string]any, openRouter bool, upstreamID string) ([]byte, string, error) {
	fileURL := remoteFileURL(parsed)
	if fileURL != "" {
		return a.getBytes(ctx, fileURL)
	}
	if openRouter {
		raw, code, err := a.doJSON(ctx, http.MethodGet, "/videos/"+url.PathEscape(upstreamID)+"/content", nil)
		if err != nil {
			return nil, "", err
		}
		if code < 300 && len(raw) > 0 && !json.Valid(raw) {
			return raw, "video/mp4", nil
		}
		if code < 300 {
			nested := map[string]any{}
			_ = json.Unmarshal(raw, &nested)
			if u := remoteFileURL(nested); u != "" {
				return a.getBytes(ctx, u)
			}
		}
	}
	return nil, "", fmt.Errorf("upstream completed without video url")
}

func (a RemoteAdapter) doJSON(ctx context.Context, method, path string, body any) ([]byte, int, error) {
	var rdr io.Reader
	if body != nil && method != http.MethodGet {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, 0, err
		}
		rdr = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, joinURL(a.BaseURL, path), rdr)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(a.APIKey))
	if body != nil && method != http.MethodGet {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := a.client().Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxProviderBody))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return raw, resp.StatusCode, nil
}

func (a RemoteAdapter) getBytes(ctx context.Context, rawURL string) ([]byte, string, error) {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, "", fmt.Errorf("invalid asset url")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", err
	}
	resp, err := a.client().Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("asset download %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxAssetBody))
	if err != nil {
		return nil, "", err
	}
	ctype := resp.Header.Get("Content-Type")
	if i := strings.Index(ctype, ";"); i >= 0 {
		ctype = strings.TrimSpace(ctype[:i])
	}
	return raw, ctype, nil
}

type arkImage struct {
	URL  string
	Role string
}

func arkImages(in SubmitInput) []arkImage {
	var out []arkImage
	switch in.TaskType {
	case TaskFirstLastFrame:
		if in.FirstFrame != "" {
			out = append(out, arkImage{URL: in.FirstFrame, Role: "first_frame"})
		}
		if in.LastFrame != "" {
			out = append(out, arkImage{URL: in.LastFrame, Role: "last_frame"})
		}
	case TaskI2V, TaskFirstFrame:
		url := in.FirstFrame
		if url == "" && len(in.Images) > 0 {
			url = in.Images[0]
		}
		if url != "" {
			out = append(out, arkImage{URL: url, Role: "first_frame"})
		}
	default:
		for _, img := range in.Images {
			out = append(out, arkImage{URL: img, Role: "reference_image"})
		}
		if in.FirstFrame != "" {
			out = append(out, arkImage{URL: in.FirstFrame, Role: "first_frame"})
		}
		if in.LastFrame != "" {
			out = append(out, arkImage{URL: in.LastFrame, Role: "last_frame"})
		}
	}
	return out
}

func openRouterFrames(in SubmitInput) []map[string]any {
	var frames []map[string]any
	if in.FirstFrame != "" {
		frames = append(frames, map[string]any{"type": "image_url", "image_url": map[string]string{"url": in.FirstFrame}})
	} else if len(in.Images) > 0 {
		frames = append(frames, map[string]any{"type": "image_url", "image_url": map[string]string{"url": in.Images[0]}})
	}
	if in.LastFrame != "" {
		frames = append(frames, map[string]any{"type": "image_url", "image_url": map[string]string{"url": in.LastFrame}})
	}
	return frames
}

func mapRemoteStatus(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "succeeded", "success", "completed", "complete":
		return StatusCompleted
	case "failed", "error", "expired":
		return StatusFailed
	case "cancelled", "canceled", "cancelled_by_user":
		return StatusCancelled
	case "queued", "pending", "running", "in_progress", "processing", "submitted", "":
		return StatusInProgress
	default:
		return StatusInProgress
	}
}

func usageFromRemote(parsed map[string]any) map[string]int {
	out := map[string]int{}
	if u, ok := parsed["usage"].(map[string]any); ok {
		if n := jsonInt(u["prompt_tokens"]); n > 0 {
			out["prompt_tokens"] = n
		}
		if n := jsonInt(u["completion_tokens"]); n > 0 {
			out["completion_tokens"] = n
		}
		if n := jsonInt(u["video_seconds"]); n > 0 {
			out["video_seconds"] = n
		}
		if n := jsonInt(u["image_count"]); n > 0 {
			out["image_count"] = n
		}
		if n := jsonInt(u["audio_seconds"]); n > 0 {
			out["audio_seconds"] = n
		}
	}
	if n := jsonInt(parsed["duration"]); n > 0 && out["video_seconds"] == 0 {
		out["video_seconds"] = n
	}
	return out
}

func remoteFileURL(parsed map[string]any) string {
	if u := firstString(parsed, "video_url", "url"); u != "" {
		return u
	}
	if content, ok := parsed["content"].(map[string]any); ok {
		if u := firstString(content, "video_url", "url"); u != "" {
			return u
		}
	}
	if output, ok := parsed["output"].(map[string]any); ok {
		if u := firstString(output, "video_url", "url"); u != "" {
			return u
		}
	}
	for _, key := range []string{"unsigned_urls", "urls"} {
		if arr, ok := parsed[key].([]any); ok && len(arr) > 0 {
			if s, ok := arr[0].(string); ok && s != "" {
				return s
			}
		}
	}
	return ""
}

func firstString(m map[string]any, keys ...string) string {
	for _, key := range keys {
		switch v := m[key].(type) {
		case string:
			if strings.TrimSpace(v) != "" {
				return v
			}
		}
	}
	return ""
}

func jsonInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	case string:
		var i int
		_, _ = fmt.Sscanf(n, "%d", &i)
		return i
	default:
		return 0
	}
}

func joinURL(base, path string) string {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return base + path
}

func truncate(raw []byte, n int) string {
	s := strings.TrimSpace(string(raw))
	if len(s) <= n {
		return s
	}
	return s[:n]
}
