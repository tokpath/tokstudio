package portal

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/cooperation"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"github.com/tokpath/tokstudio/backend/internal/payments"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"sort"
	"strconv"
	"strings"
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
		if op.Kind == organization.Platform {
			view = "supplier"
		}
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
		v["exit_policy_summary"] = cooperation.ProcurementExitTermsSummary(q.ExitTerms)
		a.procurementPartyNames(ctx, v, q.SupplierID, q.BuyerID)
		if q.ContractAttachmentID != "" {
			v["contract_url"] = "/api/v1/operators/" + op.ID + "/contracts/" + q.ContractAttachmentID + "/content"
		}
		status := "quote"
		actions := []string{}
		if view == "supplier" {
			if q.Enabled {
				actions = []string{"disable_quote"}
			} else {
				status = "disabled"
				actions = []string{"enable_quote"}
			}
		}
		if view == "buyer" && q.Enabled && (q.ValidUntil == nil || time.Now().Before(*q.ValidUntil)) {
			actions = []string{"purchase"}
		}
		row := entity(q.ID, status, 0, v)
		row["allowed_actions"] = actions
		items = append(items, row)
	}
	orders, e := a.Cooperation.ListPurchaseOrders(ctx, nil, actor, op.ID, view, 100)
	if e != nil {
		handleError(c, e)
		return
	}
	sort.SliceStable(orders, func(i, j int) bool {
		return purchaseOccupiesInventory(orders[i]) && !purchaseOccupiesInventory(orders[j])
	})
	unpaidHold, deliveryHold := ledger.Zero(), ledger.Zero()
	holdingOrders := 0
	for _, o := range orders {
		v := values(o)
		delete(v, "expires_at")
		v["record_kind"] = "order"
		v["exit_policy_summary"] = cooperation.ProcurementExitTermsSummary(o.ExitTerms)
		a.procurementPartyNames(ctx, v, o.SupplierID, o.BuyerID)
		if o.ContractAttachmentID != "" {
			v["contract_url"] = "/api/v1/operators/" + op.ID + "/contracts/" + o.ContractAttachmentID + "/content"
		}
		held := o.PaidATG.Add(o.GiftATG)
		v["price_usd"] = o.DueUSD
		v["reserved_atg"] = ledger.Zero()
		if purchaseOccupiesInventory(o) {
			v["reserved_atg"] = held
			holdingOrders++
			if o.Status == "paid" {
				deliveryHold = deliveryHold.Add(held)
			} else {
				unpaidHold = unpaidHold.Add(held)
			}
		}
		v["reserved_note"] = purchaseHoldNote(o)
		v["payment_hold"] = purchasePaymentHold(o)
		row := entity(o.ID, o.Status, 0, v)
		row["allowed_actions"] = []string{}
		if o.Status != "completed" && o.Status != "closed" {
			if view == "buyer" {
				row["allowed_actions"] = []string{"checkout", "query_payment", "wallet_receipt"}
			} else {
				row["allowed_actions"] = []string{"fund", "fulfill"}
			}
			if o.Status == "awaiting_payment" && o.AllocatedUSD.Sign() == 0 {
				row["allowed_actions"] = append(row["allowed_actions"].([]string), "cancel")
			}
		}
		items = append(items, row)
	}
	authorizations, attempts, e := a.AutoReplenishments.List(ctx, actor, op.ID)
	if e != nil {
		handleError(c, e)
		return
	}
	if view == "buyer" {
		for _, auth := range authorizations {
			v := values(auth)
			v["record_kind"] = "authorization"
			if q, err := a.Cooperation.GetPurchaseQuote(ctx, nil, actor, op.ID, auth.QuoteID); err == nil {
				v["name"] = q.Name
				v["exit_policy_summary"] = cooperation.ProcurementExitTermsSummary(q.ExitTerms)
			}
			a.procurementPartyNames(ctx, v, auth.SupplierID, auth.BuyerID)
			row := entity(auth.ID, "authorization", auth.Version, v)
			row["allowed_actions"] = []string{"authorization", "cancel_authorization"}
			items = append(items, row)
		}
		for _, attempt := range attempts {
			v := values(attempt)
			v["record_kind"] = "replenishment_attempt"
			row := entity(attempt.ID, attempt.State, 0, v)
			row["allowed_actions"] = []string{}
			items = append(items, row)
		}
	}
	p["title"] = "采购、供货与自动补货"
	p["description"] = "固定报价保留成交与退出条款。未付款采购订单不设付款期限，按购买加赠送数量持续占用供货库存，付清交付或取消后释放。下表列出每一笔占用。自动扣款须另有真实支付授权；线下模式只自动建待付订单。"
	p["summary"] = []gin.H{
		{"label": "未付款占用", "value": unpaidHold.String(), "unit": "ATG", "detail": "这些订单没有付款截止时间。付清并交付，或取消尚未分配收款的订单后释放。"},
		{"label": "待交付占用", "value": deliveryHold.String(), "unit": "ATG", "detail": "收款已经分配，库存仍占用到交付完成。"},
		{"label": "正在占用的订单", "value": strconv.Itoa(holdingOrders), "detail": "未付款或已付款未交付的采购订单数量。"},
	}
	p["items"] = items
	p["views"] = []gin.H{{"value": "buyer", "label": "我方采购"}, {"value": "supplier", "label": "我方供货"}}
	if op.Kind == organization.Platform {
		p["views"] = []gin.H{{"value": "supplier", "label": "我方供货"}}
	}
	if op.Kind == organization.Channel {
		p["views"] = []gin.H{{"value": "buyer", "label": "我方采购"}}
	}
	p["columns"] = []gin.H{col("record_kind", "记录类型", "text"), col("name", "报价或订单名称", "text"), col("buyer_name", "采购方", "text"), col("supplier_name", "供货方", "text"), col("reserved_atg", "占用供货库存", "atg"), col("reserved_note", "占用说明", "text"), col("payment_hold", "付款期限", "text"), col("price_usd", "成交USD", "usd"), col("allocated_usd", "已分配USD", "usd"), col("paid_atg", "购买库存", "atg"), col("gift_atg", "赠送库存", "atg"), col("exit_policy_summary", "成交时的退出规则", "text"), col("terms_summary", "合作约定", "text"), col("contract_url", "合作合同", "link"), col("state", "执行状态", "status")}
	p["operations"] = []operation{{Name: "quote", Label: "向直属采购方发布供货报价", Method: "POST", Path: base, Fields: []field{defaultField("action", "办理类型", "hidden", "quote"), f("buyer_id", "直属采购方", "text"), optional("name", "报价名称（留空自动命名）", "text"), f("price_usd", "收款USD", "decimal"), f("paid_atg", "购买库存ATG", "decimal"), defaultField("gift_atg", "赠送库存ATG", "decimal", "0"), optional("terms_summary", "补充合作约定（可选）", "textarea"), supplyExitPolicyField(), optional("contract_attachment_id", "已上传合作合同（可选）", "text"), optional("valid_until", "报价截止时间", "datetime"), defaultField("enabled", "允许按报价下单", "boolean", true)}}, {Name: "authorization", Label: "设定有限自动补货授权", Method: "POST", Path: base, Fields: []field{defaultField("action", "办理类型", "hidden", "authorization"), f("quote_id", "已确认的供货报价", "text"), optional("supplier_id", "报价供货方", "hidden"), f("trigger_available_atg", "采购可用库存低于ATG时触发", "decimal"), f("single_limit_usd", "每次最高USD", "decimal"), f("period_limit_usd", "周期最高USD", "decimal"), selectField("period", "UTC自然周期", "month|月", "quarter|季", "year|年"), selectField("mode", "执行方式", "offline|仅建立待付订单", "automatic|已获授权Stripe自动扣款"), optional("merchant_id", "真实供货收款商户配置编号", "text"), optional("payment.payment_method", "Stripe已授权pm_编号", "password"), optional("payment.customer_ref", "Stripe已授权cus_编号", "password"), f("consent_reference", "本次补货范围与付款授权说明", "textarea"), defaultField("enabled", "启用授权", "boolean", true), defaultField("expected_version", "当前授权版本", "hidden", 0)}, Confirmation: "确认固定报价、单次限额及UTC周期限额。未知付款仅查询原交易，不能自动再次扣款。"}}
	p["row_operations"] = []operation{{Name: "purchase", Label: "按原报价创建采购订单", Method: "POST", Path: base + "/{id}/purchase", Fields: []field{}, Confirmation: "确认原报价购买/赠送库存、付款额与退出条款。创建后按购买加赠送数量占用供货库存，直到付清交付或取消，不增加可用采购库存。"}, {Name: "disable_quote", Label: "停止按此报价下单", Method: "POST", Path: base + "/{id}/disable_quote", Fields: []field{}, Confirmation: "停止后，采购方不能再按这条报价创建新订单。已经创建的订单仍按原条款办理。"}, {Name: "enable_quote", Label: "恢复按此报价下单", Method: "POST", Path: base + "/{id}/enable_quote", Fields: []field{}, Confirmation: "恢复后，采购方可以继续按这条报价创建新订单。"}, {Name: "checkout", Label: "发起真实付款", Method: "POST", Path: base + "/{id}/checkout", Fields: []field{f("merchant_id", "付款方式", "text")}, Description: "本次付款尝试有时效，订单本身不因此关闭。未付清时库存继续占用，可再次发起付款或取消。核实付款并交付后库存才交付给采购方。"}, {Name: "query_payment", Label: "查询原付款与交付", Method: "POST", Path: base + "/{id}/query_payment", Fields: []field{}}, {Name: "wallet_receipt", Label: "提交链上付款查找线索", Method: "POST", Path: base + "/{id}/wallet_receipt", Fields: []field{f("transaction_hash", "实际交易Hash（仅查找线索）", "text"), f("log_index", "目标Transfer事件索引（十六进制）", "text")}, Description: "只适用于原EVM钱包checkout。服务端独立核验网络、合约、地址和确认数，提交线索不等于付款成功。"}, {Name: "fund", Label: "分配已核实收款", Permission: "payments.manage", Method: "POST", Path: base + "/{id}/fund", Fields: []field{f("receipt_id", "本方已核实收款编号", "text"), f("amount_usd", "本次分配USD", "decimal")}}, {Name: "fulfill", Label: "重试实际库存交付", Method: "POST", Path: base + "/{id}/fulfill", Fields: []field{}}, {Name: "cancel", Label: "取消未付款订单", Method: "POST", Path: base + "/{id}/cancel", Fields: []field{f("evidence", "取消原因", "textarea")}, Confirmation: "确认取消这笔尚未分配收款的订单？取消后立即释放它占用的供货库存。"}, {Name: "cancel_authorization", Label: "停止未来自动补货", Method: "POST", Path: base + "/{id}/cancel_authorization", IncludeVersion: true, Fields: []field{}}}
	ops := p["operations"].([]operation)
	for i := range ops[1].Fields {
		f := &ops[1].Fields[i]
		if f.Name == "merchant_id" || strings.HasPrefix(f.Name, "payment.") {
			f.VisibleWhen = &fieldVisibility{Field: "mode", Values: []string{"automatic"}}
		}
		if f.Name == "mode" {
			f.Default = "offline"
		}
	}
	p["operations"] = []operation{}
	if view == "supplier" && op.Kind != organization.Channel {
		p["operations"] = []operation{ops[0]}
	}
	if view == "buyer" && op.ParentOperatorID != nil {
		if len(authorizations) == 0 {
			p["operations"] = []operation{ops[1]}
		}
		revise := ops[1]
		revise.Label = "修订本方自动补货授权"
		revise.Fields = append([]field{f("id", "原补货授权编号", "hidden")}, revise.Fields...)
		for i := range revise.Fields {
			field := &revise.Fields[i]
			if field.Name == "action" {
				field.Kind = "hidden"
			}
			if field.Name == "consent_reference" {
				field.Name, field.Label = "renewed_consent_reference", "本次修订的独立授权凭证（请重新确认并填写）"
			}
			if field.Name == "merchant_id" || field.Name == "payment.payment_method" || field.Name == "payment.customer_ref" {
				field.VisibleWhen = &fieldVisibility{Field: "mode", Values: []string{"automatic"}}
			}
			if field.Name == "payment.payment_method" || field.Name == "payment.customer_ref" {
				field.Help = "修订同一收款配置的自动授权时，两项同时留空沿用原加密付款资料；更换收款配置或任一项时须重新填写完整资料。"
			}
		}
		revise.Description = "修订本经营方唯一的原自动补货授权，按当前版本保存，不取消重建。每次修订须重新确认并填写本次独立授权依据；敏感付款资料不回显。"
		p["row_operations"] = append(p["row_operations"].([]operation), revise)
	}
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
		if in.ExpectedVersion > 0 {
			var revision struct {
				Consent string `json:"renewed_consent_reference"`
			}
			if json.Unmarshal(raw, &revision) != nil {
				handleError(c, organization.ErrInvalidInput)
				return
			}
			in.ConsentReference = revision.Consent
		}
		result, err := a.command(c, "procurement-authorization:"+op.ID, raw, func(tx *gorm.DB) (any, error) {
			svc, coop := *a.AutoReplenishments, *a.Cooperation
			coop.DB, svc.Cooperation = tx, &coop
			quote, err := a.Cooperation.GetPurchaseQuote(ctx, tx, actor, op.ID, in.QuoteID)
			if err != nil {
				return nil, err
			}
			if quote.BuyerID != op.ID || (in.SupplierID != "" && in.SupplierID != quote.SupplierID) {
				return nil, organization.ErrForbidden
			}
			in.SupplierID = quote.SupplierID
			out, err := svc.SaveAuthorization(ctx, actor, in)
			if errors.Is(err, cooperation.ErrConflict) {
				return nil, organization.ErrConflict
			}
			if errors.Is(err, cooperation.ErrReplenishment) {
				return nil, organization.ErrInvalidInput
			}
			return gin.H{"id": out.ID, "status": "authorized", "version": out.Version, "authorization": out}, err
		})
		jsonResponse(c, result, err)
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
	if action == "disable_quote" || action == "enable_quote" {
		var in struct{}
		if !body(c, &in) {
			return
		}
		quote, e := a.Cooperation.GetPurchaseQuote(ctx, nil, actor, op.ID, id)
		if e != nil {
			handleError(c, e)
			return
		}
		if quote.SupplierID != op.ID {
			handleError(c, organization.ErrForbidden)
			return
		}
		enabled := action == "enable_quote"
		if e = a.Cooperation.SetQuoteEnabled(ctx, nil, actor, id, enabled); e != nil {
			handleError(c, e)
			return
		}
		status := "disabled"
		if enabled {
			status = "quote"
		}
		c.JSON(200, commandResult(id, status))
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
		Evidence  string         `json:"evidence"`
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
			var original cooperation.ReplenishmentAuthorization
			if err := tx.Where("id=? AND buyer_id=?", id, op.ID).Take(&original).Error; err != nil {
				return nil, err
			}
			svc := *a.AutoReplenishments
			copyCoop := *a.Cooperation
			copyCoop.DB = tx
			svc.Cooperation = &copyCoop
			e := svc.CancelAuthorization(ctx, actor, op.ID, in.Version)
			if errors.Is(e, cooperation.ErrConflict) {
				e = organization.ErrConflict
			}
			return commandResult(id, "cancelled"), e
		case "cancel":
			reason := strings.TrimSpace(in.Evidence)
			if reason == "" || len(reason) > 600 {
				return nil, organization.ErrInvalidInput
			}
			o, e := a.Cooperation.GetPurchase(ctx, tx, actor, op.ID, id)
			if e != nil {
				return nil, e
			}
			if o.BuyerID != op.ID && o.SupplierID != op.ID {
				return nil, organization.ErrForbidden
			}
			e = a.Cooperation.ClosePurchase(ctx, tx, actor, id, commerce.PaymentClosureProof{VerifiedUnpaid: true, EvidenceReference: reason})
			return commandResult(id, "closed"), e
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
		po := payments.Order{ID: id, OperatorID: o.SupplierID, MerchantID: in.MerchantID, Money: money, Valuation: val, AcceptedAt: o.CreatedAt, ExpiresAt: time.Now().UTC().Add(30 * time.Minute), Description: "TokenHub procurement " + id, NotifyURL: a.Config.PublicBaseURL + "/v1/payments/webhooks/" + in.MerchantID}
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
	if intent.Driver == "manual" {
		action["kind"], action["instructions"] = "offline", checkout.Instructions
	}
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

func (a *Application) procurementPartyNames(ctx context.Context, v gin.H, supplierID, buyerID string) {
	if supplier, err := a.Organization.GetOperator(ctx, nil, supplierID); err == nil {
		v["supplier_name"] = supplier.Name
	}
	if buyer, err := a.Organization.GetOperator(ctx, nil, buyerID); err == nil {
		v["buyer_name"] = buyer.Name
	}
}

func purchaseOccupiesInventory(o cooperation.PurchaseOrder) bool {
	return o.Status == "awaiting_payment" || o.Status == "paid"
}

func purchaseHoldNote(o cooperation.PurchaseOrder) string {
	switch {
	case o.Status == "awaiting_payment" && o.AllocatedUSD.Sign() > 0:
		return "已有收款分配，库存仍占用；付清前不能取消"
	case o.Status == "awaiting_payment":
		return "未付款，持续占用直到付清或取消"
	case o.Status == "paid":
		return "已付款，持续占用直到交付完成"
	default:
		return "已释放，不再占用供货库存"
	}
}

func purchasePaymentHold(o cooperation.PurchaseOrder) string {
	switch o.Status {
	case "awaiting_payment":
		return "不限时，直到付清或取消"
	case "paid":
		return "已付清，占用到交付完成"
	default:
		return "订单已结束，不再等待付款"
	}
}
