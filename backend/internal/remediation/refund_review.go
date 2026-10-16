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
		if out.RequiredReviews > 1 && out.RequestedBy == actor {
			return ErrReview
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
		var n int64
		if e = t.Table("remediation_reviews").Where("case_id=? AND decision='approve'", id).Count(&n).Error; e != nil {
			return e
		}
		if n < int64(out.RequiredReviews) {
			return s.event(t, op, id, "refund.reviewed", actor, evidence, map[string]any{"approvals": n, "required": out.RequiredReviews})
		}
		if out.Kind == "package_exception" {
			if e = s.approvePackage(ctx, t, out, actor); e != nil {
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
func (s *Service) approvePackage(ctx context.Context, t *gorm.DB, c Case, actor string) error {
	if c.GrantID == nil || c.GrantHoldID == nil || c.CustomerID == nil || c.OrderID == nil {
		return ErrConflict
	}
	at, e := instant(t)
	if e != nil {
		return e
	}
	event, e := s.Ledger.CancelReservedGrant(ctx, t, c.OperatorID, *c.CustomerID, *c.GrantID, *c.GrantHoldID, "refund:"+c.ID, actor, c.Evidence, at)
	if e != nil {
		return e
	}
	if event.PaidUSD.Cmp(c.AmountUSD) != 0 || event.Amount.Cmp(c.CreditATG) != 0 {
		return ErrConflict
	}
	// Unused value is returned to its original asset class. Commission funding
	// cannot be laundered into a cash refund or a refundable USD topup.
	if c.InternalUSD.Sign() > 0 {
		if _, e = s.Commerce.RestoreWalletFunding(ctx, t, c.OperatorID, *c.CustomerID, *c.OrderID, c.ID, c.InternalUSD); e != nil {
			return e
		}
	}
	if c.CommissionUSD.Sign() > 0 {
		if e = s.Commerce.RestoreCommissionFunding(ctx, t, c.OperatorID, *c.CustomerID, *c.OrderID, c.ID, c.CommissionUSD); e != nil {
			return e
		}
	}
	sales, e := s.account(ctx, t, c.OperatorID, c.OperatorID, "control:sales")
	if e != nil {
		return e
	}
	lines := []ledger.Line{{AccountID: sales.ID, Amount: c.AmountUSD.Neg()}}
	for _, r := range []struct {
		Purpose string
		Amount  ledger.Decimal
	}{{"customer:usd", c.InternalUSD}, {"commission:available", c.CommissionUSD}} {
		if r.Amount.Sign() > 0 {
			a, e := s.account(ctx, t, c.OperatorID, *c.CustomerID, r.Purpose)
			if e != nil {
				return e
			}
			lines = append(lines, ledger.Line{AccountID: a.ID, Amount: r.Amount})
		}
	}
	legs, e := s.Legs(ctx, t, c.OperatorID, c.ID)
	if e != nil {
		return e
	}
	for i := range legs {
		a, e := s.account(ctx, t, c.OperatorID, legs[i].ID, "refund:payable")
		if e != nil {
			return e
		}
		legs[i].SourceAccountID = &a.ID
		lines = append(lines, ledger.Line{AccountID: a.ID, Amount: legs[i].AmountUSD})
	}
	if _, e = s.Ledger.Post(ctx, t, ledger.Posting{OperatorID: c.OperatorID, BusinessKey: "exception-refund:" + c.ID, Kind: "package_exception_refund", SourceID: *c.OrderID, ActorID: actor, Reason: c.Evidence, Lines: lines}); e != nil {
		return e
	}
	for i := range legs {
		h, e := s.Ledger.Reserve(ctx, t, ledger.Reservation{OperatorID: c.OperatorID, BusinessKey: "refund:" + legs[i].ID, Purpose: "refund", ExpiresAt: holdExpiry(at), Amounts: []ledger.Line{{AccountID: *legs[i].SourceAccountID, Amount: legs[i].AmountUSD}}})
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
	if c.Kind == "package_exception" && c.State != "pending_review" {
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
	if c.Kind == "wallet" {
		if e = s.Commerce.ReleaseWalletRefund(ctx, t, c.OperatorID, *c.CustomerID, c.ID); e != nil {
			return e
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
	if c.Kind == "wallet" {
		if e := s.Commerce.CompleteWalletRefund(ctx, t, c.OperatorID, *c.CustomerID, c.ID); e != nil {
			return e
		}
	}
	at, e := instant(t)
	if e != nil {
		return e
	}
	c.State = "completed"
	c.CompletedAt = &at
	return t.Save(c).Error
}
