package catalog

import (
	"errors"
	"testing"

	"github.com/tokpath/tokstudio/backend/internal/modelruntime"
)

func TestSupplierQuoteIgnoresClientMeteringAndPricesTextPerMillionTokens(t *testing.T) {
	model := SupplierModel{Capabilities: []modelruntime.Capability{{
		Operation: modelruntime.Chat,
		Quotes: []modelruntime.CapabilityQuote{
			{Key: "input", Rate: "3", Bound: "128000"},
			{Key: "output", Rate: "15", Bound: "4096"},
			{Key: "cache", Rate: "0.30", Bound: "128000"},
		},
		UsageRules:     []modelruntime.UsageRule{{Component: "custom", Unit: "token", Path: "/not-used"}},
		BillableBounds: map[string]string{"custom": "1"},
	}}, Prices: []ComponentRate{{Key: "custom", Unit: "token", Per: "1", Rate: "9"}}}
	if e := normalizeSupplierModel(&model); e != nil {
		t.Fatal(e)
	}
	if len(model.Prices) != 3 || model.Prices[0].Key != "input" || model.Prices[0].Per != "1000000" || model.Prices[0].Rate != "3" || model.Prices[2].Per != "1000000" {
		t.Fatalf("%#v", model.Prices)
	}
	capability := model.Capabilities[0]
	if len(capability.Quotes) != 0 || capability.UsageRules[0].Path != "/prompt_tokens" || capability.UsageRules[2].Path != "/prompt_tokens_details/cached_read_tokens" || !capability.UsageRules[2].ZeroWhenAbsent {
		t.Fatalf("%#v", capability)
	}
	if len(capability.SubtractPaths["input"]) != 1 || capability.SubtractPaths["input"][0] != "/prompt_tokens_details/cached_read_tokens" {
		t.Fatalf("%#v", capability.SubtractPaths)
	}
}

func TestSupplierQuoteUsesEachCapabilityQuantity(t *testing.T) {
	model := SupplierModel{Capabilities: []modelruntime.Capability{
		{Operation: modelruntime.Image, Quotes: []modelruntime.CapabilityQuote{{Key: "image", Rate: "0.04", Bound: "4"}}},
		{Operation: modelruntime.Speech, Quotes: []modelruntime.CapabilityQuote{{Key: "characters", Rate: "15", Bound: "1000000"}}},
		{Operation: modelruntime.Transcription, Quotes: []modelruntime.CapabilityQuote{{Key: "audio_seconds", Rate: "0.006", Bound: "3600"}}},
		{Operation: modelruntime.Video, Quotes: []modelruntime.CapabilityQuote{{Key: "video_seconds", Rate: "0.05", Bound: "60"}}},
	}}
	if e := normalizeSupplierModel(&model); e != nil {
		t.Fatal(e)
	}
	want := []ComponentRate{
		{Key: "image", Unit: "image", Per: "1", Rate: "0.04"},
		{Key: "characters", Unit: "character", Per: "1000000", Rate: "15"},
		{Key: "audio_seconds", Unit: "second", Per: "1", Rate: "0.006"},
		{Key: "video_seconds", Unit: "second", Per: "1", Rate: "0.05"},
	}
	if len(model.Prices) != len(want) {
		t.Fatalf("%#v", model.Prices)
	}
	for i, price := range want {
		got := model.Prices[i]
		if got.Key != price.Key || got.Unit != price.Unit || got.Per != price.Per || got.Rate != price.Rate || model.Capabilities[i].UsageRules[0].Path == "" {
			t.Fatalf("%d %#v", i, model.Capabilities[i])
		}
	}
	paths := []string{"/output_count", "/input_chars", "/seconds", "/provider/seconds"}
	for i, path := range paths {
		if model.Capabilities[i].UsageRules[0].Path != path {
			t.Fatalf("path %s %#v", path, model.Capabilities[i].UsageRules)
		}
	}
}

func TestSupplierQuoteRejectsATextCapabilityWithoutOutput(t *testing.T) {
	model := SupplierModel{Capabilities: []modelruntime.Capability{{Operation: modelruntime.Chat, Quotes: []modelruntime.CapabilityQuote{{Key: "input", Rate: "1", Bound: "1000"}}}}}
	if !errors.Is(normalizeSupplierModel(&model), ErrInvalidInput) {
		t.Fatal("chat accepted without an output price")
	}
}

func TestSupplierQuoteRejectsConflictingRatesAndUnknownOperations(t *testing.T) {
	conflict := SupplierModel{Capabilities: []modelruntime.Capability{
		{Operation: modelruntime.Chat, Streaming: true, Quotes: []modelruntime.CapabilityQuote{{Key: "input", Rate: "1", Bound: "10"}, {Key: "output", Rate: "2", Bound: "10"}}},
		{Operation: modelruntime.Chat, Quotes: []modelruntime.CapabilityQuote{{Key: "input", Rate: "9", Bound: "10"}, {Key: "output", Rate: "2", Bound: "10"}}},
	}}
	if !errors.Is(normalizeSupplierModel(&conflict), ErrInvalidInput) {
		t.Fatal("two input rates accepted")
	}
	unknown := SupplierModel{Capabilities: []modelruntime.Capability{{Operation: modelruntime.VideoRetrieve, Quotes: []modelruntime.CapabilityQuote{{Key: "video_seconds", Rate: "1", Bound: "1"}}}}}
	if !errors.Is(normalizeSupplierModel(&unknown), ErrInvalidInput) {
		t.Fatal("video retrieval accepted as a priced capability")
	}
}
