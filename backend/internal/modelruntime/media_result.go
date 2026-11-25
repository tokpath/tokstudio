package modelruntime

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/maximhq/bifrost/core/schemas"
)

func (b *Bifrost) fetchMedia(ctx context.Context, target string, limit int64) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	u, err := url.Parse(target)
	if err != nil || u.Hostname() == "" || u.User != nil || u.Fragment != "" ||
		u.Scheme != "https" && !(b.config.AllowTestLoopback && u.Scheme == "http") ||
		b.config.ResourceGuard == nil || b.config.ResourceGuard(ctx, target) != nil {
		return nil, ErrUsageUnknown
	}
	client := b.mediaClient()
	defer client.CloseIdleConnections()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, ErrUsageUnknown
	}
	// Result links can be issued by a CDN and are fetched without credentials.
	resp, err := client.Do(req)
	if err != nil {
		return nil, ErrUsageUnknown
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, ErrUsageUnknown
	}
	return readMediaBody(resp.Body, limit)
}

func readMediaBody(body io.Reader, limit int64) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(body, limit+1))
	if err != nil || len(raw) == 0 || int64(len(raw)) > limit {
		return nil, ErrUsageUnknown
	}
	return raw, nil
}

// Async results must be usable through our retention window even after a
// provider's signed URL has expired. Keep the bytes and strip remote links.
func (b *Bifrost) retainImageResponse(ctx context.Context, response any) (any, error) {
	original, ok := response.(*schemas.BifrostImageGenerationResponse)
	if !ok || len(original.Data) == 0 {
		return nil, ErrUsageUnknown
	}
	retained := *original
	retained.Data = append([]schemas.ImageData(nil), original.Data...)
	remaining := int64(23 << 20)
	for i := range retained.Data {
		result := &retained.Data[i]
		var raw []byte
		var err error
		if result.B64JSON != "" {
			if base64.StdEncoding.DecodedLen(len(result.B64JSON)) > int(remaining)+2 {
				return nil, ErrUsageUnknown
			}
			raw, err = base64.StdEncoding.DecodeString(result.B64JSON)
		} else {
			raw, err = b.fetchMedia(ctx, result.URL, remaining)
		}
		if err != nil || int64(len(raw)) > remaining {
			return nil, ErrUsageUnknown
		}
		cfg, _, err := image.DecodeConfig(bytes.NewReader(raw))
		if err != nil || cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > 65536 || cfg.Height > 65536 {
			return nil, ErrUsageUnknown
		}
		remaining -= int64(len(raw))
		result.B64JSON, result.URL = base64.StdEncoding.EncodeToString(raw), ""
	}
	return &retained, nil
}
