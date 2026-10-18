package portal

import (
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Presentation is read-only. Command fields and immutable saved evidence retain
// their original IDs; a human label must never become a command authority.
type humanPresentation struct {
	Title     string            `json:"title,omitempty"`
	TitleKind string            `json:"title_kind,omitempty"`
	Subtitle  string            `json:"subtitle,omitempty"`
	Labels    map[string]string `json:"labels"`
}
type humanLabelRow struct{ ID, Label string }

func humanString(v gin.H, keys ...string) string {
	for _, key := range keys {
		if text, ok := v[key].(string); ok && strings.TrimSpace(text) != "" {
			return text
		}
	}
	return ""
}
func humanJoin(parts ...string) string {
	out := []string{}
	for _, p := range parts {
		if strings.TrimSpace(p) != "" && !contains(out, p) {
			out = append(out, p)
		}
	}
	return strings.Join(out, " · ")
}
func humanTime(v gin.H) string {
	for _, key := range []string{"created_at", "effective_at", "registered_at", "observed_at", "paid_at"} {
		if raw := humanString(v, key); raw != "" {
			if at, e := time.Parse(time.RFC3339Nano, raw); e == nil {
				return at.UTC().Format("2006-01-02 15:04:05 UTC")
			}
		}
	}
	return ""
}
func readableIdentifier(raw string) string {
	if _, e := uuid.Parse(raw); e == nil {
		return ""
	}
	return raw
}

// All relation queries enforce the exact workspace boundary independently of
// the source list. A parent membership never authorizes child customer names,
// emails, transactions, or provider credentials.
func (a *Application) projectResourceLabels(c *gin.Context, p gin.H, resource string) error {
	rows, ok := p["items"].([]any)
	if !ok || len(rows) == 0 || a.DB == nil {
		return nil
	}
	id := subject(c)
	if id == nil {
		return nil
	}
	op := c.Param("operatorId")
	customer := ""
	if id.Session.Kind == "customer" {
		op = id.Customer.Ownership.OperatorID
		customer = id.Customer.Customer.ID
	}
	if op == "" {
		return nil
	}
	refs := map[string][]string{}
	add := func(group, value string) {
		// Domain audit subjects can be business keys, not UUID entities. Do
		// not cast such keys into UUID directory predicates.
		if group != "model" && group != "credential" {
			if _, err := uuid.Parse(value); err != nil {
				return
			}
		}
		if value != "" && !contains(refs[group], value) {
			refs[group] = append(refs[group], value)
		}
	}
	for _, raw := range rows {
		row, ok := raw.(gin.H)
		if !ok {
			continue
		}
		v, ok := row["values"].(gin.H)
		if !ok {
			continue
		}
		for key, val := range v {
			text, ok := val.(string)
			if !ok {
				continue
			}
			switch key {
			case "operator_id", "original_operator_id", "supplier_id", "buyer_id", "channel_id", "brand_operator_id", "issuer_id", "parent_operator_id":
				add("operator", text)
			case "brand_id":
				add("brand", text)
			case "customer_id", "kol_customer_id", "owner_customer_id":
				add("customer", text)
			case "owner_id":
				add("customer", text)
				add("operator", text)
			case "scope_id":
				if humanString(v, "scope_kind") == "customer" {
					add("customer", text)
				} else if humanString(v, "scope_kind") == "brand" {
					add("brand", text)
				} else {
					add("operator", text)
				}
			case "requested_by", "actor_id":
				add("customer", text)
				add("staff", text)
			case "subject_id":
				switch c.Query("view") {
				case "commerce":
					add("order", text)
					add("receipt", text)
					add("package", text)
					add("grant", text)
				case "incentives":
					add("customer", text)
					add("order", text)
					add("campaign", text)
				case "branding":
					add("operator", text)
					add("brand", text)
				default:
					add("customer", text)
					add("operator", text)
					add("brand", text)
				}
			case "staff_id", "created_by", "approved_by", "verified_by":
				add("staff", text)
			case "key_id":
				add("key", text)
			case "notification_id":
				add("notification", text)
			case "grant_id", "account_id":
				add("grant", text)
			case "campaign_id", "merchant_campaign_id", "kol_campaign_id":
				add("campaign", text)
			case "credential_id":
				add("credential", text)
			case "recipient_id":
				add("staff", text)
				add("customer", text)
			case "order_id":
				add("order", text)
			case "receipt_id":
				add("receipt", text)
			case "contract_attachment_id":
				add("contract", text)
			case "package_id", "plan_id":
				add("package", text)
			case "public_model_id", "model_id":
				add("model", text)
			}
		}
	}
	labels := map[string]string{}
	load := func(group string, q *gorm.DB) error {
		if len(refs[group]) == 0 {
			return nil
		}
		var found []humanLabelRow
		if err := q.Where("id IN ?", refs[group]).Scan(&found).Error; err != nil {
			return err
		}
		for _, r := range found {
			if r.Label != "" {
				labels[r.ID] = r.Label
			}
		}
		return nil
	}
	db := a.DB.WithContext(c.Request.Context())
	// Only the current operator and its direct supply neighbours are business
	// counterparties. Do not traverse the organization's descendants.
	if err := load("operator", db.Table("organization_operators").Select("id, concat_ws(' · ',name,NULLIF(legal_name,name)) AS label").Where("id=? OR parent_operator_id=? OR id=(SELECT parent_operator_id FROM organization_operators WHERE id=?)", op, op, op)); err != nil {
		return err
	}
	if err := load("brand", db.Table("organization_brands").Select("id,name AS label").Where("id IN (SELECT brand_id FROM organization_operators WHERE id=? OR parent_operator_id=?)", op, op)); err != nil {
		return err
	}
	if len(refs["customer"]) > 0 {
		mayEmail := customer != ""
		if !mayEmail {
			for _, permission := range []string{"customers.read", "refunds.manage", "withdrawals.manage"} {
				if a.Organization.RequirePermission(c.Request.Context(), nil, id.Session.SubjectID, op, permission) == nil {
					mayEmail = true
					break
				}
			}
		}
		selectLabel := "c.display_name AS label"
		if mayEmail {
			selectLabel = "concat_ws(' · ',NULLIF(c.display_name,''),NULLIF((SELECT email_normalized FROM organization_customer_identities e WHERE e.customer_id=c.id AND e.is_current=true LIMIT 1),'')) AS label"
		}
		q := db.Table("organization_customers c").Select("c.id,"+selectLabel).Where("c.id IN ?", refs["customer"])
		if customer != "" {
			q = q.Where("c.id=?", customer)
		} else {
			q = q.Where("EXISTS (SELECT 1 FROM organization_service_ownerships own WHERE own.customer_id=c.id AND own.operator_id=? AND own.ended_at IS NULL)", op)
		}
		var found []humanLabelRow
		if err := q.Scan(&found).Error; err != nil {
			return err
		}
		for _, r := range found {
			if r.Label != "" {
				labels[r.ID] = r.Label
			}
		}
	}
	if len(refs["staff"]) > 0 && customer == "" {
		var found []humanLabelRow
		if err := db.Table("portal_staff s").Select("s.id,concat_ws(' · ',NULLIF(s.display_name,''),s.email) AS label").Where("s.id IN ?", refs["staff"]).Where("EXISTS (SELECT 1 FROM organization_staff_memberships m WHERE m.staff_id=s.id AND m.operator_id=?)", op).Scan(&found).Error; err != nil {
			return err
		}
		for _, r := range found {
			labels[r.ID] = r.Label
		}
	}
	if len(refs["credential"]) > 0 && customer == "" && op == a.Installation.OperatorID && a.Organization.RequirePermission(c.Request.Context(), nil, id.Session.SubjectID, op, "catalog.manage") == nil {
		var found []struct{ ID, ProviderID, Fingerprint string }
		if err := db.Table("tokenhub_provider_credentials").Select("id,provider_id,fingerprint").Where("id IN ?", refs["credential"]).Scan(&found).Error; err != nil {
			return err
		}
		for _, r := range found {
			labels[r.ID] = humanJoin("供应商凭据", readableIdentifier(r.ProviderID), r.Fingerprint)
		}
	}
	packageQ := db.Table("commerce_packages").Select("id,name AS label").Where("operator_id=?", op)
	if err := load("package", packageQ); err != nil {
		return err
	}
	if len(refs["order"]) > 0 {
		var found []struct{ ID, Kind, Price, Created, Package string }
		q := db.Table("commerce_orders o").Select("o.id,o.kind,o.due_usd::text AS price,to_char(o.created_at AT TIME ZONE 'UTC','YYYY-MM-DD HH24:MI:SS') AS created,coalesce(p.name,'') AS package").Joins("LEFT JOIN commerce_package_quotes q ON q.id=o.quote_id").Joins("LEFT JOIN commerce_packages p ON p.id=q.package_id").Where("o.id IN ?", refs["order"])
		if customer != "" {
			q = q.Where("o.customer_id=?", customer)
		} else {
			q = q.Where("o.operator_id=?", op)
		}
		if err := q.Scan(&found).Error; err != nil {
			return err
		}
		for _, r := range found {
			kind := map[string]string{"topup": "余额充值", "package": "套餐购买", "renewal": "套餐续费"}[r.Kind]
			if kind == "" {
				kind = "订单"
			}
			labels[r.ID] = humanJoin(kind, r.Package, r.Price+" USD", r.Created+" UTC")
		}
	}
	if len(refs["receipt"]) > 0 {
		var found []struct{ ID, ProviderReceiptID, Currency, OriginalAmount string }
		q := db.Table("commerce_receipts r").Select("DISTINCT r.id,r.provider_receipt_id,r.currency,r.original_amount").Where("r.id IN ?", refs["receipt"])
		if customer != "" {
			q = q.Where("EXISTS (SELECT 1 FROM commerce_receipt_allocations a JOIN commerce_orders o ON o.id=a.order_id WHERE a.receipt_id=r.id AND o.customer_id=?)", customer)
		} else {
			q = q.Where("r.operator_id=?", op)
		}
		if err := q.Scan(&found).Error; err != nil {
			return err
		}
		for _, r := range found {
			labels[r.ID] = humanJoin(readableIdentifier(r.ProviderReceiptID), r.OriginalAmount+" "+r.Currency)
		}
		if customer == "" {
			// A purchase allocation permits only a minimal label for its
			// original receipt, never the supplier's payer or merchant data.
			var purchases []struct{ ID, Price, Created string }
			if err := db.Table("commerce_receipts r").Select("DISTINCT r.id,a.amount_usd::text AS price,to_char(r.created_at AT TIME ZONE 'UTC','YYYY-MM-DD HH24:MI:SS') AS created").Joins("JOIN cooperation_purchase_allocations a ON a.receipt_id=r.id").Joins("JOIN cooperation_purchase_orders o ON o.id=a.order_id").Where("r.id IN ? AND o.buyer_id=?", refs["receipt"], op).Scan(&purchases).Error; err != nil {
				return err
			}
			for _, r := range purchases {
				if labels[r.ID] == "" {
					labels[r.ID] = humanJoin(r.Price+" USD", r.Created+" UTC")
				}
			}
		}
	}
	if len(refs["key"]) > 0 {
		var found []humanLabelRow
		q := db.Table("usage_keys k").Select("k.id,k.name AS label").Where("k.id IN ?", refs["key"])
		if customer != "" {
			q = q.Where("k.customer_id=?", customer)
		} else {
			q = q.Where("EXISTS (SELECT 1 FROM organization_service_ownerships own WHERE own.customer_id=k.customer_id AND own.operator_id=? AND own.ended_at IS NULL)", op)
		}
		if err := q.Scan(&found).Error; err != nil {
			return err
		}
		for _, r := range found {
			labels[r.ID] = r.Label
		}
	}
	if len(refs["notification"]) > 0 {
		var found []humanLabelRow
		q := db.Table("operations_notifications").Select("id,title AS label").Where("id IN ? AND operator_id=?", refs["notification"], op)
		if customer != "" {
			q = q.Where("recipient_id=?", customer)
		}
		if err := q.Scan(&found).Error; err != nil {
			return err
		}
		for _, r := range found {
			labels[r.ID] = r.Label
		}
	}
	if len(refs["grant"]) > 0 {
		var found []struct{ ID, AccountID, Created, Total string }
		q := db.Table("ledger_grants").Select("id,account_id,total::text AS total,to_char(effective_at AT TIME ZONE 'UTC','YYYY-MM-DD HH24:MI:SS') AS created").Where("operator_id=? AND (id IN ? OR account_id IN ?)", op, refs["grant"], refs["grant"])
		if customer != "" {
			q = q.Where("customer_id=?", customer)
		}
		if err := q.Scan(&found).Error; err != nil {
			return err
		}
		for _, r := range found {
			labels[r.ID] = humanJoin(r.Total+" ATG", r.Created+" UTC")
			labels[r.AccountID] = labels[r.ID]
		}
	}
	if len(refs["campaign"]) > 0 {
		var found []humanLabelRow
		q := db.Table("incentives_campaigns c").Select("c.id,concat_ws(' · ',p.name,v.gift_atg::text || ' ATG',to_char(v.starts_at AT TIME ZONE 'UTC','YYYY-MM-DD HH24:MI:SS') || ' UTC') AS label").Joins("JOIN incentives_campaign_versions v ON v.campaign_id=c.id AND v.version=c.current_version").Joins("JOIN commerce_packages p ON p.id=v.package_id").Where("c.operator_id=? AND c.id IN ?", op, refs["campaign"])
		if customer != "" {
			q = q.Where("c.owner_customer_id=?", customer)
		}
		if err := q.Scan(&found).Error; err != nil {
			return err
		}
		for _, r := range found {
			labels[r.ID] = r.Label
		}
	}

	if len(refs["model"]) > 0 {
		var found []humanLabelRow
		q := db.Table("tokenhub_public_models m").Select("m.id,m.definition->>'display_name' AS label").Where("m.id IN ?", refs["model"])
		if op != a.Installation.OperatorID {
			q = q.Where("EXISTS (SELECT 1 FROM tokenhub_model_listings l WHERE l.public_model_id=m.id AND l.operator_id=?)", op)
		}
		if err := q.Scan(&found).Error; err != nil {
			return err
		}
		for _, r := range found {
			labels[r.ID] = humanJoin(r.Label, r.ID)
		}
	}
	if len(refs["contract"]) > 0 && customer == "" {
		var found []humanLabelRow
		if err := cooperationContractScope(db.Table("organization_cooperation_contracts"), op).Select("id,filename AS label").Where("id IN ?", refs["contract"]).Scan(&found).Error; err != nil {
			return err
		}
		for _, r := range found {
			labels[r.ID] = r.Label
		}
	}
	// Reuse already-authorized rows for references between cases, legs and
	// execution records. This never fetches another case just to name a relation.
	ownTitles := map[string]string{}
	for _, raw := range rows {
		if row, ok := raw.(gin.H); ok {
			v, _ := row["values"].(gin.H)
			ownTitles[fmt.Sprint(row["id"])] = humanRecordTitle(v, resource, labels)
		}
	}
	for _, raw := range rows {
		row, ok := raw.(gin.H)
		if !ok {
			continue
		}
		v, ok := row["values"].(gin.H)
		if !ok {
			continue
		}
		names := map[string]string{}
		for key, val := range v {
			text, ok := val.(string)
			if !ok {
				continue
			}
			if label := labels[text]; label != "" {
				names[key] = label
			} else if label := ownTitles[text]; label != "" && strings.HasSuffix(key, "_id") {
				names[key] = label
			}
		}
		title := ownTitles[fmt.Sprint(row["id"])]
		subtitle := humanString(v, "email", "legal_name", "identity", "hostname", "filename")
		if subtitle == title {
			subtitle = ""
		}
		if subtitle == "" {
			subtitle = humanJoin(names["customer_id"], names["supplier_id"], names["buyer_id"])
		}
		prefix := ""
		if humanString(v, "name", "display_name", "title", "filename", "hostname", "operator_name", "model", "public_model_id", "model_id") == "" && readableIdentifier(humanString(v, "invoice_reference", "provider_receipt_id", "provider_case_id")) == "" {
			prefix = strings.Split(title, " · ")[0]
		}
		row["presentation"] = humanPresentation{Title: title, TitleKind: prefix, Subtitle: subtitle, Labels: names}
	}
	return nil
}
func humanRecordTitle(v gin.H, resource string, labels map[string]string) string {
	if v == nil {
		return "业务记录"
	}
	if name := humanString(v, "name", "display_name", "title", "filename", "hostname", "operator_name"); name != "" {
		return name
	}
	if name := readableIdentifier(humanString(v, "invoice_reference", "provider_receipt_id", "provider_case_id")); name != "" {
		return name
	}
	if model := humanString(v, "model", "public_model_id", "model_id"); model != "" {
		if label := labels[model]; label != "" {
			return label
		}
		return model
	}
	kinds := map[string]string{"case": "退款事项", "leg": "退款分项", "actual_refund_payment": "实际退款付款", "exit": "采购退出事项", "refund_leg": "退出退款分项", "buyer_refund_leg": "采购方退款分项", "quote": "采购报价", "order": "采购订单", "authorization": "自动补货授权", "replenishment_attempt": "自动补货执行", "topup": "余额充值", "package": "套餐购买", "renewal": "套餐续费"}
	kind := kinds[humanString(v, "record_kind", "kind")]
	if kind == "" {
		kind = map[string]string{"orders": "订单", "payments": "收款", "assets": "资产账户", "ledger": "资产账户", "marketing": "营销活动", "activities": "KOL 活动", "withdrawals": "佣金提现", "commissions": "佣金规则", "payment-methods": "收款方式", "providers": "供应商凭据", "coverage": "履约覆盖", "credit-sources": "ATG 来源", "staff": "经营人员", "invoices": "客户发票", "procurement-invoices": "采购发票", "evidence": "证据文件"}[resource]
	}
	if kind == "" {
		kind = "业务记录"
	}
	if resource == "payment-methods" {
		return humanJoin(kind, humanString(v, "driver"), humanString(v, "currency"))
	}
	if resource == "providers" {
		return humanJoin(kind, readableIdentifier(humanString(v, "provider_id")), humanString(v, "fingerprint"))
	}
	return humanJoin(kind, labels[humanString(v, "package_id", "plan_id")], humanTime(v))
}
