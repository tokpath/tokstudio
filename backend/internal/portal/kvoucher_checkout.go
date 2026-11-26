package portal

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"github.com/tokpath/tokstudio/backend/internal/payments"
	"gorm.io/gorm"
)

func (a *Application) openCustomerKVoucherPayment(c *gin.Context) {
	customerID := subject(c).Customer.Customer.ID
	a.openKVoucherPayment(c, func(ctx context.Context, tx *gorm.DB, orderID string) (string, string, error) {
		order, err := a.Commerce.GetOrder(ctx, tx, customerID, orderID)
		if err != nil {
			return "", "", err
		}
		if err := requireOpenCustomerOrder(ctx, tx, order); err != nil {
			return "", "", err
		}
		var checkout checkoutRow
		if err := tx.First(&checkout, "order_id=? AND customer_id=?", order.ID, customerID).Error; err != nil {
			return "", "", err
		}
		if checkout.MerchantID == nil {
			return "", "", commerce.ErrNotReady
		}
		return order.OperatorID, *checkout.MerchantID, nil
	})
}

func (a *Application) openProcurementKVoucherPayment(c *gin.Context, op organization.Operator, actor string) {
	a.openKVoucherPayment(c, func(ctx context.Context, tx *gorm.DB, orderID string) (string, string, error) {
		order, err := a.Cooperation.GetPurchase(ctx, tx, actor, op.ID, orderID)
		if err != nil {
			return "", "", err
		}
		if order.BuyerID != op.ID {
			return "", "", organization.ErrForbidden
		}
		if order.Status != "awaiting_payment" || !order.AllocatedMoney.IsZero() {
			return "", "", commerce.ErrOrderState
		}
		automatic, err := a.AutoReplenishments.UsesAutomaticPayment(ctx, tx, order.ID)
		if err != nil {
			return "", "", err
		}
		if automatic {
			return "", "", commerce.ErrOrderState
		}
		var checkout procurementCheckout
		if err := tx.First(&checkout, "order_id=? AND buyer_id=?", order.ID, op.ID).Error; err != nil {
			return "", "", err
		}
		return order.SupplierID, checkout.MerchantID, nil
	})
}

func (a *Application) openKVoucherPayment(c *gin.Context, checkoutScope func(context.Context, *gorm.DB, string) (string, string, error)) {
	c.Header("Cache-Control", "no-store")
	var in struct {
		ExpectedGeneration *int64 `json:"expected_generation"`
	}
	if !body(c, &in) {
		return
	}
	if in.ExpectedGeneration == nil || *in.ExpectedGeneration < 0 {
		handleError(c, payments.ErrInvalid)
		return
	}
	actionKey, ok := key(c)
	if !ok {
		return
	}
	orderUUID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		handleError(c, commerce.ErrInvalidInput)
		return
	}
	ctx, orderID := c.Request.Context(), orderUUID.String()
	checkout, generation, err := a.Payments.OpenKVoucherCheckout(ctx, orderID, *in.ExpectedGeneration, actionKey, checkoutScope)
	if errors.Is(err, payments.ErrCheckoutUnavailable) {
		c.JSON(http.StatusOK, gin.H{"id": orderID, "status": "payment_already_opened", "gkash_generation": generation})
		return
	}
	if errors.Is(err, payments.ErrConflict) {
		err = commerce.ErrPaymentUnknown
	}
	if err != nil {
		handleError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"id": orderID, "status": "payment_ready", "gkash_generation": generation,
		"payment_action": gin.H{"kind": "form", "url": checkout.URL, "fields": checkout.FormFields},
	})
}

// Opening an available form while its payment is being checked invalidates
// the earlier check for a concurrent payment method change.
func sameCheckedPayment(current, checked payments.IntentView) bool {
	return current.MerchantID == checked.MerchantID && current.ProviderID == checked.ProviderID &&
		current.KVoucherFormState == checked.KVoucherFormState && current.KVoucherFormGeneration == checked.KVoucherFormGeneration
}

func checkoutPaymentAction(intent payments.IntentView, checkout payments.Checkout) gin.H {
	var action gin.H
	switch {
	case intent.Driver == "kvoucher":
		action = gin.H{"kind": "form", "gkash_opened": intent.KVoucherFormState != payments.KVoucherFormReady, "gkash_generation": intent.KVoucherFormGeneration}
	case intent.Driver == "manual":
		action = gin.H{"kind": "offline", "address": checkout.PaymentAddress, "instructions": checkout.Instructions}
	case checkout.ClientSecret != "":
		action = gin.H{"kind": "stripe", "client_secret": checkout.ClientSecret, "publishable_key": checkout.PublishableKey}
	case checkout.URL != "" && len(checkout.FormFields) > 0:
		action = gin.H{"kind": "form", "url": checkout.URL, "fields": checkout.FormFields}
	case checkout.URL != "":
		action = gin.H{"kind": "redirect", "url": checkout.URL}
	case checkout.PaymentAddress != "":
		action = gin.H{"kind": "offline", "address": checkout.PaymentAddress}
	default:
		action = gin.H{"kind": "offline"}
	}
	action["settlement_money"] = checkout.SettlementMoney
	action["fee_quote"] = checkout.FeeQuote
	action["payment_quote"] = checkout.PaymentQuote
	if checkout.Instructions != "" {
		action["instructions"] = checkout.Instructions
	}
	if checkout.PayMoney != nil {
		action["amount"] = checkout.PayMoney.Amount.String()
		action["currency"] = checkout.PayMoney.Currency
	}
	return action
}
