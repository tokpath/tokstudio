package billing

import (
	"testing"
	"time"
)

func TestClassifyDiffMatchConfirmedCharge(t *testing.T) {
	row := classifyDiff(diffInput{
		RequestID:    "req_ok",
		UsageID:      "usg_ok",
		OccurredAt:   time.Unix(1, 0).UTC(),
		State:        UsageConfirmed,
		UsageMinor:   160000,
		ChargeMinor:  160000,
		LedgerDebit:  160000,
		ChargeCount:  1,
		MissingUsage: false,
	})
	if !row.Match || row.Status != DiffMatch {
		t.Fatalf("confirmed equal charge must match: %+v", row)
	}
	if row.AlreadyPending || row.MissingUsage {
		t.Fatalf("match must not look like a gap: %+v", row)
	}
}

func TestClassifyDiffMismatchMissingUsage(t *testing.T) {
	row := classifyDiff(diffInput{
		RequestID:     "req_omit",
		State:         UsagePending,
		UsageMinor:    0,
		ChargeMinor:   0,
		ReservedMinor: 1_000_000,
		MissingUsage:  true,
		AuthStatus:    AuthPendingReconciliation,
	})
	if row.Match || row.Status != DiffMismatch {
		t.Fatalf("missing usage must mismatch: %+v", row)
	}
	if !row.AlreadyPending || !row.MissingUsage {
		t.Fatalf("omit must already be pending: %+v", row)
	}
	if row.ChargeMinor != 0 {
		t.Fatalf("omit must not carry an estimated debit: %+v", row)
	}
}

func TestClassifyDiffMismatchUnequalCharge(t *testing.T) {
	row := classifyDiff(diffInput{
		RequestID:   "req_gap",
		State:       UsageConfirmed,
		UsageMinor:  200000,
		ChargeMinor: 100000,
		ChargeCount: 1,
	})
	if row.Match || row.Status != DiffMismatch {
		t.Fatalf("unequal charge must mismatch: %+v", row)
	}
}

func TestClassifyDiffMismatchDoubleCharge(t *testing.T) {
	row := classifyDiff(diffInput{
		RequestID:   "req_dup",
		State:       UsageConfirmed,
		UsageMinor:  160000,
		ChargeMinor: 320000,
		ChargeCount: 2,
	})
	if row.Match {
		t.Fatal("two customer charges must never classify as match")
	}
}

func TestClassifyDiffNoEstimateOnPending(t *testing.T) {
	row := classifyDiff(diffInput{
		RequestID:    "req_hold",
		State:        UsagePending,
		UsageMinor:   0,
		ChargeCount:  0,
		MissingUsage: true,
	})
	if row.UsageMinor != 0 || row.ChargeMinor != 0 || row.Match {
		t.Fatalf("pending gap must not invent a debit: %+v", row)
	}
}
