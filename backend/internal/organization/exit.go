package organization

import (
	"context"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// OperatorExitChecker must include real, unsettled receipts even when their
// original commercial order has expired. A missing checker cannot close supply.
type OperatorExitChecker interface {
	CheckOperatorExit(context.Context, *gorm.DB, string) ([]Requirement, error)
}

// Configure before concurrent request handling starts.
func (s *Service) SetOperatorExitChecker(checker OperatorExitChecker) { s.exitChecker = checker }

// LockOperatorResponsibility fences the recording of actual settlement facts
// against an operator takeover or formal exit. It never rejects a real fact
// merely because new business has stopped.
func (s *Service) LockOperatorResponsibility(ctx context.Context, tx *gorm.DB, operator string) error {
	if tx == nil || !validID(operator) {
		return ErrInvalidInput
	}
	if _, ok := tx.Statement.ConnPool.(gorm.TxCommitter); !ok {
		return ErrInvalidInput
	}
	var row operatorRow
	return classify(tx.WithContext(ctx).Clauses(clause.Locking{Strength: "SHARE"}).First(&row, "id=?", operator).Error)
}

// LockCustomerResponsibility is a trusted settlement fence after the payment
// source domain has resolved the original beneficiary. An old receipt remains
// recordable after a change of service operator; the customer identity persists.
func (s *Service) LockCustomerResponsibility(ctx context.Context, tx *gorm.DB, customer string) error {
	if tx == nil || !validID(customer) {
		return ErrInvalidInput
	}
	if _, ok := tx.Statement.ConnPool.(gorm.TxCommitter); !ok {
		return ErrInvalidInput
	}
	var row customerRow
	return classify(tx.WithContext(ctx).Clauses(clause.Locking{Strength: "SHARE"}).First(&row, "id=?", customer).Error)
}
