package modelpricing

import (
	"context"
	"encoding/json"
	"github.com/maximhq/bifrost/core/schemas"
	"github.com/maximhq/bifrost/framework/configstore/tables"
	"github.com/tokpath/tokstudio/backend/internal/modelruntime"
	"math/big"
	"testing"
	"time"
)

func fp(v float64) *float64 { return &v }
func costAttempt(t *testing.T, rate float64) modelruntime.Attempt {
	t.Helper()
	raw, e := json.Marshal(Snapshot{Version: "accepted-v1", AcceptedAt: time.Now(), Rows: []tables.TableModelPricing{{Provider: "openai", Model: "gpt-test", Mode: "chat", InputCostPerToken: fp(rate), OutputCostPerToken: fp(rate)}}})
	if e != nil {
		t.Fatal(e)
	}
	return modelruntime.Attempt{CostSnapshot: raw}
}
func TestCostUsesPinnedBifrostSheetAndDoesNotTrustGenericCost(t *testing.T) {
	a := costAttempt(t, .001)
	response := &schemas.BifrostChatResponse{Usage: &schemas.BifrostLLMUsage{PromptTokens: 12, CompletionTokens: 5, TotalTokens: 17, Cost: &schemas.BifrostCost{TotalCost: 99}}, ExtraFields: schemas.BifrostResponseExtraFields{RequestType: schemas.ChatCompletionRequest, RoutingInfo: schemas.RoutingInfo{Provider: schemas.OpenAI, Model: "public-alias"}, RawResponse: map[string]any{"usage": map[string]any{"cost": 7}}}}
	got := (&Service{}).Observe(context.Background(), a, response)
	if got.ReferenceCost == nil || got.Source != "bifrost_calculated" || got.Version != "accepted-v1" || got.ReferenceCost.Currency != "USD" {
		t.Fatal(got)
	}
	value, ok := new(big.Rat).SetString(got.ReferenceCost.Amount)
	if !ok || value.Cmp(big.NewRat(17, 1000)) != 0 {
		t.Fatal(got.ReferenceCost.Amount)
	}
	if response.ExtraFields.RoutingInfo.Model != "public-alias" {
		t.Fatal("cost identity changed public response")
	}
	a.Provider = "openrouter"
	got = (&Service{}).Observe(context.Background(), a, response)
	if got.Source != "provider_reported" || got.ReportedCost == nil || got.ReportedCost.Amount != "7" || got.ReferenceCost == nil || got.ReportedCost.Currency != "USD" {
		t.Fatal(got)
	}
}
func TestMissingCostIsNotFreeAndEstimateUsesSDKRequestType(t *testing.T) {
	a := costAttempt(t, .001)
	response := &schemas.BifrostChatResponse{ExtraFields: schemas.BifrostResponseExtraFields{RequestType: schemas.ChatCompletionRequest}}
	if got := (&Service{}).Observe(context.Background(), a, response); got.ReferenceCost != nil || got.Source != "unavailable" {
		t.Fatal(got)
	}
	c := modelruntime.DefaultBilling(modelruntime.Chat).Capability(modelruntime.Chat, false)
	r, _ := modelruntime.DecodeProtocol("req", modelruntime.Chat, []byte(`{"model":"public","messages":[{"role":"user","content":"hi"}],"max_tokens":10}`))
	if estimate := (&Service{}).Estimate(context.Background(), a.CostSnapshot, r, c); estimate == nil {
		t.Fatal("known datasheet produced no route estimate")
	}
	if got := (&Service{}).Observe(context.Background(), modelruntime.Attempt{}, &schemas.BifrostChatResponse{Usage: &schemas.BifrostLLMUsage{PromptTokens: 12}}); got.ReferenceCost != nil {
		t.Fatal("missing sheet recorded free")
	}
}
func TestVideoSDKBackfillNeverBecomesActualCost(t *testing.T) {
	a := costAttempt(t, .001)
	var snapshot Snapshot
	_ = json.Unmarshal(a.CostSnapshot, &snapshot)
	snapshot.Rows = []tables.TableModelPricing{{Provider: "openai", Model: "sora-test", Mode: "video_generation", OutputCostPerSecond: fp(.1)}}
	a.CostSnapshot, _ = json.Marshal(snapshot)
	seconds := "8"
	v := &schemas.BifrostVideoGenerationResponse{Status: schemas.VideoStatusCompleted, Seconds: &seconds, ExtraFields: schemas.BifrostResponseExtraFields{RequestType: schemas.VideoRetrieveRequest}}
	if got := (&Service{}).Observe(context.Background(), a, v); got.ReferenceCost != nil {
		t.Fatal("SDK request backfill billed as actual")
	}
	v.ExtraFields.RawResponse = map[string]any{"status": "completed", "seconds": "4"}
	got := (&Service{}).Observe(context.Background(), a, v)
	if got.ReferenceCost == nil || got.ReferenceCost.Amount != "0.4" {
		t.Fatal(got)
	}
	if *v.Seconds != "8" {
		t.Fatal("mutated runtime envelope")
	}
}

func TestCompletedImageStreamHasOneOutputForUpstreamCost(t *testing.T) {
	a := costAttempt(t, .001)
	var snapshot Snapshot
	_ = json.Unmarshal(a.CostSnapshot, &snapshot)
	snapshot.Rows = []tables.TableModelPricing{{Provider: "openai", Model: "image-test", Mode: "image_generation", OutputCostPerImage: fp(.04)}}
	a.CostSnapshot, _ = json.Marshal(snapshot)
	chunk := &schemas.BifrostStreamChunk{BifrostImageGenerationStreamResponse: &schemas.BifrostImageGenerationStreamResponse{Type: schemas.ImageGenerationEventTypeCompleted, URL: "https://example.test/image.png", ExtraFields: schemas.BifrostResponseExtraFields{RequestType: schemas.ImageGenerationStreamRequest}}}
	got := (&Service{}).Observe(context.Background(), a, chunk)
	if got.ReferenceCost == nil || got.ReferenceCost.Amount != "0.04" {
		t.Fatal(got)
	}
	chunk.BifrostImageGenerationStreamResponse.Type = schemas.ImageEventType("image_generation.partial_image")
	if got = (&Service{}).Observe(context.Background(), a, chunk); got.ReferenceCost != nil {
		t.Fatal("partial image was charged", got)
	}
}
