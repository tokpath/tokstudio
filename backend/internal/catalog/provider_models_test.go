package catalog

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/tokpath/tokstudio/backend/internal/platform/config"
	"github.com/tokpath/tokstudio/backend/internal/platform/db"
)

func TestProviderModelStatusControlsPricing(t *testing.T) {
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
	ctx := context.Background()
	svc := New(gdb)
	provider, err := svc.CreateProvider(ctx, ProviderInput{Name: "Status test provider", Adapter: "test"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = gdb.Where("provider_id = ?", provider.ID).Delete(&providerModelRow{}).Error
		_ = gdb.Where("id = ?", provider.ID).Delete(&providerRow{}).Error
	}()
	if provider.Slug != provider.ID {
		t.Fatalf("provider identifier should be generated, got id=%q slug=%q", provider.ID, provider.Slug)
	}
	const upstreamID = "test/upstream-model"
	input := ProviderModelInput{UpstreamModelID: upstreamID, UnitCosts: map[string]string{"input": "0.0000004", "output": "0.0000008"}}
	model, err := svc.SaveProviderModel(ctx, provider.ID, input)
	if err != nil {
		t.Fatal(err)
	}
	if model.DisplayName != upstreamID || model.Status != "active" {
		t.Fatalf("unexpected new model: %+v", model)
	}
	if _, err := svc.PricedProviderModel(ctx, provider.ID, upstreamID, "text"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetProviderModelEnabled(ctx, provider.ID, upstreamID, false); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.PricedProviderModel(ctx, provider.ID, upstreamID, "text"); !errors.Is(err, ErrProviderModelUnpriced) {
		t.Fatalf("disabled model should not route, got %v", err)
	}
	model, err = svc.SaveProviderModel(ctx, provider.ID, input)
	if err != nil {
		t.Fatal(err)
	}
	if model.Status != "inactive" {
		t.Fatalf("editing price re-enabled model: %+v", model)
	}
	if _, err := svc.SetProviderModelEnabled(ctx, provider.ID, upstreamID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.PricedProviderModel(ctx, provider.ID, upstreamID, "text"); err != nil {
		t.Fatal(err)
	}
}
