package modelruntime

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/maximhq/bifrost/core/schemas"
)

// The pinned Bifrost xAI provider has no native image edit or video adapter.
// This uses only the approved destination configured for the selected account.
func (b *Bifrost) grokMedia(ctx context.Context, a Attempt, r Request, credential SubscriptionCredential, out Result) (Result, error) {
	method := http.MethodPost
	var body []byte
	if r.Operation == VideoRetrieve || r.Operation == VideoDownload {
		method = http.MethodGet
	} else {
		var err error
		body, err = grokMediaInput(a, r)
		if err != nil {
			return out, err
		}
	}
	raw, contentType, out, err := b.requestSubscriptionMedia(ctx, credential, method, body, out)
	if err != nil {
		return out, err
	}
	if r.Operation == VideoDownload && strings.HasPrefix(contentType, "video/") {
		if http.DetectContentType(raw) != "video/mp4" {
			return out, ErrUsageUnknown
		}
		out.Binary, out.ContentType, out.TaskID, out.State = raw, "video/mp4", a.TaskID, "completed"
		return out, nil
	}
	if r.Operation.IsImage() {
		var images schemas.BifrostImageGenerationResponse
		if json.Unmarshal(raw, &images) != nil || len(images.Data) == 0 {
			return out, &UpstreamFailure{}
		}
		return b.finishResponse(ctx, &images, a, r, out)
	}
	return b.grokVideoResult(ctx, a, r, raw, out)
}

func grokMediaInput(a Attempt, r Request) ([]byte, error) {
	var input struct {
		Prompt         string               `json:"prompt"`
		Images         []schemas.ImageInput `json:"images"`
		InputReference string               `json:"input_reference"`
		VideoURI       string               `json:"video_uri"`
	}
	if json.Unmarshal(r.Input, &input) != nil || strings.TrimSpace(input.Prompt) == "" || r.Stream {
		return nil, ErrInvalidRequest
	}
	var params map[string]json.RawMessage
	if json.Unmarshal(r.Params, &params) != nil {
		return nil, ErrInvalidRequest
	}
	payload := map[string]any{"model": a.UpstreamModel, "prompt": input.Prompt}
	if r.VideoAction != "" {
		if r.Operation != Video || r.VideoAction != a.VideoAction || a.UpstreamModel != grokVideoEditModel || input.VideoURI == "" ||
			r.VideoAction != VideoEdit && r.VideoAction != VideoExtend {
			return nil, ErrUnsupported
		}
		payload["video"] = map[string]string{"url": input.VideoURI}
		if r.VideoAction == VideoExtend {
			payload["duration"] = params["duration"]
		}
		return json.Marshal(payload)
	}
	if input.VideoURI != "" {
		return nil, ErrInvalidRequest
	}
	for name, value := range params {
		switch name {
		case "aspect_ratio", "resolution":
			payload[name] = value
		case "size":
			var size string
			if json.Unmarshal(value, &size) != nil {
				return nil, ErrInvalidRequest
			}
			if size != "" && size != "auto" {
				ratio, resolution, err := grokMediaSize(size, r.Operation)
				if err != nil || params["aspect_ratio"] != nil || params["resolution"] != nil {
					return nil, ErrUnsupported
				}
				payload["aspect_ratio"], payload["resolution"] = ratio, resolution
			}
		case "seconds":
			if r.Operation != Video {
				return nil, ErrUnsupported
			}
			seconds, err := strconv.Atoi(strings.Trim(string(value), "\""))
			if err != nil || seconds < 1 || seconds > 15 {
				return nil, ErrInvalidRequest
			}
			payload["duration"] = seconds
		case "n", "quality", "response_format":
			if !r.Operation.IsImage() {
				return nil, ErrUnsupported
			}
			payload[name] = value
		default:
			return nil, ErrUnsupported
		}
	}
	if r.Operation == Video && input.InputReference != "" {
		if !validMediaURL(input.InputReference) && !(strings.HasPrefix(input.InputReference, "data:image/") && len(assetBytes(input.InputReference)) > 0) {
			return nil, ErrInvalidRequest
		}
		payload["image"] = map[string]string{"url": input.InputReference}
	}
	if r.Operation == ImageEdit {
		if len(input.Images) == 0 || len(input.Images) > 5 {
			return nil, ErrInvalidRequest
		}
		images := make([]map[string]string, 0, len(input.Images))
		for _, image := range input.Images {
			target := image.URL
			if target == "" {
				mime := http.DetectContentType(image.Image)
				if !strings.HasPrefix(mime, "image/") {
					return nil, ErrInvalidRequest
				}
				target = "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(image.Image)
			}
			images = append(images, map[string]string{"url": target})
		}
		if len(images) == 1 {
			payload["image"] = images[0]
		} else {
			payload["images"] = images
		}
	}
	return json.Marshal(payload)
}

func grokMediaSize(size string, op Operation) (string, string, error) {
	var w, h int64
	if !parseSize(size, &w, &h) {
		return "", "", ErrInvalidRequest
	}
	divisor, remainder := w, h
	for remainder != 0 {
		divisor, remainder = remainder, divisor%remainder
	}
	ratio := strconv.FormatInt(w/divisor, 10) + ":" + strconv.FormatInt(h/divisor, 10)
	shorter := min(w, h)
	if op == Video {
		if shorter == 480 || shorter == 720 || shorter == 1080 {
			return ratio, strconv.FormatInt(shorter, 10) + "p", nil
		}
		return "", "", ErrUnsupported
	}
	switch max(w, h) {
	case 1024:
		return ratio, "1k", nil
	case 2048:
		return ratio, "2k", nil
	default:
		return "", "", ErrUnsupported
	}
}

func (b *Bifrost) grokVideoResult(ctx context.Context, a Attempt, r Request, raw []byte, out Result) (Result, error) {
	var task struct {
		ID     string `json:"request_id"`
		Status string `json:"status"`
		Video  struct {
			URL      string      `json:"url"`
			Duration json.Number `json:"duration"`
			Width    int         `json:"width"`
			Height   int         `json:"height"`
		} `json:"video"`
	}
	if json.Unmarshal(raw, &task) != nil || task.ID != "" && a.TaskID != "" && task.ID != a.TaskID {
		return out, &UpstreamFailure{}
	}
	if task.ID == "" {
		task.ID = a.TaskID
	}
	if task.ID == "" || task.Status == "failed" || task.Status == "expired" {
		return out, &UpstreamFailure{}
	}
	if task.Status != "done" && task.Status != "completed" {
		out.TaskID, out.State = task.ID, "submitted"
		if r.Operation == VideoDownload {
			return out, ErrUsageUnknown
		}
		return out, nil
	}
	if task.Video.URL == "" {
		return out, ErrUsageUnknown
	}
	clip, _ := b.fetchMedia(ctx, task.Video.URL, 32<<20)
	if http.DetectContentType(clip) == "video/mp4" {
		out.Binary, out.ContentType = clip, "video/mp4"
	}
	if r.Operation == VideoDownload {
		if len(out.Binary) == 0 {
			return out, ErrUsageUnknown
		}
		out.TaskID, out.State = task.ID, "completed"
		return out, nil
	}
	duration := string(task.Video.Duration)
	if seconds, ok := numeric(task.Video.Duration); !ok || seconds.Sign() <= 0 {
		measured := mp4Seconds(clip)
		if measured == nil || measured.Sign() <= 0 {
			out.TaskID, out.State = task.ID, "submitted"
			return out, nil
		}
		duration = measured.FloatString(18)
	}
	native := map[string]any{"status": "completed", "duration": duration}
	response := &schemas.BifrostVideoGenerationResponse{
		ID: task.ID, Model: a.UpstreamModel, Object: "video", Status: schemas.VideoStatusCompleted,
		Videos:      []schemas.VideoOutput{{Type: "url", URL: &task.Video.URL, ContentType: "video/mp4"}},
		ExtraFields: schemas.BifrostResponseExtraFields{RawResponse: native},
		Seconds:     &duration,
	}
	if task.Video.Width > 0 && task.Video.Height > 0 {
		response.Size = strconv.Itoa(task.Video.Width) + "x" + strconv.Itoa(task.Video.Height)
	} else {
		response.Size = mp4DisplaySize(clip)
	}
	if response.Size != "" {
		native["size"] = response.Size
	}
	if profile := a.Capability.Billing; profile != nil && response.Size == "" {
		for _, item := range profile.Output {
			for _, variant := range item.Variants {
				if item.Modality == "video" && variant.Size != "" {
					return out, ErrUsageUnknown
				}
			}
		}
	}
	result, err := b.finishResponse(ctx, response, a, r, out)
	if result.State == "completed" {
		// Clients retrieve the task through the original key. The signed CDN
		// link is used internally to verify and retrieve the video.
		response.Videos = nil
		result.Output, _ = publicOutput(response, r.PublicModel)
	}
	return result, err
}

func subscriptionRetryHeader(value string) *time.Time {
	value = strings.TrimSpace(value)
	if seconds, err := strconv.ParseInt(value, 10, 32); err == nil && seconds > 0 {
		at := time.Now().Add(time.Duration(seconds) * time.Second)
		return &at
	}
	if at, err := http.ParseTime(value); err == nil && at.After(time.Now()) {
		return &at
	}
	return nil
}
