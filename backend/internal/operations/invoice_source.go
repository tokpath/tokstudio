package operations

import (
	"context"
	"encoding/json"
	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"gorm.io/gorm"
)

// ReceiptAdjustment is a read projection, not permission to execute a refund.
// Unresolved disputes preserve the invoice for review until the financial owner
// supplies verified outcomes.
type ReceiptAdjustment struct {
	ReceiptID      string         `json:"receipt_id"`
	RemovedMoney     ledger.Decimal `json:"removed_money"`
	ReviewRequired bool           `json:"review_required"`
	Reference      string         `json:"reference"`
}
type ReceiptAdjustments interface {
	InvoiceReceiptAdjustments(context.Context, *gorm.DB, string, []string) ([]ReceiptAdjustment, error)
}
type PartyReceiptAdjustments interface {
	InvoicePartyReceiptAdjustments(context.Context, *gorm.DB, string, string, []string) ([]ReceiptAdjustment, error)
}
type CommerceInvoiceSource struct {
	Commerce    *commerce.Service
	Adjustments ReceiptAdjustments
}

func (d CommerceInvoiceSource) InvoiceBasis(ctx context.Context, tx *gorm.DB, operator, customer, receipt string) (InvoiceBasis, error) {
	out := InvoiceBasis{ReceiptID: receipt}
	if d.Commerce == nil || d.Adjustments == nil {
		return out, ErrNotReady
	}
	basis, e := d.Commerce.InvoiceReceiptBasis(ctx, tx, operator, customer, receipt)
	if e != nil {
		return out, e
	}
	adjustments, e := d.Adjustments.InvoiceReceiptAdjustments(ctx, tx, operator, []string{receipt})
	if e != nil {
		return out, e
	}
	if len(adjustments) != 1 || adjustments[0].ReceiptID != receipt {
		return out, ErrNotReady
	}
	a := adjustments[0]
	if a.RemovedMoney.Sign() < 0 || !a.RemovedMoney.Valid() {
		return out, ErrInvalid
	}
	partySource, ok := d.Adjustments.(PartyReceiptAdjustments)
	if !ok {
		return out, ErrNotReady
	}
	parties, e := partySource.InvoicePartyReceiptAdjustments(ctx, tx, operator, customer, []string{receipt})
	if e != nil {
		return out, e
	}
	if len(parties) != 1 || parties[0].ReceiptID != receipt || parties[0].RemovedMoney.Sign() < 0 {
		return out, ErrNotReady
	}
	party := parties[0]
	global := basis.GrossMoney.Sub(a.RemovedMoney)
	if global.Sign() < 0 {
		global = ledger.Zero()
	}
	out.GlobalEligibleMoney = &global
	out.EligibleMoney = basis.AllocatedMoney.Sub(party.RemovedMoney)
	if out.EligibleMoney.Sign() < 0 {
		out.EligibleMoney = ledger.Zero()
	}
	out.ReceiptMoney = &out.EligibleMoney
	capStage := func(v ledger.Decimal) ledger.Decimal {
		if v.Cmp(out.EligibleMoney) > 0 {
			return out.EligibleMoney
		}
		return v
	}
	out.TopUpMoney = capStage(basis.TopUpMoney)
	out.PurchaseMoney = capStage(basis.PurchaseMoney)
	out.ConsumptionMoney = capStage(basis.ConsumptionMoney)
	out.ReviewRequired = a.ReviewRequired || party.ReviewRequired
	out.AdjustmentReference = a.Reference
	b, _ := json.Marshal(map[string]any{"commerce": basis, "adjustment": a, "party_adjustment": party})
	out.Evidence = string(b)
	return out, nil
}
