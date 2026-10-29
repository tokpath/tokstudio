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
	Network              string          `json:"network"`
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

func (a *Application) receiptAccountingCommand(c *gin.Context, op organization.Operator, actor string) {
	var in struct {
		FeeMoney          *ledger.Decimal `json:"fee_money"`
		OriginalFeeAmount *ledger.Decimal `json:"original_fee_amount"`
		SettledAt         *time.Time      `json:"settled_at"`
		Evidence          string          `json:"evidence"`
	}
	if !body(c, &in) {
		return
	}
	ctx := c.Request.Context()
	raw, err := a.command(c, "receipt-accounting:"+op.ID+":"+c.Param("id"), in, func(tx *gorm.DB) (any, error) {
		receipt, err := a.Commerce.GetReceipt(ctx, tx, op.ID, c.Param("id"))
		if err != nil {
			return nil, err
		}
		settings, err := ledger.GetMonetary(ctx, tx)
		if err != nil {
			return nil, err
		}
		fee := in.FeeMoney
		originalFee := in.OriginalFeeAmount
		if receipt.Currency == settings.Currency {
			if fee != nil && originalFee != nil && fee.Cmp(*originalFee) != 0 {
				return nil, commerce.ErrInvalidInput
			}
			if fee == nil {
				fee = originalFee
			}
			if originalFee == nil {
				originalFee = fee
			}
		}
		accounting := commerce.ReceiptAccounting{ReceiptID: receipt.ID, OriginalFeeAmount: originalFee, Evidence: in.Evidence, VerifiedBy: "staff:" + actor}
		if fee != nil {
			accounting.SettlementFeeConfirmed = true
			accounting.FeeMoney = *fee
			accounting.NetMoney = receipt.GrossMoney.Sub(*fee)
			accounting.SettledAt = in.SettledAt
		}
		out, err := a.Commerce.RecordReceiptAccounting(ctx, tx, accounting)
		return commandResult(out.ID, "confirmed"), err
	})
	jsonResponse(c, raw, err)
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
	fact := commerce.VerifiedReceipt{OperatorID: operator, ProviderReceiptID: in.ProviderReceiptID, GrossMoney: gross, Currency: currency, OriginalAmount: original, Network: in.Network, RecipientReference: in.RecipientReference, PayerReference: in.PayerReference, ValuationEvidence: evidence, VerificationEvidence: in.VerificationEvidence, PaidAt: in.PaidAt, FeeStatus: "pending", OriginalFeeAmount: in.OriginalFeeAmount}
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
	return fact, nil
}

func manualReceiptFields() []field {
	gross := f("settlement_amount", "已核实订单入账金额（结算币种，扣己方手续费前）", "decimal")
	gross.Help = "仅登记这笔真实原币收款对应的订单清算金额。收款方承担的手续费不减少订单交付额度。"
	original := f("original_amount", "实际原币收款总额（扣己方手续费前）", "decimal")
	original.Help = "保留真实收款币种与金额；付款人另付的发送费或链上手续费不计入。"
	fee := optional("original_fee_amount", "己方收款手续费（原币，可稍后核实）", "decimal")
	fee.Help = "只记录收款方承担的真实原币费用。未核实时留空，确实没有时填0。"
	settlementFee := receiptFeeField()
	settlementFee.Required = false
	return []field{optional("order_id", "关联订单（可选）", "text"), optional("provider_receipt_id", "收款流水或收据号（可选）", "text"), f("currency", "真实收款币种", "text"), original, gross, f("settlement_evidence", "本笔交易与订单清算金额的核实依据", "textarea"), optional("recipient_reference", "收款方资料（可选）", "text"), optional("payer_reference", "付款方资料（可选）", "text"), f("paid_at", "真实付款时刻", "datetime"), optional("verification_evidence", "收款说明或凭证（可选）", "textarea"), fee, settlementFee, optional("settled_at", "通道结算到账时间（清算手续费已核实时填写）", "datetime"), optional("network", "原网络（链上收款时填写）", "text")}
}

func receiptFeeField() field {
	fee := f("fee_money", "已核实清算手续费（结算币种）", "decimal")
	fee.Help = "仅在通道真实提供或人工明确核实结算币种费用时登记。同币费用可直接记录；不能按原币费用自动折算。该费用由收款方承担，不减少订单交付金额。"
	return fee
}

func (a *Application) allocateWholeRetailReceipt(ctx context.Context, tx *gorm.DB, actor, operator, receiptID, orderID, businessKey string) (commerce.Allocation, error) {
	var order commerce.Order
	if err := tx.WithContext(ctx).Table("commerce_orders").Where("id=? AND operator_id=?", orderID, operator).Take(&order).Error; err != nil {
		return commerce.Allocation{}, err
	}
	receipt, err := a.Commerce.GetReceipt(ctx, tx, operator, receiptID)
	if err != nil {
		return commerce.Allocation{}, err
	}
	if receipt.Provider == "manual" {
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
	if err = tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", "procurement-checkout:"+orderID).Error; err != nil {
		return salesReceiptAllocation{}, err
	}
	var checkout procurementCheckout
	found := tx.Where("order_id=?", orderID).Limit(1).Find(&checkout)
	if found.Error != nil {
		return salesReceiptAllocation{}, found.Error
	}
	if found.RowsAffected > 0 {
		var merchant struct{ Driver string }
		if err = tx.Table("payments_merchants").Select("driver").Where("id=? AND operator_id=?", checkout.MerchantID, operator).Take(&merchant).Error; err != nil {
			return salesReceiptAllocation{}, err
		}
		if merchant.Driver != "manual" {
			return salesReceiptAllocation{}, commerce.ErrPaymentUnknown
		}
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
	if _, err = a.Cooperation.FundPurchase(ctx, tx, a.Commerce, actor, orderID, receiptID, businessKey, receipt.GrossMoney); err != nil {
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
		Purpose      string
		OwnerID      string
		Unit         string
		Amount       ledger.Decimal
		BalanceAfter ledger.Decimal
	}
	lines := []line{}
	if len(ids) > 0 {
		err = a.DB.WithContext(c.Request.Context()).Table("ledger_entries e").Select("e.journal_id,a.purpose,a.owner_id,e.unit,e.amount,e.balance_after").Joins("JOIN ledger_accounts a ON a.id=e.account_id AND a.operator_id=?", op.ID).Joins("JOIN ledger_journals j ON j.id=e.journal_id AND j.operator_id=?", op.ID).Where("e.journal_id IN ?", ids).Order("e.id").Scan(&lines).Error
		if err != nil {
			handleError(c, err)
			return
		}
	}
	items := []any{}
	for _, row := range rows {
		v := values(row)
		entries := []gin.H{}
		for _, entry := range lines {
			if entry.JournalID == row.ID {
				entries = append(entries, gin.H{"purpose": entry.Purpose, "owner_id": entry.OwnerID, "unit": entry.Unit, "amount": entry.Amount, "balance_after": entry.BalanceAfter})
			}
		}
		v["entries"] = entries
		items = append(items, entity(row.ID, "recorded", 0, v))
	}
	p["items"] = items
	p["search_supported"], p["status_supported"] = false, false
	p["columns"] = []gin.H{col("kind", "账务事项", "text"), col("reason", "入账说明", "text"), col("created_at", "入账时间", "datetime"), col("entries", "账户分录", "details")}
	a.resourceResponse(c, p)
}
