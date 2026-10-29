package portal

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/cooperation"
	"gorm.io/gorm"
)

// A supplier's sales inbox includes its own retail and wholesale orders. The
// original domain records remain the sole authority for payment and delivery.
type salesOrder struct {
	ID, Kind, Status, Counterparty string
	CreatedAt                      time.Time
	retail                         *commerce.Order
	supply                         *cooperation.PurchaseOrder
}

func (r salesOrder) row() gin.H {
	var v gin.H
	actions := []string{}
	if r.retail != nil {
		v = values(*r.retail)
		actions = append(actions, "fulfill")
	} else {
		v = values(*r.supply)
		v["kind"] = "supply"
		v["external_allocated_money"] = r.supply.AllocatedMoney
		if r.Status == "paid" {
			actions = append(actions, "fulfill_supply")
		}
	}
	v["counterparty_name"], v["order_id"] = r.Counterparty, r.ID
	row := entity(r.ID, r.Status, 1, v)
	row["allowed_actions"] = actions
	return row
}

func (r salesOrder) cursor() string {
	return commerce.NextOrderCursor(commerce.Order{ID: r.ID, CreatedAt: r.CreatedAt})
}

func (a *Application) salesOrders(ctx context.Context, operator, cursor, search, status string, limit int, collectionOnly bool) ([]salesOrder, error) {
	if limit < 1 || limit > 100 {
		return nil, commerce.ErrInvalidInput
	}
	// Both sources are scoped before the union; filtering and pagination happen
	// once across the combined set, rather than merging two truncated pages.
	sources := `(
	 SELECT o.id,o.created_at,o.status,o.kind,
	 COALESCE(NULLIF(c.display_name,''),i.email_normalized,'') AS counterparty,
	 concat_ws(' ',o.id::text,o.kind,o.status,c.display_name,i.email_normalized) AS search_text,
	 (o.status='pending' AND o.external_allocated_money=0 AND o.due_money>o.internal_money+o.commission_money) AS collectable
	 FROM commerce_orders o JOIN organization_customers c ON c.id=o.customer_id
	 LEFT JOIN organization_customer_identities i ON i.customer_id=c.id AND i.is_current=true
	 WHERE o.operator_id=?
	 UNION ALL
	 SELECT o.id,o.created_at,o.status,'supply' AS kind,b.name AS counterparty,
	 concat_ws(' ',o.id::text,o.name,o.status,'supply 供货采购',b.name) AS search_text,
	 (o.status='awaiting_payment' AND o.allocated_money=0) AS collectable
	 FROM cooperation_purchase_orders o JOIN organization_operators b ON b.id=o.buyer_id
	 WHERE o.supplier_id=?
	) AS sales_orders`
	q := a.DB.WithContext(ctx).Table(sources, operator, operator).Select("id,created_at,status,kind,counterparty")
	if collectionOnly {
		q = q.Where("collectable")
	}
	if status = strings.TrimSpace(status); status != "" {
		q = q.Where("status=?", status)
	}
	if search = strings.TrimSpace(search); search != "" {
		term := "%" + strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(search) + "%"
		q = q.Where("search_text ILIKE ?", term)
	}
	if cursor != "" {
		body, err := base64.RawURLEncoding.DecodeString(cursor)
		var value commerce.OrderCursor
		if err != nil || json.Unmarshal(body, &value) != nil || uuid.Validate(value.ID) != nil || value.At.IsZero() {
			return nil, commerce.ErrInvalidInput
		}
		q = q.Where("(created_at,id)<(?,?)", value.At, value.ID)
	}
	rows := []salesOrder{}
	if err := q.Order("created_at DESC,id DESC").Limit(limit).Scan(&rows).Error; err != nil {
		return nil, err
	}
	retailIDs, supplyIDs := []string{}, []string{}
	for _, row := range rows {
		if row.Kind == "supply" {
			supplyIDs = append(supplyIDs, row.ID)
		} else {
			retailIDs = append(retailIDs, row.ID)
		}
	}
	retail := map[string]*commerce.Order{}
	supply := map[string]*cooperation.PurchaseOrder{}
	if len(retailIDs) > 0 {
		var orders []commerce.Order
		if err := a.DB.WithContext(ctx).Table("commerce_orders").Where("operator_id=? AND id IN ?", operator, retailIDs).Find(&orders).Error; err != nil {
			return nil, err
		}
		for i := range orders {
			retail[orders[i].ID] = &orders[i]
		}
	}
	if len(supplyIDs) > 0 {
		var orders []cooperation.PurchaseOrder
		if err := a.DB.WithContext(ctx).Where("supplier_id=? AND id IN ?", operator, supplyIDs).Find(&orders).Error; err != nil {
			return nil, err
		}
		for i := range orders {
			supply[orders[i].ID] = &orders[i]
		}
	}
	for i := range rows {
		rows[i].retail, rows[i].supply = retail[rows[i].ID], supply[rows[i].ID]
		if rows[i].retail == nil && rows[i].supply == nil {
			return nil, gorm.ErrRecordNotFound
		}
	}
	return rows, nil
}
