package portal

import (
	"context"
	"github.com/google/uuid"
	"testing"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/modelruntime"
	"github.com/tokpath/tokstudio/backend/internal/organization"
)

func TestPostgresProductPricingNeedsNoManualPublicationPaperwork(t *testing.T) {
	f := setupPortal(t)
	f.login(t, "staff", "owner@example.test", "owner-password-test")
	ctx, operator := context.Background(), f.a.Installation.OperatorID
	base := "/v1/operators/" + operator
	model := "new-unlisted-model"
	status, out, _ := f.call(t, "POST", base+"/catalog", map[string]any{"configuration": map[string]any{"model": map[string]any{"id": model, "capabilities": []map[string]string{{"operation": "chat"}}}, "expected_version": 0}}, "")
	mustOK(t, status, out)
	status, out, _ = f.call(t, "GET", base+"/pricing", nil, "")
	mustOK(t, status, out)
	items := out["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["status"] != "disabled" {
		t.Fatal("unlisted model cannot be prepared", out)
	}
	status, out, _ = f.call(t, "GET", base+"/pricing?q=not-a-matching-model", nil, "")
	mustOK(t, status, out)
	if len(out["items"].([]any)) != 0 || out["search_supported"] != true {
		t.Fatal("model search is unavailable or ignored", out)
	}
	for _, name := range []string{model, "another-model"} {
		if name != model {
			if e := f.a.Catalog.SavePublicModel(ctx, operator, catalog.PublicModel{ID: name, Capabilities: []modelruntime.Capability{{Operation: modelruntime.Chat}}}, 0); e != nil {
				t.Fatal(e)
			}
		}
		key := uuid.NewString()
		price := map[string]any{"action": "price", "price": map[string]any{"model_id": name, "scope": map[string]string{"kind": "retail"}, "layer": "regular", "rule": map[string]any{"mode": "manual", "components": []map[string]string{{"key": "input", "unit": "token", "per": "1000000", "rate": "1"}, {"key": "cache", "unit": "token", "per": "1000000", "rate": "0.1"}, {"key": "output", "unit": "token", "per": "1000000", "rate": "2"}}}}}
		status, out, _ = f.call(t, "POST", base+"/pricing", map[string]any{"configuration": price}, key)
		mustOK(t, status, out)
		if out["status"] != "published" {
			t.Fatal("each model's first price must publish directly", out)
		}
		status, replay, _ := f.call(t, "POST", base+"/pricing", map[string]any{"configuration": price}, key)
		mustOK(t, status, replay)
		if replay["id"] != out["id"] {
			t.Fatal("publication replay generated another version")
		}
	}
	// Revisions have no caller-created notice ID, evidence, publication flag or
	// timestamp. They still cannot bypass real notification and price snapshots.
	change := map[string]any{"configuration": map[string]any{"action": "price", "price": map[string]any{"model_id": model, "scope": map[string]string{"kind": "retail"}, "layer": "regular", "rule": map[string]any{"mode": "manual", "components": []map[string]string{{"key": "input", "unit": "token", "per": "1000000", "rate": "3"}}}}}}
	status, out, _ = f.call(t, "POST", base+"/pricing", change, "")
	mustOK(t, status, out)
	if out["status"] != "announcing" {
		t.Fatal("revision bypassed notice", out)
	}
	var pending catalog.CatalogChange
	if err := f.a.DB.First(&pending, "id=?", out["id"]).Error; err != nil || pending.Evidence != "system:catalog-publication" || !pending.EffectiveAt.After(time.Now()) {
		t.Fatal("automatic publication missing", pending, err)
	}

	// Package publishing accepts the business inputs without IDs, contract
	// references or a manually chosen timezone. Editing preserves its timezone.
	packageInput := map[string]any{"name": "Simple package", "price_usd": "80", "paid_atg": "80", "gift_atg": "20", "period": "month", "new_purchase_enabled": true, "renewal_enabled": true, "expected_version": 0}
	status, out, _ = f.call(t, "POST", base+"/plans", packageInput, "")
	mustOK(t, status, out)
	pkg, err := f.a.Commerce.GetPackage(ctx, nil, operator, out["id"].(string))
	if err != nil {
		t.Fatal(err)
	}
	quote, err := f.a.Commerce.GetQuote(ctx, nil, *pkg.CurrentQuoteID)
	if err != nil || quote.Timezone != "Asia/Shanghai" || quote.PriceUSD.Cmp(ledger.MustDecimal("80")) != 0 {
		t.Fatal(quote, err)
	}
	packageInput["package_id"], packageInput["expected_version"], packageInput["new_purchase_enabled"] = pkg.ID, 1, false
	status, out, _ = f.call(t, "POST", base+"/plans", packageInput, "")
	mustOK(t, status, out)
	pkg, err = f.a.Commerce.GetPackage(ctx, nil, operator, pkg.ID)
	if err != nil || pkg.NewPurchaseEnabled || !pkg.RenewalEnabled {
		t.Fatal("new sales and renewal are coupled", pkg, err)
	}
	quote, err = f.a.Commerce.GetQuote(ctx, nil, *pkg.CurrentQuoteID)
	if err != nil || quote.Timezone != "Asia/Shanghai" || quote.Version != 2 {
		t.Fatal("editing lost package identity or timezone", quote, err)
	}
	active := f.configureExternalCatalog(t, f.server.URL)
	f.openRegistration(t)
	business, err := f.a.Organization.GetBusinessStatus(ctx, nil, operator)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.a.Organization.SetBusinessStatus(ctx, nil, organization.SetBusinessStatusInput{ActorStaffID: f.a.Installation.OwnerStaffID, OperatorID: operator, Feature: organization.ModelUsage, Enabled: true, ExpectedVersion: business.Version, EvidenceReference: "synthetic live service"}, f.a); err != nil {
		t.Fatal(err)
	}
	current, err := f.a.Catalog.PublicModel(ctx, active)
	if err != nil {
		t.Fatal(err)
	}
	profile := map[string]any{"input": []map[string]string{{"modality": "text", "template": "characters"}}, "output": []map[string]string{{"modality": "text", "template": "tokens"}}}
	request := map[string]any{"configuration": map[string]any{"action": "billing", "model_id": active, "operation": "chat", "billing": profile, "expected_version": current.Version}}
	status, out, _ = f.call(t, "POST", base+"/pricing", request, "")
	mustOK(t, status, out)
	if out["status"] != "announcing" {
		t.Fatal("open-model template still needs manual notice inputs", out)
	}
	firstChange := out["id"]
	status, out, _ = f.call(t, "GET", base+"/references/models?q="+active, nil, "")
	mustOK(t, status, out)
	defaults := out["items"].([]any)[0].(map[string]any)["defaults"].(map[string]any)
	meters := defaults["pricing_components"].([]any)
	if meters[0].(map[string]any)["key"] != "input_text_characters" {
		t.Fatal("cannot prepare prices until template activates", defaults)
	}
	profile["input"] = []map[string]string{{"modality": "text", "template": "characters"}, {"modality": "image", "template": "megapixels"}}
	status, out, _ = f.call(t, "POST", base+"/pricing", request, "")
	mustOK(t, status, out)
	pending = catalog.CatalogChange{}
	if err := f.a.DB.First(&pending, "id=?", firstChange).Error; err != nil || pending.State != "superseded" {
		t.Fatal("re-edit left stale changes competing", pending, err)
	}
	clock := time.Now().UTC().Add(4 * time.Minute)
	f.a.CatalogChanges.Now = func() time.Time { return clock }
	if _, err = f.a.CatalogChanges.Run(ctx, 20); err != nil {
		t.Fatal(err)
	}
	clock = clock.Add(2 * time.Minute)
	if _, err = f.a.CatalogChanges.Run(ctx, 20); err != nil {
		t.Fatal(err)
	}
	still, err := f.a.Catalog.PublicModel(ctx, active)
	if err != nil || still.Version != current.Version {
		t.Fatal("live service switched before replacement prices were ready", still, err)
	}
	price := map[string]any{"configuration": map[string]any{"action": "price", "price": map[string]any{"model_id": active, "scope": map[string]string{"kind": "retail"}, "layer": "regular", "rule": map[string]any{"mode": "manual", "components": []map[string]string{
		{"key": "input_text_characters", "unit": "character", "per": "1000", "rate": "1"},
		{"key": "input_image_megapixels", "unit": "megapixel", "per": "1", "rate": "2"},
		{"key": "input_image_megapixels:1024x1024", "unit": "megapixel", "per": "1", "rate": "3"},
		{"key": "output_text_tokens", "unit": "token", "per": "1000000", "rate": "4"},
	}}}}}
	status, out, _ = f.call(t, "POST", base+"/pricing", price, "")
	mustOK(t, status, out)
	if _, err = f.a.CatalogChanges.Run(ctx, 20); err != nil {
		t.Fatal(err)
	}
	clock = clock.Add(3 * time.Minute)
	if _, err = f.a.CatalogChanges.Run(ctx, 20); err != nil {
		t.Fatal(err)
	}
	ready, err := f.a.Catalog.PublicModel(ctx, active)
	if err != nil || ready.Version != current.Version+1 || ready.Billing[modelruntime.Chat].Input[0].Template != "characters" {
		t.Fatal("later prices did not unblock automatic template publication", ready, err)
	}
}
