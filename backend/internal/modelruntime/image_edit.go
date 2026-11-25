package modelruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"net/url"
	"strings"

	"github.com/maximhq/bifrost/core/schemas"
)

// DecodeImageEdit puts JSON image references and multipart uploads into the
// same Bifrost input. The mask is a parameter, not an extra output image.
func decodeImageEdit(fields map[string]json.RawMessage) (json.RawMessage, error) {
	var input schemas.ImageEditInput
	if json.Unmarshal(fields["prompt"], &input.Prompt) != nil || strings.TrimSpace(input.Prompt) == "" {
		return nil, ErrInvalidRequest
	}
	images := fields["images"]
	if image, exists := fields["image"]; exists {
		if len(images) > 0 {
			return nil, ErrInvalidRequest
		}
		images = image
	}
	if len(bytes.TrimSpace(images)) == 0 {
		return nil, ErrInvalidRequest
	}
	var items []json.RawMessage
	if bytes.TrimSpace(images)[0] == '[' {
		if json.Unmarshal(images, &items) != nil || len(items) == 0 || len(items) > 10 {
			return nil, ErrInvalidRequest
		}
	} else {
		items = []json.RawMessage{images}
	}
	for _, raw := range items {
		var image schemas.ImageInput
		if json.Unmarshal(raw, &image) != nil {
			return nil, ErrInvalidRequest
		}
		var alias struct {
			ImageURL string `json:"image_url"`
		}
		if bytes.TrimSpace(raw)[0] == '{' && json.Unmarshal(raw, &alias) == nil && alias.ImageURL != "" {
			if len(image.Image) > 0 || image.URL != "" {
				return nil, ErrInvalidRequest
			}
			image.URL = alias.ImageURL
		}
		if image.URL != "" && len(image.Image) > 0 || image.URL == "" && len(image.Image) == 0 {
			return nil, ErrInvalidRequest
		}
		if strings.HasPrefix(image.URL, "data:image/") {
			image.Image, image.URL = assetBytes(image.URL), ""
			if len(image.Image) == 0 {
				return nil, ErrInvalidRequest
			}
		}
		if image.URL != "" && !validImageURL(image.URL) {
			return nil, ErrInvalidRequest
		}
		input.Images = append(input.Images, image)
	}
	delete(fields, "prompt")
	delete(fields, "images")
	delete(fields, "image")
	return json.Marshal(input)
}

func validImageURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil && u.Fragment == ""
}

func (b *Bifrost) loadEditImages(ctx context.Context, input *schemas.ImageEditInput) error {
	remaining := int64(16 << 20)
	for i := range input.Images {
		image := &input.Images[i]
		if image.URL != "" {
			raw, err := b.fetchMedia(ctx, image.URL, remaining)
			if err != nil {
				return ErrInvalidRequest
			}
			image.Image, image.URL = raw, ""
		}
		remaining -= int64(len(image.Image))
		if remaining < 0 {
			return ErrInvalidRequest
		}
	}
	return nil
}
