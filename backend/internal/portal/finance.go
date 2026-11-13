package portal

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/cooperation"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"github.com/tokpath/tokstudio/backend/internal/payments"
	"gorm.io/gorm"
)

// Only independently supplied facts belong in the manual receipt form.
// Observation time and same-currency settlement values are derived.
type manualReceiptInput struct {
	OrderID              string          `json:"order_id"`
	ProviderReceiptID    string          `json:"provider_receipt_id"`
	GrossMoney           ledger.Decimal  `json:"gross_money"`
	SettlementAmount     ledger.Decimal  `json:"settlement_amount"`
	Currency             string          `json:"currency"`
	OriginalAmount       string          `json:"original_amount"`
	RecipientReference   string          `json:"recipient_reference"`
	PayerReference       string          `json:"payer_reference"`
	SettlementEvidence   string          `json:"settlement_evidence"`
	ValuationEvidence    string          `json:"-"`
	OriginalFeeAmount    *ledger.Decimal `json:"original_fee_amount"`
	VerificationEvidence string          `json:"verification_evidence"`
	PaidAt               *time.Time      `json:"paid_at"`
	FeeStatus            string          `json:"fee_status"`
	FeeMoney             *ledger.Decimal `json:"fee_money"`
	SettledAt            *time.Time      `json:"settled_at"`
}

func (in manualReceiptInput) verified(operator, settlementCurrency string) (commerce.VerifiedReceipt, error) {
	currency := strings.ToUpper(strings.TrimSpace(in.Currency))
	gross := in.GrossMoney
	if !in.SettlementAmount.IsZero() {
		if !gross.IsZero() && gross.Cmp(in.SettlementAmount) != 0 {
			return commerce.VerifiedReceipt{}, commerce.ErrInvalidInput
		}
		gross = in.SettlementAmount
	}
	if currency == "" || settlementCurrency == "" || in.PaidAt == nil || gross.Sign() <= 0 || (in.FeeStatus == "confirmed" && in.FeeMoney == nil && in.OriginalFeeAmount == nil) {
		return commerce.VerifiedReceipt{}, commerce.ErrInvalidInput
	}
	evidence := strings.TrimSpace(in.SettlementEvidence)
	if evidence == "" {
		evidence = in.VerificationEvidence
	}
	original := in.OriginalAmount
	if currency == settlementCurrency {
		if original != "" {
			amount, e := ledger.ParseDecimal(original)
			if e != nil || amount.Cmp(gross) != 0 {
				return commerce.VerifiedReceipt{}, commerce.ErrInvalidInput
			}
		}
		original = gross.String()
		if evidence == "" {
			evidence = "Verified receipt in the settlement currency"
		}
		if in.OriginalFeeAmount == nil && in.FeeMoney != nil {
			in.OriginalFeeAmount = in.FeeMoney
		}
	}
	amount, e := ledger.ParseDecimal(original)
	if e != nil || amount.Sign() <= 0 || evidence == "" {
		return commerce.VerifiedReceipt{}, commerce.ErrInvalidInput
	}
	fact := commerce.VerifiedReceipt{OperatorID: operator, ProviderReceiptID: in.ProviderReceiptID, GrossMoney: gross, Currency: currency, OriginalAmount: original, RecipientReference: in.RecipientReference, PayerReference: in.PayerReference, ValuationEvidence: evidence, VerificationEvidence: in.VerificationEvidence, PaidAt: in.PaidAt, FeeStatus: "pending", OriginalFeeAmount: in.OriginalFeeAmount}
	if in.OriginalFeeAmount != nil {
		if in.OriginalFeeAmount.Sign() < 0 || !in.OriginalFeeAmount.Valid() || in.OriginalFeeAmount.Cmp(amount) > 0 {
			return fact, commerce.ErrInvalidInput
		}
		net := amount.Sub(*in.OriginalFeeAmount)
		fact.OriginalNetAmount = &net
		if currency == settlementCurrency {
			fact.FeeMoney = in.OriginalFeeAmount
		}
	}
	if in.FeeMoney != nil {
		fact.FeeMoney = in.FeeMoney
	}
	if fact.FeeMoney != nil {
		if fact.FeeMoney.Sign() < 0 || fact.FeeMoney.Cmp(gross) > 0 {
			return fact, commerce.ErrInvalidInput
		}
		if currency == settlementCurrency && in.OriginalFeeAmount != nil && fact.FeeMoney.Cmp(*in.OriginalFeeAmount) != 0 {
			return fact, commerce.ErrInvalidInput
		}
		net := gross.Sub(*fact.FeeMoney)
		fact.NetMoney = &net
		fact.FeeStatus = "confirmed"
		fact.SettledAt = in.SettledAt
	}
	// The normal offline form records money actually received in person.
	// Existing explicit fee evidence, when supplied, determines its net amount.
	fact.SettlementCurrency = currency
	fact.SettlementNetAmount = &amount
	if fact.OriginalNetAmount != nil {
		fact.SettlementNetAmount = fact.OriginalNetAmount
	}
	return fact, nil
}

func manualReceiptFields() []field {
	gross := f("settlement_amount", "已核实收款总额（结算币种，含支付手续费）", "decimal")
	gross.Help = "填写付款人本次实际支付的总额。关联订单后，系统按付款时固定的规则区分订单本金和支付手续费，套餐、采购或充值权益只按本金交付。"
	original := f("original_amount", "实际原币收款总额", "decimal")
	original.Help = "记录真实收款币种与金额；付款人在外部自行支付的转账手续费不计入。"
	return []field{optional("order_id", "关联订单（可选）", "text"), optional("provider_receipt_id", "收款流水或收据号（可选）", "text"), f("currency", "真实收款币种", "text"), original, gross, f("settlement_evidence", "本笔交易与订单清算金额的核实依据", "textarea"), optional("recipient_reference", "收款方资料（可选）", "text"), optional("payer_reference", "付款方资料（可选）", "text"), f("paid_at", "真实付款时刻", "datetime"), optional("verification_evidence", "收款说明或凭证（可选）", "textarea")}
}

func (a *Application) allocateWholeRetailReceipt(ctx context.Context, tx *gorm.DB, actor, operator, receiptID, orderID, businessKey string) (commerce.Allocation, error) {
	if err := payments.LockOrderPayment(tx, orderID); err != nil {
		return commerce.Allocation{}, err
	}
	var order commerce.Order
	if err := tx.WithContext(ctx).Table("commerce_orders").Where("id=? AND operator_id=?", orderID, operator).Take(&order).Error; err != nil {
		return commerce.Allocation{}, err
	}
	receipt, err := a.Commerce.GetReceipt(ctx, tx, operator, receiptID)
	if err != nil {
		return commerce.Allocation{}, err
	}
	if err = a.validateCollectionReceipt(ctx, tx, operator, orderID, &receipt); err != nil {
		return commerce.Allocation{}, err
	}
	if receipt.Provider == "manual" {
		received, err := payments.HasOrderReceipt(ctx, tx, operator, orderID)
		if err != nil {
			return commerce.Allocation{}, err
		}
		if received {
			return commerce.Allocation{}, commerce.ErrPaymentUnknown
		}
		var checkout checkoutRow
		found := tx.WithContext(ctx).Where("order_id=?", order.ID).Limit(1).Find(&checkout)
		if found.Error != nil {
			return commerce.Allocation{}, found.Error
		}
		if checkout.MerchantID != nil {
			merchant, err := a.Payments.Merchant(ctx, operator, *checkout.MerchantID)
			if err != nil {
				return commerce.Allocation{}, err
			}
			if merchant.Driver != "manual" {
				return commerce.Allocation{}, commerce.ErrPaymentUnknown
			}
		}
	}
	return a.Commerce.AllocateReceipt(ctx, tx, commerce.AllocateReceiptInput{ActorStaffID: actor, OperatorID: operator, BusinessKey: businessKey, ReceiptID: receiptID, OrderID: orderID, AmountMoney: order.DueMoney.Sub(order.InternalMoney).Sub(order.CommissionMoney)})
}

// Receipt selection spans both types of sales order, but settlement always
// delegates to the original domain and its single-payment/idempotency checks.
type salesReceiptAllocation struct {
	ID              string
	DeliveryPending bool
}

func (r salesReceiptAllocation) result(status string) gin.H {
	out := commandResult(r.ID, status)
	if r.DeliveryPending {
		out["message"] = "收款已关联，采购额度待交付；请在原订单重试交付。"
	}
	return out
}

func (a *Application) allocateWholeSalesReceipt(ctx context.Context, tx *gorm.DB, actor, operator, receiptID, orderID, businessKey string) (salesReceiptAllocation, error) {
	var retail commerce.Order
	err := tx.WithContext(ctx).Table("commerce_orders").Where("id=? AND operator_id=?", orderID, operator).Take(&retail).Error
	if err == nil {
		allocation, err := a.allocateWholeRetailReceipt(ctx, tx, actor, operator, receiptID, orderID, businessKey)
		return salesReceiptAllocation{ID: allocation.ID}, err
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return salesReceiptAllocation{}, err
	}
	var order cooperation.PurchaseOrder
	if err = tx.WithContext(ctx).Where("id=? AND supplier_id=?", orderID, operator).Take(&order).Error; err != nil {
		return salesReceiptAllocation{}, err
	}
	if err = a.Organization.RequirePermission(ctx, tx, actor, operator, "payments.manage"); err != nil {
		return salesReceiptAllocation{}, err
	}
	return a.allocatePurchaseReceipt(ctx, tx, actor, order, receiptID, businessKey)
}

func (a *Application) allocatePurchaseReceipt(ctx context.Context, tx *gorm.DB, actor string, order cooperation.PurchaseOrder, receiptID, businessKey string) (salesReceiptAllocation, error) {
	operator, orderID := order.SupplierID, order.ID
	if err := payments.LockOrderPayment(tx, orderID); err != nil {
		return salesReceiptAllocation{}, err
	}
	received, err := payments.HasOrderReceipt(ctx, tx, operator, orderID)
	if err != nil {
		return salesReceiptAllocation{}, err
	}
	if received {
		return salesReceiptAllocation{}, commerce.ErrPaymentUnknown
	}
	driver, _, err := procurementPaymentState(ctx, tx, order)
	if err != nil {
		return salesReceiptAllocation{}, err
	}
	if driver != "" && driver != "manual" {
		return salesReceiptAllocation{}, commerce.ErrPaymentUnknown
	}
	automatic, err := a.AutoReplenishments.UsesAutomaticPayment(ctx, tx, orderID)
	if err != nil {
		return salesReceiptAllocation{}, err
	}
	if automatic {
		return salesReceiptAllocation{}, commerce.ErrPaymentUnknown
	}
	receipt, err := a.Commerce.GetReceipt(ctx, tx, operator, receiptID)
	if err != nil {
		return salesReceiptAllocation{}, err
	}
	if err = a.validateCollectionReceipt(ctx, tx, operator, orderID, &receipt); err != nil {
		return salesReceiptAllocation{}, err
	}
	if _, err = a.Cooperation.FundPurchase(ctx, tx, a.Commerce, actor, orderID, receiptID, businessKey, receipt.GrossMoney.Sub(receipt.PaymentFeeMoney)); err != nil {
		return salesReceiptAllocation{}, err
	}
	// The payment remains allocated even if stock cannot be delivered now.
	// Retrying delivery must never require recording or allocating another payment.
	deliveryErr := tx.Transaction(func(delivery *gorm.DB) error {
		_, err := a.Cooperation.FulfillPurchase(ctx, delivery, orderID, actor)
		return err
	})
	var allocation cooperation.PurchaseAllocation
	if err := tx.Where("order_id=? AND receipt_id=?", orderID, receiptID).Take(&allocation).Error; err != nil {
		return salesReceiptAllocation{}, err
	}
	return salesReceiptAllocation{ID: allocation.ID, DeliveryPending: deliveryErr != nil}, nil
}

func (a *Application) ledgerJournalsResource(c *gin.Context, op organization.Operator, p gin.H) {
	rows, err := a.Ledger.ListJournals(c.Request.Context(), nil, op.ID, "", c.Query("cursor"), 51)
	if err != nil {
		handleError(c, err)
		return
	}
	if len(rows) > 50 {
		rows = rows[:50]
		p["next_cursor"] = rows[49].ID
	}
	ids := []string{}
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	type line struct {
		JournalID    string
		AccountID    string
		Purpose      string
		OwnerID      string
		Unit         string
		Amount       ledger.Decimal
		BalanceAfter ledger.Decimal
	}
	lines := []line{}
	if len(ids) > 0 {
		err = a.DB.WithContext(c.Request.Context()).Table("ledger_entries e").Select("e.journal_id,e.account_id,a.purpose,a.owner_id,e.unit,e.amount,e.balance_after").Joins("JOIN ledger_accounts a ON a.id=e.account_id AND a.operator_id=?", op.ID).Joins("JOIN ledger_journals j ON j.id=e.journal_id AND j.operator_id=?", op.ID).Where("e.journal_id IN ?", ids).Order("e.id").Scan(&lines).Error
		if err != nil {
			handleError(c, err)
			return
		}
	}
	accountIDs, sourceIDs := []string{}, []string{}
	for _, entry := range lines {
		accountIDs = append(accountIDs, entry.AccountID)
	}
	for _, row := range rows {
		sourceIDs = append(sourceIDs, row.SourceID)
	}
	accounts := []ledger.Account{}
	if len(accountIDs) > 0 {
		if err = a.DB.WithContext(c.Request.Context()).Where("operator_id=? AND id IN ?", op.ID, accountIDs).Find(&accounts).Error; err != nil {
			handleError(c, err)
			return
		}
	}
	displays, sources, err := a.ledgerDisplays(c, op.ID, accounts, sourceIDs)
	if err != nil {
		handleError(c, err)
		return
	}
	items := []any{}
	for _, row := range rows {
		v := values(row)
		entries := []gin.H{}
		for _, entry := range lines {
			if entry.JournalID == row.ID {
				entries = append(entries, gin.H{"account_id": entry.AccountID, "display": displays[entry.AccountID], "purpose": entry.Purpose, "owner_id": entry.OwnerID, "unit": entry.Unit, "amount": entry.Amount, "balance_after": entry.BalanceAfter})
			}
		}
		v["entries"] = entries
		v["source"] = sources[row.SourceID]
		items = append(items, entity(row.ID, "recorded", 0, v))
	}
	p["items"] = items
	p["search_supported"], p["status_supported"] = false, false
	p["columns"] = []gin.H{col("kind", "账务事项", "text"), col("reason", "入账说明", "text"), col("created_at", "入账时间", "datetime"), col("entries", "账户分录", "details")}
	a.resourceResponse(c, p)
}
