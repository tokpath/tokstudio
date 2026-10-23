package portal

import (
	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/cooperation"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"github.com/tokpath/tokstudio/backend/internal/payments"
	"strings"
	"time"
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
		if contains([]string{"customers", "orders", "receipts"}, kind) || query == "" || strings.Contains(strings.ToLower(label+value), query) {
			items = append(items, referenceChoice{value, label, defaults})
		}
	}
	switch kind {
	case "pricing-models":
		if customer || operator != a.Installation.OperatorID || !require("catalog.manage") {
			return
		}
		rows, e := a.ModelPricing.Rows(ctx, query)
		if e != nil {
			handleError(c, e)
			return
		}
		for _, r := range rows {
			add(r.ID, humanJoin(r.Model, r.Provider, r.Mode), gin.H{"cost_provider": r.Provider, "cost_model": r.Model, "model": r.Model})
		}
	case "cost-suppliers":
		if customer || operator != a.Installation.OperatorID || !require("platform.manage") {
			return
		}
		rows, e := a.Catalog.AdminCredentials(ctx, operator)
		if e != nil {
			handleError(c, e)
			return
		}
		seen := map[string]bool{}
		for _, r := range rows {
			if !seen[r.ProviderID] {
				add(r.ProviderID, r.Name, nil)
				seen[r.ProviderID] = true
			}
		}
	case "branding-brands":
		if customer || !require("organization.read") {
			return
		}
		op, err := a.Organization.GetOperator(ctx, nil, operator)
		if err != nil {
			handleError(c, err)
			return
		}
		targets, err := a.appearanceTargets(c, op, false)
		if err != nil {
			handleError(c, err)
			return
		}
		for _, target := range targets {
			if target.Kind != organization.Channel && a.Organization.RequireBrandBranding(ctx, nil, actor, target.BrandID) == nil {
				add(target.ID, target.Name, nil)
			}
		}
	case "export-subjects":
		if customer || !require("reports.export") {
			return
		}
		var e error
		items, next, e = a.exportSubjectChoices(c, operator, actor, query)
		if e != nil {
			handleError(c, e)
			return
		}
	case "supply-failure-approvals":
		if customer || !require("cooperation.manage") {
			return
		}
		rows := []cooperation.SupplyFailureApproval{}
		if e := a.DB.WithContext(ctx).Where("buyer_id=?", operator).Order("approved_at DESC,id DESC").Find(&rows).Error; e != nil {
			handleError(c, e)
			return
		}
		for _, r := range rows {
			add(r.ID, humanJoin("已批准供货失约", r.ApprovedAt.UTC().Format("2006-01-02 15:04:05 UTC"), r.Evidence), nil)
		}
	case "recipients":
		if customer || !require("operations.manage") {
			return
		}
		mayEmail := a.Organization.RequirePermission(ctx, nil, actor, operator, "customers.read") == nil
		customerLabel := "c.display_name || ' · 客户'"
		if mayEmail {
			customerLabel = "concat_ws(' · ',NULLIF(c.display_name,''),NULLIF(e.email_normalized,''),'客户')"
		}
		var rows []humanLabelRow
		sql := `SELECT id,label FROM (
            SELECT s.id::text AS id,concat_ws(' · ',NULLIF(s.display_name,''),s.email,'经营人员') AS label FROM portal_staff s
            WHERE s.status='active' AND EXISTS (SELECT 1 FROM organization_staff_memberships m WHERE m.staff_id=s.id AND m.operator_id=? AND m.status='active')
            UNION ALL SELECT c.id::text AS id,` + customerLabel + ` AS label FROM organization_customers c
            LEFT JOIN organization_customer_identities e ON e.customer_id=c.id AND e.is_current=true
            WHERE EXISTS(SELECT 1 FROM organization_service_ownerships own WHERE own.customer_id=c.id AND own.operator_id=? AND own.ended_at IS NULL)
        ) recipients WHERE id>? AND (label ILIKE ? OR id ILIKE ?) ORDER BY id LIMIT 51`
		if e := a.DB.WithContext(ctx).Raw(sql, operator, operator, c.Query("cursor"), "%"+query+"%", "%"+query+"%").Scan(&rows).Error; e != nil {
			handleError(c, e)
			return
		}
		if len(rows) > 50 {
			rows = rows[:50]
			next = rows[49].ID
		}
		for _, r := range rows {
			items = append(items, referenceChoice{Value: r.ID, Label: r.Label})
		}
	case "contracts":
		if customer || !require("cooperation.manage", "organization.create") {
			return
		}
		rows, e := a.listOwnedCooperationContracts(ctx, operator)
		if e != nil {
			handleError(c, e)
			return
		}
		for _, r := range rows {
			add(r.ID, r.Filename, nil)
		}
	case "routes":
		if customer || !require("catalog.manage", "sales.manage") {
			return
		}
		if operator != a.Installation.OperatorID {
			handleError(c, organization.ErrForbidden)
			return
		}
		var rows []struct{ ID, ProviderID, Provider, Model, PublicModelID string }
		if e := a.DB.WithContext(ctx).Table("tokenhub_upstreams").Select("id,provider_id,public_model_id,definition->>'provider' AS provider,definition->>'model' AS model").Order("id").Scan(&rows).Error; e != nil {
			handleError(c, e)
			return
		}
		for _, r := range rows {
			add(r.ID, humanJoin(r.Model, r.Provider, r.PublicModelID), gin.H{"provider_id": r.ProviderID})
		}
	case "models":
		if customer {
			handleError(c, organization.ErrForbidden)
			return
		}
		if !require("catalog.manage", "sales.manage") {
			return
		}
		var models []catalog.PublicModel
		var err error
		if operator == a.Installation.OperatorID {
			models, err = a.Catalog.AdminModels(ctx, operator)
		} else {
			models, err = a.Catalog.PricingModels(ctx, operator)
		}
		if err != nil {
			handleError(c, err)
			return
		}
		buyer := c.Query("buyer_id")
		if buyer != "" {
			child, e := a.Organization.GetOperator(ctx, nil, buyer)
			if e != nil || child.ParentOperatorID == nil || *child.ParentOperatorID != operator {
				handleError(c, organization.ErrForbidden)
				return
			}
		}
		var listings []struct {
			PublicModelID string
			Version       uint64
		}
		if err = a.DB.WithContext(ctx).Table("tokenhub_model_listings").Select("public_model_id,version").Where("operator_id=?", operator).Scan(&listings).Error; err != nil {
			handleError(c, err)
			return
		}
		listingVersions := map[string]uint64{}
		for _, row := range listings {
			listingVersions[row.PublicModelID] = row.Version
		}
		authorizationVersions := map[string]uint64{}
		if buyer != "" {
			var rows []struct {
				PublicModelID string
				Version       uint64
			}
			if err = a.DB.WithContext(ctx).Table("tokenhub_model_authorizations").Select("public_model_id,version").Where("seller_id=? AND buyer_id=?", operator, buyer).Scan(&rows).Error; err != nil {
				handleError(c, err)
				return
			}
			for _, row := range rows {
				authorizationVersions[row.PublicModelID] = row.Version
			}
		}
		for _, model := range models {
			prepared, err := a.preparedPricingModel(ctx, model)
			if err != nil {
				handleError(c, err)
				return
			}
			defaults := gin.H{"public_model_id": model.ID, "identity": model.Identity, "listing_version": listingVersions[model.ID], "billing": prepared.Billing, "pricing_components": prepared.BillingComponents(), "expected_version": model.Version}
			if buyer != "" {
				defaults["authorization_version"] = authorizationVersions[model.ID]
			}
			add(model.ID, model.ID, defaults)
		}
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
			items = append(items, referenceChoice{Value: r.ID, Label: humanJoin(r.DisplayName, r.Email, "客户"), Defaults: gin.H{"scope_kind": "customer"}})
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
			add(r.ID, humanJoin(r.DisplayName, r.Email), gin.H{"customer_id": r.ID})
		}
	case "plans":
		if !require("sales.manage", "marketing.manage", "refunds.manage") {
			return
		}
		rows := []commerce.Package{}
		var e error
		if customer {
			rows, e = a.Commerce.ListPackages(ctx, nil, operator)
		} else {
			e = a.DB.WithContext(ctx).Table("commerce_packages").Where("operator_id=?", operator).Order("created_at,id").Find(&rows).Error
		}
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
		rows, e := a.Commerce.ListOperatorOrders(ctx, nil, operator, c.Query("cursor"), query, 51)
		if e != nil {
			handleError(c, e)
			return
		}
		if len(rows) > 50 {
			rows = rows[:50]
			next = commerce.NextOrderCursor(rows[49])
		}
		for _, r := range rows {
			add(r.ID, humanJoin(map[string]string{"topup": "余额充值", "package": "套餐购买", "renewal": "套餐续费"}[r.Kind], r.DueUSD.String()+" USD", r.CreatedAt.UTC().Format("2006-01-02 15:04:05 UTC")), gin.H{"order_id": r.ID, "customer_id": r.CustomerID})
		}
	case "receipts":
		if customer {
			rows, e := a.Commerce.CustomerHistoricalReceiptReferences(ctx, nil, actor, query, c.Query("cursor"), 50)
			if e != nil {
				handleError(c, e)
				return
			}
			if len(rows) == 50 {
				more, e := a.Commerce.CustomerHistoricalReceiptReferences(ctx, nil, actor, query, rows[49].ID, 1)
				if e != nil {
					handleError(c, e)
					return
				}
				if len(more) > 0 {
					next = rows[49].ID
				}
			}
			for _, r := range rows {
				add(r.ID, humanJoin(r.Provider, r.GrossUSD.String()+" USD", readableIdentifier(r.ProviderReceiptID)), gin.H{"issuer_id": r.OperatorID})
			}
			break
		}
		if !require("payments.manage", "refunds.manage", "invoices.manage") {
			return
		}
		rows, e := a.Commerce.SearchOperatorReceipts(ctx, nil, operator, query, c.Query("cursor"), 50, c.Query("whole") == "true")
		if e != nil {
			handleError(c, e)
			return
		}
		if len(rows) == 50 {
			more, e := a.Commerce.SearchOperatorReceipts(ctx, nil, operator, query, rows[49].ID, 1, c.Query("whole") == "true")
			if e != nil {
				handleError(c, e)
				return
			}
			if len(more) > 0 {
				next = rows[49].ID
			}
		}
		for _, r := range rows {
			add(r.ID, r.Provider+" · "+r.GrossUSD.String()+" USD · "+r.ProviderReceiptID, gin.H{"receipt_id": r.ID, "amount_usd": r.GrossUSD.Sub(r.AllocatedUSD).String(), "currency": r.Currency})
		}
	case "supplier-models", "cost-models":
		permission := "catalog.manage"
		if kind == "cost-models" {
			permission = "platform.manage"
		}
		if customer || !require(permission) {
			return
		}
		if operator != a.Installation.OperatorID {
			handleError(c, organization.ErrForbidden)
			return
		}
		models, e := a.Catalog.AdminSupplierModels(ctx, operator)
		if e != nil {
			handleError(c, e)
			return
		}
		credentials, e := a.Catalog.AdminCredentials(ctx, operator)
		if e != nil {
			handleError(c, e)
			return
		}
		labels := supplierLabels(credentials)
		for _, model := range models {
			if !model.Enabled && kind != "cost-models" {
				continue
			}
			add(model.ID, humanJoin(model.Model, labels[model.CredentialID], model.Provider), gin.H{"provider_id": model.ProviderID, "credential_id": model.CredentialID})
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
			add(r.ID, credentialChoiceLabel(r.Name, r.ProviderID, r.Fingerprint), gin.H{"id": r.ID, "name": r.Name, "provider_id": r.ProviderID, "enabled": r.Enabled, "expected_version": r.Version})
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
			add(r.ID, humanJoin(r.Driver, map[bool]string{true: "已启用", false: "未启用"}[r.Enabled]), gin.H{"id": r.ID, "driver": r.Driver, "enabled": r.Enabled, "asset_network_version_id": r.AssetNetworkVersionID, "expected_version": r.Version})
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
		if !require("cooperation.manage", "organization.create", "organization.read", "sales.manage") {
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
			if r.Enabled && len(r.GiftTiers) > 0 && (r.ValidUntil == nil || time.Now().Before(*r.ValidUntil)) {
				add(r.ID, humanJoin(r.Name, "1 USD = 1 ATG + 阶梯赠送"), gin.H{"quote_id": r.ID, "supplier_id": r.SupplierID, "gift_tiers": r.GiftTiers})
			}
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
	if !contains([]string{"customers", "scope-subjects", "orders", "receipts", "recipients", "export-subjects"}, kind) {
		start := 0
		if cursor := c.Query("cursor"); cursor != "" {
			for i, item := range items {
				if item.Value == cursor {
					start = i + 1
					break
				}
			}
		}
		if start > 0 {
			items = items[start:]
		}
		if len(items) > 50 {
			items = items[:50]
			next = items[49].Value
		}
	}
	c.JSON(200, gin.H{"items": items, "next_cursor": next})
}

var _ payments.Merchant
