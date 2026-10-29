package cooperation

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/payments"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"math/big"
	"time"
)

var ErrNotProcurementRefund = errors.New("refund does not belong to procurement exit")
var ErrRefundFX = errors.New("external refund execution cannot exactly discharge the USD obligation")

type ReceiptRefundReservations interface {
	ReserveReceiptRefund(context.Context, *gorm.DB, string, string, string, ledger.Decimal) error
	CompleteReceiptRefund(context.Context, *gorm.DB, string, string, string) error
	ReleaseReceiptRefund(context.Context, *gorm.DB, string, string, string) error
	RequireReceiptRefundExecution(context.Context, *gorm.DB, string, string, string) error
}
type ProcurementRefunds struct {
	Cooperation          *Service
	Payments             *payments.Service
	Collector            *commerce.Service
	Capacity             ReceiptRefundReservations
	ObserveReceiptRefund func(context.Context, *gorm.DB, string, string, string, string, ledger.Decimal) error
}
type ProcurementExitRefund struct {
	ID                 string          `json:"id"`
	ExitID             string          `json:"exit_id"`
	BuyerID            string          `json:"buyer_id"`
	SupplierID         string          `json:"supplier_id"`
	BusinessKey        string          `json:"business_key"`
	Fingerprint        string          `json:"-"`
	ReceiptID          string          `json:"receipt_id"`
	SourceKind         string          `json:"source_kind" gorm:"default:payment"`
	MerchantID         string          `json:"merchant_id"`
	OriginOrderID      string          `json:"origin_order_id"`
	AmountMoney        ledger.Decimal  `json:"amount_money"`
	HoldID             string          `json:"hold_id"`
	State              string          `json:"state"`
	VerifiedFactID     *string         `json:"verified_fact_id,omitempty"`
	ExecutionValuation json.RawMessage `gorm:"type:jsonb" json:"execution_valuation,omitempty"`
	CreatedBy          string          `json:"created_by"`
	CreatedAt          time.Time       `json:"created_at"`
	ExecutionMode      string          `json:"execution_mode"`
	ExecutionKey       string          `json:"execution_key"`
	ExecutionEvidence  string          `json:"execution_evidence"`
	FeeStatus          string          `json:"fee_status" gorm:"default:pending"`
	FeeMoney           *ledger.Decimal `json:"fee_money,omitempty"`
	SettledMoney       ledger.Decimal  `json:"settled_money"`
	ActualMoney        ledger.Decimal  `json:"actual_money"`
	ExcessMoney        ledger.Decimal  `json:"excess_money"`
}

func (ProcurementExitRefund) TableName() string { return "cooperation_procurement_exit_refunds" }
func (p *ProcurementRefunds) Ready() bool {
	return p.Cooperation != nil && p.Payments != nil && p.Collector != nil && p.Capacity != nil
}
func (p *ProcurementRefunds) payable(ctx context.Context, tx *gorm.DB, exit ProcurementExit) (ledger.Account, error) {
	return p.Cooperation.Ledger.EnsureAccount(ctx, tx, ledger.Account{OperatorID: exit.SupplierID, OwnerID: exit.ID, Purpose: "procurement_exit:payable", Unit: "MONEY"})
}
func (s *Service) bookExitObligation(ctx context.Context, tx *gorm.DB, exit ProcurementExit) error {
	if exit.ContractMoney.IsZero() {
		return nil
	}
	for _, side := range []struct{ operator, purpose string }{{exit.SupplierID, "procurement_exit:payable"}, {exit.BuyerID, "procurement_exit:receivable"}} {
		a, e := s.Ledger.EnsureAccount(ctx, tx, ledger.Account{OperatorID: side.operator, OwnerID: exit.ID, Purpose: side.purpose, Unit: "MONEY"})
		if e != nil {
			return e
		}
		control, e := s.Ledger.EnsureAccount(ctx, tx, ledger.Account{OperatorID: side.operator, OwnerID: side.operator, Purpose: "control:procurement_exit_obligation", Unit: "MONEY", AllowNegative: true})
		if e != nil {
			return e
		}
		if _, e = s.Ledger.Post(ctx, tx, ledger.Posting{OperatorID: side.operator, BusinessKey: "procurement-exit:" + exit.ID, Kind: "procurement_exit_obligation", SourceID: exit.ID, ActorID: exit.CreatedBy, Reason: exit.Evidence, Lines: []ledger.Line{{AccountID: a.ID, Amount: exit.ContractMoney}, {AccountID: control.ID, Amount: exit.ContractMoney.Neg()}}}); e != nil {
			return e
		}
	}
	return nil
}

// PrepareRefund reserves both the approved exit liability and the shared
// original-receipt capacity. It cannot refund a receipt from another purchase.
func (p *ProcurementRefunds) PrepareRefund(ctx context.Context, actor, exitID, receiptID, key string, amount ledger.Decimal) (ProcurementExitRefund, error) {
	out := ProcurementExitRefund{}
	if !p.Ready() {
		return out, ErrReplenishment
	}
	if amount.Sign() <= 0 || !amount.Valid() || key == "" {
		return out, ErrConflict
	}
	e := p.Cooperation.within(ctx, nil, func(t *gorm.DB) error {
		var exit ProcurementExit
		if e := t.First(&exit, "id=?", exitID).Error; e != nil {
			return e
		}
		if e := p.Cooperation.Organization.RequirePermission(ctx, t, actor, exit.SupplierID, "payments.manage"); e != nil {
			return e
		}
		if e := p.Cooperation.Lock(ctx, t, exit.BuyerID); e != nil {
			return e
		}
		if e := t.Clauses(clause.Locking{Strength: "UPDATE"}).First(&exit, "id=?", exitID).Error; e != nil {
			return e
		}
		fp := hash([]string{exitID, receiptID, key, amount.String()})
		q := t.Where("exit_id=? AND business_key=?", exitID, key).Limit(1).Find(&out)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected > 0 {
			if out.Fingerprint != fp {
				return ErrConflict
			}
			return nil
		}
		service := *p.Payments
		service.DB = t
		source, e := service.PaymentSourceForReceipt(ctx, exit.SupplierID, receiptID)
		sourceKind := "payment"
		if errors.Is(e, gorm.ErrRecordNotFound) {
			if _, e = p.manualReceipt(ctx, t, exit.SupplierID, receiptID); e != nil {
				return e
			}
			sourceKind = "manual_receipt"
		} else if e != nil {
			return e
		}
		var removal ProcurementRemoval
		if e = t.First(&removal, "id=?", exit.RemovalID).Error; e != nil {
			return e
		}
		var sources []RemovedSource
		if json.Unmarshal(removal.Sources, &sources) != nil {
			return ErrConflict
		}
		eligible := new(big.Rat)
		for _, src := range sources {
			if src.PaidATG.IsZero() || src.OriginalPaidATG.IsZero() {
				continue
			}
			if _, e := uuid.Parse(src.OrderID); e != nil {
				continue
			}
			var order PurchaseOrder
			if e = t.First(&order, "id=? AND supplier_id=? AND buyer_id=?", src.OrderID, exit.SupplierID, exit.BuyerID).Error; e != nil {
				return e
			}
			var allocated ledger.Decimal
			if e = t.Model(&PurchaseAllocation{}).Where("order_id=? AND receipt_id=?", order.ID, receiptID).Select("COALESCE(sum(amount_money),0)::text").Row().Scan(&allocated); e != nil {
				return e
			}
			if allocated.IsZero() {
				continue
			}
			part := new(big.Rat).Mul(src.OriginalPaidMoney.Rat(), new(big.Rat).Quo(src.PaidATG.Rat(), src.OriginalPaidATG.Rat()))
			part.Mul(part, new(big.Rat).Quo(allocated.Rat(), order.DueMoney.Rat()))
			eligible.Add(eligible, part)
		}
		cap, _ := ledger.Quantize(eligible)
		var used ledger.Decimal
		if e = t.Model(&ProcurementExitRefund{}).Where("exit_id=? AND receipt_id=? AND state<>'released_unpaid'", exitID, receiptID).Select("COALESCE(sum(amount_money),0)::text").Row().Scan(&used); e != nil {
			return e
		}
		if used.Add(amount).Cmp(cap) > 0 {
			return ErrConflict
		}
		id := uuid.NewString()
		if e = p.Capacity.ReserveReceiptRefund(ctx, t, exit.SupplierID, receiptID, id, amount); e != nil {
			return e
		}
		payable, e := p.payable(ctx, t, exit)
		if e != nil {
			return e
		}
		now, e := businessTime(ctx, t)
		if e != nil {
			return e
		}
		hold, e := p.Cooperation.Ledger.Reserve(ctx, t, ledger.Reservation{OperatorID: exit.SupplierID, BusinessKey: "procurement-exit-refund:" + id, Purpose: "procurement_exit_refund", ExpiresAt: now.Add(365 * 24 * time.Hour), Amounts: []ledger.Line{{AccountID: payable.ID, Amount: amount}}})
		if e != nil {
			return e
		}
		out = ProcurementExitRefund{ID: id, ExitID: exitID, BuyerID: exit.BuyerID, SupplierID: exit.SupplierID, BusinessKey: key, Fingerprint: fp, ReceiptID: receiptID, SourceKind: sourceKind, MerchantID: source.MerchantID, OriginOrderID: source.OriginalOrder.ID, AmountMoney: amount, HoldID: hold.ID, State: "prepared", CreatedBy: actor, CreatedAt: now}
		if sourceKind == "manual_receipt" {
			return t.Omit("MerchantID").Create(&out).Error
		}
		return t.Create(&out).Error
	})
	return out, e
}

// RequireApprovedRefund is suitable for a composite payments approval adapter.
// It rejects FX mismatch before sending, leaving the original USD debt open.
func (p *ProcurementRefunds) RequireApprovedRefund(ctx context.Context, tx *gorm.DB, actor string, r payments.Refund) error {
	return p.requireRefundMatch(ctx, tx, actor, r, false)
}

// RequireVerifiedManualRefund matches an already executed bank fact to the
// durable approved manual instruction. New disputes must block new transfers,
// but cannot make a previously completed transfer disappear.
func (p *ProcurementRefunds) RequireVerifiedManualRefund(ctx context.Context, tx *gorm.DB, actor string, r payments.Refund) error {
	return p.requireRefundMatch(ctx, tx, actor, r, true)
}
func (p *ProcurementRefunds) requireRefundMatch(ctx context.Context, tx *gorm.DB, actor string, r payments.Refund, verifiedManual bool) error {
	var leg ProcurementExitRefund
	result := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=?", r.ID).Limit(1).Find(&leg)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotProcurementRefund
	}
	if e := p.Cooperation.Organization.RequirePermission(ctx, tx, actor, leg.SupplierID, "payments.manage"); e != nil {
		return e
	}
	if leg.State != "prepared" && leg.State != "executing" && leg.State != "manual_executing" {
		return ErrConflict
	}
	if r.MoneyAmount.Cmp(leg.AmountMoney) != 0 || r.Order.ID != leg.OriginOrderID || r.Order.OperatorID != leg.SupplierID || r.Order.MerchantID != leg.MerchantID {
		return ErrConflict
	}
	if verifiedManual {
		if leg.ExecutionMode != "manual" || leg.ExecutionKey == "" || leg.State != "manual_executing" && leg.State != "executing" {
			return ErrConflict
		}
	} else {
		if leg.ExecutionMode == "manual" {
			return ErrConflict
		}
		if e := p.Capacity.RequireReceiptRefundExecution(ctx, tx, leg.SupplierID, leg.ReceiptID, leg.ID); e != nil {
			return e
		}
	}
	if !verifiedManual && r.Money.Currency != r.Order.Settlement().Currency {
		value, e := r.RefundValuation.Convert(r.Money, r.ValuedAt)
		if e != nil || value.Amount.Cmp(leg.AmountMoney) != 0 {
			return ErrRefundFX
		}
	}
	return tx.Model(&ProcurementExitRefund{}).Where("id=?", leg.ID).Update("state", "executing").Error
}
func (p *ProcurementRefunds) Execute(ctx context.Context, actor, id string, money payments.Money, valuation payments.Valuation, valuedAt time.Time, exchangeRef string) (payments.RefundStatus, error) {
	if !p.Ready() {
		return payments.RefundStatus{}, ErrReplenishment
	}
	var leg ProcurementExitRefund
	if e := p.Cooperation.DB.WithContext(ctx).First(&leg, "id=?", id).Error; e != nil {
		return payments.RefundStatus{}, e
	}
	if leg.SourceKind == "manual_receipt" {
		return payments.RefundStatus{}, payments.ErrUnsupported
	}
	source, e := p.Payments.PaymentSourceForReceipt(ctx, leg.SupplierID, leg.ReceiptID)
	if e != nil {
		return payments.RefundStatus{}, e
	}
	service := *p.Payments
	service.RefundApproval = p
	service.RefundSink = p
	return service.SubmitRefund(ctx, actor, leg.SupplierID, leg.MerchantID, payments.Refund{ID: leg.ID, Order: source.OriginalOrder, Money: money, MoneyAmount: leg.AmountMoney, RefundValuation: valuation, ValuedAt: valuedAt, FXExecutionReference: exchangeRef})
}
func (p *ProcurementRefunds) RecordVerifiedRefund(ctx context.Context, tx *gorm.DB, m payments.Merchant, o payments.Order, r payments.Refund, f payments.Fact, value payments.MoneyValue) error {
	return p.recordRefundObservation(ctx, tx, m, o, r, f, value)
}
func (p *ProcurementRefunds) Run(ctx context.Context, limit int) ([]ProcurementExitRefund, error) {
	if !p.Ready() {
		return nil, ErrReplenishment
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	legs := []ProcurementExitRefund{}
	if e := p.Cooperation.DB.WithContext(ctx).Where("state IN ('executing','manual_executing','exception')").Order("last_checked_at,id").Limit(limit).Find(&legs).Error; e != nil {
		return nil, e
	}
	service := *p.Payments
	service.RefundSink = p
	for _, leg := range legs {
		if e := p.Cooperation.DB.WithContext(ctx).Model(&ProcurementExitRefund{}).Where("id=?", leg.ID).Update("last_checked_at", gorm.Expr("clock_timestamp()")).Error; e != nil {
			return legs, e
		}
		if leg.SourceKind == "manual_receipt" && leg.MerchantID == "" {
			continue
		}
		work, cancel := context.WithTimeout(ctx, 25*time.Second)
		executions, e := service.RefundExecutions(work, leg.SupplierID, leg.MerchantID, leg.ID)
		if e == nil {
			for _, execution := range executions {
				if execution.State != "succeeded" {
					_, _ = service.QueryRefund(work, leg.SupplierID, leg.MerchantID, execution.ExecutionID)
				}
			}
		}
		cancel()
	}
	return legs, nil
}
func (p *ProcurementRefunds) List(ctx context.Context, actor, buyer string) ([]ProcurementExit, []ProcurementExitRefund, error) {
	exits := []ProcurementExit{}
	legs := []ProcurementExitRefund{}
	e := p.Cooperation.within(ctx, nil, func(t *gorm.DB) error {
		if e := p.Cooperation.Organization.RequirePermission(ctx, t, actor, buyer, "cooperation.manage"); e != nil {
			return e
		}
		if e := t.Where("buyer_id=?", buyer).Order("created_at DESC").Limit(100).Find(&exits).Error; e != nil {
			return e
		}
		return t.Where("buyer_id=?", buyer).Order("created_at DESC").Limit(100).Find(&legs).Error
	})
	return exits, legs, e
}

func (p *ProcurementRefunds) ListForOperator(ctx context.Context, actor, operator string) ([]ProcurementExit, []ProcurementExitRefund, error) {
	exits := []ProcurementExit{}
	legs := []ProcurementExitRefund{}
	if !p.Ready() {
		return exits, legs, ErrReplenishment
	}
	e := p.Cooperation.within(ctx, nil, func(tx *gorm.DB) error {
		if e := p.Cooperation.Organization.RequirePermission(ctx, tx, actor, operator, "payments.manage"); e != nil {
			return e
		}
		if e := tx.Where("supplier_id=? OR buyer_id=?", operator, operator).Order("created_at DESC,id").Limit(100).Find(&exits).Error; e != nil {
			return e
		}
		return tx.Where("supplier_id=? OR buyer_id=?", operator, operator).Order("created_at DESC,id").Limit(100).Find(&legs).Error
	})
	return exits, legs, e
}

// ReleasePrepared cancels only a command that has never entered payment
// execution. Unknown or provider-submitted refunds keep both reservations.
func (p *ProcurementRefunds) ReleasePrepared(ctx context.Context, actor, id, reason string) error {
	if !p.Ready() || reason == "" {
		return ErrConflict
	}
	return p.Cooperation.within(ctx, nil, func(t *gorm.DB) error {
		var leg ProcurementExitRefund
		if e := t.Clauses(clause.Locking{Strength: "UPDATE"}).First(&leg, "id=?", id).Error; e != nil {
			return e
		}
		if e := p.Cooperation.Organization.RequirePermission(ctx, t, actor, leg.SupplierID, "payments.manage"); e != nil {
			return e
		}
		if leg.State == "released_unpaid" {
			return nil
		}
		if leg.State != "prepared" {
			return ErrConflict
		}
		if _, e := p.Cooperation.Ledger.Release(ctx, t, leg.SupplierID, leg.HoldID, "procurement-exit-cancel:"+leg.ID); e != nil {
			return e
		}
		if e := p.Capacity.ReleaseReceiptRefund(ctx, t, leg.SupplierID, leg.ReceiptID, leg.ID); e != nil {
			return e
		}
		return t.Model(&leg).Update("state", "released_unpaid").Error
	})
}
