package commerce

import (
	"context"
	"gorm.io/gorm"
)

// Wallet balance and holds are authoritative in ledger. This scope fence
// coordinates account operations with ownership transfers and receipt disputes.
func walletScope(tx *gorm.DB, operator, customer string) error {
	return lockCommand(tx, "wallet", operator+":"+customer)
}
func (s *Service) LockWallet(ctx context.Context, tx *gorm.DB, operator, customer string) error {
	if tx == nil || !validID(operator) || !validID(customer) {
		return ErrInvalidInput
	}
	return walletScope(tx.WithContext(ctx), operator, customer)
}
