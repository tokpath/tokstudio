package portal

import (
	"context"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/cooperation"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"github.com/tokpath/tokstudio/backend/internal/payments"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
)

type procurementCheckout struct {
	OrderID, BuyerID, ActorID, MerchantID string
	PaymentOrder                          json.RawMessage `gorm:"type:jsonb"`
	CreatedAt                             time.Time
}

func (procurementCheckout) TableName() string { return "portal_procurement_checkouts" }
func (a *Application) procurementResource(c *gin.Context, op organization.Operator, actor string, p gin.H, base string) {
	ctx := c.Request.Context()
	items := []any{}
	view := c.Query("view")
	if view == "" {
		view = "buyer"
	}
	if view != "buyer" && view != "supplier" {
		handleError(c, cooperation.ErrConflict)
		return
	}
	var quotes []cooperation.PurchaseQuote
	var e error
	if view == "buyer" {
		quotes, e = a.Cooperation.ListBuyerQuotes(ctx, nil, actor, op.ID)
	} else {
		quotes, e = a.Cooperation.ListSupplierQuotes(ctx, nil, actor, op.ID)
	}
	if e != nil {
		handleError(c, e)
		return
	}
	for _, q := range quotes {
		v := values(q)
		v["record_kind"] = "quote"
		row := entity(q.ID, "quote", 0, v)
		row["allowed_actions"] = []string{}
		if view == "buyer" && q.Enabled {
			row["allowed_actions"] = []string{"purchase"}
		}
		items = append(items, row)
	}
	orders, e := a.Cooperation.ListPurchaseOrders(ctx, nil, actor, op.ID, view, 100)
	if e != nil {
		handleError(c, e)
		return
	}
	for _, o := range orders {
		v := values(o)
		v["record_kind"] = "order"
		v["price_usd"] = o.DueUSD
		row := entity(o.ID, o.Status, 0, v)
		row["allowed_actions"] = []string{}
		if o.Status != "completed" && o.Status != "closed" {
			if view == "buyer" {
				row["allowed_actions"] = []string{"checkout", "query_payment", "wallet_receipt"}
			} else {
				row["allowed_actions"] = []string{"fund", "fulfill"}
			}
		}
		items = append(items, row)
	}
	authorizations, attempts, e := a.AutoReplenishments.List(ctx, actor, op.ID)
	if e != nil {
		handleError(c, e)
		return
	}
	for _, auth := range authorizations {
		v := values(auth)
		v["record_kind"] = "authorization"
		row := entity(auth.ID, "authorization", auth.Version, v)
		row["allowed_actions"] = []string{"cancel_authorization"}
		items = append(items, row)
	}
	for _, attempt := range attempts {
		v := values(attempt)
		v["record_kind"] = "replenishment_attempt"
		row := entity(attempt.ID, attempt.State, 0, v)
		row["allowed_actions"] = []string{}
		items = append(items, row)
	}
	p["title"] = "采购、供货与自动补货"
	p["description"] = "固定报价保留成交与退出条款。自动扣款须另有真实支付授权；线下模式只自动建待付订单。切换采购/供货视图可查看双方各自责任。"
	p["items"] = items
	p["views"] = []gin.H{{"value": "buyer", "label": "我方采购"}, {"value": "supplier", "label": "我方供货"}}
	p["columns"] = []gin.H{col("record_kind", "记录类型", "text"), col("buyer_id", "采购方", "text"), col("supplier_id", "供货方", "text"), col("price_usd", "成交USD", "usd"), col("allocated_usd", "已分配USD", "usd"), col("paid_atg", "购买库存", "atg"), col("gift_atg", "赠送库存", "atg"), col("exit_terms", "原退出条款", "text"), col("state", "执行状态", "status")}
	p["operations"] = []operation{{Name: "quote", Label: "向直属采购方发布供货报价", Method: "POST", Path: base, Fields: []field{defaultField("action", "办理类型", "text", "quote"), f("buyer_id", "直属采购方编号", "text"), f("price_usd", "收款USD", "decimal"), f("paid_atg", "购买库存ATG", "decimal"), f("gift_atg", "赠送库存ATG", "decimal"), f("terms_reference", "成交条款编号", "text"), defaultField("exit_terms.version", "退出条款版本", "integer", 1), f("exit_terms.voluntary_refund", "允许自愿退出退款", "boolean"), selectField("exit_terms.method", "合同退款算法", "remaining_paid_proportional|剩余购买库存原价比例", "non_refundable|合同不退款"), f("exit_terms.terms_reference", "退出条款编号", "text"), optional("valid_until", "报价截止时间", "datetime"), defaultField("enabled", "允许按报价下单", "boolean", true)}}, {Name: "authorization", Label: "设定有限自动补货授权", Method: "POST", Path: base, Fields: []field{defaultField("action", "办理类型", "text", "authorization"), f("quote_id", "固定成交报价编号", "text"), f("supplier_id", "报价供货方编号", "text"), f("trigger_available_atg", "采购可用库存低于ATG时触发", "decimal"), f("single_limit_usd", "每次最高USD", "decimal"), f("period_limit_usd", "周期最高USD", "decimal"), selectField("period", "UTC自然周期", "month|月", "quarter|季", "year|年"), selectField("mode", "执行方式", "offline|仅建立待付订单", "automatic|已获授权Stripe自动扣款"), optional("merchant_id", "真实供货收款商户配置编号", "text"), optional("payment.payment_method", "Stripe已授权pm_编号", "password"), optional("payment.customer_ref", "Stripe已授权cus_编号", "password"), f("consent_reference", "独立自动付款授权凭证", "text"), defaultField("enabled", "启用授权", "boolean", true), defaultField("expected_version", "当前授权版本（首次0）", "integer", 0)}, Confirmation: "确认固定报价、单次限额及UTC周期限额。未知付款仅查询原交易，不能自动再次扣款。"}}
	p["row_operations"] = []operation{{Name: "purchase", Label: "按原报价创建采购订单", Method: "POST", Path: base + "/{id}/purchase", Fields: []field{}, Confirmation: "确认原报价购买/赠送库存、付款额与退出条款。创建订单不增加可用采购库存。"}, {Name: "checkout", Label: "发起真实付款", Method: "POST", Path: base + "/{id}/checkout", Fields: []field{f("merchant_id", "供货方收款配置编号", "text")}, Description: "付款操作返回实际支付步骤，核实付款并交付后才可用。"}, {Name: "query_payment", Label: "查询原付款与交付", Method: "POST", Path: base + "/{id}/query_payment", Fields: []field{}}, {Name: "wallet_receipt", Label: "提交链上付款查找线索", Method: "POST", Path: base + "/{id}/wallet_receipt", Fields: []field{f("transaction_hash", "实际交易Hash（仅查找线索）", "text"), f("log_index", "目标Transfer事件索引（十六进制）", "text")}, Description: "只适用于原EVM钱包checkout。服务端独立核验网络、合约、地址和确认数，提交线索不等于付款成功。"}, {Name: "fund", Label: "分配已核实收款", Method: "POST", Path: base + "/{id}/fund", Fields: []field{f("receipt_id", "本方已核实收款编号", "text"), f("amount_usd", "本次分配USD", "decimal")}}, {Name: "fulfill", Label: "重试实际库存交付", Method: "POST", Path: base + "/{id}/fulfill", Fields: []field{}}, {Name: "cancel_authorization", Label: "停止未来自动补货", Method: "POST", Path: base + "/{id}/cancel_authorization", IncludeVersion: true, Fields: []field{}}}
	a.resourceResponse(c, p)
}
func (a *Application) procurementCommand(c *gin.Context, op organization.Operator, actor string) {
	raw, ok := decodeConfiguration(c)
	if !ok {
		return
	}
	var kind struct {
		Action string `json:"action"`
	}
	if json.Unmarshal(raw, &kind) != nil {
		handleError(c, cooperation.ErrConflict)
		return
	}
	ctx := c.Request.Context()
	if kind.Action == "authorization" {
		var in cooperation.ReplenishmentAuthorizationInput
		if json.Unmarshal(raw, &in) != nil {
			handleError(c, cooperation.ErrConflict)
			return
		}
		in.BuyerID = op.ID
		out, e := a.AutoReplenishments.SaveAuthorization(ctx, actor, in)
		if e != nil {
			handleError(c, e)
			return
		}
		c.JSON(200, gin.H{"id": out.ID, "status": "authorized", "version": out.Version})
		return
	}
	result, e := a.command(c, "procurement:"+op.ID, raw, func(tx *gorm.DB) (any, error) {
		if kind.Action != "quote" {
			return nil, cooperation.ErrConflict
		}
		var in cooperation.PurchaseQuote
		if json.Unmarshal(raw, &in) != nil {
			return nil, cooperation.ErrConflict
		}
		in.SupplierID = op.ID
		out, e := a.Cooperation.PublishPurchaseQuote(ctx, tx, actor, in)
		return commandResult(out.ID, "published"), e
	})
	jsonResponse(c, result, e)
}
func (a *Application) procurementAction(c *gin.Context, op organization.Operator, actor, action string) {
	if action == "wallet_receipt" {
		a.procurementWalletReceipt(c, op.ID, actor)
		return
	}
	ctx := c.Request.Context()
	id := c.Param("id")
	businessKey, _ := key(c)
	if action == "checkout" {
		a.procurementPayment(c, op, actor)
		return
	}
	if action == "query_payment" {
		var in struct{}
		if !body(c, &in) {
			return
		}
		var cr procurementCheckout
		if e := a.DB.WithContext(ctx).First(&cr, "order_id=? AND buyer_id=?", id, op.ID).Error; e != nil {
			handleError(c, e)
			return
		}
		_, e := a.Payments.Reconcile(ctx, func() string { var po payments.Order; _ = json.Unmarshal(cr.PaymentOrder, &po); return po.OperatorID }(), cr.MerchantID, id)
		if e != nil {
			handleError(c, e)
			return
		}
		c.JSON(200, commandResult(id, "verified_pending_delivery"))
		return
	}
	var in struct {
		ReceiptID string         `json:"receipt_id"`
		Amount    ledger.Decimal `json:"amount_usd"`
		Version   int64          `json:"version"`
	}
	if !body(c, &in) {
		return
	}
	raw, e := a.command(c, "procurement-action:"+op.ID+":"+id+":"+action, in, func(tx *gorm.DB) (any, error) {
		switch action {
		case "purchase":
			q, e := a.Cooperation.GetPurchaseQuote(ctx, tx, actor, op.ID, id)
			if e != nil {
				return nil, e
			}
			if q.BuyerID != op.ID {
				return nil, organization.ErrForbidden
			}
			out, e := a.Cooperation.CreatePurchaseOrder(ctx, tx, actor, op.ID, id, businessKey)
			return commandResult(out.ID, out.Status), e
		case "cancel_authorization":
			svc := *a.AutoReplenishments
			copyCoop := *a.Cooperation
			copyCoop.DB = tx
			svc.Cooperation = &copyCoop
			e := svc.CancelAuthorization(ctx, actor, op.ID, in.Version)
			return commandResult(id, "cancelled"), e
		case "fund", "fulfill":
			o, e := a.Cooperation.GetPurchase(ctx, tx, actor, op.ID, id)
			if e != nil {
				return nil, e
			}
			if o.SupplierID != op.ID {
				return nil, organization.ErrForbidden
			}
			if action == "fund" {
				out, e := a.Cooperation.FundPurchase(ctx, tx, a.Commerce, actor, id, in.ReceiptID, businessKey, in.Amount)
				return commandResult(out.ID, out.Status), e
			}
			_, e = a.Cooperation.FulfillPurchase(ctx, tx, id, actor)
			return commandResult(id, "completed"), e
		}
		return nil, cooperation.ErrConflict
	})
	jsonResponse(c, raw, e)
}
func (a *Application) procurementPayment(c *gin.Context, op organization.Operator, actor string) {
	var in struct {
		MerchantID string `json:"merchant_id"`
	}
	if !body(c, &in) {
		return
	}
	ctx := c.Request.Context()
	id := c.Param("id")
	o, e := a.Cooperation.GetPurchase(ctx, nil, actor, op.ID, id)
	if e != nil {
		handleError(c, e)
		return
	}
	if o.BuyerID != op.ID {
		handleError(c, organization.ErrForbidden)
		return
	}
	var saved procurementCheckout
	e = a.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", "procurement-checkout:"+id).Error; e != nil {
			return e
		}
		q := tx.Where("order_id=?", id).Limit(1).Find(&saved)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected > 0 {
			if saved.MerchantID != in.MerchantID {
				return payments.ErrConflict
			}
			return nil
		}
		methods, e := a.Payments.PublicMethods(ctx, o.SupplierID)
		if e != nil {
			return e
		}
		currency := ""
		for _, method := range methods {
			if method.ID == in.MerchantID {
				currency = method.Currency
			}
		}
		if currency == "" {
			return payments.ErrNotConfigured
		}
		money, val, e := a.paymentValue(ctx, o.SupplierID, currency, o.DueUSD, o.CreatedAt)
		if e != nil {
			return e
		}
		po := payments.Order{ID: id, OperatorID: o.SupplierID, MerchantID: in.MerchantID, Money: money, Valuation: val, AcceptedAt: o.CreatedAt, ExpiresAt: o.ExpiresAt, Description: "TokenHub procurement " + id, NotifyURL: a.Config.PublicBaseURL + "/v1/payments/webhooks/" + in.MerchantID}
		encoded, _ := json.Marshal(po)
		saved = procurementCheckout{OrderID: id, BuyerID: op.ID, ActorID: actor, MerchantID: in.MerchantID, PaymentOrder: encoded, CreatedAt: time.Now().UTC()}
		return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&saved).Error
	})
	if e != nil {
		handleError(c, e)
		return
	}
	var po payments.Order
	_ = json.Unmarshal(saved.PaymentOrder, &po)
	checkout, e := a.Payments.Create(ctx, po.OperatorID, po.MerchantID, po)
	if e != nil {
		handleError(c, e)
		return
	}
	action := procurementPaymentAction(checkout)
	intent, e := a.Payments.PaymentIntent(ctx, po.OperatorID, saved.MerchantID, id)
	if e != nil {
		handleError(c, e)
		return
	}
	action["network"], action["token_contract"] = intent.Network, intent.TokenContract
	if intent.Driver == "evm_wallet" {
		action["wallet_receipt_path"] = "/api/v1/operators/" + op.ID + "/procurement/" + id + "/wallet_receipt"
	}
	c.JSON(200, gin.H{"id": id, "status": checkout.State, "message": "Pay the original checkout, then verify actual receipt and delivery.", "order": gin.H{"id": id, "status": checkout.State, "kind": "procurement", "operator_name": po.OperatorID, "amount_usd": o.DueUSD, "paid_usd": o.AllocatedUSD, "created_at": o.CreatedAt, "payment_action": action}})
}
func (a *Application) deliverProcurementReceipt(ctx context.Context, cr procurementCheckout, receiptID string) error {
	return a.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		o, e := a.Cooperation.GetPurchase(ctx, tx, cr.ActorID, cr.BuyerID, cr.OrderID)
		if e != nil {
			return e
		}
		r, e := a.Commerce.GetReceipt(ctx, tx, o.SupplierID, receiptID)
		if e != nil {
			return e
		}
		amount := r.GrossUSD.Sub(r.AllocatedUSD)
		due := o.DueUSD.Sub(o.AllocatedUSD)
		if amount.Cmp(due) > 0 {
			amount = due
		}
		if amount.Sign() > 0 {
			if _, e = a.Cooperation.ApplyVerifiedPurchaseReceipt(ctx, tx, a.Commerce, o.ID, receiptID, "verified-procurement:"+receiptID, amount); e != nil {
				return e
			}
		}
		_, e = a.Cooperation.FulfillPurchase(ctx, tx, o.ID, "verified-procurement-worker")
		return e
	})
}

var _ commerce.Order

func procurementPaymentAction(ch payments.Checkout) gin.H {
	p := gin.H{}
	switch {
	case ch.ClientSecret != "":
		p = gin.H{"kind": "stripe", "client_secret": ch.ClientSecret, "publishable_key": ch.PublishableKey}
	case ch.URL != "":
		p = gin.H{"kind": "redirect", "url": ch.URL}
	case ch.PaymentAddress != "":
		p = gin.H{"kind": "stablecoin", "address": ch.PaymentAddress}
	default:
		p = gin.H{"kind": "offline"}
	}
	if ch.PayMoney != nil {
		p["amount"] = ch.PayMoney.Amount.String()
		p["currency"] = ch.PayMoney.Currency
	}
	return p
}
