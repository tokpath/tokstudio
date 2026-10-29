package remediation

import (
	"context"
	"fmt"

	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/operations"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"gorm.io/gorm"
)

// InvoiceReceiptAdjustments supplies one explicit answer for each requested
// original receipt; absent adjustments are confirmed zero, not missing data.
func (s *Service) InvoiceReceiptAdjustments(ctx context.Context, tx *gorm.DB, operator string, receiptIDs []string) ([]operations.ReceiptAdjustment, error) {
	out := []operations.ReceiptAdjustment{}
	if len(receiptIDs) > 1000 {
		return nil, ErrInvalid
	}
	e := s.within(ctx, tx, func(t *gorm.DB) error {
		for _, id := range receiptIDs {
			if _, e := s.Commerce.GetReceipt(ctx, t, operator, id); e != nil {
				return e
			}
			var amounts struct {
				Paid   ledger.Decimal
				Review bool
				Count  int64
			}
			if e := t.Model(&capacity{}).Select("COALESCE(sum(completed_money),0)::text AS paid, COALESCE(bool_or(state='reserved'),false) AS review,count(*) AS count").Where("operator_id=? AND receipt_id=?", operator, id).Scan(&amounts).Error; e != nil {
				return e
			}
			var disputes struct {
				Removed ledger.Decimal
				Review  bool
				Count   int64
			}
			if e := t.Table("remediation_chargebacks").Select("COALESCE(sum(CASE WHEN state='invalid_payment' THEN amount_money ELSE 0 END),0)::text AS removed,COALESCE(bool_or(state='disputed'),false) AS review,count(*) AS count").Where("operator_id=? AND receipt_id=?", operator, id).Scan(&disputes).Error; e != nil {
				return e
			}
			out = append(out, operations.ReceiptAdjustment{ReceiptID: id, RemovedMoney: amounts.Paid.Add(disputes.Removed), ReviewRequired: amounts.Review || disputes.Review, Reference: fmt.Sprintf("receipt:%s/refund-legs:%d/chargebacks:%d", id, amounts.Count, disputes.Count)})
		}
		return nil
	})
	return out, e
}

// InvoicePartyReceiptAdjustments is a trusted invoice projection, distinct
// from the whole-receipt cap above. Only this customer's actually completed
// external refund legs reduce its basis; another customer's refund is excluded.
// Callers use both caps independently and never add the two adjustments.
func (s *Service) InvoicePartyReceiptAdjustments(ctx context.Context, tx *gorm.DB, operator, customer string, receiptIDs []string) ([]operations.ReceiptAdjustment, error) {
	if !validID(operator) || !validID(customer) || len(receiptIDs) > 1000 {
		return nil, ErrInvalid
	}
	rows := make([]operations.ReceiptAdjustment, 0, len(receiptIDs))
	e := s.within(ctx, tx, func(t *gorm.DB) error {
		for _, receipt := range receiptIDs {
			if _, e := s.Commerce.GetReceipt(ctx, t, operator, receipt); e != nil {
				return e
			}
			var party struct {
				Removed ledger.Decimal
				Review  bool
			}
			if e := t.Table("remediation_refund_legs l").Select("COALESCE(sum(l.settled_money),0)::text AS removed,COALESCE(bool_or(l.state NOT IN ('completed','cancelled')),false) AS review").Joins("JOIN remediation_cases c ON c.id=l.case_id").Where("l.source_operator_id=? AND l.receipt_id=? AND c.customer_id=?", operator, receipt, customer).Scan(&party).Error; e != nil {
				return e
			}
			var disputed bool
			if e := t.Table("remediation_chargebacks").Select("COALESCE(bool_or(state IN ('disputed','invalid_payment')),false)").Where("operator_id=? AND receipt_id=?", operator, receipt).Scan(&disputed).Error; e != nil {
				return e
			}
			rows = append(rows, operations.ReceiptAdjustment{ReceiptID: receipt, RemovedMoney: party.Removed, ReviewRequired: party.Review || disputed, Reference: fmt.Sprintf("receipt:%s/customer:%s/external_refund:%s", receipt, customer, party.Removed.String())})
		}
		return nil
	})
	return rows, e
}

func (s *Service) CustomerCases(ctx context.Context, customer, brand string) ([]Case, error) {
	c, e := s.Organization.GetCustomer(ctx, nil, customer)
	if e != nil {
		return nil, e
	}
	if c.BrandID != brand {
		return nil, ErrScope
	}
	var out []Case
	e = s.DB.WithContext(ctx).Where("customer_id=?", customer).Order("created_at DESC,id").Limit(200).Find(&out).Error
	return out, e
}
func (s *Service) CustomerCancelRefundByBrand(ctx context.Context, customer, brand, id string) (Case, error) {
	c, e := s.Organization.GetCustomer(ctx, nil, customer)
	if e != nil {
		return Case{}, e
	}
	if c.BrandID != brand {
		return Case{}, ErrScope
	}
	var r Case
	if e = s.DB.WithContext(ctx).First(&r, "id=? AND customer_id=?", id, customer).Error; e != nil {
		return r, e
	}
	return s.CustomerCancelRefund(ctx, customer, r.OperatorID, id)
}

func (s *Service) CustomerClosureRequirements(ctx context.Context, tx *gorm.DB, customer, brand string) ([]organization.Requirement, error) {
	var out []organization.Requirement
	e := s.within(ctx, tx, func(t *gorm.DB) error {
		c, e := s.Organization.GetCustomer(ctx, t, customer)
		if e != nil {
			return e
		}
		if c.BrandID != brand {
			return ErrScope
		}
		var payouts, disputes int64
		if e = t.Model(&Case{}).Where("customer_id=? AND state NOT IN ('completed','cancelled','rejected')", customer).Count(&payouts).Error; e != nil {
			return e
		}
		if e = t.Raw(`SELECT count(*) FROM remediation_chargebacks cb WHERE cb.state='disputed' AND (EXISTS(SELECT 1 FROM commerce_receipt_allocations a JOIN commerce_orders o ON o.id=a.order_id WHERE a.receipt_id=cb.receipt_id AND o.customer_id=?) OR EXISTS(SELECT 1 FROM commerce_wallet_funding_lots l WHERE l.receipt_id=cb.receipt_id AND l.customer_id=?))`, customer, customer).Row().Scan(&disputes); e != nil {
			return e
		}
		out = []organization.Requirement{{Code: "payouts", Description: "refunds must be paid or explicitly cancelled before closure", Ready: payouts == 0}, {Code: "disputes", Description: "original payment disputes must be resolved before closure", Ready: disputes == 0}}
		return nil
	})
	return out, e
}
