package remediation

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/payments"
	"gorm.io/gorm"
)

const externalTestContract = "0x1111111111111111111111111111111111111111"
const externalTestMerchant = "0x2222222222222222222222222222222222222222"
const externalTestOriginalSender = "0x3333333333333333333333333333333333333333"
const externalTestDestination = "0x4444444444444444444444444444444444444444"

type externalRefundFixture struct {
	fixture
	merchant    payments.Merchant
	order       payments.Order
	receipt     string
	leg         RefundLeg
	destination RefundDestination
	blockTime   time.Time
}

// The RPC protocol fixture exercises the real read-only ERC20 driver. It is
// synthetic chain state; it is not evidence of a live chain/account test.
func externalRefundSetup(t *testing.T) externalRefundFixture {
	return externalRefundSetupFor(t, setup(t), true)
}
func externalRefundSetupFor(t *testing.T, f fixture, request bool, registryAuthority ...fixture) externalRefundFixture {
	t.Helper()
	at, e := instant(f.db)
	if e != nil {
		t.Fatal(e)
	}
	blockAt := at.Truncate(time.Second)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			JSONRPC string            `json:"jsonrpc"`
			ID      int               `json:"id"`
			Method  string            `json:"method"`
			Params  []json.RawMessage `json:"params"`
		}
		if json.NewDecoder(r.Body).Decode(&request) != nil {
			t.Error("invalid RPC request")
			w.WriteHeader(400)
			return
		}
		var result any
		switch request.Method {
		case "eth_chainId":
			result = "0x1"
		case "eth_call":
			result = "0x6"
		case "eth_blockNumber":
			result = "0x3e8"
		case "eth_getBlockByNumber":
			result = map[string]any{"hash": "0x" + strings.Repeat("a", 64), "number": "0x3e7", "timestamp": fmt.Sprintf("0x%x", blockAt.Unix())}
		case "eth_getTransactionReceipt":
			var tx string
			_ = json.Unmarshal(request.Params[0], &tx)
			sender, recipient, amount := externalTestMerchant, externalTestDestination, int64(20000000)
			if tx == "0x"+strings.Repeat("1", 64) {
				sender, recipient, amount = externalTestOriginalSender, externalTestMerchant, 100000000
			}
			if tx == "0x"+strings.Repeat("3", 64) {
				recipient = externalTestOriginalSender
			}
			if tx == "0x"+strings.Repeat("4", 64) {
				amount = 19000000
			}
			if tx == "0x"+strings.Repeat("5", 64) {
				amount = 1000000
			}
			if tx == "0x"+strings.Repeat("6", 64) {
				sender = "0x6666666666666666666666666666666666666666"
				amount = 19000000
			}
			if tx == "0x"+strings.Repeat("7", 64) {
				sender = "0x6666666666666666666666666666666666666666"
				amount = 1000000
			}
			log := map[string]any{"address": externalTestContract, "transactionHash": tx, "blockHash": "0x" + strings.Repeat("a", 64), "logIndex": "0x0", "topics": []string{"0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef", "0x" + strings.Repeat("0", 24) + sender[2:], "0x" + strings.Repeat("0", 24) + recipient[2:]}, "data": fmt.Sprintf("0x%064x", amount), "removed": false}
			result = map[string]any{"transactionHash": tx, "blockHash": "0x" + strings.Repeat("a", 64), "blockNumber": "0x3e7", "status": "0x1", "logs": []any{log}}
		default:
			t.Errorf("unexpected RPC %s", request.Method)
			w.WriteHeader(400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result})
	}))
	t.Cleanup(server.Close)
	transport := payments.NewTransport(true)
	pay := payments.NewService(f.db, "isolated-refund-destination-encryption-key", f.s.Organization, transport, payments.CommerceSink{Commerce: f.s.Commerce})
	pay.RefundApproval = f.s
	pay.RefundSink = f.s
	f.s.Payments = pay
	registrar := f
	if len(registryAuthority) > 0 {
		registrar = registryAuthority[0]
	}
	asset, e := pay.PublishAssetNetwork(f.ctx, registrar.owner, registrar.operator, payments.AssetNetwork{Driver: "evm_wallet", Asset: "USDT", Network: "1", TokenContract: externalTestContract, TokenDecimals: 6, MinimumConfirmations: 2, Enabled: true, EvidenceReference: "isolated protocol fixture verified chain and ERC20 metadata"}, 0)
	if e != nil {
		t.Fatal(e)
	}
	m := payments.Merchant{ID: uuid.NewString(), OperatorID: f.operator, Driver: "evm_wallet", Enabled: true, AssetNetworkVersionID: asset.VersionID, Credentials: map[string]string{"rpc_url": server.URL, "chain_id": "1", "token_contract": externalTestContract, "token_decimals": "6", "wallet_address": externalTestMerchant, "asset": "USDT", "min_confirmations": "2"}}
	if _, e = pay.SaveMerchant(f.ctx, f.owner, m, 0); e != nil {
		t.Fatal(e)
	}
	topup, e := f.s.Commerce.CreateTopUpOrder(f.ctx, nil, commerce.CreateTopUpInput{OperatorID: f.operator, CustomerID: f.customer, BusinessKey: "verified-chain-topup", ExpectedOwnershipVersion: 1, AmountUSD: d("100")})
	if e != nil {
		t.Fatal(e)
	}
	o := payments.Order{ID: topup.ID, OperatorID: f.operator, MerchantID: m.ID, Money: payments.Money{Currency: "USDT", Amount: d("100")}, Valuation: payments.Valuation{ID: "verified-original-stablecoin-usd", Currency: "USDT", USDPerUnit: d("1"), Source: "synthetic independently valued actual token", EffectiveAt: at.Add(-time.Hour), ExpiresAt: at.Add(time.Hour)}, AcceptedAt: at, ExpiresAt: at.Add(time.Hour)}
	if _, e = pay.Create(f.ctx, f.operator, m.ID, o); e != nil {
		t.Fatal(e)
	}
	o.ProviderID = "0x" + strings.Repeat("1", 64) + "#0x0"
	fact, e := pay.Drivers["evm_wallet"].Lookup(f.ctx, m, o)
	if e != nil {
		t.Fatal(e)
	}
	if e = pay.RecordFact(f.ctx, m, o, fact); e != nil {
		t.Fatal(e)
	}
	var link struct{ ReceiptID string }
	if e = f.db.Table("payments_verified_facts").Select("receipt_id").Where("id=?", fact.ID).Take(&link).Error; e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.Commerce.AllocateReceipt(f.ctx, nil, commerce.AllocateReceiptInput{ActorStaffID: f.owner, OperatorID: f.operator, ReceiptID: link.ReceiptID, OrderID: topup.ID, BusinessKey: "verified-chain-allocation", AmountUSD: d("100")}); e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.Commerce.Fulfill(f.ctx, nil, topup.ID, f.owner); e != nil {
		t.Fatal(e)
	}
	out := externalRefundFixture{fixture: f, merchant: m, order: o, receipt: link.ReceiptID, destination: RefundDestination{Network: "1", Asset: "USDT", TokenContract: externalTestContract, Address: externalTestDestination}, blockTime: blockAt}
	if !request {
		return out
	}
	c, e := f.s.RequestWalletRefund(f.ctx, WalletRefundInput{OperatorID: f.operator, CustomerID: f.customer, BusinessKey: "stablecoin-refund-20", AmountUSD: d("20"), Reason: "unused deposit to verified original payer destination"})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.ReviewRefund(f.ctx, f.owner, f.operator, c.ID, "approve", "verified original source and unused balance"); e != nil {
		t.Fatal(e)
	}
	legs, e := f.s.Legs(f.ctx, nil, f.operator, c.ID)
	if e != nil || len(legs) != 1 {
		t.Fatal(legs, e)
	}
	out.leg = legs[0]
	return out
}

func (f externalRefundFixture) authorize(t *testing.T) RefundDestinationAuthorization {
	t.Helper()
	customer, e := f.s.Organization.GetCustomer(f.ctx, nil, f.customer)
	if e != nil {
		t.Fatal(e)
	}
	p, e := f.s.ProposeRefundDestination(f.ctx, f.customer, customer.BrandID, f.leg.ID, "customer-checked-destination", RefundDestinationProposalInput{Destination: f.destination, AuthorizationReference: "customer explicitly confirmed chain asset contract and receiving account"})
	if e != nil {
		t.Fatal(e)
	}
	a, e := f.s.AuthorizeRefundDestination(f.ctx, f.owner, f.operator, f.leg.ID, "finance-verified-destination", RefundDestinationAuthorizationInput{ProposalID: p.ID, PayerAuthorizationReference: "original payer independently consented exchange withdrawal destination", VerificationEvidence: "original transaction proof and original payer ownership of receiving exchange account verified"})
	if e != nil {
		t.Fatal(e)
	}
	return a
}
func (f externalRefundFixture) refund(t *testing.T, a RefundDestinationAuthorization, tx string) (payments.Refund, payments.EVMRefundVerification) {
	t.Helper()
	r := payments.Refund{ID: f.leg.ID, Order: f.order, USDAmount: d("20"), Money: payments.Money{Currency: "USDT", Amount: d("20")}, ValuedAt: f.blockTime, RefundValuation: payments.Valuation{ID: "actual-refund-fx", Currency: "USDT", USDPerUnit: d("1"), Source: "verified at actual execution", EffectiveAt: f.blockTime.Add(-time.Minute), ExpiresAt: f.blockTime.Add(time.Minute)}, FXExecutionReference: "independently verified refund execution rate"}
	v := payments.EVMRefundVerification{ReceiptID: f.receipt, TransactionHash: "0x" + strings.Repeat(tx, 64), LogIndex: "0x0", Destination: payments.ExternalRefundDestination{Network: a.Network, Asset: a.Asset, TokenContract: a.TokenContract, Address: a.Address, AuthorizationReference: a.ID, VerificationEvidence: "lookup actual refund transaction"}}
	return r, v
}

func TestPostgresStablecoinRefundRequiresLatestCustomerAndOriginalPayerDestinationAuthorization(t *testing.T) {
	f := externalRefundSetup(t)
	if _, e := f.s.BeginManualRefund(f.ctx, f.owner, f.operator, f.leg.ID, "no-authorization", "finance proposes sending"); !errors.Is(e, ErrReview) {
		t.Fatal("stablecoin instruction without destination", e)
	}
	user, _ := f.s.Organization.GetCustomer(f.ctx, nil, f.customer)
	old, e := f.s.ProposeRefundDestination(f.ctx, f.customer, user.BrandID, f.leg.ID, "old-destination", RefundDestinationProposalInput{Destination: f.destination, AuthorizationReference: "customer first choice"})
	if e != nil {
		t.Fatal(e)
	}
	updated := f.destination
	updated.Address = "0x5555555555555555555555555555555555555555"
	latest, e := f.s.ProposeRefundDestination(f.ctx, f.customer, user.BrandID, f.leg.ID, "updated-destination", RefundDestinationProposalInput{Destination: updated, AuthorizationReference: "customer explicitly changed before authorization"})
	if e != nil {
		t.Fatal(e)
	}
	input := RefundDestinationAuthorizationInput{ProposalID: old.ID, PayerAuthorizationReference: "original payer consent", VerificationEvidence: "original payer and address verified"}
	if _, e = f.s.AuthorizeRefundDestination(f.ctx, f.owner, f.operator, f.leg.ID, "stale-authorization", input); !errors.Is(e, ErrConflict) {
		t.Fatal("stale customer destination accepted", e)
	}
	input.ProposalID = latest.ID
	input.Destination = f.destination
	if _, e = f.s.AuthorizeRefundDestination(f.ctx, f.owner, f.operator, f.leg.ID, "staff-substitution", input); !errors.Is(e, ErrConflict) {
		t.Fatal("staff replaced customer endpoint", e)
	}
	input.Destination = RefundDestination{}
	a, e := f.s.AuthorizeRefundDestination(f.ctx, f.owner, f.operator, f.leg.ID, "actual-authorization", input)
	if e != nil || a.Address != updated.Address {
		t.Fatal(a, e)
	}
	if _, e = f.s.ProposeRefundDestination(f.ctx, f.customer, user.BrandID, f.leg.ID, "change-after-authorization", RefundDestinationProposalInput{Destination: f.destination, AuthorizationReference: "too late"}); !errors.Is(e, ErrState) {
		t.Fatal("authorized endpoint mutated", e)
	}
	if e = f.db.Transaction(func(tx *gorm.DB) error {
		return tx.Model(&a).Where("id=?", a.ID).Update("address", externalTestOriginalSender).Error
	}); e == nil {
		t.Fatal("authorization immutable trigger missing")
	}
	if _, e = f.s.BeginManualRefund(f.ctx, f.owner, f.operator, f.leg.ID, "authorized-send", "finance instructed exact authorized account"); e != nil {
		t.Fatal(e)
	}
	proof := ExecutionProof{BusinessKey: "untrusted-json", Reference: "txhash", Evidence: "claimed RPC", ActualUSD: d("20"), Currency: "USDT", Amount: "20", OccurredAt: f.blockTime}
	if _, e = f.s.RecordManualRefund(f.ctx, f.owner, f.operator, f.leg.ID, proof); !errors.Is(e, ErrReview) {
		t.Fatal("manual JSON bypassed chain verifier", e)
	}
}

func TestPostgresStablecoinRefundRPCSettlesAuthorizedAddressDifferentFromOriginalSender(t *testing.T) {
	f := externalRefundSetup(t)
	a := f.authorize(t)
	if a.Address == externalTestOriginalSender {
		t.Fatal("fixture does not cover exchange sender distinction")
	}
	if _, e := f.s.BeginManualRefund(f.ctx, f.owner, f.operator, f.leg.ID, "authorized-chain-send", "original payer authorized destination verified"); e != nil {
		t.Fatal(e)
	}
	r, v := f.refund(t, a, "2")
	status, e := f.s.Payments.RecordEVMRefund(f.ctx, f.owner, f.operator, f.merchant.ID, r, v)
	if e != nil || status.State != "succeeded" {
		t.Fatal(status, e)
	}
	if balance := f.balance(t); balance.Balance.Cmp(d("80")) != 0 || !balance.Held.IsZero() {
		t.Fatal(balance)
	}
	rows, e := f.s.RefundAccounting(f.ctx, nil, f.owner, f.operator, f.leg.ID)
	if e != nil || len(rows) != 1 || rows[0].ExternalRefund == nil || rows[0].ExternalRefund.Address != a.Address || rows[0].ExternalRefund.SenderAddress != externalTestMerchant || rows[0].ExternalRefund.TransactionHash != v.TransactionHash || rows[0].ExternalRefund.LogIndex != "0x0" || rows[0].FeeStatus != "pending" {
		t.Fatal(rows, e)
	}
	if _, e = f.s.Payments.RecordEVMRefund(f.ctx, f.owner, f.operator, f.merchant.ID, r, v); e != nil {
		t.Fatal("actual fact replay", e)
	}
	rows, e = f.s.RefundAccounting(f.ctx, nil, f.owner, f.operator, f.leg.ID)
	if e != nil || len(rows) != 1 {
		t.Fatal("duplicate actual payment", rows, e)
	}
	v.Destination.Address = externalTestOriginalSender
	if _, e = f.s.Payments.RecordEVMRefund(f.ctx, f.owner, f.operator, f.merchant.ID, r, v); e == nil {
		t.Fatal("changed destination accepted")
	}
}

func TestPostgresStablecoinRefundWrongRecipientAndFXMismatchRetainActualFactsWithoutClearingDebt(t *testing.T) {
	for _, tc := range []struct {
		name, tx  string
		wantError error
	}{{"wrong recipient", "3", payments.ErrSettlementReview}, {"actual amount short", "4", payments.ErrSettlementReview}} {
		t.Run(tc.name, func(t *testing.T) {
			f := externalRefundSetup(t)
			a := f.authorize(t)
			if _, e := f.s.BeginManualRefund(f.ctx, f.owner, f.operator, f.leg.ID, "actual-send-to-review", "finance began durable external instruction"); e != nil {
				t.Fatal(e)
			}
			r, v := f.refund(t, a, tc.tx)
			out, e := f.s.Payments.RecordEVMRefund(f.ctx, f.owner, f.operator, f.merchant.ID, r, v)
			if tc.wantError != nil && !errors.Is(e, tc.wantError) {
				t.Fatal(out, e)
			}
			if tc.tx == "4" && !errors.Is(e, payments.ErrSettlementReview) {
				t.Fatal(out, e)
			}
			if out.State != "verified_pending_settlement" {
				t.Fatal(out, e)
			}
			var facts int64
			if e = f.db.Table("payments_verified_facts").Where("kind='refund'").Count(&facts).Error; e != nil || facts != 1 {
				t.Fatal("actual chain fact lost", facts, e)
			}
			wantBalance, wantHeld := "100", "20"
			if tc.tx == "4" {
				wantBalance, wantHeld = "81", "1"
			}
			if balance := f.balance(t); balance.Balance.Cmp(d(wantBalance)) != 0 || balance.Held.Cmp(d(wantHeld)) != 0 {
				t.Fatal("actual partial payment did not preserve only the remaining liability", balance)
			}
			c, e := f.s.GetCase(f.ctx, nil, f.operator, f.leg.CaseID)
			if e != nil || c.State == "completed" {
				t.Fatal(c, e)
			}
			cash, e := f.s.account(f.ctx, f.db, f.operator, f.operator, "control:external_cash")
			wantCash := "20"
			if tc.tx == "4" {
				wantCash = "19"
			}
			if e != nil || cash.Balance.Cmp(d(wantCash)) != 0 {
				t.Fatal("actual refund cash disappeared", cash, e)
			}
			if tc.tx == "3" {
				cap, e := f.s.ReceiptRefundCapacity(f.ctx, nil, f.operator, f.receipt)
				if e != nil || !cap.CompletedUSD.IsZero() || cap.ReservedUSD.Cmp(d("20")) != 0 {
					t.Fatal("wrong recipient discharged customer debt", cap, e)
				}
			}
		})
	}
}

func TestPostgresStablecoinRefundSupplementPreservesBothRPCExecutionFacts(t *testing.T) {
	f := externalRefundSetup(t)
	a := f.authorize(t)
	if _, e := f.s.BeginManualRefund(f.ctx, f.owner, f.operator, f.leg.ID, "first-instruction", "send approved original 20"); e != nil {
		t.Fatal(e)
	}
	r, v := f.refund(t, a, "4")
	if out, e := f.s.Payments.RecordEVMRefund(f.ctx, f.owner, f.operator, f.merchant.ID, r, v); !errors.Is(e, payments.ErrSettlementReview) || out.State != "verified_pending_settlement" {
		t.Fatal(out, e)
	}
	if _, e := f.s.BeginManualRefund(f.ctx, f.owner, f.operator, f.leg.ID, "new-supplemental-instruction", "verified actual19; explicitly authorize remaining1"); e != nil {
		t.Fatal(e)
	}
	_, v = f.refund(t, a, "5")
	if out, e := f.s.Payments.RecordEVMRefund(f.ctx, f.owner, f.operator, f.merchant.ID, r, v); e != nil || out.State != "succeeded" {
		t.Fatal(out, e)
	}
	if a := f.balance(t); a.Balance.Cmp(d("80")) != 0 || !a.Held.IsZero() {
		t.Fatal(a)
	}
	var facts, intents int64
	_ = f.db.Table("payments_verified_facts").Where("kind='refund'").Count(&facts).Error
	_ = f.db.Table("payments_refund_intents").Where("business_refund_id=?", f.leg.ID).Count(&intents).Error
	if facts != 2 || intents != 2 {
		t.Fatal("supplement rewrote original execution", facts, intents)
	}
	cap, e := f.s.ReceiptRefundCapacity(f.ctx, nil, f.operator, f.receipt)
	if e != nil || cap.CompletedUSD.Cmp(d("20")) != 0 || !cap.ReservedUSD.IsZero() {
		t.Fatal(cap, e)
	}
}
