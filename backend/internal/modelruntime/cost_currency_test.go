package modelruntime

import (
	"context"
	"github.com/maximhq/bifrost/core/schemas"
	"testing"
)

type nativeCostMeter struct{ observations map[int]*CostObservation }

func (m nativeCostMeter) Observe(_ context.Context, _ Attempt, response any) *CostObservation {
	return m.observations[response.(*schemas.BifrostStreamChunk).BifrostImageGenerationStreamResponse.Index]
}

func TestImageCostAggregationKeepsCurrenciesAndCostBasesSeparate(t *testing.T) {
	meter := nativeCostMeter{map[int]*CostObservation{
		0: {Source: "provider_reported", ReferenceCost: &Money{Currency: "USD", Amount: "0.04"}, ReportedCost: &Money{Currency: "CNY", Amount: "0.3"}},
		1: {Source: "provider_reported", ReferenceCost: &Money{Currency: "USD", Amount: "0.04"}, ReportedCost: &Money{Currency: "CNY", Amount: "0.4"}},
	}}
	stream := &billingStream{}
	chunk := func(index int) *schemas.BifrostStreamChunk {
		return &schemas.BifrostStreamChunk{BifrostImageGenerationStreamResponse: &schemas.BifrostImageGenerationStreamResponse{Index: index, Type: schemas.ImageGenerationEventTypeCompleted}}
	}
	stream.cost(context.Background(), meter, Attempt{}, chunk(0))
	got := stream.cost(context.Background(), meter, Attempt{}, chunk(1))
	if got.ReferenceCost.Currency != "USD" || got.ReferenceCost.Amount != "0.080000000000000000000000" || got.ReportedCost.Currency != "CNY" || got.ReportedCost.Amount != "0.700000000000000000000000" {
		t.Fatal(got)
	}
	replay := stream.cost(context.Background(), meter, Attempt{}, chunk(1))
	if replay.ReferenceCost.Amount != got.ReferenceCost.Amount || replay.ReportedCost.Amount != got.ReportedCost.Amount {
		t.Fatal("replayed image doubled cost", replay)
	}
	meter.observations[1].ReportedCost.Currency = "EUR"
	got = stream.cost(context.Background(), meter, Attempt{}, chunk(1))
	if got.ReportedCost != nil || got.ReferenceCost == nil || got.ReferenceCost.Currency != "USD" {
		t.Fatal("mixed currencies were added", got)
	}
}
