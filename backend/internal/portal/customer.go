package portal

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"github.com/tokpath/tokstudio/backend/internal/usage"
	"gorm.io/gorm"
)

func (a *Application) customerRoutes(r *gin.RouterGroup) {
	for _, resource := range []string{"invoices", "refunds", "account", "topups", "activities", "withdrawals", "credit-sources"} {
		resource := resource
		r.GET("/"+resource+"/workspace", func(c *gin.Context) {
			c.Params = append(c.Params, gin.Param{Key: "resource", Value: resource})
			a.customerWorkspace(c)
		})
	}
	r.GET("/notifications/preferences", a.notificationPreferences)
	r.GET("/notifications", a.notifications)
	r.POST("/notifications/:id/read", a.readNotification)
	r.POST("/notifications/preferences", a.setNotificationPreferences)
	r.GET("/refunds/:id/destinations", a.customerRefundDestinations)
	r.POST("/refunds/legs/:id/destination", a.proposeRefundDestination)
	r.POST("/refunds/legs/:id/responsible-destination", a.proposeResponsibleRefundDestination)
	r.GET("/refunds", a.refunds)
	r.POST("/refunds", a.requestRefund)
	r.POST("/refunds/:id/cancel", a.cancelRefund)
	r.GET("/references/:reference", a.references)
	r.GET("/invoices/:id/files", a.invoiceFiles)
	r.GET("/invoices/:id/files/:fileID", a.downloadInvoiceFile)
	r.POST("/invoices/:id/replacement", a.replaceInvoice)
	r.POST("/invoices", a.requestInvoice)
	r.POST("/activities", a.publishKOL)
	r.POST("/account/email-changes", a.emailChange)
	r.POST("/account/waive-credits", a.waiveClosingCredits)
	r.POST("/account/lifecycle/:action", a.customerLifecycle)
	r.GET("/account/closure", a.closureStatus)
	r.GET("/credits", a.credits)
	r.GET("/models", a.models)
	r.GET("/plans/catalog", a.plans)
	r.GET("/plans", a.subscriptions)
	r.GET("/plans/:id/offers", a.offers)
	r.PATCH("/plans/:id/renewal", a.renewal)
	r.GET("/keys", a.listKeys)
	r.POST("/keys", a.createKey)
	r.PATCH("/keys/:id", a.updateKey)
	r.POST("/keys/:id/reset", a.resetKey)
	r.POST("/keys/:id/enable", a.setKeyState("active"))
	r.POST("/keys/:id/disable", a.setKeyState("disabled"))
	r.DELETE("/keys/:id", a.setKeyState("deleted"))
	r.GET("/orders", a.orders)
	r.GET("/orders/:id", a.order)
	r.POST("/orders/quotes", a.quote)
	r.POST("/orders", a.createOrder)
	r.POST("/orders/:id/reconcile", a.reconcileOrder)
	r.POST("/orders/:id/wallet-receipts", a.customerWalletReceipt)
	r.GET("/requests", a.requests)
	r.GET("/requests/:id", a.requestDetail)
	r.GET("/requests/:id/result", a.requestResult)
	r.POST("/requests/:id/cancel", a.cancelRequest)
	r.POST("/playground", a.playground)
	r.GET("/referrals", a.referrals)
	r.GET("/commissions", a.commissions)
	r.GET("/:resource/workspace", a.customerWorkspace)
	r.POST("/topups", a.topup)
	r.POST("/withdrawals/quotes", a.withdrawalQuote)
	r.POST("/withdrawals/quotes/:id/confirm", a.confirmWithdrawal)
	r.POST("/withdrawals/:id/cancel", a.cancelWithdrawal)
}
func (a *Application) publicOperator(c *gin.Context) (organization.Operator, error) {
	if id := subject(c); id != nil && id.Session.Kind == "customer" {
		return a.Organization.GetOperator(c.Request.Context(), nil, id.Customer.Ownership.OperatorID)
	}
	if code := strings.TrimSpace(inviteCode(c)); code != "" {
		at, e := a.Organization.ResolveRegistrationIntent(c.Request.Context(), nil, brand(c).ID, code)
		if e != nil {
			return organization.Operator{}, e
		}
		return at.Operator, nil
	}
	return a.Organization.GetOperator(c.Request.Context(), nil, brand(c).OwnerOperatorID)
}
func (a *Application) publicContext(c *gin.Context) {
	ctx := c.Request.Context()
	b := brand(c)
	op, e := a.Organization.GetOperator(ctx, nil, b.OwnerOperatorID)
	if e != nil {
		handleError(c, e)
		return
	}
	if id := subject(c); id != nil && id.Session.Kind == "customer" && inviteCode(c) == "" {
		op, e = a.Organization.GetOperator(ctx, nil, id.Customer.Ownership.OperatorID)
		if e != nil {
			handleError(c, e)
			return
		}
	}
	var invitation any
	if code := strings.TrimSpace(inviteCode(c)); code != "" {
		at, err := a.Organization.ResolveRegistrationIntent(ctx, nil, b.ID, code)
		if err != nil {
			invitation = gin.H{"code": code, "status": "invalid", "message": "邀请码无效、已停用或不属于当前品牌，请修改或明确移除"}
		} else {
			op = at.Operator
			t := "channel"
			if at.InviterID != nil {
				t = "user"
			}
			invitation = gin.H{"code": code, "status": "valid", "type": t, "operator": a.operatorDTO(ctx, op)}
			if at.InviterID != nil {
				person, err := a.Organization.GetCustomer(ctx, nil, *at.InviterID)
				if err == nil {
					invitation.(gin.H)["inviter_display_name"] = person.DisplayName
				}
			}
		}
	}
	theme, e := a.Branding.ResolveTheme(ctx, nil, b.ID, channelThemeID(op))
	if e != nil {
		handleError(c, e)
		return
	}
	status, e := a.Organization.GetBusinessStatus(ctx, nil, op.ID)
	if e != nil {
		handleError(c, e)
		return
	}
	available := []gin.H{{"capability": "registration", "enabled": status.Registration && a.Mail.Available()}, {"capability": "new_purchase", "enabled": status.NewPurchase}, {"capability": "renewal", "enabled": status.Renewal}, {"capability": "model_usage", "enabled": status.ModelUsage}, {"capability": "activity", "enabled": status.Activity}, {"capability": "settlement", "enabled": status.Settlement}}
	for _, entry := range available {
		if !entry["enabled"].(bool) {
			entry["reason"] = "当前经营方尚未开放或配置未完成"
		}
	}
	methods, e := a.Payments.PublicMethods(ctx, op.ID)
	if e != nil {
		handleError(c, e)
		return
	}
	pm := []any{}
	for _, m := range methods {
		kind := "online"
		if m.Driver == "manual" {
			kind = "offline"
		}
		if m.Driver == "nowpayments" || m.Driver == "evm_wallet" {
			kind = "stablecoin"
		}
		pm = append(pm, gin.H{"id": m.ID, "name": m.Driver, "kind": kind, "status": "ready", "currency": m.Currency, "instructions": m.Instructions})
	}
	result := gin.H{"brand": a.brandDTO(ctx, b), "operator": a.operatorDTO(ctx, op), "theme": theme.Theme, "availability": available, "payment_methods": pm}
	if invitation != nil {
		result["invitation"] = invitation
	}
	c.JSON(200, result)
}
func (a *Application) models(c *gin.Context) {
	op, e := a.publicOperator(c)
	if e != nil {
		handleError(c, e)
		return
	}
	models, e := a.Catalog.PublicCatalog(c.Request.Context(), op.ID, time.Now().UTC())
	if e != nil {
		handleError(c, e)
		return
	}
	items := []any{}
	for _, m := range models {
		caps := []string{}
		seen := map[string]bool{}
		for _, cap := range m.Capabilities {
			operation := string(cap.Operation)
			if operation == "image_generation" {
				operation = "image"
			}
			if operation == "video_generation" {
				operation = "video"
			}
			if !seen[operation] {
				caps = append(caps, operation)
				seen[operation] = true
			}
		}
		prices := []any{}
		for _, p := range m.Prices {
			prices = append(prices, gin.H{"component": p.Component, "amount_atg": p.AmountATG, "unit": p.Unit + " / " + p.Per, "tiers": p.Tiers})
		}
		status := "active"
		if m.Status != "available" {
			status = m.Status
		}
		items = append(items, gin.H{"id": m.ID, "name": m.Name, "status": status, "capabilities": caps, "prices": prices, "capability_details": m.Capabilities, "upcoming_changes": m.UpcomingChanges})
	}
	c.JSON(200, gin.H{"items": items})
}
func validity(p ledger.Period) string {
	if p == ledger.Lifetime {
		return "unlimited"
	}
	return string(p)
}
func (a *Application) plans(c *gin.Context) {
	op, e := a.publicOperator(c)
	if e != nil {
		handleError(c, e)
		return
	}
	pkgs, e := a.Commerce.ListPackages(c.Request.Context(), nil, op.ID)
	if e != nil {
		handleError(c, e)
		return
	}
	items := []any{}
	for _, p := range pkgs {
		if p.CurrentQuoteID == nil {
			continue
		}
		q, e := a.Commerce.GetQuote(c.Request.Context(), nil, *p.CurrentQuoteID)
		if e != nil {
			handleError(c, e)
			return
		}
		status := "disabled"
		if p.NewPurchaseEnabled {
			status = "active"
		}
		items = append(items, gin.H{"id": p.ID, "name": p.Name, "price_usd": q.PriceUSD, "purchase_atg": q.PaidATG, "gift_atg": q.GiftATG, "validity": validity(q.Period), "auto_renew_available": p.RenewalEnabled && q.Period != ledger.Lifetime, "status": status, "version": q.Version, "description": q.TermsReference})
	}
	c.JSON(200, gin.H{"items": items})
}
func (a *Application) credits(c *gin.Context) {
	id := subject(c)
	ctx := c.Request.Context()
	customer, operator := id.Customer.Customer.ID, id.Customer.Ownership.OperatorID
	summary, e := a.Ledger.CustomerAssetSummary(ctx, nil, operator, customer, time.Time{})
	if e != nil {
		handleError(c, e)
		return
	}
	account := func(purpose string) (ledger.Account, error) {
		row, e := a.Ledger.FindAccount(ctx, nil, operator, customer, purpose)
		if errors.Is(e, gorm.ErrRecordNotFound) {
			return ledger.Account{}, nil
		}
		return row, e
	}
	usd, e := account("customer:usd")
	if e != nil {
		handleError(c, e)
		return
	}
	comm, e := account("commission:available")
	if e != nil {
		handleError(c, e)
		return
	}
	frozen, e := account("commission:frozen")
	if e != nil {
		handleError(c, e)
		return
	}

	overview, e := a.Incentives.PromotionOverview(ctx, nil, operator, customer)
	q := overview.Qualification
	if e != nil {
		handleError(c, e)
		return
	}
	policy, e := a.Incentives.CurrentPolicy(ctx, nil, operator)
	if e != nil {
		handleError(c, e)
		return
	}
	spendable, e := a.Incentives.CommissionSpendable(ctx, nil, operator, customer)
	if e != nil {
		handleError(c, e)
		return
	}
	restricted := ledger.Zero()
	if spendable.Cmp(comm.Available()) < 0 {
		restricted = comm.Available().Sub(spendable)
	} else {
		spendable = comm.Available()
	}
	c.JSON(200, gin.H{"customer_atg": gin.H{"available": summary.AvailableATG, "held": summary.HeldATG, "purchased_remaining": summary.PurchasedRemainingATG, "gift_remaining": summary.GiftRemainingATG, "expired": summary.ExpiredATG}, "usd_balance": gin.H{"available": usd.Available(), "held": usd.Held}, "commission_usd": gin.H{"available": spendable, "source_restricted": restricted, "held": comm.Held, "frozen": frozen.Balance, "recovery_due": q.PendingOffsetUSD}, "qualification": gin.H{"confirmed_usd": q.ConfirmedUSD, "pending_usd": overview.PendingConsumptionUSD, "pending_correction_usd": overview.PendingCorrectionUSD, "threshold_usd": policy.ThresholdUSD, "qualified_at": q.AcquiredAt, "function_status": overview.FunctionState.Status}})
}
func (a *Application) keyDTO(ctx context.Context, k usage.Key) (gin.H, error) {
	return a.keyDTOIn(ctx, nil, k)
}
func (a *Application) keyDTOIn(ctx context.Context, tx *gorm.DB, k usage.Key) (gin.H, error) {
	budget, e := a.Usage.KeyBudget(ctx, tx, k, time.Time{})
	if e != nil {
		return nil, e
	}
	var models []string
	_ = json.Unmarshal(k.ModelIDs, &models)
	return gin.H{"id": k.ID, "name": k.Name, "prefix": k.Prefix, "status": k.Status, "model_ids": models, "budget": budget, "rate_limit_rpm": k.RateLimitRPM, "concurrency_limit": k.ConcurrencyLimit, "version": k.Version, "created_at": k.CreatedAt}, nil
}
func (a *Application) listKeys(c *gin.Context) {
	rows, e := a.Usage.ListKeys(c.Request.Context(), subject(c).Customer.Customer.ID)
	if e != nil {
		handleError(c, e)
		return
	}
	items := []any{}
	for _, k := range rows {
		dto, e := a.keyDTO(c.Request.Context(), k)
		if e != nil {
			handleError(c, e)
			return
		}
		items = append(items, dto)
	}
	c.JSON(200, gin.H{"items": items})
}

type keyInput struct {
	Name     string   `json:"name"`
	ModelIDs []string `json:"model_ids"`
	Budget   struct {
		LimitATG *ledger.Decimal `json:"limit_atg"`
		Period   ledger.Period   `json:"period"`
		Timezone string          `json:"timezone"`
	} `json:"budget"`
	RateLimitRPM     *int64 `json:"rate_limit_rpm"`
	ConcurrencyLimit *int64 `json:"concurrency_limit"`
	Version          int64  `json:"version"`
}

func (in keyInput) domain() usage.KeyInput {
	return usage.KeyInput{Name: in.Name, ModelIDs: in.ModelIDs, BudgetLimit: in.Budget.LimitATG, Period: in.Budget.Period, Timezone: in.Budget.Timezone, RateLimitRPM: in.RateLimitRPM, ConcurrencyLimit: in.ConcurrencyLimit}
}
func (a *Application) createKey(c *gin.Context) {
	var in keyInput
	if !body(c, &in) {
		return
	}
	k, _ := key(c)
	out, e := a.Usage.CreateKey(c.Request.Context(), subject(c).Customer.Customer.ID, k, in.domain())
	if e != nil {
		handleError(c, e)
		return
	}
	dto, e := a.keyDTO(c.Request.Context(), out.Key)
	if e != nil {
		handleError(c, e)
		return
	}
	c.JSON(200, gin.H{"key": dto, "secret": out.Secret})
}
func (a *Application) updateKey(c *gin.Context) {
	var in keyInput
	if !body(c, &in) {
		return
	}
	customer := subject(c).Customer.Customer.ID
	ctx := c.Request.Context()
	raw, e := a.command(c, "key-config:"+customer+":"+c.Param("id"), in, func(tx *gorm.DB) (any, error) {
		svc := *a.Usage
		svc.DB = tx
		out, e := svc.UpdateKey(ctx, customer, c.Param("id"), in.Version, in.domain())
		if e != nil {
			return nil, e
		}
		return a.keyDTOIn(ctx, tx, out)
	})
	jsonResponse(c, raw, e)
}
func (a *Application) resetKey(c *gin.Context) {
	var in struct{ Version int64 }
	if !body(c, &in) {
		return
	}
	k, _ := key(c)
	out, e := a.Usage.ResetKey(c.Request.Context(), subject(c).Customer.Customer.ID, c.Param("id"), k, in.Version)
	if e != nil {
		handleError(c, e)
		return
	}
	dto, e := a.keyDTO(c.Request.Context(), out.Key)
	if e != nil {
		handleError(c, e)
		return
	}
	c.JSON(200, gin.H{"key": dto, "secret": out.Secret})
}
func (a *Application) setKeyState(state string) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in struct{ Version int64 }
		if !body(c, &in) {
			return
		}
		customer := subject(c).Customer.Customer.ID
		raw, e := a.command(c, "key-state:"+customer+":"+c.Param("id")+":"+state, in, func(tx *gorm.DB) (any, error) {
			svc := *a.Usage
			svc.DB = tx
			out, e := svc.SetKeyState(c.Request.Context(), customer, c.Param("id"), state, in.Version)
			return gin.H{"id": out.ID, "status": out.Status, "version": out.Version}, e
		})
		jsonResponse(c, raw, e)
	}
}
func (a *Application) commissions(c *gin.Context) {
	id := subject(c)
	rows, next, e := a.Incentives.CommissionViews(c.Request.Context(), nil, id.Customer.Ownership.OperatorID, id.Customer.Customer.ID, c.Query("cursor"), 50)
	if e != nil {
		handleError(c, e)
		return
	}
	c.JSON(200, gin.H{"items": rows, "next_cursor": next})
}
func inviteCode(c *gin.Context) string {
	if v := c.Query("invite_code"); v != "" {
		return v
	}
	return c.Query("invite")
}
