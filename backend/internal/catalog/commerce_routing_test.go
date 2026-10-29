package catalog

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/modelruntime"
)

func TestPostgresRoutingCapacityCircuitRecoveryAndLateObservation(t *testing.T) {
	c := newCommerceTestStore(t)
	seedCommerceModel(t, c)
	ctx := context.Background()
	rows, e := c.upstreams(ctx, "model")
	if e != nil {
		t.Fatal(e)
	}
	u := rows[0]
	var wg sync.WaitGroup
	results := make(chan string, 12)
	errs := make(chan error, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := fmt.Sprintf("attempt-%d", i)
			e := c.ClaimRoute(ctx, u.ID, id, "request", time.Now().Add(time.Minute))
			if e == nil {
				results <- id
			} else if !errors.Is(e, ErrRouteUnavailable) {
				errs <- e
			}
		}(i)
	}
	wg.Wait()
	close(results)
	close(errs)
	for e := range errs {
		t.Fatal(e)
	}
	ids := []string{}
	for id := range results {
		ids = append(ids, id)
	}
	if len(ids) != 12 {
		t.Fatalf("healthy route admitted %d", len(ids))
	}
	bad := modelruntime.Result{State: "completed", Usage: modelruntime.Usage{Reliable: false}}
	good := modelruntime.Result{State: "completed", Usage: modelruntime.Usage{Reliable: true}}
	if e = c.ObserveRoute(ctx, ids[0], bad); e != nil {
		t.Fatal(e)
	}
	if e = c.ObserveRoute(ctx, ids[0], bad); e != nil {
		t.Fatal(e)
	}
	view, e := c.AdminRouting(ctx, "platform", "model")
	if e != nil || view.Health[0].Circuit != "closed" || view.Health[0].UnknownUsage != 1 {
		t.Fatal("one unverifiable result or duplicate tripped circuit", view, e)
	}
	if e = c.ObserveRoute(ctx, ids[1], bad); e != nil {
		t.Fatal(e)
	}
	view, e = c.AdminRouting(ctx, "platform", "model")
	if e != nil {
		t.Fatal(e)
	}
	if view.Health[0].Circuit != "open" || view.Health[0].UnknownUsage != 2 {
		t.Fatalf("duplicate counted: %+v", view.Health)
	}
	if e = c.ClaimRoute(ctx, u.ID, "too-early", "request", time.Now().Add(time.Minute)); !errors.Is(e, ErrRouteUnavailable) {
		t.Fatalf("cooldown bypass: %v", e)
	}
	// An earlier in-flight success cannot clear a newer trip epoch.
	if e = c.ObserveRoute(ctx, ids[2], good); e != nil {
		t.Fatal(e)
	}
	view, _ = c.AdminRouting(ctx, "platform", "model")
	if view.Health[0].Circuit != "open" {
		t.Fatal("late success reset circuit")
	}
	for _, id := range ids[3:] {
		if e = c.ObserveRoute(ctx, id, modelruntime.Result{State: "not_dispatched"}); e != nil {
			t.Fatal(e)
		}
	}
	if e = c.DB.Model(&RouteHealth{}).Where("upstream_id=?", u.ID).Update("open_until", time.Now().Add(-time.Second)).Error; e != nil {
		t.Fatal(e)
	}
	if e = c.ClaimRoute(ctx, u.ID, "probe-1", "request", time.Now().Add(time.Minute)); e != nil {
		t.Fatal(e)
	}
	if e = c.ClaimRoute(ctx, u.ID, "parallel-probe", "request", time.Now().Add(time.Minute)); !errors.Is(e, ErrRouteUnavailable) {
		t.Fatalf("multiple recovery probes: %v", e)
	}
	if e = c.ObserveRoute(ctx, "probe-1", good); e != nil {
		t.Fatal(e)
	}
	view, _ = c.AdminRouting(ctx, "platform", "model")
	if view.Health[0].Circuit != "open" || view.Health[0].RecoverySuccesses != 1 {
		t.Fatal("premature recovery")
	}
	if e = c.ClaimRoute(ctx, u.ID, "probe-2", "request", time.Now().Add(time.Minute)); e != nil {
		t.Fatal(e)
	}
	if e = c.ObserveRoute(ctx, "probe-2", good); e != nil {
		t.Fatal(e)
	}
	view, _ = c.AdminRouting(ctx, "platform", "model")
	if view.Health[0].Circuit != "closed" || view.Health[0].OpenUntil != nil {
		t.Fatal("verified recovery failed")
	}
	if e = c.DB.Exec("DELETE FROM tokenhub_route_events").Error; e == nil {
		t.Fatal("health evidence could be deleted")
	}
}

func TestPostgresRoutingPoliciesChooseCostAndLatencyWithAuthority(t *testing.T) {
	c := newCommerceTestStore(t)
	seedCommerceModel(t, c)
	ctx := context.Background()
	p := RoutingPolicy{ModelID: "model", Strategy: "lowest_cost"}
	if e := c.SaveRoutingPolicy(ctx, "oem", p, 0); !errors.Is(e, ErrCatalogForbidden) {
		t.Fatal("OEM set platform strategy")
	}
	if e := c.SaveRoutingPolicy(ctx, "platform", p, 0); e != nil {
		t.Fatal(e)
	}
	expensive, cheap := "2", "1"
	q := ModelQuote{ModelID: "model", Candidates: []UpstreamModel{{ID: "expensive", Priority: 0}, {ID: "cheap", Priority: 1}}, Supplier: []SupplierPriceSnapshot{{UpstreamID: "expensive", EstimateMoney: &expensive}, {UpstreamID: "cheap", EstimateMoney: &cheap}}}
	if e := c.rankRoutes(ctx, &q); e != nil {
		t.Fatal(e)
	}
	if q.Candidates[0].ID != "cheap" {
		t.Fatal("cost policy ignored")
	}
	p.Strategy = "lowest_latency"
	if e := c.SaveRoutingPolicy(ctx, "platform", p, 1); e != nil {
		t.Fatal(e)
	}
	if e := c.SaveRoutingPolicy(ctx, "platform", p, 1); !errors.Is(e, ErrCatalogVersion) {
		t.Fatal("stale policy overwrite")
	}
	rows, _ := c.upstreams(ctx, "model")
	u := rows[0]
	u.ID = "second"
	u.Version = 0
	if e := c.SaveUpstream(ctx, "platform", u, 0); e != nil {
		t.Fatal(e)
	}
	for id, latency := range map[string]int64{"upstream": 300, "second": 100} {
		if e := c.DB.Create(&RouteHealth{UpstreamID: id, Circuit: "closed", LatencyMicros: latency}).Error; e != nil {
			t.Fatal(e)
		}
	}
	q.Candidates = []UpstreamModel{{ID: "upstream", Priority: 0}, {ID: "second", Priority: 1}}
	if e := c.rankRoutes(ctx, &q); e != nil {
		t.Fatal(e)
	}
	if q.Candidates[0].ID != "second" {
		t.Fatal("latency policy ignored")
	}
}
