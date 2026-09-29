package catalog

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/tokpath/tokstudio/backend/internal/platform/config"
	"github.com/tokpath/tokstudio/backend/internal/platform/db"
)

func TestLoadOfoxSnapshots(t *testing.T) {
	items, err := loadOfoxSnapshots()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) < 100 {
		t.Fatalf("expected 100+ ofox models, got %d", len(items))
	}
	seen := map[string]bool{}
	kinds := map[string]int{}
	for _, item := range items {
		if item.ID == "" || item.DisplayName == "" {
			t.Fatalf("incomplete snapshot: %+v", item)
		}
		if seen[item.ID] {
			t.Fatalf("duplicate id %s", item.ID)
		}
		seen[item.ID] = true
		kinds[item.Kind]++
		if item.SellPrice != nil {
			raw, _ := json.Marshal(item.SellPrice)
			if json.Valid(raw) == false {
				t.Fatalf("bad sell_price for %s", item.ID)
			}
		}
	}
	if kinds["text"] == 0 || kinds["video"] == 0 || kinds["image"] == 0 {
		t.Fatalf("missing modalities: %+v", kinds)
	}
}

func TestPublicModelStatus(t *testing.T) {
	if PublicModelStatus("published") != "available" {
		t.Fatal(PublicModelStatus("published"))
	}
	if PublicModelStatus("deprecated") != "unavailable" {
		t.Fatal(PublicModelStatus("deprecated"))
	}
}

func TestPublicSiteContent(t *testing.T) {
	site := PublicSiteContent()
	if site["source"] != "https://ofox.ai" {
		t.Fatalf("source: %+v", site["source"])
	}
	if site["leaderboards"] == nil {
		t.Fatal("leaderboards missing")
	}
}

func TestOfoxSkipReasonAndPresetStatus(t *testing.T) {
	if ofoxSkipReason("") != "empty_id" {
		t.Fatal(ofoxSkipReason(""))
	}
	if ofoxSkipReason("tokenhub/echo-1") != "tokenhub_seed" {
		t.Fatal(ofoxSkipReason("tokenhub/echo-1"))
	}
	if ofoxSkipReason("z-ai/glm-5.3-flash") != "" {
		t.Fatal("ofox public ids should be imported")
	}
	if ofoxPresetStatus("available") != "published" {
		t.Fatal(ofoxPresetStatus("available"))
	}
	if ofoxPresetStatus("deprecated") != "deprecated" {
		t.Fatal(ofoxPresetStatus("deprecated"))
	}
	if ofoxPresetStatus("draft") != "published" {
		t.Fatal("snapshot rows are preset as reviewed+published, not draft")
	}
}

func TestImportOfoxSnapshotPublishesReviewedCatalog(t *testing.T) {
	if os.Getenv("TOKENHUB_DATABASE_URL") == "" {
		t.Skip("integration test requires TOKENHUB_DATABASE_URL")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	gdb, err := db.Open(cfg.DatabaseURL)
	if err != nil {
		t.Fatal(err)
	}
	module, fs := Migrations()
	if err := db.Apply(gdb, []db.ModuleMigrations{{Module: module, FS: fs}}); err != nil {
		t.Fatal(err)
	}
	result, err := New(gdb).ImportOfoxSnapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Total < 100 || result.Imported < 100 {
		t.Fatalf("expected 100+ imported ofox models, got %+v", result)
	}
	if result.Imported+result.Skipped != result.Total {
		t.Fatalf("imported+skipped must equal total: %+v", result)
	}

	const publicID = "z-ai/glm-5.3-flash"
	var model publicModelRow
	if err := gdb.Where("public_id = ?", publicID).First(&model).Error; err != nil {
		t.Fatal(err)
	}
	if model.Status != "published" || model.SyncState != SyncPublished {
		t.Fatalf("preset must be reviewed+published, got status=%s sync_state=%s", model.Status, model.SyncState)
	}
	if model.CreatedByUserID != "" || model.ReviewedByUserID != "" {
		t.Fatalf("system preset must leave actor ids empty: created=%q reviewed=%q", model.CreatedByUserID, model.ReviewedByUserID)
	}

	var price priceRow
	if err := gdb.Where("public_model_id = ?", model.ID).First(&price).Error; err != nil {
		t.Fatal(err)
	}
	if price.Status != "published" {
		t.Fatalf("preset price must be published, got %s", price.Status)
	}

	again, err := New(gdb).ImportOfoxSnapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if again.Imported != result.Imported {
		t.Fatalf("re-import should stay idempotent: first=%+v second=%+v", result, again)
	}
}

func TestOfoxImportDoesNotGrantOrOverwrite(t *testing.T) {
	if os.Getenv("TOKENHUB_DATABASE_URL") == "" {
		t.Skip("integration test requires TOKENHUB_DATABASE_URL")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	gdb, err := db.Open(cfg.DatabaseURL)
	if err != nil {
		t.Fatal(err)
	}
	module, fs := Migrations()
	if err := db.Apply(gdb, []db.ModuleMigrations{{Module: module, FS: fs}}); err != nil {
		t.Fatal(err)
	}
	tx := gdb.Begin()
	defer tx.Rollback()
	item := OfoxModelSnapshot{ID: "test-import/no-auto-grant", DisplayName: "Imported", Kind: "text", SellPrice: map[string]any{"input": "1"}}
	if _, _, err := upsertOfoxModel(tx, item); err != nil {
		t.Fatal(err)
	}
	var model publicModelRow
	if err := tx.Where("public_id = ?", item.ID).First(&model).Error; err != nil {
		t.Fatal(err)
	}
	var grants int64
	if err := tx.Model(&channelPolicyRow{}).Where("public_model_id = ?", model.ID).Count(&grants).Error; err != nil {
		t.Fatal(err)
	}
	if grants != 0 {
		t.Fatalf("import created %d channel grants", grants)
	}
	if err := tx.Model(&model).Update("display_name", "Edited").Error; err != nil {
		t.Fatal(err)
	}
	if _, _, err := upsertOfoxModel(tx, item); err != nil {
		t.Fatal(err)
	}
	if err := tx.Where("public_id = ?", item.ID).First(&model).Error; err != nil {
		t.Fatal(err)
	}
	if model.DisplayName != "Edited" {
		t.Fatalf("import overwrote edited model: %s", model.DisplayName)
	}
}

func TestOfoxSeedFilesExist(t *testing.T) {
	for _, p := range []string{"seed/ofox-models.json", "seed/ofox-site.json"} {
		if _, err := ofoxSeedFS.ReadFile(p); err != nil {
			t.Fatal(p, err)
		}
	}
	if _, err := os.Stat("seed/ofox-models.json"); err != nil {
		t.Fatal(err)
	}
}
