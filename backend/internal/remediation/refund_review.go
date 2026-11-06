package remediation

import (
	"context"
	"errors"

	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"gorm.io/gorm"
)

type review struct {
	CaseID     string
	ReviewerID string
	Decision   string
	Evidence   string
}

func (s *Service) ReviewRefund(ctx context.Context, actor, op, id, decision, evidence string) (Case, error) {
	var out Case
	if decision != "approve" && decision != "reject" || evidence == "" {
		return out, ErrInvalid
	}
	e := s.within(ctx, nil, func(t *gorm.DB) error {
		if e := s.Organization.RequirePermission(ctx, t, actor, op, "refunds.manage"); e != nil {
			return e
		}
		var e error
		out, e = s.lockCase(ctx, t, op, id)
		if e != nil {
			return e
		}
		var old review
		e = t.Table("remediation_reviews").Where("case_id=? AND reviewer_id=?", id, actor).Take(&old).Error
		if e == nil {
			if old.Decision != decision || old.Evidence != evidence {
				return ErrConflict
			}
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		if out.State != "pending_review" {
			return ErrState
		}
		at, e := instant(t)
		if e != nil {
			return e
		}
		if e = t.Exec("INSERT INTO remediation_reviews(case_id,reviewer_id,decision,evidence,created_at) VALUES(?,?,?,?,?)", id, actor, decision, evidence, at).Error; e != nil {
			return e
		}
		if decision == "reject" {
			return s.cancel(ctx, t, &out, actor, evidence, "rejected")
		}
		if out.Kind == "package_exception" {
			if e = s.approveUndeliveredPackage(ctx, t, out, actor); e != nil {
				return e
			}
		}
		if out.Kind == "procurement_nondelivery" {
			if e = s.approveUndeliveredPurchase(ctx, t, out, actor); e != nil {
				return e
			}
		}
		out.State = "approved"
		if e = t.Save(&out).Error; e != nil {
			return e
		}
		if e = t.Model(&RefundLeg{}).Where("case_id=? AND state='reserved'", id).Update("state", "approved").Error; e != nil {
			return e
		}
		if e = s.completeCase(ctx, t, &out); e != nil {
			return e
		}
		return s.event(t, op, id, "refund.approved", actor, evidence, out)
	})
	return out, e
}
func (s *Service) approveUndeliveredPackage(ctx context.Context, t *gorm.DB, c Case, actor string) error {
	if c.GrantID != nil || c.CustomerID == nil || c.OrderID == nil {
		return ErrState
	}
	o, e := s.Commerce.ReleaseUndeliveredPackage(ctx, t, *c.CustomerID, *c.OrderID, actor)
	if e != nil {
		return e
	}
	external := o.ExternalAllocatedMoney
	if external.Sign() == 0 {
		return nil
	}
	at, e := instant(t)
	if e != nil {
		return e
	}
	funds, e := s.account(ctx, t, c.OperatorID, o.ID, "order:funds")
	if e != nil {
		return e
	}
	legs, e := s.Legs(ctx, t, c.OperatorID, c.ID)
	if e != nil {
		return e
	}
	lines := []ledger.Line{{AccountID: funds.ID, Amount: external.Neg()}}
	sum := ledger.Zero()
	for i := range legs {
		payable, e := s.account(ctx, t, c.OperatorID, legs[i].ID, "refund:payable")
		if e != nil {
			return e
		}
		legs[i].SourceAccountID = &payable.ID
		lines = append(lines, ledger.Line{AccountID: payable.ID, Amount: legs[i].AmountMoney})
		sum = sum.Add(legs[i].AmountMoney)
	}
	if sum.Cmp(external) != 0 {
		return ErrConflict
	}
	if _, e = s.Ledger.Post(ctx, t, ledger.Posting{OperatorID: c.OperatorID, BusinessKey: "undelivered-package:" + c.ID, Kind: "package_nondelivery_refund", SourceID: o.ID, ActorID: actor, Reason: c.Evidence, Lines: lines}); e != nil {
		return e
	}
	for i := range legs {
		h, e := s.Ledger.Reserve(ctx, t, ledger.Reservation{OperatorID: c.OperatorID, BusinessKey: "refund:" + legs[i].ID, Purpose: "refund", ExpiresAt: holdExpiry(at), Amounts: []ledger.Line{{AccountID: *legs[i].SourceAccountID, Amount: legs[i].AmountMoney}}})
		if e != nil {
			return e
		}
		legs[i].HoldID = &h.ID
		if e = t.Save(&legs[i]).Error; e != nil {
			return e
		}
	}
	return nil
}
func (s *Service) approveUndeliveredPurchase(ctx context.Context, t *gorm.DB, c Case, actor string) error {
	if c.OrderID == nil {
		return ErrState
	}
	o, allocation, e := s.Cooperation.MarkUndeliveredPurchaseRefund(ctx, t, c.OperatorID, *c.OrderID)
	if e != nil {
		return e
	}
	if o.DueMoney.Cmp(c.AmountMoney) != 0 || allocation.AmountMoney.Cmp(c.AmountMoney) != 0 {
		return ErrConflict
	}
	at, e := instant(t)
	if e != nil {
		return e
	}
	funds, e := s.account(ctx, t, c.OperatorID, o.ID, "order:funds")
	if e != nil {
		return e
	}
	legs, e := s.Legs(ctx, t, c.OperatorID, c.ID)
	if e != nil {
		return e
	}
	if len(legs) != 1 || legs[0].ReceiptID != allocation.ReceiptID || legs[0].AmountMoney.Cmp(o.DueMoney) != 0 {
		return ErrConflict
	}
	payable, e := s.account(ctx, t, c.OperatorID, legs[0].ID, "refund:payable")
	if e != nil {
		return e
	}
	if _, e = s.Ledger.Post(ctx, t, ledger.Posting{OperatorID: c.OperatorID, BusinessKey: "undelivered-purchase:" + c.ID, Kind: "procurement_nondelivery_refund", SourceID: o.ID, ActorID: actor, Reason: c.Evidence, Lines: []ledger.Line{{AccountID: funds.ID, Amount: o.DueMoney.Neg()}, {AccountID: payable.ID, Amount: o.DueMoney}}}); e != nil {
		return e
	}
	h, e := s.Ledger.Reserve(ctx, t, ledger.Reservation{OperatorID: c.OperatorID, BusinessKey: "refund:" + legs[0].ID, Purpose: "refund", ExpiresAt: holdExpiry(at), Amounts: []ledger.Line{{AccountID: payable.ID, Amount: legs[0].AmountMoney}}})
	if e != nil {
		return e
	}
	legs[0].SourceAccountID = &payable.ID
	legs[0].HoldID = &h.ID
	if e = t.Save(&legs[0]).Error; e != nil {
		return e
	}
	return s.Cooperation.CloseUndeliveredPurchase(ctx, t, c.OperatorID, o.ID)
}
func (s *Service) CancelRefund(ctx context.Context, actor, op, id, evidence string) (Case, error) {
	var out Case
	e := s.within(ctx, nil, func(t *gorm.DB) error {
		if e := s.Organization.RequirePermission(ctx, t, actor, op, "refunds.manage"); e != nil {
			return e
		}
		var e error
		out, e = s.lockCase(ctx, t, op, id)
		if e != nil {
			return e
		}
		return s.cancel(ctx, t, &out, actor, evidence, "cancelled")
	})
	return out, e
}
func (s *Service) CustomerCancelRefund(ctx context.Context, customer, op, id string) (Case, error) {
	var out Case
	e := s.within(ctx, nil, func(t *gorm.DB) error {
		var e error
		out, e = s.lockCase(ctx, t, op, id)
		if e != nil {
			return e
		}
		if out.CustomerID == nil || *out.CustomerID != customer || out.Kind != "wallet" {
			return ErrScope
		}
		return s.cancel(ctx, t, &out, customer, "customer cancelled unused balance return", "cancelled")
	})
	return out, e
}
func (s *Service) cancel(ctx context.Context, t *gorm.DB, c *Case, actor, evidence, state string) error {
	if evidence == "" {
		return ErrInvalid
	}
	if c.State == state {
		return nil
	}
	if c.State != "pending_review" && c.State != "approved" {
		return ErrState
	}
	if (c.Kind == "package_exception" || c.Kind == "procurement_nondelivery") && c.State != "pending_review" {
		return ErrState
	}
	legs, e := s.Legs(ctx, t, c.OperatorID, c.ID)
	if e != nil {
		return e
	}
	for _, leg := range legs {
		if leg.State != "reserved" && leg.State != "approved" {
			return ErrState
		}
	}
	for _, leg := range legs {
		if e = s.ReleaseReceiptRefund(ctx, t, leg.SourceOperatorID, leg.ReceiptID, leg.ID); e != nil {
			return e
		}
		if leg.HoldID != nil {
			if _, e = s.Ledger.Release(ctx, t, c.OperatorID, *leg.HoldID, "refund-cancel:"+leg.ID); e != nil {
				return e
			}
		}
		if e = t.Model(&RefundLeg{}).Where("id=?", leg.ID).Update("state", "cancelled").Error; e != nil {
			return e
		}
	}
	if c.GrantHoldID != nil {
		if _, e = s.Ledger.Release(ctx, t, c.OperatorID, *c.GrantHoldID, "refund-cancel:"+c.ID); e != nil {
			return e
		}
		at, e := instant(t)
		if e != nil {
			return e
		}
		events, e := s.Ledger.ExpireCredits(ctx, t, c.OperatorID, *c.CustomerID, at)
		if e != nil {
			return e
		}
		for _, event := range events {
			g, e := s.Ledger.GetGrant(ctx, t, event.GrantID)
			if e != nil {
				return e
			}
			if e = s.Incentives.ObserveGrantEvent(ctx, t, g, event); e != nil {
				return e
			}
		}
	}
	if c.Kind == "package_exception" && c.OrderID != nil && c.CustomerID != nil && c.State == "pending_review" {
		if e = s.Commerce.ClearUndeliveredRefund(ctx, t, *c.CustomerID, *c.OrderID); e != nil {
			return e
		}
	}
	if c.Kind == "procurement_nondelivery" && c.OrderID != nil && c.State == "pending_review" {
		if e = s.Cooperation.ClearUndeliveredPurchaseRefund(ctx, t, c.OperatorID, *c.OrderID); e != nil {
			return e
		}
	}
	c.State = state
	if e = t.Save(c).Error; e != nil {
		return e
	}
	return s.event(t, c.OperatorID, c.ID, "refund."+state, actor, evidence, c)
}
func (s *Service) completeCase(ctx context.Context, t *gorm.DB, c *Case) error {
	var count int64
	if e := t.Model(&RefundLeg{}).Where("case_id=? AND state NOT IN ('completed','cancelled')", c.ID).Count(&count).Error; e != nil {
		return e
	}
	if count > 0 {
		return nil
	}

	at, e := instant(t)
	if e != nil {
		return e
	}
	c.State = "completed"
	c.CompletedAt = &at
	return t.Save(c).Error
}
