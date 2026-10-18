package portal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/incentives"
	"github.com/tokpath/tokstudio/backend/internal/jobs"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/payments"
	"gorm.io/gorm"
)

type quoteRow struct {
	ID, CustomerID, OperatorID, PackageID, QuoteID, TermsVersion string
	Selection, Preview                                           json.RawMessage
	ExpiresAt, CreatedAt                                         time.Time
}

func (quoteRow) TableName() string { return "portal_order_quotes" }

type checkoutRow struct {
	OrderID, CustomerID        string
	MerchantID                 *string
	AutoRenew, AllowCommission bool
	Action                     json.RawMessage
	PaymentOrder               json.RawMessage `gorm:"type:jsonb"`
	CreatedAt                  time.Time
}

func (checkoutRow) TableName() string { return "portal_order_checkout" }

type valuationRow struct {
	ID, OperatorID, Currency, Source, ActorID string
	USDPerUnit                                ledger.Decimal
	EffectiveAt, ExpiresAt, CreatedAt         time.Time
}

func (valuationRow) TableName() string { return "portal_valuations" }
func (a *Application) offers(c *gin.Context) {
	id := subject(c)
	rows, e := a.Incentives.EligibleActivities(c.Request.Context(), nil, id.Customer.Ownership.OperatorID, id.Customer.Customer.ID, c.Param("id"))
	if e != nil {
		handleError(c, e)
		return
	}
	items := []any{}
	for _, o := range rows {
		if !o.Eligible {
			continue
		}
		kind := "operator"
		if o.Kind == "kol" {
			kind = "kol"
		}
		items = append(items, gin.H{"id": o.ID, "name": fmt.Sprintf("%s 活动 %s", kind, o.ID[:8]), "kind": kind, "description": "赠送 " + o.GiftATG.String() + " ATG；组合资格以确认报价为准", "allow_combination": o.AllowCombination, "compatible_campaign_ids": o.CompatibleCampaignIDs})
	}
	c.JSON(200, gin.H{"items": items})
}
func (a *Application) quote(c *gin.Context) {
	var in struct {
		PlanID             string `json:"plan_id"`
		OperatorActivityID string `json:"operator_activity_id"`
		KOLActivityID      string `json:"kol_activity_id"`
	}
	if !body(c, &in) {
		return
	}
	id := subject(c)
	ctx := c.Request.Context()
	raw, e := a.command(c, "quote:"+id.Customer.Customer.ID, in, func(tx *gorm.DB) (any, error) {
		p, e := a.Commerce.GetPackage(ctx, tx, id.Customer.Ownership.OperatorID, in.PlanID)
		if e != nil {
			return nil, e
		}
		if !p.NewPurchaseEnabled || p.CurrentQuoteID == nil {
			return nil, commerce.ErrOrderState
		}
		q, e := a.Commerce.GetQuote(ctx, tx, *p.CurrentQuoteID)
		if e != nil {
			return nil, e
		}
		selection, _ := json.Marshal(incentives.ActivitySelection{MerchantCampaignID: in.OperatorActivityID, KOLCampaignID: in.KOLActivityID})
		m, e := a.Incentives.PreviewQuote(ctx, tx, id.Customer.Customer.ID, id.Customer.Ownership.OperatorID, q.ID, selection)
		if e != nil {
			return nil, e
		}
		op, e := a.Organization.GetOperator(ctx, tx, q.OperatorID)
		if e != nil {
			return nil, e
		}
		now := time.Now().UTC()
		terms := fmt.Sprintf("%s:%d", q.ID, q.Version)
		qid := uuid.NewString()
		dto := gin.H{"id": qid, "plan_id": p.ID, "plan_name": p.Name, "operator_name": op.Name, "amount_usd": m.FinalUSD, "purchase_atg": q.PaidATG, "plan_gift_atg": q.GiftATG, "operator_gift_atg": m.MerchantGiftATG, "kol_gift_atg": m.KOLGiftATG, "total_atg": q.PaidATG.Add(q.GiftATG).Add(m.MerchantGiftATG).Add(m.KOLGiftATG), "validity": validity(q.Period), "terms_version": terms, "expires_at": now.Add(15 * time.Minute), "warnings": []string{"报价预览不会预留活动名额；提交时将重新验证资格及覆盖"}}
		preview, _ := json.Marshal(dto)
		e = tx.Create(&quoteRow{ID: qid, CustomerID: id.Customer.Customer.ID, OperatorID: q.OperatorID, PackageID: p.ID, QuoteID: q.ID, TermsVersion: terms, Selection: selection, Preview: preview, ExpiresAt: now.Add(15 * time.Minute), CreatedAt: now}).Error
		return dto, e
	})
	jsonResponse(c, raw, e)
}
func (a *Application) createOrder(c *gin.Context) {
	var in struct {
		QuoteID      string `json:"quote_id"`
		TermsVersion string `json:"terms_version"`
		Funding      struct {
			USDBalance       ledger.Decimal `json:"usd_balance"`
			CommissionUSD    ledger.Decimal `json:"commission_usd"`
			ExternalMethodID string         `json:"external_method_id"`
		} `json:"funding"`
		AutoRenew       bool `json:"auto_renew"`
		AllowCommission bool `json:"allow_commission"`
	}
	if !body(c, &in) {
		return
	}
	id := subject(c)
	ctx := c.Request.Context()
	businessKey, _ := key(c)
	raw, e := a.command(c, "order:"+id.Customer.Customer.ID, in, func(tx *gorm.DB) (any, error) {
		var qr quoteRow
		if e := tx.First(&qr, "id=? AND customer_id=? AND operator_id=? AND expires_at>?", in.QuoteID, id.Customer.Customer.ID, id.Customer.Ownership.OperatorID, time.Now()).Error; e != nil {
			return nil, e
		}
		if qr.TermsVersion != in.TermsVersion {
			return nil, commerce.ErrConflict
		}
		p, e := a.Commerce.GetPackage(ctx, tx, qr.OperatorID, qr.PackageID)
		if e != nil {
			return nil, e
		}
		if p.CurrentQuoteID == nil || *p.CurrentQuoteID != qr.QuoteID {
			return nil, commerce.ErrConflict
		}
		var expected struct {
			Amount       ledger.Decimal `json:"amount_usd"`
			MerchantGift ledger.Decimal `json:"operator_gift_atg"`
			KOLGift      ledger.Decimal `json:"kol_gift_atg"`
		}
		if e = json.Unmarshal(qr.Preview, &expected); e != nil {
			return nil, e
		}
		order, e := a.Commerce.CreateOrder(ctx, tx, commerce.CreateOrderInput{CustomerID: id.Customer.Customer.ID, OperatorID: qr.OperatorID, QuoteID: qr.QuoteID, BusinessKey: businessKey, ExpectedOwnershipVersion: id.Customer.Ownership.Version, InternalUSD: in.Funding.USDBalance, CommissionUSD: in.Funding.CommissionUSD, ActivitySelection: qr.Selection, Validity: 30 * time.Minute})
		if e != nil {
			return nil, e
		}
		if order.DueUSD.Cmp(expected.Amount) != 0 || order.MerchantGiftATG.Cmp(expected.MerchantGift) != 0 || order.KOLGiftATG.Cmp(expected.KOLGift) != 0 {
			return nil, commerce.ErrConflict
		}
		ext := order.DueUSD.Sub(order.InternalUSD).Sub(order.CommissionUSD)
		var merchant *string
		if ext.Sign() > 0 {
			if in.Funding.ExternalMethodID == "" {
				return nil, commerce.ErrNotPaid
			}
			m, e := a.Payments.Merchant(ctx, order.OperatorID, in.Funding.ExternalMethodID)
			if e != nil {
				return nil, e
			}
			if !m.Enabled {
				return nil, payments.ErrNotConfigured
			}
			merchant = &m.ID
		}
		cr := checkoutRow{OrderID: order.ID, CustomerID: order.CustomerID, MerchantID: merchant, AutoRenew: in.AutoRenew, AllowCommission: in.AllowCommission, CreatedAt: time.Now()}
		if e = tx.Create(&cr).Error; e != nil {
			return nil, e
		}
		if ext.Sign() == 0 {
			if _, e = a.Commerce.Fulfill(ctx, tx, order.ID, "customer:"+order.CustomerID); e != nil {
				return nil, e
			}
			if e = a.authorizeRenewal(ctx, tx, order, cr); e != nil {
				return nil, e
			}
		}
		return gin.H{"id": order.ID}, nil
	})
	if e != nil {
		handleError(c, e)
		return
	}
	var out struct{ ID string }
	_ = json.Unmarshal(raw, &out)
	order, e := a.Commerce.GetOrder(ctx, nil, id.Customer.Customer.ID, out.ID)
	if e != nil {
		handleError(c, e)
		return
	}
	if e = a.ensureCheckout(ctx, order); e != nil {
		handleError(c, e)
		return
	}
	dto, e := a.orderDTO(ctx, order)
	if e != nil {
		handleError(c, e)
		return
	}
	c.JSON(200, dto)
}
func (a *Application) authorizeRenewal(ctx context.Context, tx *gorm.DB, o commerce.Order, cr checkoutRow) error {
	if !cr.AutoRenew || o.Kind == "topup" || o.Period == ledger.Lifetime {
		return nil
	}
	_, e := a.Commerce.EnableAutoRenewal(ctx, tx, o.CustomerID, o.ID, "customer explicit order consent:"+o.BusinessKey, cr.AllowCommission)
	return e
}
func (a *Application) paymentValue(ctx context.Context, operator, currency string, due ledger.Decimal, at time.Time) (payments.Money, payments.Valuation, error) {
	if currency == "USD" {
		return payments.Money{Currency: "USD", Amount: due}, payments.Valuation{ID: "usd-native", Currency: "USD", USDPerUnit: ledger.MustDecimal("1"), Source: "USD-denominated obligation", EffectiveAt: at.Add(-time.Second), ExpiresAt: at.Add(24 * time.Hour)}, nil
	}
	var row valuationRow
	if e := a.DB.WithContext(ctx).Where("operator_id=? AND currency=? AND effective_at<=? AND expires_at>?", operator, currency, at, at).Order("created_at DESC,id DESC").First(&row).Error; e != nil {
		return payments.Money{}, payments.Valuation{}, payments.ErrNotConfigured
	}
	r := new(big.Rat).Quo(due.Rat(), row.USDPerUnit.Rat())
	amount, remainder := ledger.Quantize(r)
	if remainder.Sign() != 0 {
		return payments.Money{}, payments.Valuation{}, payments.ErrInvalid
	}
	return payments.Money{Currency: currency, Amount: amount}, payments.Valuation{ID: row.ID, Currency: row.Currency, USDPerUnit: row.USDPerUnit, Source: row.Source, EffectiveAt: row.EffectiveAt, ExpiresAt: row.ExpiresAt}, nil
}
func (a *Application) ensureCheckout(ctx context.Context, o commerce.Order) error {
	if o.Status == "completed" {
		return nil
	}
	var cr checkoutRow
	var paymentOrder payments.Order
	e := a.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", "checkout:"+o.ID).Error; e != nil {
			return e
		}
		if e := tx.First(&cr, "order_id=? AND customer_id=?", o.ID, o.CustomerID).Error; e != nil {
			return e
		}
		if cr.MerchantID == nil {
			return nil
		}
		if len(cr.PaymentOrder) > 0 {
			return json.Unmarshal(cr.PaymentOrder, &paymentOrder)
		}
		methods, e := a.Payments.PublicMethods(ctx, o.OperatorID)
		if e != nil {
			return e
		}
		currency := ""
		for _, m := range methods {
			if m.ID == *cr.MerchantID {
				currency = m.Currency
			}
		}
		if currency == "" {
			return payments.ErrNotConfigured
		}
		due := o.DueUSD.Sub(o.InternalUSD).Sub(o.CommissionUSD)
		money, value, e := a.paymentValue(ctx, o.OperatorID, currency, due, o.CreatedAt)
		if e != nil {
			return e
		}
		paymentOrder = payments.Order{ID: o.ID, OperatorID: o.OperatorID, MerchantID: *cr.MerchantID, Money: money, Valuation: value, AcceptedAt: o.CreatedAt, ExpiresAt: o.ExpiresAt, NotifyURL: a.Config.PublicBaseURL + "/v1/payments/webhooks/" + *cr.MerchantID, Description: "TokenHub order " + o.ID}
		raw, e := json.Marshal(paymentOrder)
		if e != nil {
			return e
		}
		return tx.Model(&checkoutRow{}).Where("order_id=?", o.ID).Update("payment_order", raw).Error
	})
	if e != nil {
		return e
	}
	if cr.MerchantID == nil {
		return nil
	}
	_, e = a.Payments.Create(ctx, o.OperatorID, *cr.MerchantID, paymentOrder)
	return e
}
func (a *Application) orderDTO(ctx context.Context, o commerce.Order) (gin.H, error) {
	op, e := a.Organization.GetOperator(ctx, nil, o.OperatorID)
	if e != nil {
		return nil, e
	}
	dto := gin.H{"id": o.ID, "status": o.Status, "kind": o.Kind, "operator_name": op.Name, "amount_usd": o.DueUSD, "paid_usd": o.ExternalAllocatedUSD, "delivered_at": o.CompletedAt, "created_at": o.CreatedAt, "expires_at": o.ExpiresAt, "purchase_atg": o.PaidATG, "gift_atg": o.PackageGiftATG.Add(o.MerchantGiftATG).Add(o.KOLGiftATG)}
	if o.InternalStatus == "captured" {
		dto["paid_usd"] = o.ExternalAllocatedUSD.Add(o.InternalUSD).Add(o.CommissionUSD)
	}
	if o.QuoteID != nil {
		q, e := a.Commerce.GetQuote(ctx, nil, *o.QuoteID)
		if e == nil {
			p, e := a.Commerce.GetPackage(ctx, nil, o.OperatorID, q.PackageID)
			if e == nil {
				dto["plan_name"] = p.Name
			}
		}
	}
	var cr checkoutRow
	e = a.DB.WithContext(ctx).First(&cr, "order_id=? AND customer_id=?", o.ID, o.CustomerID).Error
	if e != nil && !errors.Is(e, gorm.ErrRecordNotFound) {
		return nil, e
	}
	if e == nil && cr.MerchantID != nil && o.Status != "completed" {
		intent, e := a.Payments.PaymentIntent(ctx, o.OperatorID, *cr.MerchantID, o.ID)
		if e != nil && !errors.Is(e, gorm.ErrRecordNotFound) {
			return nil, e
		}
		if e == nil && intent.Checkout != nil {
			ch := intent.Checkout
			action := gin.H{}
			switch {
			case intent.Driver == "manual":
				action = gin.H{"kind": "offline", "address": ch.PaymentAddress, "instructions": ch.Instructions}
			case ch.ClientSecret != "":
				action = gin.H{"kind": "stripe", "client_secret": ch.ClientSecret, "publishable_key": ch.PublishableKey}
			case ch.URL != "":
				action = gin.H{"kind": "redirect", "url": ch.URL}
			case ch.PaymentAddress != "":
				action = gin.H{"kind": "stablecoin", "address": ch.PaymentAddress}
			default:
				action = gin.H{"kind": "offline"}
			}
			if ch.PayMoney != nil {
				action["amount"] = ch.PayMoney.Amount.String()
				action["currency"] = ch.PayMoney.Currency
			}
			action["network"], action["token_contract"] = intent.Network, intent.TokenContract
			if intent.Driver == "evm_wallet" {
				action["wallet_receipt_path"] = "/api/v1/me/orders/" + o.ID + "/wallet-receipts"
			}
			dto["payment_action"] = action
		}
	}
	return dto, nil
}
func (a *Application) orders(c *gin.Context) {
	rows, e := a.Commerce.ListCustomerOrdersPage(c.Request.Context(), nil, subject(c).Customer.Customer.ID, c.Query("cursor"), c.Query("q"), 51)
	if e != nil {
		handleError(c, e)
		return
	}
	next := ""
	if len(rows) > 50 {
		rows = rows[:50]
		next = commerce.NextOrderCursor(rows[len(rows)-1])
	}
	items := []any{}
	for _, o := range rows {
		dto, e := a.orderDTO(c.Request.Context(), o)
		if e != nil {
			handleError(c, e)
			return
		}
		items = append(items, dto)
	}
	c.JSON(200, gin.H{"items": items, "next_cursor": next})
}
func (a *Application) order(c *gin.Context) {
	o, e := a.Commerce.GetOrder(c.Request.Context(), nil, subject(c).Customer.Customer.ID, c.Param("id"))
	if e != nil {
		handleError(c, e)
		return
	}
	dto, e := a.orderDTO(c.Request.Context(), o)
	if e != nil {
		handleError(c, e)
		return
	}
	c.JSON(200, dto)
}
func (a *Application) reconcileOrder(c *gin.Context) {
	id := subject(c)
	ctx := c.Request.Context()
	o, e := a.Commerce.GetOrder(ctx, nil, id.Customer.Customer.ID, c.Param("id"))
	if e != nil {
		handleError(c, e)
		return
	}
	var cr checkoutRow
	e = a.DB.WithContext(ctx).First(&cr, "order_id=? AND customer_id=?", o.ID, o.CustomerID).Error
	if e != nil || cr.MerchantID == nil {
		handleError(c, commerce.ErrNotReady)
		return
	}
	if _, e = a.Payments.Reconcile(ctx, o.OperatorID, *cr.MerchantID, o.ID); e != nil {
		handleError(c, e)
		return
	}
	c.JSON(200, commandResult(o.ID, "reconciliation_queued"))
}
func (a *Application) topup(c *gin.Context) {
	var in struct {
		Amount ledger.Decimal `json:"amount_usd"`
		Method string         `json:"external_method_id"`
	}
	if !body(c, &in) {
		return
	}
	id := subject(c)
	k, _ := key(c)
	ctx := c.Request.Context()
	raw, e := a.command(c, "topup:"+id.Customer.Customer.ID, in, func(tx *gorm.DB) (any, error) {
		o, e := a.Commerce.CreateTopUpOrder(ctx, tx, commerce.CreateTopUpInput{CustomerID: id.Customer.Customer.ID, OperatorID: id.Customer.Ownership.OperatorID, BusinessKey: k, ExpectedOwnershipVersion: id.Customer.Ownership.Version, AmountUSD: in.Amount, Validity: 30 * time.Minute})
		if e != nil {
			return nil, e
		}
		m, e := a.Payments.Merchant(ctx, o.OperatorID, in.Method)
		if e != nil {
			return nil, e
		}
		if !m.Enabled {
			return nil, payments.ErrNotConfigured
		}
		e = tx.Create(&checkoutRow{OrderID: o.ID, CustomerID: o.CustomerID, MerchantID: &m.ID, CreatedAt: time.Now()}).Error
		return gin.H{"id": o.ID}, e
	})
	if e != nil {
		handleError(c, e)
		return
	}
	var result struct{ ID string }
	_ = json.Unmarshal(raw, &result)
	o, e := a.Commerce.GetOrder(ctx, nil, id.Customer.Customer.ID, result.ID)
	if e == nil {
		e = a.ensureCheckout(ctx, o)
	}
	if e != nil {
		handleError(c, e)
		return
	}
	dto, e := a.orderDTO(ctx, o)
	if e != nil {
		handleError(c, e)
		return
	}
	c.JSON(200, gin.H{"id": o.ID, "status": o.Status, "order": dto})
}
func (a *Application) subscriptions(c *gin.Context) {
	id := subject(c)
	rows, e := a.Commerce.ListCustomerDeliveries(c.Request.Context(), nil, id.Customer.Customer.ID, 100)
	if e != nil {
		handleError(c, e)
		return
	}
	items := []any{}
	ids := []string{}
	for _, row := range rows {
		if row.Delivery.GrantID != nil {
			ids = append(ids, *row.Delivery.GrantID)
		}
	}
	remainders, e := a.Ledger.GrantRemainders(c.Request.Context(), nil, id.Customer.Ownership.OperatorID, id.Customer.Customer.ID, ids)
	if e != nil {
		handleError(c, e)
		return
	}
	byGrant := map[string]ledger.GrantRemaining{}
	for _, r := range remainders {
		byGrant[r.GrantID] = r
	}
	for _, row := range rows {
		paid, gift := ledger.Zero(), ledger.Zero()
		if row.Delivery.GrantID != nil {
			remaining := byGrant[*row.Delivery.GrantID]
			paid = remaining.PurchasedATG
			gift = remaining.GiftATG
		}
		status := "active"
		if row.Delivery.ExpiresAt != nil && !time.Now().Before(*row.Delivery.ExpiresAt) {
			status = "expired"
		}
		out := gin.H{"id": row.Order.ID, "name": row.Name, "status": status, "purchase_remaining_atg": paid, "gift_remaining_atg": gift, "effective_at": row.Delivery.EffectiveAt, "expires_at": row.Delivery.ExpiresAt, "auto_renew": false, "allow_commission": false, "requires_confirmation": false, "version": 1}
		if s := row.Subscription; s != nil {
			out["id"] = s.ID
			out["auto_renew"] = s.Status == "active"
			out["allow_commission"] = s.AllowCommission
			out["version"] = s.Version
			out["status"] = s.Status
			out["requires_current_plan"] = s.OriginalOperatorID != s.OperatorID
			if s.OriginalOperatorID != s.OperatorID {
				out["changed_terms"] = []string{"经营责任已转至当前品牌方，请明确选择其当前可续费套餐及新条款，原成交历史保持。"}
			}
			out["requires_confirmation"] = s.Status == "needs_confirmation"
		}
		items = append(items, out)
	}
	c.JSON(200, gin.H{"items": items})
}
func (a *Application) renewal(c *gin.Context) {
	var in struct {
		AutoRenew       bool   `json:"auto_renew"`
		AllowCommission bool   `json:"allow_commission"`
		Version         int64  `json:"version"`
		Confirmed       bool   `json:"confirmed_changed_terms"`
		PlanID          string `json:"plan_id"`
	}
	if !body(c, &in) {
		return
	}
	id := subject(c)
	ctx := c.Request.Context()
	raw, e := a.command(c, "renewal:"+id.Customer.Customer.ID+":"+c.Param("id"), in, func(tx *gorm.DB) (any, error) {
		s, e := a.Commerce.GetSubscription(ctx, tx, id.Customer.Customer.ID, c.Param("id"))
		if errors.Is(e, gorm.ErrRecordNotFound) && in.AutoRenew {
			out, err := a.Commerce.EnableAutoRenewal(ctx, tx, id.Customer.Customer.ID, c.Param("id"), "customer explicitly enabled renewal", in.AllowCommission)
			if err != nil {
				return nil, err
			}
			return commandResult(out.ID, out.Status), nil
		}
		if e != nil {
			return nil, e
		}
		if s.Version != in.Version {
			return nil, commerce.ErrConflict
		}
		if !in.AutoRenew {
			e = a.Commerce.CancelAutoRenewal(ctx, tx, id.Customer.Customer.ID, s.ID)
		} else {
			planID := s.PackageID
			if s.OriginalOperatorID != s.OperatorID {
				if !in.Confirmed || in.PlanID == "" {
					return nil, commerce.ErrConflict
				}
				planID = in.PlanID
			}
			p, err := a.Commerce.GetPackage(ctx, tx, s.OperatorID, planID)
			if err != nil {
				return nil, err
			}
			if p.CurrentQuoteID == nil {
				return nil, commerce.ErrNotReady
			}
			if !in.Confirmed && s.AuthorizedQuoteID != *p.CurrentQuoteID {
				return nil, commerce.ErrConflict
			}
			_, e = a.Commerce.ConfirmRenewal(ctx, tx, id.Customer.Customer.ID, s.ID, *p.CurrentQuoteID, "explicit browser renewal consent", in.Version, in.AllowCommission)
		}
		return commandResult(s.ID, "updated"), e
	})
	jsonResponse(c, raw, e)
}

// ReceiptSink persists verified external facts first, then schedules separate
// recoverable allocation/delivery work. A late/partial receipt stays auditable.
type portalReceiptSink struct {
	Application *Application
	payments.CommerceSink
}

func (s portalReceiptSink) LockReceiptAcceptance(ctx context.Context, tx *gorm.DB, operator, orderID string) error {
	customer, e := s.Application.Commerce.PaymentOrderCustomer(ctx, tx, operator, orderID)
	if e == nil {
		return s.Application.Organization.LockCustomerResponsibility(ctx, tx, customer)
	}
	if !errors.Is(e, gorm.ErrRecordNotFound) {
		return e
	}
	buyer, e := s.Application.Cooperation.ProcurementBuyerForPayment(ctx, tx, operator, orderID)
	if e == nil {
		return s.Application.Organization.LockOperatorResponsibility(ctx, tx, buyer)
	}
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return nil
	}
	return e
}

func (s portalReceiptSink) RecordVerifiedReceipt(ctx context.Context, tx *gorm.DB, m payments.Merchant, o payments.Order, f payments.Fact, v payments.USDValue) (string, error) {
	id, e := s.CommerceSink.RecordVerifiedReceipt(ctx, tx, m, o, f, v)
	if e != nil {
		return "", e
	}
	payload, _ := json.Marshal(map[string]string{"receipt_id": id, "order_id": o.ID, "operator_id": o.OperatorID})
	_, e = s.Application.Jobs.EnqueueTx(tx, jobs.Input{Queue: "portal-delivery", Kind: "receipt", BusinessKey: id + ":" + o.ID, Payload: payload, MaxAttempts: 20})
	return id, e
}
func (a *Application) RunDeliveryJobs(ctx context.Context, owner string, limit int) error {
	rows, e := a.Jobs.Claim(ctx, "portal-delivery", owner, 30*time.Second, limit)
	if e != nil {
		return e
	}
	for _, job := range rows {
		var p struct {
			ReceiptID  string `json:"receipt_id"`
			OrderID    string `json:"order_id"`
			OperatorID string `json:"operator_id"`
		}
		e = json.Unmarshal(job.Payload, &p)
		if e == nil {
			e = a.deliverReceipt(ctx, p.OperatorID, p.ReceiptID, p.OrderID)
		}
		if e != nil {
			_ = a.Jobs.Fail(ctx, job.ID, owner, job.Fence, "allocation or delivery requires retry/review")
			continue
		}
		if e = a.Jobs.Complete(ctx, job.ID, owner, job.Fence); e != nil {
			return e
		}
	}
	return nil
}
func (a *Application) deliverReceipt(ctx context.Context, op, receiptID, orderID string) error {
	// These IDs originate exclusively from verified payment intents/jobs.
	var procurement procurementCheckout
	q := a.DB.WithContext(ctx).Where("order_id=?", orderID).Limit(1).Find(&procurement)
	if q.Error != nil {
		return q.Error
	}
	if q.RowsAffected > 0 {
		return a.deliverProcurementReceipt(ctx, procurement, receiptID)
	}
	var cr checkoutRow
	if e := a.DB.WithContext(ctx).First(&cr, "order_id=?", orderID).Error; e != nil {
		return e
	}
	var order commerce.Order
	e := a.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		o, e := a.Commerce.GetOrder(ctx, tx, cr.CustomerID, orderID)
		if e != nil {
			return e
		}
		order = o
		if o.OperatorID != op {
			return commerce.ErrScope
		}
		r, e := a.Commerce.GetReceipt(ctx, tx, op, receiptID)
		if e != nil {
			return e
		}
		due := o.DueUSD.Sub(o.InternalUSD).Sub(o.CommissionUSD).Sub(o.ExternalAllocatedUSD)
		amount := r.GrossUSD.Sub(r.AllocatedUSD)
		if amount.Cmp(due) > 0 {
			amount = due
		}
		if amount.Sign() > 0 {
			_, e = a.Commerce.AllocateReceipt(ctx, tx, commerce.AllocateReceiptInput{OperatorID: op, BusinessKey: "verified-payment:" + receiptID + ":" + orderID, ReceiptID: receiptID, OrderID: orderID, AmountUSD: amount})
		}
		return e
	})
	if e != nil {
		return e
	}
	return a.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, e := a.Commerce.Fulfill(ctx, tx, orderID, "verified-payment-worker"); e != nil {
			return e
		}
		return a.authorizeRenewal(ctx, tx, order, cr)
	})
}
func (a *Application) paymentWebhook(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20)
	raw, e := io.ReadAll(c.Request.Body)
	if e != nil {
		fail(c, 400, "invalid_webhook", "回调内容无效", false)
		return
	}
	_, e = a.Payments.ReceiveWebhook(c.Request.Context(), c.Param("merchantId"), payments.Webhook{Headers: c.Request.Header.Clone(), Body: raw, ReceivedAt: time.Now().UTC()})
	if e != nil {
		fail(c, 400, "invalid_webhook", "回调签名或商户配置无效", false)
		return
	}
	c.JSON(200, gin.H{"received": true})
}
