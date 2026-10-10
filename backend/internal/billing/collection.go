package billing

// capWalletCharge 限制从现金里实扣的金额，使可用余额不会变成负数。
func capWalletCharge(actual, entitlement, walletReserved, walletAvailable int64) int64 {
	if actual < 0 {
		actual = 0
	}
	if entitlement < 0 {
		entitlement = 0
	}
	if entitlement > actual {
		entitlement = actual
	}
	if walletReserved < 0 {
		walletReserved = 0
	}
	if walletAvailable < 0 {
		walletAvailable = 0
	}
	need := actual - entitlement
	maxWallet := walletReserved + walletAvailable
	if need > maxWallet {
		return maxWallet
	}
	if need < 0 {
		return 0
	}
	return need
}
