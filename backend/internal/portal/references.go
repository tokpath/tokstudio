package portal

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"github.com/tokpath/tokstudio/backend/internal/payments"
	"strings"
)

type referenceChoice struct {
	Value    string `json:"value"`
	Label    string `json:"label"`
	Defaults gin.H  `json:"defaults,omitempty"`
}

func (a *Application) references(c *gin.Context) {
	ctx := c.Request.Context()
	kind := c.Param("reference")
	id := subject(c)
	customer := id.Session.Kind == "customer"
	operator := c.Param("operatorId")
	actor := id.Session.SubjectID
	if customer {
		operator = id.Customer.Ownership.OperatorID
	}
	items := []referenceChoice{}
	query := strings.ToLower(strings.TrimSpace(c.Query("q")))
	next := ""
	if len(query) > 200 {
		handleError(c, organization.ErrInvalidInput)
		return
	}
	require := func(actions ...string) bool {
		if customer {
			return true
		}
		for _, action := range actions {
			if a.Organization.RequirePermission(ctx, nil, actor, operator, action) == nil {
				return true
			}
		}
		handleError(c, organization.ErrForbidden)
		return false
	}
	add := func(value, label string, defaults gin.H) {
		if len(items) >= 50 {
			return
		}
		if contains([]string{"customers", "orders", "receipts"}, kind) || query == "" || strings.Contains(strings.ToLower(label+value), query) {
			items = append(items, referenceChoice{value, label + " · " + value, defaults})
		}
	}
	switch kind {
	case "payment-assets":
		if customer || !require("payments.manage") {
			return
		}
		rows, e := a.Payments.ListSupportedAssets(ctx)
		if e != nil {
			handleError(c, e)
			return
		}
		for _, r := range rows {
			add(r.VersionID, r.Driver+" · "+r.Asset+" · 网络 "+r.Network+" · 合约 "+r.TokenContract, gin.H{"asset_network_version_id": r.VersionID, "driver": r.Driver})
		}
	case "scope-subjects":
		if customer || !require("security.manage") {
			return
		}
		op, e := a.Organization.GetOperator(ctx, nil, operator)
		if e != nil {
			handleError(c, e)
			return
		}
		add(op.ID, "本经营方 · "+op.Name, gin.H{"scope_kind": "operator"})
		if op.Kind != organization.Channel {
			add(op.BrandID, "本品牌", gin.H{"scope_kind": "brand"})
		}
		rows, cursor, e := a.Organization.ReferenceCustomers(ctx, nil, actor, operator, query, c.Query("cursor"), 48)
		if e != nil {
			handleError(c, e)
			return
		}
		next = cursor
		for _, r := range rows {
			items = append(items, referenceChoice{Value: r.ID, Label: r.DisplayName + " · " + r.ID, Defaults: gin.H{"scope_kind": "customer"}})
		}
	case "customers":
		if customer {
			handleError(c, organization.ErrForbidden)
			return
		}
		rows, cursor, e := a.Organization.ReferenceCustomers(ctx, nil, actor, operator, query, c.Query("cursor"), 50)
		if e != nil {
			handleError(c, e)
			return
		}
		next = cursor
		for _, r := range rows {
			add(r.ID, r.DisplayName+" · "+r.Email, gin.H{"customer_id": r.ID})
		}
	case "plans":
		if !require("sales.manage", "marketing.manage", "refunds.manage") {
			return
		}
		rows, e := a.Commerce.ListPackages(ctx, nil, operator)
		if e != nil {
			handleError(c, e)
			return
		}
		for _, r := range rows {
			defaults := gin.H{"package_id": r.ID, "name": r.Name, "new_purchase_enabled": r.NewPurchaseEnabled, "renewal_enabled": r.RenewalEnabled}
			label := r.Name
			if r.CurrentQuoteID != nil {
				q, e := a.Commerce.GetQuote(ctx, nil, *r.CurrentQuoteID)
				if e != nil {
					handleError(c, e)
					return
				}
				defaults["expected_version"] = q.Version
				for k, v := range values(q) {
					if k != "id" && k != "package_id" {
						defaults[k] = v
					}
				}
				label += " · " + q.PriceUSD.String() + " USD"
			}
			add(r.ID, label, defaults)
		}
	case "orders":
		if customer {
			handleError(c, organization.ErrForbidden)
			return
		}
		if !require("sales.manage", "refunds.manage", "payments.manage") {
			return
		}
		rows, e := a.Commerce.ListOperatorOrders(ctx, nil, operator, c.Query("cursor"), query, 50)
		if e != nil {
			handleError(c, e)
			return
		}
		for _, r := range rows {
			add(r.ID, r.Kind+" · "+r.DueUSD.String()+" USD · "+r.Status, gin.H{"order_id": r.ID, "customer_id": r.CustomerID})
		}
	case "receipts":
		if customer {
			rows, e := a.Commerce.CustomerHistoricalReceiptReferences(ctx, nil, actor, query, c.Query("cursor"), 50)
			if e != nil {
				handleError(c, e)
				return
			}
			for _, r := range rows {
				add(r.ID, r.Provider+" · "+r.GrossUSD.String()+" USD · 原开票方 "+r.OperatorID, gin.H{"issuer_id": r.OperatorID})
			}
			break
		}
		if !require("payments.manage", "refunds.manage", "invoices.manage") {
			return
		}
		rows, e := a.Commerce.SearchOperatorReceipts(ctx, nil, operator, query, c.Query("cursor"), 50)
		if e != nil {
			handleError(c, e)
			return
		}
		for _, r := range rows {
			add(r.ID, r.Provider+" · "+r.GrossUSD.String()+" USD · "+r.ProviderReceiptID, gin.H{"receipt_id": r.ID, "amount_usd": r.GrossUSD.Sub(r.AllocatedUSD).String(), "currency": r.Currency})
		}
	case "credentials":
		if customer || !require("catalog.manage") {
			return
		}
		if operator != a.Installation.OperatorID {
			handleError(c, organization.ErrForbidden)
			return
		}
		rows, e := a.Catalog.AdminCredentials(ctx, operator)
		if e != nil {
			handleError(c, e)
			return
		}
		for _, r := range rows {
			add(r.ID, "供应商 "+r.ProviderID+" · "+r.Fingerprint, gin.H{"id": r.ID, "provider_id": r.ProviderID, "enabled": r.Enabled, "expected_version": r.Version})
		}
	case "merchants":
		if customer {
			handleError(c, organization.ErrForbidden)
			return
		}
		if !require("payments.manage") {
			return
		}
		rows, e := a.Payments.ListMerchantConfigs(ctx, actor, operator)
		if e != nil {
			handleError(c, e)
			return
		}
		for _, r := range rows {
			add(r.ID, r.Driver+" · "+fmt.Sprint(r.Enabled), gin.H{"id": r.ID, "driver": r.Driver, "enabled": r.Enabled, "asset_network_version_id": r.AssetNetworkVersionID, "expected_version": r.Version})
		}
	case "supplier-methods":
		if customer {
			handleError(c, organization.ErrForbidden)
			return
		}
		if !require("cooperation.manage") {
			return
		}
		edge, e := a.Organization.GetSupplyAgreement(ctx, nil, operator)
		if e != nil {
			handleError(c, e)
			return
		}
		rows, e := a.Payments.PublicMethods(ctx, edge.SupplierID)
		if e != nil {
			handleError(c, e)
			return
		}
		for _, r := range rows {
			add(r.ID, r.Driver+" · "+r.Currency, nil)
		}
	case "buyers", "suppliers":
		if customer {
			handleError(c, organization.ErrForbidden)
			return
		}
		if !require("cooperation.manage", "organization.create", "organization.read") {
			return
		}
		if kind == "buyers" {
			children, e := a.Organization.Children(ctx, nil, operator)
			if e != nil {
				handleError(c, e)
				return
			}
			for _, child := range children {
				add(child.ID, child.Name+" · "+child.LegalName, nil)
			}
		} else {
			edge, e := a.Organization.GetSupplyAgreement(ctx, nil, operator)
			if e != nil {
				handleError(c, e)
				return
			}
			supplier, e := a.Organization.GetOperator(ctx, nil, edge.SupplierID)
			if e != nil {
				handleError(c, e)
				return
			}
			add(supplier.ID, supplier.Name+" · "+supplier.LegalName, nil)
		}
	case "purchase-quotes":
		if customer {
			handleError(c, organization.ErrForbidden)
			return
		}
		if !require("cooperation.manage") {
			return
		}
		rows, e := a.Cooperation.ListBuyerQuotes(ctx, nil, actor, operator)
		if e != nil {
			handleError(c, e)
			return
		}
		for _, r := range rows {
			add(r.ID, r.PriceUSD.String()+" USD · "+r.PaidATG.String()+" + "+r.GiftATG.String()+" ATG", gin.H{"quote_id": r.ID, "supplier_id": r.SupplierID})
		}
	case "campaigns":
		if customer {
			rows, e := a.Incentives.OwnCampaigns(ctx, nil, operator, actor)
			if e != nil {
				handleError(c, e)
				return
			}
			for _, r := range rows {
				defaults := values(r.Version)
				defaults["campaign_id"] = r.Campaign.ID
				defaults["expected_version"] = r.Version.Version
				add(r.Campaign.ID, "KOL · "+r.Version.GiftATG.String()+" ATG", defaults)
			}
		} else {
			if !require("marketing.manage") {
				return
			}
			rows, e := a.Incentives.CampaignsForOperator(ctx, nil, actor, operator)
			if e != nil {
				handleError(c, e)
				return
			}
			for _, r := range rows {
				defaults := values(r.Version)
				defaults["campaign_id"] = r.Campaign.ID
				defaults["expected_version"] = r.Version.Version
				add(r.Campaign.ID, r.Campaign.Kind+" · "+r.Version.GiftATG.String()+" ATG", defaults)
			}
		}
	default:
		handleError(c, organization.ErrInvalidInput)
		return
	}
	c.JSON(200, gin.H{"items": items, "next_cursor": next})
}

var _ payments.Merchant
