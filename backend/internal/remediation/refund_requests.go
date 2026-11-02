package remediation

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type WalletRefundInput struct {
	OperatorID  string         `json:"operator_id"`
	CustomerID  string         `json:"customer_id"`
	BusinessKey string         `json:"business_key"`
	AmountMoney ledger.Decimal `json:"amount_money"`
	Reason      string         `json:"reason"`
}

func (s *Service) RequestWalletRefund(ctx context.Context, in WalletRefundInput) (Case, error) {
	var out Case
	if !validID(in.OperatorID) || !validID(in.CustomerID) || in.BusinessKey == "" || len(in.BusinessKey) > 150 || !validAmount(in.AmountMoney) || in.Reason == "" {
		return out, ErrInvalid
	}
	e := s.within(ctx, nil, func(t *gorm.DB) error {
		if e := s.Organization.RequireCustomerAction(ctx, t, in.CustomerID, in.OperatorID, "wallet_refund"); e != nil {
			return e
		}
		own, e := s.Organization.CurrentOwnership(ctx, t, in.CustomerID)
		if e != nil {
			return e
		}
		if _, e = s.Organization.LockOwnership(ctx, t, in.CustomerID, in.OperatorID, own.Version); e != nil {
			return e
		}
		if e = lock(t, in.OperatorID+":"+in.CustomerID); e != nil {
			return e
		}
		found, e := s.existing(t, in.OperatorID, in.BusinessKey, fingerprint(in), &out)
		if e != nil || found {
			return e
		}
		at, e := instant(t)
		if e != nil {
			return e
		}
		out = Case{ID: uuid.NewString(), OperatorID: in.OperatorID, CustomerID: &in.CustomerID, BusinessKey: in.BusinessKey, Fingerprint: fingerprint(in), Kind: "wallet", AmountMoney: in.AmountMoney, State: "pending_review", Reason: in.Reason, Evidence: "customer requested unused USD return", RequestedBy: in.CustomerID, CreatedAt: at}
		if e = t.Create(&out).Error; e != nil {
			return e
		}
		allocations, e := s.Commerce.ReserveWalletRefund(ctx, t, in.OperatorID, in.CustomerID, out.ID, in.AmountMoney)
		if e != nil {
			return e
		}
		grouped := map[string]ledger.Decimal{}
		for _, a := range allocations {

			grouped[a.ReceiptID] = grouped[a.ReceiptID].Add(a.AmountMoney)
		}
		account, e := s.account(ctx, t, in.OperatorID, in.CustomerID, "customer:money")
		if e != nil {
			return e
		}
		if e = s.createLegs(ctx, t, out, grouped, &account, "customer requested unused USD return"); e != nil {
			return e
		}
		legs, e := s.Legs(ctx, t, out.OperatorID, out.ID)
		if e != nil {
			return e
		}
		for _, leg := range legs {
			for _, a := range allocations {
				if a.ReceiptID == leg.ReceiptID {
					if e = t.Exec("INSERT INTO remediation_wallet_links(leg_id,allocation_id) VALUES(?,?)", leg.ID, a.ID).Error; e != nil {
						return e
					}
				}
			}
		}
		return nil
	})
	return out, e
}
func (s *Service) existing(t *gorm.DB, op, key, fp string, out *Case) (bool, error) {
	e := t.First(out, "operator_id=? AND business_key=?", op, key).Error
	if e == nil {
		if out.Fingerprint != fp {
			return true, ErrConflict
		}
		return true, nil
	}
	if !errors.Is(e, gorm.ErrRecordNotFound) {
		return false, e
	}
	return false, nil
}

// Unallocated returns are staff-managed because there may be no registered
// customer. The receipt's payer evidence remains the refund destination basis.
func (s *Service) RequestUnallocatedRefund(ctx context.Context, actor, op, receipt, key, reason, evidence string, amount ledger.Decimal) (Case, error) {
	var out Case
	if !validID(op) || !validID(receipt) || key == "" || len(key) > 150 || reason == "" || evidence == "" || !validAmount(amount) {
		return out, ErrInvalid
	}
	fp := fingerprint([]string{actor, op, receipt, key, reason, evidence, amount.String()})
	e := s.within(ctx, nil, func(t *gorm.DB) error {
		if e := s.Organization.RequirePermission(ctx, t, actor, op, "refunds.manage"); e != nil {
			return e
		}
		if e := lock(t, op+":receipt:"+receipt); e != nil {
			return e
		}
		found, e := s.existing(t, op, key, fp, &out)
		if e != nil || found {
			return e
		}
		if _, e = s.Commerce.GetReceipt(ctx, t, op, receipt); e != nil {
			return e
		}
		a, e := s.account(ctx, t, op, receipt, "receipt:unallocated")
		if e != nil {
			return e
		}
		at, e := instant(t)
		if e != nil {
			return e
		}
		out = Case{ID: uuid.NewString(), OperatorID: op, BusinessKey: key, Fingerprint: fp, Kind: "unallocated", AmountMoney: amount, State: "pending_review", Reason: reason, Evidence: evidence, RequestedBy: actor, CreatedAt: at}
		if e = t.Create(&out).Error; e != nil {
			return e
		}
		return s.createLegs(ctx, t, out, map[string]ledger.Decimal{receipt: amount}, &a, evidence)
	})
	return out, e
}

func (s *Service) createLegs(ctx context.Context, t *gorm.DB, c Case, amounts map[string]ledger.Decimal, source *ledger.Account, evidence string) error {
	ids := []string{}
	for id, a := range amounts {
		if a.Sign() > 0 {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	for _, id := range ids {
		if e := lock(t, c.OperatorID+":receipt:"+id); e != nil {
			return e
		}
		var origin string
		if e := t.Table("commerce_receipts").Select("operator_id").Where("id=?", id).Row().Scan(&origin); e != nil {
			return e
		}
		leg := RefundLeg{SourceOperatorID: origin, ID: uuid.NewString(), CaseID: c.ID, OperatorID: c.OperatorID, ReceiptID: id, AmountMoney: amounts[id], State: "reserved", CreatedAt: c.CreatedAt}
		if e := s.ReserveReceiptRefund(ctx, t, origin, id, leg.ID, leg.AmountMoney); e != nil {
			return e
		}
		if source != nil {
			h, e := s.Ledger.Reserve(ctx, t, ledger.Reservation{OperatorID: c.OperatorID, BusinessKey: "refund:" + leg.ID, Purpose: "refund", ExpiresAt: c.CreatedAt.AddDate(10, 0, 0), Amounts: []ledger.Line{{AccountID: source.ID, Amount: leg.AmountMoney}}})
			if e != nil {
				return e
			}
			leg.HoldID = &h.ID
			leg.SourceAccountID = &source.ID
		}
		if e := t.Create(&leg).Error; e != nil {
			return e
		}
	}
	return s.event(t, c.OperatorID, c.ID, "refund.requested", c.RequestedBy, evidence, c)
}

type PackageExceptionInput struct {
	ActorStaffID string `json:"actor_staff_id"`
	OperatorID   string `json:"operator_id"`
	CustomerID   string `json:"customer_id"`
	OrderID      string `json:"order_id"`
	BusinessKey  string `json:"business_key"`
	Reason       string `json:"reason"`
	Evidence     string `json:"evidence"`
}

// RequestPackageException refunds a package or renewal that was paid but never
// received ATG. A delivered grant cannot be turned back into money.
func (s *Service) RequestPackageException(ctx context.Context, in PackageExceptionInput) (Case, error) {
	var out Case
	if in.Reason != "transaction_error" && in.Reason != "unable_to_fulfill" {
		return out, ErrInvalid
	}
	if !validID(in.CustomerID) || !validID(in.OrderID) || in.BusinessKey == "" || len(in.BusinessKey) > 150 || in.Evidence == "" {
		return out, ErrInvalid
	}
	e := s.within(ctx, nil, func(t *gorm.DB) error {
		if e := s.Organization.RequirePermission(ctx, t, in.ActorStaffID, in.OperatorID, "refunds.manage"); e != nil {
			return e
		}
		own, e := s.Organization.CurrentOwnership(ctx, t, in.CustomerID)
		if e != nil {
			return e
		}
		if _, e = s.Organization.LockOwnership(ctx, t, in.CustomerID, in.OperatorID, own.Version); e != nil {
			return e
		}
		if e = s.Cooperation.Lock(ctx, t, in.OperatorID); e != nil {
			return e
		}
		if e = s.Incentives.LockBusiness(ctx, t, in.OperatorID); e != nil {
			return e
		}
		if e = lock(t, in.OperatorID+":"+in.CustomerID); e != nil {
			return e
		}
		found, e := s.existing(t, in.OperatorID, in.BusinessKey, fingerprint(in), &out)
		if e != nil || found {
			return e
		}
		o, e := s.Commerce.MarkUndeliveredRefund(ctx, t, in.CustomerID, in.OrderID)
		if e != nil {
			return e
		}
		if o.OperatorID != in.OperatorID {
			return ErrScope
		}
		at, e := instant(t)
		if e != nil {
			return e
		}
		out = Case{ID: uuid.NewString(), OperatorID: in.OperatorID, CustomerID: &in.CustomerID, BusinessKey: in.BusinessKey, Fingerprint: fingerprint(in), Kind: "package_exception", OrderID: &o.ID, AmountMoney: o.DueMoney, State: "pending_review", Reason: in.Reason, Evidence: in.Evidence, RequestedBy: in.ActorStaffID, CreatedAt: at, InternalMoney: o.InternalMoney, CommissionMoney: o.CommissionMoney}
		if e = t.Create(&out).Error; e != nil {
			return e
		}
		var allocs []struct {
			ReceiptID   string
			AmountMoney ledger.Decimal
		}
		if e = t.Table("commerce_receipt_allocations").Select("receipt_id,sum(amount_money) AS amount_money").Where("order_id=?", o.ID).Group("receipt_id").Order("receipt_id").Scan(&allocs).Error; e != nil {
			return e
		}
		group := map[string]ledger.Decimal{}
		external := ledger.Zero()
		for _, a := range allocs {
			group[a.ReceiptID] = a.AmountMoney
			external = external.Add(a.AmountMoney)
		}
		if external.Cmp(o.DueMoney.Sub(o.InternalMoney).Sub(o.CommissionMoney)) != 0 {
			return ErrConflict
		}
		return s.createLegs(ctx, t, out, group, nil, in.Evidence)
	})
	return out, e
}

type ProcurementNonDeliveryInput struct {
	ActorStaffID string `json:"actor_staff_id"`
	SupplierID   string `json:"supplier_id"`
	OrderID      string `json:"order_id"`
	BusinessKey  string `json:"business_key"`
	Reason       string `json:"reason"`
	Evidence     string `json:"evidence"`
}

// RequestProcurementNonDelivery refunds a purchase that was paid but never
// received ATG. Delivered procurement inventory is not refunded here.
func (s *Service) RequestProcurementNonDelivery(ctx context.Context, in ProcurementNonDeliveryInput) (Case, error) {
	var out Case
	if in.Reason == "" || !validID(in.OrderID) || in.BusinessKey == "" || len(in.BusinessKey) > 150 || in.Evidence == "" {
		return out, ErrInvalid
	}
	e := s.within(ctx, nil, func(t *gorm.DB) error {
		if e := s.Organization.RequirePermission(ctx, t, in.ActorStaffID, in.SupplierID, "refunds.manage"); e != nil {
			return e
		}
		found, e := s.existing(t, in.SupplierID, in.BusinessKey, fingerprint(in), &out)
		if e != nil || found {
			return e
		}
		o, allocation, e := s.Cooperation.MarkUndeliveredPurchaseRefund(ctx, t, in.SupplierID, in.OrderID)
		if e != nil {
			return e
		}
		at, e := instant(t)
		if e != nil {
			return e
		}
		out = Case{ID: uuid.NewString(), OperatorID: in.SupplierID, BusinessKey: in.BusinessKey, Fingerprint: fingerprint(in), Kind: "procurement_nondelivery", OrderID: &o.ID, AmountMoney: o.DueMoney, State: "pending_review", Reason: in.Reason, Evidence: in.Evidence, RequestedBy: in.ActorStaffID, CreatedAt: at}
		if e = t.Create(&out).Error; e != nil {
			return e
		}
		return s.createLegs(ctx, t, out, map[string]ledger.Decimal{allocation.ReceiptID: allocation.AmountMoney}, nil, in.Evidence)
	})
	return out, e
}

func (s *Service) lockCase(ctx context.Context, t *gorm.DB, op, id string) (Case, error) {
	c, e := s.GetCase(ctx, t, op, id)
	if e != nil {
		return c, e
	}
	if c.CustomerID != nil {
		own, e := s.Organization.CurrentOwnership(ctx, t, *c.CustomerID)
		if e != nil {
			return c, e
		}
		if _, e = s.Organization.LockOwnership(ctx, t, *c.CustomerID, op, own.Version); e != nil {
			return c, e
		}
		if e = s.Cooperation.Lock(ctx, t, op); e != nil {
			return c, e
		}
		if e = s.Incentives.LockBusiness(ctx, t, op); e != nil {
			return c, e
		}
		if e = lock(t, op+":"+*c.CustomerID); e != nil {
			return c, e
		}
	}
	e = t.Clauses(clause.Locking{Strength: "UPDATE"}).First(&c, "id=? AND operator_id=?", id, op).Error
	return c, e
}
func holdExpiry(at time.Time) time.Time { return at.AddDate(10, 0, 0) }
