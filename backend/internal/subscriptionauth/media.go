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
	case VideoGeneration, VideoRetrieve, VideoDownload:
		return kind == Grok
	default:
		return false
	}
}

func (c Connection) MediaEndpoint(operation string) (MediaEndpoint, bool) {
	e, ok := c.Media[operation]
	if e.Protocol == "" {
		e.Protocol = "openai"
	}
	return e, ok
}

func (c Connection) validateMedia(kind string, loopback bool) error {
	for op := range c.Media {
		e, _ := c.MediaEndpoint(op)
		if !MediaAllowed(kind, op) || !validEndpoint(e.URL, loopback) ||
			e.Streaming && (e.Protocol != "openai" || op != ImageGeneration && op != ImageEdit) {
			return ErrOAuthNotConfigured
		}
		u, _ := url.Parse(e.URL)
		want := map[string]string{
			ImageGeneration: "/images/generations", ImageEdit: "/images/edits",
			VideoGeneration: "/videos", VideoRetrieve: "/videos/{task_id}", VideoDownload: "/videos/{task_id}/content",
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
	if hasCreate != hasRetrieve || hasRetrieve && create.Protocol != retrieve.Protocol ||
		hasCreate && create.Protocol == "openai" && !hasDownload ||
		hasDownload && (!hasCreate || download.Protocol != create.Protocol) {
		return ErrOAuthNotConfigured
	}
	return nil
}

var mediaPathID = regexp.MustCompile(`^[a-zA-Z0-9_-][a-zA-Z0-9._-]{0,255}$`)

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
