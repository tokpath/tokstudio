package app_test

import (
	"context"
	"errors"
	"github.com/tokpath/tokstudio/backend/internal/ops"
	"github.com/tokpath/tokstudio/backend/internal/platform/config"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

type dashboardTrafficStub struct{}

func (dashboardTrafficStub) DimStats(_ context.Context, dim string) ([]ops.DimStat, error) {
	return []ops.DimStat{{Dimension: dim, Key: "fixture", Requests: 10, Successes: 9, Errors: 1}}, nil
}
func (dashboardTrafficStub) DailySeries(context.Context, time.Time) ([]ops.DailyTraffic, error) {
	return nil, nil
}
func (dashboardTrafficStub) ErrorBreakdown(context.Context) (map[string]int64, error) {
	return map[string]int64{"timeout": 1}, nil
}

type dashboardMoneyFailure struct{}

func (dashboardMoneyFailure) Money(context.Context) (*ops.MoneyView, error) {
	return nil, errors.New("isolated read failure")
}
func (dashboardMoneyFailure) DimMoney(context.Context, string) ([]ops.DimStat, error) {
	return nil, errors.New("isolated read failure")
}
func (dashboardMoneyFailure) DailySeries(context.Context, time.Time) ([]ops.DailyMoney, error) {
	return nil, errors.New("isolated read failure")
}
func TestOverhaulDashboardPartialFailure(t *testing.T) {
	if os.Getenv("TOKENHUB_DATABASE_URL") == "" {
		t.Skip("isolated postgres required")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.BootstrapAdmin = "dashboard-isolated-admin"
	cfg.BootstrapUser = "dashboard-isolated-user"
	a := mustApp(t, cfg)
	a.Ops.SetSources(dashboardTrafficStub{}, dashboardMoneyFailure{}, nil, nil, nil)
	server := httptest.NewServer(a.Router())
	defer server.Close()
	dashboard := getAuthJSON(t, server.URL+"/admin/ops/dashboard", cfg.BootstrapAdmin)["dashboard"].(map[string]any)
	totals := dashboard["totals"].(map[string]any)
	if _, ok := totals["revenue_minor"]; ok {
		t.Fatalf("failed finance became zero: %+v", totals)
	}
	if totals["success_rate"] != 0.9 {
		t.Fatalf("healthy traffic disappeared after money failure: %+v", totals)
	}
	if dashboard["module_errors"].(map[string]any)["finance"] != "read_error" {
		t.Fatalf("partial failure not identified %+v", dashboard)
	}
	if dashboard["runbooks"] == nil || dashboard["provider_health"] == nil {
		t.Fatalf("unrelated tasks disappeared %+v", dashboard)
	}
}
