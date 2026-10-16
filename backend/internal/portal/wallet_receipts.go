package portal

import (
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"github.com/tokpath/tokstudio/backend/internal/payments"
)

type walletReceiptHint struct {
	TransactionHash string `json:"transaction_hash"`
	LogIndex        string `json:"log_index"`
}

func (a *Application) customerWalletReceipt(c *gin.Context) {
	var in walletReceiptHint
	if !body(c, &in) {
		return
	}
	ctx := c.Request.Context()
	id := subject(c)
	order, e := a.Commerce.GetOrder(ctx, nil, id.Customer.Customer.ID, c.Param("id"))
	if e != nil {
		handleError(c, e)
		return
	}
	var checkout checkoutRow
	if e = a.DB.WithContext(ctx).First(&checkout, "order_id=? AND customer_id=?", order.ID, id.Customer.Customer.ID).Error; e != nil || checkout.MerchantID == nil {
		if e == nil {
			e = payments.ErrNotConfigured
		}
		handleError(c, e)
		return
	}
	_, e = a.Payments.VerifyWalletReceipt(ctx, order.OperatorID, *checkout.MerchantID, order.ID, in.TransactionHash, in.LogIndex)
	if e != nil {
		handleError(c, e)
		return
	}
	c.JSON(200, commandResult(order.ID, "verified_pending_delivery"))
}

func (a *Application) procurementWalletReceipt(c *gin.Context, operator, actor string) {
	var in walletReceiptHint
	if !body(c, &in) {
		return
	}
	ctx := c.Request.Context()
	order, e := a.Cooperation.GetPurchase(ctx, nil, actor, operator, c.Param("id"))
	if e != nil {
		handleError(c, e)
		return
	}
	if order.BuyerID != operator {
		handleError(c, organization.ErrForbidden)
		return
	}
	var checkout procurementCheckout
	if e = a.DB.WithContext(ctx).First(&checkout, "order_id=? AND buyer_id=?", order.ID, operator).Error; e != nil {
		handleError(c, e)
		return
	}
	var paymentOrder payments.Order
	if json.Unmarshal(checkout.PaymentOrder, &paymentOrder) != nil || paymentOrder.ID != order.ID || paymentOrder.OperatorID != order.SupplierID {
		handleError(c, payments.ErrConflict)
		return
	}
	_, e = a.Payments.VerifyWalletReceipt(ctx, paymentOrder.OperatorID, checkout.MerchantID, paymentOrder.ID, in.TransactionHash, in.LogIndex)
	if e != nil {
		handleError(c, e)
		return
	}
	c.JSON(200, commandResult(order.ID, "verified_pending_delivery"))
}
