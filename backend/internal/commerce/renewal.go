package commerce

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (s *Service) EnableAutoRenewal(ctx context.Context, tx *gorm.DB, customerID, orderID, evidence string, allowCommission bool) (out Subscription, err error) {
	if !validID(customerID) || !validID(orderID) || strings.TrimSpace(evidence) == "" {
		return out, ErrInvalidInput
	}
	err = s.within(ctx, tx, func(tx *gorm.DB) error {
		o, err := s.GetOrder(ctx, tx, customerID, orderID)
		if err != nil {
			return err
		}
		if o.Status != "completed" || o.Kind == "topup" || o.QuoteID == nil || o.Period == ledger.Lifetime || o.SubscriptionID != nil {
			return ErrOrderState
		}
		var d deliveryRow
		if err = tx.First(&d, "order_id=?", orderID).Error; err != nil {
			return err
		}
		if d.ExpiresAt == nil {
			return ErrOrderState
		}
		quote, err := s.GetQuote(ctx, tx, *o.QuoteID)
		if err != nil {
			return err
		}
		if err = lockCommand(tx, "subscription-source", orderID); err != nil {
			return err
		}
		var existing subscriptionRow
		q := tx.Where("source_order_id=?", orderID).Limit(1).Find(&existing)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected > 0 {
			out = Subscription(existing)
			return nil
		}
		r := subscriptionRow{ID: uuid.NewString(), OperatorID: o.OperatorID, CustomerID: customerID, PackageID: quote.PackageID, SourceOrderID: orderID, AuthorizedQuoteID: quote.ID, AuthorizationEvidence: evidence, AllowCommission: allowCommission, Status: "active", NextDueAt: *d.ExpiresAt, Version: 1, CreatedAt: s.now()}
		r.OriginalOperatorID = o.OriginalOperatorID
		if o.OriginalOperatorID != "" && o.OriginalOperatorID != o.OperatorID {
			r.Status = "needs_confirmation"
		}
		if err = tx.Create(&r).Error; err != nil {
			return err
		}
		out = Subscription(r)
		return s.event(tx, o.OperatorID, r.ID, "renewal.authorized", out)
	})
	return
}
func (s *Service) GetSubscription(ctx context.Context, tx *gorm.DB, customerID, id string) (Subscription, error) {
	if !validID(customerID) || !validID(id) {
		return Subscription{}, ErrInvalidInput
	}
	var r subscriptionRow
	err := s.dbFor(ctx, tx).First(&r, "id=? AND customer_id=?", id, customerID).Error
	return Subscription(r), classify(err)
}
func (s *Service) CancelAutoRenewal(ctx context.Context, tx *gorm.DB, customerID, id string) error {
	return s.within(ctx, tx, func(tx *gorm.DB) error {
		var r subscriptionRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&r, "id=? AND customer_id=?", id, customerID).Error; err != nil {
			return err
		}
		if r.Status == "cancelled" {
			return nil
		}
		if err := tx.Model(&r).Updates(map[string]any{"status": "cancelled", "version": r.Version + 1}).Error; err != nil {
			return err
		}
		return s.event(tx, r.OperatorID, r.ID, "renewal.cancelled", map[string]any{"customer_id": customerID})
	})
}
func (s *Service) ConfirmRenewal(ctx context.Context, tx *gorm.DB, customerID, id, quoteID, evidence string, expectedVersion int64, allowCommission bool) (out Subscription, err error) {
	if evidence == "" || expectedVersion < 1 {
		return out, ErrInvalidInput
	}
	err = s.within(ctx, tx, func(tx *gorm.DB) error {
		var r subscriptionRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&r, "id=? AND customer_id=?", id, customerID).Error; err != nil {
			return err
		}
		if r.Version != expectedVersion {
			return ErrConflict
		}
		quote, err := s.GetQuote(ctx, tx, quoteID)
		if err != nil {
			return err
		}
		if quote.OperatorID != r.OperatorID {
			return ErrScope
		}
		if quote.PackageID != r.PackageID {
			authorized, err := s.GetQuote(ctx, tx, r.AuthorizedQuoteID)
			if err != nil {
				return err
			}
			if authorized.OperatorID == r.OperatorID {
				return ErrScope
			}
		}
		p, err := s.GetPackage(ctx, tx, r.OperatorID, quote.PackageID)
		if err != nil {
			return err
		}
		if p.CurrentQuoteID == nil || *p.CurrentQuoteID != quoteID {
			return ErrConflict
		}
		if !p.RenewalEnabled {
			return organization.ErrBusinessClosed
		}
		if quote.Period == ledger.Lifetime {
			return ErrOrderState
		}
		r.AuthorizedQuoteID = quoteID
		r.PackageID = quote.PackageID
		r.AuthorizationEvidence = evidence
		r.AllowCommission = allowCommission
		r.Status = "active"
		r.Version++
		if err = tx.Save(&r).Error; err != nil {
			return err
		}
		out = Subscription(r)
		return s.event(tx, r.OperatorID, r.ID, "renewal.reconfirmed", out)
	})
	return
}

// RecordRenewalNotice records a durable customer-facing notification for the
// exact cycle and quote. Consent to adverse changes is checked separately.
func (s *Service) RecordRenewalNotice(ctx context.Context, tx *gorm.DB, id, quoteID string, cycle time.Time, evidence string) error {
	if !validID(id) || !validID(quoteID) || cycle.IsZero() || evidence == "" {
		return ErrInvalidInput
	}
	return s.within(ctx, tx, func(tx *gorm.DB) error {
		var r subscriptionRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&r, "id=?", id).Error; err != nil {
			return err
		}
		if r.Status == "cancelled" || !r.NextDueAt.Equal(cycle) {
			return ErrConflict
		}
		q, err := s.GetQuote(ctx, tx, quoteID)
		if err != nil {
			return err
		}
		if q.PackageID != r.PackageID || q.OperatorID != r.OperatorID {
			return ErrScope
		}
		return tx.Model(&r).Updates(map[string]any{"notice_quote_id": quoteID, "notice_cycle_due_at": cycle.UTC(), "notice_delivered_at": s.now(), "notice_evidence": evidence, "version": r.Version + 1}).Error
	})
}

func (s *Service) RenewSubscription(ctx context.Context, tx *gorm.DB, id string) (out RenewalResult, err error) {
	if !validID(id) {
		return out, ErrInvalidInput
	}
	err = s.within(ctx, tx, func(tx *gorm.DB) error {
		if err := lockCommand(tx, "renewal", id); err != nil {
			return err
		}
		var observed subscriptionRow
		if err := tx.First(&observed, "id=?", id).Error; err != nil {
			return err
		}
		own, err := s.org.CurrentOwnership(ctx, tx, observed.CustomerID)
		if err != nil {
			return err
		}
		if own.OperatorID != observed.OperatorID {
			return organization.ErrStaleOwnership
		}
		if _, err = s.org.LockOwnership(ctx, tx, own.CustomerID, own.OperatorID, own.Version); err != nil {
			return err
		}
		var r subscriptionRow
		if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&r, "id=?", id).Error; err != nil {
			return err
		}
		if r.Status == "cancelled" {
			out.Status = "cancelled"
			return nil
		}
		if r.Status == "needs_confirmation" {
			out.Status = "needs_confirmation"
			return nil
		}
		if s.now().Before(r.NextDueAt) {
			return ErrRenewalNotDue
		}
		if err = s.org.RequireBusiness(ctx, tx, r.OperatorID, organization.Renewal); err != nil {
			return err
		}
		p, err := s.GetPackage(ctx, tx, r.OperatorID, r.PackageID)
		if err != nil {
			return err
		}
		if !p.RenewalEnabled || p.CurrentQuoteID == nil {
			return organization.ErrBusinessClosed
		}
		current, err := s.GetQuote(ctx, tx, *p.CurrentQuoteID)
		if err != nil {
			return err
		}
		authorized, err := s.GetQuote(ctx, tx, r.AuthorizedQuoteID)
		if err != nil {
			return err
		}
		if RequiresRenewalConfirmation(authorized, current) {
			if r.Status == "needs_confirmation" {
				out.Status = "needs_confirmation"
				return nil
			}
			if err = tx.Model(&r).Updates(map[string]any{"status": "needs_confirmation", "version": r.Version + 1}).Error; err != nil {
				return err
			}
			out.Status = "needs_confirmation"
			return s.event(tx, r.OperatorID, r.ID, "renewal.confirmation_required", map[string]any{"quote_id": current.ID, "cycle_due_at": r.NextDueAt})
		}
		if r.NoticeQuoteID == nil || *r.NoticeQuoteID != current.ID || r.NoticeCycleDueAt == nil || !r.NoticeCycleDueAt.Equal(r.NextDueAt) || r.NoticeDeliveredAt == nil || r.NoticeEvidence == "" {
			out.Status = "notification_required"
			return nil
		}
		usd, err := s.account(ctx, tx, r.OperatorID, r.CustomerID, "customer:money")
		if err != nil {
			return err
		}
		fromMoney := minimum(usd.Available(), current.PriceMoney)
		remainder := current.PriceMoney.Sub(fromMoney)
		fromCommission := ledger.Zero()
		if remainder.Sign() > 0 && r.AllowCommission {
			commission, err := s.account(ctx, tx, r.OperatorID, r.CustomerID, "commission:available")
			if err != nil {
				return err
			}
			fromCommission = minimum(commission.Available(), remainder)
			remainder = remainder.Sub(fromCommission)
		}
		if remainder.Sign() > 0 {
			if r.Status == "needs_funds" {
				out.Status = "needs_funds"
				return nil
			}
			if err = tx.Model(&r).Updates(map[string]any{"status": "needs_funds", "version": r.Version + 1}).Error; err != nil {
				return err
			}
			out.Status = "needs_funds"
			return s.event(tx, r.OperatorID, r.ID, "renewal.insufficient_funds", map[string]any{"cycle_due_at": r.NextDueAt, "missing_money": remainder})
		}
		cycle := r.NextDueAt.UTC()
		order, err := s.CreateOrder(ctx, tx, CreateOrderInput{CustomerID: r.CustomerID, OperatorID: r.OperatorID, QuoteID: current.ID, BusinessKey: "renew:" + r.ID + ":" + cycle.Format(time.RFC3339Nano), ExpectedOwnershipVersion: own.Version, InternalMoney: fromMoney, CommissionMoney: fromCommission, Validity: UnpaidPaymentWindow, SubscriptionID: &r.ID, CycleDueAt: &cycle, renewalAuthorized: true})
		if err != nil {
			return err
		}
		delivery, err := s.Fulfill(ctx, tx, order.ID, "system:authorized-renewal")
		if err != nil {
			return err
		}
		status := "active"
		nextDue := r.NextDueAt
		if delivery.ExpiresAt == nil {
			status = "cancelled"
		} else {
			nextDue = *delivery.ExpiresAt
		}
		if err = tx.Model(&r).Updates(map[string]any{"status": status, "next_due_at": nextDue, "authorized_quote_id": current.ID, "notice_quote_id": nil, "notice_cycle_due_at": nil, "notice_delivered_at": nil, "notice_evidence": "", "version": r.Version + 1}).Error; err != nil {
			return err
		}
		updated, err := s.GetOrder(ctx, tx, r.CustomerID, order.ID)
		if err != nil {
			return err
		}
		out = RenewalResult{Status: "completed", Order: &updated, Delivery: &delivery}
		return nil
	})
	return
}
func minimum(a, b ledger.Decimal) ledger.Decimal {
	if a.Cmp(b) < 0 {
		return a
	}
	return b
}
