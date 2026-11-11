package payments

import (
	"context"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"gorm.io/gorm"
	"strings"
	"time"
)

// responsibleRefundContext never opens the old merchant. The business owner
// proves the assumed obligation and original receipt relationship; only the
// current payer's independently configured account is available to this module.
func (s *Service) responsibleRefundContext(ctx context.Context, actor, op, merchant, driver string, r Refund, d ExternalRefundDestination) (Merchant, Refund, ResponsibleRefundBasis, error) {
	var m Merchant
	var basis ResponsibleRefundBasis
	authority, ok := s.RefundApproval.(ResponsibleRefundAuthority)
	if !ok || s.Authorize == nil || s.RefundSink == nil {
		return m, r, basis, ErrNotConfigured
	}
	if e := s.Authorize.RequirePermission(ctx, s.DB, actor, op, "payments.manage"); e != nil {
		return m, r, basis, e
	}
	var e error
	m, e = s.Merchant(ctx, op, merchant)
	if e != nil {
		return m, r, basis, e
	}
	if m.Driver != driver {
		return m, r, basis, ErrUnsupported
	}
	m.Enabled = true // This records an independently authorized transfer already executed.
	if d, e = NormalizeExternalRefundDestination(d); e != nil {
		return m, r, basis, e
	}
	e = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var e error
		basis, e = authority.RequireResponsibleRefundExecution(ctx, tx, actor, op, r.ID, d)
		return e
	})
	if e != nil {
		return m, r, basis, e
	}
	if basis.RefundID != r.ID || basis.OperatorID != op || basis.SourceOperatorID == "" || basis.ReceiptID == "" || basis.Currency != d.Asset || basis.AmountMoney.Sign() <= 0 || !basis.AmountMoney.Valid() {
		return m, r, basis, ErrConflict
	}
	if basis.ExecutionMerchantID != "" && basis.ExecutionMerchantID != merchant {
		return m, r, basis, ErrForbidden
	}
	if driver == "manual" {
		if d.Network != "bank" || d.TokenContract != "" || m.Credentials["currency"] != basis.Currency || m.Credentials["recipient_reference"] == "" {
			return m, r, basis, ErrConflict
		}
	} else {
		return m, r, basis, ErrUnsupported
	}
	at := r.ValuedAt
	if at.IsZero() {
		at = basis.AuthorizedAt
		if at.IsZero() {
			at = basis.CreatedAt
		}
		if at.IsZero() {
			return m, r, basis, ErrInvalid
		}
	}
	settings, e := ledger.GetMonetary(ctx, s.DB)
	if e != nil {
		return m, r, basis, e
	}
	// The real transfer is independently read or verified before it is recorded.
	r.Order = Order{ID: r.ID, ContextKind: "refund_execution", OperatorID: op, MerchantID: m.ID, Money: Money{Currency: basis.Currency}, SettlementMoney: Money{Currency: settings.Currency, Amount: basis.AmountMoney}, AcceptedAt: at, ExpiresAt: at.Add(time.Hour)}
	if basis.Currency == settings.Currency {
		r.Order.Money.Amount = basis.AmountMoney
	}
	r.MoneyAmount = basis.AmountMoney
	return m, r, basis, nil
}

func (s *Service) RecordResponsibleBankRefund(ctx context.Context, actor, op, merchant string, r Refund, v ExternalRefundVerification) (RefundStatus, error) {
	return s.recordResponsibleExternal(ctx, actor, op, merchant, "manual", r, v)
}
func (s *Service) recordResponsibleExternal(ctx context.Context, actor, op, merchant, driver string, r Refund, v ExternalRefundVerification) (RefundStatus, error) {
	d, e := NormalizeExternalRefundDestination(v.Destination)
	if e != nil {
		return RefundStatus{}, e
	}
	m, r, basis, e := s.responsibleRefundContext(ctx, actor, op, merchant, driver, r, d)
	if e != nil {
		return RefundStatus{}, e
	}
	now, e := databaseTime(ctx, s.DB)
	if e != nil {
		return RefundStatus{}, e
	}
	if v.ReceiptID != "" && v.ReceiptID != basis.ReceiptID || strings.TrimSpace(v.ExternalReference) == "" || v.EvidenceReference == "" || v.ExecutedAt.IsZero() || v.ExecutedAt.After(now.Add(5*time.Minute)) || v.Money.Validate() != nil || v.Money.Currency != basis.Currency {
		return RefundStatus{}, ErrInvalid
	}
	r.Money = v.Money
	r.ExecutionID = "external:" + fingerprint([]string{r.ID, driver, d.Network, v.ExternalReference})
	if e = externalValuationAt(r, &v.ExecutedAt); e != nil {
		return RefundStatus{}, e
	}
	f, e := refundFact(m, r, v.ExternalReference, v.Money, &v.ExecutedAt, map[string]any{"responsibility_reference": r.ID, "source_operator_id": basis.SourceOperatorID, "original_receipt_id": basis.ReceiptID, "actual_execution_evidence": v.EvidenceReference, "verified_by": "staff:" + actor})
	if e != nil {
		return RefundStatus{}, e
	}
	f.ID = "external:refund:" + fingerprint([]string{driver, m.Credentials["account_id"], d.Network, d.Asset, strings.ToLower(v.ExternalReference)})
	method := "finance_verified_external_record"
	sender := m.Credentials["account_id"]
	if driver == "manual" {
		f.ID = manualBankFactID("refund", m.Credentials["recipient_reference"], v.ExternalReference)
		method = "finance_verified_bank_record"
		sender = m.Credentials["recipient_reference"]
	}
	f.RefundSource = &RefundSource{SourceOperatorID: basis.SourceOperatorID, ReceiptID: basis.ReceiptID, ResponsibilityReference: r.ID}
	f.ExternalRefund = &ExternalRefundExecution{Destination: d, SenderAddress: sender, RecipientAddress: d.Address, VerificationMethod: method, VerifiedBy: "staff:" + actor}
	return s.recordExternalRefund(ctx, actor, m, r, f, d, fingerprint(v), nil)
}
