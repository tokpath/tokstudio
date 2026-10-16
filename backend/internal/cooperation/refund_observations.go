package cooperation

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/payments"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strings"
	"time"
)

type ProcurementRefundObservation struct {
	ID                string          `json:"id"`
	LegID             string          `json:"leg_id"`
	FactID            string          `json:"fact_id"`
	ActualUSD         ledger.Decimal  `json:"actual_usd"`
	SettledUSD        ledger.Decimal  `json:"settled_usd"`
	ExcessUSD         ledger.Decimal  `json:"excess_usd"`
	RecipientMatches  bool            `json:"recipient_matches"`
	Money             json.RawMessage `json:"money" gorm:"type:jsonb"`
	Valuation         json.RawMessage `json:"valuation" gorm:"type:jsonb"`
	ExternalExecution json.RawMessage `json:"external_execution,omitempty" gorm:"type:jsonb"`
	OccurredAt        *time.Time      `json:"occurred_at,omitempty"`
	CreatedAt         time.Time       `json:"created_at"`
}

func (ProcurementRefundObservation) TableName() string {
	return "cooperation_procurement_refund_observations"
}

func (p *ProcurementRefunds) recordRefundObservation(ctx context.Context, tx *gorm.DB, m payments.Merchant, o payments.Order, r payments.Refund, f payments.Fact, value payments.USDValue) error {
	if tx == nil || !p.Ready() || p.ObserveReceiptRefund == nil {
		return ErrReplenishment
	}
	var leg ProcurementExitRefund
	q := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=?", r.ID).Limit(1).Find(&leg)
	if q.Error != nil {
		return q.Error
	}
	if q.RowsAffected == 0 {
		return ErrNotProcurementRefund
	}
	if !f.Verified() || f.Kind != "refund" || f.OperatorID != leg.SupplierID || f.OrderID != o.ID || r.USDAmount.Cmp(leg.AmountUSD) != 0 || !value.Amount.Valid() || value.Amount.Sign() < 0 {
		return ErrRefundFX
	}
	if source := f.RefundSource; source != nil {
		if o.ContextKind != "refund_execution" || o.ID != leg.ID || source.SourceOperatorID != leg.SupplierID || source.ReceiptID != leg.ReceiptID || source.ResponsibilityReference != leg.ID || f.ExternalRefund == nil {
			return ErrConflict
		}
		var a RefundDestinationAuthorization
		if e := tx.First(&a, "id=? AND leg_id=? AND supplier_id=?", f.ExternalRefund.Destination.AuthorizationReference, leg.ID, leg.SupplierID).Error; e != nil {
			return e
		}
		if a.Version <= 1 || a.ExecutionMerchantID != m.ID {
			return ErrConflict
		}
	} else if m.ID != leg.MerchantID || o.ID != leg.OriginOrderID {
		return ErrRefundFX
	}
	var old ProcurementRefundObservation
	q = tx.Where("fact_id=?", f.ID).Limit(1).Find(&old)
	if q.Error != nil {
		return q.Error
	}
	if q.RowsAffected > 0 {
		if old.LegID != leg.ID || old.ActualUSD.Cmp(value.Amount) != 0 {
			return ErrConflict
		}
		if leg.State != "completed" {
			return errors.Join(ErrRefundFX, payments.ErrSettlementReview)
		}
		return nil
	}
	recipientMatches := true
	if x := f.ExternalRefund; x != nil {
		d, e := p.destinationByID(ctx, tx, leg, x.Destination.AuthorizationReference)
		if e != nil {
			return e
		}
		actual, e := payments.NormalizeExternalRefundDestination(x.Destination)
		if e != nil || actual != d {
			return ErrConflict
		}
		recipientMatches = x.RecipientAddress == d.Address
		if _, ok := newDecimalNetwork(d.Network); ok {
			recipientMatches = strings.EqualFold(x.RecipientAddress, d.Address)
		}
	}
	remaining := leg.AmountUSD.Sub(leg.SettledUSD)
	settled := value.Amount
	if !recipientMatches {
		settled = ledger.Zero()
	}
	if settled.Cmp(remaining) > 0 {
		settled = remaining
	}
	excess := value.Amount.Sub(settled)
	if recipientMatches && value.Amount.Sign() > 0 {
		if e := p.ObserveReceiptRefund(ctx, tx, leg.SupplierID, leg.ReceiptID, leg.ID, f.ID, value.Amount); e != nil {
			return e
		}
	}
	var exit ProcurementExit
	if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&exit, "id=?", leg.ExitID).Error; e != nil {
		return e
	}
	payable, e := p.payable(ctx, tx, exit)
	if e != nil {
		return e
	}
	cash, e := p.Cooperation.Ledger.EnsureAccount(ctx, tx, ledger.Account{OperatorID: exit.SupplierID, OwnerID: exit.SupplierID, Purpose: "control:external_cash", Unit: "USD", AllowNegative: true})
	if e != nil {
		return e
	}
	lines := []ledger.Line{}
	if settled.Sign() > 0 {
		lines = append(lines, ledger.Line{AccountID: payable.ID, Amount: settled.Neg()})
	}
	if value.Amount.Sign() > 0 {
		lines = append(lines, ledger.Line{AccountID: cash.ID, Amount: value.Amount})
	}
	if excess.Sign() > 0 {
		loss, e := p.Cooperation.Ledger.EnsureAccount(ctx, tx, ledger.Account{OperatorID: exit.SupplierID, OwnerID: exit.SupplierID, Purpose: refundLossPurpose(recipientMatches), Unit: "USD", AllowNegative: true})
		if e != nil {
			return e
		}
		lines = append(lines, ledger.Line{AccountID: loss.ID, Amount: excess.Neg()})
	}
	posting := ledger.Posting{OperatorID: exit.SupplierID, BusinessKey: "procurement-refund-observation:" + f.ID, Kind: "procurement_exit_refund", SourceID: leg.ID, ActorID: "adapter:" + f.Driver, Reason: string(f.Evidence), Lines: lines}
	if len(lines) > 0 {
		if settled.Sign() > 0 {
			if _, e = p.Cooperation.Ledger.Capture(ctx, tx, leg.HoldID, posting, nil); e != nil {
				return e
			}
		} else if _, e = p.Cooperation.Ledger.Post(ctx, tx, posting); e != nil {
			return e
		}
	}
	newRemaining := remaining.Sub(settled)
	holdID := leg.HoldID
	if settled.Sign() > 0 && newRemaining.Sign() > 0 {
		now, e := businessTime(ctx, tx)
		if e != nil {
			return e
		}
		h, e := p.Cooperation.Ledger.Reserve(ctx, tx, ledger.Reservation{OperatorID: leg.SupplierID, BusinessKey: "procurement-refund-remainder:" + f.ID, Purpose: "procurement_exit_refund", ExpiresAt: now.Add(365 * 24 * time.Hour), Amounts: []ledger.Line{{AccountID: payable.ID, Amount: newRemaining}}})
		if e != nil {
			return e
		}
		holdID = h.ID
	}
	if recipientMatches && value.Amount.Sign() > 0 {
		receivable, e := p.Cooperation.Ledger.EnsureAccount(ctx, tx, ledger.Account{OperatorID: exit.BuyerID, OwnerID: exit.ID, Purpose: "procurement_exit:receivable", Unit: "USD"})
		if e != nil {
			return e
		}
		received, e := p.Cooperation.Ledger.EnsureAccount(ctx, tx, ledger.Account{OperatorID: exit.BuyerID, OwnerID: exit.BuyerID, Purpose: "control:external_receipt", Unit: "USD", AllowNegative: true})
		if e != nil {
			return e
		}
		buyerLines := []ledger.Line{{AccountID: received.ID, Amount: value.Amount}}
		if settled.Sign() > 0 {
			buyerLines = append(buyerLines, ledger.Line{AccountID: receivable.ID, Amount: settled.Neg()})
		}
		if excess.Sign() > 0 {
			unmatched, e := p.Cooperation.Ledger.EnsureAccount(ctx, tx, ledger.Account{OperatorID: exit.BuyerID, OwnerID: exit.BuyerID, Purpose: "control:procurement_refund_excess", Unit: "USD", AllowNegative: true})
			if e != nil {
				return e
			}
			buyerLines = append(buyerLines, ledger.Line{AccountID: unmatched.ID, Amount: excess.Neg()})
		}
		if _, e = p.Cooperation.Ledger.Post(ctx, tx, ledger.Posting{OperatorID: exit.BuyerID, BusinessKey: "procurement-refund-received:" + f.ID, Kind: "procurement_exit_refund_received", SourceID: leg.ID, ActorID: "adapter:" + f.Driver, Reason: string(f.Evidence), Lines: buyerLines}); e != nil {
			return e
		}
	}
	now, e := businessTime(ctx, tx)
	if e != nil {
		return e
	}
	money, _ := json.Marshal(f.Money)
	valuation, _ := json.Marshal(value)
	var external json.RawMessage
	if f.ExternalRefund != nil {
		external, _ = json.Marshal(f.ExternalRefund)
	}
	observation := ProcurementRefundObservation{ID: uuid.NewString(), LegID: leg.ID, FactID: f.ID, ActualUSD: value.Amount, SettledUSD: settled, ExcessUSD: excess, RecipientMatches: recipientMatches, Money: money, Valuation: valuation, ExternalExecution: external, OccurredAt: f.OccurredAt, CreatedAt: now}
	if e = tx.Create(&observation).Error; e != nil {
		return e
	}
	state := "completed"
	if newRemaining.Sign() > 0 || excess.Sign() > 0 || value.RemainderNumerator != "0" {
		state = "exception"
	}
	if e = tx.Model(&leg).Updates(map[string]any{"state": state, "hold_id": holdID, "settled_usd": leg.SettledUSD.Add(settled), "actual_usd": leg.ActualUSD.Add(value.Amount), "excess_usd": leg.ExcessUSD.Add(excess), "verified_fact_id": f.ID, "execution_valuation": string(valuation)}).Error; e != nil {
		return e
	}
	paid := exit.RefundedUSD.Add(settled)
	exitState := "awaiting_refund"
	if paid.Cmp(exit.ContractUSD) == 0 {
		exitState = "awaiting_precision"
		if exit.ContractRemainderNumerator == "0" {
			exitState = "settled"
		}
	}
	if state == "exception" {
		exitState = "refund_exception"
	}
	if e = tx.Model(&exit).Updates(map[string]any{"refunded_usd": paid, "state": exitState}).Error; e != nil {
		return e
	}
	if state == "exception" {
		return errors.Join(ErrRefundFX, payments.ErrSettlementReview)
	}
	return nil
}

func newDecimalNetwork(s string) (uint64, bool) {
	var n uint64
	if s == "" {
		return 0, false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, false
		}
		n = n*10 + uint64(r-'0')
	}
	return n, true
}

func refundLossPurpose(recipientMatches bool) string {
	if recipientMatches {
		return "control:refund_overpayment"
	}
	return "control:refund_misdirected"
}
