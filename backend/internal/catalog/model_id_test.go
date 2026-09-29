package catalog

import "testing"

func TestGeneratedModelPublicID(t *testing.T) {
	for _, tc := range []struct {
		vendor string
		name   string
		want   string
	}{
		{"Alibaba", "HappyHorse 1.0", "alibaba/happyhorse-1.0"},
		{"阿里", "通义千问 3.5", "阿里/通义千问-3.5"},
		{"Open AI", "GPT/5.6", "open-ai/gpt-5.6"},
	} {
		if got := generatedModelPublicID(tc.vendor, tc.name); got != tc.want {
			t.Errorf("generatedModelPublicID(%q, %q) = %q, want %q", tc.vendor, tc.name, got, tc.want)
		}
	}
}
