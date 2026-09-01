package catalog

import "testing"

func TestInferKind(t *testing.T) {
	cases := []struct {
		kind, id, name, want string
	}{
		{"text", "z-ai/glm-5.3", "GLM", "text"},
		{"IMAGE", "x/y", "X", "image"},
		{"", "bytedance/seedance-2.5", "Seedance", "video"},
		{"", "alibaba/wan-2.7", "Wan", "video"},
		{"", "google/gemini-3-pro-image", "Gemini Image", "image"},
		{"", "openai/text-embedding-3", "Embedding", "embedding"},
		{"", "openai/whisper-1", "Whisper", "audio"},
		{"", "tokenhub/echo-1", "Echo", "text"},
	}
	for _, tc := range cases {
		got := InferKind(ModelView{Kind: tc.kind, ID: tc.id, DisplayName: tc.name})
		if got != tc.want {
			t.Fatalf("InferKind(%q, %q, %q)=%q want %q", tc.kind, tc.id, tc.name, got, tc.want)
		}
	}
}

func TestApplyPublicFiltersVendorAndKind(t *testing.T) {
	items := []ModelView{
		{ID: "z-ai/glm-5.3-flash", Vendor: "z-ai", DisplayName: "GLM 5.3 Flash", Kind: "text", Description: "fast glm"},
		{ID: "z-ai/glm-image", Vendor: "z-ai", DisplayName: "GLM Image", Kind: "image"},
		{ID: "openai/gpt-5.6-sol", Vendor: "openai", DisplayName: "GPT-5.6 Sol", Kind: "text"},
		{ID: "bytedance/seedance-2.5", Vendor: "bytedance", DisplayName: "Seedance 2.5"},
	}

	all := ApplyPublicFilters(items, ModelListQuery{})
	if all.Total != 4 {
		t.Fatalf("unfiltered total=%d", all.Total)
	}
	if len(all.Facets.Vendors) != 3 || len(all.Facets.Kinds) < 2 {
		t.Fatalf("facets: %+v", all.Facets)
	}

	zai := ApplyPublicFilters(items, ModelListQuery{Vendor: "z-ai"})
	if zai.Total != 2 {
		t.Fatalf("vendor z-ai total=%d items=%v", zai.Total, idsOf(zai.Items))
	}
	for _, item := range zai.Items {
		if item.Vendor != "z-ai" {
			t.Fatalf("leaked vendor %s", item.Vendor)
		}
	}
	if !facetHas(zai.Facets.Kinds, "text", 1) || !facetHas(zai.Facets.Kinds, "image", 1) {
		t.Fatalf("kind facets should be scoped to vendor: %+v", zai.Facets.Kinds)
	}

	text := ApplyPublicFilters(items, ModelListQuery{Kind: "text"})
	if text.Total != 2 {
		t.Fatalf("kind text total=%d items=%v", text.Total, idsOf(text.Items))
	}
	if !facetHas(text.Facets.Vendors, "z-ai", 1) || !facetHas(text.Facets.Vendors, "openai", 1) {
		t.Fatalf("vendor facets should be scoped to kind: %+v", text.Facets.Vendors)
	}
	if facetHas(text.Facets.Vendors, "bytedance", 1) {
		t.Fatalf("video vendor should not appear in text facets: %+v", text.Facets.Vendors)
	}

	combo := ApplyPublicFilters(items, ModelListQuery{Vendor: "z-ai", Kind: "text", Q: "glm"})
	if combo.Total != 1 || combo.Items[0].ID != "z-ai/glm-5.3-flash" {
		t.Fatalf("combo: %+v", combo.Items)
	}

	one := ApplyPublicFilters(items, ModelListQuery{ID: "openai/gpt-5.6-sol"})
	if one.Total != 1 || one.Items[0].ID != "openai/gpt-5.6-sol" {
		t.Fatalf("id filter: %+v", one.Items)
	}

	limited := ApplyPublicFilters(items, ModelListQuery{Kind: "text", Limit: 1})
	if limited.Total != 2 || len(limited.Items) != 1 {
		t.Fatalf("limit should slice items but keep total: total=%d n=%d", limited.Total, len(limited.Items))
	}
}

func TestParseLimit(t *testing.T) {
	if ParseLimit("") != 0 || ParseLimit("nope") != 0 || ParseLimit("0") != 0 {
		t.Fatal("empty/invalid should be 0")
	}
	if ParseLimit("12") != 12 {
		t.Fatal(ParseLimit("12"))
	}
	if ParseLimit("9999") != 500 {
		t.Fatal("cap at 500")
	}
}

func idsOf(items []ModelView) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.ID)
	}
	return out
}

func facetHas(facets []FacetCount, id string, count int) bool {
	for _, item := range facets {
		if item.ID == id && item.Count == count {
			return true
		}
	}
	return false
}
