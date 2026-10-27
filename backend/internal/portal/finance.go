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
// Observation time, native USD equivalence and net value are derived.
type manualReceiptInput struct {
	OrderID              string          `json:"order_id"`
	ProviderReceiptID    string          `json:"provider_receipt_id"`
	GrossUSD             ledger.Decimal  `json:"gross_usd"`
	Currency             string          `json:"currency"`
	OriginalAmount       string          `json:"original_amount"`
	Network              string          `json:"network"`
	RecipientReference   string          `json:"recipient_reference"`
	PayerReference       string          `json:"payer_reference"`
	ValuationEvidence    string          `json:"valuation_evidence"`
	VerificationEvidence string          `json:"verification_evidence"`
	PaidAt               *time.Time      `json:"paid_at"`
	FeeStatus            string          `json:"fee_status"`
	FeeUSD               *ledger.Decimal `json:"fee_usd"`
	SettledAt            *time.Time      `json:"settled_at"`
}

func (a *Application) receiptAccountingCommand(c *gin.Context, op organization.Operator, actor string) {
	var in struct {
		FeeUSD    ledger.Decimal `json:"fee_usd"`
		SettledAt *time.Time     `json:"settled_at"`
		Evidence  string         `json:"evidence"`
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
		out, err := a.Commerce.RecordReceiptAccounting(ctx, tx, commerce.ReceiptAccounting{ReceiptID: receipt.ID, FeeUSD: in.FeeUSD, NetUSD: receipt.GrossUSD.Sub(in.FeeUSD), SettledAt: in.SettledAt, Evidence: in.Evidence, VerifiedBy: "staff:" + actor})
		return commandResult(out.ID, "confirmed"), err
	})
	jsonResponse(c, raw, err)
}

func (in manualReceiptInput) verified(operator string) (commerce.VerifiedReceipt, error) {
	currency := strings.ToUpper(strings.TrimSpace(in.Currency))
	if currency == "" {
		currency = "USD"
	}
	if in.FeeStatus == "" {
		in.FeeStatus = "pending"
	}
	if in.PaidAt == nil {
		return commerce.VerifiedReceipt{}, commerce.ErrInvalidInput
	}
	fact := commerce.VerifiedReceipt{OperatorID: operator, ProviderReceiptID: in.ProviderReceiptID, GrossUSD: in.GrossUSD, Currency: currency, OriginalAmount: in.OriginalAmount, Network: in.Network, RecipientReference: in.RecipientReference, PayerReference: in.PayerReference, ValuationEvidence: in.ValuationEvidence, VerificationEvidence: in.VerificationEvidence, PaidAt: in.PaidAt, FeeStatus: in.FeeStatus, FeeUSD: in.FeeUSD, SettledAt: in.SettledAt}
	if currency == "USD" {
		fact.OriginalAmount = in.GrossUSD.String()
		fact.ValuationEvidence = "Native USD bank receipt; no currency conversion"
	}
	if in.FeeStatus == "confirmed" {
		if in.FeeUSD == nil {
			return fact, commerce.ErrInvalidInput
		}
		net := in.GrossUSD.Sub(*in.FeeUSD)
		fact.NetUSD = &net
	}
	return fact, nil
}

func foreignField(name, label, kind, currency string) field {
	v := f(name, label, kind)
	v.VisibleWhen = &fieldVisibility{Field: currency, Values: []string{"USD"}, Not: true}
	return v
}

func manualReceiptFields() []field {
	fees := defaultField("fee_status", "手续费", "select", "pending")
	fees.Options = selectField("", "", "pending|稍后核实", "confirmed|金额已核实").Options
	fee := f("fee_usd", "已核实手续费 USD", "decimal")
	fee.VisibleWhen = &fieldVisibility{Field: "fee_status", Values: []string{"confirmed"}}
	settled := optional("settled_at", "通道结算到账时间（可选）", "datetime")
	settled.VisibleWhen = fee.VisibleWhen
	return []field{optional("order_id", "关联订单（可选）", "text"), f("provider_receipt_id", "外部唯一收款流水", "text"), defaultField("currency", "实际入账币种", "text", "USD"), f("gross_usd", "实际入账总额USD", "decimal"), foreignField("original_amount", "原币种实收金额", "decimal", "currency"), foreignField("valuation_evidence", "本次USD估值依据", "text", "currency"), f("recipient_reference", "实际收款账号证明", "text"), f("payer_reference", "实际付款人账号证明", "text"), f("paid_at", "真实付款时刻", "datetime"), f("verification_evidence", "真实银行核实凭证", "textarea"), fees, fee, settled, optional("network", "原网络（链上收款时填写）", "text")}
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
	return a.Commerce.AllocateReceipt(ctx, tx, commerce.AllocateReceiptInput{ActorStaffID: actor, OperatorID: operator, BusinessKey: businessKey, ReceiptID: receiptID, OrderID: orderID, AmountUSD: order.DueUSD.Sub(order.InternalUSD).Sub(order.CommissionUSD)})
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
	if _, err = a.Cooperation.FundPurchase(ctx, tx, a.Commerce, actor, orderID, receiptID, businessKey, receipt.GrossUSD); err != nil {
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
