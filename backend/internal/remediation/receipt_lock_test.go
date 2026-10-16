package remediation

import (
	"fmt"
	"testing"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"gorm.io/gorm"
)

func TestPostgresAllocationTakesBusinessFenceBeforeReceiptAndOrderRows(t *testing.T) {
	f := setup(t)
	o, e := f.s.Commerce.CreateOrder(f.ctx, nil, commerce.CreateOrderInput{OperatorID: f.operator, CustomerID: f.customer, QuoteID: f.quote.ID, BusinessKey: "allocation-lock-order", ExpectedOwnershipVersion: 1})
	if e != nil {
		t.Fatal(e)
	}
	r := f.receipt(t, "allocation-lock-receipt", "80")
	pid := make(chan int, 1)
	done := make(chan error, 1)
	e = f.db.Transaction(func(remediationTx *gorm.DB) error {
		// This is the real remediation lock order: business, affected order,
		// then the original receipt. The allocator must wait at business before
		// taking the receipt, otherwise these two flows form a deadlock cycle.
		if e := f.s.Cooperation.Lock(f.ctx, remediationTx, f.operator); e != nil {
			return e
		}
		if e := remediationTx.Exec("SELECT id FROM commerce_orders WHERE id=? FOR UPDATE", o.ID).Error; e != nil {
			return e
		}
		go func() {
			done <- f.db.Transaction(func(allocationTx *gorm.DB) error {
				var processID int
				if e := allocationTx.Raw("SELECT pg_backend_pid()").Scan(&processID).Error; e != nil {
					return e
				}
				pid <- processID
				_, e := f.s.Commerce.AllocateReceipt(f.ctx, allocationTx, commerce.AllocateReceiptInput{OperatorID: f.operator, ReceiptID: r.ID, OrderID: o.ID, BusinessKey: "ordered-allocation", AmountUSD: d("80")})
				return e
			})
		}()
		processID := <-pid
		deadline := time.Now().Add(5 * time.Second)
		for {
			var blocked bool
			if e := f.db.Raw("SELECT cardinality(pg_blocking_pids(?))>0", processID).Scan(&blocked).Error; e != nil {
				return e
			}
			if blocked {
				break
			}
			if time.Now().After(deadline) {
				return fmt.Errorf("allocation never reached transaction fence")
			}
			time.Sleep(10 * time.Millisecond)
		}
		return remediationTx.Exec("SELECT id FROM commerce_receipts WHERE id=? FOR UPDATE NOWAIT", r.ID).Error
	})
	allocationError := <-done
	if e != nil {
		t.Fatal("allocator locked receipt before business fence", e)
	}
	if allocationError != nil {
		t.Fatal(allocationError)
	}
}
