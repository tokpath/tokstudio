package portal

import (
	"context"
	"errors"
	"math/big"

	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/operations"
	"gorm.io/gorm"
)

var (
	errInvoiceTopUp  = errors.New("充值不能开票。请选择已完成的套餐购买或续费。")
	errInvoiceAmount = errors.New("本次申请超过这笔购买还可以开的金额。")
	errInvoiceStage  = errors.New("这笔购买按当前开票阶段还不能开票。余额充值不能开票；用余额支付的套餐，要等经营方把开票阶段设为完成购买或实际消费。")
)

type invoiceRoom struct {
	party  ledger.Decimal
	global ledger.Decimal
}

// customerInvoiceLines turns a selected purchase into the original receipts
// that funded it. A balance top-up is not a purchase and is rejected. Receipt
// ids that already funded a real purchase stay valid for older requests.
func (a *Application) customerInvoiceLines(ctx context.Context, customer, issuer string, lines []operations.InvoiceLine) ([]operations.InvoiceLine, error) {
	if len(lines) == 0 || len(lines) > 100 {
		return nil, operations.ErrInvalid
	}
	policy := operations.InvoicePolicy{OperatorID: issuer, PartyKind: "customer"}
	if err := a.DB.WithContext(ctx).Where("operator_id=? AND party_kind='customer'", issuer).Limit(1).Find(&policy).Error; err != nil {
		return nil, err
	}
	if !policy.Enabled {
		return nil, operations.ErrNotReady
	}
	rooms := map[string]invoiceRoom{}
	taken := map[string]ledger.Decimal{}
	out := []operations.InvoiceLine{}
	for _, line := range lines {
		if line.AmountMoney.Sign() <= 0 || !line.AmountMoney.Valid() {
			return nil, operations.ErrInvalid
		}
		funding, err := a.Commerce.OrderInvoiceFunding(ctx, nil, issuer, customer, line.ReceiptID)
		if err != nil {
			return nil, err
		}
		if funding.Found {
			expanded, err := a.expandPurchaseInvoice(ctx, customer, issuer, policy, funding, line.AmountMoney, rooms, taken)
			if err != nil {
				return nil, err
			}
			out = append(out, expanded...)
			continue
		}
		expanded, err := a.keepInvoiceReceipt(ctx, customer, issuer, policy, line, rooms, taken)
		if err != nil {
			return nil, err
		}
		out = append(out, expanded)
	}
	return mergeInvoiceLines(out), nil
}

func (a *Application) expandPurchaseInvoice(ctx context.Context, customer, issuer string, policy operations.InvoicePolicy, funding commerce.OrderInvoiceFunding, requested ledger.Decimal, rooms map[string]invoiceRoom, taken map[string]ledger.Decimal) ([]operations.InvoiceLine, error) {
	if funding.Status != "completed" || (funding.Kind != "package" && funding.Kind != "renewal") {
		return nil, errInvoiceTopUp
	}
	remaining := requested
	covered := ledger.Zero()
	out := []operations.InvoiceLine{}
	for _, slice := range funding.Slices {
		if slice.AmountMoney.Sign() <= 0 {
			continue
		}
		cap, err := purchaseSliceCap(policy.Stage, policy.MaximumRatio, slice.AmountMoney, funding.Consumed, funding.DueMoney, slice.Direct)
		if err != nil {
			return nil, err
		}
		covered = covered.Add(cap)
		if cap.Sign() <= 0 || remaining.Sign() <= 0 {
			continue
		}
		room, err := a.invoiceReceiptRoom(ctx, issuer, customer, slice.ReceiptID, policy, rooms)
		if err != nil {
			return nil, err
		}
		take := minDecimal(cap, room.party.Sub(taken[slice.ReceiptID]), room.global.Sub(taken[slice.ReceiptID]), remaining)
		if take.Sign() <= 0 {
			continue
		}
		taken[slice.ReceiptID] = taken[slice.ReceiptID].Add(take)
		remaining = remaining.Sub(take)
		out = append(out, operations.InvoiceLine{ReceiptID: slice.ReceiptID, AmountMoney: take})
	}
	if remaining.Sign() > 0 {
		if covered.Sign() <= 0 {
			return nil, errInvoiceStage
		}
		return nil, errInvoiceAmount
	}
	return out, nil
}

func (a *Application) keepInvoiceReceipt(ctx context.Context, customer, issuer string, policy operations.InvoicePolicy, line operations.InvoiceLine, rooms map[string]invoiceRoom, taken map[string]ledger.Decimal) (operations.InvoiceLine, error) {
	basis, err := a.Operations.Invoices.InvoiceBasis(ctx, nil, issuer, customer, line.ReceiptID)
	if err != nil {
		if errors.Is(err, commerce.ErrInvalidInput) || errors.Is(err, commerce.ErrNotFound) || errors.Is(err, gorm.ErrRecordNotFound) {
			return line, errInvoiceTopUp
		}
		return line, err
	}
	eligible, err := operations.EligibleInvoiceAmount(basis, policy.Stage, policy.MaximumRatio)
	if err != nil {
		return line, err
	}
	if eligible.Sign() <= 0 {
		if basis.PurchaseMoney.Sign() > 0 {
			return line, errInvoiceStage
		}
		return line, errInvoiceTopUp
	}
	room, err := a.invoiceReceiptRoom(ctx, issuer, customer, line.ReceiptID, policy, rooms)
	if err != nil {
		return line, err
	}
	if line.AmountMoney.Cmp(room.party.Sub(taken[line.ReceiptID])) > 0 || line.AmountMoney.Cmp(room.global.Sub(taken[line.ReceiptID])) > 0 {
		return line, errInvoiceAmount
	}
	taken[line.ReceiptID] = taken[line.ReceiptID].Add(line.AmountMoney)
	return line, nil
}

func (a *Application) invoiceReceiptRoom(ctx context.Context, issuer, customer, receipt string, policy operations.InvoicePolicy, rooms map[string]invoiceRoom) (invoiceRoom, error) {
	if room, ok := rooms[receipt]; ok {
		return room, nil
	}
	basis, err := a.Operations.Invoices.InvoiceBasis(ctx, nil, issuer, customer, receipt)
	if err != nil {
		return invoiceRoom{}, err
	}
	eligible, err := operations.EligibleInvoiceAmount(basis, policy.Stage, policy.MaximumRatio)
	if err != nil {
		return invoiceRoom{}, err
	}
	global := eligible
	if basis.GlobalEligibleMoney != nil {
		global = *basis.GlobalEligibleMoney
	}
	var partyUsed, globalUsed ledger.Decimal
	db := a.DB.WithContext(ctx)
	if err = db.Raw(`SELECT COALESCE(sum(src.amount_money),0)::text FROM operations_invoice_sources src JOIN operations_invoices i ON i.id=src.invoice_id WHERE i.operator_id=? AND i.party_kind='customer' AND i.customer_id=? AND src.receipt_id=? AND i.state NOT IN ('rejected','voided')`, issuer, customer, receipt).Row().Scan(&partyUsed); err != nil {
		return invoiceRoom{}, err
	}
	if err = db.Raw(`SELECT COALESCE(sum(src.amount_money),0)::text FROM operations_invoice_sources src JOIN operations_invoices i ON i.id=src.invoice_id WHERE i.operator_id=? AND src.receipt_id=? AND i.state NOT IN ('rejected','voided')`, issuer, receipt).Row().Scan(&globalUsed); err != nil {
		return invoiceRoom{}, err
	}
	room := invoiceRoom{party: eligible.Sub(partyUsed), global: global.Sub(globalUsed)}
	if room.party.Sign() < 0 {
		room.party = ledger.Zero()
	}
	if room.global.Sign() < 0 {
		room.global = ledger.Zero()
	}
	rooms[receipt] = room
	return room, nil
}

func purchaseSliceCap(stage string, ratio, amount, consumed, due ledger.Decimal, direct bool) (ledger.Decimal, error) {
	value := amount
	switch stage {
	case "receipt":
		if !direct {
			value = ledger.Zero()
		}
	case "purchase":
	case "consumption":
		if due.Sign() <= 0 || amount.Sign() < 0 || amount.Cmp(due) > 0 || consumed.Sign() < 0 {
			return ledger.Zero(), operations.ErrInvalid
		}
		part := new(big.Rat).Mul(consumed.Rat(), new(big.Rat).Quo(amount.Rat(), due.Rat()))
		value, _ = ledger.Quantize(part)
	default:
		return ledger.Zero(), operations.ErrInvalid
	}
	if value.Sign() < 0 || ratio.Sign() < 0 || !ratio.Valid() {
		return ledger.Zero(), operations.ErrInvalid
	}
	out, _ := ledger.Quantize(new(big.Rat).Mul(value.Rat(), ratio.Rat()))
	return out, nil
}

func minDecimal(values ...ledger.Decimal) ledger.Decimal {
	out := values[0]
	for _, value := range values[1:] {
		if value.Cmp(out) < 0 {
			out = value
		}
	}
	if out.Sign() < 0 {
		return ledger.Zero()
	}
	return out
}

func mergeInvoiceLines(lines []operations.InvoiceLine) []operations.InvoiceLine {
	index := map[string]int{}
	out := []operations.InvoiceLine{}
	for _, line := range lines {
		if at, ok := index[line.ReceiptID]; ok {
			out[at].AmountMoney = out[at].AmountMoney.Add(line.AmountMoney)
			continue
		}
		index[line.ReceiptID] = len(out)
		out = append(out, line)
	}
	return out
}
