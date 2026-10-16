package payments

import (
	"context"
	"encoding/json"
	"math/big"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/ledger"
)

// EVMWallet verifies an explicitly configured ERC-20 Transfer log using a
// read-only JSON-RPC endpoint. It has no signing key, sendTransaction, payout,
// refund, address generation or custody capability.
type EVMWallet struct{ HTTP *Transport }

func (EVMWallet) Name() string { return "evm_wallet" }
func (EVMWallet) Validate(m Merchant) []string {
	missing := required(m, "rpc_url", "chain_id", "token_contract", "token_decimals", "wallet_address", "asset", "min_confirmations")
	if forbiddenWalletSecret(m.Credentials) {
		missing = append(missing, "private_keys_are_not_accepted")
	}
	for _, key := range []string{"token_contract", "wallet_address"} {
		if !validHex(m.Credentials[key], 20) {
			missing = append(missing, "valid_"+key)
		}
	}
	dec, e := strconv.ParseUint(m.Credentials["token_decimals"], 10, 8)
	if e != nil || dec > 18 {
		missing = append(missing, "token_decimals_0_to_18")
	}
	confirmations, e := strconv.ParseUint(m.Credentials["min_confirmations"], 10, 32)
	if e != nil || confirmations == 0 || confirmations > 100000 {
		missing = append(missing, "positive_min_confirmations")
	}
	return missing
}
func forbiddenWalletSecret(credentials map[string]string) bool {
	for key, value := range credentials {
		if value == "" {
			continue
		}
		key = strings.NewReplacer("_", "", "-", "", " ", "").Replace(strings.ToLower(key))
		switch key {
		case "privatekey", "walletprivatekey", "mnemonic", "seed", "seedphrase", "signingkey", "secretkey":
			return true
		}
	}
	return false
}
func validHex(value string, n int) bool {
	if !strings.HasPrefix(value, "0x") || len(value) != 2+n*2 {
		return false
	}
	_, ok := new(big.Int).SetString(value[2:], 16)
	return ok
}
func hexInt(value string) (*big.Int, error) {
	if !strings.HasPrefix(value, "0x") || len(value) < 3 {
		return nil, ErrInvalid
	}
	n, ok := new(big.Int).SetString(value[2:], 16)
	if !ok || n.Sign() < 0 {
		return nil, ErrInvalid
	}
	return n, nil
}
func (d EVMWallet) rpc(ctx context.Context, m Merchant, method string, params any, result any) error {
	target := m.Credentials["rpc_url"]
	u, e := url.Parse(target)
	if e != nil || u.User != nil || u.Host == "" || u.Fragment != "" {
		return ErrInvalid
	}
	allowed := false
	for _, host := range d.HTTP.AllowedRPCHosts {
		if host == u.Hostname() && u.Scheme == "https" {
			allowed = true
		}
	}
	if d.HTTP.AllowTestLoopback && u.Scheme == "http" && u.Hostname() == "127.0.0.1" {
		allowed = true
	}
	if !allowed {
		return ErrForbidden
	}
	body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
	raw, _, e := d.HTTP.do(ctx, http.MethodPost, target, body, http.Header{"Content-Type": []string{"application/json"}})
	if e != nil {
		return e
	}
	var envelope struct {
		JSONRPC string `json:"jsonrpc"`
		ID      int
		Result  json.RawMessage
		Error   json.RawMessage
	}
	if json.Unmarshal(raw, &envelope) != nil || envelope.ID != 1 || envelope.JSONRPC != "2.0" || (len(envelope.Error) > 0 && string(envelope.Error) != "null") {
		return ErrProvider
	}
	if len(envelope.Result) == 0 || string(envelope.Result) == "null" {
		return ErrNotFinal
	}
	if json.Unmarshal(envelope.Result, result) != nil {
		return ErrProvider
	}
	return nil
}
func (d EVMWallet) Create(_ context.Context, m Merchant, o Order) (Checkout, error) {
	if e := ready(m, d); e != nil {
		return Checkout{}, e
	}
	if o.Validate(m) != nil || o.Money.Currency != m.Credentials["asset"] {
		return Checkout{}, ErrInvalid
	}
	return Checkout{State: "pending", PaymentAddress: m.Credentials["wallet_address"], PayMoney: &o.Money}, nil
}
func (d EVMWallet) Lookup(ctx context.Context, m Merchant, o Order) (Fact, error) {
	return d.lookupTransfer(ctx, m, o, "", true)
}

// lookupTransfer is read-only. Refund verification checks the original paying
// wallet and records the actual recipient, even if it differs from the approved
// destination; that real transfer remains pending business settlement.
func (d EVMWallet) lookupTransfer(ctx context.Context, m Merchant, o Order, sender string, enforceRecipient bool) (Fact, error) {
	if e := ready(m, d); e != nil {
		return Fact{}, e
	}
	if o.Validate(m) != nil || o.Money.Currency != m.Credentials["asset"] {
		return Fact{}, ErrInvalid
	}
	txHash, index, ok := strings.Cut(o.ProviderID, "#")
	if !ok || !validHex(txHash, 32) {
		return Fact{}, ErrInvalid
	}
	logIndex, e := hexInt(index)
	if e != nil {
		return Fact{}, e
	}
	var chain string
	if e = d.rpc(ctx, m, "eth_chainId", []any{}, &chain); e != nil {
		return Fact{}, e
	}
	actualChain, e := hexInt(chain)
	expectedChain, valid := new(big.Int).SetString(m.Credentials["chain_id"], 10)
	if e != nil || !valid || expectedChain.Sign() <= 0 || actualChain.Cmp(expectedChain) != 0 {
		return Fact{}, ErrConflict
	}
	var decimalsHex string
	if e = d.rpc(ctx, m, "eth_call", []any{map[string]string{"to": m.Credentials["token_contract"], "data": "0x313ce567"}, "latest"}, &decimalsHex); e != nil {
		return Fact{}, e
	}
	decimals, e := hexInt(decimalsHex)
	configured, _ := strconv.Atoi(m.Credentials["token_decimals"])
	if e != nil || !decimals.IsInt64() || decimals.Int64() != int64(configured) {
		return Fact{}, ErrConflict
	}
	var receipt struct {
		TransactionHash string `json:"transactionHash"`
		BlockHash       string `json:"blockHash"`
		BlockNumber     string `json:"blockNumber"`
		Status          string
		Logs            []struct {
			Address         string
			Topics          []string
			Data            string
			LogIndex        string `json:"logIndex"`
			TransactionHash string `json:"transactionHash"`
			BlockHash       string `json:"blockHash"`
			Removed         bool
		}
	}
	if e = d.rpc(ctx, m, "eth_getTransactionReceipt", []any{txHash}, &receipt); e != nil {
		return Fact{}, e
	}
	if !strings.EqualFold(receipt.TransactionHash, txHash) || receipt.Status != "0x1" || !validHex(receipt.BlockHash, 32) {
		return Fact{}, ErrConflict
	}
	number, e := hexInt(receipt.BlockNumber)
	if e != nil {
		return Fact{}, e
	}
	var headHex string
	if e = d.rpc(ctx, m, "eth_blockNumber", []any{}, &headHex); e != nil {
		return Fact{}, e
	}
	head, e := hexInt(headHex)
	if e != nil || head.Cmp(number) < 0 {
		return Fact{}, ErrNotFinal
	}
	confirmed := new(big.Int).Add(new(big.Int).Sub(head, number), big.NewInt(1))
	minimum, _ := new(big.Int).SetString(m.Credentials["min_confirmations"], 10)
	if confirmed.Cmp(minimum) < 0 {
		return Fact{}, ErrNotFinal
	}
	var block struct{ Hash, Number, Timestamp string }
	if e = d.rpc(ctx, m, "eth_getBlockByNumber", []any{receipt.BlockNumber, false}, &block); e != nil {
		return Fact{}, e
	}
	if !strings.EqualFold(block.Hash, receipt.BlockHash) || block.Number != receipt.BlockNumber {
		return Fact{}, ErrConflict
	}
	timestamp, e := hexInt(block.Timestamp)
	if e != nil || !timestamp.IsInt64() {
		return Fact{}, ErrInvalid
	}
	paidAt := time.Unix(timestamp.Int64(), 0).UTC()
	if paidAt.After(time.Now().Add(5 * time.Minute)) {
		return Fact{}, ErrInvalid
	}
	const transferTopic = "0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef"
	for _, log := range receipt.Logs {
		li, e := hexInt(log.LogIndex)
		if e != nil || li.Cmp(logIndex) != 0 {
			continue
		}
		if log.Removed || !strings.EqualFold(log.Address, m.Credentials["token_contract"]) || !strings.EqualFold(log.TransactionHash, txHash) || !strings.EqualFold(log.BlockHash, receipt.BlockHash) || len(log.Topics) != 3 || !strings.EqualFold(log.Topics[0], transferTopic) || !validHex(log.Topics[1], 32) || !validHex(log.Topics[2], 32) || !validHex(log.Data, 32) {
			return Fact{}, ErrConflict
		}
		actualSender, actualRecipient := "0x"+log.Topics[1][26:], "0x"+log.Topics[2][26:]
		if (enforceRecipient && !strings.EqualFold(actualRecipient, m.Credentials["wallet_address"])) || (sender != "" && !strings.EqualFold(actualSender, sender)) {
			return Fact{}, ErrConflict
		}
		units, e := hexInt(log.Data)
		if e != nil || units.Sign() <= 0 {
			return Fact{}, ErrInvalid
		}
		quantity := new(big.Rat).SetFrac(units, new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(configured)), nil))
		amount, remainder := ledger.Quantize(quantity)
		if remainder.Sign() != 0 || !amount.Valid() {
			return Fact{}, ErrInvalid
		}
		fact, e := receiptFact(m, o, txHash+"#0x"+logIndex.Text(16), Money{Currency: o.Money.Currency, Amount: amount}, &paidAt, map[string]any{"chain_id": actualChain.String(), "transaction_hash": strings.ToLower(txHash), "log_index": "0x" + logIndex.Text(16), "block_hash": block.Hash, "block_number": block.Number, "confirmations": confirmed.String(), "token_contract": strings.ToLower(log.Address), "token_decimals": configured, "recipient": strings.ToLower(actualRecipient), "sender": strings.ToLower(actualSender), "raw_units": units.String(), "rpc_verified": true})
		if e != nil {
			return Fact{}, e
		}
		fact.ID = "evm:" + actualChain.String() + ":" + strings.ToLower(txHash) + ":" + logIndex.Text(16)
		return fact, nil
	}
	return Fact{}, ErrNotFinal
}
func (EVMWallet) Refund(context.Context, Merchant, Refund) (RefundStatus, error) {
	return RefundStatus{}, ErrUnsupported
}
func (EVMWallet) LookupRefund(context.Context, Merchant, Refund) (RefundStatus, error) {
	return RefundStatus{}, ErrUnsupported
}
func (EVMWallet) VerifyWebhook(context.Context, Merchant, Webhook) (Hint, error) {
	return Hint{}, ErrUnsupported
}
