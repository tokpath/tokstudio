package payments

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"gorm.io/gorm"
)

func TestPlatformAssetVersionsGateNewCollectionsPreserveHistoricalFacts(t *testing.T) {
	base := os.Getenv("TOKENHUB_PAYMENT_MOCK_URL")
	if base == "" {
		t.Skip("independent HTTP/RPC lab required")
	}
	db := paymentDB(t)
	_, migrations := organization.Migrations()
	entries, e := fs.ReadDir(migrations, ".")
	if e != nil {
		t.Fatal(e)
	}
	for _, entry := range entries {
		raw, e := fs.ReadFile(migrations, entry.Name())
		if e != nil {
			t.Fatal(e)
		}
		if e = db.Exec(string(raw)).Error; e != nil {
			t.Fatal(e)
		}
	}
	ctx := context.Background()
	org := organization.New(db)
	owner := uuid.NewString()
	p, e := org.BootstrapPlatform(ctx, nil, organization.BootstrapInput{OwnerStaffID: owner, BrandName: "Asset tests", OperatorName: "Platform", LegalName: "Asset Test Legal"})
	if e != nil {
		t.Fatal(e)
	}
	child, e := org.CreateOperator(ctx, nil, organization.CreateOperatorInput{ActorStaffID: owner, ParentOperatorID: p.Operator.ID, Kind: organization.Channel, Name: "Child"})
	if e != nil {
		t.Fatal(e)
	}
	transport := NewTransport(true)
	defer transport.Client.CloseIdleConnections()
	s := NewService(db, "asset-registry-test", org, transport, &sqlReceiptSink{})
	contract := "0x" + strings.Repeat("aa", 20)
	wallet := "0x" + strings.Repeat("bb", 20)
	asset := AssetNetwork{Driver: "evm_wallet", Asset: "USDC", Network: "1", TokenContract: contract, TokenDecimals: 6, MinimumConfirmations: 12, Enabled: true, EvidenceReference: "independent verified canonical network/token source"}
	if _, e = s.PublishAssetNetwork(ctx, owner, child.Operator.ID, asset, 0); !errors.Is(e, ErrForbidden) {
		t.Fatalf("non-platform publisher: %v", e)
	}
	asset, e = s.PublishAssetNetwork(ctx, owner, p.Operator.ID, asset, 0)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.PublishAssetNetwork(ctx, owner, p.Operator.ID, asset, 0); !errors.Is(e, ErrConflict) {
		t.Fatalf("stale publication: %v", e)
	}
	m := Merchant{ID: uuid.NewString(), OperatorID: p.Operator.ID, Driver: "evm_wallet", Enabled: true, APIBaseURL: base, Credentials: map[string]string{"rpc_url": base + "/rpc", "wallet_address": wallet}}
	if _, e = s.SaveMerchant(ctx, owner, m, 0); !errors.Is(e, ErrNotConfigured) {
		t.Fatalf("missing platform version: %v", e)
	}
	m.AssetNetworkVersionID = asset.VersionID
	if _, e = s.SaveMerchant(ctx, owner, m, 0); e != nil {
		t.Fatal(e)
	}
	m, e = s.Merchant(ctx, m.OperatorID, m.ID)
	if e != nil {
		t.Fatal(e)
	}
	if m.Credentials["chain_id"] != "1" || m.Credentials["token_contract"] != contract || m.Credentials["min_confirmations"] != "12" {
		t.Fatalf("metadata not derived: %+v", m.Credentials)
	}
	bad := m
	bad.ID = uuid.NewString()
	bad.Credentials = map[string]string{"rpc_url": base + "/rpc", "wallet_address": wallet, "token_contract": "0x" + strings.Repeat("ee", 20)}
	if _, e = s.SaveMerchant(ctx, owner, bad, 0); !errors.Is(e, ErrConflict) {
		t.Fatalf("arbitrary contract enabled: %v", e)
	}
	now, e := databaseTime(ctx, db)
	if e != nil {
		t.Fatal(e)
	}
	o := Order{ID: uuid.NewString(), OperatorID: m.OperatorID, MerchantID: m.ID, Money: Money{Currency: "USDC", Amount: ledger.FromInt64(10)}, SettlementMoney: Money{Currency: "USD", Amount: ledger.FromInt64(10)}, AcceptedAt: now, ExpiresAt: now.Add(time.Hour)}
	if _, e = s.Create(ctx, m.OperatorID, m.ID, o); e != nil {
		t.Fatal(e)
	}
	asset.Enabled = false
	asset.EffectiveAt = time.Time{}
	disabled, e := s.PublishAssetNetwork(ctx, owner, p.Operator.ID, asset, 1)
	if e != nil {
		t.Fatal(e)
	}
	methods, e := s.PublicMethods(ctx, m.OperatorID)
	if e != nil || len(methods) != 0 {
		t.Fatalf("disabled asset still public: %+v %v", methods, e)
	}
	next := o
	next.ID = uuid.NewString()
	if _, e = s.Create(ctx, m.OperatorID, m.ID, next); !errors.Is(e, ErrNotConfigured) {
		t.Fatalf("new collection after disable: %v", e)
	}
	current, e := s.ListSupportedAssets(ctx)
	if e != nil || len(current) != 0 {
		t.Fatalf("active registry: %+v %v", current, e)
	}
	history, e := s.AdminAssetNetworks(ctx, owner, p.Operator.ID)
	if e != nil || len(history) != 2 || history[0].VersionID != disabled.VersionID {
		t.Fatalf("immutable registry history: %+v %v", history, e)
	}
	m.Enabled = false
	if _, e = s.SaveMerchant(ctx, owner, m, 1); e != nil {
		t.Fatal(e)
	}
	intent, e := s.PaymentIntent(ctx, m.OperatorID, m.ID, o.ID)
	if e != nil || intent.Driver != "evm_wallet" || intent.Network != "1" || intent.TokenContract != contract {
		t.Fatalf("old intent metadata unavailable: %+v %v", intent, e)
	}
	txhash := "0x" + strings.ReplaceAll(uuid.NewString()+uuid.NewString(), "-", "")
	raw, _ := json.Marshal(map[string]any{"transaction_hash": txhash, "raw_units": "10000000", "sender": "0x" + strings.Repeat("cc", 20), "recipient": wallet, "token_contract": contract, "head": 120, "reorg": false})
	response, e := http.Post(base+"/__control/protocol/evm", "application/json", bytes.NewReader(raw))
	if e != nil {
		t.Fatal(e)
	}
	response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatalf("control status %d", response.StatusCode)
	}
	fact, e := s.VerifyWalletReceipt(ctx, m.OperatorID, m.ID, o.ID, txhash, "0x0")
	if !errors.Is(e, ErrAwaitingSettlement) || !fact.Verified() {
		t.Fatalf("historical receipt blocked by disable: %+v %v", fact, e)
	}
	// Re-enabling a new revision cannot rewrite the old checkout confirmation rule.
	disabled.Enabled = true
	disabled.MinimumConfirmations = 20
	disabled.EffectiveAt = time.Time{}
	if _, e = s.PublishAssetNetwork(ctx, owner, p.Operator.ID, disabled, 2); e != nil {
		t.Fatal(e)
	}
	original, e := s.loadOrder(ctx, m, o.ID)
	if e != nil {
		t.Fatal(e)
	}
	old, e := s.historicalMerchant(ctx, m, original)
	if e != nil || old.Credentials["min_confirmations"] != "12" {
		t.Fatalf("historical confirmation threshold changed: %v %+v", e, old.Credentials)
	}
	if e = db.Exec("UPDATE payments_asset_network_versions SET enabled=true WHERE version_id=?", asset.VersionID).Error; e == nil {
		t.Fatal("published asset version changed")
	}
}

func TestRefundExecutionSealingNeverQueriesOrRetriesProvider(t *testing.T) {
	db := paymentDB(t)
	s := NewService(db, "closure-test", testPaymentAuth{}, nil, nil)
	m := Merchant{ID: uuid.NewString(), OperatorID: uuid.NewString(), Driver: "manual", Enabled: true, Credentials: map[string]string{"currency": "USD", "recipient_reference": "own-bank-account", "instructions": "bank transfer"}}
	if _, e := s.SaveMerchant(context.Background(), "finance", m, 0); e != nil {
		t.Fatal(e)
	}
	ctx := context.Background()
	leg := uuid.NewString()
	r := Refund{ID: leg}
	encrypted, e := s.seal(r)
	if e != nil {
		t.Fatal(e)
	}
	final, e := s.seal(RefundStatus{State: "succeeded", Fact: &Fact{ID: "actual-fact", Kind: "refund"}})
	if e != nil {
		t.Fatal(e)
	}
	row := map[string]any{"merchant_id": m.ID, "operator_id": m.OperatorID, "refund_id": leg, "business_refund_id": leg, "order_id": "original", "fingerprint": "original", "state": "outcome_unknown", "refund_ciphertext": encrypted, "result_ciphertext": final, "created_at": time.Now().UTC(), "updated_at": time.Now().UTC()}
	if e = db.Table("payments_refund_intents").Create(row).Error; e != nil {
		t.Fatal(e)
	}
	if e = db.Transaction(func(tx *gorm.DB) error { return s.SealRefundExecutions(ctx, tx, m.OperatorID, leg, "closed-route") }); !errors.Is(e, ErrOutcomeUnknown) {
		t.Fatalf("unknown execution sealed: %v", e)
	}
	if e = s.claimRefundQuery(ctx, m.OperatorID, m.ID, leg); e != nil {
		t.Fatal(e)
	}
	if e = db.Transaction(func(tx *gorm.DB) error {
		return s.SealRefundExecutions(ctx, tx, m.OperatorID, leg, "blocked-during-network-query")
	}); !errors.Is(e, ErrOutcomeUnknown) {
		t.Fatalf("active network claim was sealed: %v", e)
	}
	if e = db.Table("payments_refund_intents").Where("refund_id=?", leg).Update("state", "verified_pending_settlement").Error; e != nil {
		t.Fatal(e)
	}
	if e = db.Transaction(func(tx *gorm.DB) error {
		facts, e := s.RefundExecutionClosureFacts(ctx, tx, m.OperatorID, leg)
		if e != nil {
			return e
		}
		if len(facts) != 1 || facts[0].FactID != "actual-fact" {
			t.Fatalf("closure source %+v", facts)
		}
		return s.SealRefundExecutions(ctx, tx, m.OperatorID, leg, "closed-route")
	}); e != nil {
		t.Fatal(e)
	}
	if e = s.claimRefundQuery(ctx, m.OperatorID, m.ID, leg); !errors.Is(e, ErrConflict) {
		t.Fatalf("sealed execution claimed for a network query: %v", e)
	}
	result, e := s.QueryRefund(ctx, m.OperatorID, m.ID, leg)
	if e != nil || result.State != "closed" || result.Fact == nil || result.Fact.ID != "actual-fact" {
		t.Fatalf("sealed result: %+v %v", result, e)
	}
}
