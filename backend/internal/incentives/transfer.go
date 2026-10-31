package incentives

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/organization"
	"gorm.io/gorm"
)

type TransferInput = organization.ResponsibilityTransferInput

var ErrTransferBlocked = errors.New("incentives: takeover requires outstanding business resolution")

func (s *Service) PreflightTransferResponsibility(ctx context.Context, tx *gorm.DB, in TransferInput) error {
	if err := s.org.LockResponsibilityTransfer(ctx, tx, in); err != nil {
		return err
	}
	var p promiseRow
	q := tx.Where("operator_id=? AND status='reserved'", in.FromOperatorID).Limit(1).Find(&p)
	if q.Error != nil {
		return q.Error
	}
	if q.RowsAffected > 0 {
		return fmt.Errorf("%w: uncompleted registration promise %s", ErrTransferBlocked, p.IntentID)
	}
	ids := make([]string, 0, len(in.Customers))
	for id := range in.Customers {
		ids = append(ids, id)
	}
	var count int64
	for _, spec := range []struct{ table, column string }{{"incentives_qualification_accounts", "customer_id"}, {"incentives_function_states", "customer_id"}, {"incentives_order_snapshots", "customer_id"}, {"incentives_confirmation_events", "customer_id"}, {"incentives_commission_entries", "beneficiary_id"}, {"incentives_offsets", "customer_id"}, {"incentives_withdrawals", "customer_id"}, {"incentives_commission_funding_lots", "customer_id"}, {"incentives_commission_funding_commands", "customer_id"}} {
		query := tx.Table(spec.table).Where("operator_id=?", in.FromOperatorID)
		if len(ids) > 0 {
			query = query.Where(spec.column+" NOT IN ?", ids)
		}
		if err := query.Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return fmt.Errorf("%w: %s contains customers outside complete transfer", ErrTransferBlocked, spec.table)
		}
	}
	if err := tx.Table("incentives_qualification_accounts").Where("operator_id=? AND customer_id IN ?", in.ToOperatorID, ids).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return fmt.Errorf("%w: target already contains qualification projection", ErrTransferBlocked)
	}
	return nil
}

func (s *Service) TransferResponsibility(ctx context.Context, tx *gorm.DB, in TransferInput) error {
	if tx == nil {
		return ErrInvalidInput
	}
	return s.within(ctx, tx, func(t *gorm.DB) error {
		operators := []string{in.FromOperatorID, in.ToOperatorID}
		sort.Strings(operators)
		for _, id := range operators {
			if err := s.LockBusiness(ctx, t, id); err != nil {
				return err
			}
		}
		var old struct{ Fingerprint string }
		q := t.Table("incentives_responsibility_transfers").Where("takeover_id=?", in.TakeoverID).Limit(1).Find(&old)
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
		var withdrawals []withdrawalRow
		if err := t.Where("operator_id=? AND status IN ('requested','approved')", in.FromOperatorID).Find(&withdrawals).Error; err != nil {
			return err
		}
		for _, w := range withdrawals {
			newHold, ok := in.HoldIDs[w.HoldID]
			if !ok || !validID(newHold) {
				return fmt.Errorf("%w: missing mapped withdrawal hold %s", ErrTransferBlocked, w.HoldID)
			}
		}
		// Every customer in the channel is transferred together, so fixed two-hop
		// snapshots and precision carries retain their identities and rates.
		for _, table := range []string{"incentives_qualification_accounts", "incentives_function_states", "incentives_campaigns", "incentives_order_snapshots", "incentives_confirmation_events", "incentives_commission_entries", "incentives_commission_carries", "incentives_corrections", "incentives_offsets", "incentives_withdrawals", "incentives_registration_promises", "incentives_kol_customer_counts", "incentives_combinations", "incentives_commission_funding_lots", "incentives_commission_funding_commands", "incentives_commission_source_flags", "incentives_commission_order_flags"} {
			if err := t.Table(table).Where("operator_id=?", in.FromOperatorID).Update("operator_id", in.ToOperatorID).Error; err != nil {
				return err
			}
		}
		for _, w := range withdrawals {
			if err := t.Model(&withdrawalRow{}).Where("id=?", w.ID).Updates(map[string]any{"hold_id": in.HoldIDs[w.HoldID], "status": "requested", "approved_by": nil}).Error; err != nil {
				return err
			}
		}
		if err := t.Exec(`INSERT INTO incentives_responsibility_transfers(takeover_id,from_operator_id,to_operator_id,fingerprint,actor_id,evidence,created_at) VALUES(?,?,?,?,?,?,clock_timestamp())`, in.TakeoverID, in.FromOperatorID, in.ToOperatorID, fp, in.ActorID, in.Evidence).Error; err != nil {
			return err
		}
		for customer, version := range in.Customers {
			if err := t.Exec(`INSERT INTO incentives_responsibility_transfer_customers(takeover_id,customer_id,old_ownership_version) VALUES(?,?,?)`, in.TakeoverID, customer, version).Error; err != nil {
				return err
			}
		}
		return s.event(t, in.ToOperatorID, in.TakeoverID, "channel.incentives_responsibility_transferred", map[string]any{"from_operator_id": in.FromOperatorID, "customers": len(in.Customers), "evidence": in.Evidence})
	})
}

func (s *Service) policyForCustomerAt(tx *gorm.DB, operator, customer string, at time.Time) (Policy, error) {
	var moved struct{ FromOperatorID string }
	q := tx.Table("incentives_responsibility_transfers t").Select("t.from_operator_id").Joins("JOIN incentives_responsibility_transfer_customers c ON c.takeover_id=t.takeover_id").Where("t.to_operator_id=? AND c.customer_id=? AND t.created_at>?", operator, customer, at).Order("t.created_at DESC").Limit(1).Find(&moved)
	if q.Error != nil {
		return Policy{}, q.Error
	}
	if q.RowsAffected > 0 {
		return s.policyAt(tx, moved.FromOperatorID, at)
	}
	return s.policyAt(tx, operator, at)
}
