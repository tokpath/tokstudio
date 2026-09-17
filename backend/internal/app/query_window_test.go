package app

import (
	"testing"
	"time"
)

func TestParseQueryWindowHalfOpen(t *testing.T) {
	since, until, err := parseQueryWindow("2026-09-16", "2026-09-16")
	if err != nil {
		t.Fatal(err)
	}
	if !since.Equal(time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("date-only from: %s", since)
	}
	if !until.Equal(time.Date(2026, 9, 17, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("date-only to must be next UTC midnight exclusive: %s", until)
	}

	since, until, err = parseQueryWindow("2026-09-15T16:00:00Z", "2026-09-16T16:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	last := time.Date(2026, 9, 16, 15, 59, 59, 0, time.UTC)
	next := time.Date(2026, 9, 16, 16, 0, 0, 0, time.UTC)
	if last.Before(since) || !last.Before(until) {
		t.Fatalf("end-of-day instant must be in [%s, %s): last=%s", since, until, last)
	}
	if !next.Before(until) && !next.Equal(until) {
		t.Fatalf("next-day start is the exclusive end: until=%s next=%s", until, next)
	}
	if !next.Equal(until) {
		t.Fatalf("exclusive end should equal next local midnight UTC: %s", until)
	}

	if _, _, err = parseQueryWindow("2026-09-17", "2026-09-16"); err != errInvalidQueryRange {
		t.Fatalf("reversed dates: %v", err)
	}
	if _, _, err = parseQueryWindow("not-a-date", ""); err != errInvalidQueryTime {
		t.Fatalf("invalid from: %v", err)
	}
	if _, _, err = parseQueryWindow("", "2026-13-40"); err != errInvalidQueryTime {
		t.Fatalf("invalid to: %v", err)
	}
}
