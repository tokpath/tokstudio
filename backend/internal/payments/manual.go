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
	missing := required(m, "recipient_reference", "instructions", "currency")
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
	return Checkout{State: "pending", PaymentAddress: m.Credentials["recipient_reference"], Instructions: m.Credentials["instructions"], PayMoney: &o.Money}, nil
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
	ExternalReference string    `json:"external_reference"`
	Money             Money     `json:"money"`
	PaidAt            time.Time `json:"paid_at"`
	EvidenceReference string    `json:"evidence_reference"`
	PayerReference    string    `json:"payer_reference"`
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
	if m.Driver != "manual" || v.ExternalReference == "" || v.PaidAt.IsZero() || v.PaidAt.After(now.Add(5*time.Minute)) || v.EvidenceReference == "" || v.PayerReference == "" {
		return Fact{}, ErrInvalid
	}
	o, e := s.loadOrder(ctx, m, orderID)
	if e != nil {
		return Fact{}, e
	}
	fact, e := receiptFact(m, o, v.ExternalReference, v.Money, &v.PaidAt, map[string]string{"verified_by": "staff:" + actor, "evidence_reference": v.EvidenceReference, "payer_reference": v.PayerReference, "recipient_reference": m.Credentials["recipient_reference"]})
	if e != nil {
		return Fact{}, e
	}
	fact.ID = manualBankFactID("receipt", m.Credentials["recipient_reference"], v.ExternalReference)
	e = s.RecordFact(ctx, m, o, fact)
	return fact, e
}
func manualEvidence(f Fact) map[string]string {
	var evidence map[string]string
	_ = json.Unmarshal(f.Evidence, &evidence)
	return evidence
}
