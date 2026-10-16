package ledger

import (
	"errors"
	"time"
)

type Period string

const (
	Lifetime Period = "lifetime"
	Month    Period = "month"
	Quarter  Period = "quarter"
	Year     Period = "year"
)

var ErrPeriod = errors.New("invalid period or timezone")

func PeriodEnd(start time.Time, period Period, timezone string) (*time.Time, error) {
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return nil, ErrPeriod
	}
	months := 0
	switch period {
	case Lifetime:
		return nil, nil
	case Month:
		months = 1
	case Quarter:
		months = 3
	case Year:
		months = 12
	default:
		return nil, ErrPeriod
	}
	s := start.In(loc)
	first := time.Date(s.Year(), s.Month()+time.Month(months), 1, s.Hour(), s.Minute(), s.Second(), s.Nanosecond(), loc)
	last := time.Date(first.Year(), first.Month()+1, 0, 12, 0, 0, 0, loc).Day()
	day := s.Day()
	if day > last {
		day = last
	}
	end := time.Date(first.Year(), first.Month(), day, s.Hour(), s.Minute(), s.Second(), s.Nanosecond(), loc).UTC()
	return &end, nil
}

type Window struct {
	Start time.Time  `json:"start"`
	End   *time.Time `json:"end,omitempty"`
}

func BudgetWindow(accepted, created time.Time, period Period, timezone string) (Window, error) {
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return Window{}, ErrPeriod
	}
	if accepted.Before(created) {
		return Window{}, ErrPeriod
	}
	if period == Lifetime {
		return Window{Start: created.UTC()}, nil
	}
	t := accepted.In(loc)
	month := t.Month()
	months := 1
	switch period {
	case Month:
	case Quarter:
		month = time.Month(((int(month)-1)/3)*3 + 1)
		months = 3
	case Year:
		month = 1
		months = 12
	default:
		return Window{}, ErrPeriod
	}
	start := time.Date(t.Year(), month, 1, 0, 0, 0, 0, loc).UTC()
	end := time.Date(t.Year(), month+time.Month(months), 1, 0, 0, 0, 0, loc).UTC()
	return Window{start, &end}, nil
}
