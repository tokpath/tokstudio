package ops

import (
	"context"
	"time"
)

func ClampDays(days int) int {
	if days < 1 {
		return 7
	}
	if days > 90 {
		return 90
	}
	return days
}

func SeriesSince(now time.Time, days int) time.Time {
	days = ClampDays(days)
	today := time.Date(now.UTC().Year(), now.UTC().Month(), now.UTC().Day(), 0, 0, 0, 0, time.UTC)
	return today.AddDate(0, 0, -(days - 1))
}

func MergeDaily(days int, traffic []DailyTraffic, money []DailyMoney, now time.Time) []DayPoint {
	days = ClampDays(days)
	byDay := map[string]DayPoint{}
	for _, row := range traffic {
		point := byDay[row.Day]
		point.Day = row.Day
		point.Requests = row.Requests
		point.Successes = row.Successes
		point.Errors = row.Errors
		if row.Requests > 0 {
			point.SuccessRate = float64(row.Successes) / float64(row.Requests)
		}
		byDay[row.Day] = point
	}
	for _, row := range money {
		point := byDay[row.Day]
		point.Day = row.Day
		point.UsageMinor = row.UsageMinor
		point.RevenueMinor = row.RevenueMinor
		point.CostMinor = row.CostMinor
		point.MarginMinor = row.RevenueMinor - row.CostMinor
		byDay[row.Day] = point
	}
	out := make([]DayPoint, 0, days)
	start := SeriesSince(now, days)
	today := time.Date(now.UTC().Year(), now.UTC().Month(), now.UTC().Day(), 0, 0, 0, 0, time.UTC)
	for day := start; !day.After(today); day = day.AddDate(0, 0, 1) {
		key := day.Format("2006-01-02")
		point := byDay[key]
		point.Day = key
		out = append(out, point)
	}
	return out
}

func (s *Service) Series(ctx context.Context, days int) ([]DayPoint, error) {
	days = ClampDays(days)
	now := time.Now().UTC()
	since := SeriesSince(now, days)
	var traffic []DailyTraffic
	var money []DailyMoney
	var err error
	if s.traffic != nil {
		traffic, err = s.traffic.DailySeries(ctx, since)
		if err != nil {
			return nil, err
		}
	}
	if s.money != nil {
		money, err = s.money.DailySeries(ctx, since)
		if err != nil {
			return nil, err
		}
	}
	return MergeDaily(days, traffic, money, now), nil
}
