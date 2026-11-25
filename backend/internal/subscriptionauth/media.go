package subscriptionauth

import (
	"net/url"
	"regexp"
	"strings"
)

const (
	ImageGeneration = "image_generation"
	ImageEdit       = "image_edit"
	VideoGeneration = "video_generation"
	VideoEdit       = "video_edit"
	VideoExtend     = "video_extension"
	VideoRetrieve   = "video_retrieve"
	VideoDownload   = "video_download"
)

type MediaEndpoint struct {
	URL       string `json:"url"`
	Protocol  string `json:"protocol,omitempty"`
	Streaming bool   `json:"streaming,omitempty"`
}

func MediaAllowed(kind, operation string) bool {
	switch operation {
	case ImageGeneration, ImageEdit:
		return kind == Codex || kind == Grok || kind == Gemini || kind == Antigravity
	case VideoGeneration, VideoEdit, VideoExtend, VideoRetrieve, VideoDownload:
		return kind == Grok
	default:
		return false
	}
}

func (c Connection) MediaEndpoint(operation string) (MediaEndpoint, bool) {
	e, ok := c.Media[operation]
	if ok && e.Protocol == "" {
		e.Protocol = "openai"
	}
	if ok {
		return e, true
	}
	return c.presetMediaEndpoint(operation)
}

func (c Connection) presetMediaEndpoint(operation string) (MediaEndpoint, bool) {
	if c.MediaPreset == "" {
		return MediaEndpoint{}, false
	}
	base, ok := c.inferenceBase()
	if !ok {
		return MediaEndpoint{}, false
	}
	switch operation {
	case ImageGeneration, ImageEdit:
		if c.MediaPreset == "gemini" {
			root, ok := strings.CutSuffix(base, "/endpoints/openapi")
			if ok {
				return MediaEndpoint{URL: root + "/publishers/google/models/{model}:generateContent", Protocol: "gemini"}, true
			}
			root, ok = strings.CutSuffix(base, "/openai")
			if ok {
				return MediaEndpoint{URL: root + "/models/{model}:generateContent", Protocol: "gemini"}, true
			}
			return MediaEndpoint{}, false
		}
		path := "/images/generations"
		if operation == ImageEdit {
			path = "/images/edits"
		}
		protocol := c.MediaPreset
		if protocol == "codex" {
			protocol = "openai"
		}
		return MediaEndpoint{URL: base + path, Protocol: protocol, Streaming: protocol == "openai"}, true
	case VideoGeneration, VideoEdit, VideoExtend, VideoRetrieve, VideoDownload:
		if c.MediaPreset != "xai" {
			return MediaEndpoint{}, false
		}
		switch operation {
		case VideoGeneration:
			return MediaEndpoint{URL: base + "/videos/generations", Protocol: "xai"}, true
		case VideoEdit:
			return MediaEndpoint{URL: base + "/videos/edits", Protocol: "xai"}, true
		case VideoExtend:
			return MediaEndpoint{URL: base + "/videos/extensions", Protocol: "xai"}, true
		case VideoRetrieve:
			return MediaEndpoint{URL: base + "/videos/{task_id}", Protocol: "xai"}, true
		}
	}
	return MediaEndpoint{}, false
}

// Derive sibling endpoints only from the same configured inference origin
// and path prefix. Query parameters cannot be inherited or reinterpreted.
func (c Connection) inferenceBase() (string, bool) {
	u, err := url.Parse(c.InferenceURL)
	if err != nil || u.Host == "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.User != nil {
		return "", false
	}
	for _, suffix := range []string{"/responses", "/chat/completions", "/messages"} {
		if base, ok := strings.CutSuffix(c.InferenceURL, suffix); ok {
			return base, true
		}
	}
	return "", false
}

func (c Connection) validateMedia(kind string, loopback bool) error {
	switch c.MediaPreset {
	case "":
	case "openai":
		if !MediaAllowed(kind, ImageGeneration) {
			return ErrOAuthNotConfigured
		}
	case "codex":
		if kind != Codex || !NativeCodexEndpoint(c.InferenceURL) {
			return ErrOAuthNotConfigured
		}
	case "xai":
		if kind != Grok {
			return ErrOAuthNotConfigured
		}
	case "gemini":
		if kind != Gemini && kind != Antigravity {
			return ErrOAuthNotConfigured
		}
	default:
		return ErrOAuthNotConfigured
	}
	if c.MediaPreset != "" {
		if _, ok := c.presetMediaEndpoint(ImageGeneration); !ok {
			return ErrOAuthNotConfigured
		}
	}
	for op := range c.Media {
		if !MediaAllowed(kind, op) {
			return ErrOAuthNotConfigured
		}
	}
	for _, op := range []string{ImageGeneration, ImageEdit, VideoGeneration, VideoEdit, VideoExtend, VideoRetrieve, VideoDownload} {
		e, ok := c.MediaEndpoint(op)
		if !ok {
			continue
		}
		if !MediaAllowed(kind, op) || !validEndpoint(e.URL, loopback) ||
			e.Streaming && (e.Protocol != "openai" || op != ImageGeneration && op != ImageEdit) {
			return ErrOAuthNotConfigured
		}
		if (op == VideoEdit || op == VideoExtend) && e.Protocol != "xai" {
			return ErrOAuthNotConfigured
		}
		u, _ := url.Parse(e.URL)
		want := map[string]string{
			ImageGeneration: "/images/generations", ImageEdit: "/images/edits",
			VideoGeneration: "/videos", VideoEdit: "/videos/edits", VideoExtend: "/videos/extensions",
			VideoRetrieve: "/videos/{task_id}", VideoDownload: "/videos/{task_id}/content",
		}[op]
		switch e.Protocol {
		case "openai":
		case "gemini":
			if kind != Gemini && kind != Antigravity {
				return ErrOAuthNotConfigured
			}
			want = "/{model}:generateContent"
		case "xai":
			if kind != Grok {
				return ErrOAuthNotConfigured
			}
			if op == VideoGeneration {
				want = "/videos/generations"
			}
		default:
			return ErrOAuthNotConfigured
		}
		if u.RawQuery != "" || !strings.HasSuffix(u.Path, want) ||
			strings.ContainsAny(strings.TrimSuffix(e.URL, want), "{}") ||
			!strings.HasSuffix(e.URL, want) {
			return ErrOAuthNotConfigured
		}
	}
	create, hasCreate := c.MediaEndpoint(VideoGeneration)
	retrieve, hasRetrieve := c.MediaEndpoint(VideoRetrieve)
	download, hasDownload := c.MediaEndpoint(VideoDownload)
	_, hasEdit := c.MediaEndpoint(VideoEdit)
	_, hasExtend := c.MediaEndpoint(VideoExtend)
	if hasCreate != hasRetrieve || hasRetrieve && create.Protocol != retrieve.Protocol ||
		hasCreate && create.Protocol == "openai" && !hasDownload ||
		(hasEdit || hasExtend) && (!hasRetrieve || retrieve.Protocol != "xai") ||
		hasDownload && (!hasCreate || download.Protocol != create.Protocol) {
		return ErrOAuthNotConfigured
	}
	return nil
}

func NativeCodexEndpoint(endpoint string) bool {
	switch endpoint {
	case "https://chatgpt.com/backend-api/codex/responses",
		"https://chatgpt.com/backend-api/codex/images/generations",
		"https://chatgpt.com/backend-api/codex/images/edits":
		return true
	}
	return false
}

var mediaPathID = regexp.MustCompile(`^[a-zA-Z0-9_-][a-zA-Z0-9._-]{0,255}$`)
var discoveredModelID = regexp.MustCompile(`^[a-zA-Z0-9_-][a-zA-Z0-9._-]{0,95}(/[a-zA-Z0-9_-][a-zA-Z0-9._-]{0,95}){0,2}$`)

func (e MediaEndpoint) Resolve(model, task string) (string, error) {
	if strings.Contains(e.URL, "{model}") {
		model = strings.TrimPrefix(model, "google/")
		if !mediaPathID.MatchString(model) {
			return "", ErrCredential
		}
		return strings.Replace(e.URL, "{model}", model, 1), nil
	}
	if strings.Contains(e.URL, "{task_id}") {
		if !mediaPathID.MatchString(task) {
			return "", ErrCredential
		}
		return strings.Replace(e.URL, "{task_id}", task, 1), nil
	}
	return e.URL, nil
}
