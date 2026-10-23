package catalog

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/modelruntime"
)

func TestPostgresPendingBillingAcceptsResolutionPricesWithoutBreakingLiveMeters(t *testing.T) {
	s := newCommerceTestStore(t)
	seedCommerceModel(t, s)
	c, now, _ := testController(t, s, "platform")
	ctx := context.Background()
	scope := PriceScope{Kind: "retail", SellerID: "platform"}
	base := priceAt("base", scope, now.Add(-time.Hour), "1")
	if err := s.PublishPrice(ctx, "platform", base, c.ValidateCoverage); err != nil {
		t.Fatal(err)
	}
	model, err := s.PublicModel(ctx, "model")
	if err != nil {
		t.Fatal(err)
	}
	version := model.Version
	model.Billing[modelruntime.Chat] = modelruntime.BillingProfile{Input: []modelruntime.BillingItem{{Modality: "text", Template: "tokens"}, {Modality: "image", Template: "megapixels"}}, Output: []modelruntime.BillingItem{{Modality: "text", Template: "tokens"}}}
	modelChange, err := c.SchedulePublicModel(ctx, "platform", model, version, now.Add(10*time.Minute), "")
	if err != nil {
		t.Fatal(err)
	}
	promo := priceAt(uuid.NewString(), scope, now.Add(2*time.Minute), "0.5")
	promo.Layer = "promotion"
	end := now.Add(5 * time.Minute)
	promo.EndsAt = &end
	if _, err = c.SchedulePrice(ctx, "platform", promo, ""); err != nil {
		t.Fatal(err)
	}
	price := PriceVersion{ID: uuid.NewString(), Scope: scope, ModelID: "model", Layer: "regular", EffectiveAt: now.Add(3 * time.Minute), Published: true, Rule: PriceRule{Mode: "manual", Components: []ComponentRate{
		{Key: "input_text_tokens", Unit: "token", Per: "1000000", Rate: "2"},
		{Key: "cache_text", Unit: "token", Per: "1000000", Rate: "0.2"},
		{Key: "input_image_megapixels", Unit: "megapixel", Per: "1", Rate: "3"},
		{Key: "input_image_megapixels:1024x1024", Unit: "megapixel", Per: "1", Rate: "4"},
		{Key: "output_text_tokens", Unit: "token", Per: "1000000", Rate: "5"},
	}}}
	if _, err = c.SchedulePrice(ctx, "platform", price, ""); err != nil {
		t.Fatal(err)
	}
	if _, err = c.Run(ctx, 20); err != nil {
		t.Fatal(err)
	}
	live, err := s.PublicModel(ctx, "model")
	if err != nil || live.Version != version || live.Billing[modelruntime.Chat].Input[0].Modality != "all" {
		t.Fatal("price preparation changed the active model", live, err)
	}
	*now = price.EffectiveAt.Add(3 * time.Minute)
	resolved, err := (PriceResolver{Source: s}).Resolve(ctx, scope, "model", *now)
	if err != nil {
		t.Fatal(err)
	}
	rates := map[string]string{}
	for _, component := range resolved.Components {
		rates[component.Key] = component.Rate
	}
	if rates["input"] != "1" || rates["input_image_megapixels:1024x1024"] != "4" {
		t.Fatal("new price lost live rates or pending size prices", rates)
	}
	*now = modelChange.EffectiveAt.Add(time.Second)
	if _, err = c.Run(ctx, 20); err != nil {
		t.Fatal(err)
	}
	live, err = s.PublicModel(ctx, "model")
	if err != nil || live.Version != version+1 {
		t.Fatal("template publication was invalidated by size preparation", live, err)
	}
	found := false
	for _, component := range live.BillingComponents() {
		found = found || component.Key == "input_image_megapixels:1024x1024"
	}
	if !found {
		t.Fatal("published template lost its prepared resolution", live)
	}
}
