package plans

import (
	"testing"
	"time"
)

func TestPeriodEndOnceDoesNotExpire(t *testing.T) {
	start := time.Date(2026, time.May, 10, 15, 0, 0, 0, time.UTC)
	if got := periodEnd(start, PeriodOnce); got != nil {
		t.Fatalf("once credit has no end, got %s", got)
	}
	if got := periodEnd(start, ""); got != nil {
		t.Fatalf("missing period has no end, got %s", got)
	}
	if got := periodEnd(start, PeriodMonthly); got == nil || !got.Equal(start.AddDate(0, 1, 0)) {
		t.Fatalf("month end %v", got)
	}
}
