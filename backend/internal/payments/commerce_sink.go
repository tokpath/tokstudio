package payments

import (
	"context"
	"encoding/json"

	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"gorm.io/gorm"
)

// CommerceSink records the verified gross value only. Unknown merchant fees
// remain unknown; reliable provider settlement facts use commerce accounting.
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
	fee := ledger.Zero()
	if o.FeeQuote != nil && value.Amount.Cmp(o.FeeQuote.TotalMoney) == 0 {
		fee = o.FeeQuote.FeeMoney
	}
	receipt, e := s.Commerce.RecordReceipt(ctx, tx, commerce.VerifiedReceipt{PaymentFeeMoney: fee, OperatorID: f.OperatorID, Provider: f.Driver + ":" + m.ID, ProviderReceiptID: f.ProviderID, GrossMoney: value.Amount, FeeStatus: "pending", Currency: f.Money.Currency, OriginalAmount: f.Money.Amount.String(), RecipientReference: reference, PayerReference: payer, ValuationEvidence: string(valuation), VerificationEvidence: string(f.Evidence), VerifiedBy: verifiedBy, PaidAt: f.OccurredAt, ObservedAt: f.ObservedAt})
	if e != nil {
		return "", e
	}
	if e = s.ConfirmVerifiedReceiptFee(ctx, tx, receipt.ID, f); e != nil {
		return "", e
	}
	return receipt.ID, nil
}

// A provider fee is an independent fact, never the configured customer charge.
func (s CommerceSink) ConfirmVerifiedReceiptFee(ctx context.Context, tx *gorm.DB, id string, f Fact) error {
	if !f.Verified() {
		return ErrInvalid
	}
	if f.MerchantNet != nil {
		if e := s.Commerce.RecordReceiptSettlement(ctx, tx, id, f.MerchantNet.Currency, f.MerchantNet.Amount, "adapter:"+f.Driver); e != nil {
			return e
		}
	}
	if f.MerchantFee == nil || !f.Verified() {
		return nil
	}
	r, e := s.Commerce.GetReceipt(ctx, tx, f.OperatorID, id)
	if e != nil {
		return e
	}
	if f.MerchantFee.Currency != r.Currency {
		return nil
	}
	if f.MerchantFee.Amount.Sign() < 0 || !f.MerchantFee.Amount.Valid() {
		return ErrInvalid
	}
	original, e := ledger.ParseDecimal(r.OriginalAmount)
	if e != nil {
		return e
	}
	// Some providers settle an additional charge separately when costs exceed
	// this receipt. Preserve the provider fact without blocking the real receipt
	// or pretending that its net settlement can be represented by this model.
	if f.MerchantFee.Amount.Cmp(original) > 0 {
		return nil
	}
	_, e = s.Commerce.RecordReceiptAccounting(ctx, tx, commerce.ReceiptAccounting{ReceiptID: id, OriginalFeeAmount: &f.MerchantFee.Amount, Evidence: string(f.Evidence), VerifiedBy: "adapter:" + f.Driver})
	return e
}
