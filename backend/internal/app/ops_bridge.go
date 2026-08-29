package app

import (
	"context"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/billing"
	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"github.com/tokpath/tokstudio/backend/internal/gateway"
	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/ops"
)

type trafficBridge struct {
	gateway *gateway.Service
}

func (b *trafficBridge) DimStats(ctx context.Context, dimension string) ([]ops.DimStat, error) {
	rows, err := b.gateway.DimStats(ctx, dimension)
	if err != nil {
		return nil, err
	}
	out := make([]ops.DimStat, 0, len(rows))
	for _, row := range rows {
		out = append(out, ops.DimStat{
			Dimension: row.Dimension, Key: row.Key, Requests: row.Requests,
			Successes: row.Successes, Errors: row.Errors, SuccessRate: row.SuccessRate,
			LatencyP50MS: row.LatencyP50MS, LatencyP95MS: row.LatencyP95MS, Fallbacks: row.Fallbacks,
		})
	}
	return out, nil
}

func (b *trafficBridge) DailySeries(ctx context.Context, since time.Time) ([]ops.DailyTraffic, error) {
	rows, err := b.gateway.DailySeries(ctx, since)
	if err != nil {
		return nil, err
	}
	out := make([]ops.DailyTraffic, 0, len(rows))
	for _, row := range rows {
		out = append(out, ops.DailyTraffic{Day: row.Day, Requests: row.Requests, Successes: row.Successes, Errors: row.Errors})
	}
	return out, nil
}

type moneyBridge struct {
	billing *billing.Service
}

func (b *moneyBridge) Money(ctx context.Context) (*ops.MoneyView, error) {
	item, err := b.billing.Report(ctx)
	if err != nil {
		return nil, err
	}
	view := &ops.MoneyView{
		RevenueMinor: item.RevenueMinor, UpstreamMinor: item.UpstreamMinor,
		WholesaleMinor: item.WholesaleMinor, CommissionMinor: item.CommissionMinor,
		RefundMinor: item.RefundMinor, GrossProfitMinor: item.GrossProfitMinor,
		PendingCount: item.PendingCount,
	}
	if risk, err := b.billing.Risk(ctx); err == nil && risk != nil {
		view.LowBalanceWallets = risk.LowBalanceWallets
		view.ReservedMinor = risk.ReservedMinor
		view.ChannelSpendMinor = risk.ChannelSpendMinor
	}
	return view, nil
}

func (b *moneyBridge) DimMoney(ctx context.Context, dimension string) ([]ops.DimStat, error) {
	rows, err := b.billing.DimMoney(ctx, dimension)
	if err != nil {
		return nil, err
	}
	out := make([]ops.DimStat, 0, len(rows))
	for _, row := range rows {
		out = append(out, ops.DimStat{
			Dimension: row.Dimension, Key: row.Key, UsageMinor: row.UsageMinor,
			RevenueMinor: row.RevenueMinor, CostMinor: row.CostMinor,
			MarginMinor: row.RevenueMinor - row.CostMinor,
		})
	}
	return out, nil
}

func (b *moneyBridge) DailySeries(ctx context.Context, since time.Time) ([]ops.DailyMoney, error) {
	rows, err := b.billing.DailySeries(ctx, since)
	if err != nil {
		return nil, err
	}
	out := make([]ops.DailyMoney, 0, len(rows))
	for _, row := range rows {
		out = append(out, ops.DailyMoney{
			Day: row.Day, UsageMinor: row.UsageMinor, RevenueMinor: row.RevenueMinor, CostMinor: row.CostMinor,
		})
	}
	return out, nil
}

type healthBridge struct {
	catalog *catalog.Service
}

func (b *healthBridge) MarkHealth(ctx context.Context, providerID, health string) error {
	return b.catalog.MarkHealth(ctx, providerID, health)
}

type roleBridge struct {
	identity *identity.Service
}

func (b *roleBridge) MapUserRoles(ctx context.Context, userIDs []string) (map[string]string, error) {
	return b.identity.MapUserAcquisitionRoles(ctx, userIDs)
}
