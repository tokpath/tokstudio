package commerce

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Fulfill is an internal trusted command. Customer-facing callers must first
// select the order with GetOrder(customerID, orderID). Funds, ATG, campaign and
// capacity resolution and the one delivery record all commit atomically.
func (s *Service) Fulfill(ctx context.Context, tx *gorm.DB, orderID, actorID string) (out Delivery, err error) {
	if !validID(orderID) || strings.TrimSpace(actorID) == "" {
		return out, ErrInvalidInput
	}
	err = s.within(ctx, tx, func(tx *gorm.DB) error {
		if err := lockCommand(tx, "order", orderID); err != nil {
			return err
		}
		var observed orderRow
		if err := tx.First(&observed, "id=?", orderID).Error; err != nil {
			return err
		}
		if observed.Status == "completed" {
			var d deliveryRow
			if err := tx.First(&d, "order_id=?", orderID).Error; err != nil {
				return err
			}
			out = Delivery(d)
			return nil
		}
		if _, err := s.org.LockOwnership(ctx, tx, observed.CustomerID, observed.OperatorID, observed.OwnershipVersion); err != nil {
			return err
		}
		if err := s.lockBusinessScope(ctx, tx, observed.OperatorID); err != nil {
			return err
		}
		var row orderRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, "id=?", orderID).Error; err != nil {
			return err
		}
		o := Order(row)
		before := o
		if o.Status == "completed" {
			var d deliveryRow
			if err := tx.First(&d, "order_id=?", orderID).Error; err != nil {
				return err
			}
			out = Delivery(d)
			return nil
		}
		if o.ExternalAllocatedMoney.Cmp(externalDue(o)) != 0 {
			return ErrNotPaid
		}
		if o.Status != "paid" && o.Status != "exception" {
			return ErrOrderState
		}
		if o.CoverageStatus == "released" {
			if s.coverage == nil {
				return ErrNotReady
			}
			c, err := s.coverage.Restore(ctx, tx, o)
			if err != nil {
				return err
			}
			if c.Reference == "" || c.Version < 1 {
				return ErrNotReady
			}
			o.CoverageReference = c.Reference
			o.CoverageVersion = c.Version
			o.CoverageStatus = "reserved"
		}
		if internalTotal(o).Sign() > 0 && o.InternalStatus == "released" {
			hold, err := s.reserveInternal(ctx, tx, o, true)
			if err != nil {
				return err
			}
			o.HoldID = &hold.ID
			o.InternalStatus = "held"
		}
		if internalTotal(o).Sign() > 0 && (o.InternalStatus != "held" || o.HoldID == nil) {
			return ErrNotPaid
		}
		now := s.now()
		if s.databaseFulfillmentClock {
			if err := tx.Raw(`SELECT clock_timestamp()`).Scan(&now).Error; err != nil {
				return err
			}
			now = now.UTC()
		}
		end, err := ledger.PeriodEnd(now, o.Period, o.Timezone)
		if err != nil {
			return err
		}
		d := deliveryRow{ID: uuid.NewString(), OrderID: o.ID, EffectiveAt: now, ExpiresAt: end, DeliveredAt: now}
		if o.Kind == "topup" {
			if err = s.createWalletLots(tx, o); err != nil {
				return err
			}
			from, err := s.account(ctx, tx, o.OperatorID, o.ID, "order:funds")
			if err != nil {
				return err
			}
			to, err := s.account(ctx, tx, o.OperatorID, o.CustomerID, "customer:money")
			if err != nil {
				return err
			}
			if _, err = s.ledger.Post(ctx, tx, ledger.Posting{OperatorID: o.OperatorID, BusinessKey: "topup:" + o.ID, Kind: "wallet_topup", SourceID: o.ID, ActorID: actorID, Reason: "verified fully paid wallet topup", Lines: []ledger.Line{{AccountID: from.ID, Amount: o.DueMoney.Neg()}, {AccountID: to.ID, Amount: o.DueMoney}}}); err != nil {
				return err
			}
		} else {
			if s.coverage == nil || o.CoverageStatus != "reserved" {
				return ErrNotReady
			}
			if err = s.resolveOrderWalletFunding(tx, o, true); err != nil {
				return err
			}
			sales, err := s.account(ctx, tx, o.OperatorID, o.OperatorID, "control:sales")
			if err != nil {
				return err
			}
			if internalTotal(o).Sign() > 0 {
				if o.CommissionMoney.Sign() > 0 {
					if s.commissionFunding == nil {
						return ErrNotReady
					}
					if err := s.commissionFunding.RequireCommissionFundingExecution(ctx, tx, o.OperatorID, o.CustomerID, "order", o.ID); err != nil {
						return err
					}
				}
				lines := []ledger.Line{{AccountID: sales.ID, Amount: internalTotal(o)}}
				for _, fund := range []struct {
					purpose string
					amount  ledger.Decimal
				}{{"customer:money", o.InternalMoney}, {"commission:available", o.CommissionMoney}} {
					if fund.amount.Sign() > 0 {
						a, err := s.account(ctx, tx, o.OperatorID, o.CustomerID, fund.purpose)
						if err != nil {
							return err
						}
						lines = append(lines, ledger.Line{AccountID: a.ID, Amount: fund.amount.Neg()})
					}
				}
				if _, err = s.ledger.Capture(ctx, tx, *o.HoldID, ledger.Posting{OperatorID: o.OperatorID, BusinessKey: "order-internal:" + o.ID, Kind: "package_payment", SourceID: o.ID, ActorID: actorID, Reason: "capture selected customer USD and available commission", Lines: lines}, nil); err != nil {
					return err
				}
				o.InternalStatus = "captured"
				if o.CommissionMoney.Sign() > 0 {
					if err := s.commissionFunding.CompleteCommissionFunding(ctx, tx, o.OperatorID, o.CustomerID, o.ID); err != nil {
						return err
					}
				}
			}
			if o.ExternalAllocatedMoney.Sign() > 0 {
				funds, err := s.account(ctx, tx, o.OperatorID, o.ID, "order:funds")
				if err != nil {
					return err
				}
				if _, err = s.ledger.Post(ctx, tx, ledger.Posting{OperatorID: o.OperatorID, BusinessKey: "order-external:" + o.ID, Kind: "package_payment", SourceID: o.ID, ActorID: actorID, Reason: "consume verified allocated receipt funds", Lines: []ledger.Line{{AccountID: funds.ID, Amount: o.ExternalAllocatedMoney.Neg()}, {AccountID: sales.ID, Amount: o.ExternalAllocatedMoney}}}); err != nil {
					return err
				}
			}
			sources := []ledger.GrantSource{}
			for _, source := range []ledger.GrantSource{{Kind: "paid", Amount: o.PaidATG}, {Kind: "package_gift", Amount: o.PackageGiftATG}, {Kind: "merchant_gift", Amount: o.MerchantGiftATG}, {Kind: "kol_gift", Amount: o.KOLGiftATG}} {
				if source.Amount.Sign() > 0 {
					sources = append(sources, source)
				}
			}
			grant, err := s.ledger.IssueGrant(ctx, tx, ledger.GrantInput{OperatorID: o.OperatorID, CustomerID: o.CustomerID, OrderID: o.ID, BusinessKey: "order:" + o.ID, ActorID: actorID, Reason: "fully paid package delivery", PaidMoney: o.DueMoney, Sources: sources, EffectiveAt: now, ExpiresAt: end})
			if err != nil {
				return err
			}
			d.GrantID = &grant.ID
			if err = s.coverage.Fulfill(ctx, tx, o); err != nil {
				return err
			}
			o.CoverageStatus = "fulfilled"
			if o.MarketingReference != "" {
				if s.marketing == nil {
					return ErrNotReady
				}
				if err = s.marketing.Fulfill(ctx, tx, o); err != nil {
					return err
				}
			}
		}
		if err = tx.Create(&d).Error; err != nil {
			return err
		}
		o.Status = "completed"
		o.ExceptionReason = ""
		o.CompletedAt = &now
		row = orderRow(o)
		if err = tx.Save(&row).Error; err != nil {
			return err
		}
		out = Delivery(d)
		return s.event(tx, o.OperatorID, o.ID, "order.delivered", map[string]any{"actor_id": actorID, "evidence": o.ID, "before": before, "after": o, "order_id": o.ID, "delivery": out, "paid_money": o.DueMoney, "beneficiary_customer_id": o.CustomerID})
	})
	return
}
