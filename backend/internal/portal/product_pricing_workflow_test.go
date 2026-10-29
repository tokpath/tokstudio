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
	// A revision needs the operator's time. Notices go out before the response,
	// and a time only 30 seconds ahead is accepted.
	effective := time.Now().UTC().Add(30 * time.Second).Truncate(time.Microsecond)
	change := map[string]any{"configuration": map[string]any{"action": "price", "price": map[string]any{"model_id": model, "scope": map[string]string{"kind": "retail"}, "layer": "regular", "effective_at": effective, "rule": map[string]any{"mode": "manual", "components": []map[string]string{{"key": "input", "unit": "token", "per": "1000000", "rate": "3"}}}}}}
	status, out, _ = f.call(t, "POST", base+"/pricing", change, "")
	mustOK(t, status, out)
	if out["status"] != "applied" {
		t.Fatal("revision was not notified on submit", out)
	}
	var pending catalog.CatalogChange
	if err := f.a.DB.First(&pending, "id=?", out["id"]).Error; err != nil || pending.AnnouncedAt == nil || !pending.EffectiveAt.Equal(effective) {
		t.Fatal("operator effective time was replaced", pending, err)
	}
	omitted := map[string]any{"configuration": map[string]any{"action": "price", "price": map[string]any{"model_id": model, "scope": map[string]string{"kind": "retail"}, "layer": "regular", "rule": map[string]any{"mode": "manual", "components": []map[string]string{{"key": "input", "unit": "token", "per": "1000000", "rate": "4"}}}}}}
	status, out, _ = f.call(t, "POST", base+"/pricing", omitted, "")
	if status != 400 {
		t.Fatal("revision without an effective time was accepted", status, out)
	}

	// Package publishing accepts the business inputs without IDs, contract
	// references or a manually chosen timezone. Editing preserves its timezone.
	packageInput := map[string]any{"name": "Simple package", "price_money": "80", "paid_atg": "80", "gift_atg": "20", "period": "month", "new_purchase_enabled": true, "renewal_enabled": true, "expected_version": 0}
	status, out, _ = f.call(t, "POST", base+"/plans", packageInput, "")
	mustOK(t, status, out)
	pkg, err := f.a.Commerce.GetPackage(ctx, nil, operator, out["id"].(string))
	if err != nil {
		t.Fatal(err)
	}
	quote, err := f.a.Commerce.GetQuote(ctx, nil, *pkg.CurrentQuoteID)
	if err != nil || quote.Timezone != "Asia/Shanghai" || quote.PriceMoney.Cmp(ledger.MustDecimal("80")) != 0 {
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
	templateAt := time.Now().UTC().Add(8 * time.Minute).Truncate(time.Microsecond)
	request := map[string]any{"configuration": map[string]any{"action": "billing", "model_id": active, "operation": "chat", "billing": profile, "expected_version": current.Version, "effective_at": templateAt}}
	status, out, _ = f.call(t, "POST", base+"/pricing", request, "")
	mustOK(t, status, out)
	if out["status"] != "announced" {
		t.Fatal("open-model template was not announced on submit", out)
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
	price := map[string]any{"configuration": map[string]any{"action": "price", "price": map[string]any{"model_id": active, "scope": map[string]string{"kind": "retail"}, "layer": "regular", "effective_at": time.Now().UTC().Add(time.Minute), "rule": map[string]any{"mode": "manual", "components": []map[string]string{
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

func TestPostgresFirstListingImmediatelyUpdatesPricingAndCustomerModels(t *testing.T) {
	f := setupPortal(t)
	ctx, op := context.Background(), f.a.Installation.OperatorID
	first := f.configureExternalCatalog(t, f.server.URL)
	second := f.configureExternalCatalog(t, f.server.URL)
	// Prepare a verified, priced new model before its first retail listing.
	if err := f.a.DB.Exec("DELETE FROM tokenhub_model_listings WHERE operator_id=? AND public_model_id=?", op, second).Error; err != nil {
		t.Fatal(err)
	}
	f.login(t, "staff", "owner@example.test", "owner-password-test")
	base := "/v1/operators/" + op
	input := map[string]any{"configuration": map[string]any{"action": "listing", "model_id": second, "enabled": true, "expected_version": 0}}
	key := uuid.NewString()
	status, out, _ := f.call(t, "POST", base+"/pricing", input, key)
	mustOK(t, status, out)
	if out["status"] != "configured" || out["message"] != "本经营方已上架此模型。" {
		t.Fatal("new model unexpectedly delayed", out)
	}
	status, replay, _ := f.call(t, "POST", base+"/pricing", input, key)
	mustOK(t, status, replay)
	if replay["id"] != out["id"] {
		t.Fatal("listing replay changed result", replay)
	}
	var changes int64
	if err := f.a.DB.Table("tokenhub_catalog_changes").Where("operator_id=? AND kind='listing' AND resource_id=?", op, second).Count(&changes).Error; err != nil || changes != 0 {
		t.Fatal("first listing created a notification plan", changes, err)
	}
	var checks int64
	if err := f.a.DB.Table("portal_price_checks").Where("price_id=? AND operator_id=?", "listing:"+op+":"+second+":1", op).Count(&checks).Error; err != nil || checks != 1 {
		t.Fatal("first listing did not recalculate coverage exactly once", checks, err)
	}
	status, out, _ = f.call(t, "GET", base+"/pricing", nil, "")
	mustOK(t, status, out)
	found := map[string]bool{}
	for _, r := range out["items"].([]any) {
		row := r.(map[string]any)
		if row["id"] == first || row["id"] == second {
			found[row["id"].(string)] = row["status"] == "available"
		}
	}
	if !found[first] || !found[second] {
		t.Fatal("pricing workspace did not update", out)
	}
	status, out, _ = f.call(t, "GET", "/v1/public/models", nil, "")
	mustOK(t, status, out)
	found = map[string]bool{}
	for _, r := range out["items"].([]any) {
		row := r.(map[string]any)
		if row["id"] == first || row["id"] == second {
			found[row["id"].(string)] = true
		}
	}
	if !found[first] || !found[second] {
		t.Fatal("platform direct customer catalog did not update", out)
	}
	change, err := f.a.CatalogChanges.ScheduleListing(ctx, op, second, false, 1, time.Now().Add(3*time.Minute), "")
	if err != nil {
		t.Fatal(err)
	}
	status, out, _ = f.call(t, "GET", base+"/pricing", nil, "")
	mustOK(t, status, out)
	foundPending := false
	for _, r := range out["items"].([]any) {
		row := r.(map[string]any)
		if row["id"] != second {
			continue
		}
		for _, p := range row["values"].(map[string]any)["pending_changes"].([]any) {
			pending := p.(map[string]any)
			if pending["kind"] == "listing" && pending["enabled"] == false && pending["status"] == change.State && pending["effective_at"] != "" {
				foundPending = true
			}
		}
	}
	if !foundPending {
		t.Fatal("pending unlisting direction or time missing", out)
	}
}
