package payments

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"math/big"
	"strings"
	"time"
	"unicode"

	"gorm.io/gorm"
)

type ExternalRefundDestination struct {
	Network                string `json:"network"`
	Asset                  string `json:"asset"`
	TokenContract          string `json:"token_contract"`
	Address                string `json:"address"`
	AuthorizationReference string `json:"authorization_reference"`
	VerificationEvidence   string `json:"verification_evidence"`
}
type ExternalRefundExecution struct {
	Destination        ExternalRefundDestination `json:"destination"`
	TransactionHash    string                    `json:"transaction_hash,omitempty"`
	LogIndex           string                    `json:"log_index,omitempty"`
	BlockHash          string                    `json:"block_hash,omitempty"`
	Confirmations      string                    `json:"confirmations,omitempty"`
	SenderAddress      string                    `json:"sender_address,omitempty"`
	RecipientAddress   string                    `json:"recipient_address"`
	VerificationMethod string                    `json:"verification_method"`
	VerifiedBy         string                    `json:"verified_by"`
}
type EVMRefundVerification struct {
	ReceiptID       string                    `json:"receipt_id"`
	TransactionHash string                    `json:"transaction_hash"`
	LogIndex        string                    `json:"log_index"`
	Destination     ExternalRefundDestination `json:"destination"`
}
type ExternalRefundVerification struct {
	ReceiptID         string                    `json:"receipt_id"`
	ExternalReference string                    `json:"external_reference"`
	Money             Money                     `json:"money"`
	ExecutedAt        time.Time                 `json:"executed_at"`
	Destination       ExternalRefundDestination `json:"destination"`
	EvidenceReference string                    `json:"evidence_reference"`
}
type ExternalRefundApproval interface {
	RequireExternalRefundFact(context.Context, *gorm.DB, string, Refund, ExternalRefundDestination) error
}

type ResponsibleRefundBasis struct {
	RefundID, OperatorID, SourceOperatorID, ReceiptID, Currency string
	ExecutionMerchantID                                         string
	AmountUSD, RemainingUSD                                     ledger.Decimal
	AuthorizedAt, CreatedAt                                     time.Time
}
type RefundSource struct {
	SourceOperatorID        string `json:"source_operator_id"`
	ReceiptID               string `json:"receipt_id"`
	ResponsibilityReference string `json:"responsibility_reference"`
}
type ResponsibleRefundAuthority interface {
	RequireResponsibleRefundExecution(context.Context, *gorm.DB, string, string, string, ExternalRefundDestination) (ResponsibleRefundBasis, error)
}

func NormalizeExternalRefundDestination(d ExternalRefundDestination) (ExternalRefundDestination, error) {
	d.Network = strings.ToLower(strings.TrimSpace(d.Network))
	d.Asset = strings.ToUpper(strings.TrimSpace(d.Asset))
	d.Address = strings.TrimSpace(d.Address)
	d.TokenContract = strings.TrimSpace(d.TokenContract)
	if d.Network == "" || len(d.Network) > 64 || d.Asset == "" || len(d.Asset) > 32 || d.Address == "" || len(d.Address) > 256 || len(d.TokenContract) > 256 || strings.TrimSpace(d.AuthorizationReference) == "" || len(d.AuthorizationReference) > 1024 || strings.TrimSpace(d.VerificationEvidence) == "" || len(d.VerificationEvidence) > 2048 {
		return d, ErrInvalid
	}
	for _, v := range []string{d.Network, d.Asset, d.Address, d.TokenContract} {
		for _, r := range v {
			if unicode.IsSpace(r) || unicode.IsControl(r) {
				return d, ErrInvalid
			}
		}
	}
	if n, ok := new(big.Int).SetString(d.Network, 10); ok {
		if n.Sign() <= 0 || !validHex(d.Address, 20) || !validHex(d.TokenContract, 20) {
			return d, ErrInvalid
		}
		d.Network = n.String()
		d.Address = strings.ToLower(d.Address)
		d.TokenContract = strings.ToLower(d.TokenContract)
	}
	return d, nil
}

// ValidateExternalRefundDestinationForSource binds an authorization to the
// original asset and network. NOWPayments currency tickers are provider codes
// (e.g. USDTTRC20), not a guessed base symbol; operators must separately record
// verified provider asset metadata before enabling external returns.
func ValidateExternalRefundDestinationForSource(m Merchant, source PaymentSource, d ExternalRefundDestination) error {
	var e error
	d, e = NormalizeExternalRefundDestination(d)
	if e != nil {
		return e
	}
	if m.ID != source.MerchantID || m.OperatorID != source.OriginalOrder.OperatorID || m.Driver != source.Driver || d.Asset != strings.ToUpper(source.OriginalOrder.Money.Currency) {
		return ErrConflict
	}
	switch m.Driver {
	case "evm_wallet":
		if d.Network != m.Credentials["chain_id"] || !strings.EqualFold(d.TokenContract, m.Credentials["token_contract"]) || d.Asset != strings.ToUpper(m.Credentials["asset"]) {
			return ErrConflict
		}
	case "nowpayments":
		if m.Credentials["pay_network"] == "" || m.Credentials["pay_token_contract"] == "" || m.Credentials["pay_asset_verification_reference"] == "" {
			return ErrNotConfigured
		}
		if d.Asset != strings.ToUpper(m.Credentials["pay_currency"]) || d.Network != strings.ToLower(m.Credentials["pay_network"]) || d.TokenContract != m.Credentials["pay_token_contract"] {
			return ErrConflict
		}
	default:
		return ErrUnsupported
	}
	return nil
}

// externalRefundContext verifies the original merchant/source and durable
// destination instruction before any RPC work. The caller never supplies keys.
func (s *Service) externalRefundContext(ctx context.Context, actor, op, merchant, receipt string, r Refund, d ExternalRefundDestination, driver string) (Merchant, Refund, error) {
	var m Merchant
	approval, ok := s.RefundApproval.(ExternalRefundApproval)
	if s.Authorize == nil || !ok || s.RefundSink == nil {
		return m, r, ErrNotConfigured
	}
	if e := s.Authorize.RequirePermission(ctx, s.DB, actor, op, "payments.manage"); e != nil {
		return m, r, e
	}
	var e error
	m, e = s.Merchant(ctx, op, merchant)
	if e != nil {
		return m, r, e
	}
	if m.Driver != driver {
		return m, r, ErrUnsupported
	}
	source, e := s.PaymentSourceForReceipt(ctx, op, receipt)
	if e != nil {
		return m, r, e
	}
	if source.MerchantID != merchant || source.Driver != driver || source.OriginalOrder.ID != r.Order.ID {
		return m, r, ErrConflict
	}
	r.Order = source.OriginalOrder
	m, e = s.historicalMerchant(ctx, m, r.Order)
	if e != nil {
		return m, r, e
	}
	if e := ValidateExternalRefundDestinationForSource(m, source, d); e != nil {
		return m, r, e
	}
	if d.Asset != strings.ToUpper(r.Order.Money.Currency) || r.USDAmount.Sign() <= 0 || !r.USDAmount.Valid() {
		return m, r, ErrInvalid
	}
	e = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error { return approval.RequireExternalRefundFact(ctx, tx, actor, r, d) })
	return m, r, e
}

// RecordEVMRefund only reads chain state. It never asks a wallet to sign or send.
// The authorized destination is distinct from the original transfer's sender.
func (s *Service) RecordEVMRefund(ctx context.Context, actor, op, merchant string, r Refund, v EVMRefundVerification) (RefundStatus, error) {
	d, e := NormalizeExternalRefundDestination(v.Destination)
	if e != nil {
		return RefundStatus{}, e
	}
	if !validHex(v.TransactionHash, 32) {
		return RefundStatus{}, ErrInvalid
	}
	index, e := hexInt(v.LogIndex)
	if e != nil {
		return RefundStatus{}, e
	}
	m, r, e := s.externalRefundContext(ctx, actor, op, merchant, v.ReceiptID, r, d, "evm_wallet")
	if e != nil {
		return RefundStatus{}, e
	}
	if d.Network != m.Credentials["chain_id"] || !strings.EqualFold(d.TokenContract, m.Credentials["token_contract"]) || d.Asset != strings.ToUpper(m.Credentials["asset"]) {
		return RefundStatus{}, ErrConflict
	}
	driver, ok := s.Drivers["evm_wallet"].(EVMWallet)
	if !ok {
		return RefundStatus{}, ErrNotConfigured
	}
	query := r.Order
	query.ProviderID = strings.ToLower(v.TransactionHash) + "#0x" + index.Text(16)
	transfer, e := driver.lookupTransfer(ctx, m, query, m.Credentials["wallet_address"], false)
	if e != nil {
		return RefundStatus{}, e
	}
	var proof struct {
		BlockHash     string `json:"block_hash"`
		Confirmations string `json:"confirmations"`
		Sender        string `json:"sender"`
		Recipient     string `json:"recipient"`
	}
	if json.Unmarshal(transfer.Evidence, &proof) != nil {
		return RefundStatus{}, ErrProvider
	}
	r.Money = transfer.Money
	r.ExecutionID = "external:" + fingerprint([]string{r.ID, "evm", d.Network, strings.ToLower(v.TransactionHash), index.Text(16)})
	if e = externalValuationAt(r, transfer.OccurredAt); e != nil {
		return RefundStatus{}, e
	}
	f, e := refundFact(m, r, query.ProviderID, transfer.Money, transfer.OccurredAt, map[string]any{"original_receipt_id": v.ReceiptID, "chain_verification": json.RawMessage(transfer.Evidence), "destination": d})
	if e != nil {
		return RefundStatus{}, e
	}
	f.ID = "evm:refund:" + d.Network + ":" + strings.ToLower(v.TransactionHash) + ":" + index.Text(16)
	f.ExternalRefund = &ExternalRefundExecution{Destination: d, TransactionHash: strings.ToLower(v.TransactionHash), LogIndex: "0x" + index.Text(16), BlockHash: proof.BlockHash, Confirmations: proof.Confirmations, SenderAddress: proof.Sender, RecipientAddress: proof.Recipient, VerificationMethod: "read_only_rpc_erc20_transfer", VerifiedBy: "rpc:" + m.ID}
	return s.recordExternalRefund(ctx, actor, m, r, f, d, fingerprint(v), nil)
}

// RecordExternalRefund is the explicitly reviewed external NOWPayments return
// path. It records finance-verified actual execution evidence; the provider's
// create/refund API is never called and this capability is not advertised as an
// automatic refund. Destination authorization is enforced by the business owner.
func (s *Service) RecordExternalRefund(ctx context.Context, actor, op, merchant string, r Refund, v ExternalRefundVerification) (RefundStatus, error) {
	d, e := NormalizeExternalRefundDestination(v.Destination)
	if e != nil {
		return RefundStatus{}, e
	}
	m, r, e := s.externalRefundContext(ctx, actor, op, merchant, v.ReceiptID, r, d, "nowpayments")
	if e != nil {
		return RefundStatus{}, e
	}
	now, e := databaseTime(ctx, s.DB)
	if e != nil {
		return RefundStatus{}, e
	}
	if strings.TrimSpace(v.ExternalReference) == "" || v.EvidenceReference == "" || v.ExecutedAt.IsZero() || v.ExecutedAt.After(now.Add(5*time.Minute)) || v.Money.Validate() != nil || v.Money.Currency != r.Order.Money.Currency {
		return RefundStatus{}, ErrInvalid
	}
	r.Money = v.Money
	r.ExecutionID = "external:" + fingerprint([]string{r.ID, "nowpayments", d.Network, v.ExternalReference})
	if e = externalValuationAt(r, &v.ExecutedAt); e != nil {
		return RefundStatus{}, e
	}
	f, e := refundFact(m, r, v.ExternalReference, v.Money, &v.ExecutedAt, map[string]any{"original_receipt_id": v.ReceiptID, "actual_execution_evidence": v.EvidenceReference, "destination": d, "verified_by": "staff:" + actor})
	if e != nil {
		return RefundStatus{}, e
	}
	f.ID = "external:refund:" + fingerprint([]string{m.Driver, m.Credentials["account_id"], d.Network, d.Asset, strings.ToLower(v.ExternalReference)})
	f.ExternalRefund = &ExternalRefundExecution{Destination: d, RecipientAddress: d.Address, VerificationMethod: "finance_verified_external_record", VerifiedBy: "staff:" + actor}
	return s.recordExternalRefund(ctx, actor, m, r, f, d, fingerprint(v), nil)
}
func externalValuationAt(r Refund, at *time.Time) error {
	if r.Money.Validate() != nil || at == nil || at.IsZero() {
		return ErrInvalid
	}
	if r.Money.Currency == "USD" {
		return nil
	}
	if r.ValuedAt.IsZero() || r.FXExecutionReference == "" || r.RefundValuation.Validate(r.Money.Currency, *at) != nil || r.ValuedAt.Before(at.Add(-5*time.Minute)) || r.ValuedAt.After(at.Add(5*time.Minute)) {
		return ErrInvalid
	}
	return nil
}

func validateExternalExecution(f Fact) error {
	if f.ExternalRefund == nil {
		return nil
	}
	x := f.ExternalRefund
	d, e := NormalizeExternalRefundDestination(x.Destination)
	if e != nil {
		return e
	}
	if x.VerifiedBy == "" || x.RecipientAddress == "" {
		return ErrInvalid
	}
	if _, numeric := new(big.Int).SetString(d.Network, 10); numeric && !validHex(x.RecipientAddress, 20) {
		return ErrInvalid
	}
	return nil
}

func (s *Service) recordExternalRefund(ctx context.Context, actor string, m Merchant, r Refund, f Fact, d ExternalRefundDestination, verificationHash string, mismatch error) (RefundStatus, error) {
	var out RefundStatus
	var settlementErr error
	fp := fingerprint([]string{fingerprint(r), verificationHash})
	e := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := s.Authorize.RequirePermission(ctx, tx, actor, m.OperatorID, "payments.manage"); e != nil {
			return e
		}
		if e := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", m.ID+":refund:"+r.ID).Error; e != nil {
			return e
		}
		var old refundIntentRow
		q := tx.Table("payments_refund_intents").Where("merchant_id=? AND refund_id=?", m.ID, refundExecutionKey(r)).Limit(1).Find(&old)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected > 0 {
			if old.Fingerprint != fp {
				return ErrConflict
			}
			if e := s.open(old.ResultCiphertext, &out); e != nil {
				return e
			}
			if out.Fact == nil || out.Fact.ExecutionValue == nil {
				return ErrInvalid
			}
			out.Fact.proof = true
		} else {
			copy := *s
			copy.DB = tx
			if e := bindRefundFact(tx, m, r, f); e != nil {
				return e
			}
			if e := copy.RecordFact(ctx, m, r.Order, f); e != nil {
				return e
			}
			out = RefundStatus{ProviderID: f.ProviderID, State: "verified_pending_settlement", Fact: &f}
			command, e := s.seal(r)
			if e != nil {
				return e
			}
			result, e := s.seal(out)
			if e != nil {
				return e
			}
			now, e := databaseTime(ctx, tx)
			if e != nil {
				return e
			}
			if e = tx.Table("payments_refund_intents").Create(map[string]any{"merchant_id": m.ID, "refund_id": refundExecutionKey(r), "business_refund_id": r.ID, "operator_id": m.OperatorID, "order_id": r.Order.ID, "fingerprint": fp, "state": "verified_pending_settlement", "refund_ciphertext": command, "provider_id": f.ProviderID, "result_ciphertext": result, "created_at": now, "updated_at": now}).Error; e != nil {
				return e
			}
		}
		if old.State != "succeeded" {
			settlementErr = refundSettlement(tx, func(t *gorm.DB) error {
				if e := validateExternalExecution(*out.Fact); e != nil {
					return e
				}
				if mismatch != nil {
					return fmt.Errorf("external transfer recipient differs from approved destination: %w", mismatch)
				}
				if f.RefundSource != nil {
					authority, ok := s.RefundApproval.(ResponsibleRefundAuthority)
					if !ok {
						return ErrNotConfigured
					}
					basis, e := authority.RequireResponsibleRefundExecution(ctx, t, actor, m.OperatorID, r.ID, d)
					if e != nil {
						return e
					}
					if basis.SourceOperatorID != f.RefundSource.SourceOperatorID || basis.ReceiptID != f.RefundSource.ReceiptID || basis.AmountUSD.Cmp(r.USDAmount) != 0 {
						return ErrConflict
					}
				} else if e := s.RefundApproval.(ExternalRefundApproval).RequireExternalRefundFact(ctx, t, actor, r, d); e != nil {
					return e
				}
				return s.RefundSink.RecordVerifiedRefund(ctx, t, m, r.Order, r, *out.Fact, *out.Fact.ExecutionValue)
			})
		}
		out.State = "succeeded"
		if settlementErr != nil {
			out.State = "verified_pending_settlement"
		}
		encoded, e := s.seal(out)
		if e != nil {
			return e
		}
		return tx.Table("payments_refund_intents").Where("merchant_id=? AND refund_id=?", m.ID, refundExecutionKey(r)).Updates(map[string]any{"state": out.State, "result_ciphertext": encoded, "updated_at": gorm.Expr("clock_timestamp()")}).Error
	})
	if e != nil {
		return out, e
	}
	return out, settlementErr
}
