package commerce

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// WalletFundingAllocation links a cash balance debit to its original verified
// receipt. SourceOperatorID is immutable even when responsibility is transferred.
type WalletFundingAllocation struct {
	ID               string         `json:"id"`
	LotID            string         `json:"lot_id"`
	ReceiptID        string         `json:"receipt_id"`
	TopUpOrderID     string         `json:"top_up_order_id"`
	SourceOperatorID string         `json:"source_operator_id"`
	OrderID          string         `json:"order_id,omitempty"`
	RefundID         string         `json:"refund_id,omitempty"`
	RestorationID    string         `json:"restoration_id,omitempty"`
	AmountMoney      ledger.Decimal `json:"amount_money"`
	RestoredMoney    ledger.Decimal `json:"restored_money"`
	RefundedMoney    ledger.Decimal `json:"refunded_money"`
	Status           string         `json:"status"`
}

type walletLot struct {
	ID, OperatorID, SourceOperatorID, CustomerID, TopUpOrderID, ReceiptID, ReceiptAllocationID string
	AmountMoney, ReservedMoney, SpentMoney                                                     ledger.Decimal
	CreatedAt                                                                                  time.Time
}

func (walletLot) TableName() string { return "commerce_wallet_funding_lots" }

type walletCommand struct {
	ID, OperatorID, CustomerID, Kind, ReferenceID, Status string
	SourceOrderID                                         *string
	RestrictedReceiptID                                   *string
	Attempt                                               int
	AmountMoney                                           ledger.Decimal
	CreatedAt                                             time.Time
}

func (walletCommand) TableName() string { return "commerce_wallet_funding_commands" }

type walletAllocation struct {
	ID, CommandID, LotID, Status              string
	AmountMoney, RestoredMoney, RefundedMoney ledger.Decimal
	OriginalAllocationID                      *string
}

func (walletAllocation) TableName() string { return "commerce_wallet_funding_allocations" }

func walletScope(tx *gorm.DB, operator, customer string) error {
	return lockCommand(tx, "wallet", operator+":"+customer)
}
func walletInput(tx *gorm.DB, operator, customer, reference string) bool {
	return tx != nil && validID(operator) && validID(customer) && validID(reference)
}

// LockWalletFunding exposes the provenance fence to trusted cross-domain
// workflows which must lock wallet sources before receipt and ledger rows.
func (s *Service) LockWalletFunding(ctx context.Context, tx *gorm.DB, operator, customer string) error {
	if tx == nil || !validID(operator) || !validID(customer) {
		return ErrInvalidInput
	}
	return walletScope(tx.WithContext(ctx), operator, customer)
}
func (s *Service) createWalletLots(tx *gorm.DB, o Order) error {
	if err := walletScope(tx, o.OperatorID, o.CustomerID); err != nil {
		return err
	}
	var allocations []allocationRow
	if err := tx.Where("order_id=?", o.ID).Order("created_at,id").Find(&allocations).Error; err != nil {
		return err
	}
	total := ledger.Zero()
	for _, a := range allocations {
		if a.OperatorID != o.OperatorID {
			return ErrScope
		}
		total = total.Add(a.AmountMoney)
		r := walletLot{ID: uuid.NewString(), OperatorID: o.OperatorID, SourceOperatorID: o.OperatorID, CustomerID: o.CustomerID, TopUpOrderID: o.ID, ReceiptID: a.ReceiptID, ReceiptAllocationID: a.ID, AmountMoney: a.AmountMoney, CreatedAt: a.CreatedAt}
		if err := tx.Create(&r).Error; err != nil {
			return err
		}
	}
	if total.Cmp(o.DueMoney) != 0 {
		return ErrNotPaid
	}
	return nil
}

func (s *Service) fundingAllocations(tx *gorm.DB, command walletCommand) ([]WalletFundingAllocation, error) {
	var out []WalletFundingAllocation
	err := tx.Table("commerce_wallet_funding_allocations a").Select("a.id,a.lot_id,l.receipt_id,l.top_up_order_id,l.source_operator_id,a.amount_money,a.restored_money,a.refunded_money,a.status").Joins("JOIN commerce_wallet_funding_lots l ON l.id=a.lot_id").Where("a.command_id=?", command.ID).Order("l.created_at,l.id,a.id").Scan(&out).Error
	for i := range out {
		switch command.Kind {
		case "order":
			out[i].OrderID = command.ReferenceID
		case "refund":
			out[i].RefundID = command.ReferenceID
		case "restore":
			out[i].RestorationID = command.ReferenceID
			if command.SourceOrderID != nil {
				out[i].OrderID = *command.SourceOrderID
			}
		}
	}
	return out, err
}
func (s *Service) latestWalletCommand(tx *gorm.DB, operator, customer, kind, reference string) (walletCommand, bool, error) {
	var c walletCommand
	q := tx.Where("kind=? AND reference_id=?", kind, reference).Order("attempt DESC").Limit(1).Find(&c)
	if q.Error != nil {
		return c, false, q.Error
	}
	if q.RowsAffected == 0 {
		return c, false, nil
	}
	if c.OperatorID != operator || c.CustomerID != customer {
		return c, false, ErrScope
	}
	return c, true, nil
}
func (s *Service) reserveWalletFunding(tx *gorm.DB, operator, customer, kind, reference string, amount ledger.Decimal, reopen bool, receiptIDs ...string) (walletCommand, error) {
	if err := walletScope(tx, operator, customer); err != nil {
		return walletCommand{}, err
	}
	old, found, err := s.latestWalletCommand(tx, operator, customer, kind, reference)
	if err != nil {
		return old, err
	}
	if found && old.AmountMoney.Cmp(amount) != 0 {
		return old, ErrConflict
	}
	var restricted *string
	if len(receiptIDs) > 0 {
		restricted = &receiptIDs[0]
	}
	if found && ((old.RestrictedReceiptID == nil) != (restricted == nil) || restricted != nil && old.RestrictedReceiptID != nil && *restricted != *old.RestrictedReceiptID) {
		return old, ErrConflict
	}
	if found && (old.Status != "released" || !reopen) {
		return old, nil
	}
	var lots []walletLot
	query := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("operator_id=? AND customer_id=? AND amount_money>reserved_money+spent_money", operator, customer)
	if restricted != nil {
		query = query.Where("receipt_id=?", *restricted)
	}
	if err = query.Order("created_at,id").Find(&lots).Error; err != nil {
		return walletCommand{}, err
	}
	available := ledger.Zero()
	for _, l := range lots {
		available = available.Add(l.AmountMoney.Sub(l.ReservedMoney).Sub(l.SpentMoney))
	}
	if available.Cmp(amount) < 0 {
		return walletCommand{}, ledger.ErrInsufficient
	}
	c := walletCommand{ID: uuid.NewString(), OperatorID: operator, CustomerID: customer, Kind: kind, ReferenceID: reference, Attempt: 1, AmountMoney: amount, Status: "reserved", CreatedAt: s.now()}
	c.RestrictedReceiptID = restricted
	if found {
		c.Attempt = old.Attempt + 1
	}
	if err = tx.Create(&c).Error; err != nil {
		return c, err
	}
	remaining := amount
	for _, l := range lots {
		if remaining.IsZero() {
			break
		}
		take := l.AmountMoney.Sub(l.ReservedMoney).Sub(l.SpentMoney)
		if take.Cmp(remaining) > 0 {
			take = remaining
		}
		a := walletAllocation{ID: uuid.NewString(), CommandID: c.ID, LotID: l.ID, AmountMoney: take, Status: "reserved"}
		if err = tx.Create(&a).Error; err != nil {
			return c, err
		}
		if err = tx.Model(&walletLot{}).Where("id=?", l.ID).Update("reserved_money", gorm.Expr("reserved_money+?", take)).Error; err != nil {
			return c, err
		}
		remaining = remaining.Sub(take)
	}
	return c, nil
}

func (s *Service) resolveOrderWalletFunding(tx *gorm.DB, o Order, consume bool) error {
	if o.InternalMoney.IsZero() {
		return nil
	}
	if err := walletScope(tx, o.OperatorID, o.CustomerID); err != nil {
		return err
	}
	c, found, err := s.latestWalletCommand(tx, o.OperatorID, o.CustomerID, "order", o.ID)
	if err != nil {
		return err
	}
	if !found || c.AmountMoney.Cmp(o.InternalMoney) != 0 {
		return ErrNotPaid
	}
	return s.resolveWalletFunding(tx, c, "", consume)
}

// ReserveWalletRefund is an internal trusted refund-workflow command. It only
// reserves provenance; its caller must reserve cash in ledger in this same tx.
func (s *Service) ReserveWalletRefund(ctx context.Context, tx *gorm.DB, operator, customer, refundID string, amount ledger.Decimal) (out []WalletFundingAllocation, err error) {
	if !walletInput(tx, operator, customer, refundID) || amount.Sign() <= 0 || !amount.Valid() {
		return nil, ErrInvalidInput
	}
	err = s.within(ctx, tx, func(t *gorm.DB) error {
		c, e := s.reserveWalletFunding(t, operator, customer, "refund", refundID, amount, false)
		if e != nil {
			return e
		}
		out, e = s.fundingAllocations(t, c)
		return e
	})
	return
}

// ReserveWalletReceipt freezes only the named original receipt's unspent lots.
// Insufficient exposure never falls back to unrelated legitimate funding.
func (s *Service) ReserveWalletReceipt(ctx context.Context, tx *gorm.DB, operator, customer, reservationID, receiptID string, amount ledger.Decimal) (out []WalletFundingAllocation, err error) {
	if !walletInput(tx, operator, customer, reservationID) || !validID(receiptID) || amount.Sign() <= 0 || !amount.Valid() {
		return nil, ErrInvalidInput
	}
	err = s.within(ctx, tx, func(t *gorm.DB) error {
		c, e := s.reserveWalletFunding(t, operator, customer, "refund", reservationID, amount, false, receiptID)
		if e != nil {
			return e
		}
		out, e = s.fundingAllocations(t, c)
		return e
	})
	return
}

func (s *Service) resolveWalletFunding(tx *gorm.DB, c walletCommand, allocationID string, consume bool) error {
	var rows []walletAllocation
	q := tx.Where("command_id=?", c.ID)
	if allocationID != "" {
		q = q.Where("id=?", allocationID)
	}
	if err := q.Order("id").Find(&rows).Error; err != nil {
		return err
	}
	if len(rows) == 0 {
		return ErrNotFound
	}
	target := "released"
	if consume {
		target = "consumed"
	}
	for _, a := range rows {
		if a.Status != target && a.Status != "reserved" {
			return ErrOrderState
		}
	}
	for _, a := range rows {
		if a.Status == target {
			continue
		}
		if a.Status != "reserved" {
			return ErrOrderState
		}
		remaining := a.AmountMoney.Sub(a.RefundedMoney)
		changes := map[string]any{"reserved_money": gorm.Expr("reserved_money-?", remaining)}
		if consume {
			changes["spent_money"] = gorm.Expr("spent_money+?", remaining)
		}
		if err := tx.Model(&walletLot{}).Where("id=?", a.LotID).Updates(changes).Error; err != nil {
			return err
		}
		allocationChanges := map[string]any{"status": target}
		if consume && c.Kind == "refund" {
			allocationChanges["refunded_money"] = a.AmountMoney
		}
		if err := tx.Model(&walletAllocation{}).Where("id=?", a.ID).Updates(allocationChanges).Error; err != nil {
			return err
		}
	}
	var states []string
	if err := tx.Model(&walletAllocation{}).Where("command_id=?", c.ID).Distinct().Pluck("status", &states).Error; err != nil {
		return err
	}
	status := "partial"
	if len(states) == 1 {
		status = states[0]
	}
	return tx.Model(&walletCommand{}).Where("id=?", c.ID).Update("status", status).Error
}
func (s *Service) resolveWalletRefund(ctx context.Context, tx *gorm.DB, operator, customer, refundID, allocationID string, consume bool) error {
	if !walletInput(tx, operator, customer, refundID) || (allocationID != "" && !validID(allocationID)) {
		return ErrInvalidInput
	}
	return s.within(ctx, tx, func(t *gorm.DB) error {
		if err := walletScope(t, operator, customer); err != nil {
			return err
		}
		c, found, e := s.latestWalletCommand(t, operator, customer, "refund", refundID)
		if e != nil {
			return e
		}
		if !found {
			return ErrNotFound
		}
		return s.resolveWalletFunding(t, c, allocationID, consume)
	})
}
func (s *Service) CompleteWalletRefund(ctx context.Context, tx *gorm.DB, operator, customer, refundID string) error {
	return s.resolveWalletRefund(ctx, tx, operator, customer, refundID, "", true)
}
func (s *Service) ReleaseWalletRefund(ctx context.Context, tx *gorm.DB, operator, customer, refundID string) error {
	return s.resolveWalletRefund(ctx, tx, operator, customer, refundID, "", false)
}
func (s *Service) CompleteWalletRefundAllocation(ctx context.Context, tx *gorm.DB, operator, customer, refundID, allocationID string) error {
	return s.resolveWalletRefund(ctx, tx, operator, customer, refundID, allocationID, true)
}
func (s *Service) ReleaseWalletRefundAllocation(ctx context.Context, tx *gorm.DB, operator, customer, refundID, allocationID string) error {
	return s.resolveWalletRefund(ctx, tx, operator, customer, refundID, allocationID, false)
}

// OrderWalletFunding returns actual consumed USD sources and each source's
// cumulative restoration. Released payment attempts are retained internally.
func (s *Service) OrderWalletFunding(ctx context.Context, tx *gorm.DB, operator, customer, orderID string) ([]WalletFundingAllocation, error) {
	if !validID(operator) || !validID(customer) || !validID(orderID) {
		return nil, ErrInvalidInput
	}
	c, found, err := s.latestWalletCommand(s.dbFor(ctx, tx), operator, customer, "order", orderID)
	if err != nil {
		return nil, err
	}
	if !found {
		return []WalletFundingAllocation{}, nil
	}
	return s.fundingAllocations(s.dbFor(ctx, tx), c)
}

// RestoreWalletFunding restores the exact consumed lots after an approved
// package correction. The caller credits customer USD in ledger in the same tx.
func (s *Service) RestoreWalletFunding(ctx context.Context, tx *gorm.DB, operator, customer, orderID, restorationID string, amount ledger.Decimal) (out []WalletFundingAllocation, err error) {
	if !walletInput(tx, operator, customer, restorationID) || !validID(orderID) || amount.Sign() <= 0 || !amount.Valid() {
		return nil, ErrInvalidInput
	}
	err = s.within(ctx, tx, func(t *gorm.DB) error {
		if e := walletScope(t, operator, customer); e != nil {
			return e
		}
		old, found, e := s.latestWalletCommand(t, operator, customer, "restore", restorationID)
		if e != nil {
			return e
		}
		if found {
			if old.SourceOrderID == nil || *old.SourceOrderID != orderID || old.AmountMoney.Cmp(amount) != 0 {
				return ErrConflict
			}
			out, e = s.fundingAllocations(t, old)
			return e
		}
		original, found, e := s.latestWalletCommand(t, operator, customer, "order", orderID)
		if e != nil {
			return e
		}
		if !found || original.Status != "consumed" {
			return ErrOrderState
		}
		var sources []walletAllocation
		if e = t.Where("command_id=? AND status='consumed'", original.ID).Order("id").Find(&sources).Error; e != nil {
			return e
		}
		available := ledger.Zero()
		for _, a := range sources {
			available = available.Add(a.AmountMoney.Sub(a.RestoredMoney))
		}
		if amount.Cmp(available) > 0 {
			return ErrExcessAllocation
		}
		c := walletCommand{ID: uuid.NewString(), OperatorID: operator, CustomerID: customer, Kind: "restore", ReferenceID: restorationID, SourceOrderID: &orderID, Attempt: 1, AmountMoney: amount, Status: "restored", CreatedAt: s.now()}
		if e = t.Create(&c).Error; e != nil {
			return e
		}
		remaining := amount
		for _, a := range sources {
			if remaining.IsZero() {
				break
			}
			take := a.AmountMoney.Sub(a.RestoredMoney)
			if take.IsZero() {
				continue
			}
			if take.Cmp(remaining) > 0 {
				take = remaining
			}
			r := walletAllocation{ID: uuid.NewString(), CommandID: c.ID, LotID: a.LotID, AmountMoney: take, Status: "restored", OriginalAllocationID: &a.ID}
			if e = t.Create(&r).Error; e != nil {
				return e
			}
			if e = t.Model(&walletAllocation{}).Where("id=?", a.ID).Update("restored_money", gorm.Expr("restored_money+?", take)).Error; e != nil {
				return e
			}
			if e = t.Model(&walletLot{}).Where("id=?", a.LotID).Update("spent_money", gorm.Expr("spent_money-?", take)).Error; e != nil {
				return e
			}
			remaining = remaining.Sub(take)
		}
		out, e = s.fundingAllocations(t, c)
		return e
	})
	return
}
