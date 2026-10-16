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

func validFunding(in CreateOrderInput) bool {
	return validID(in.CustomerID) && validID(in.OperatorID) && validID(in.QuoteID) && in.ExpectedOwnershipVersion > 0 && strings.TrimSpace(in.BusinessKey) != "" && len(in.BusinessKey) <= 150 && in.InternalUSD.Sign() >= 0 && in.CommissionUSD.Sign() >= 0 && in.InternalUSD.Valid() && in.CommissionUSD.Valid() && validity(in.Validity) > 0 && validity(in.Validity) <= 24*time.Hour
}
func (s *Service) CreateOrder(ctx context.Context, tx *gorm.DB, in CreateOrderInput) (out Order, err error) {
	if !validFunding(in) {
		return out, ErrInvalidInput
	}
	if (in.SubscriptionID == nil) != (in.CycleDueAt == nil) {
		return out, ErrInvalidInput
	}
	if in.SubscriptionID != nil && !in.renewalAuthorized {
		return out, ErrInvalidInput
	}
	err = s.within(ctx, tx, func(tx *gorm.DB) error {
		if err := lockCommand(tx, in.OperatorID+":"+in.CustomerID, in.BusinessKey); err != nil {
			return err
		}
		var existing orderRow
		q := tx.Where("operator_id=? AND customer_id=? AND business_key=?", in.OperatorID, in.CustomerID, in.BusinessKey).Limit(1).Find(&existing)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected > 0 {
			if existing.Fingerprint != fingerprint(in) {
				return ErrConflict
			}
			out = Order(existing)
			return nil
		}
		own, err := s.org.LockOwnership(ctx, tx, in.CustomerID, in.OperatorID, in.ExpectedOwnershipVersion)
		if err != nil {
			return err
		}
		if err = s.lockBusinessScope(ctx, tx, in.OperatorID); err != nil {
			return err
		}
		c, err := s.org.GetCustomer(ctx, tx, in.CustomerID)
		if err != nil {
			return err
		}
		if c.Status != "active" {
			return ErrScope
		}
		feature := organization.NewPurchase
		kind := "package"
		if in.SubscriptionID != nil {
			feature = organization.Renewal
			kind = "renewal"
		}
		if err = s.org.RequireCustomerAction(ctx, tx, in.CustomerID, in.OperatorID, string(feature)); err != nil {
			return err
		}
		if err = s.org.RequireBusiness(ctx, tx, in.OperatorID, feature); err != nil {
			return err
		}
		quote, err := s.GetQuote(ctx, tx, in.QuoteID)
		if err != nil {
			return err
		}
		if quote.OperatorID != in.OperatorID {
			return ErrScope
		}
		p, err := s.GetPackage(ctx, tx, in.OperatorID, quote.PackageID)
		if err != nil {
			return err
		}
		if p.CurrentQuoteID == nil || *p.CurrentQuoteID != quote.ID {
			return ErrConflict
		}
		if (kind == "package" && !p.NewPurchaseEnabled) || (kind == "renewal" && !p.RenewalEnabled) {
			return organization.ErrBusinessClosed
		}
		o := Order{ID: uuid.NewString(), OperatorID: in.OperatorID, BrandID: own.BrandID, CustomerID: in.CustomerID, OwnershipVersion: own.Version, BusinessKey: in.BusinessKey, Fingerprint: fingerprint(in), Kind: kind, QuoteID: &quote.ID, DueUSD: quote.PriceUSD, PaidATG: quote.PaidATG, PackageGiftATG: quote.GiftATG, InternalUSD: in.InternalUSD, CommissionUSD: in.CommissionUSD, InternalStatus: "none", CoverageStatus: "none", Status: "pending", Period: quote.Period, Timezone: quote.Timezone, SubscriptionID: in.SubscriptionID, CycleDueAt: in.CycleDueAt, ExpiresAt: s.now().Add(validity(in.Validity)), CreatedAt: s.now()}
		o.OriginalOperatorID = o.OperatorID
		o.OriginalOwnershipVersion = o.OwnershipVersion
		if s.marketing != nil {
			m, err := s.marketing.Quote(ctx, tx, o, in.ActivitySelection)
			if err != nil {
				return err
			}
			if m.FinalUSD.Sign() <= 0 || !m.FinalUSD.Valid() || m.MerchantGiftATG.Sign() < 0 || m.KOLGiftATG.Sign() < 0 || !m.MerchantGiftATG.Valid() || !m.KOLGiftATG.Valid() {
				return ErrInvalidInput
			}
			o.DueUSD = m.FinalUSD
			o.MerchantGiftATG = m.MerchantGiftATG
			o.KOLGiftATG = m.KOLGiftATG
			o.MarketingReference = m.Reference
			o.CommissionSnapshot = m.CommissionSnapshot
		} else if len(in.ActivitySelection) > 0 && string(in.ActivitySelection) != "null" {
			return ErrNotReady
		}
		if internalTotal(o).Cmp(o.DueUSD) > 0 {
			return ErrInvalidInput
		}
		if !orderTotalATG(o).Valid() {
			return ErrInvalidInput
		}
		if s.coverage == nil {
			return ErrNotReady
		}
		commitment, err := s.coverage.Reserve(ctx, tx, o)
		if err != nil {
			return err
		}
		if commitment.Reference == "" || commitment.Version < 1 {
			return ErrNotReady
		}
		o.CoverageReference = commitment.Reference
		o.CoverageVersion = commitment.Version
		o.CoverageStatus = "reserved"
		if internalTotal(o).Sign() > 0 {
			hold, err := s.reserveInternal(ctx, tx, o, false)
			if err != nil {
				return err
			}
			o.HoldID = &hold.ID
			o.InternalStatus = "held"
		}
		if externalDue(o).IsZero() {
			o.Status = "paid"
		}
		r := orderRow(o)
		if err = tx.Create(&r).Error; err != nil {
			return err
		}
		out = o
		return s.event(tx, o.OperatorID, o.ID, "order.created", map[string]any{"before": nil, "after": o})
	})
	return
}
func (s *Service) CreateTopUpOrder(ctx context.Context, tx *gorm.DB, in CreateTopUpInput) (out Order, err error) {
	if !validID(in.CustomerID) || !validID(in.OperatorID) || strings.TrimSpace(in.BusinessKey) == "" || len(in.BusinessKey) > 150 || in.ExpectedOwnershipVersion < 1 || in.AmountUSD.Sign() <= 0 || !in.AmountUSD.Valid() || validity(in.Validity) <= 0 || validity(in.Validity) > 24*time.Hour {
		return out, ErrInvalidInput
	}
	err = s.within(ctx, tx, func(tx *gorm.DB) error {
		if err := lockCommand(tx, in.OperatorID+":"+in.CustomerID, in.BusinessKey); err != nil {
			return err
		}
		var existing orderRow
		q := tx.Where("operator_id=? AND customer_id=? AND business_key=?", in.OperatorID, in.CustomerID, in.BusinessKey).Limit(1).Find(&existing)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected > 0 {
			if existing.Fingerprint != fingerprint(in) {
				return ErrConflict
			}
			out = Order(existing)
			return nil
		}
		own, err := s.org.LockOwnership(ctx, tx, in.CustomerID, in.OperatorID, in.ExpectedOwnershipVersion)
		if err != nil {
			return err
		}
		if err = s.lockBusinessScope(ctx, tx, in.OperatorID); err != nil {
			return err
		}
		c, err := s.org.GetCustomer(ctx, tx, in.CustomerID)
		if err != nil {
			return err
		}
		if c.Status != "active" {
			return ErrScope
		}
		if err = s.org.RequireCustomerAction(ctx, tx, in.CustomerID, in.OperatorID, string(organization.NewPurchase)); err != nil {
			return err
		}
		if err = s.org.RequireBusiness(ctx, tx, in.OperatorID, organization.NewPurchase); err != nil {
			return err
		}
		o := Order{ID: uuid.NewString(), OperatorID: in.OperatorID, BrandID: own.BrandID, CustomerID: in.CustomerID, OwnershipVersion: own.Version, BusinessKey: in.BusinessKey, Fingerprint: fingerprint(in), Kind: "topup", DueUSD: in.AmountUSD, InternalStatus: "none", CoverageStatus: "none", Status: "pending", Period: ledger.Lifetime, Timezone: "UTC", ExpiresAt: s.now().Add(validity(in.Validity)), CreatedAt: s.now()}
		o.OriginalOperatorID = o.OperatorID
		o.OriginalOwnershipVersion = o.OwnershipVersion
		r := orderRow(o)
		if err = tx.Create(&r).Error; err != nil {
			return err
		}
		out = o
		return s.event(tx, o.OperatorID, o.ID, "order.created", map[string]any{"before": nil, "after": o})
	})
	return
}
func (s *Service) reserveInternal(ctx context.Context, tx *gorm.DB, o Order, restored bool) (ledger.Hold, error) {
	if o.CommissionUSD.Sign() > 0 {
		if s.commissionFunding == nil {
			return ledger.Hold{}, ErrNotReady
		}
		if err := s.commissionFunding.ReserveCommissionFunding(ctx, tx, o.OperatorID, o.CustomerID, o.ID, o.CommissionUSD); err != nil {
			return ledger.Hold{}, err
		}
	}
	if o.InternalUSD.Sign() > 0 {
		c, err := s.reserveWalletFunding(tx, o.OperatorID, o.CustomerID, "order", o.ID, o.InternalUSD, restored)
		if err != nil {
			return ledger.Hold{}, err
		}
		if c.Status != "reserved" {
			return ledger.Hold{}, ErrOrderState
		}
	}
	lines := []ledger.Line{}
	for _, fund := range []struct {
		purpose string
		amount  ledger.Decimal
	}{{"customer:usd", o.InternalUSD}, {"commission:available", o.CommissionUSD}} {
		if fund.amount.Sign() > 0 {
			a, err := s.account(ctx, tx, o.OperatorID, o.CustomerID, fund.purpose)
			if err != nil {
				return ledger.Hold{}, err
			}
			lines = append(lines, ledger.Line{AccountID: a.ID, Amount: fund.amount})
		}
	}
	key := "order-funds:" + o.ID
	expiry := o.ExpiresAt
	if restored {
		key += ":restored"
		expiry = s.now().Add(30 * time.Minute)
	}
	return s.ledger.Reserve(ctx, tx, ledger.Reservation{OperatorID: o.OperatorID, BusinessKey: key, Purpose: "order_payment", ExpiresAt: expiry, Amounts: lines})
}
func (s *Service) GetOrder(ctx context.Context, tx *gorm.DB, customerID, orderID string) (Order, error) {
	if !validID(orderID) || !validID(customerID) {
		return Order{}, ErrInvalidInput
	}
	var r orderRow
	err := s.dbFor(ctx, tx).First(&r, "id=? AND customer_id=?", orderID, customerID).Error
	return Order(r), classify(err)
}
func (s *Service) CloseOrder(ctx context.Context, tx *gorm.DB, customerID, orderID string, proof PaymentClosureProof) (out Order, err error) {
	if !proof.VerifiedUnpaid || strings.TrimSpace(proof.EvidenceReference) == "" {
		return out, ErrPaymentUnknown
	}
	err = s.within(ctx, tx, func(tx *gorm.DB) error {
		var observed orderRow
		if err := tx.First(&observed, "id=? AND customer_id=?", orderID, customerID).Error; err != nil {
			return err
		}
		if _, err := s.org.LockOwnership(ctx, tx, customerID, observed.OperatorID, observed.OwnershipVersion); err != nil {
			return err
		}
		if err := s.lockBusinessScope(ctx, tx, observed.OperatorID); err != nil {
			return err
		}
		var r orderRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&r, "id=? AND customer_id=?", orderID, customerID).Error; err != nil {
			return err
		}
		o := Order(r)
		before := o
		if o.Status == "closed" {
			out = o
			return nil
		}
		if o.Status != "pending" || o.ExternalAllocatedUSD.Sign() > 0 {
			return ErrOrderState
		}
		if o.InternalStatus == "held" && o.HoldID != nil {
			if o.CommissionUSD.Sign() > 0 {
				if s.commissionFunding == nil {
					return ErrNotReady
				}
				if err := s.commissionFunding.ReleaseCommissionFunding(ctx, tx, o.OperatorID, o.CustomerID, o.ID); err != nil {
					return err
				}
			}
			if err := s.resolveOrderWalletFunding(tx, o, false); err != nil {
				return err
			}
			if _, err := s.ledger.Release(ctx, tx, o.OperatorID, *o.HoldID, "close-order:"+o.ID); err != nil {
				return err
			}
			o.InternalStatus = "released"
		}
		if o.CoverageStatus == "reserved" {
			if s.coverage == nil {
				return ErrNotReady
			}
			if err := s.coverage.Release(ctx, tx, o); err != nil {
				return err
			}
			o.CoverageStatus = "released"
		}
		if o.MarketingReference != "" {
			if s.marketing == nil {
				return ErrNotReady
			}
			if err := s.marketing.Release(ctx, tx, o); err != nil {
				return err
			}
		}
		now := s.now()
		o.Status = "closed"
		o.ClosedAt = &now
		r = orderRow(o)
		if err := tx.Save(&r).Error; err != nil {
			return err
		}
		out = o
		return s.event(tx, o.OperatorID, o.ID, "order.closed", map[string]any{"evidence": proof.EvidenceReference, "before": before, "after": o})
	})
	return
}
