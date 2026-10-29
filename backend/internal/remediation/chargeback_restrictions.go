package remediation

import (
	"context"
	"errors"
	"sort"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/incentives"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type chargebackOrder struct {
	OperatorID, CustomerID, OrderID, Status string
	DueMoney, FundingMoney                  ledger.Decimal
}

func (s *Service) lockReceiptBusiness(ctx context.Context, t *gorm.DB, op, receipt string) error {
	if t == nil || s.Organization == nil || s.Commerce == nil || s.Cooperation == nil || s.Incentives == nil {
		return ErrNotReady
	}
	orders, wallets, e := s.chargebackOrders(ctx, t, op, receipt)
	if e != nil {
		return e
	}
	customers := map[string]bool{}
	for _, o := range orders {
		if o.OperatorID == op {
			customers[o.CustomerID] = true
		}
	}
	for _, w := range wallets {
		if w.OperatorID == op {
			customers[w.CustomerID] = true
		}
	}
	ids := make([]string, 0, len(customers))
	for id := range customers {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		own, e := s.Organization.CurrentOwnership(ctx, t, id)
		if e != nil {
			return e
		}
		if own.OperatorID == op {
			if _, e = s.Organization.LockOwnership(ctx, t, id, op, own.Version); e != nil {
				return e
			}
		}
	}
	if e = s.Cooperation.Lock(ctx, t, op); e != nil {
		return e
	}
	return s.Incentives.LockBusiness(ctx, t, op)
}

func (s *Service) flagChargebackCommissionSources(ctx context.Context, t *gorm.DB, actor string, cb Chargeback, gross ledger.Decimal) error {
	orders, _, e := s.chargebackOrders(ctx, t, cb.OperatorID, cb.ReceiptID)
	if e != nil {
		return e
	}
	for _, order := range orders {
		if order.OperatorID != cb.OperatorID || order.Status != "completed" {
			continue
		}
		var refunded ledger.Decimal
		if e = t.Model(&Case{}).Select("COALESCE(sum(amount_money),0)::text").Where("order_id=? AND kind='package_exception' AND state NOT IN ('pending_review','rejected','cancelled')", order.OrderID).Row().Scan(&refunded); e != nil {
			return e
		}
		basis := order.DueMoney.Sub(refunded)
		var invalid ledger.Decimal
		if e = t.Table("remediation_chargeback_order_claims").Select("COALESCE(sum(amount_money),0)::text").Where("order_id=? AND state='invalid'", order.OrderID).Row().Scan(&invalid); e != nil {
			return e
		}
		basis = basis.Sub(invalid)
		if basis.Sign() <= 0 {
			continue
		}
		claim := fraction(order.FundingMoney, cb.AmountMoney, gross)
		if claim.Sign() <= 0 {
			continue
		}
		if e = s.Incentives.FlagCommissionOrder(ctx, t, actor, cb.OperatorID, order.OrderID, "chargeback:"+cb.ID, cb.Evidence, claim, basis); e != nil {
			return e
		}
	}
	return nil
}

func (s *Service) chargebackOrders(ctx context.Context, t *gorm.DB, op, receipt string) ([]chargebackOrder, []commerce.ReceiptWalletExposure, error) {
	exposure, e := s.Commerce.ReceiptExposure(ctx, t, op, receipt)
	if e != nil {
		return nil, nil, e
	}
	orders := make([]chargebackOrder, 0, len(exposure.Orders))
	for _, o := range exposure.Orders {
		orders = append(orders, chargebackOrder{OperatorID: o.OperatorID, CustomerID: o.CustomerID, OrderID: o.OrderID, Status: "completed", DueMoney: o.DueMoney, FundingMoney: o.FundingMoney})
	}
	var pending []chargebackOrder
	if e = t.Raw(`SELECT o.operator_id,o.customer_id,o.id AS order_id,o.status,o.due_money,sum(a.amount_money)::text AS funding_money FROM commerce_receipt_allocations a JOIN commerce_orders o ON o.id=a.order_id WHERE a.operator_id=? AND a.receipt_id=? AND o.status<>'completed' GROUP BY o.operator_id,o.customer_id,o.id,o.status,o.due_money ORDER BY o.id`, op, receipt).Scan(&pending).Error; e != nil {
		return nil, nil, e
	}
	orders = append(orders, pending...)
	return orders, exposure.Wallets, nil
}

// This fence precedes all receipt/case/account locks. It uses the same
// organization -> cooperation -> wallet -> receipt order as financial commands
// and never waits for a customer's ownership while holding a case row lock.
func (s *Service) lockChargeback(ctx context.Context, t *gorm.DB, op, id string) (Chargeback, error) {
	var observed Chargeback
	if t == nil || s.Organization == nil || s.Commerce == nil || s.Cooperation == nil || s.Incentives == nil {
		return observed, ErrNotReady
	}
	if e := t.First(&observed, "id=? AND operator_id=?", id, op).Error; e != nil {
		return observed, e
	}
	orders, wallets, e := s.chargebackOrders(ctx, t, op, observed.ReceiptID)
	if e != nil {
		return observed, e
	}
	customers := map[string]bool{}
	for _, o := range orders {
		if o.OperatorID == op {
			customers[o.CustomerID] = true
		}
	}
	for _, w := range wallets {
		if w.OperatorID == op {
			customers[w.CustomerID] = true
		}
	}
	var held []chargebackHold
	if e = t.Where("chargeback_id=? AND customer_id IS NOT NULL", id).Find(&held).Error; e != nil {
		return observed, e
	}
	for _, h := range held {
		customers[*h.CustomerID] = true
	}
	ids := make([]string, 0, len(customers))
	for c := range customers {
		ids = append(ids, c)
	}
	sort.Strings(ids)
	locked := []string{}
	for _, customer := range ids {
		own, e := s.Organization.CurrentOwnership(ctx, t, customer)
		if e != nil {
			return observed, e
		}
		if own.OperatorID != op {
			continue
		}
		if _, e = s.Organization.LockOwnership(ctx, t, customer, op, own.Version); e != nil {
			return observed, e
		}
		locked = append(locked, customer)
	}
	if e = s.Cooperation.Lock(ctx, t, op); e != nil {
		return observed, e
	}
	if e = s.Incentives.LockBusiness(ctx, t, op); e != nil {
		return observed, e
	}
	orderIDs := []string{}
	for _, o := range orders {
		if o.OperatorID == op {
			orderIDs = append(orderIDs, o.OrderID)
		}
	}
	if len(orderIDs) > 0 {
		var lockedOrders []struct{ ID string }
		if e = t.Table("commerce_orders").Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").Where("id IN ?", orderIDs).Order("id").Find(&lockedOrders).Error; e != nil {
			return observed, e
		}
	}
	for _, customer := range locked {
		if e = s.Commerce.LockWalletFunding(ctx, t, op, customer); e != nil {
			return observed, e
		}
	}
	if e = lock(t, op+":receipt:"+observed.ReceiptID); e != nil {
		return observed, e
	}
	if _, e = s.Commerce.LockReceipt(ctx, t, op, observed.ReceiptID); e != nil {
		return observed, e
	}
	e = t.Clauses(clause.Locking{Strength: "UPDATE"}).First(&observed, "id=? AND operator_id=?", id, op).Error
	return observed, e
}

func (s *Service) noUnsettledSourceHold(t *gorm.DB, a ledger.Account) error {
	var known ledger.Decimal
	if e := t.Model(&chargebackHold{}).Select("COALESCE(sum(amount),0)::text").Where("source_account_id=? AND state='held'", a.ID).Row().Scan(&known); e != nil {
		return e
	}
	if a.Held.Cmp(known) > 0 {
		return ErrReview
	}
	return nil
}

func (s *Service) applyChargebackRestrictions(ctx context.Context, t *gorm.DB, actor, op, id, evidence string) error {
	if evidence == "" {
		return ErrInvalid
	}
	if e := s.Organization.RequirePermission(ctx, t, actor, op, "refunds.manage"); e != nil {
		return e
	}
	cb, e := s.lockChargeback(ctx, t, op, id)
	if e != nil {
		return e
	}
	if cb.State != "disputed" {
		return ErrState
	}
	if cb.RestrictionsAppliedAt != nil {
		return nil
	}
	r, e := s.Commerce.GetReceipt(ctx, t, op, cb.ReceiptID)
	if e != nil {
		return e
	}
	var reserved, returned, claimed ledger.Decimal
	if e = t.Model(&capacity{}).Select("COALESCE(sum(CASE WHEN state='reserved' THEN amount_money-completed_money ELSE 0 END),0)::text,COALESCE(sum(completed_money),0)::text").Where("operator_id=? AND receipt_id=?", op, cb.ReceiptID).Row().Scan(&reserved, &returned); e != nil {
		return e
	}
	if reserved.Sign() > 0 {
		return ErrReview
	}
	if e = t.Model(&Chargeback{}).Select("COALESCE(sum(amount_money),0)::text").Where("operator_id=? AND receipt_id=? AND id<>? AND (state='invalid_payment' OR (state='disputed' AND restrictions_applied_at IS NOT NULL))", op, cb.ReceiptID, id).Row().Scan(&claimed); e != nil {
		return e
	}
	basis := r.GrossMoney.Sub(returned).Sub(claimed)
	if basis.Sign() <= 0 || cb.AmountMoney.Cmp(basis) > 0 {
		return ErrReview
	}
	orders, wallets, e := s.chargebackOrders(ctx, t, op, cb.ReceiptID)
	if e != nil {
		return e
	}
	at, e := instant(t)
	if e != nil {
		return e
	}
	freeze := func(account ledger.Account, customer, grant, order, wallet *string, amount ledger.Decimal, kind string) error {
		if e := s.noUnsettledSourceHold(t, account); e != nil {
			return e
		}
		amount = min(amount, account.Available())
		if amount.Sign() <= 0 {
			return nil
		}
		h, e := s.Ledger.Reserve(ctx, t, ledger.Reservation{OperatorID: op, BusinessKey: "dispute:" + id + ":" + account.ID, Purpose: "chargeback", ExpiresAt: holdExpiry(at), Amounts: []ledger.Line{{AccountID: account.ID, Amount: amount}}})
		if e != nil {
			return e
		}
		return t.Create(&chargebackHold{ID: uuid.NewString(), ChargebackID: id, OperatorID: op, CustomerID: customer, GrantID: grant, OrderID: order, WalletReservationID: wallet, HoldID: h.ID, SourceAccountID: account.ID, Amount: amount, Kind: kind, State: "held", CreatedAt: at}).Error
	}
	unallocated, e := s.account(ctx, t, op, cb.ReceiptID, "receipt:unallocated")
	if e != nil {
		return e
	}
	if e = freeze(unallocated, nil, nil, nil, nil, fraction(unallocated.Available(), cb.AmountMoney, basis), "unallocated"); e != nil {
		return e
	}
	for _, w := range wallets {
		if w.OperatorID != op {
			continue
		}
		amount := fraction(w.AvailableMoney, cb.AmountMoney, basis)
		if amount.IsZero() {
			continue
		}
		account, e := s.account(ctx, t, op, w.CustomerID, "customer:money")
		if e != nil {
			return e
		}
		if e = s.noUnsettledSourceHold(t, account); e != nil {
			return e
		}
		if account.Available().Cmp(amount) < 0 {
			return ErrConflict
		}
		reservation := uuid.NewString()
		if _, e = s.Commerce.ReserveWalletReceipt(ctx, t, op, w.CustomerID, reservation, cb.ReceiptID, amount); e != nil {
			return e
		}
		customer := w.CustomerID
		if e = freeze(account, &customer, nil, nil, &reservation, amount, "wallet"); e != nil {
			return e
		}
	}
	for _, o := range orders {
		if o.OperatorID != op {
			continue
		}
		var refunds, receiptRefunds, allClaims, sourceClaims ledger.Decimal
		if e = t.Model(&Case{}).Select("COALESCE(sum(amount_money),0)::text").Where("order_id=? AND kind='package_exception' AND state NOT IN ('pending_review','rejected','cancelled')", o.OrderID).Row().Scan(&refunds); e != nil {
			return e
		}
		if e = t.Table("remediation_refund_legs l").Select("COALESCE(sum(l.amount_money),0)::text").Joins("JOIN remediation_cases c ON c.id=l.case_id").Where("c.order_id=? AND c.kind='package_exception' AND c.state NOT IN ('pending_review','rejected','cancelled') AND l.receipt_id=?", o.OrderID, cb.ReceiptID).Row().Scan(&receiptRefunds); e != nil {
			return e
		}
		if e = t.Table("remediation_chargeback_order_claims").Select("COALESCE(sum(amount_money),0)::text,COALESCE(sum(CASE WHEN receipt_id=? THEN amount_money ELSE 0 END),0)::text", cb.ReceiptID).Where("order_id=? AND state IN ('active','invalid')", o.OrderID).Row().Scan(&allClaims, &sourceClaims); e != nil {
			return e
		}
		sourceFunding := o.FundingMoney.Sub(receiptRefunds).Sub(sourceClaims)
		orderBasis := o.DueMoney.Sub(refunds).Sub(allClaims)
		if sourceFunding.Sign() <= 0 || orderBasis.Sign() <= 0 {
			continue
		}
		claim := fraction(sourceFunding, cb.AmountMoney, basis)
		if claim.Sign() <= 0 {
			continue
		}
		if claim.Cmp(orderBasis) > 0 {
			return ErrConflict
		}
		if e = t.Exec(`INSERT INTO remediation_chargeback_order_claims(chargeback_id,receipt_id,order_id,amount_money,state) VALUES(?,?,?,?,'active')`, id, cb.ReceiptID, o.OrderID, claim).Error; e != nil {
			return e
		}
		customer := o.CustomerID
		if o.Status != "completed" {
			funds, e := s.account(ctx, t, op, o.OrderID, "order:funds")
			if e != nil {
				return e
			}
			if e = freeze(funds, &customer, nil, &o.OrderID, nil, claim, "order_funds"); e != nil {
				return e
			}
			continue
		}
		var g ledger.Grant
		if e = t.First(&g, "order_id=? AND operator_id=?", o.OrderID, op).Error; e != nil {
			return e
		}
		account, e := s.Ledger.AccountInTransaction(ctx, t, g.AccountID)
		if e != nil {
			return e
		}
		if e = freeze(account, &customer, &g.ID, &o.OrderID, nil, fraction(account.Available(), claim, orderBasis), "grant"); e != nil {
			return e
		}
		events, e := s.Incentives.RemediationEvents(ctx, t, op, o.OrderID)
		if e != nil {
			return e
		}
		for _, event := range events {
			var pendingClaims ledger.Decimal
			if e = t.Model(&chargebackEvent{}).Select("COALESCE(sum(amount_money),0)::text").Where("source_event_id=? AND state IN ('disputed','observed')", event.SourceEventID).Row().Scan(&pendingClaims); e != nil {
				return e
			}
			remaining := event.PaidMoney.Sub(event.ReversedMoney).Sub(pendingClaims)
			amount := fraction(remaining, claim, orderBasis)
			if amount.Sign() <= 0 {
				continue
			}
			if e = s.Incentives.HoldCommissionSource(ctx, t, actor, op, event.SourceEventID, "chargeback:"+id, evidence); e != nil {
				if errors.Is(e, incentives.ErrBlocked) {
					return ErrReview
				}
				return e
			}
			state := "observed"
			if event.Status != "confirmed" {
				if e = s.Incentives.HoldEventDispute(ctx, t, actor, op, event.SourceEventID, "chargeback:"+id, evidence); e != nil {
					return e
				}
				state = "disputed"
			}
			if e = t.Create(&chargebackEvent{ChargebackID: id, SourceEventID: event.SourceEventID, OperatorID: op, AmountMoney: amount, State: state}).Error; e != nil {
				return e
			}
		}
	}
	if e = s.Incentives.SealCommissionOrderRestrictions(ctx, t, op, "chargeback:"+id); e != nil {
		return e
	}
	if e = t.Model(&Chargeback{}).Where("id=?", id).Update("restrictions_applied_at", at).Error; e != nil {
		return e
	}
	return s.event(t, op, id, "chargeback.source_restrictions_applied", actor, evidence, map[string]any{"remaining_receipt_basis_money": basis, "claimed_money": cb.AmountMoney})
}
