package payments

import (
	"context"
	"encoding/json"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"strings"
	"time"
)

type Manual struct{}

func (Manual) Name() string { return "manual" }
func (Manual) Validate(m Merchant) []string {
	missing := required(m, "instructions", "currency")
	currency := m.Credentials["currency"]
	if currency != "" && (currency != strings.ToUpper(currency) || (Money{Currency: currency, Amount: ledger.FromInt64(1)}).Validate() != nil) {
		missing = append(missing, "currency_uppercase_code")
	}
	return missing
}
func (d Manual) Create(_ context.Context, m Merchant, o Order) (Checkout, error) {
	if e := ready(m, d); e != nil {
		return Checkout{}, e
	}
	if o.Validate(m) != nil || o.Money.Currency != m.Credentials["currency"] {
		return Checkout{}, ErrInvalid
	}
	settlement := o.Settlement()
	checkout := Checkout{State: "pending", PaymentAddress: m.Credentials["recipient_reference"], Instructions: m.Credentials["instructions"], SettlementMoney: &settlement}
	if o.Money.Currency == settlement.Currency {
		checkout.PayMoney = &o.Money
	}
	return checkout, nil
}
func (Manual) Lookup(context.Context, Merchant, Order) (Fact, error) { return Fact{}, ErrNotFinal }
func (Manual) Refund(context.Context, Merchant, Refund) (RefundStatus, error) {
	return RefundStatus{}, ErrUnsupported
}
func (Manual) LookupRefund(context.Context, Merchant, Refund) (RefundStatus, error) {
	return RefundStatus{}, ErrUnsupported
}
func (Manual) VerifyWebhook(context.Context, Merchant, Webhook) (Hint, error) {
	return Hint{}, ErrUnsupported
}

type ManualVerification struct {
	ExternalReference string         `json:"external_reference"`
	Money             Money          `json:"money"`
	PaidAt            time.Time      `json:"paid_at"`
	EvidenceReference string         `json:"evidence_reference"`
	PayerReference    string         `json:"payer_reference"`
	SettlementAmount  ledger.Decimal `json:"settlement_amount,omitempty"`
}

func (s *Service) RecordManual(ctx context.Context, actor, operator, merchant, orderID string, v ManualVerification) (Fact, error) {
	if s.Authorize == nil {
		return Fact{}, ErrForbidden
	}
	if e := s.Authorize.RequirePermission(ctx, s.DB, actor, operator, "payments.manage"); e != nil {
		return Fact{}, e
	}
	m, e := s.Merchant(ctx, operator, merchant)
	if e != nil {
		return Fact{}, e
	}
	now, e := databaseTime(ctx, s.DB)
	if e != nil {
		return Fact{}, e
	}
	if m.Driver != "manual" || v.PaidAt.IsZero() || v.PaidAt.After(now.Add(5*time.Minute)) {
		return Fact{}, ErrInvalid
	}
	if strings.TrimSpace(v.ExternalReference) == "" {
		v.ExternalReference = ManualReference("receipt", merchant, orderID)
	}
	o, e := s.loadOrder(ctx, m, orderID)
	if e != nil {
		return Fact{}, e
	}
	if !v.SettlementAmount.Valid() || v.SettlementAmount.Sign() < 0 || (v.Money.Currency == o.Settlement().Currency && !v.SettlementAmount.IsZero() && v.SettlementAmount.Cmp(v.Money.Amount) != 0) {
		return Fact{}, ErrInvalid
	}
	fact, e := receiptFact(m, o, v.ExternalReference, v.Money, &v.PaidAt, map[string]string{"verified_by": "staff:" + actor, "evidence_reference": v.EvidenceReference, "payer_reference": v.PayerReference, "recipient_reference": m.Credentials["recipient_reference"]})
	if e != nil {
		return Fact{}, e
	}
	if v.Money.Currency != o.Settlement().Currency && v.SettlementAmount.Sign() > 0 {
		value, e := (Valuation{Currency: o.Settlement().Currency, Amount: v.SettlementAmount, Source: v.EvidenceReference}).Convert(v.Money, time.Time{})
		if e != nil {
			return Fact{}, e
		}
		value.VerifiedBy = "staff:" + actor
		fact.ExecutionValue = &value
	}
	fact.ID = manualBankFactID("receipt", ManualAccountScope(operator, m.Credentials["recipient_reference"]), v.ExternalReference)
	e = s.RecordFact(ctx, m, o, fact)
	return fact, e
}

// Internal references identify a staff-confirmed cash/offline payment without
// inventing a bank transaction or requiring an account that does not exist.
func ManualReference(kind, scope, key string) string {
	return "manual:" + kind + ":" + fingerprint([]string{scope, key})
}

func ManualAccountScope(operator, account string) string {
	if value := strings.TrimSpace(account); value != "" {
		return value
	}
	return "operator:" + operator
}
func manualEvidence(f Fact) map[string]string {
	var evidence map[string]string
	_ = json.Unmarshal(f.Evidence, &evidence)
	return evidence
}
