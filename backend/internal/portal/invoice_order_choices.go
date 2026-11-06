package portal

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/operations"
	"strings"
	"time"
)

// The command independently rechecks ownership, state, and the fixed amount.
func (a *Application) invoiceOrderChoices(c *gin.Context, customer bool, actor, operator, query, currency string) {
	type choice struct {
		ID, Issuer, Name, Kind, Amount string
		CreatedAt                      time.Time
	}
	db := a.DB.WithContext(c.Request.Context())
	var sql string
	var party any
	if customer {
		party = actor
		sql = `SELECT o.id::text,o.original_operator_id::text AS issuer,COALESCE(p.name,'') AS name,o.kind,o.due_money::text AS amount,o.created_at
   FROM commerce_orders o LEFT JOIN commerce_package_quotes q ON q.id=o.quote_id LEFT JOIN commerce_packages p ON p.id=q.package_id
   WHERE o.customer_id=? AND o.status='completed' AND o.kind IN ('package','renewal') AND o.due_money>0
   AND NOT EXISTS(SELECT 1 FROM operations_invoice_orders l JOIN operations_invoices i ON i.id=l.invoice_id WHERE l.order_id=o.id AND i.party_kind='customer' AND i.state NOT IN ('rejected','voided'))`
	} else {
		party = operator
		sql = `SELECT o.id::text,o.supplier_id::text AS issuer,o.name,'procurement' AS kind,o.due_money::text AS amount,o.created_at FROM cooperation_purchase_orders o
   WHERE o.buyer_id=? AND o.status='completed' AND o.due_money>0
   AND NOT EXISTS(SELECT 1 FROM operations_invoice_orders l JOIN operations_invoices i ON i.id=l.invoice_id WHERE l.order_id=o.id AND i.party_kind='procurement' AND i.state NOT IN ('rejected','voided'))`
	}
	cursor := c.Query("cursor")
	if cursor != "" {
		if _, e := uuid.Parse(cursor); e != nil {
			handleError(c, operations.ErrInvalid)
			return
		}
	}
	rows := []choice{}
	pattern := "%" + strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(query) + "%"
	if e := db.Raw("SELECT * FROM ("+sql+") purchases WHERE id>? AND (id ILIKE ? OR name ILIKE ?) ORDER BY id LIMIT 51", party, cursor, pattern, pattern).Scan(&rows).Error; e != nil {
		handleError(c, e)
		return
	}
	next := ""
	if len(rows) > 50 {
		rows = rows[:50]
		next = rows[49].ID
	}
	items := []referenceChoice{}
	for _, r := range rows {
		kind := map[string]string{"package": "套餐购买", "renewal": "套餐续费", "procurement": "采购订单"}[r.Kind]
		items = append(items, referenceChoice{Value: r.ID, Label: humanJoin(kind, r.Name, ledger.MustDecimal(r.Amount).String()+" "+currency, r.CreatedAt.UTC().Format("2006-01-02 15:04:05 UTC")), Defaults: gin.H{"issuer_id": r.Issuer, "supplier_id": r.Issuer}})
	}
	c.JSON(200, gin.H{"items": items, "next_cursor": next})
}
