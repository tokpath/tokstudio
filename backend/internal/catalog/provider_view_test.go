package catalog

import "testing"

func TestFilterProvidersMatchesMappedModels(t *testing.T) {
	items := []ProviderView{
		{
			ID: "prd_openai", Name: "OpenAI", Slug: "openai", Kind: "direct", Adapter: "openai", Status: "active", Health: "available",
			Models: []MappedModelView{{PublicID: "openai/gpt-5.6", Vendor: "openai", DisplayName: "GPT-5.6"}},
		},
		{
			ID: "prd_or", Name: "OpenRouter Media", Slug: "openrouter-seedance", Kind: "aggregator", Adapter: "openrouter",
			Models: []MappedModelView{{PublicID: "bytedance/seedance-1.0", Vendor: "bytedance"}},
		},
	}
	openai := FilterProviders(items, "gpt-5.6")
	if len(openai) != 1 || openai[0].Slug != "openai" {
		t.Fatalf("filter by mapped public id: %+v", openai)
	}
	agg := FilterProviders(items, "聚合")
	if len(agg) != 0 {
		t.Fatalf("kind is stored as aggregator, not Chinese label: %+v", agg)
	}
	or := FilterProviders(items, "aggregator")
	if len(or) != 1 || or[0].Slug != "openrouter-seedance" {
		t.Fatalf("filter by kind: %+v", or)
	}
}

func TestAttachMappedModelsKeepsEmptySlice(t *testing.T) {
	items := []ProviderView{{ID: "prd_empty", Slug: "empty"}, {ID: "prd_openai", Slug: "openai"}}
	got := attachMappedModels(items, []mappedModelScan{
		{ProviderID: "prd_openai", PublicID: "openai/gpt-5.6", Vendor: "openai", DisplayName: "GPT", UpstreamModelID: "gpt-5.6", Status: "active"},
		{ProviderID: "prd_openai", PublicID: "openai/gpt-4.1", Vendor: "openai", DisplayName: "GPT 4.1", UpstreamModelID: "gpt-4.1", Status: "active"},
	})
	if got[0].Models == nil || len(got[0].Models) != 0 {
		t.Fatalf("unmapped provider should keep empty models: %+v", got[0])
	}
	if len(got[1].Models) != 2 || got[1].Models[0].PublicID != "openai/gpt-5.6" {
		t.Fatalf("openai should keep both mappings: %+v", got[1])
	}
}
