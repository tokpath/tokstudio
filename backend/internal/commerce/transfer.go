package commerce

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/tokpath/tokstudio/backend/internal/organization"
	"gorm.io/gorm"
)

type TransferInput = organization.ResponsibilityTransferInput

var ErrTransferBlocked = errors.New("commerce: takeover requires outstanding business resolution")

func (s *Service) PreflightTransferResponsibility(ctx context.Context, tx *gorm.DB, in TransferInput) error {
	if err := s.org.LockResponsibilityTransfer(ctx, tx, in); err != nil {
		return err
	}
	var unfinished orderRow
	q := tx.Where("operator_id=? AND status NOT IN ('completed','closed')", in.FromOperatorID).Limit(1).Find(&unfinished)
	if q.Error != nil {
		return q.Error
	}
	if q.RowsAffected > 0 {
		return fmt.Errorf("%w: order %s is %s", ErrTransferBlocked, unfinished.ID, unfinished.Status)
	}
	var collisions int64
	if err := tx.Raw(`SELECT count(*) FROM commerce_orders a JOIN commerce_orders b ON b.business_key=a.business_key AND b.customer_id=a.customer_id AND b.operator_id=? WHERE a.operator_id=?`, in.ToOperatorID, in.FromOperatorID).Scan(&collisions).Error; err != nil {
		return err
	}
	if collisions > 0 {
		return ErrConflict
	}
	return nil
}

// TransferResponsibility moves future service scope, retaining immutable
// receipts, quote versions and original ownership facts. Ledger migration and
// organization ownership changes must commit in the caller's same transaction.
func (s *Service) TransferResponsibility(ctx context.Context, tx *gorm.DB, in TransferInput) error {
	if tx == nil {
		return ErrInvalidInput
	}
	return s.within(ctx, tx, func(t *gorm.DB) error {
		if err := lockCommand(t, "takeover", in.TakeoverID); err != nil {
			return err
		}
		var old struct{ Fingerprint string }
		q := t.Table("commerce_responsibility_transfers").Where("takeover_id=?", in.TakeoverID).Limit(1).Find(&old)
		if q.Error != nil {
			return q.Error
		}
		fp := fingerprint(in)
		if q.RowsAffected > 0 {
			if old.Fingerprint != fp {
				return ErrConflict
			}
			return nil
		}
		if err := s.PreflightTransferResponsibility(ctx, t, in); err != nil {
			return err
		}
		ids := make([]string, 0, len(in.Customers))
		for id := range in.Customers {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			if err := walletScope(t, in.FromOperatorID, id); err != nil {
				return err
			}
			if err := walletScope(t, in.ToOperatorID, id); err != nil {
				return err
			}
			if err := t.Model(&orderRow{}).Where("operator_id=? AND customer_id=?", in.FromOperatorID, id).Updates(map[string]any{"operator_id": in.ToOperatorID, "ownership_version": in.Customers[id] + 1}).Error; err != nil {
				return err
			}
			if err := t.Model(&subscriptionRow{}).Where("operator_id=? AND customer_id=?", in.FromOperatorID, id).Updates(map[string]any{"operator_id": in.ToOperatorID, "status": gorm.Expr("CASE WHEN status='cancelled' THEN status ELSE 'needs_confirmation' END"), "version": gorm.Expr("version+1"), "notice_quote_id": nil, "notice_cycle_due_at": nil, "notice_delivered_at": nil, "notice_evidence": ""}).Error; err != nil {
				return err
			}
			for _, table := range []string{"commerce_wallet_funding_lots", "commerce_wallet_funding_commands"} {
				if err := t.Table(table).Where("operator_id=? AND customer_id=?", in.FromOperatorID, id).Update("operator_id", in.ToOperatorID).Error; err != nil {
					return err
				}
			}
			if err := t.Model(&ManualGift{}).Where("operator_id=? AND customer_id=?", in.FromOperatorID, id).Updates(map[string]any{"operator_id": in.ToOperatorID, "ownership_version": in.Customers[id] + 1}).Error; err != nil {
				return err
			}
		}
		if err := t.Exec(`INSERT INTO commerce_responsibility_transfers(takeover_id,from_operator_id,to_operator_id,fingerprint,actor_id,evidence,created_at) VALUES(?,?,?,?,?,?,clock_timestamp())`, in.TakeoverID, in.FromOperatorID, in.ToOperatorID, fp, in.ActorID, in.Evidence).Error; err != nil {
			return err
		}
		return s.event(t, in.ToOperatorID, in.TakeoverID, "channel.commerce_responsibility_transferred", map[string]any{"from_operator_id": in.FromOperatorID, "customers": len(ids), "evidence": in.Evidence})
	})
}
