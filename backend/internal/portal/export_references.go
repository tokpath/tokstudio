package portal

import (
	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/organization"
)

// The export selector follows the exact source predicates used by the export
// itself. Names are resolved only for this operator's current customers and
// direct business counterparties, never for a descendant's customer directory.
func (a *Application) exportSubjectChoices(c *gin.Context, operator, actor, query string) ([]referenceChoice, string, error) {
	kind := c.Query("kind")
	if kind == "" {
		return []referenceChoice{}, "", nil
	}
	source := ""
	args := []any{}
	switch kind {
	case "ledger":
		source = "SELECT DISTINCT owner_id AS id FROM ledger_accounts WHERE operator_id=?"
		args = []any{operator}
	case "requests":
		source = "SELECT DISTINCT customer_id AS id FROM usage_requests WHERE operator_id=?"
		args = []any{operator}
	case "procurement":
		source = "SELECT DISTINCT CASE WHEN supplier_id=? THEN buyer_id ELSE supplier_id END AS id FROM cooperation_purchase_orders WHERE supplier_id=? OR buyer_id=?"
		args = []any{operator, operator, operator}
	default:
		return nil, "", organization.ErrInvalidInput
	}
	mayEmail := false
	for _, permission := range []string{"customers.read", "refunds.manage", "withdrawals.manage"} {
		if a.Organization.RequirePermission(c.Request.Context(), nil, actor, operator, permission) == nil {
			mayEmail = true
			break
		}
	}
	customerLabel := "c.display_name"
	emailJoin := ""
	if mayEmail {
		customerLabel = "concat_ws(' · ',NULLIF(c.display_name,''),NULLIF(e.email_normalized,''))"
		emailJoin = "LEFT JOIN organization_customer_identities e ON e.customer_id=c.id AND e.is_current=true"
	}
	sql := `SELECT id,label FROM (
 SELECT owners.id::text AS id,CASE WHEN c.id IS NOT NULL THEN ` + customerLabel + ` ELSE concat_ws(' · ',o.name,NULLIF(o.legal_name,o.name)) END AS label
 FROM (` + source + `) owners
 LEFT JOIN organization_customers c ON c.id=owners.id AND EXISTS(SELECT 1 FROM organization_service_ownerships own WHERE own.customer_id=c.id AND own.operator_id=? AND own.ended_at IS NULL)
 ` + emailJoin + `
 LEFT JOIN organization_operators o ON o.id=owners.id AND (o.id=? OR o.parent_operator_id=? OR o.id=(SELECT parent_operator_id FROM organization_operators WHERE id=?))
 WHERE c.id IS NOT NULL OR o.id IS NOT NULL
 ) scoped WHERE id>? AND (label ILIKE ? OR id ILIKE ?) ORDER BY id LIMIT 51`
	args = append(args, operator, operator, operator, operator, c.Query("cursor"), "%"+query+"%", "%"+query+"%")
	var found []humanLabelRow
	if err := a.DB.WithContext(c.Request.Context()).Raw(sql, args...).Scan(&found).Error; err != nil {
		return nil, "", err
	}
	next := ""
	if len(found) > 50 {
		found = found[:50]
		next = found[49].ID
	}
	items := make([]referenceChoice, 0, len(found))
	for _, r := range found {
		items = append(items, referenceChoice{Value: r.ID, Label: r.Label})
	}
	return items, next, nil
}
