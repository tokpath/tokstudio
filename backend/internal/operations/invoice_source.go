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
	RemovedUSD     ledger.Decimal `json:"removed_usd"`
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
	if a.RemovedUSD.Sign() < 0 || !a.RemovedUSD.Valid() {
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
	if len(parties) != 1 || parties[0].ReceiptID != receipt || parties[0].RemovedUSD.Sign() < 0 {
		return out, ErrNotReady
	}
	party := parties[0]
	global := basis.GrossUSD.Sub(a.RemovedUSD)
	if global.Sign() < 0 {
		global = ledger.Zero()
	}
	out.GlobalEligibleUSD = &global
	out.EligibleUSD = basis.AllocatedUSD.Sub(party.RemovedUSD)
	if out.EligibleUSD.Sign() < 0 {
		out.EligibleUSD = ledger.Zero()
	}
	out.ReceiptUSD = &out.EligibleUSD
	capStage := func(v ledger.Decimal) ledger.Decimal {
		if v.Cmp(out.EligibleUSD) > 0 {
			return out.EligibleUSD
		}
		return v
	}
	out.TopUpUSD = capStage(basis.TopUpUSD)
	out.PurchaseUSD = capStage(basis.PurchaseUSD)
	out.ConsumptionUSD = capStage(basis.ConsumptionUSD)
	out.ReviewRequired = a.ReviewRequired || party.ReviewRequired
	out.AdjustmentReference = a.Reference
	b, _ := json.Marshal(map[string]any{"commerce": basis, "adjustment": a, "party_adjustment": party})
	out.Evidence = string(b)
	return out, nil
}
