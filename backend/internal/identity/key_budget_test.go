package identity

import (
	"testing"
	"time"
)

func TestBudgetPeriodStart(t *testing.T) {
	now := time.Date(2026, time.May, 10, 15, 0, 0, 0, time.UTC)
	if got := budgetPeriodStart(now, BudgetPeriodMonth); !got.Equal(time.Date(2026, time.May, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("month start %s", got)
	}
	if got := budgetPeriodStart(now, BudgetPeriodQuarter); !got.Equal(time.Date(2026, time.April, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("quarter start %s", got)
	}
	if got := budgetPeriodStart(now, BudgetPeriodYear); !got.Equal(time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("year start %s", got)
	}
	if got := budgetPeriodStart(now, BudgetPeriodLifetime); !got.IsZero() {
		t.Fatalf("lifetime has no window, got %s", got)
	}
}

func TestRollKeyBudgetResetsUsedAtPeriodBoundary(t *testing.T) {
	limit := int64(100)
	old := time.Date(2026, time.April, 1, 0, 0, 0, 0, time.UTC)
	row := apiKeyRow{BudgetLimitMinor: &limit, BudgetPeriod: BudgetPeriodMonth, BudgetUsedMinor: 80, BudgetReservedMinor: 10, BudgetWindowStart: &old}
	if !rollKeyBudget(&row, time.Date(2026, time.May, 2, 0, 0, 0, 0, time.UTC)) {
		t.Fatal("expected a new month")
	}
	if row.BudgetUsedMinor != 0 || row.BudgetReservedMinor != 10 {
		t.Fatalf("used reset, reservation kept: %+v", row)
	}
	used, reserved := applyKeyBudget(row.BudgetUsedMinor, row.BudgetReservedMinor, 150, -10, row.BudgetLimitMinor)
	if used != 100 || reserved != 0 {
		t.Fatalf("remaining floors at zero, got used %d reserved %d", used, reserved)
	}
}

func TestNormalizeBudgetPeriod(t *testing.T) {
	if got, err := normalizeBudgetPeriod("", true); err != nil || got != BudgetPeriodLifetime {
		t.Fatalf("default lifetime %s %v", got, err)
	}
	if got, err := normalizeBudgetPeriod("month", true); err != nil || got != "month" {
		t.Fatalf("month %s %v", got, err)
	}
	if _, err := normalizeBudgetPeriod("weekly", true); err == nil {
		t.Fatal("unknown period accepted")
	}
	if got, err := normalizeBudgetPeriod("weekly", false); err != nil || got != BudgetPeriodLifetime {
		t.Fatalf("unlimited ignores period %s %v", got, err)
	}
}
