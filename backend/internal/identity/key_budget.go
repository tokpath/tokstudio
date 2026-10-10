package identity

import "time"

const (
	BudgetPeriodLifetime = "lifetime"
	BudgetPeriodMonth    = "month"
	BudgetPeriodQuarter  = "quarter"
	BudgetPeriodYear     = "year"
)

func normalizeBudgetPeriod(period string, limited bool) (string, error) {
	if !limited {
		return BudgetPeriodLifetime, nil
	}
	if period == "" {
		period = BudgetPeriodLifetime
	}
	switch period {
	case BudgetPeriodLifetime, BudgetPeriodMonth, BudgetPeriodQuarter, BudgetPeriodYear:
		return period, nil
	default:
		return "", ErrInvalidKeyLimits
	}
}

func budgetPeriodStart(now time.Time, period string) time.Time {
	now = now.UTC()
	switch period {
	case BudgetPeriodMonth:
		return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	case BudgetPeriodQuarter:
		month := time.Month((int(now.Month())-1)/3*3 + 1)
		return time.Date(now.Year(), month, 1, 0, 0, 0, 0, time.UTC)
	case BudgetPeriodYear:
		return time.Date(now.Year(), time.January, 1, 0, 0, 0, 0, time.UTC)
	default:
		return time.Time{}
	}
}

// rollKeyBudget 进入新的月、季或年时，已用累计从零开始。处理中的占用仍保留，等原请求结束。
func rollKeyBudget(row *apiKeyRow, now time.Time) bool {
	if row == nil || row.BudgetLimitMinor == nil || row.BudgetPeriod == "" || row.BudgetPeriod == BudgetPeriodLifetime {
		return false
	}
	start := budgetPeriodStart(now, row.BudgetPeriod)
	if start.IsZero() {
		return false
	}
	if row.BudgetWindowStart != nil && !row.BudgetWindowStart.Before(start) {
		return false
	}
	row.BudgetUsedMinor = 0
	row.BudgetWindowStart = &start
	return true
}

func applyKeyBudget(used, reserved, deltaUsed, deltaReserved int64, limit *int64) (int64, int64) {
	used += deltaUsed
	reserved += deltaReserved
	if used < 0 {
		used = 0
	}
	if reserved < 0 {
		reserved = 0
	}
	if limit != nil && used > *limit {
		used = *limit
	}
	return used, reserved
}
