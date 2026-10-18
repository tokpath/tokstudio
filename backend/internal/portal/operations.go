package portal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/operations"
	"github.com/tokpath/tokstudio/backend/internal/organization"
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
		return operations.Recipient{ID: customer.ID, OperatorID: operator, Email: email, Brand: b.Name, EmailVerified: e == nil}, e
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
			return operations.Recipient{ID: staff.ID, OperatorID: operator, Email: staff.Email, Brand: b.Name, EmailVerified: staff.EmailVerified}, nil
		}
	}
	return operations.Recipient{}, operations.ErrForbidden
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
		p["row_operations"] = []operation{{Name: "replacement", Label: "作废后按原来源申请重开", Method: "POST", Path: "/api/v1/me/invoices/{id}/replacement", Fields: businessFields(operation{Path: "/api/v1/me/invoices", Fields: []field{jsonField("")}})}}
		p["columns"] = []gin.H{col("operator_id", "原始开票经营方", "text"), col("amount_usd", "可开票责任", "usd"), col("state", "状态", "status"), col("invoice_reference", "开票凭证", "text"), col("reason", "说明", "text"), col("created_at", "申请时间", "datetime")}
		p["operations"] = []operation{{Name: "invoice", Label: "按原始收款申请发票", Method: "POST", Path: "/api/v1/me/invoices", Fields: []field{jsonField("lines:[{receipt_id,amount_usd}]、details:{title,tax_id,email,address}。同一外部收款跨充值与购包只可开票一次。")}}}
		policy, e := a.Operations.GetInvoicePolicy(ctx, customer, op, true)
		if e != nil {
			handleError(c, e)
			return true
		}
		p["summary"] = []gin.H{{"label": "可申请阶段", "value": policy.Stage}, {"label": "最高原收款比例", "value": policy.MaximumRatio.String()}, {"label": "办理规则", "value": policy.RulesReference}}
		if !policy.Enabled {
			p["availability"] = []gin.H{{"capability": "invoices", "enabled": false, "reason": "当前经营方尚未开放新的开票申请。历史原始收款仍按原开票方规则核实办理。"}}
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
		rows, e := a.Operations.ListDeliveries(ctx, actor, op.ID, 100)
		if e != nil {
			handleError(c, e)
			return true
		}
		for _, row := range rows {
			items = append(items, entity(row.NotificationID, row.State, 1, row))
		}
		p["columns"] = []gin.H{col("notification_id", "通知", "text"), col("state", "邮件状态", "status"), col("attempts", "尝试次数", "text"), col("error_code", "原因", "text"), col("next_attempt_at", "下次尝试", "datetime")}
		p["operations"] = []operation{{Name: "announce", Label: "发送业务通知", Method: "POST", Path: base, Fields: []field{jsonField("recipient_id、category(mandatory/usage/marketing)、severity(info/warning/critical)、title、body、reference；营销邮件须先取得客户授权。")}}}
		p["row_operations"] = []operation{{Name: "retry", Label: "重试原通知", Method: "POST", Path: base + "/{id}/retry", Fields: []field{}}}
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
		p["operations"] = []operation{{Name: "reconcile", Label: "按领域事实运行对账", Method: "POST", Path: base, Fields: []field{}}}
		p["row_operations"] = []operation{{Name: "resolve", Label: "登记修复依据", Method: "POST", Path: base + "/{id}/resolve", Fields: []field{f("adjustment", "真实冲正/修复记录", "text"), f("reason", "原因", "textarea")}, Description: "只有可核实的领域修复记录才能解决差额；此操作不改余额。"}}
	case "exports":
		rows, e := a.Operations.ListExports(ctx, actor, op.ID)
		if e != nil {
			handleError(c, e)
			return true
		}
		for _, row := range rows {
			v := values(row)
			if row.State == "ready" {
				v["download_url"] = "/api/v1/operators/" + op.ID + "/exports/" + row.ID + "/download"
			}
			items = append(items, entity(row.ID, row.State, 1, v))
		}
		p["columns"] = []gin.H{col("kind", "类型", "text"), col("state", "状态", "status"), col("row_count", "记录数", "text"), col("expires_at", "失效", "datetime"), col("download_url", "私有下载地址", "link")}
		p["operations"] = []operation{{Name: "export", Label: "创建私有导出", Method: "POST", Path: base, Fields: []field{jsonField("kind(ledger/requests/procurement)、owner_id(可选)、from、to；选择有限时间范围。")}}}
	case "risk", "alerts":
		a.riskWorkspace(c, op, actor, resource, base, p)
		return true
	case "invoices":
		rows, next, e := a.Operations.ListOperatorInvoicesPage(ctx, actor, op.ID, c.Query("cursor"), c.Query("q"), c.Query("status"), 50)
		p["next_cursor"] = next
		p["search_supported"], p["status_supported"] = true, true
		if e != nil {
			handleError(c, e)
			return true
		}
		for _, row := range rows {
			items = append(items, entity(row.ID, row.State, 1, row))
		}
		p["columns"] = []gin.H{col("customer_id", "客户", "text"), col("amount_usd", "金额", "usd"), col("state", "状态", "status"), col("invoice_reference", "开票凭证", "text"), col("reason", "原因", "text")}
		policy, e := a.Operations.GetInvoicePolicy(ctx, actor, op.ID, false)
		if e != nil {
			handleError(c, e)
			return true
		}
		p["summary"] = []gin.H{{"label": "当前规则版本", "value": fmt.Sprint(policy.Version)}, {"label": "开票阶段", "value": policy.Stage}, {"label": "最高原收款比例", "value": policy.MaximumRatio.String()}, {"label": "规则依据", "value": policy.RulesReference}}
		invoiceOps := []operation{}
		for _, party := range []string{"customer", "procurement"} {
			current, e := a.Operations.GetIssuerInvoicePolicy(ctx, actor, op.ID, party)
			if e != nil {
				handleError(c, e)
				return true
			}
			label := "零售客户"
			if party == "procurement" {
				label = "经营方采购"
			}
			invoiceOps = append(invoiceOps, operation{Name: "invoice_policy_" + party, Label: "设置" + label + "开票规则", Method: "POST", Path: base, Fields: []field{defaultField("party_kind", "办理对象", "hidden", party), defaultField("enabled", "开放申请", "boolean", current.Enabled), selectField("stage", "允许开票阶段", "receipt|原始收款", "top_up|充值完成", "purchase|完成购买", "consumption|实际消费"), f("maximum_ratio", "最高原收款比例（0到1）", "decimal"), f("rules_reference", "公开办理规则依据", "text"), defaultField("expected_version", "版本", "hidden", current.Version)}})
		}
		p["operations"] = invoiceOps
		p["row_operations"] = []operation{{Name: "register", Label: "登记实际开票结果", Method: "POST", Path: base + "/{id}/register", Fields: []field{selectField("state", "实际办理状态", "issued|外部开具已完成", "rejected|不予办理并说明原因", "voided|原票据已依法作废"), f("invoice_reference", "发票凭证编号", "text"), f("evidence", "证据引用", "text"), f("reason", "说明", "textarea")}}}
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
			PartyKind      string         `json:"party_kind"`
			Enabled        bool           `json:"enabled"`
			Stage          string         `json:"stage"`
			MaximumRatio   ledger.Decimal `json:"maximum_ratio"`
			RulesReference string         `json:"rules_reference"`
			Expected       int64          `json:"expected_version"`
		}
		if !body(c, &in) {
			return true
		}
		out, e := a.Operations.SaveInvoicePolicy(ctx, actor, operations.InvoicePolicy{OperatorID: op.ID, PartyKind: in.PartyKind, Enabled: in.Enabled, Stage: in.Stage, MaximumRatio: in.MaximumRatio, RulesReference: in.RulesReference}, in.Expected)
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
		c.JSON(200, commandResult(out.ID, "completed"))
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
		if json.Unmarshal(raw, &in) != nil {
			handleError(c, operations.ErrInvalid)
			return true
		}
		out, e := a.Operations.RequestExport(ctx, actor, op.ID, in)
		if e != nil {
			handleError(c, e)
			return true
		}
		c.JSON(200, commandResult(out.ID, "queued"))
		return true
	case "risk", "alerts":
		raw, ok := decodeConfiguration(c)
		if !ok {
			return true
		}
		var in struct {
			operations.RiskRule
			ExpectedVersion int64 `json:"expected_version"`
		}
		if json.Unmarshal(raw, &in) != nil {
			handleError(c, operations.ErrInvalid)
			return true
		}
		in.OperatorID = op.ID
		result, e := a.command(c, "risk-rule:"+op.ID, in, func(tx *gorm.DB) (any, error) {
			out, e := a.Operations.SaveRiskRuleTx(ctx, tx, actor, in.RiskRule, in.ExpectedVersion)
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
	c.Header("Content-Disposition", "attachment; filename=export.jsonl")
	c.Data(200, "application/x-ndjson", raw)
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

var _ = errors.Is
