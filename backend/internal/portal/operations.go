package portal

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/operations"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"github.com/tokpath/tokstudio/backend/internal/platform/httpx"
	"gorm.io/gorm"
)

type recipientDirectory struct{ Application *Application }

func (d recipientDirectory) ResolveRecipient(ctx context.Context, tx *gorm.DB, operator, actor string) (operations.Recipient, error) {
	a := d.Application
	op, e := a.Organization.GetOperator(ctx, tx, operator)
	if e != nil {
		return operations.Recipient{}, e
	}
	b, e := a.Organization.GetBrand(ctx, tx, op.BrandID)
	if e != nil {
		return operations.Recipient{}, e
	}
	customer, e := a.Organization.GetCustomer(ctx, tx, actor)
	if e == nil {
		own, e := a.Organization.CurrentOwnership(ctx, tx, actor)
		if e != nil || own.OperatorID != operator {
			return operations.Recipient{}, operations.ErrForbidden
		}
		email, e := a.Organization.CustomerEmail(ctx, tx, actor)
		return operations.Recipient{ID: customer.ID, OperatorID: operator, Email: email, Brand: b.Name, BrandID: op.BrandID, EmailVerified: e == nil}, e
	}
	var staff Staff
	gdb := a.DB.WithContext(ctx)
	if tx != nil {
		gdb = tx.WithContext(ctx)
	}
	if e = gdb.First(&staff, "id=? AND status='active'", actor).Error; e != nil {
		return operations.Recipient{}, e
	}
	workspaces, e := a.Organization.MembershipsForStaff(ctx, tx, actor)
	if e != nil {
		return operations.Recipient{}, e
	}
	for _, ws := range workspaces {
		if ws.Operator.ID == operator {
			return operations.Recipient{ID: staff.ID, OperatorID: operator, Email: staff.Email, Brand: b.Name, BrandID: op.BrandID, EmailVerified: staff.EmailVerified}, nil
		}
	}
	return operations.Recipient{}, operations.ErrForbidden
}
func (a *Application) unreadNotifications(c *gin.Context) {
	id := subject(c)
	count, e := a.Operations.UnreadNotificationCount(c.Request.Context(), id.Customer.Customer.ID, id.Customer.Ownership.OperatorID)
	if e != nil {
		handleError(c, e)
		return
	}
	c.JSON(200, gin.H{"count": count})
}
func (a *Application) staffUnreadNotifications(c *gin.Context) {
	count, e := a.Operations.UnreadNotificationCount(c.Request.Context(), subject(c).Staff.ID, c.Param("operatorId"))
	if e != nil {
		handleError(c, e)
		return
	}
	c.JSON(200, gin.H{"count": count})
}
func (a *Application) readAllStaffNotifications(c *gin.Context) {
	var empty struct{}
	if !body(c, &empty) {
		return
	}
	// Membership is enough. This does not require operations.manage, because it
	// only writes the signed-in person's own read timestamps.
	count, e := a.Operations.ReadAllNotifications(c.Request.Context(), subject(c).Staff.ID, c.Param("operatorId"))
	if e != nil {
		handleError(c, e)
		return
	}
	c.JSON(200, gin.H{"id": c.Param("operatorId"), "status": "updated", "count": count})
}
func (a *Application) notifications(c *gin.Context) {
	id := subject(c)
	rows, next, e := a.Operations.ListNotificationsPage(c.Request.Context(), id.Customer.Customer.ID, id.Customer.Ownership.OperatorID, c.Query("cursor"), c.Query("q"), 50)
	if e != nil {
		handleError(c, e)
		return
	}
	items := []any{}
	for _, n := range rows {
		items = append(items, gin.H{"id": n.ID, "title": n.Title, "body": n.Body, "category": n.Category, "necessary": n.Category == "mandatory", "system_generated": n.SystemGenerated(), "read_at": n.ReadAt, "created_at": n.CreatedAt})
	}
	c.JSON(200, gin.H{"items": items, "next_cursor": next})
}
func (a *Application) notificationPreferences(c *gin.Context) {
	id := subject(c)
	p, e := a.Operations.Preferences(c.Request.Context(), id.Customer.Customer.ID, id.Customer.Ownership.OperatorID)
	if e != nil {
		handleError(c, e)
		return
	}
	c.JSON(200, p)
}
func (a *Application) readNotification(c *gin.Context) {
	id := subject(c)
	e := a.Operations.ReadNotification(c.Request.Context(), id.Customer.Customer.ID, id.Customer.Ownership.OperatorID, c.Param("id"))
	if e != nil {
		handleError(c, e)
		return
	}
	c.JSON(200, commandResult(c.Param("id"), "read"))
}
func (a *Application) readAllNotifications(c *gin.Context) {
	var empty struct{}
	if !body(c, &empty) {
		return
	}
	id := subject(c)
	count, e := a.Operations.ReadAllNotifications(c.Request.Context(), id.Customer.Customer.ID, id.Customer.Ownership.OperatorID)
	if e != nil {
		handleError(c, e)
		return
	}
	c.JSON(200, gin.H{"id": id.Customer.Customer.ID, "status": "updated", "count": count})
}
func (a *Application) customerOperationsWorkspace(c *gin.Context, resource string, p gin.H) bool {
	id := subject(c)
	ctx := c.Request.Context()
	op, customer := id.Customer.Ownership.OperatorID, id.Customer.Customer.ID
	switch resource {
	case "invoices":
		p["search_supported"], p["status_supported"] = true, true
		rows, next, e := a.Operations.ListCustomerInvoices(ctx, customer, c.Query("cursor"), 50, c.Query("q"), c.Query("status"))
		if e != nil {
			handleError(c, e)
			return true
		}
		items := []any{}
		for _, row := range rows {
			r := entity(row.ID, row.State, 1, row)
			r["allowed_actions"] = []string{}
			if row.State == "voided" {
				r["allowed_actions"] = []string{"replacement"}
			}
			items = append(items, r)
		}
		p["title"] = "发票与凭证"
		p["next_cursor"] = next
		p["items"] = items
		p["row_operations"] = []operation{{Name: "replacement", Label: "按原订单申请重开", Method: "POST", Path: "/api/v1/me/invoices/{id}/replacement", Fields: businessFields(operation{Path: "/api/v1/me/invoices", Fields: []field{jsonField("")}})}}
		p["columns"] = []gin.H{col("operator_id", "原始开票经营方", "text"), col("amount_money", "开票金额", "money"), col("state", "状态", "status"), col("invoice_reference", "开票凭证", "text"), col("reason", "说明", "text"), col("created_at", "申请时间", "datetime")}
		p["operations"] = []operation{{Name: "invoice", Label: "申请发票", Method: "POST", Path: "/api/v1/me/invoices", Fields: []field{jsonField("选择已完成的套餐购买或续费。余额充值不能开票。")}}}
		policy, e := a.Operations.GetInvoicePolicy(ctx, customer, op, true)
		if e != nil {
			handleError(c, e)
			return true
		}
		p["summary"] = []gin.H{{"label": "办理规则", "value": policy.RulesReference}}
		if !policy.Enabled {
			p["availability"] = []gin.H{{"capability": "invoices", "enabled": false, "reason": "当前经营方尚未开放新的开票申请。已申请的票据仍由原开票经营方办理。"}}
		}

	default:
		return false
	}
	a.resourceResponse(c, p)
	return true
}
func (a *Application) operatorOperationsResource(c *gin.Context, op organization.Operator, actor, resource string, p gin.H, base string) bool {
	ctx := c.Request.Context()
	items := []any{}
	switch resource {
	case "reports":
		a.financialResource(c, op, actor, p)
		return true
	case "overview":
		var metrics map[string]ledger.Decimal
		e := a.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var e error
			metrics, e = a.Operations.Source.Metrics(ctx, tx, op.ID, time.Now())
			return e
		})
		if e != nil {
			handleError(c, e)
			return true
		}
		for name, v := range metrics {
			items = append(items, entity(name, "calculated", 1, gin.H{"metric": name, "value": v}))
		}
		p["title"] = "经营事实报表"
		p["columns"] = []gin.H{col("metric", "事实指标", "text"), col("value", "精确值", "decimal")}
		p["description"] = "来自当前经营方真实账本、采购和请求事实；未知供应商成本单列，不作为零成本。"
	case "notifications":
		personal, events, next, e := a.Operations.ListNotificationGroups(ctx, actor, op.ID, c.Query("cursor"), c.Query("q"), c.Query("status"), 30)
		if e != nil {
			handleError(c, e)
			return true
		}
		if e = a.labelNotificationRecipients(c, op.ID, actor, &personal, events); e != nil {
			handleError(c, e)
			return true
		}
		items = append(items, entity(personal.Key, "grouped", 1, personal))
		for _, event := range events {
			items = append(items, entity(event.Key, "grouped", 1, event))
		}
		p["title"] = "通知管理"
		p["description"] = "发给你的通知单独显示未读条数。可以一次把发给你的未读全部标成已读，也可以打开详情后逐条标成已读。发给其他人的同一次通知合成一组，显示未读人数和总人数。同一事项的两次发送是两组。"
		p["search_supported"], p["status_supported"], p["next_cursor"] = true, true, next
		p["columns"] = []gin.H{col("recipient_id", "收件人", "text"), col("category", "通知类别", "text"), col("state", "邮件状态", "status"), col("read_status", "阅读状态", "status"), col("created_at", "发送时间", "datetime")}
		readAll := operation{Name: "read_all", Label: "全部标为已读", Method: "POST", Path: base + "/read-all", Fields: []field{}, Description: "只把发给你的未读通知标成已读。发给其他人的通知不会改变。", Confirmation: "确认把发给你的未读通知全部标为已读？"}
		p["operations"] = []operation{{Name: "announce", Label: "发送业务通知", Method: "POST", Path: base, Fields: []field{jsonField("收件人、通知类别、标题和正文")}}, readAll}
		p["row_operations"] = []operation{{Name: "read", Label: "标为已读", Method: "POST", Path: base + "/{id}/read", Fields: []field{}}, {Name: "retry", Label: "重试原通知", Method: "POST", Path: base + "/{id}/retry", Fields: []field{}}}
		if a.Organization.RequirePermission(ctx, nil, actor, op.ID, "operations.manage") != nil {
			p["title"] = "我的通知"
			p["description"] = "查看发给本人的业务通知。可以一次全部标为已读，也可以逐条标为已读。"
			p["operations"] = []operation{readAll}
			p["row_operations"] = []operation{{Name: "read", Label: "标为已读", Method: "POST", Path: base + "/{id}/read", Fields: []field{}}}
		}

	case "reconciliation":
		rows, e := a.Operations.ListDiscrepancies(ctx, actor, op.ID, 100)
		if e != nil {
			handleError(c, e)
			return true
		}
		for _, row := range rows {
			items = append(items, entity(row.ID, row.State, 1, row))
		}
		p["columns"] = []gin.H{col("domain", "领域", "text"), col("kind", "差额类型", "text"), col("reference", "原始事实", "text"), col("unit", "单位", "text"), col("expected", "应有", "decimal"), col("actual", "实际", "decimal")}
		p["description"] = "这里只列出对不上的差额，不列出正常的订单和收款。运行对账后如果列表仍是空的，表示没有差额，也没有异常数据。"
		p["operations"] = []operation{{Name: "reconcile", Label: "按领域事实运行对账", Method: "POST", Path: base, Fields: []field{}, Description: "核对本经营方的账户余额、占用金额、每笔记账是否借贷平衡，以及采购额度是否与批次和账户一致。没有差额时不会新增记录。"}}
		p["row_operations"] = []operation{{Name: "resolve", Label: "登记修复依据", Method: "POST", Path: base + "/{id}/resolve", Fields: []field{f("adjustment", "真实冲正/修复记录", "text"), f("reason", "原因", "textarea")}, Description: "只有可核实的领域修复记录才能解决差额；此操作不改余额。"}}
	case "exports":
		rows, e := a.Operations.ListExports(ctx, actor, op.ID)
		if e != nil {
			handleError(c, e)
			return true
		}
		for _, row := range rows {
			v := values(row)
			state := row.State
			if !time.Now().UTC().Before(row.ExpiresAt) {
				state = "expired"
			}
			if state == "ready" {
				v["download_url"] = "/api/v1/operators/" + op.ID + "/exports/" + row.ID + "/download"
			}
			items = append(items, entity(row.ID, state, 1, v))
		}
		p["title"], p["description"] = "数据导出", "按日期导出当前经营方的数据，生成后下载 CSV。"
		p["columns"] = []gin.H{col("kind", "类型", "text"), col("state", "状态", "status"), col("row_count", "记录数", "text"), col("expires_at", "失效", "datetime"), col("download_url", "私有下载地址", "link")}
		p["operations"] = []operation{{Name: "export", Label: "生成 CSV", Method: "POST", Path: base, Fields: []field{jsonField("kind、from、to")}}}
	case "alerts":
		a.riskWorkspace(c, op, actor, resource, base, p)
		return true
	case "invoices":
		matches, e := a.invoiceSearchParties(c, op.ID, false)
		if e != nil {
			handleError(c, e)
			return true
		}
		rows, next, e := a.Operations.ListOperatorInvoicesPage(ctx, actor, op.ID, c.Query("cursor"), c.Query("q"), c.Query("status"), 50, matches...)
		p["next_cursor"] = next
		p["search_supported"], p["status_supported"] = true, true
		if e != nil {
			handleError(c, e)
			return true
		}
		for _, row := range rows {
			item, err := a.invoiceEntity(c, row)
			if err != nil {
				handleError(c, err)
				return true
			}
			item["allowed_actions"] = []string{}
			if row.State == "requested" {
				item["allowed_actions"] = []string{"register", "reject"}
			}
			if row.State == "issued" || row.State == "review_required" {
				item["allowed_actions"] = []string{"void"}
			}
			items = append(items, item)
		}
		p["title"] = "我方开票"
		p["description"] = "核对申请人、购买订单和开票资料，实际开票后上传文件并登记；不予办理时说明原因。"
		p["columns"] = []gin.H{col("applicant", "申请人", "text"), col("applicant_kind", "申请类型", "text"), col("amount_money", "开票金额", "money"), col("invoice_reference", "发票凭证编号", "text"), col("reason", "说明", "text")}
		policy, e := a.Operations.GetInvoicePolicy(ctx, actor, op.ID, false)
		if e != nil {
			handleError(c, e)
			return true
		}
		p["summary"] = []gin.H{{"label": "当前规则版本", "value": fmt.Sprint(policy.Version)}, {"label": "规则依据", "value": policy.RulesReference}}
		invoiceOps := []operation{}
		for _, party := range []string{"customer", "procurement"} {
			if party == "procurement" && op.Kind == organization.Channel {
				continue
			}
			current, e := a.Operations.GetIssuerInvoicePolicy(ctx, actor, op.ID, party)
			if e != nil {
				handleError(c, e)
				return true
			}
			label := "零售客户"
			if party == "procurement" {
				label = "经营方采购"
			}

			invoiceOps = append(invoiceOps, operation{Name: "invoice_policy_" + party, Label: "设置" + label + "开票规则", Method: "POST", Path: base, Fields: []field{defaultField("party_kind", "办理对象", "hidden", party), defaultField("enabled", "开放申请", "boolean", current.Enabled), defaultField("rules_reference", "公开办理规则依据", "text", current.RulesReference), defaultField("expected_version", "版本", "hidden", current.Version)}})
		}
		p["operations"] = invoiceOps
		p["row_operations"] = []operation{
			{Name: "register", Label: "开具发票", Method: "POST", Path: base + "/{id}/register", Fields: []field{defaultField("state", "实际办理状态", "hidden", "issued"), f("invoice_reference", "发票凭证编号", "text"), optional("reason", "说明", "textarea")}},
			{Name: "reject", Label: "拒绝申请", Method: "POST", Path: base + "/{id}/register", Fields: []field{defaultField("state", "实际办理状态", "hidden", "rejected"), f("reason", "拒绝原因", "textarea")}},
			{Name: "void", Label: "登记作废", Method: "POST", Path: base + "/{id}/register", Fields: []field{defaultField("state", "实际办理状态", "hidden", "voided"), f("reason", "作废原因", "textarea")}},
		}
	default:
		return false
	}
	p["items"] = items
	a.resourceResponse(c, p)
	return true
}
func (a *Application) operatorOperationsCommand(c *gin.Context, op organization.Operator, actor, resource string) bool {
	ctx := c.Request.Context()
	switch resource {
	case "invoices":
		var in struct {
			PartyKind      string `json:"party_kind"`
			Enabled        bool   `json:"enabled"`
			RulesReference string `json:"rules_reference"`
			Expected       int64  `json:"expected_version"`
		}
		if !body(c, &in) {
			return true
		}
		if in.PartyKind == "procurement" && op.Kind == organization.Channel {
			handleError(c, operations.ErrInvalid)
			return true
		}
		out, e := a.Operations.SaveInvoicePolicy(ctx, actor, operations.InvoicePolicy{OperatorID: op.ID, PartyKind: in.PartyKind, Enabled: in.Enabled, RulesReference: in.RulesReference}, in.Expected)
		if e != nil {
			handleError(c, e)
			return true
		}
		c.JSON(200, gin.H{"id": op.ID, "status": "configured", "version": out.Version})
		return true
	case "reconciliation":
		var in struct{}
		if !body(c, &in) {
			return true
		}
		out, e := a.Operations.RunReconciliation(ctx, actor, op.ID)
		if e != nil {
			handleError(c, e)
			return true
		}
		result := commandResult(out.ID, "completed")
		result["message"] = "对账已完成，已发现差额。请回到列表核对应有金额和实际金额。没有出现在列表里的部分没有异常。"
		if out.FindingCount == 0 {
			result["message"] = "对账已完成，没有发现差额，也没有异常数据。回到列表后仍然是空的，这是正常结果。"
		}
		c.JSON(200, result)
		return true
	case "notifications":
		raw, ok := decodeConfiguration(c)
		if !ok {
			return true
		}
		var in operations.Notification
		if json.Unmarshal(raw, &in) != nil {
			handleError(c, operations.ErrInvalid)
			return true
		}
		in.OperatorID = op.ID
		in.BusinessKey, _ = key(c)
		if in.Reference == "" {
			in.Reference = "staff-notice:" + in.BusinessKey
		}
		if in.Severity == "" {
			in.Severity = "info"
		}
		out, e := a.Operations.Announce(ctx, actor, in)
		if e != nil {
			handleError(c, e)
			return true
		}
		c.JSON(200, commandResult(out.ID, "queued"))
		return true
	case "exports":
		raw, ok := decodeConfiguration(c)
		if !ok {
			return true
		}
		var in operations.ExportQuery
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&in) != nil {
			handleError(c, operations.ErrInvalid)
			return true
		}
		result, e := a.command(c, "export:"+op.ID, in, func(tx *gorm.DB) (any, error) {
			svc := *a.Operations
			svc.DB = tx
			out, e := svc.RequestExport(ctx, actor, op.ID, in)
			return commandResult(out.ID, "queued"), e
		})
		jsonResponse(c, result, e)
		return true
	case "alerts":
		raw, ok := decodeConfiguration(c)
		if !ok {
			return true
		}
		var in riskRuleInput
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&in) != nil {
			handleError(c, operations.ErrInvalid)
			return true
		}
		rule, valid := in.rule(op.ID)
		if !valid {
			handleError(c, operations.ErrInvalid)
			return true
		}
		result, e := a.command(c, "risk-rule:"+op.ID, in, func(tx *gorm.DB) (any, error) {
			out, e := a.Operations.SaveRiskRuleTx(ctx, tx, actor, rule, in.ExpectedVersion)
			return gin.H{"id": out.ID, "status": "saved", "version": out.Version}, e
		})
		jsonResponse(c, result, e)
		return true
	}
	return false
}
func (a *Application) operatorOperationsRowCommand(c *gin.Context, op organization.Operator, actor, resource, action string) bool {
	ctx := c.Request.Context()
	var e error
	switch {
	case resource == "notifications" && action == "read":
		var empty struct{}
		if !body(c, &empty) {
			return true
		}
		e = a.Operations.ReadNotification(ctx, actor, op.ID, c.Param("id"))
	case resource == "notifications" && action == "retry":
		var empty struct{}
		if !body(c, &empty) {
			return true
		}
		e = a.Operations.RetryDelivery(ctx, actor, op.ID, c.Param("id"))
	case resource == "reconciliation" && action == "resolve":
		var in struct{ Adjustment, Reason string }
		if !body(c, &in) {
			return true
		}
		e = a.Operations.ResolveDiscrepancy(ctx, actor, op.ID, c.Param("id"), in.Adjustment, in.Reason)
	case resource == "invoices" && action == "register":
		var in struct {
			State            string `json:"state"`
			Reference        string `json:"invoice_reference"`
			Evidence, Reason string
		}
		if !body(c, &in) {
			return true
		}
		// The uploaded private file is the evidence; staff do not invent another reference.
		if in.State == "issued" {
			in.Evidence = "invoice-files:" + c.Param("id")
			if in.Reason == "" {
				in.Reason = "实际开票已完成"
			}
		}
		if in.State == "voided" {
			application, err := a.Operations.ReadInvoiceApplication(ctx, actor, op.ID, c.Param("id"), false, "")
			if err != nil {
				handleError(c, err)
				return true
			}
			in.Reference, in.Evidence = application.Invoice.InvoiceReference, application.Invoice.EvidenceReference
		}
		_, e = a.Operations.RegisterInvoice(ctx, actor, op.ID, c.Param("id"), in.State, in.Reference, in.Evidence, in.Reason)
	default:
		return false
	}
	if e != nil {
		handleError(c, e)
		return true
	}
	c.JSON(200, commandResult(c.Param("id"), "updated"))
	return true
}
func (a *Application) downloadExport(c *gin.Context) {
	op, actor, ok := a.staffScope(c, "exports")
	if !ok {
		return
	}
	raw, e := a.Operations.DownloadExport(c.Request.Context(), actor, op.ID, c.Param("id"))
	if e != nil {
		handleError(c, e)
		return
	}
	raw, e = operations.LocalizeExportCSV(raw, func(source string) string { return httpx.LocalizedText(httpx.RequestLocale(c.Request), source) })
	if e != nil {
		handleError(c, e)
		return
	}
	c.Header("Content-Disposition", "attachment; filename=tokenhub-export.csv")
	c.Header("Cache-Control", "private, no-store")
	c.Data(200, "text/csv; charset=utf-8", raw)
}
func (a *Application) requestInvoice(c *gin.Context) {
	raw, ok := decodeConfiguration(c)
	if !ok {
		return
	}
	var in struct {
		IssuerID string                    `json:"issuer_id"`
		Lines    []operations.InvoiceLine  `json:"lines"`
		Details  operations.InvoiceDetails `json:"details"`
	}
	if json.Unmarshal(raw, &in) != nil {
		handleError(c, operations.ErrInvalid)
		return
	}
	id := subject(c)
	key, _ := key(c)
	if in.IssuerID == "" {
		in.IssuerID = id.Customer.Ownership.OperatorID
	}

	out, e := a.Operations.RequestHistoricalInvoice(c.Request.Context(), id.Customer.Customer.ID, in.IssuerID, key, in.Lines, in.Details)
	if e != nil {
		handleError(c, e)
		return
	}
	c.JSON(200, commandResult(out.ID, out.State))
}
func (a *Application) setNotificationPreferences(c *gin.Context) {
	var in struct {
		Marketing  bool `json:"marketing"`
		UsageEmail bool `json:"usage_email"`
	}
	if !body(c, &in) {
		return
	}
	id := subject(c)
	_, e := a.Operations.SetPreferences(c.Request.Context(), id.Customer.Customer.ID, id.Customer.Ownership.OperatorID, in.Marketing, in.UsageEmail)
	if e != nil {
		handleError(c, e)
		return
	}
	c.JSON(200, commandResult(id.Customer.Customer.ID, "preferences_updated"))
}

func (a *Application) labelNotificationRecipients(c *gin.Context, operator, actor string, personal *operations.NotificationGroup, events []operations.NotificationGroup) error {
	ids := []string{}
	seen := map[string]bool{}
	collect := func(notices []operations.NotificationNotice) {
		for _, notice := range notices {
			if notice.RecipientID != "" && !seen[notice.RecipientID] {
				seen[notice.RecipientID] = true
				ids = append(ids, notice.RecipientID)
			}
		}
	}
	collect(personal.Notices)
	for i := range events {
		collect(events[i].Notices)
	}
	if len(ids) == 0 {
		return nil
	}
	labels := map[string]string{}
	var staff []humanLabelRow
	if err := a.DB.WithContext(c.Request.Context()).Table("portal_staff s").Select("s.id,concat_ws(' · ',NULLIF(s.display_name,''),s.email) AS label").Where("s.id IN ?", ids).Where("EXISTS (SELECT 1 FROM organization_staff_memberships m WHERE m.staff_id=s.id AND m.operator_id=?)", operator).Scan(&staff).Error; err != nil {
		return err
	}
	for _, row := range staff {
		if row.Label != "" {
			labels[row.ID] = row.Label
		}
	}
	mayEmail := false
	for _, permission := range []string{"customers.read", "refunds.manage", "withdrawals.manage"} {
		if a.Organization.RequirePermission(c.Request.Context(), nil, actor, operator, permission) == nil {
			mayEmail = true
			break
		}
	}
	selectLabel := "c.display_name AS label"
	if mayEmail {
		selectLabel = "concat_ws(' · ',NULLIF(c.display_name,''),NULLIF((SELECT email_normalized FROM organization_customer_identities e WHERE e.customer_id=c.id AND e.is_current=true LIMIT 1),'')) AS label"
	}
	var customers []humanLabelRow
	if err := a.DB.WithContext(c.Request.Context()).Table("organization_customers c").Select("c.id,"+selectLabel).Where("c.id IN ?", ids).Where("EXISTS (SELECT 1 FROM organization_service_ownerships own WHERE own.customer_id=c.id AND own.operator_id=? AND own.ended_at IS NULL)", operator).Scan(&customers).Error; err != nil {
		return err
	}
	for _, row := range customers {
		if row.Label != "" && labels[row.ID] == "" {
			labels[row.ID] = row.Label
		}
	}
	apply := func(notices []operations.NotificationNotice) {
		for i := range notices {
			if label := labels[notices[i].RecipientID]; label != "" {
				notices[i].RecipientName = label
			}
		}
	}
	apply(personal.Notices)
	for i := range events {
		apply(events[i].Notices)
	}
	return nil
}

var _ = errors.Is

// Recipient details are loaded only when opened and are cursor-paginated.
func (a *Application) staffNotificationNotices(c *gin.Context) {
	op, actor, ok := a.staffScope(c, "notifications")
	if !ok {
		return
	}
	rows, next, err := a.Operations.ListNotificationNotices(c.Request.Context(), actor, op.ID, c.Query("group"), c.Query("cursor"), c.Query("q"), c.Query("status"), 30)
	if err != nil {
		handleError(c, err)
		return
	}
	group := operations.NotificationGroup{Notices: rows}
	if err = a.labelNotificationRecipients(c, op.ID, actor, &group, nil); err != nil {
		handleError(c, err)
		return
	}
	items := []any{}
	for _, notice := range group.Notices {
		items = append(items, entity(notice.ID, notice.ReadStatus, 1, notice))
	}
	c.JSON(200, gin.H{"title": "通知明细", "description": "", "items": items, "next_cursor": next, "columns": []any{}, "operations": []operation{}, "row_operations": []operation{}})
}
