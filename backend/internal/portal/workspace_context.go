package portal

import (
	"database/sql"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/incentives"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/operations"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"github.com/tokpath/tokstudio/backend/internal/remediation"
	"github.com/tokpath/tokstudio/backend/internal/usage"
	"gorm.io/gorm"
)

// A workspace is a read projection of exact business relationships. Its sections
// do not broaden membership or financial permissions, and never accept a client
// supplied customer/operator relationship as proof of ownership.
type workspaceSection struct {
	Resource string `json:"resource"`
	Title    string `json:"title"`
	Page     gin.H  `json:"page"`
}
type workspaceContext struct {
	SubjectOperations []operation        `json:"subject_operations,omitempty"`
	Subject           gin.H              `json:"subject"`
	Sections          []workspaceSection `json:"sections"`
	NextActions       []string           `json:"next_actions"`
}

func validContextSelection(c *gin.Context) bool {
	if _, e := uuid.Parse(c.Param("id")); e != nil {
		fail(c, 400, "invalid_id", "业务记录编号无效", false)
		return false
	}
	if cursor := c.Query("cursor"); cursor != "" {
		if _, e := uuid.Parse(cursor); e != nil {
			fail(c, 400, "invalid_cursor", "分页位置无效，请重新查询", false)
			return false
		}
	}
	return true
}
func contextRows[T any](q *gorm.DB, cursor string) ([]T, error) {
	if cursor != "" {
		q = q.Where("id < ?", cursor)
	}
	var rows []T
	err := q.Order("id DESC").Limit(51).Find(&rows).Error
	return rows, err
}
func contextPage[T any](title string, rows []T, columns []gin.H, convert func(T) gin.H) gin.H {
	p := page(title)
	items := []any{}
	for i, row := range rows {
		if i == 50 {
			break
		}
		items = append(items, convert(row))
	}
	p["items"], p["columns"] = items, columns
	p["description"] = "仅显示与当前记录直接关联、且你有权查看的业务事实。"
	if len(rows) > 50 {
		p["next_cursor"] = items[49].(gin.H)["id"]
	}
	return p
}
func (a *Application) contextSection(c *gin.Context, tx *gorm.DB, actor, op, resource string) bool {
	if requested := c.Query("section"); requested != "" && requested != resource {
		return false
	}
	if a.Organization.RequirePermission(c.Request.Context(), tx, actor, op, permissionFor(resource)) == nil {
		return true
	}
	// Match the order-list read scope without granting sales mutations.
	if resource == "orders" && a.Organization.RequirePermission(c.Request.Context(), tx, actor, op, "payments.manage") == nil {
		return true
	}
	if resource == "refunds" && a.Organization.RequirePermission(c.Request.Context(), tx, actor, op, "payments.manage") == nil {
		return true
	}
	return resource == "exits" && (a.Organization.RequirePermission(c.Request.Context(), tx, actor, op, "payments.manage") == nil || a.Organization.RequirePermission(c.Request.Context(), tx, actor, op, "refunds.manage") == nil)
}
func (a *Application) appendContextSection(c *gin.Context, out *workspaceContext, resource, title string, p gin.H) {
	a.prepareResourcePage(c, p, resource)
	out.Sections = append(out.Sections, workspaceSection{resource, title, p})
}
func orderContextRow(row commerce.Order) gin.H {
	v := values(row)
	v["paid_usd"] = row.InternalUSD.Add(row.CommissionUSD).Add(row.ExternalAllocatedUSD)
	r := entity(row.ID, row.Status, 1, v)
	r["allowed_actions"] = []string{}
	if row.Status == "paid" || row.Status == "paid_pending_delivery" || row.Status == "partially_paid" || row.Status == "pending_payment" || row.Status == "pending" {
		r["allowed_actions"] = []string{"fulfill"}
	}
	return r
}
func contextOrderOperations(op string) []operation {
	return []operation{{Name: "fulfill", Label: "核实资金并重试交付", Method: "POST", Path: "/api/v1/operators/" + op + "/orders/{id}/fulfill", Fields: []field{}, Permission: "sales.manage", Confirmation: "只重试原订单交付。系统将重新校验足额收款、活动资格与可售空间，不会再次收取客户资金。"}}
}
func (a *Application) customerContext(c *gin.Context) {
	op, actor, ok := a.staffScope(c, "customers")
	if !ok || !validContextSelection(c) {
		return
	}
	ctx, customerID := c.Request.Context(), c.Param("id")
	out := workspaceContext{Sections: []workspaceSection{}, NextActions: []string{}}
	err := a.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		own, err := a.Organization.CurrentOwnership(ctx, tx, customerID)
		if err != nil {
			return err
		}
		if own.OperatorID != op.ID {
			return organization.ErrForbidden
		}
		customer, err := a.Organization.GetCustomer(ctx, tx, customerID)
		if err != nil {
			return err
		}
		email, err := a.Organization.CustomerEmail(ctx, tx, customerID)
		if err != nil {
			return err
		}
		out.Subject = entity(customer.ID, customer.Status, own.Version, gin.H{"name": customer.DisplayName, "email": email, "registered_at": customer.RegisteredAt, "ownership_version": own.Version})
		cursor := c.Query("cursor")
		if a.contextSection(c, tx, actor, op.ID, "orders") {
			rows, err := contextRows[commerce.Order](tx.Table("commerce_orders").Where("operator_id=? AND customer_id=?", op.ID, customerID), cursor)
			if err != nil {
				return err
			}
			p := contextPage("购买与交付", rows, []gin.H{col("id", "订单", "text"), col("kind", "类型", "text"), col("due_usd", "应付", "usd"), col("paid_usd", "已付", "usd"), col("status", "交付状态", "status"), col("created_at", "创建时间", "datetime")}, orderContextRow)
			p["row_operations"] = contextOrderOperations(op.ID)
			a.appendContextSection(c, &out, "orders", "购买与交付", p)
		}
		if a.contextSection(c, tx, actor, op.ID, "assets") {
			rows, err := contextRows[ledger.Account](tx.Where("operator_id=? AND owner_id=?", op.ID, customerID), cursor)
			if err != nil {
				return err
			}
			p := contextPage("客户资产", rows, []gin.H{col("purpose", "资产用途", "text"), col("unit", "单位", "text"), col("balance", "账面余额", "decimal"), col("held", "占用", "decimal"), col("available", "可用", "decimal")}, func(r ledger.Account) gin.H {
				v := values(r)
				v["available"] = r.Available()
				return entity(r.ID, "open", r.Version, v)
			})
			a.appendContextSection(c, &out, "assets", "客户资产", p)
		}
		// Customer support can see this customer's retail service facts, never
		// provider costs, routes, payloads, secrets, or other operators' requests.
		if c.Query("section") == "" || c.Query("section") == "requests" {
			rows, err := contextRows[usage.Request](tx.Where("operator_id=? AND customer_id=?", op.ID, customerID), cursor)
			if err != nil {
				return err
			}
			p := contextPage("模型调用", rows, []gin.H{col("model_id", "公开模型", "text"), col("operation", "调用类型", "text"), col("status", "状态", "status"), col("charged_atg", "原扣费", "atg"), col("net_charged_atg", "冲正后净扣费", "atg"), col("accepted_at", "受理时间", "datetime")}, func(r usage.Request) gin.H {
				return entity(r.ID, r.Status, 1, gin.H{"model_id": r.ModelID, "operation": r.Operation, "charged_atg": r.ChargedATG, "net_charged_atg": r.ChargedATG.Sub(r.CorrectedChargedATG), "accepted_at": r.AcceptedAt, "reason": r.Reason})
			})
			a.appendContextSection(c, &out, "requests", "模型调用", p)
		}
		if a.contextSection(c, tx, actor, op.ID, "commissions") {
			rows, err := contextRows[incentives.CommissionEntry](tx.Table("incentives_commission_entries").Where("operator_id=? AND beneficiary_id=?", op.ID, customerID), cursor)
			if err != nil {
				return err
			}
			p := contextPage("推广收益", rows, []gin.H{col("order_id", "来源订单", "text"), col("hop", "邀请层级", "text"), col("amount_usd", "应计", "usd"), col("reversed_usd", "已冲正", "usd"), col("status", "状态", "status"), col("confirm_at", "确认时间", "datetime")}, func(r incentives.CommissionEntry) gin.H { return entity(r.ID, r.Status, 1, r) })
			var qualification incentives.Qualification
			if err := tx.Table("incentives_qualification_accounts").Where("operator_id=? AND customer_id=?", op.ID, customerID).Find(&qualification).Error; err != nil {
				return err
			}
			state := "尚未取得"
			if qualification.AcquiredAt != nil {
				state = "已正式取得"
			}
			p["summary"] = []gin.H{{"label": "已确认真实支出", "value": qualification.ConfirmedUSD.String(), "unit": "USD"}, {"label": "分佣资格", "value": state, "detail": "资格累计包含确认的到期购买价值；佣金仅来自实际有偿调用。"}}
			a.appendContextSection(c, &out, "commissions", "推广收益", p)
		}
		if a.contextSection(c, tx, actor, op.ID, "withdrawals") {
			rows, err := contextRows[incentives.Withdrawal](tx.Table("incentives_withdrawals").Where("operator_id=? AND customer_id=?", op.ID, customerID), cursor)
			if err != nil {
				return err
			}
			p := contextPage("佣金提现", rows, []gin.H{col("amount_usd", "总额", "usd"), col("fee_usd", "手续费", "usd"), col("net_usd", "实付", "usd"), col("recipient_reference", "收款资料", "text"), col("status", "状态", "status")}, func(r incentives.Withdrawal) gin.H {
				row := entity(r.ID, r.Status, 1, r)
				row["allowed_actions"] = withdrawalAllowedActions(r.Status)
				return row
			})
			p["row_operations"] = withdrawalRowOperations("/api/v1/operators/" + op.ID + "/withdrawals")
			a.appendContextSection(c, &out, "withdrawals", "佣金提现", p)
		}
		if a.contextSection(c, tx, actor, op.ID, "invoices") {
			rows, err := contextRows[operations.Invoice](tx.Where("operator_id=? AND customer_id=?", op.ID, customerID), cursor)
			if err != nil {
				return err
			}
			p := contextPage("客户票据", rows, []gin.H{col("amount_usd", "金额", "usd"), col("state", "状态", "status"), col("invoice_reference", "凭证编号", "text"), col("created_at", "申请时间", "datetime")}, func(r operations.Invoice) gin.H { return entity(r.ID, r.State, 1, r) })
			p["row_operations"] = []operation{{Name: "register", Label: "登记实际开票结果", Method: "POST", Path: "/api/v1/operators/" + op.ID + "/invoices/{id}/register", Fields: []field{selectField("state", "实际办理状态", "issued|已开具", "rejected|不予办理", "voided|已作废"), f("invoice_reference", "发票凭证编号", "text"), f("evidence", "证据引用", "text"), f("reason", "说明", "textarea")}}}
			a.appendContextSection(c, &out, "invoices", "客户票据", p)
		}
		if a.contextSection(c, tx, actor, op.ID, "refunds") {
			rows, err := contextRows[remediation.Case](tx.Where("operator_id=? AND customer_id=?", op.ID, customerID), cursor)
			if err != nil {
				return err
			}
			a.appendContextSection(c, &out, "refunds", "退款与异常", refundContextPage(op.ID, rows))
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		handleError(c, err)
		return
	}
	for _, section := range out.Sections {
		if err := a.projectResourceLabels(c, section.Page, section.Resource); err != nil {
			handleError(c, err)
			return
		}
	}
	if err := a.projectResourceLabels(c, gin.H{"items": []any{out.Subject}}, "orders"); err != nil {
		handleError(c, err)
		return
	}
	c.JSON(200, out)
}
func refundContextPage(op string, rows []remediation.Case) gin.H {
	return contextPage("退款与异常", rows, []gin.H{col("kind", "类型", "text"), col("amount_usd", "USD责任", "usd"), col("state", "状态", "status"), col("reason", "原因", "text"), col("manage_url", "办理", "link")}, func(r remediation.Case) gin.H {
		v := values(r)
		v["manage_url"] = "/manage/" + op + "/refunds?record=" + r.ID
		row := entity(r.ID, r.State, 1, v)
		row["allowed_actions"] = []string{}
		return row
	})
}
func (a *Application) orderContext(c *gin.Context) {
	op, actor, ok := a.staffScope(c, "orders")
	if !ok || !validContextSelection(c) {
		return
	}
	ctx, orderID := c.Request.Context(), c.Param("id")
	out := workspaceContext{Sections: []workspaceSection{}, NextActions: []string{}}
	err := a.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var order commerce.Order
		if err := tx.Table("commerce_orders").Where("id=? AND operator_id=?", orderID, op.ID).Take(&order).Error; err != nil {
			return err
		}
		out.Subject = orderContextRow(order)
		if a.Organization.RequirePermission(ctx, tx, actor, op.ID, "sales.manage") == nil {
			out.SubjectOperations = contextOrderOperations(op.ID)
		}
		switch order.Status {
		case "pending", "pending_payment", "partially_paid":
			out.NextActions = append(out.NextActions, "核实收款并分配到此订单，足额后交付。")
		case "paid", "paid_pending_delivery":
			out.NextActions = append(out.NextActions, "客户已足额付款，核实可售空间并完成原订单交付。")
		case "fulfilled", "completed":
			out.NextActions = append(out.NextActions, "权益已交付，可查看下方生效时间与额度来源。")
		default:
			out.NextActions = append(out.NextActions, "按原交易事实处理当前状态，避免再次收款。")
		}
		cursor := c.Query("cursor")
		if (c.Query("section") == "" || c.Query("section") == "payments" || c.Query("section") == "receipt-valuations") && a.Organization.RequirePermission(ctx, tx, actor, op.ID, permissionFor("payments")) == nil {
			type linkedReceipt struct {
				commerce.Receipt
				OrderAllocatedUSD ledger.Decimal `json:"order_allocated_usd"`
			}
			q := tx.Table("commerce_receipts AS r").Select("r.*, x.order_allocated_usd").Joins("JOIN (SELECT receipt_id, SUM(amount_usd) AS order_allocated_usd FROM commerce_receipt_allocations WHERE operator_id=? AND order_id=? GROUP BY receipt_id) x ON x.receipt_id=r.id", op.ID, orderID).Where("r.operator_id=?", op.ID)
			rows, err := contextRows[linkedReceipt](q, cursor)
			if err != nil {
				return err
			}
			p := contextPage("关联收款", rows, []gin.H{col("provider", "通道", "text"), col("gross_usd", "收款总额", "usd"), col("order_allocated_usd", "分配至此单", "usd"), col("original_amount", "原币实收", "text"), col("currency", "币种", "text"), col("verification_evidence", "核实证据", "text"), col("paid_at", "实际付款时间", "datetime")}, func(r linkedReceipt) gin.H {
				v := values(r)
				v["order_id"] = orderID
				return entity(r.ID, r.FeeStatus, 1, v)
			})
			p["operations"] = []operation{{Name: "allocate_existing", Label: "将已核实收款分配到此单", Method: "POST", Path: "/api/v1/operators/" + op.ID + "/order-context/" + orderID + "/allocate", Fields: []field{f("receipt_id", "选择已核实收款", "text"), f("amount_usd", "本次分配 USD", "decimal")}, Permission: "payments.manage", Confirmation: "仅分配已经核实的原收款，不新建付款或收款事实。系统核对未分配余额与当前订单责任。"}}
			a.appendContextSection(c, &out, "payments", "关联收款", p)
			facts, err := a.Payments.PendingReceiptObligations(ctx, tx, op.ID, []string{orderID})
			if err != nil {
				return err
			}
			fp := page("原币待核实")
			fi := []any{}
			for _, r := range facts {
				v := values(r)
				v["manage_url"] = "/manage/" + op.ID + "/receipt-valuations"
				fi = append(fi, entity(r.FactID, r.State, 0, v))
			}
			fp["items"] = fi
			fp["columns"] = []gin.H{col("money", "已收到原币", "details"), col("reason", "待核实原因", "text"), col("observed_at", "观察时间", "datetime"), col("manage_url", "办理", "link")}
			a.appendContextSection(c, &out, "receipt-valuations", "原币待核实", fp)
		}
		if a.contextSection(c, tx, actor, op.ID, "assets") {
			rows, err := contextRows[ledger.Grant](tx.Where("operator_id=? AND order_id=?", op.ID, orderID), cursor)
			if err != nil {
				return err
			}
			p := contextPage("交付权益", rows, []gin.H{col("total", "发放总额", "atg"), col("consumed", "已使用", "atg"), col("expired", "已到期", "atg"), col("cancelled", "已撤销", "atg"), col("effective_at", "生效时间", "datetime"), col("expires_at", "到期时间", "datetime")}, func(r ledger.Grant) gin.H { return entity(r.ID, "issued", 1, r) })
			a.appendContextSection(c, &out, "assets", "交付权益", p)
		}
		if a.contextSection(c, tx, actor, op.ID, "refunds") {
			rows, err := contextRows[remediation.Case](tx.Where("operator_id=? AND order_id=?", op.ID, orderID), cursor)
			if err != nil {
				return err
			}
			a.appendContextSection(c, &out, "refunds", "退款与异常", refundContextPage(op.ID, rows))
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		handleError(c, err)
		return
	}
	for _, section := range out.Sections {
		if err := a.projectResourceLabels(c, section.Page, section.Resource); err != nil {
			handleError(c, err)
			return
		}
	}
	if err := a.projectResourceLabels(c, gin.H{"items": []any{out.Subject}}, "orders"); err != nil {
		handleError(c, err)
		return
	}
	c.JSON(200, out)
}

// This contextual command delegates to the same receipt allocation domain
// operation as the existing payments route; it introduces no payment behavior.
func (a *Application) allocateOrderReceipt(c *gin.Context) {
	op, actor, ok := a.staffScope(c, "payments")
	if !ok || !validContextSelection(c) {
		return
	}
	var input struct {
		ReceiptID string         `json:"receipt_id"`
		AmountUSD ledger.Decimal `json:"amount_usd"`
	}
	if !body(c, &input) {
		return
	}
	businessKey, ok := key(c)
	if !ok {
		return
	}
	result, err := a.Commerce.AllocateReceipt(c.Request.Context(), nil, commerce.AllocateReceiptInput{ActorStaffID: actor, OperatorID: op.ID, BusinessKey: businessKey, ReceiptID: input.ReceiptID, OrderID: c.Param("id"), AmountUSD: input.AmountUSD})
	if err != nil {
		handleError(c, err)
		return
	}
	c.JSON(200, gin.H{"id": result.ID, "status": "allocated", "message": "原收款已分配，请查看当前订单交付状态。"})
}
