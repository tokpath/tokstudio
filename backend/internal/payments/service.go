package payments

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"io/fs"
	"math/big"
	"sort"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/ledger"
	platformcrypto "github.com/tokpath/tokstudio/backend/internal/platform/crypto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

//go:embed schema.sql
var schema string

//go:embed schema.sql zz_refund_executions.sql zz_refund_fact_bindings.sql zz_supported_assets.sql zz_refund_closures.sql zzz_receipt_valuations.sql zzzz_receipt_fact_recovery.sql zzzzz_native_settlement.sql zzzzzz_payment_quotes.sql
var migrationFS embed.FS

//go:embed zz_refund_executions.sql
var refundExecutionSchema string

//go:embed zz_refund_fact_bindings.sql
var refundFactBindingSchema string

//go:embed zz_supported_assets.sql
var supportedAssetSchema string

//go:embed zz_refund_closures.sql
var refundClosureSchema string

//go:embed zzz_receipt_valuations.sql
var receiptValuationSchema string

//go:embed zzzz_receipt_fact_recovery.sql
var receiptRecoverySchema string

//go:embed zzzzz_native_settlement.sql
var nativeSettlementSchema string

//go:embed zzzzzz_payment_quotes.sql
var paymentQuoteSchema string

func Migrations() (string, fs.FS) { return "payments", migrationFS }

func Schema() string {
	return schema + "\n" + refundExecutionSchema + "\n" + refundFactBindingSchema + "\n" + supportedAssetSchema + "\n" + refundClosureSchema + "\n" + receiptValuationSchema + "\n" + receiptRecoverySchema + "\n" + nativeSettlementSchema + "\n" + paymentQuoteSchema
}

type Authorizer interface {
	RequirePermission(context.Context, *gorm.DB, string, string, string) error
}
type ReceiptSink interface {
	RecordVerifiedReceipt(context.Context, *gorm.DB, Merchant, Order, Fact, MoneyValue) (string, error)
}
type ReceiptCorrectionSink interface {
	ConfirmVerifiedPaymentTime(context.Context, *gorm.DB, string, Fact) error
}
type RefundApproval interface {
	RequireApprovedRefund(context.Context, *gorm.DB, string, Refund) error
}
type Service struct {
	FXTransport    *Transport
	FXBaseURL      string
	DB             *gorm.DB
	EncryptionKey  string
	Authorize      Authorizer
	Drivers        map[string]Driver
	Sink           ReceiptSink
	RefundApproval RefundApproval
	RefundSink     RefundFactSink
}

func NewService(db *gorm.DB, key string, auth Authorizer, http *Transport, sink ReceiptSink) *Service {
	return &Service{FXTransport: http, DB: db, EncryptionKey: key, Authorize: auth, Drivers: map[string]Driver{"stripe": Stripe{http}, "alipay": Alipay{http}, "wechat": Wechat{http}, "nowpayments": NOWPayments{http}, "evm_wallet": EVMWallet{http}, "manual": Manual{}, "kvoucher": KVoucher{http}}, Sink: sink}
}

type MerchantView struct {
	Merchant
	MissingFields []string `json:"missing_fields"`
	FieldsSet     []string `json:"fields_set"`
}
type merchantRow struct {
	ID, OperatorID, Driver string
	Definition             []byte
	CredentialsCiphertext  string
	Enabled                bool
	Version                uint64
}

func (s *Service) SaveMerchant(ctx context.Context, actor string, m Merchant, expected uint64) (MerchantView, error) {
	if m.Driver == "evm_wallet" && forbiddenWalletSecret(m.Credentials) {
		return MerchantView{}, ErrForbidden
	}
	if s.Authorize == nil || s.EncryptionKey == "" || m.ID == "" || m.OperatorID == "" {
		return MerchantView{}, ErrForbidden
	}
	d, ok := s.Drivers[m.Driver]
	if !ok {
		return MerchantView{}, ErrUnsupported
	}
	view := MerchantView{Merchant: m}
	e := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := s.Authorize.RequirePermission(ctx, tx, actor, m.OperatorID, "payments.manage"); e != nil {
			return e
		}
		if m.Enabled {
			if e := s.applyAssetNetwork(ctx, tx, &m, true); e != nil {
				return e
			}
		}
		view = MerchantView{Merchant: m, MissingFields: d.Validate(m)}
		for k, v := range m.Credentials {
			if v != "" {
				view.FieldsSet = append(view.FieldsSet, k)
			}
		}
		sort.Strings(view.FieldsSet)
		if m.Enabled && len(view.MissingFields) > 0 {
			return ErrNotConfigured
		}
		m.Version = expected + 1
		view.Version = m.Version
		raw, e := json.Marshal(m)
		if e != nil {
			return e
		}
		secret, e := json.Marshal(m.Credentials)
		if e != nil {
			return e
		}
		cipher, e := platformcrypto.Seal(s.EncryptionKey, string(secret))
		if e != nil {
			return e
		}
		fields := map[string]any{"id": m.ID, "operator_id": m.OperatorID, "driver": m.Driver, "definition": string(raw), "credentials_ciphertext": cipher, "enabled": m.Enabled, "version": m.Version}
		if expected > 0 {
			var previous merchantRow
			if e := tx.Table("payments_merchants").Where("id=? AND operator_id=?", m.ID, m.OperatorID).Take(&previous).Error; e != nil {
				return e
			}
			old, e := s.openMerchant(previous)
			if e != nil {
				return e
			}
			for _, key := range []string{"app_id", "seller_id", "mch_id", "account_id", "recipient_reference", "wallet_address", "chain_id", "token_contract", "asset", "pay_currency"} {
				if old.Credentials[key] != m.Credentials[key] {
					return ErrForbidden
				}
			}
			for _, key := range []string{"pay_network", "pay_token_contract"} {
				if old.Credentials[key] != "" && old.Credentials[key] != m.Credentials[key] {
					return ErrForbidden
				}
			}
		}
		if e := tx.Table("payments_merchant_versions").Create(map[string]any{"merchant_id": m.ID, "version": m.Version, "operator_id": m.OperatorID, "definition": string(raw), "credentials_ciphertext": cipher, "created_at": time.Now().UTC()}).Error; e != nil {
			return e
		}
		if expected == 0 {
			return tx.Table("payments_merchants").Create(fields).Error
		}
		delete(fields, "id")
		delete(fields, "operator_id")
		delete(fields, "driver")
		result := tx.Table("payments_merchants").Where("id=? AND operator_id=? AND driver=? AND version=?", m.ID, m.OperatorID, m.Driver, expected).Updates(fields)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrConflict
		}
		return nil
	})
	return view, e
}
func (s *Service) Merchant(ctx context.Context, operator, id string) (Merchant, error) {
	var row merchantRow
	if e := s.DB.WithContext(ctx).Table("payments_merchants").Where("id=? AND operator_id=?", id, operator).Take(&row).Error; e != nil {
		return Merchant{}, e
	}
	return s.openMerchant(row)
}
func (s *Service) merchantByID(ctx context.Context, id string) (Merchant, error) {
	var row merchantRow
	if e := s.DB.WithContext(ctx).Table("payments_merchants").Where("id=?", id).Take(&row).Error; e != nil {
		return Merchant{}, e
	}
	return s.openMerchant(row)
}
func (s *Service) openMerchant(row merchantRow) (Merchant, error) {
	var m Merchant
	if json.Unmarshal(row.Definition, &m) != nil {
		return m, ErrInvalid
	}
	plain, e := platformcrypto.Open(s.EncryptionKey, row.CredentialsCiphertext)
	if e != nil {
		return m, ErrNotConfigured
	}
	if json.Unmarshal([]byte(plain), &m.Credentials) != nil {
		return m, ErrNotConfigured
	}
	return m, nil
}
func (s *Service) seal(value any) (string, error) {
	raw, e := json.Marshal(value)
	if e != nil {
		return "", e
	}
	return platformcrypto.Seal(s.EncryptionKey, string(raw))
}
func (s *Service) open(raw string, value any) error {
	plain, e := platformcrypto.Open(s.EncryptionKey, raw)
	if e != nil {
		return ErrNotConfigured
	}
	if json.Unmarshal([]byte(plain), value) != nil {
		return ErrInvalid
	}
	return nil
}
func fingerprint(value any) string {
	raw, _ := json.Marshal(value)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

type intentRow struct {
	MerchantID, OrderID, OperatorID, Fingerprint, State, OrderCiphertext, CheckoutCiphertext, ProviderID string
	CreatedAt, UpdatedAt                                                                                 time.Time
}

// Create durably claims the logical operation before contacting the provider.
// A timeout/crash stays outcome_unknown; duplicate callers cannot blindly create
// another payment. Verified callbacks or explicit provider lookup reconcile it.
func (s *Service) Create(ctx context.Context, operator, merchant string, o Order) (Checkout, error) {
	if o.ContextKind != "" {
		return Checkout{}, ErrInvalid
	}
	m, e := s.Merchant(ctx, operator, merchant)
	if e != nil {
		return Checkout{}, e
	}
	d, ok := s.Drivers[m.Driver]
	if !ok {
		return Checkout{}, ErrUnsupported
	}
	if e = ready(m, d); e != nil {
		return Checkout{}, e
	}
	if o.Validate(m) != nil {
		return Checkout{}, ErrInvalid
	}
	settings, e := ledger.RequireMonetary(ctx, s.DB)
	if e != nil {
		return Checkout{}, e
	}
	if o.Settlement().Currency != settings.Currency {
		return Checkout{}, ErrConflict
	}
	if supported, _ := SettlementSupport(m, settings.Currency); !supported {
		return Checkout{}, ErrUnsupported
	}
	if o.Money.Currency != merchantCurrency(m) {
		return Checkout{}, ErrInvalid
	}
	o.PaymentDriver = m.Driver
	o.AssetNetworkVersionID = m.AssetNetworkVersionID
	o.Network, o.TokenContract, o.MinimumConfirmations = "", "", ""
	if m.Driver == "evm_wallet" {
		o.Network, o.TokenContract, o.MinimumConfirmations = m.Credentials["chain_id"], m.Credentials["token_contract"], m.Credentials["min_confirmations"]
	}
	if m.Driver == "nowpayments" {
		o.Network, o.TokenContract = m.Credentials["pay_network"], m.Credentials["pay_token_contract"]
	}
	hash := fingerprint(struct {
		Order                      Order
		PaymentMethod, CustomerRef string
	}{o, o.PaymentMethod, o.CustomerRef})
	var exists int64
	if e = s.DB.WithContext(ctx).Table("payments_intents").Where("merchant_id=? AND order_id=?", merchant, o.ID).Count(&exists).Error; e != nil {
		return Checkout{}, e
	}
	var quoteErr error
	if exists == 0 {
		var quoted Order
		quoted, quoteErr = s.preparePaymentQuote(ctx, m, o)
		if quoteErr == nil {
			o = quoted
		}
	}
	fresh := false
	var saved intentRow
	e = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", merchant+":"+o.ID).Error; e != nil {
			return e
		}
		q := tx.Table("payments_intents").Where("merchant_id=? AND order_id=?", merchant, o.ID).Limit(1).Find(&saved)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected > 0 {
			if saved.OperatorID != operator || saved.Fingerprint != hash {
				return ErrConflict
			}
			return nil
		}
		if e := s.applyAssetNetwork(ctx, tx, &m, true); e != nil {
			return e
		}
		now, e := databaseTime(ctx, tx)
		if e != nil {
			return e
		}
		if !now.Before(o.ExpiresAt) || o.AcceptedAt.After(now.Add(30*time.Second)) {
			return ErrInvalid
		}
		cipher, e := s.seal(o)
		if e != nil {
			return e
		}
		fresh = true
		return tx.Table("payments_intents").Create(map[string]any{"merchant_id": merchant, "order_id": o.ID, "operator_id": operator, "fingerprint": hash, "state": "executing", "order_ciphertext": cipher, "created_at": now, "updated_at": now}).Error
	})
	if e != nil {
		return Checkout{}, e
	}
	if !fresh {
		if saved.State == "verified" {
			return Checkout{ProviderID: saved.ProviderID, State: "verified"}, nil
		}
		if saved.State == "failed" {
			return Checkout{ProviderID: saved.ProviderID, State: "failed"}, ErrProvider
		}
		if saved.State == "created" && saved.CheckoutCiphertext != "" {
			var checkout Checkout
			if e = s.open(saved.CheckoutCiphertext, &checkout); e != nil {
				return checkout, e
			}
			return checkout, nil
		}
		return Checkout{}, ErrOutcomeUnknown
	}
	var checkout Checkout
	callErr := quoteErr
	if callErr == nil {
		checkout, callErr = d.Create(ctx, m, o)
	}
	if m.Driver == "nowpayments" && callErr == nil && checkout.PayMoney != nil {
		q := o.PaymentQuote
		if q == nil {
			q = &PaymentQuote{ID: uuid.NewString(), SettlementMoney: o.Settlement(), Source: "NOWPayments", SourceURL: "https://nowpayments.io/", RateDate: time.Now().UTC(), QuotedAt: time.Now().UTC(), ExpiresAt: o.ExpiresAt}
		}
		if o.ProviderPriceMoney != nil {
			invoiceMoney := *o.ProviderPriceMoney
			q.InvoiceMoney = &invoiceMoney
			q.Source = "NOWPayments + Frankfurter"
		}
		q.PayMoney = *checkout.PayMoney
		ratio := new(big.Rat).Quo(q.PayMoney.Amount.Rat(), q.SettlementMoney.Amount.Rat())
		q.Rate, _ = ledger.Quantize(ratio)
		q.RateNumerator, q.RateDenominator = ratio.Num().String(), ratio.Denom().String()
		o.PaymentQuote = q
		o.Money = q.PayMoney
	}
	if o.PaymentQuote != nil && checkout.PayMoney != nil && (checkout.PayMoney.Currency != o.Money.Currency || checkout.PayMoney.Amount.Cmp(o.Money.Amount) != 0) {
		checkout = Checkout{ProviderID: checkout.ProviderID}
		callErr = ErrConflict
	}
	checkout.PaymentQuote = o.PaymentQuote
	if checkout.PayMoney == nil && o.Money.Amount.Sign() > 0 {
		payment := o.Money
		checkout.PayMoney = &payment
	}

	if checkout.SettlementMoney == nil {
		settlement := o.Settlement()
		checkout.SettlementMoney = &settlement
	}
	state := "created"
	if callErr != nil {
		state = "outcome_unknown"
		// A quote failure happens before any payment creation attempt. Record
		// that proof so the customer can cancel or choose another method.
		if quoteErr != nil || errors.Is(callErr, ErrUnsupported) || errors.Is(callErr, ErrInvalid) || errors.Is(callErr, ErrProvider) || errors.Is(callErr, ErrNotConfigured) || errors.Is(callErr, ErrForbidden) {
			state = "failed"
		}
	}
	orderCipher, sealErr := s.seal(o)
	if sealErr != nil {
		return Checkout{}, sealErr
	}
	cipher, sealErr := s.seal(checkout)
	if sealErr != nil {
		return Checkout{}, sealErr
	}
	persistCtx, cancelPersist := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancelPersist()
	updateErr := s.DB.WithContext(persistCtx).Table("payments_intents").Where("merchant_id=? AND order_id=?", merchant, o.ID).Updates(map[string]any{"state": state, "order_ciphertext": orderCipher, "checkout_ciphertext": cipher, "provider_id": checkout.ProviderID, "updated_at": time.Now().UTC()}).Error
	if updateErr != nil {
		return checkout, ErrOutcomeUnknown
	}
	return checkout, callErr
}
func (s *Service) loadOrder(ctx context.Context, m Merchant, id string) (Order, error) {
	var row intentRow
	if e := s.DB.WithContext(ctx).Table("payments_intents").Where("merchant_id=? AND order_id=? AND operator_id=?", m.ID, id, m.OperatorID).Take(&row).Error; e != nil {
		return Order{}, e
	}
	var o Order
	if e := s.open(row.OrderCiphertext, &o); e != nil {
		return o, e
	}
	o.ProviderID = row.ProviderID
	return o, nil
}

// ReceiveWebhook authenticates first, then records a durable minimal hint. It
// does not require the provider query to succeed in the HTTP callback window.
func (s *Service) ReceiveWebhook(ctx context.Context, merchant string, w Webhook) (Hint, error) {
	m, e := s.merchantByID(ctx, merchant)
	if e != nil {
		return Hint{}, e
	}
	d := s.Drivers[m.Driver]
	if d == nil {
		return Hint{}, ErrUnsupported
	}
	hint, e := d.VerifyWebhook(ctx, m, w)
	if e != nil || !hint.SignatureVerified {
		return Hint{}, ErrSignature
	}
	if hint.Kind == "test" {
		return hint, nil
	}
	if hint.OrderID == "" {
		return Hint{}, ErrInvalid
	}
	if _, e = s.loadOrder(ctx, m, hint.OrderID); e != nil {
		return Hint{}, e
	}
	if w.ReceivedAt.IsZero() {
		w.ReceivedAt = time.Now().UTC()
	}
	hash := fingerprint(json.RawMessage(w.Body))
	raw, _ := json.Marshal(hint)
	e = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		fields := map[string]any{"merchant_id": m.ID, "event_id": hint.EventID, "body_digest": hash, "hint": string(raw), "state": "pending", "received_at": w.ReceivedAt}
		result := tx.Table("payments_webhook_inbox").Clauses(clause.OnConflict{DoNothing: true}).Create(fields)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			var old struct{ BodyDigest string }
			if e := tx.Table("payments_webhook_inbox").Where("merchant_id=? AND event_id=?", m.ID, hint.EventID).Take(&old).Error; e != nil {
				return e
			}
			if old.BodyDigest != hash {
				return ErrConflict
			}
		}
		return nil
	})
	return hint, e
}
func (s *Service) Reconcile(ctx context.Context, operator, merchant, orderID string) (Fact, error) {
	m, e := s.Merchant(ctx, operator, merchant)
	if e != nil {
		return Fact{}, e
	}
	o, e := s.loadOrder(ctx, m, orderID)
	if e != nil {
		return Fact{}, e
	}
	m, e = s.historicalMerchant(ctx, m, o)
	if e != nil {
		return Fact{}, e
	}
	d := s.Drivers[m.Driver]
	fact, e := d.Lookup(ctx, m, o)
	if e != nil {
		return Fact{}, e
	}
	e = s.RecordFact(ctx, m, o, fact)
	return fact, e
}
func (s *Service) ProcessInbox(ctx context.Context, merchant, eventID string) (Fact, error) {
	m, e := s.merchantByID(ctx, merchant)
	if e != nil {
		return Fact{}, e
	}
	var inbox struct {
		Hint       []byte
		State      string
		ReceivedAt time.Time
	}
	if e = s.DB.WithContext(ctx).Table("payments_webhook_inbox").Where("merchant_id=? AND event_id=?", merchant, eventID).Take(&inbox).Error; e != nil {
		return Fact{}, e
	}
	var hint Hint
	if json.Unmarshal(inbox.Hint, &hint) != nil {
		return Fact{}, ErrUnsupported
	}
	if hint.Kind == "refund" {
		id := hint.ReferenceID
		if id == "" {
			var row struct{ RefundID string }
			if e := s.DB.WithContext(ctx).Table("payments_refund_intents").Where("merchant_id=? AND order_id=? AND provider_id=?", m.ID, hint.OrderID, hint.ProviderID).Take(&row).Error; e != nil {
				return Fact{}, e
			}
			id = row.RefundID
		}
		result, e := s.QueryRefund(ctx, m.OperatorID, m.ID, id)
		if e != nil {
			return Fact{}, e
		}
		if result.Fact == nil {
			return Fact{}, ErrNotFinal
		}
		if result.Fact.ProviderID != hint.ProviderID {
			return Fact{}, ErrConflict
		}
		e = s.DB.WithContext(ctx).Table("payments_webhook_inbox").Where("merchant_id=? AND event_id=?", merchant, eventID).Updates(map[string]any{"state": "processed", "processed_at": time.Now().UTC(), "error_code": nil, "lease_until": nil}).Error
		return *result.Fact, e
	}
	if hint.Kind != "receipt" {
		return Fact{}, ErrUnsupported
	}
	o, e := s.loadOrder(ctx, m, hint.OrderID)
	if e != nil {
		return Fact{}, e
	}
	if o.ProviderID == "" {
		o.ProviderID = hint.ProviderID
	}
	m, e = s.historicalMerchant(ctx, m, o)
	if e != nil {
		return Fact{}, e
	}
	d := s.Drivers[m.Driver]
	fact, e := d.Lookup(ctx, m, o)
	if e != nil {
		return Fact{}, e
	}
	if fact.ProviderID != hint.ProviderID {
		return Fact{}, ErrConflict
	}
	if fact.OccurredAt == nil && hint.OccurredAt != nil {
		fact.OccurredAt = hint.OccurredAt
		fact.Evidence, _ = json.Marshal(map[string]any{"query": fact.Evidence, "authenticated_event": map[string]any{"event_id": hint.EventID, "occurred_at": hint.OccurredAt, "signature_verified": true}})
	}
	if hint.Final && inbox.ReceivedAt.Before(fact.ObservedAt) {
		fact.ObservedAt = inbox.ReceivedAt
	}
	e = s.RecordFact(ctx, m, o, fact)
	if e != nil {
		return fact, e
	}
	e = s.DB.WithContext(ctx).Table("payments_webhook_inbox").Where("merchant_id=? AND event_id=?", merchant, eventID).Updates(map[string]any{"state": "processed", "processed_at": time.Now().UTC(), "error_code": nil, "lease_until": nil}).Error
	return fact, e
}
func (s *Service) RecordFact(ctx context.Context, m Merchant, o Order, incoming Fact) error {
	if !incoming.Verified() || incoming.MerchantID != m.ID || incoming.OperatorID != m.OperatorID || incoming.OrderID != o.ID || o.Validate(m) != nil {
		return ErrInvalid
	}
	if incoming.Kind != "receipt" && (incoming.Kind != "refund" || incoming.ExecutionValue == nil || !incoming.ExecutionValue.Amount.Valid() || incoming.ExecutionValue.Currency != o.Settlement().Currency) {
		return ErrInvalid
	}
	hash := fingerprint(struct {
		Kind, Operator, Merchant, Order, Provider string
		Money                                     Money
	}{incoming.Kind, incoming.OperatorID, incoming.MerchantID, incoming.OrderID, incoming.ProviderID, incoming.Money})
	var pendingErr error
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if incoming.Kind == "receipt" {
			if e := s.lockReceiptAcceptance(ctx, tx, m.OperatorID, o.ID); e != nil {
				return e
			}
		}
		if e := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", incoming.ID).Error; e != nil {
			return e
		}
		var row receiptFactRow
		q := tx.Table("payments_verified_facts").Where("id=?", incoming.ID).Limit(1).Find(&row)
		if q.Error != nil {
			return q.Error
		}
		f := incoming
		if q.RowsAffected > 0 {
			if row.Fingerprint != hash {
				return ErrConflict
			}
			// Decode into fresh storage. A shallow copy of incoming shares its
			// RawMessage backing array; JSONB normalization can expand evidence
			// and otherwise overwrite the caller's shorter JSON value on replay.
			var stored Fact
			if json.Unmarshal(row.Fact, &stored) != nil {
				return ErrInvalid
			}
			f = stored
			f.proof = true
			if incoming.ExecutionValue != nil && f.ExecutionValue == nil && row.ValuationVersion == 0 {
				if _, reason := receiptExecutionValue(o, incoming); reason != "" {
					return ErrInvalid
				}
				f.ExecutionValue = incoming.ExecutionValue
				f.Evidence = incoming.Evidence
				raw, _ := json.Marshal(f)
				if e := tx.Table("payments_verified_facts").Where("id=?", f.ID).Update("fact", string(raw)).Error; e != nil {
					return e
				}
			}
			if f.OccurredAt == nil && incoming.OccurredAt != nil {
				f.OccurredAt = incoming.OccurredAt
				f.Evidence = incoming.Evidence
				raw, _ := json.Marshal(f)
				if e := tx.Table("payments_verified_facts").Where("id=?", f.ID).Update("fact", string(raw)).Error; e != nil {
					return e
				}
				if row.ReceiptID != "" {
					if corrector, ok := s.Sink.(ReceiptCorrectionSink); ok {
						if e := corrector.ConfirmVerifiedPaymentTime(ctx, tx, row.ReceiptID, f); e != nil {
							return e
						}
					}
				}
			}
			if row.ReceiptID != "" {
				return nil
			}
			if string(row.Valuation) != "null" && len(row.Valuation) > 0 {
				var original MoneyValue
				if json.Unmarshal(row.Valuation, &original) != nil {
					return ErrInvalid
				}
				if incoming.ExecutionValue != nil && (incoming.ExecutionValue.Currency != original.Currency || incoming.ExecutionValue.Amount.Cmp(original.Amount) != 0) {
					return ErrConflict
				}
				pendingErr = s.attachReceiptDurably(ctx, tx, m, o, f, original)
				return nil
			}
		} else {
			now, e := databaseTime(ctx, tx)
			if e != nil {
				return e
			}
			f.ObservedAt = now
			raw, _ := json.Marshal(f)
			if e := tx.Table("payments_verified_facts").Create(map[string]any{"id": f.ID, "merchant_id": m.ID, "operator_id": m.OperatorID, "order_id": o.ID, "kind": f.Kind, "fingerprint": hash, "fact": string(raw), "valuation": "null", "state": "awaiting_settlement", "observed_at": now}).Error; e != nil {
				return e
			}
		}
		if f.Kind == "refund" {
			value := *f.ExecutionValue
			raw, _ := json.Marshal(value)
			if e := tx.Table("payments_verified_facts").Where("id=?", f.ID).Updates(map[string]any{"valuation": string(raw), "state": "verified"}).Error; e != nil {
				return e
			}
			return nil
		}
		value, reason := receiptExecutionValue(o, f)
		if reason != "" {
			if e := tx.Table("payments_verified_facts").Where("id=?", f.ID).Updates(map[string]any{"state": "awaiting_settlement", "valuation_reason": reason}).Error; e != nil {
				return e
			}
			if e := tx.Table("payments_intents").Where("merchant_id=? AND order_id=?", m.ID, o.ID).Updates(map[string]any{"state": "awaiting_settlement", "provider_id": f.ProviderID, "updated_at": gorm.Expr("clock_timestamp()")}).Error; e != nil {
				return e
			}
			pendingErr = ErrAwaitingValuation
			return nil
		}
		if e := s.recordReceiptValuation(ctx, tx, m, o, f, value, "automatic:"+f.ID, hash, "provider-confirmed payment time", value.EvidenceReference, "adapter:"+m.Driver); e != nil {
			return e
		}
		pendingErr = s.attachReceiptDurably(ctx, tx, m, o, f, value)
		return nil
	})
	if err != nil {
		return err
	}
	return pendingErr
}

func (s *Service) attachReceipt(ctx context.Context, tx *gorm.DB, m Merchant, o Order, f Fact, v MoneyValue) error {
	if f.Kind == "receipt" {
		if e := tx.Table("payments_intents").Where("merchant_id=? AND order_id=?", m.ID, o.ID).Updates(map[string]any{"state": "verified", "provider_id": f.ProviderID, "updated_at": time.Now().UTC()}).Error; e != nil {
			return e
		}
	}
	if s.Sink == nil || f.Kind != "receipt" || v.Amount.Sign() <= 0 {
		return nil
	}
	id, e := s.Sink.RecordVerifiedReceipt(ctx, tx, m, o, f, v)
	if e != nil {
		return e
	}
	return tx.Table("payments_verified_facts").Where("id=?", f.ID).Updates(map[string]any{"receipt_id": id, "state": "recorded"}).Error
}
