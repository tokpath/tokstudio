package app_test

import (
	"context"
	"errors"
	"github.com/tokpath/tokstudio/backend/internal/billing"
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
	second := input
	second.IdempotencyKey += "-another"
	row, e := a.Billing.RecordSupplier(ctx, "overhaul-test", second)
	if e != nil || row.ID == original {
		t.Fatalf("same note for another actual transaction: %v", e)
	}
}
