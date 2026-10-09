package app_test

import (
	"context"
	"errors"
	"github.com/tokpath/tokstudio/backend/internal/billing"
	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"github.com/tokpath/tokstudio/backend/internal/commission"
	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/platform/config"
	"os"
	"sync"
	"testing"
	"time"
)

func TestOverhaulPolicyInheritanceAndVersionConflict(t *testing.T) {
	if os.Getenv("TOKENHUB_DATABASE_URL") == "" || os.Getenv("TOKENHUB_REDIS_URL") == "" {
		t.Skip("requires isolated postgres and redis")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	a := mustApp(t, cfg)
	ctx := context.Background()
	inherited, err := a.Commission.ActivePolicy(ctx)
	if err != nil {
		t.Fatal(err)
	}
	scope := "overhaul-policy-" + time.Now().Format("150405.000000000")
	input := *inherited
	input.Version = ""
	input.ExpectedVersion = inherited.Version
	input.DirectBPS = 1000
	own, err := a.Commission.UpdateChannelPolicy(ctx, scope, input)
	if err != nil {
		t.Fatalf("first OEM policy based on inherited version: %v", err)
	}
	if own.Version == inherited.Version {
		t.Fatal("new policy needs a distinct version")
	}
	if _, err = a.Commission.UpdateChannelPolicy(ctx, scope, input); !errors.Is(err, commission.ErrConflict) {
		t.Fatalf("stale inherited update: %v", err)
	}
	input.ExpectedVersion = own.Version
	var wg sync.WaitGroup
	outcomes := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, e := a.Commission.UpdateChannelPolicy(ctx, scope, input); outcomes <- e }()
	}
	wg.Wait()
	close(outcomes)
	successes, conflicts := 0, 0
	for e := range outcomes {
		if e == nil {
			successes++
		} else if errors.Is(e, commission.ErrConflict) {
			conflicts++
		} else {
			t.Fatal(e)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("concurrent edits: success %d conflict %d", successes, conflicts)
	}
	current, err := a.Commission.ActivePolicy(ctx)
	if err != nil || current.Version != inherited.Version {
		t.Fatal("OEM edit changed platform policy")
	}
}

func TestOverhaulSupplierRetryAndChangedPayload(t *testing.T) {
	if os.Getenv("TOKENHUB_DATABASE_URL") == "" || os.Getenv("TOKENHUB_REDIS_URL") == "" {
		t.Skip("requires isolated postgres and redis")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	a := mustApp(t, cfg)
	ctx := context.Background()
	input := billing.SupplierInput{ChannelOrgID: identity.OfficialChannelID, AmountMinor: billing.MinorPerUSD, SourceType: "provider_invoice", VendorName: "Test vendor", Memo: "same ordinary note", IdempotencyKey: "overhaul-supplier-" + time.Now().Format("150405.000000000")}
	var wg sync.WaitGroup
	ids := make(chan string, 2)
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			row, e := a.Billing.RecordSupplier(ctx, "overhaul-test", input)
			if e != nil {
				errs <- e
			} else {
				ids <- row.ID
			}
		}()
	}
	wg.Wait()
	close(ids)
	close(errs)
	for e := range errs {
		t.Fatal(e)
	}
	original := ""
	count := 0
	for id := range ids {
		if original != "" && id != original {
			t.Fatal("concurrent retry created duplicate records")
		}
		original = id
		count++
	}
	if count != 2 {
		t.Fatal("retry did not return both outcomes")
	}
	changed := input
	changed.AmountMinor++
	if _, e := a.Billing.RecordSupplier(ctx, "overhaul-test", changed); !errors.Is(e, billing.ErrConflict) {
		t.Fatalf("changed amount reused operation: %v", e)
	}
	if _, e := a.Billing.RecordSupplier(ctx, "different-actor", input); !errors.Is(e, billing.ErrConflict) {
		t.Fatalf("cross-actor operation reused: %v", e)
	}
	found, err := a.Billing.SupplierOperation(ctx, "overhaul-test", identity.OfficialChannelID, input.IdempotencyKey)
	if err != nil || found.ID != original {
		t.Fatalf("scoped recovery: %v", err)
	}
	if _, err := a.Billing.SupplierOperation(ctx, "different-actor", identity.OfficialChannelID, input.IdempotencyKey); !errors.Is(err, billing.ErrNotFound) {
		t.Fatalf("operation leaked across actors: %v", err)
	}
	second := input
	second.IdempotencyKey += "-another"
	row, e := a.Billing.RecordSupplier(ctx, "overhaul-test", second)
	if e != nil || row.ID == original {
		t.Fatalf("same note for another actual transaction: %v", e)
	}
}

func TestOverhaulModelReadinessNeedsObservedHealth(t *testing.T) {
	if os.Getenv("TOKENHUB_DATABASE_URL") == "" || os.Getenv("TOKENHUB_REDIS_URL") == "" {
		t.Skip("isolated postgres and redis required")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	a := mustApp(t, cfg)
	ctx := context.Background()
	provider, err := a.Catalog.CreateProvider(ctx, catalog.ProviderInput{Name: "Readiness test", Adapter: "test"})
	if err != nil {
		t.Fatal(err)
	}
	model, _, err := a.Catalog.CreateModel(ctx, catalog.ModelInput{Vendor: "readiness", DisplayName: "Model " + time.Now().Format("150405.000000000"), Capabilities: map[string]any{"kind": "text"}, InitialPrice: map[string]any{"input": "0.000001", "output": "0.000002", "currency": "USD"}}, cfg.BootstrapAdmin)
	if err != nil {
		t.Fatal(err)
	}
	initial, err := a.Catalog.ModelReadiness(ctx, model.ID)
	if err != nil {
		t.Fatal(err)
	}
	if initial.Callable || len(initial.RouteIDs) != 0 || initial.RuntimeState != "not_configured" {
		t.Fatalf("draft should be incomplete: %+v", initial)
	}
	if _, err = a.Catalog.PublishModel(ctx, model.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = a.Catalog.SaveProviderModel(ctx, provider.ID, catalog.ProviderModelInput{UpstreamModelID: "readiness/upstream", UnitCosts: map[string]string{"input": "0.0000004", "output": "0.0000008"}}); err != nil {
		t.Fatal(err)
	}
	route, err := a.Catalog.CreateRoute(ctx, catalog.RouteInput{PublicModelID: model.ID, Strategy: "priority", Status: "active", Candidates: []catalog.RouteCandidateIn{{ProviderID: provider.ID, UpstreamModelID: "readiness/upstream", Priority: 1, Weight: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	ready, err := a.Catalog.ModelReadiness(ctx, model.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !ready.Callable || ready.RuntimeState != "unknown" || len(ready.RouteIDs) != 1 || ready.RouteIDs[0] != route.ID {
		t.Fatalf("defaults must not imply observed health: %+v", ready)
	}
	if _, err = a.Catalog.Probe(ctx, provider.ID); err != nil {
		t.Fatal(err)
	}
	ready, err = a.Catalog.ModelReadiness(ctx, model.ID)
	if err != nil || ready.RuntimeState != "healthy" {
		t.Fatalf("actual controlled probe: %+v %v", ready, err)
	}
	if _, err = a.Catalog.SetProviderModelEnabled(ctx, provider.ID, "readiness/upstream", false); err != nil {
		t.Fatal(err)
	}
	ready, err = a.Catalog.ModelReadiness(ctx, model.ID)
	if err != nil || ready.Callable {
		t.Fatalf("disabled cost model must block calls: %+v %v", ready, err)
	}
}
