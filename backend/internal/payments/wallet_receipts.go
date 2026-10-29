package payments

import (
	"context"
	"strings"
)

// VerifyWalletReceipt is a trusted exact-order command. An authenticated caller
// must select its own checkout before invoking it. A transaction reference is
// only a lookup hint: network, token, destination, event and confirmations are
// independently read from the configured RPC; only the verified fact is saved.
func (s *Service) VerifyWalletReceipt(ctx context.Context, operator, merchant, orderID, transactionHash, logIndex string) (Fact, error) {
	if !validHex(transactionHash, 32) {
		return Fact{}, ErrInvalid
	}
	index, e := hexInt(logIndex)
	if e != nil {
		return Fact{}, e
	}
	m, e := s.Merchant(ctx, operator, merchant)
	if e != nil {
		return Fact{}, e
	}
	if m.Driver != "evm_wallet" {
		return Fact{}, ErrUnsupported
	}
	o, e := s.loadOrder(ctx, m, orderID)
	if e != nil {
		return Fact{}, e
	}
	m, e = s.historicalMerchant(ctx, m, o)
	if e != nil {
		return Fact{}, e
	}
	o.ProviderID = strings.ToLower(transactionHash) + "#0x" + index.Text(16)
	d, ok := s.Drivers[m.Driver]
	if !ok {
		return Fact{}, ErrNotConfigured
	}
	fact, e := d.Lookup(ctx, m, o)
	if e != nil {
		return Fact{}, e
	}
	if e = s.RecordFact(ctx, m, o, fact); e != nil {
		return fact, e
	}
	return fact, nil
}
