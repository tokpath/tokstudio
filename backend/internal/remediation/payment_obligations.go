package remediation

import (
	"context"
	"errors"
	"fmt"

	"github.com/tokpath/tokstudio/backend/internal/cooperation"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"gorm.io/gorm"
)

// CheckOperatorExit joins actual receipt obligations in the formal supply-exit
// transaction. Commercial order expiry is never evidence of repayment.
func (s *Service) CheckOperatorExit(ctx context.Context, tx *gorm.DB, operator string) ([]organization.Requirement, error) {
	if tx == nil || s.Payments == nil || s.Cooperation == nil || s.Commerce == nil {
		return nil, ErrNotReady
	}
	pending, e := s.Payments.PendingReceiptObligations(ctx, tx, operator, nil)
	if e != nil {
		return nil, e
	}
	funds, e := s.Commerce.UnsettledReceiptFunds(ctx, tx, operator, nil)
	if e != nil {
		return nil, e
	}
	procurementErr := s.Cooperation.RequireNoPendingProcurementReceipts(ctx, tx, operator, s.Payments)
	if procurementErr != nil && !errors.Is(procurementErr, cooperation.ErrLiability) {
		return nil, procurementErr
	}
	return []organization.Requirement{{Code: "pending_receipts", Description: fmt.Sprintf("%d actual original-currency receipts await verified valuation or receipt matching; %d recorded receipts retain unallocated principal", len(pending), len(funds)), Ready: len(pending) == 0 && len(funds) == 0}, {Code: "pending_procurement_receipts", Description: "actual procurement payments collected by suppliers must be valued and matched before exit", Ready: procurementErr == nil}}, nil
}

func (s *Service) requireTakeoverReceiptSettlement(ctx context.Context, tx *gorm.DB, operator string) error {
	checks, e := s.CheckOperatorExit(ctx, tx, operator)
	if e != nil {
		return e
	}
	for _, check := range checks {
		if !check.Ready {
			return fmt.Errorf("%w: %s: %s", ErrTakeoverBlocked, check.Code, check.Description)
		}
	}
	return nil
}
