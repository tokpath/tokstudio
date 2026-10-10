package billing

import "testing"

func TestCapWalletChargeNeverGoesNegative(t *testing.T) {
	if got := capWalletCharge(120000, 0, 100000, 0); got != 100000 {
		t.Fatalf("cash stops at the reserved balance, got %d", got)
	}
	if got := capWalletCharge(1100000, 0, 100000, 900000); got != 1000000 {
		t.Fatalf("cash stops at zero, got %d", got)
	}
	if got := capWalletCharge(15, 10, 0, 50); got != 5 {
		t.Fatalf("amount inside the cash balance is collected, got %d", got)
	}
}
