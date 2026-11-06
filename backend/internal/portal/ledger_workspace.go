package portal

import (
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/organization"
)

// These are read models: saved accounts, journals and command identifiers are
// never rewritten to make a label. Every relation stays in the current workspace.
type ledgerSource struct {
	Kind       string     `json:"kind"`
	Name       string     `json:"name"`
	At         *time.Time `json:"at,omitempty"`
	URL        string     `json:"url,omitempty"`
	Amount     string     `json:"amount,omitempty"`
	Unit       string     `json:"unit,omitempty"`
	CustomerID string     `json:"-"`
	BuyerID    string     `json:"-"`
}
type ledgerAccountDisplay struct {
	Owner        string          `json:"owner"`
	Purpose      string          `json:"purpose"`
	Auxiliary    bool            `json:"auxiliary"`
	Source       *ledgerSource   `json:"source,omitempty"`
	EffectiveAt  *time.Time      `json:"effective_at,omitempty"`
	ExpiresAt    *time.Time      `json:"expires_at,omitempty"`
	Availability string          `json:"availability,omitempty"`
	Available    *ledger.Decimal `json:"available,omitempty"`
}

func ledgerPurpose(p string) string {
	labels := map[string]string{
		"customer:money": "支付余额", "commission:available": "可用佣金", "commission:frozen": "冻结佣金", "procurement:atg": "采购额度",
		"order:funds": "订单已分配款", "receipt:unallocated": "未分配收款", "control:provider_receivable": "通道未结应收", "control:external_cash": "外部现金往来", "control:merchant_fees": "经营方收款手续费", "control:refund_fees": "经营方退款手续费", "control:sales": "套餐销售记账", "control:wholesale_sales": "供货销售记账", "control:credits": "客户权益平衡", "control:cancelled": "权益收回记账", "control:commissions": "佣金计提记账", "control:commission_payouts": "佣金付款记账", "control:withdrawal_fees": "提现手续费", "control:procurement": "采购额度平衡", "control:procurement_removal": "采购退出额度记账", "control:takeover": "接管权益对账", "control:takeover_settlement": "接管结清对账", "procurement_exit:payable": "采购退出应付款", "procurement_exit:receivable": "采购退出应收款", "control:procurement_exit_obligation": "采购退出结算记账", "control:external_receipt": "外部收款往来", "control:procurement_refund_excess": "采购退款差额记账",
	}
	if label := labels[p]; label != "" {
		return label
	}
	for _, item := range [][2]string{{"customer:atg:", "客户 ATG 权益"}, {"control:rounding:", "计量尾差记账"}, {"cost:supplier:", "供应商调用成本"}, {"control:supplier:", "供应商成本平衡"}, {"control:", "其他记账辅助账户"}} {
		if strings.HasPrefix(p, item[0]) {
			return item[1]
		}
	}
	return "其他记账账户"
}

func (a *Application) ledgerSources(c *gin.Context, op string, ids []string) (map[string]*ledgerSource, error) {
	out := map[string]*ledgerSource{}
	valid := []string{}
	for _, id := range ids {
		if _, err := uuid.Parse(id); err == nil {
			valid = append(valid, id)
		}
	}
	ids = valid
	if len(ids) == 0 {
		return out, nil
	}
	db := a.DB.WithContext(c.Request.Context())
	permissions := map[string]bool{}
	link := func(resource, id string) string {
		permission := permissionFor(resource)
		allowed, checked := permissions[permission]
		if !checked {
			allowed = a.Organization.RequirePermission(c.Request.Context(), nil, subject(c).Session.SubjectID, op, permission) == nil
			if !allowed && resource == "orders" {
				allowed = a.Organization.RequirePermission(c.Request.Context(), nil, subject(c).Session.SubjectID, op, "payments.manage") == nil
			}
			permissions[permission] = allowed
		}
		if !allowed {
			return ""
		}
		parameter := "record"
		if resource == "payments" {
			parameter = "q"
		}
		return "/manage/" + op + "/" + resource + "?" + parameter + "=" + url.QueryEscape(id)
	}
	var orders []struct {
		ID, Kind, CustomerID, Name, Amount string
		CreatedAt                          time.Time
	}
	err := db.Table("commerce_orders o").Select("o.id,o.kind,o.customer_id,COALESCE(p.name,'') AS name,o.due_money::text AS amount,o.created_at").Joins("LEFT JOIN commerce_package_quotes q ON q.id=o.quote_id").Joins("LEFT JOIN commerce_packages p ON p.id=q.package_id AND p.operator_id=o.operator_id").Where("o.operator_id=? AND o.id IN ?", op, ids).Scan(&orders).Error
	if err != nil {
		return nil, err
	}
	for _, o := range orders {
		kind := map[string]string{"topup": "余额充值", "package": "套餐购买", "renewal": "套餐续费"}[o.Kind]
		if kind == "" {
			kind = "订单"
		}
		at := o.CreatedAt
		out[o.ID] = &ledgerSource{Kind: kind, Name: o.Name, CustomerID: o.CustomerID, Amount: o.Amount, Unit: "MONEY", At: &at, URL: link("orders", o.ID)}
	}
	var purchases []struct {
		ID, BuyerID, Name, Amount string
		CreatedAt                 time.Time
	}
	err = db.Table("cooperation_purchase_orders").Select("id,buyer_id,name,due_money::text AS amount,created_at").Where("(supplier_id=? OR buyer_id=?) AND id IN ?", op, op, ids).Scan(&purchases).Error
	if err != nil {
		return nil, err
	}
	for _, o := range purchases {
		at := o.CreatedAt
		out[o.ID] = &ledgerSource{Kind: "采购订单", Name: o.Name, BuyerID: o.BuyerID, Amount: o.Amount, Unit: "MONEY", At: &at, URL: link("orders", o.ID)}
	}
	var receipts []struct {
		ID, CustomerID, BuyerID, Amount, Currency string
		CreatedAt                                 time.Time
	}
	err = db.Table("commerce_receipts r").Select(`r.id,r.original_amount AS amount,r.currency,r.created_at,
 (SELECT o.customer_id FROM commerce_receipt_allocations al JOIN commerce_orders o ON o.id=al.order_id AND o.operator_id=r.operator_id WHERE al.receipt_id=r.id LIMIT 1) AS customer_id,
 (SELECT o.buyer_id FROM cooperation_purchase_allocations al JOIN cooperation_purchase_orders o ON o.id=al.order_id AND o.supplier_id=r.operator_id WHERE al.receipt_id=r.id LIMIT 1) AS buyer_id`).Where("r.operator_id=? AND r.id IN ?", op, ids).Scan(&receipts).Error
	if err != nil {
		return nil, err
	}
	// Provider receipt identifiers may contain redeemable codes. The readable
	// source uses amount/time; the original receipt page owns its safe details.
	for _, r := range receipts {
		at := r.CreatedAt
		out[r.ID] = &ledgerSource{Kind: "收款", CustomerID: r.CustomerID, BuyerID: r.BuyerID, Amount: r.Amount, Unit: r.Currency, At: &at, URL: link("payments", r.ID)}
	}
	return out, nil
}

func (a *Application) ledgerDisplays(c *gin.Context, op string, accounts []ledger.Account, extraIDs []string) (map[string]ledgerAccountDisplay, map[string]*ledgerSource, error) {
	ids := append([]string{}, extraIDs...)
	accountIDs := []string{}
	for _, r := range accounts {
		ids = append(ids, r.OwnerID)
		accountIDs = append(accountIDs, r.ID)
	}
	var grants []ledger.Grant
	if len(accountIDs) > 0 {
		if err := a.DB.WithContext(c.Request.Context()).Where("operator_id=? AND account_id IN ?", op, accountIDs).Find(&grants).Error; err != nil {
			return nil, nil, err
		}
	}
	giftKinds := map[string]string{}
	if len(accountIDs) > 0 {
		var giftRows []struct{ AccountID, Kind string }
		if err := a.DB.WithContext(c.Request.Context()).Table("ledger_grant_sources s").Select("g.account_id,s.kind").Joins("JOIN ledger_grants g ON g.id=s.grant_id").Where("g.operator_id=? AND g.account_id IN ? AND s.kind IN ?", op, accountIDs, []string{"registration_gift", "manual_gift"}).Scan(&giftRows).Error; err != nil {
			return nil, nil, err
		}
		for _, r := range giftRows {
			giftKinds[r.AccountID] = map[string]string{"registration_gift": "注册赠送", "manual_gift": "定向赠送"}[r.Kind]
		}
	}
	grantByAccount := map[string]ledger.Grant{}
	for _, g := range grants {
		grantByAccount[g.AccountID] = g
		ids = append(ids, g.OrderID)
	}
	sources, err := a.ledgerSources(c, op, ids)
	if err != nil {
		return nil, nil, err
	}
	labelRows := []any{}
	for _, r := range accounts {
		v := gin.H{"owner_id": r.OwnerID}
		src := sources[r.OwnerID]
		if g, ok := grantByAccount[r.ID]; ok {
			src = sources[g.OrderID]
		}
		if src != nil {
			v["customer_id"] = src.CustomerID
			v["buyer_id"] = src.BuyerID
		}
		labelRows = append(labelRows, entity(r.ID, "", 0, v))
	}
	if err := a.projectResourceLabels(c, gin.H{"items": labelRows}, "ledger"); err != nil {
		return nil, nil, err
	}
	displays := map[string]ledgerAccountDisplay{}
	var now time.Time
	if err := a.DB.WithContext(c.Request.Context()).Raw("SELECT clock_timestamp()").Row().Scan(&now); err != nil {
		return nil, nil, err
	}
	for i, r := range accounts {
		labels := labelRows[i].(gin.H)["presentation"].(humanPresentation).Labels
		owner := labels["customer_id"]
		if owner == "" {
			owner = labels["buyer_id"]
		}
		if owner == "" {
			owner = labels["owner_id"]
		}
		d := ledgerAccountDisplay{Owner: owner, Purpose: ledgerPurpose(r.Purpose), Auxiliary: !ledger.IsBusinessAccount(r.Purpose), Source: sources[r.OwnerID]}
		if !d.Auxiliary {
			n := r.Available()
			d.Available = &n
		}
		if r.Purpose == "commission:frozen" {
			n := ledger.Zero()
			d.Available = &n
			d.Availability = "等待佣金确认"
		}
		if g, ok := grantByAccount[r.ID]; ok {
			d.Source = sources[g.OrderID]
			d.EffectiveAt = &g.EffectiveAt
			d.ExpiresAt = g.ExpiresAt
			if d.Source == nil {
				kind := giftKinds[r.ID]
				if kind == "" {
					kind = "权益发放"
				}
				d.Source = &ledgerSource{Kind: kind, At: &g.CreatedAt}
			}
			if now.Before(g.EffectiveAt) {
				d.Availability = "尚未生效"
				n := ledger.Zero()
				d.Available = &n
			} else if g.ExpiresAt != nil && !now.Before(*g.ExpiresAt) {
				d.Availability = "已到期"
				n := ledger.Zero()
				d.Available = &n
			}
			// A grant source is also the business source of issuance/expiry journals.
			sources[g.ID] = d.Source
		}
		displays[r.ID] = d
	}
	return displays, sources, nil
}

func (a *Application) ledgerAccountsResource(c *gin.Context, op organization.Operator, p gin.H) {
	include := c.Query("include_auxiliary") == "true"
	rows, err := a.Ledger.ListAssetAccounts(c.Request.Context(), op.ID, c.Query("cursor"), c.Query("q"), 51, include)
	if err != nil {
		handleError(c, err)
		return
	}
	if len(rows) > 50 {
		rows = rows[:50]
		p["next_cursor"] = rows[49].ID
	}
	displays, _, err := a.ledgerDisplays(c, op.ID, rows, nil)
	if err != nil {
		handleError(c, err)
		return
	}
	items := []any{}
	for _, r := range rows {
		v := values(r)
		v["display"] = displays[r.ID]
		items = append(items, entity(r.ID, "", r.Version, v))
	}
	p["items"] = items
	p["search_supported"] = true
	p["status_supported"] = false
	p["columns"] = []gin.H{col("owner_id", "所属对象", "text"), col("purpose", "资产类型", "text"), col("balance", "账面余额", "decimal"), col("held", "占用", "decimal")}
	a.resourceResponse(c, p)
}
