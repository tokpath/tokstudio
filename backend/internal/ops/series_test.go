package ops

import (
	"testing"
	"time"
)

func TestClampThresholds(t *testing.T) {
	got := ClampThresholds(Thresholds{SuccessRateMin: 0, MinRequests: 0, PendingCount: 0})
	if got.SuccessRateMin != 0.5 || got.MinRequests != 5 || got.PendingCount != 1 {
		t.Fatalf("defaults: %+v", got)
	}
	got = ClampThresholds(Thresholds{SuccessRateMin: 0.8, MinRequests: 10, PendingCount: 3})
	if got.SuccessRateMin != 0.8 || got.MinRequests != 10 || got.PendingCount != 3 {
		t.Fatalf("keep: %+v", got)
	}
	got = ClampThresholds(Thresholds{SuccessRateMin: 1.5, MinRequests: 20000, PendingCount: 2})
	if got.SuccessRateMin != 0.5 || got.MinRequests != 10000 || got.PendingCount != 2 {
		t.Fatalf("clamp: %+v", got)
	}
}

func TestClampDays(t *testing.T) {
	if ClampDays(0) != 7 || ClampDays(120) != 90 || ClampDays(14) != 14 {
		t.Fatalf("clamp: %d %d %d", ClampDays(0), ClampDays(120), ClampDays(14))
	}
}

func TestMergeDailyFillsEmptyDays(t *testing.T) {
	now := time.Date(2026, 8, 29, 15, 0, 0, 0, time.UTC)
	points := MergeDaily(3, []DailyTraffic{{Day: "2026-08-29", Requests: 4, Successes: 3, Errors: 1}}, []DailyMoney{{Day: "2026-08-29", RevenueMinor: 10, CostMinor: 4}}, now)
	if len(points) != 3 {
		t.Fatalf("len=%d", len(points))
	}
	if points[0].Day != "2026-08-27" || points[2].Day != "2026-08-29" {
		t.Fatalf("days=%v %v", points[0].Day, points[2].Day)
	}
	if points[2].Requests != 4 || points[2].MarginMinor != 6 || points[2].SuccessRate != 0.75 {
		t.Fatalf("today=%+v", points[2])
	}
	if points[0].Requests != 0 {
		t.Fatalf("empty day should stay zero: %+v", points[0])
	}
}
