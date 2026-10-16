package organization

import (
	"context"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ResponsibilityTransferInput is an internal all-or-nothing channel takeover
// command. Customers contains every old ownership version, never a partial set.
type ResponsibilityTransferInput struct {
	FromOperatorID, ToOperatorID, TakeoverID, ActorID, Evidence string
	Customers                                                   map[string]int64
	AccountIDs                                                  map[string]string
	HoldIDs                                                     map[string]string
}

// LockResponsibilityTransfer validates the brand boundary and locks the full
// customer set. It accepts the ownership stage immediately before or after the
// same transaction's ownership transfer; it never updates assets by itself.
func (s *Service) LockResponsibilityTransfer(ctx context.Context, tx *gorm.DB, in ResponsibilityTransferInput) error {
	if tx == nil || !validID(in.FromOperatorID) || !validID(in.ToOperatorID) || !validID(in.TakeoverID) || in.ActorID == "" || strings.TrimSpace(in.Evidence) == "" || in.FromOperatorID == in.ToOperatorID {
		return ErrInvalidInput
	}
	from, e := s.GetOperator(ctx, tx, in.FromOperatorID)
	if e != nil {
		return e
	}
	to, e := s.GetOperator(ctx, tx, in.ToOperatorID)
	if e != nil {
		return e
	}
	brand, e := s.GetBrand(ctx, tx, from.BrandID)
	if e != nil {
		return e
	}
	if from.Kind != Channel || from.ParentOperatorID == nil || *from.ParentOperatorID != to.ID || from.BrandID != to.BrandID || brand.OwnerOperatorID != to.ID {
		return ErrInvalidRelationship
	}
	var current []ownershipRow
	if e = tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("ended_at IS NULL AND (operator_id=? OR customer_id IN ?)", in.FromOperatorID, transferCustomerIDs(in.Customers)).Order("customer_id").Find(&current).Error; e != nil {
		return e
	}
	if len(current) != len(in.Customers) {
		return ErrStaleOwnership
	}
	for _, r := range current {
		v, ok := in.Customers[r.CustomerID]
		if !ok || v < 1 {
			return ErrStaleOwnership
		}
		if r.OperatorID == in.FromOperatorID && r.Version == v {
			continue
		}
		if r.OperatorID != in.ToOperatorID || r.Version != v+1 {
			return ErrStaleOwnership
		}
		var count int64
		if e = tx.Model(&ownershipRow{}).Where("customer_id=? AND operator_id=? AND version=? AND ended_at IS NOT NULL", r.CustomerID, in.FromOperatorID, v).Count(&count).Error; e != nil {
			return e
		}
		if count != 1 {
			return ErrStaleOwnership
		}
	}
	return nil
}
func transferCustomerIDs(customers map[string]int64) []string {
	out := make([]string, 0, len(customers))
	for id := range customers {
		out = append(out, id)
	}
	return out
}
