package portal

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/incentives"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"gorm.io/gorm"
)

type field struct {
	LookupURL   string           `json:"lookup_url,omitempty"`
	VisibleWhen *fieldVisibility `json:"visible_when,omitempty"`
	Name        string           `json:"name"`
	Label       string           `json:"label"`
	Kind        string           `json:"kind"`
	Required    bool             `json:"required,omitempty"`
	Help        string           `json:"help,omitempty"`
	Options     []gin.H          `json:"options,omitempty"`
	Default     any              `json:"default_value,omitempty"`
}
type operation struct {
	Name, Label, Method, Path             string
	Fields                                []field
	Description, Confirmation, Permission string
	IncludeVersion                        bool
}

func (o operation) MarshalJSON() ([]byte, error) {
	return json.Marshal(gin.H{"name": o.Name, "label": o.Label, "method": o.Method, "path": o.Path, "fields": businessFields(o), "description": o.Description, "confirmation": o.Confirmation, "permission": o.Permission, "include_version": o.IncludeVersion})
}
func optional(name, label, kind string) field { return field{Name: name, Label: label, Kind: kind} }
func selectField(name, label string, options ...string) field {
	v := f(name, label, "select")
	for _, option := range options {
		parts := strings.SplitN(option, "|", 2)
		text := parts[0]
		if len(parts) > 1 {
			text = parts[1]
		}
		v.Options = append(v.Options, gin.H{"value": parts[0], "label": text})
	}
	return v
}
func defaultField(name, label, kind string, value any) field {
	v := f(name, label, kind)
	v.Default = value
	return v
}
func f(name, label, kind string) field {
	return field{Name: name, Label: label, Kind: kind, Required: true}
}
func jsonField(help string) field {
	v := f("configuration", "配置 JSON", "json")
	v.Help = help
	return v
}
func page(title string) gin.H {
	return gin.H{"title": title, "description": "当前经营范围的真实记录；金额按原始高精度十进制展示", "items": []any{}, "columns": []gin.H{}, "operations": []operation{}, "row_operations": []operation{}}
}
func values(v any) gin.H {
	raw, _ := json.Marshal(v)
	var out gin.H
	_ = json.Unmarshal(raw, &out)
	return out
}
func entity(id, status string, version int64, v any) gin.H {
	return gin.H{"id": id, "status": status, "version": version, "values": values(v)}
}
func col(key, label, kind string) gin.H { return gin.H{"key": key, "label": label, "kind": kind} }
func (a *Application) referrals(c *gin.Context) {
	id := subject(c)
	ctx := c.Request.Context()
	op, customer := id.Customer.Ownership.OperatorID, id.Customer.Customer.ID
	overview, e := a.Incentives.PromotionOverview(ctx, nil, op, customer)
	if e != nil {
		handleError(c, e)
		return
	}
	policy, e := a.Incentives.CurrentPolicy(ctx, nil, op)
	if e != nil {
		handleError(c, e)
		return
	}
	code, e := a.Organization.CustomerInvite(ctx, nil, customer)
	if e != nil {
		handleError(c, e)
		return
	}
	scheme := "https"
	if !a.Config.IsProduction() {
		scheme = "http"
	}
	c.JSON(200, gin.H{"invite_code": code, "invite_url": scheme + "://" + a.requestHost(c) + "/register?invite=" + code, "direct_count": overview.DirectInvites, "indirect_count": overview.IndirectInvites, "qualified": overview.Qualification.AcquiredAt != nil, "function_status": overview.FunctionState.Status, "confirmed_money": overview.Qualification.ConfirmedMoney, "pending_money": overview.PendingConsumptionMoney, "threshold_money": policy.ThresholdMoney, "direct_rate": policy.DirectRate, "indirect_rate": policy.IndirectRate})
}
func (a *Application) customerWorkspace(c *gin.Context) {
	id := subject(c)
	ctx := c.Request.Context()
	op, customer := id.Customer.Ownership.OperatorID, id.Customer.Customer.ID
	resource := c.Param("resource")
	p := page(resource)
	switch resource {
	case "refunds":
		a.refundWorkspace(c, p)
		return
	case "topups":
		p["title"] = "结算币种 充值"
		rows, e := a.Commerce.ListCustomerOrdersPage(ctx, nil, customer, c.Query("cursor"), "topup", 50)
		if e != nil {
			handleError(c, e)
			return
		}
		items := []any{}
		for _, o := range rows {
			items = append(items, entity(o.ID, o.Status, 1, o))
		}
		p["items"] = items
		p["columns"] = []gin.H{col("id", "订单", "text"), col("due_money", "应付", "money"), col("external_allocated_money", "已分配", "money"), col("created_at", "创建时间", "datetime")}
		methods, e := a.Payments.PublicMethods(ctx, op)
		if e != nil {
			handleError(c, e)
			return
		}
		options := []gin.H{}
		for _, m := range methods {
			options = append(options, gin.H{"value": m.ID, "label": m.Driver + " · " + m.Currency})
		}
		if len(options) > 0 {
			method := f("external_method_id", "收款方式", "select")
			method.Options = options
			p["operations"] = []operation{{Name: "topup", Label: "创建充值订单", Method: "POST", Path: "/api/v1/me/topups", Fields: []field{f("amount_money", "充值金额 结算币种", "decimal"), method}, Confirmation: "确认按当前经营方收款方式创建充值订单。支付完成并核实后才进入 结算币种 余额。"}}
		}
	case "activities":
		rows, e := a.Incentives.OwnCampaigns(ctx, nil, op, customer)
		if e != nil {
			handleError(c, e)
			return
		}
		items := []any{}
		for _, row := range rows {
			v := values(row.Version)
			v["kind"] = row.Campaign.Kind
			v["used_uses"], v["reserved_uses"] = row.Campaign.UsedUses, row.Campaign.ReservedUses
			items = append(items, entity(row.Campaign.ID, campaignStatus(row.Version, time.Now()), row.Version.Version, v))
		}
		p["title"] = "我的 KOL 活动"
		p["items"] = items
		p["columns"] = []gin.H{col("package_id", "套餐", "text"), col("gift_atg", "赠送", "atg"), col("concession_rate", "本人佣金让利比例", "decimal"), col("starts_at", "开始", "datetime"), col("ends_at", "结束", "datetime")}
		p["operations"] = []operation{{Name: "publish", Label: "发布或修订活动", Method: "POST", Path: "/api/v1/me/activities", Fields: []field{jsonField("经营范围及活动归属由服务器确定；填写 campaign_id、package_id、gift_atg、concession_rate、starts_at、ends_at、per_customer_limit、enabled、expected_version 等实际条款。")}, Confirmation: "确认按当前资格及让利额度发布，活动只对有效直属受邀客户生效。"}}
	case "credit-sources":
		rows, e := a.Ledger.CustomerGrants(ctx, nil, op, customer)
		if e != nil {
			handleError(c, e)
			return
		}
		items := []any{}
		for _, g := range rows {
			items = append(items, entity(g.ID, "issued", 1, g))
		}
		p["title"] = "ATG 来源与期限"
		p["items"] = items
		p["columns"] = []gin.H{col("order_id", "来源订单", "text"), col("total", "总额", "atg"), col("consumed", "已消费", "atg"), col("expired", "已过期", "atg"), col("cancelled", "已取消", "atg"), col("expires_at", "到期", "datetime")}
	case "withdrawals":
		rows, e := a.Incentives.Withdrawals(ctx, nil, op, customer)
		if e != nil {
			handleError(c, e)
			return
		}
		items := []any{}
		for _, w := range rows {
			row := entity(w.ID, w.Status, 1, w)
			row["allowed_actions"] = []string{}
			if w.Status == "requested" {
				row["allowed_actions"] = []string{"cancel"}
			}
			items = append(items, row)
		}
		var previews []withdrawalPreview
		if e := a.DB.WithContext(ctx).Where("customer_id=? AND operator_id=? AND expires_at>?", customer, op, time.Now()).Order("created_at DESC,id DESC").Limit(50).Find(&previews).Error; e != nil {
			handleError(c, e)
			return
		}
		methods, err := a.Incentives.WithdrawalMethods(ctx, nil, op, true)
		if err != nil {
			handleError(c, err)
			return
		}
		active := map[string]incentives.WithdrawalPolicy{}
		for _, method := range methods {
			active[method.ID] = method
		}
		for _, preview := range previews {
			var quote incentives.WithdrawalQuote
			if json.Unmarshal(preview.Quote, &quote) != nil {
				handleError(c, incentives.ErrInvalidInput)
				return
			}
			method, available := active[quote.Policy.ID]
			if !available || quote.AmountMoney.Cmp(method.MinimumMoney) < 0 {
				continue
			}
			row := entity(preview.ID, "quoted", 1, gin.H{"method_name": quote.Policy.MethodName, "amount_money": quote.AmountMoney, "fee_money": quote.FeeMoney, "net_money": quote.NetMoney, "expires_at": preview.ExpiresAt})
			row["allowed_actions"] = []string{"confirm"}
			items = append(items, row)
		}
		p["row_operations"] = []operation{{Name: "confirm", Label: "确认费用并提交", Method: "POST", Path: "/api/v1/me/withdrawals/quotes/{id}/confirm", Fields: []field{f("recipient_reference", "本人真实收款资料", "textarea")}, Confirmation: "确认当前行的总额、手续费和实付金额，提交后将占用本人可用佣金。"}, {Name: "cancel", Label: "取消提现申请", Method: "POST", Path: "/api/v1/me/withdrawals/{id}/cancel", Fields: []field{}, Description: "仅尚未审核的本人提现申请可以取消。", Confirmation: "确认取消这笔尚未审核的提现申请？取消成功后释放本笔佣金占用；已进入审核或付款环节的申请须按最新状态继续办理。"}}
		p["title"] = "佣金提现"
		p["items"] = items
		p["columns"] = []gin.H{col("method_name", "提现方式", "text"), col("amount_money", "总额", "money"), col("fee_money", "手续费", "money"), col("net_money", "实付", "money"), col("recipient_reference", "收款资料", "text"), col("status", "状态", "status")}
		amount := f("amount_money", "提现总额 结算币种", "decimal")
		quote := operation{Name: "quote", Label: "核算提现费用", Method: "POST", Path: "/api/v1/me/withdrawals/quotes", Fields: []field{amount}}

		method := f("method_id", "提现方式", "select")
		for _, m := range methods {
			method.Options = append(method.Options, gin.H{"value": m.MethodID, "label": m.MethodName})
		}
		if len(methods) > 0 {
			method.Default = methods[0].MethodID
			minimum := methods[0].MinimumMoney.String()
			rule := "单笔提现总额不能低于经营方公布的最低额度 " + minimum + " 结算币种。"
			p["description"] = rule
			p["summary"] = []gin.H{{"label": "最低提现额度", "value": minimum, "unit": "MONEY", "detail": "单笔提现总额不能低于经营方公布的这个额度。"}}
			amount.Help = "费用从提现总额中扣除，确认后固定；经营方按实付金额线下付款。"
			quote.Fields = []field{method, amount}
			quote.Description = rule
		} else {
			p["description"] = "请等待经营方配置可用提现方式。"
			p["operations"] = []operation{}
			p["payout_methods"] = []gin.H{}
			p["currency_digits"] = incentives.CurrencyMinorDigits("USD")
			a.resourceResponse(c, p)
			return
		}
		settings, settingsErr := ledger.GetMonetary(ctx, a.DB)
		digits := 2
		if settingsErr == nil {
			digits = incentives.CurrencyMinorDigits(settings.Currency)
		}
		p["payout_methods"] = customerPayoutMethods(methods)
		p["currency_digits"] = digits
		p["operations"] = []operation{quote}
	case "account":
		email, e := a.Organization.CustomerEmail(ctx, nil, customer)
		if e != nil {
			handleError(c, e)
			return
		}
		p["title"] = "账号与安全"
		p["items"] = []any{entity(customer, id.Customer.Customer.Status, id.Customer.Customer.Version, gin.H{"email": email, "display_name": id.Customer.Customer.DisplayName, "brand_id": id.Customer.Customer.BrandID, "operator_id": op})}
		p["columns"] = []gin.H{col("email", "已验证邮箱", "text"), col("display_name", "显示名称", "text"), col("brand_id", "品牌身份范围", "text"), col("operator_id", "当前服务经营方", "text")}
		p["operations"] = []operation{{Name: "change_email", Label: "变更邮箱", Method: "POST", Path: "/api/v1/me/account/email-changes", Fields: []field{f("current_password", "当前密码", "password"), f("new_email", "新邮箱", "email")}, Description: "向新邮箱发送验证，完成后撤销旧凭据；邀请关系与资产保持连续。"}}
		customerState := id.Customer.Customer.Status
		actions := []string{}
		switch customerState {
		case "active":
			actions = []string{"stop", "begin_close"}
		case "stopped":
			actions = []string{"resume", "begin_close"}
		case "closing":
			actions = []string{"complete_close", "waive"}
		}
		accountRow := p["items"].([]any)[0].(gin.H)
		accountRow["allowed_actions"] = actions
		waiver := field{Name: "waiver_evidence", Label: "本人明确放弃剩余购买与赠送ATG的确认（可选）", Kind: "textarea", Help: "留空将保留全部ATG，余额未归零前不能完成注销。填写代表明确放弃剩余ATG；结算币种与佣金仍须真实结算，不会被删除。"}
		p["row_operations"] = []operation{{Name: "stop", Label: "停止使用服务", Method: "POST", Path: "/api/v1/me/account/lifecycle/stop", IncludeVersion: true, Fields: []field{f("evidence", "本人停止使用说明", "textarea")}, Confirmation: "停用Key及未来续费，保留全部资产、推广资格和历史佣金。恢复使用后需本人明确重新启用Key与续费。"}, {Name: "resume", Label: "恢复使用", Method: "POST", Path: "/api/v1/me/account/lifecycle/resume", IncludeVersion: true, Fields: []field{f("evidence", "本人恢复使用说明", "textarea")}}, {Name: "begin_close", Label: "开始注销清算", Method: "POST", Path: "/api/v1/me/account/lifecycle/begin-close", IncludeVersion: true, Fields: []field{f("evidence", "本人注销申请说明", "textarea"), waiver}, Confirmation: "开始注销会停止调用、续费与未来分佣，保留历史权利和资金清算。只有全部未决请求、退款、提现、争议与余额真实结清后才能完成。"}, {Name: "complete_close", Label: "核实清算并完成注销", Method: "POST", Path: "/api/v1/me/account/lifecycle/complete-close", IncludeVersion: true, Fields: []field{f("evidence", "本人确认清算完成", "textarea")}, Confirmation: "完成后撤销登录凭据。服务器逐项校验清算，不能通过点击抹去未结资产。"}}

		rowOps := p["row_operations"].([]operation)
		rowOps = append(rowOps, operation{Name: "waive", Label: "本人明确放弃剩余ATG", Method: "POST", Path: "/api/v1/me/account/waive-credits", IncludeVersion: true, Fields: []field{f("waiver_evidence", "本人明确放弃购买与赠送ATG的确认", "textarea")}, Confirmation: "仅注销清算中可执行。将真实撤销全部未使用购买与赠送ATG，不触碰结算币种或佣金。该放弃将保留不可变证据。"})
		p["row_operations"] = rowOps
		var closureChecks []organization.Requirement
		if e := a.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var err error
			closureChecks, err = a.CheckClose(ctx, tx, organization.CustomerLifecycleInput{CustomerID: customer, OperatorID: op, ActorID: customer})
			return err
		}); e != nil {
			handleError(c, e)
			return
		}
		summary := []gin.H{}
		for _, check := range closureChecks {
			state := "待处理"
			if check.Ready {
				state = "已满足"
			}
			summary = append(summary, gin.H{"label": check.Description, "value": state})
		}
		p["summary"] = summary

	default:
		if a.customerOperationsWorkspace(c, resource, p) {
			return
		}
		fail(c, 409, "capability_not_ready", "此业务正在等待必要领域配置或接线，尚不可办理", false)
		return
	}
	a.resourceResponse(c, p)
}
func (a *Application) operatorRoutes(r *gin.RouterGroup) {
	r.GET("/customer-invitation", a.channelCustomerInvitation)
	r.GET("/customer-context/:id", a.customerContext)
	r.GET("/order-context/:id", a.orderContext)
	r.POST("/order-context/:id/allocate", a.allocateOrderReceipt)
	r.GET("/refunds/:id/destination", a.staffRefundDestination)
	r.GET("/reports/financial", a.financialReport)
	r.GET("/notifications/unread-count", a.staffUnreadNotifications)
	r.GET("/notifications/notices", a.staffNotificationNotices)
	r.GET("/references/:reference", a.references)
	r.GET("/:resource/:id/files", a.invoiceFiles)
	r.GET("/:resource/:id/files/:fileID", a.downloadInvoiceFile)
	r.GET("/evidence/:id/content", a.downloadEvidence)
	r.GET("/contracts/:id/content", a.downloadContract)
	r.POST("/invoices/:id/files", a.uploadInvoiceFile)
	r.POST("/:resource/:id/replacement", a.replaceInvoice)
	r.POST("/branding/logo", a.uploadLogo)
	r.GET("/:resource", a.operatorResource)
	r.POST("/:resource", a.operatorCommand)
	r.POST("/:resource/:id/:action", a.operatorRowCommand)
	r.GET("/exports/:id/download", a.downloadExport)
}
func permissionFor(resource string) string {
	switch resource {
	case "overview", "reports", "audit":
		return "reports.view"
	case "customers":
		return "customers.read"
	case "plans", "orders", "pricing":
		return "sales.manage"
	case "payments", "payment-methods", "payment-quotes", "valuations", "receipt-valuations", "evidence":
		return "payments.manage"
	case "marketing", "commissions", "gifts":
		return "marketing.manage"
	case "assets", "ledger":
		return "ledger.read"
	case "operators":
		return "organization.read"
	case "procurement", "coverage", "exits", "procurement-invoices", "contracts":
		return "cooperation.manage"
	case "branding", "channel-themes", "domains":
		return "organization.read"
	case "staff":
		return "staff.read"
	case "business-status":
		return "business.manage"
	case "catalog", "providers", "routes", "bifrost":
		return "catalog.manage"
	case "payment-assets":
		return "providers.manage"
	case "mail":
		return "branding.manage"
	case "requests", "jobs", "supplier-costs":
		return "platform.manage"
	case "notifications", "alerts":
		return "operations.manage"
	case "exports":
		return "reports.export"
	case "invoices":
		return "invoices.manage"
	case "reconciliation":
		return "reports.view"
	case "restrictions":
		return "security.manage"
	case "withdrawals":
		return "withdrawals.manage"
	case "refunds", "disputes", "takeovers", "approvals":
		return "refunds.manage"
	}
	return ""
}
func (a *Application) staffScope(c *gin.Context, resource string) (organization.Operator, string, bool) {
	actor := subject(c).Staff.ID
	op, e := a.Organization.GetOperator(c.Request.Context(), nil, c.Param("operatorId"))
	if e != nil {
		handleError(c, e)
		return op, actor, false
	}
	permission := permissionFor(resource)
	if permission == "" {
		fail(c, 404, "not_found", "经营资源不存在", false)
		return op, actor, false
	}
	if strings.HasPrefix(permission, "platform.") || resource == "catalog" || resource == "providers" || resource == "routes" || resource == "payment-assets" || resource == "bifrost" || resource == "supplier-costs" {
		if op.ID != a.Installation.OperatorID {
			fail(c, 403, "platform_required", "此能力只由平台维护", false)
			return op, actor, false
		}

	}
	e = a.Organization.RequirePermission(c.Request.Context(), nil, actor, op.ID, permission)
	if resource == "notifications" && (c.Request.Method == "GET" || c.Request.Method == "POST" && c.Param("action") == "read") {
		_, e = a.Operations.UnreadNotificationCount(c.Request.Context(), actor, op.ID)
	}

	// Finance may inspect exact orders to allocate receipts, without receiving
	// permission to publish products or force delivery. Writes still use sales.manage.
	if e != nil && resource == "orders" && c.Request.Method == "GET" {
		e = a.Organization.RequirePermission(c.Request.Context(), nil, actor, op.ID, "payments.manage")
	}
	// Exit inventory stays with cooperation. Approving a supply failure needs
	// refund authority, and paying that refund needs payment authority. Retail
	// channel execution is likewise a payment action inside the refund workspace.
	if e != nil && resource == "exits" {
		e = a.permitAny(c, actor, op.ID, "payments.manage", "refunds.manage")
	}
	if e != nil && resource == "audit" {
		e = a.permitAny(c, actor, op.ID, "ledger.read", "branding.manage")
	}
	if e != nil && resource == "approvals" {
		e = a.permitAny(c, actor, op.ID, "withdrawals.manage")
	}
	if e != nil && resource == "refunds" {
		e = a.Organization.RequirePermission(c.Request.Context(), nil, actor, op.ID, "payments.manage")
	}
	if e != nil {
		handleError(c, e)
		return op, actor, false
	}
	return op, actor, true
}
func tagOperationPermissions(ops []operation, permission string, names ...string) []operation {
	allowed := map[string]bool{}
	for _, name := range names {
		allowed[name] = true
	}
	for i := range ops {
		if allowed[ops[i].Name] {
			ops[i].Permission = permission
		}
	}
	return ops
}
func (a *Application) permitAny(c *gin.Context, actor, operator string, permissions ...string) error {
	var last error = organization.ErrForbidden
	for _, permission := range permissions {
		last = a.Organization.RequirePermission(c.Request.Context(), nil, actor, operator, permission)
		if last == nil || !errors.Is(last, organization.ErrForbidden) {
			return last
		}
	}
	return last
}
func (a *Application) operatorResource(c *gin.Context) {
	resource := c.Param("resource")
	op, actor, ok := a.staffScope(c, resource)
	if !ok {
		return
	}
	ctx := c.Request.Context()
	p := page(resource)
	base := "/api/v1/operators/" + op.ID + "/" + resource
	items := []any{}
	switch resource {
	case "receipt-valuations":
		a.receiptValuationsResource(c, op, actor)
		return
	case "payment-assets":
		a.paymentAssetsResource(c, op, actor)
		return
	case "gifts":
		a.manualGiftsResource(c, op, actor, p, base)
		return
	case "takeovers":
		a.takeoverResource(c, op, actor)
		return
	case "evidence", "assets":
		fail(c, 404, "not_found", "页面不存在", false)
		return
	case "procurement-invoices":
		a.procurementInvoicesResource(c, op, actor, p, base)
		return
	case "restrictions":
		a.restrictionsResource(c, op, actor, p, base)
		return
	case "bifrost":
		a.modelSettingsResource(c, p, base)
		return
	case "supplier-costs":
		a.actualCostResource(c, p, base)
		return
	case "mail":
		a.mailResource(c, op, p, base)
		return
	case "exits":
		a.exitsResource(c, op, actor, p, base)
		return
	case "requests":
		a.platformRequestsResource(c, op, actor, p)
		return
	case "audit":
		a.auditResource(c, op, actor, p)
		return
	case "approvals":
		a.approvalsResource(c, op, actor, p)
		return
	case "disputes":
		a.disputesResource(c, op, actor, p, base)
		return
	case "contracts":
		a.contractResource(c, op, actor, p, base)
		return
	case "procurement":
		a.procurementResource(c, op, actor, p, base)
		return
	case "customers":
		p["search_supported"], p["status_supported"] = true, true
		rows, e := a.Organization.ListCustomersInScope(ctx, nil, actor, op.ID, 51, c.Query("cursor"), c.Query("q"), c.Query("status"))
		if e != nil {
			handleError(c, e)
			return
		}
		if len(rows) > 50 {
			rows = rows[:50]
			p["next_cursor"] = rows[49].Customer.ID
		}
		for _, row := range rows {
			items = append(items, entity(row.Customer.ID, row.Customer.Status, row.Ownership.Version, gin.H{"email": row.Email, "name": row.Customer.DisplayName, "registered_at": row.Customer.RegisteredAt, "ownership_version": row.Ownership.Version}))
		}
		p["columns"] = []gin.H{col("email", "邮箱", "text"), col("name", "名称", "text"), col("registered_at", "注册", "datetime")}
	case "plans":
		rows := []commerce.Package{}
		e := a.DB.WithContext(ctx).Table("commerce_packages").Where("operator_id=?", op.ID).Order("created_at,id").Find(&rows).Error
		if e != nil {
			handleError(c, e)
			return
		}
		for _, row := range rows {
			v := values(row)
			ver := int64(0)
			if row.CurrentQuoteID != nil {
				q, e := a.Commerce.GetQuote(ctx, nil, *row.CurrentQuoteID)
				if e != nil {
					handleError(c, e)
					return
				}
				for k, value := range values(q) {
					v[k] = value
				}
				ver = q.Version
			}
			items = append(items, entity(row.ID, "configured", ver, v))
		}
		p["columns"] = []gin.H{col("name", "名称", "text"), col("price_money", "价格", "money"), col("paid_atg", "购买", "atg"), col("gift_atg", "赠送", "atg"), col("period", "有效期", "text"), col("terms_summary", "服务约定", "text"), col("new_purchase_enabled", "新购", "boolean"), col("renewal_enabled", "续费", "boolean")}
		p["operations"] = []operation{{Name: "publish", Label: "创建套餐", Method: "POST", Path: base, Description: "填写售价、购买与赠送额度和有效期，分别控制新购与续费。保存即发布，已有订单仍按成交时条款履约。", Fields: []field{jsonField("name、package_id、price_money、paid_atg、gift_atg、period、terms_summary、new_purchase_enabled、renewal_enabled、expected_version")}}}
	case "orders":
		p["search_supported"], p["status_supported"] = true, true
		rows, e := a.salesOrders(ctx, op.ID, c.Query("cursor"), c.Query("q"), c.Query("status"), 51, false)
		if e != nil {
			handleError(c, e)
			return
		}
		if len(rows) > 50 {
			rows = rows[:50]
			p["next_cursor"] = rows[49].cursor()
		}
		for _, row := range rows {
			items = append(items, row.row())
		}
		p["columns"] = []gin.H{col("kind", "业务类型", "text"), col("counterparty_name", "客户或采购方", "text"), col("due_money", "应付", "money"), col("external_allocated_money", "外部已分配", "money"), col("paid_atg", "购买权益", "atg"), col("created_at", "创建", "datetime")}
		p["row_operations"] = []operation{{Name: "fulfill", Label: "重试交付", Permission: "sales.manage", Method: "POST", Path: base + "/{id}/fulfill", Fields: []field{}, Confirmation: "核实原订单资金、覆盖及资格后重试，不能跳过付款条件。"}}
		p["row_operations"] = append(p["row_operations"].([]operation), operation{Name: "fulfill_supply", Label: "重试供货交付", Permission: "payments.manage", Method: "POST", Path: base + "/{id}/fulfill_supply", Fields: []field{}})
	case "payment-quotes":
		a.paymentQuoteSettingsResource(c, op, p, base)
		return
	case "payment-methods":
		rows, e := a.Payments.ListMerchantConfigs(ctx, actor, op.ID)
		if e != nil {
			handleError(c, e)
			return
		}
		for _, row := range rows {
			v := values(row.Merchant)
			v["missing_fields"] = row.MissingFields
			v["fields_set"] = row.FieldsSet
			status := "pending_configuration"
			if len(row.MissingFields) == 0 && row.Merchant.Enabled {
				status = "ready"
			}
			items = append(items, entity(row.Merchant.ID, status, int64(row.Merchant.Version), v))
		}
		p["columns"] = []gin.H{col("id", "商户配置", "text"), col("driver", "协议", "text"), col("enabled", "启用", "boolean"), col("missing_fields", "待配置材料", "text"), col("fields_set", "已设置字段", "text")}
		p["operations"] = []operation{{Name: "configure", Label: "保存真实商户配置", Method: "POST", Path: base, Fields: []field{jsonField("id(UUID)、driver(stripe/alipay/wechat/nowpayments/evm_wallet/manual)、enabled、credentials（按协议必填材料）、expected_version；不回显密钥。")}, Description: "材料齐全后启用实际驱动。协议测试证据不能代替真实商户验收。"}}
	case "payments":
		p["search_supported"], p["status_supported"] = true, true
		rows, e := a.Commerce.OperatorReceipts(ctx, nil, op.ID, c.Query("cursor"), 51, c.Query("q"), c.Query("status"))
		if e != nil {
			handleError(c, e)
			return
		}
		if len(rows) > 50 {
			rows = rows[:50]
			p["next_cursor"] = rows[49].ID
		}
		for _, row := range rows {
			item := entity(row.ID, row.FeeStatus, 1, row)
			actions := []string{}
			if row.AllocatedMoney.IsZero() {
				actions = append(actions, "allocate")
			}
			if row.FeeStatus == "pending" || row.SettledAt == nil {
				actions = append(actions, "accounting")
			}
			item["allowed_actions"] = actions
			items = append(items, item)
		}
		p["columns"] = []gin.H{col("id", "收款", "text"), col("provider", "通道", "text"), col("gross_money", "收款总额（扣手续费前）", "money"), col("fee_money", "己方收款手续费", "money"), col("net_money", "净到账金额", "money"), col("allocated_money", "已分配", "money"), col("fee_status", "收款手续费核实状态", "status"), col("paid_at", "实际付款时间", "datetime")}
		p["operations"] = []operation{{Name: "receipt", Label: "登记人工核实收款", Method: "POST", Path: base, Fields: []field{jsonField("provider、provider_receipt_id、gross_money、currency、original_amount、recipient_reference、payer_reference、verification_evidence、paid_at 等原始事实。")}}}
		p["row_operations"] = []operation{{Name: "allocate", Label: "关联原订单", Method: "POST", Path: base + "/{id}/allocate", Fields: []field{f("order_id", "原订单", "text")}, Description: "整笔收款关联一个金额一致的订单，足额后自动交付。"}, {Name: "accounting", Label: "核实手续费与结算", Method: "POST", Path: base + "/{id}/accounting", Fields: []field{receiptFeeField(), optional("settled_at", "通道结算到账时间（可选）", "datetime"), f("evidence", "费用与结算核实依据", "textarea")}}}
	case "valuations":
		fail(c, 404, "not_found", "汇率配置已移除", false)
		return
	case "ledger":
		p["views"] = []gin.H{{"value": "accounts", "label": "资产账户"}, {"value": "journals", "label": "记账流水"}}
		if c.Query("view") == "journals" {
			a.ledgerJournalsResource(c, op, p)
			return
		}
		if c.Query("view") != "" && c.Query("view") != "accounts" {
			handleError(c, organization.ErrInvalidInput)
			return
		}
		p["search_supported"], p["status_supported"] = true, true
		rows, e := a.Ledger.ListOperatorAccounts(ctx, nil, op.ID, "", c.Query("cursor"), c.Query("q"), 51, c.Query("status"))
		if e != nil {
			handleError(c, e)
			return
		}
		if len(rows) > 50 {
			rows = rows[:50]
			p["next_cursor"] = rows[49].ID
		}
		for _, row := range rows {
			items = append(items, entity(row.ID, "open", row.Version, row))
		}
		p["columns"] = []gin.H{col("owner_id", "所属主体", "text"), col("purpose", "账户目的", "text"), col("unit", "单位", "text"), col("balance", "余额", "decimal"), col("held", "占用", "decimal")}
	case "operators":
		rows, e := a.Organization.Children(ctx, nil, op.ID)
		if e != nil {
			handleError(c, e)
			return
		}
		childIDs := make([]string, 0, len(rows))
		for _, row := range rows {
			childIDs = append(childIDs, row.ID)
		}
		owned := map[string]bool{}
		if len(childIDs) > 0 {
			var existing []struct{ OperatorID string }
			if e := a.DB.WithContext(ctx).Table("organization_staff_memberships").Select("DISTINCT operator_id").Where("operator_id IN ? AND role='owner'", childIDs).Scan(&existing).Error; e != nil {
				handleError(c, e)
				return
			}
			for _, membership := range existing {
				owned[membership.OperatorID] = true
			}
		}
		for _, row := range rows {
			item := entity(row.ID, row.Lifecycle, 1, row)
			item["allowed_actions"] = []string{}
			if !owned[row.ID] {
				item["allowed_actions"] = []string{"invite_owner"}
			}
			items = append(items, item)
		}
		p["columns"] = []gin.H{col("name", "经营方", "text"), col("kind", "类型", "text"), col("legal_name", "经营主体", "text"), col("brand_id", "品牌", "text")}
		p["operations"] = []operation{{Name: "create", Label: "建立 OEM / 渠道", Method: "POST", Path: base, Fields: []field{jsonField("合作类型、经营方名称、实际经营主体；OEM 另填品牌名称。可选合作约定、退出规则和合同。供货关系随创建生效，营业项目仍须单独开放。")}}}
		if op.Kind == organization.OEM {
			create := p["operations"].([]operation)[0]
			create.Label = "建立直属渠道"
			fields := businessFields(create)
			create.Fields = []field{}
			for _, input := range fields {
				if input.Name == "brand_name" || input.Name == "api_base_url" {
					continue
				}
				if input.Name == "kind" {
					input = defaultField("kind", "合作类型", "hidden", "channel")
				}
				create.Fields = append(create.Fields, input)
			}
			p["operations"] = []operation{create}
		} else if op.Kind == organization.Channel {
			p["operations"] = []operation{}
		}
		p["row_operations"] = []operation{{Name: "invite_owner", Label: "开通首位负责人", Method: "POST", Path: base + "/{id}/invite_owner", Fields: ownerAccessFields(true), Description: "为尚无有效负责人的直属经营方开通首位负责人。可手动交付登录信息或发送邮件邀请；已有账号须本人接受邀请。"}}
	case "coverage":
		a.coverageResource(c, op, base, p)
		return
	case "marketing":
		a.marketingResource(c, op, actor, base, p)
		return
	case "commissions":
		p["search_supported"], p["status_supported"] = false, false
		policy, e := a.Incentives.CurrentPolicy(ctx, nil, op.ID)
		if e != nil && !errors.Is(e, incentives.ErrNotReady) {
			handleError(c, e)
			return
		}
		withdrawal, withdrawalErr := a.Incentives.CurrentWithdrawalPolicy(ctx, nil, op.ID)
		if withdrawalErr != nil && !errors.Is(withdrawalErr, incentives.ErrNotReady) {
			handleError(c, withdrawalErr)
			return
		}
		if e == nil {
			shown := values(policy)
			delete(shown, "direct_rate")
			delete(shown, "indirect_rate")
			shown["direct_percent"] = ratePercentage(policy.DirectRate)
			shown["indirect_percent"] = ratePercentage(policy.IndirectRate)
			shown["confirmation_days"] = policyDays(policy.ConfirmationSeconds)
			shown["reward_validity_days"] = policyDays(policy.RewardValiditySeconds)
			if withdrawalErr == nil {
				shown["minimum_money"] = withdrawal.MinimumMoney
			}
			items = append(items, entity(policy.ID, "effective", policy.Version, shown))
		}
		fields := businessFields(operation{Path: base, Fields: []field{jsonField("threshold_money、direct_percent、indirect_percent、confirmation_days、invitee_atg、inviter_atg、reward_validity_days、kol_cross_activity_limit、minimum_money、expected_version")}})
		if withdrawalErr == nil {
			for i := range fields {
				if fields[i].Name == "minimum_money" {
					fields[i].Default = withdrawal.MinimumMoney
				}
			}
		}
		p["columns"] = []gin.H{col("threshold_money", "资格门槛", "money"), col("direct_percent", "直接邀请佣金 %", "decimal"), col("indirect_percent", "间接邀请佣金 %", "decimal"), col("confirmation_days", "确认等待天数", "text"), col("reward_validity_days", "注册奖励有效天数", "text"), col("invitee_atg", "受邀奖励", "atg"), col("inviter_atg", "邀请奖励", "atg"), col("kol_cross_activity_limit", "同一 KOL 跨活动每人限次", "text"), col("minimum_money", "佣金最低提现 结算币种", "money")}
		p["operations"] = []operation{{Name: "policy", Label: "设置邀请与分佣规则", Method: "POST", Path: base, Fields: fields, Description: "注册赠送、资格门槛、两级佣金和佣金最低提现额度在此统一设置。比例填写百分数，例如 15 表示 15%。提现手续费由申请人承担，审批为单人。提高门槛不影响已取得的资格，已有订单保留成交规则。"}}
	case "business-status":
		row, e := a.Organization.GetBusinessStatus(ctx, nil, op.ID)
		if e != nil {
			handleError(c, e)
			return
		}
		v := values(row)
		readiness := []gin.H{}
		destinations := map[string]string{"incentives_policy": "commissions", "verification_mail": "mail", "reward_coverage": "coverage", "package": "plans", "payment": "payment-methods", "catalog_pricing": "pricing", "coverage": "coverage"}
		for _, feature := range []organization.BusinessFeature{organization.Registration, organization.NewPurchase, organization.Renewal, organization.ModelUsage, organization.Activity, organization.Settlement} {
			requirements, err := a.Check(ctx, nil, op, feature)
			if err != nil {
				handleError(c, err)
				return
			}
			checks := []gin.H{}
			for _, requirement := range requirements {
				resource := destinations[requirement.Code]
				hint := ""
				if requirement.Code == "monetary_foundation" {
					resource = "business-status"
					if op.Kind != organization.Platform {
						resource = ""
						hint = "等待平台负责人完成清算币种和 ATG 基础比例设置并生效。"
					}
				}
				if requirement.Code == "verification_mail" && op.Kind == organization.Channel {
					resource = ""
					hint = "等待所属品牌方配置可用的邮件服务。"
				}
				checks = append(checks, gin.H{"code": requirement.Code, "description": requirement.Description, "ready": requirement.Ready, "resource": resource, "action_hint": hint})
			}
			readiness = append(readiness, gin.H{"feature": feature, "requirements": checks})
		}
		v["readiness"] = readiness
		items = append(items, entity(op.ID, op.Lifecycle, row.Version, v))
		p["columns"] = []gin.H{col("registration", "注册", "boolean"), col("new_purchase", "新购", "boolean"), col("renewal", "续费", "boolean"), col("model_usage", "调用", "boolean"), col("activity", "营销", "boolean"), col("settlement", "结算", "boolean")}
		p["search_supported"], p["status_supported"] = false, false
		p["operations"] = []operation{{Name: "set", Label: "调整营业开关", Method: "POST", Path: base, Fields: []field{jsonField("feature(registration/new_purchase/renewal/model_usage/activity/settlement)、enabled、expected_version；只有通过真实依赖检查才会开放。")}}}
	case "branding", "channel-themes":
		a.brandingResource(c, op, actor, resource, base, p)
		return
	case "jobs":
		a.jobsResource(c, p, base)
		return
	case "domains":
		a.domainResource(c, op, actor, p, base)
		return
	case "staff":
		rows, e := a.Organization.ListMemberships(ctx, nil, actor, op.ID)
		if e != nil {
			handleError(c, e)
			return
		}
		for _, row := range rows {
			var person Staff
			if e := a.DB.WithContext(ctx).Where("id=?", row.StaffID).Limit(1).Find(&person).Error; e != nil {
				handleError(c, e)
				return
			}
			var grants []string
			if e := a.DB.WithContext(ctx).Table("organization_staff_grants").Where("membership_id=?", row.ID).Order("action").Pluck("action", &grants).Error; e != nil {
				handleError(c, e)
				return
			}
			v := values(row)
			v["display_name"], v["email"] = person.DisplayName, person.Email
			v["grants"] = strings.Join(grants, ",")
			items = append(items, entity(row.ID, row.Status, 1, v))
		}
		p["columns"] = []gin.H{col("display_name", "人员", "text"), col("email", "邮箱", "text"), col("role", "角色", "text"), col("status", "状态", "status")}
		grants, e := a.staffGrantField(c, actor, op)
		if e != nil {
			handleError(c, e)
			return
		}
		p["row_operations"] = []operation{{Name: "membership", Label: "调整当前成员角色", Method: "POST", Path: base + "/{id}/membership", Fields: []field{selectField("role", "角色", "owner|负责人", "operations|运营", "finance|财务", "support|客服", "readonly|只读", "custom|明确自定义"), selectField("status", "成员状态", "active|启用", "suspended|停用"), grants}, Confirmation: "仅调整当前经营方成员；最后一位负责人不能被移除。"}}
		p["operations"] = []operation{{Name: "invite", Label: "开通或邀请经营人员", Method: "POST", Path: base, Fields: ownerAccessFields(false), Description: "为新账号设置初始密码并手动交付，或选择邮件邀请。已有账号保留原密码，由本人接受邀请加入当前经营方。"}}
	case "refunds":
		a.refundStaffResource(c, op, actor, p, base)
		return
	case "withdrawals":
		a.withdrawalStaffResource(c, op, actor, p, base)
		return
	case "catalog", "providers", "routes", "pricing":
		if a.catalogResource(c, op, actor, resource, p, base) {
			return
		}
	default:
		if a.operatorOperationsResource(c, op, actor, resource, p, base) {
			return
		}
		fail(c, 409, "capability_not_ready", "此业务尚未完成必要配置或应用接线", false)
		return
	}
	p["items"] = items
	a.resourceResponse(c, p)
}

func ownerAccessFields(owner bool) []field {
	mode := selectField("delivery_mode", "开通方式", "manual|手动开通并交付", "email|发送邮件邀请")
	mode.Default = "manual"
	password := optional("initial_password", "新账号初始密码", "password")
	password.VisibleWhen = &fieldVisibility{Field: "delivery_mode", Values: []string{"manual"}}
	role := selectField("role", "角色", "owner|负责人", "operations|运营", "finance|财务", "support|客服", "readonly|只读")
	if owner {
		role = defaultField("role", "角色", "hidden", "owner")
	}
	return []field{mode, f("email", "登录邮箱", "email"), optional("display_name", "人员姓名", "text"), password, role}
}
