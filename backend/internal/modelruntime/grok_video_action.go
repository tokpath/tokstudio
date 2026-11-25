package modelruntime

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/tokpath/tokstudio/backend/internal/subscriptionauth"
)

const grokVideoEditModel = "grok-imagine-video"

// Both operations use the published video capability and the existing task
// lifecycle. The verified provider contract determines the available actions.
func GrokVideoActions(model string, connection subscriptionauth.Connection) []Operation {
	if model != grokVideoEditModel {
		return nil
	}
	var actions []Operation
	for _, action := range []Operation{VideoEdit, VideoExtend} {
		if media, ok := connection.MediaEndpoint(string(action)); ok && media.Protocol == "xai" {
			actions = append(actions, action)
		}
	}
	return actions
}

func decodeGrokVideoAction(r Request, fields map[string]json.RawMessage) (Request, error) {
	if r.Stream {
		return Request{}, ErrUnsupported
	}
	var source struct {
		URL string `json:"url"`
	}
	var prompt string
	if json.Unmarshal(fields["prompt"], &prompt) != nil || strings.TrimSpace(prompt) == "" ||
		decodePreserved(fields["video"], &source) != nil {
		return Request{}, ErrInvalidRequest
	}
	if !validMediaURL(source.URL) && !(strings.HasPrefix(source.URL, "data:video/mp4;base64,") &&
		http.DetectContentType(assetBytes(source.URL)) == "video/mp4") {
		return Request{}, ErrInvalidRequest
	}
	r.VideoAction, r.Operation = r.Operation, Video
	r.Input, _ = json.Marshal(map[string]string{"prompt": prompt, "video_uri": source.URL})
	r.InputTypes = []string{"text", "video"}
	delete(fields, "prompt")
	delete(fields, "video")
	if r.VideoAction == VideoExtend {
		if len(fields) > 1 || len(fields) == 1 && fields["duration"] == nil {
			return Request{}, ErrUnsupported
		}
		seconds := 6
		if raw := fields["duration"]; raw != nil && (json.Unmarshal(raw, &seconds) != nil || string(raw) == "null" || seconds < 2 || seconds > 10) {
			return Request{}, ErrInvalidRequest
		}
		fields["duration"], _ = json.Marshal(seconds)
	} else if len(fields) != 0 {
		return Request{}, ErrUnsupported
	}
	r.Params, _ = json.Marshal(fields)
	return r, nil
}

// GrokVideoActionCapability reserves all potentially billable seconds of the
// completed clip. xAI accepts up to 15 seconds of source video for extension.
func GrokVideoActionCapability(c Capability, r Request) (Capability, bool) {
	if c.Billing == nil {
		return Capability{}, false
	}
	limit := "8.7"
	if r.VideoAction == VideoExtend {
		var params struct{ Duration int }
		_ = json.Unmarshal(r.Params, &params)
		limit = strconv.Itoa(15 + params.Duration)
	}
	for _, item := range c.Billing.Output {
		if item.Modality == "video" && item.Template == "seconds" {
			profile := BillingProfile{Output: []BillingItem{item}}
			selected := profile.Capability(Video, false)
			for key := range selected.BillableBounds {
				selected.BillableBounds[key] = limit
			}
			return selected, true
		}
	}
	return Capability{}, false
}
