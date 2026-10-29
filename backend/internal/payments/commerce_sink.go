package payments

import (
	"context"
	"encoding/json"

	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"gorm.io/gorm"
)

// CommerceSink records the verified gross value only. Unknown merchant fees
// remain explicitly pending; later settlement facts use commerce accounting.
// Allocation and fulfillment are separate commands owned by commerce.
type CommerceSink struct{ Commerce *commerce.Service }

func (s CommerceSink) UnsettledReceiptFunds(ctx context.Context, tx *gorm.DB, operator string, receiptIDs []string) ([]commerce.ReceiptFundObligation, error) {
	if s.Commerce == nil {
		return nil, ErrNotConfigured
	}
	return s.Commerce.UnsettledReceiptFunds(ctx, tx, operator, receiptIDs)
}

func (s CommerceSink) ConfirmVerifiedPaymentTime(ctx context.Context, tx *gorm.DB, receipt string, f Fact) error {
	if s.Commerce == nil || !f.Verified() || f.OccurredAt == nil {
		return ErrInvalid
	}
	_, e := s.Commerce.ConfirmReceiptPaymentTime(ctx, tx, receipt, *f.OccurredAt, string(f.Evidence), "adapter:"+f.Driver)
	return e
}

func (s CommerceSink) RecordVerifiedReceipt(ctx context.Context, tx *gorm.DB, m Merchant, o Order, f Fact, value MoneyValue) (string, error) {
	if s.Commerce == nil || !f.Verified() {
		return "", ErrNotConfigured
	}
	valuation, _ := json.Marshal(value)
	reference := "merchant:" + m.ID
	verifiedBy := "adapter:" + m.Driver
	payer := "provider_payment:" + f.ProviderID
	if m.Driver == "manual" {
		e := manualEvidence(f)
		reference = e["recipient_reference"]
		payer = e["payer_reference"]
		verifiedBy = e["verified_by"]
	}
	receipt, e := s.Commerce.RecordReceipt(ctx, tx, commerce.VerifiedReceipt{OperatorID: f.OperatorID, Provider: f.Driver + ":" + m.ID, ProviderReceiptID: f.ProviderID, GrossMoney: value.Amount, FeeStatus: "pending", Currency: f.Money.Currency, OriginalAmount: f.Money.Amount.String(), RecipientReference: reference, PayerReference: payer, ValuationEvidence: string(valuation), VerificationEvidence: string(f.Evidence), VerifiedBy: verifiedBy, PaidAt: f.OccurredAt, ObservedAt: f.ObservedAt})
	return receipt.ID, e
}
