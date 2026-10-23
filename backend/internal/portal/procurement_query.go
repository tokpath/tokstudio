package portal

import (
	"encoding/base64"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/cooperation"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"gorm.io/gorm"
	"sort"
	"strings"
	"time"
)

type procurementPage struct {
	Quotes           []cooperation.PurchaseQuote
	Orders           []cooperation.PurchaseOrder
	Authorizations   []cooperation.ReplenishmentAuthorization
	Attempts         []cooperation.ReplenishmentAttempt
	Keys             []procurementPageKey
	Next             string
	Unpaid, Delivery ledger.Decimal
	Holding          int
}
type procurementPageKey struct {
	At time.Time `json:"at"`
	ID string    `json:"id"`
}

func (a *Application) queryProcurement(c *gin.Context, actor, operator, side string) (out procurementPage, err error) {
	ctx := c.Request.Context()
	if err = a.Organization.RequirePermission(ctx, nil, actor, operator, "cooperation.manage"); err != nil {
		return
	}
	section := c.Query("section")
	if section != "" && section != "orders" && section != "quotes" && section != "automatic" {
		return out, organization.ErrInvalidInput
	}
	var cursor procurementPageKey
	if raw := c.Query("cursor"); raw != "" {
		b, e := base64.RawURLEncoding.DecodeString(raw)
		if e != nil || json.Unmarshal(b, &cursor) != nil || cursor.At.IsZero() || cursor.ID == "" {
			return out, organization.ErrInvalidInput
		}
	}
	scope := side + "_id" // side is validated by the resource handler.
	build := func(table, timeColumn, statusExpr, searchExpr string) *gorm.DB {
		db := a.DB.WithContext(ctx).Table(table+" r").Where("r."+scope+"=?", operator)
		if side == "buyer" && (table == "cooperation_replenishment_authorizations" || table == "cooperation_replenishment_attempts") {
			db = a.DB.WithContext(ctx).Table(table+" r").Where("r.buyer_id=?", operator)
		}
		if q := strings.TrimSpace(c.Query("q")); q != "" {
			// strpos gives literal case-insensitive search; percent/underscore aren't wildcards.
			db = db.Where("(strpos(lower("+searchExpr+"),lower(?))>0 OR EXISTS (SELECT 1 FROM organization_operators p WHERE (p.id=r.buyer_id"+func() string {
				if table == "cooperation_replenishment_attempts" {
					return ""
				}
				return " OR p.id=r.supplier_id"
			}()+") AND strpos(lower(p.name),lower(?))>0))", q, q)
		}
		if status := c.Query("status"); status != "" {
			db = db.Where("("+statusExpr+")=?", status)
		}
		if id := c.Query("record"); id != "" {
			db = db.Where("r.id::text=?", id)
		}
		if !cursor.At.IsZero() {
			db = db.Where("(r."+timeColumn+",r.id::text)<(?,?)", cursor.At, cursor.ID)
		}
		return db.Select("r.*").Order("r." + timeColumn + " DESC,r.id DESC").Limit(51)
	}
	if section == "" || section == "quotes" {
		err = build("cooperation_purchase_quotes", "created_at", "CASE WHEN gift_tiers IS NULL THEN 'archived' WHEN NOT enabled THEN 'disabled' WHEN valid_until<=clock_timestamp() THEN 'expired' ELSE 'quote' END", "concat_ws(' ',r.id::text,r.name,r.terms_summary)").Find(&out.Quotes).Error
		if err != nil {
			return
		}
		for _, r := range out.Quotes {
			out.Keys = append(out.Keys, procurementPageKey{r.CreatedAt, r.ID})
		}
	}
	if section == "" || section == "orders" {
		err = build("cooperation_purchase_orders", "created_at", "r.status", "concat_ws(' ',r.id::text,r.name,r.terms_summary,r.business_key)").Find(&out.Orders).Error
		if err != nil {
			return
		}
		for _, r := range out.Orders {
			out.Keys = append(out.Keys, procurementPageKey{r.CreatedAt, r.ID})
		}
	}
	if side == "buyer" && (section == "" || section == "automatic") {
		err = build("cooperation_replenishment_authorizations", "updated_at", "'authorization'::text", "concat_ws(' ',r.id::text,r.consent_reference)").Find(&out.Authorizations).Error
		if err != nil {
			return
		}
		for _, r := range out.Authorizations {
			out.Keys = append(out.Keys, procurementPageKey{r.UpdatedAt, r.ID})
		}
		err = build("cooperation_replenishment_attempts", "created_at", "r.state", "concat_ws(' ',r.id::text,r.purchase_order_id::text,r.error_code)").Find(&out.Attempts).Error
		if err != nil {
			return
		}
		for _, r := range out.Attempts {
			out.Keys = append(out.Keys, procurementPageKey{r.CreatedAt, r.ID})
		}
	}
	sort.Slice(out.Keys, func(i, j int) bool {
		if out.Keys[i].At.Equal(out.Keys[j].At) {
			return out.Keys[i].ID > out.Keys[j].ID
		}
		return out.Keys[i].At.After(out.Keys[j].At)
	})
	if len(out.Keys) > 50 {
		out.Keys = out.Keys[:50]
		raw, _ := json.Marshal(out.Keys[49])
		out.Next = base64.RawURLEncoding.EncodeToString(raw)
	}
	var sum struct {
		Unpaid, Delivery ledger.Decimal
		Holding          int
	}
	err = a.DB.WithContext(ctx).Table("cooperation_purchase_orders").Select("COALESCE(SUM(CASE WHEN status='awaiting_payment' THEN paid_atg+gift_atg ELSE 0 END),0) unpaid,COALESCE(SUM(CASE WHEN status='paid' THEN paid_atg+gift_atg ELSE 0 END),0) delivery,count(*) holding").Where(scope+"=? AND status IN ('awaiting_payment','paid')", operator).Scan(&sum).Error
	out.Unpaid, out.Delivery, out.Holding = sum.Unpaid, sum.Delivery, sum.Holding
	return
}
