package portal

import (
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/operations"
	"github.com/tokpath/tokstudio/backend/internal/organization"
)

// Resolve names only through invoices in the authorized workspace. Matching is
// passed to the domain query before pagination, never applied to a loaded page.
func (a *Application) invoiceSearchParties(c *gin.Context, operator string, buyer bool) ([]string, error) {
	term := strings.ToLower(strings.TrimSpace(c.Query("q")))
	ids := []string{}
	if term == "" {
		return ids, nil
	}
	scope := "i.operator_id=?"
	if buyer {
		scope = "i.party_kind='procurement' AND i.buyer_operator_id=?"
	}
	err := a.DB.WithContext(c.Request.Context()).Raw(`SELECT DISTINCT party.id FROM operations_invoices i
 CROSS JOIN LATERAL (
 SELECT c.id::text FROM organization_customers c WHERE c.id=i.customer_id AND
 (strpos(lower(c.display_name),?)>0 OR EXISTS(SELECT 1 FROM organization_customer_identities e WHERE e.customer_id=c.id AND e.is_current AND strpos(lower(e.email_normalized),?)>0))
 UNION SELECT o.id::text FROM organization_operators o WHERE o.id IN (i.operator_id,i.buyer_operator_id) AND (strpos(lower(o.name),?)>0 OR strpos(lower(o.legal_name),?)>0)
 ) party WHERE `+scope, term, term, term, term, operator).Scan(&ids).Error
	return ids, err
}

func (a *Application) invoiceEntity(c *gin.Context, invoice operations.Invoice) (gin.H, error) {
	var names struct{ Applicant, Issuer string }
	err := a.DB.WithContext(c.Request.Context()).Raw(`SELECT issuer.name AS issuer,
 CASE WHEN i.party_kind='procurement' THEN buyer.name ELSE concat_ws(' · ',NULLIF(customer.display_name,''),NULLIF((SELECT email_normalized FROM organization_customer_identities e WHERE e.customer_id=i.customer_id AND e.is_current LIMIT 1),'')) END AS applicant
 FROM operations_invoices i JOIN organization_operators issuer ON issuer.id=i.operator_id
 LEFT JOIN organization_operators buyer ON buyer.id=i.buyer_operator_id
 LEFT JOIN organization_customers customer ON customer.id=i.customer_id WHERE i.id=?`, invoice.ID).Scan(&names).Error
	if err != nil {
		return nil, err
	}
	v := values(invoice)
	v["applicant"], v["issuer_name"] = names.Applicant, names.Issuer
	v["applicant_kind"] = "客户购买"
	if invoice.PartyKind == "procurement" {
		v["applicant_kind"] = "经营方采购"
	}
	return entity(invoice.ID, invoice.State, 0, v), nil
}

func invoiceIssuanceActions(state string) []string {
	switch state {
	case "requested":
		return []string{"register", "reject"}
	case "issued", "review_required":
		return []string{"void"}
	default:
		return []string{}
	}
}

func invoiceIssuanceOperations(base string) []operation {
	path := base + "/{id}/register"
	return []operation{
		{
			Name: "register", Label: "开具发票", Method: "POST", Path: path,
			Fields: []field{
				defaultField("state", "实际办理状态", "hidden", "issued"),
				f("invoice_reference", "发票凭证编号", "text"), optional("reason", "说明", "textarea"),
			},
		},
		{
			Name: "reject", Label: "拒绝申请", Method: "POST", Path: path,
			Fields: []field{defaultField("state", "实际办理状态", "hidden", "rejected"), f("reason", "拒绝原因", "textarea")},
		},
		{
			Name: "void", Label: "登记作废", Method: "POST", Path: path,
			Fields: []field{defaultField("state", "实际办理状态", "hidden", "voided"), f("reason", "作废原因", "textarea")},
		},
	}
}

func (a *Application) invoiceApplicationOrders(c *gin.Context, application operations.InvoiceApplication) ([]gin.H, error) {
	orders := []gin.H{}
	for _, line := range application.Orders {
		var order struct {
			Name, Kind string
			CreatedAt  time.Time
		}
		db := a.DB.WithContext(c.Request.Context())
		if application.Invoice.PartyKind == "procurement" {
			if err := db.Raw("SELECT name,'procurement' AS kind,created_at FROM cooperation_purchase_orders WHERE id=? AND supplier_id=? AND buyer_id=?", line.OrderID, application.Invoice.OperatorID, application.Invoice.BuyerOperatorID).Scan(&order).Error; err != nil {
				return nil, err
			}
		} else {
			if err := db.Raw(`SELECT COALESCE(p.name,'') AS name,o.kind,o.created_at FROM commerce_orders o LEFT JOIN commerce_package_quotes q ON q.id=o.quote_id LEFT JOIN commerce_packages p ON p.id=q.package_id WHERE o.id=? AND o.original_operator_id=? AND o.customer_id=?`, line.OrderID, application.Invoice.OperatorID, application.Invoice.CustomerID).Scan(&order).Error; err != nil {
				return nil, err
			}
		}
		kind := map[string]string{"package": "套餐购买", "renewal": "套餐续费", "procurement": "采购订单"}[order.Kind]
		if kind == "" {
			kind = "购买订单"
		}
		orders = append(orders, gin.H{"order_id": line.OrderID, "name": order.Name, "kind": kind, "created_at": order.CreatedAt, "amount_money": line.AmountMoney.String()})
	}
	return orders, nil
}

// Show only human-readable instructions, including historical suppliers.
func (a *Application) procurementInvoiceInstructions(c *gin.Context, op organization.Operator, page gin.H) error {
	var suppliers []struct{ Name, Instructions string }
	err := a.DB.WithContext(c.Request.Context()).Raw(`SELECT o.name,p.instructions FROM organization_operators o JOIN operations_invoice_instructions p ON p.operator_id=o.id
 WHERE p.instructions<>'' AND (o.id=(SELECT parent_operator_id FROM organization_operators WHERE id=?) OR o.id IN (SELECT supplier_id FROM cooperation_purchase_orders WHERE buyer_id=? AND status='completed')) ORDER BY o.name`, op.ID, op.ID).Scan(&suppliers).Error
	if err != nil {
		return err
	}
	notices := []gin.H{}
	for _, supplier := range suppliers {
		notices = append(notices, gin.H{"label": supplier.Name, "value": supplier.Instructions})
	}
	page["summary"] = notices
	return nil
}
