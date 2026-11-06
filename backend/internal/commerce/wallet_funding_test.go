package commerce

import (
	"testing"

	"github.com/google/uuid"
)

func (f fixture) separatelyFundedTopups(t *testing.T) ([]Order, []Receipt) {
	t.Helper()
	var orders []Order
	var receipts []Receipt
	for _, amount := range []string{"30", "70"} {
		key := uuid.NewString()
		o, err := f.s.CreateTopUpOrder(f.ctx, nil, CreateTopUpInput{CustomerID: f.customer, OperatorID: f.operator, BusinessKey: key, ExpectedOwnershipVersion: 1, AmountMoney: d(amount)})
		if err != nil {
			t.Fatal(err)
		}
		r := f.receipt(t, key, amount, "0", o.CreatedAt)
		f.allocate(t, key, r, o, amount)
		receipts = append(receipts, r)
		orders = append(orders, o)
		if _, err = f.s.Fulfill(f.ctx, nil, o.ID, "test:delivery"); err != nil {
			t.Fatal(err)
		}
	}
	return orders, receipts
}

func TestPostgresPooledWalletPurchaseDoesNotAllocateTopups(t *testing.T) {
	f := setup(t)
	f.separatelyFundedTopups(t)
	o := f.order(t, "pooled-purchase", "80", "0")
	if _, e := f.s.Fulfill(f.ctx, nil, o.ID, "test:delivery"); e != nil {
		t.Fatal(e)
	}
	a := f.balance(t, "customer:money")
	if a.Balance.Cmp(d("20")) != 0 || !a.Held.IsZero() {
		t.Fatal(a)
	}
	for _, table := range []string{"commerce_wallet_funding_lots", "commerce_wallet_funding_allocations", "commerce_wallet_funding_commands"} {
		var count int64
		if e := f.db.Table(table).Count(&count).Error; e != nil || count != 0 {
			t.Fatal("obsolete source allocation", table, count, e)
		}
	}
}
