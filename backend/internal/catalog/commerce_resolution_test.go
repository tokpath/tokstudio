package catalog

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/modelruntime"
	"gorm.io/gorm"
)

func TestResolutionPricesAttachSizesAndReuseAnExistingVariant(t *testing.T) {
	model := PublicModel{Capabilities: []modelruntime.Capability{{Operation: modelruntime.Image}}, Billing: map[modelruntime.Operation]modelruntime.BillingProfile{
		modelruntime.Image: {Output: []modelruntime.BillingItem{{Modality: "image", Template: "images", Variants: []modelruntime.BillingVariant{{ID: "hd", Name: "高清", Size: "1024x1024"}}}}},
	}}
	components := []ComponentRate{
		{Key: "output_image_images", Unit: "image", Per: "1", Rate: "1"},
		{Key: "output_image_images:1024x1024", Unit: "image", Per: "1", Rate: "2"},
		{Key: "output_image_images:512x512", Unit: "image", Per: "1", Rate: "3"},
	}
	changed, e := applyResolutionPrices(&model, components)
	if e != nil || !changed {
		t.Fatal(e, changed)
	}
	if components[1].Key != "output_image_images:hd" || components[2].Key != "output_image_images:512x512" {
		t.Fatal(components)
	}
	variants := model.Billing[modelruntime.Image].Output[0].Variants
	if len(variants) != 2 || variants[0].ID != "hd" || variants[1].ID != "512x512" || variants[1].Size != "512x512" {
		t.Fatal(variants)
	}
	chat := PublicModel{Capabilities: []modelruntime.Capability{{Operation: modelruntime.Chat}}}
	if _, e = applyResolutionPrices(&chat, []ComponentRate{{Key: "input:1024x1024", Unit: "token", Per: "1000000", Rate: "1"}}); !errors.Is(e, ErrInvalidInput) {
		t.Fatal(e)
	}
}

func TestPublishingAResolutionPriceUpdatesAnOpenModel(t *testing.T) {
	store := newCommerceTestStore(t)
	ctx := context.Background()
	model := PublicModel{ID: "picture", Identity: "picture@1", Capabilities: []modelruntime.Capability{{Operation: modelruntime.Image}}}
	if e := store.SavePublicModel(ctx, "platform", model, 0); e != nil {
		t.Fatal(e)
	}
	if _, e := store.SaveCredential(ctx, "platform", "picture-key", "provider", "image key", "secret-to-encrypt", true, 0); e != nil {
		t.Fatal(e)
	}
	upstream := UpstreamModel{ID: "picture-upstream", PublicModelID: model.ID, Identity: model.Identity, ProviderID: "provider", Provider: "openai", Model: "image-model", CredentialID: "picture-key", Capabilities: model.Capabilities, Enabled: true, Healthy: true}
	if e := store.SaveUpstream(ctx, "platform", upstream, 0); e != nil {
		t.Fatal(e)
	}
	model.Enabled = true
	if e := store.SavePublicModel(ctx, "platform", model, 1); e != nil {
		t.Fatal(e)
	}
	accept := func(context.Context, *gorm.DB, PriceVersion) error { return nil }
	price := PriceVersion{ID: uuid.NewString(), Scope: PriceScope{Kind: "retail", SellerID: "platform"}, ModelID: model.ID, Layer: "regular", EffectiveAt: time.Now().UTC(), Published: true, Rule: PriceRule{Mode: "manual", Components: []ComponentRate{
		{Key: "output_image_images", Unit: "image", Per: "1", Rate: "1"},
		{Key: "output_image_images:1024x1024", Unit: "image", Per: "1", Rate: "3"},
	}}}
	if e := store.PublishPrice(ctx, "platform", price, accept); e != nil {
		t.Fatal(e)
	}
	saved, e := store.PublicModel(ctx, model.ID)
	if e != nil {
		t.Fatal(e)
	}
	variants := saved.Billing[modelruntime.Image].Output[0].Variants
	if len(variants) != 1 || variants[0].ID != "1024x1024" || variants[0].Size != "1024x1024" {
		t.Fatal(variants)
	}
	channel := PriceVersion{ID: uuid.NewString(), Scope: PriceScope{Kind: "retail", SellerID: "oem"}, ModelID: model.ID, Layer: "regular", EffectiveAt: time.Now().UTC(), Published: true, Rule: PriceRule{Mode: "manual", Components: []ComponentRate{
		{Key: "output_image_images", Unit: "image", Per: "1", Rate: "2"},
		{Key: "output_image_images:512x512", Unit: "image", Per: "1", Rate: "4"},
	}}}
	if e = store.PublishPrice(ctx, "oem", channel, accept); !errors.Is(e, ErrResolutionPrice) {
		t.Fatal(e)
	}
	saved, e = store.PublicModel(ctx, model.ID)
	if e != nil || len(saved.Billing[modelruntime.Image].Output[0].Variants) != 1 {
		t.Fatal(saved.Billing, e)
	}
}
