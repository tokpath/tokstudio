package catalog

import "testing"

func TestModelConfigurationReadyRequiresKindSpecificPrice(t *testing.T) {
	model := publicModelRow{PublicID: "test/model", Vendor: "test", DisplayName: "Model"}
	for _, tc := range []struct {
		name  string
		kind  string
		price map[string]any
		want  bool
	}{
		{"text", "text", map[string]any{"input": "0", "output": "0.01"}, true},
		{"text missing output", "text", map[string]any{"input": "0.01"}, false},
		{"video", "video", map[string]any{"video_second": "0.02"}, true},
		{"legacy media is not seconds", "video", map[string]any{"media": "0.13"}, false},
		{"image", "image", map[string]any{"image_count": "0.03"}, true},
		{"negative price", "image", map[string]any{"image_count": "-1"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := modelConfigurationReady(model, map[string]any{"kind": tc.kind}, tc.price)
			if got != tc.want {
				t.Fatalf("ready=%v, want %v", got, tc.want)
			}
		})
	}
}
