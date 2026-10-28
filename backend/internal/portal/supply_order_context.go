package portal

import (
	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/cooperation"
	"gorm.io/gorm"
)

// Finance can service a supplier's own wholesale sales without permission to
// administer purchasing, quotes, or the buyer's private workspace.
func (a *Application) supplyOrderContext(c *gin.Context, tx *gorm.DB, actor, operator, id string, out *workspaceContext) error {
	ctx := c.Request.Context()
	var order cooperation.PurchaseOrder
	if err := tx.Where("id=? AND supplier_id=?", id, operator).Take(&order).Error; err != nil {
		return err
	}
	var buyer struct{ Name string }
	if err := tx.Table("organization_operators").Select("name").Where("id=?", order.BuyerID).Take(&buyer).Error; err != nil {
		return err
	}
	out.Subject = (salesOrder{ID: order.ID, Status: order.Status, supply: &order, Counterparty: buyer.Name}).row()
	out.Subject["values"].(gin.H)["paid_usd"] = order.AllocatedUSD
	switch order.Status {
	case "awaiting_payment":
		out.NextActions = append(out.NextActions, "核实供货订单的一笔足额收款，关联后自动交付采购额度。")
	case "paid":
		out.NextActions = append(out.NextActions, "采购方已足额付款，保留原收款并重试供货交付。")
	case "completed":
		out.NextActions = append(out.NextActions, "采购额度已交付，下方可核对原收款。")
	default:
		out.NextActions = append(out.NextActions, "按原订单状态办理，避免重复收款。")
	}
	if a.Organization.RequirePermission(ctx, tx, actor, operator, "payments.manage") != nil {
		return nil
	}
	out.SubjectOperations = []operation{{Name: "fulfill_supply", Label: "重试供货交付", Method: "POST", Path: "/api/v1/operators/" + operator + "/orders/{id}/fulfill_supply", Fields: []field{}, Permission: "payments.manage"}}
	if section := c.Query("section"); section != "" && section != "payments" {
		return nil
	}
	type linkedReceipt struct {
		commerce.Receipt
	}
	q := tx.Table("commerce_receipts r").Select("r.*").Where("r.operator_id=? AND EXISTS (SELECT 1 FROM cooperation_purchase_allocations a WHERE a.receipt_id=r.id AND a.order_id=?)", operator, id)
	rows, err := contextRows[linkedReceipt](q, c.Query("cursor"))
	if err != nil {
		return err
	}
	p := contextPage("关联收款", rows, []gin.H{col("provider", "通道", "text"), col("gross_usd", "收款总额（扣手续费前）", "usd"), col("fee_usd", "己方收款手续费", "usd"), col("net_usd", "净到账金额", "usd"), col("currency", "币种", "text"), col("verification_evidence", "核实证据", "text"), col("paid_at", "实际付款时间", "datetime")}, func(r linkedReceipt) gin.H { return entity(r.ID, r.FeeStatus, 1, r) })
	p["operations"] = []operation{}
	if order.Status == "awaiting_payment" && order.AllocatedUSD.IsZero() {
		fields := manualReceiptFields()
		fields[0].Kind = "hidden"
		fields[0].Default = order.ID
		p["operations"] = []operation{{Name: "receipt", Label: "登记本单线下收款", Method: "POST", Path: "/api/v1/operators/" + operator + "/payments", Fields: fields, Permission: "payments.manage", Description: "核实一笔足额到账后登记，自动关联本单并交付采购额度。"}, {Name: "allocate_existing", Label: "关联已核实收款", Method: "POST", Path: "/api/v1/operators/" + operator + "/order-context/" + id + "/allocate", Fields: []field{f("receipt_id", "选择已核实收款", "text")}, Permission: "payments.manage", Description: "选择金额与本单应付一致的整笔收款，关联后自动交付。"}}
	}
	a.appendContextSection(c, out, "payments", "关联收款", p)
	return nil
}
