package cooperation

import (
	"context"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"gorm.io/gorm"
)

// TransferSalesCommitments moves already-promised, unissued registration gifts
// with their beneficiaries. It does not sell a new package or create inventory.
func (s *Service) TransferSalesCommitments(ctx context.Context, t *gorm.DB, in organization.ResponsibilityTransferInput) error {
	if t == nil {
		return ErrConflict
	}
	if e := s.Organization.LockResponsibilityTransfer(ctx, t, in); e != nil {
		return e
	}
	if e := s.Lock(ctx, t, in.FromOperatorID); e != nil {
		return e
	}
	var rows []SalesCommitment
	if e := t.Where("operator_id=? AND status='active'", in.FromOperatorID).Order("id").Find(&rows).Error; e != nil {
		return e
	}
	for _, row := range rows {
		var n int64
		if e := t.Table("incentives_registration_promises").Where("intent_id=? AND operator_id=? AND status='pending_delivery'", row.OrderID, in.ToOperatorID).Count(&n).Error; e != nil {
			return e
		}
		if n != 1 || row.BuyerID != nil {
			return ErrConflict
		}
		if e := t.Model(&SalesCommitment{}).Where("id=?", row.ID).Update("operator_id", in.ToOperatorID).Error; e != nil {
			return e
		}
	}
	return nil
}
