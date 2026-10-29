package usage

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/modelruntime"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

type loadSpanMetrics struct {
	mu     sync.Mutex
	values map[string][]float64
}

func (*loadSpanMetrics) OnStart(context.Context, sdktrace.ReadWriteSpan) {}
func (m *loadSpanMetrics) OnEnd(s sdktrace.ReadOnlySpan) {
	if !strings.HasPrefix(s.Name(), "usage.admit") && !strings.HasPrefix(s.Name(), "usage.settle") {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.values[s.Name()] = append(m.values[s.Name()], float64(s.EndTime().Sub(s.StartTime()).Microseconds())/1000)
}
func (*loadSpanMetrics) Shutdown(context.Context) error   { return nil }
func (*loadSpanMetrics) ForceFlush(context.Context) error { return nil }
func (m *loadSpanMetrics) report() map[string]any {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string]any{}
	for name, values := range m.values {
		sort.Float64s(values)
		out[name] = map[string]any{"samples": len(values), "p50_ms": values[int(float64(len(values)-1)*.5)], "p95_ms": values[int(float64(len(values)-1)*.95)], "p99_ms": values[int(float64(len(values)-1)*.99)]}
	}
	return out
}

// Opt-in mixed-load test; no fake upstream adapter, no ledger seed writes, no
// production endpoints. One hot customer deliberately stresses shared balances.
// Reported latency includes database queueing and external service time; it is
// not advertised as isolated platform overhead or commercial capacity.
func TestExternalConcurrentCallingAndSalesSoak(t *testing.T) {
	raw := os.Getenv("TOKENHUB_LOAD_DURATION")
	if raw == "" {
		t.Skip("set an explicit isolated load duration")
	}
	duration, e := time.ParseDuration(raw)
	if e != nil || duration < 10*time.Second || duration > 40*time.Minute {
		t.Fatal("load duration must be 10s..40m")
	}
	base := strings.TrimRight(os.Getenv("TOKENHUB_MODEL_MOCK_URL"), "/")
	origin, e := url.Parse(base)
	if e != nil || origin.Hostname() != "127.0.0.1" {
		t.Fatal("separately deployed loopback mock required")
	}
	calls, orders := 100, 50
	if v := os.Getenv("TOKENHUB_LOAD_CALLERS"); v != "" {
		calls, e = strconv.Atoi(v)
		if e != nil || calls < 1 || calls > 200 {
			t.Fatal("callers must be 1..200")
		}
	}
	if v := os.Getenv("TOKENHUB_LOAD_BUYERS"); v != "" {
		orders, e = strconv.Atoi(v)
		if e != nil || orders < 1 || orders > 100 {
			t.Fatal("buyers must be 1..100")
		}
	}
	metrics := &loadSpanMetrics{values: map[string][]float64{}}
	previous := otel.GetTracerProvider()
	tracing := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(metrics))
	otel.SetTracerProvider(tracing)
	defer func() { _ = tracing.Shutdown(context.Background()); otel.SetTracerProvider(previous) }()
	f, inc, _ := correctionFixture(t)
	if f.s.ResultStore == nil {
		t.Fatal("private S3 required")
	}
	f.s.UsageTimeout = 5 * time.Minute
	sqlDB, e := f.db.DB()
	if e != nil {
		t.Fatal(e)
	}
	sqlDB.SetMaxOpenConns(32)
	sqlDB.SetMaxIdleConns(32)
	c := commerce.New(f.db)
	c.SetSalesCoverage(f.coop)
	c.SetMarketing(inc)
	q, e := c.PublishPackage(f.ctx, nil, commerce.PublishPackageInput{ActorStaffID: f.owner, OperatorID: f.operator, Name: "Load package", TermsReference: "load-terms", Timezone: "UTC", PriceMoney: d("1"), PaidATG: d("800"), GiftATG: d("200"), Period: ledger.Month, NewPurchaseEnabled: true})
	if e != nil {
		t.Fatal(e)
	}
	top, e := c.CreateTopUpOrder(f.ctx, nil, commerce.CreateTopUpInput{CustomerID: f.customer, OperatorID: f.operator, BusinessKey: "load-wallet", ExpectedOwnershipVersion: 1, AmountMoney: d("1000000")})
	if e != nil {
		t.Fatal(e)
	}
	gross, zero := d("1000000"), ledger.Zero()
	receipt, e := c.RecordManualReceipt(f.ctx, nil, f.owner, commerce.VerifiedReceipt{OperatorID: f.operator, ProviderReceiptID: "load-synthetic-bank", GrossMoney: gross, NetMoney: &gross, FeeMoney: &zero, Currency: "USD", OriginalAmount: "1000000", RecipientReference: "synthetic-merchant", PayerReference: "synthetic-payer", ValuationEvidence: "USD original", VerificationEvidence: "isolated test bank statement", PaidAt: &top.CreatedAt, SettledAt: &top.CreatedAt})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = c.AllocateReceipt(f.ctx, nil, commerce.AllocateReceiptInput{ActorStaffID: f.owner, OperatorID: f.operator, BusinessKey: "fund-load-wallet", ReceiptID: receipt.ID, OrderID: top.ID, AmountMoney: gross}); e != nil {
		t.Fatal(e)
	}
	if _, e = c.Fulfill(f.ctx, nil, top.ID, f.owner); e != nil {
		t.Fatal(e)
	}
	buy := func(ctx context.Context, key string) error {
		o, e := c.CreateOrder(ctx, nil, commerce.CreateOrderInput{CustomerID: f.customer, OperatorID: f.operator, QuoteID: q.ID, BusinessKey: key, ExpectedOwnershipVersion: 1, InternalMoney: d("1")})
		if e != nil {
			return e
		}
		_, e = c.Fulfill(ctx, nil, o.ID, f.owner)
		return e
	}
	if e = buy(f.ctx, "opening-package"); e != nil {
		t.Fatal(e)
	}
	runtime, e := modelruntime.NewBifrost(f.ctx, modelruntime.BifrostConfig{Providers: []string{"openai"}, AllowTestLoopback: true, Credentials: f.cat, EndpointGuard: func(_ context.Context, target string) error {
		u, e := url.Parse(target)
		if e != nil || u.Host != origin.Host {
			return modelruntime.ErrInvalidRequest
		}
		return nil
	}})
	if e != nil {
		t.Fatal(e)
	}
	defer runtime.Close()
	f.s.Runtime = runtime
	type samples struct {
		mu       sync.Mutex
		ms       []float64
		failures map[string]int
		success  int
	}
	callStats, orderStats := &samples{failures: map[string]int{}}, &samples{failures: map[string]int{}}
	record := func(s *samples, started time.Time, e error) {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.ms = append(s.ms, float64(time.Since(started).Microseconds())/1000)
		if e != nil {
			s.failures[e.Error()]++
		} else {
			s.success++
		}
	}
	input := request(t, false)
	start := time.Now()
	deadline := start.Add(duration)
	var seq atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < calls; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for time.Now().Before(deadline) {
				key := fmt.Sprintf("load-call-%d", seq.Add(1))
				at := time.Now()
				ctx, cancel := context.WithTimeout(f.ctx, 2*time.Minute)
				ex, e := f.s.Execute(ctx, f.key.Key, key, input, nil)
				cancel()
				if e == nil && ex.Request.Status != "completed" {
					e = fmt.Errorf("unexpected terminal state %s", ex.Request.Status)
				}
				record(callStats, at, e)
			}
		}()
	}
	for i := 0; i < orders; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for time.Now().Before(deadline) {
				key := fmt.Sprintf("load-order-%d", seq.Add(1))
				at := time.Now()
				ctx, cancel := context.WithTimeout(f.ctx, 2*time.Minute)
				e := buy(ctx, key)
				cancel()
				record(orderStats, at, e)
				if remaining := time.Until(deadline); remaining > 0 {
					delay := 2 * time.Second
					if remaining < delay {
						delay = remaining
					}
					time.Sleep(delay)
				}
			}
		}()
	}
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
progress:
	for {
		select {
		case <-done:
			break progress
		case <-ticker.C:
			callStats.mu.Lock()
			nc := callStats.success
			callStats.mu.Unlock()
			orderStats.mu.Lock()
			no := orderStats.success
			orderStats.mu.Unlock()
			t.Logf("elapsed=%s completed_calls=%d fulfilled_orders=%d", time.Since(start).Round(time.Second), nc, no)
		}
	}
	summarize := func(s *samples) map[string]any {
		sort.Float64s(s.ms)
		p := func(n float64) float64 {
			if len(s.ms) == 0 {
				return 0
			}
			i := int(float64(len(s.ms)-1) * n)
			return s.ms[i]
		}
		return map[string]any{"success": s.success, "errors": s.failures, "samples": len(s.ms), "p50_ms": p(.5), "p95_ms": p(.95), "p99_ms": p(.99)}
	}
	var bad int64
	check := `SELECT count(*) FROM (
 SELECT j.id FROM ledger_journals j LEFT JOIN ledger_entries e ON e.journal_id=j.id GROUP BY j.id,e.unit HAVING count(e.id)=0 OR sum(e.amount)<>0
 UNION ALL SELECT a.id FROM ledger_accounts a LEFT JOIN (SELECT account_id,sum(amount)amount FROM ledger_entries GROUP BY account_id)e ON e.account_id=a.id WHERE a.balance<>coalesce(e.amount,0)
 UNION ALL SELECT a.id FROM ledger_accounts a LEFT JOIN (SELECT ha.account_id,sum(ha.amount)amount FROM ledger_hold_allocations ha JOIN ledger_holds h ON h.id=ha.hold_id WHERE h.status='active' GROUP BY ha.account_id)h ON h.account_id=a.id WHERE a.held<>coalesce(h.amount,0)
 )discrepancies`
	if e = f.db.Raw(check).Row().Scan(&bad); e != nil {
		t.Fatal(e)
	}
	var pending int64
	if e = f.db.Model(&Request{}).Where("business_key LIKE 'load-call-%' AND status <> 'completed'").Count(&pending).Error; e != nil {
		t.Fatal(e)
	}
	var duplicates int64
	if e = f.db.Raw("SELECT count(*) FROM (SELECT request_id FROM usage_attempts GROUP BY request_id HAVING count(*)>1)x").Row().Scan(&duplicates); e != nil {
		t.Fatal(e)
	}
	report := map[string]any{"duration_seconds": time.Since(start).Seconds(), "callers": calls, "buyers": orders, "db_max_connections": 32, "profile": "one_hot_customer_real_PG_Bifrost_HTTP_private_S3", "calls": summarize(callStats), "orders": summarize(orderStats), "ledger_discrepancies": bad, "unfinished_load_calls": pending, "duplicate_upstream_attempts": duplicates, "includes_queueing_and_external_io": true}
	report["billing_spans"] = metrics.report()
	report["after_locks_span_scope"] = "excludes initial business-fence queueing and commit; includes remaining SQL, ledger and incentive work"
	report["committed_work_span_scope"] = "initial business fences acquired through transaction commit or rollback; excludes initial queueing, includes remaining SQL/ledger/incentives; emitted only for method-owned transactions"
	b, _ := json.MarshalIndent(report, "", "  ")
	t.Log(string(b))
	if p := os.Getenv("TOKENHUB_LOAD_REPORT"); p != "" {
		if e = os.WriteFile(p, b, 0600); e != nil {
			t.Fatal(e)
		}
	}
	if bad != 0 || duplicates != 0 || pending != 0 || len(callStats.failures) > 0 || len(orderStats.failures) > 0 {
		t.Fatal("mixed load uncovered unfinished or failed work; inspect report")
	}
}
