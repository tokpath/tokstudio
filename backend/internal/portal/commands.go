package portal

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/branding"
	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/cooperation"
	"github.com/tokpath/tokstudio/backend/internal/incentives"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/modelruntime"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"github.com/tokpath/tokstudio/backend/internal/payments"
	"gorm.io/gorm"
)

func decodeConfiguration(c *gin.Context) (json.RawMessage, bool) {
	var raw json.RawMessage
	if !body(c, &raw) {
		return nil, false
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil {
		fail(c, 400, "invalid_configuration", "请输入业务字段", false)
		return nil, false
	}
	if value, ok := fields["configuration"]; ok {
		if len(fields) != 1 || !json.Valid(value) {
			fail(c, 400, "invalid_configuration", "配置内容格式错误", false)
			return nil, false
		}
		return value, true
	}
	return raw, true
}

func (a *Application) operatorCommand(c *gin.Context) {
	resource := c.Param("resource")
	op, actor, ok := a.staffScope(c, resource)
	if !ok {
		return
	}
	ctx := c.Request.Context()
	if resource == "payment-assets" {
		a.paymentAssetsCommand(c, op, actor)
		return
	}
	if resource == "gifts" {
		a.issueManualGift(c, op, actor)
		return
	}
	if resource == "contracts" {
		a.uploadContract(c, op, actor)
		return
	}
	if resource == "evidence" {
		a.uploadEvidence(c, op, actor)
		return
	}
	if resource == "procurement-invoices" {
		a.procurementInvoicesCommand(c, op, actor)
		return
	}
	if resource == "restrictions" {
		a.restrictionsCommand(c, op, actor)
		return
	}
	if resource == "bifrost" {
		a.modelSettingsCommand(c, op, actor)
		return
	}
	if resource == "supplier-costs" {
		a.actualCostCommand(c, op, actor)
		return
	}
	if resource == "mail" {
		a.mailCommand(c)
		return
	}
	if resource == "exits" {
		a.exitsCommand(c, op, actor)
		return
	}
	if resource == "disputes" {
		a.disputesCommand(c, op, actor)
		return
	}
	if resource == "procurement" {
		a.procurementCommand(c, op, actor)
		return
	}
	if resource == "takeovers" {
		a.takeoverCommand(c, op, actor)
		return
	}
	if resource == "refunds" {
		a.refundStaffCommand(c, op, actor)
		return
	}
	if resource == "withdrawals" {
		a.withdrawalStaffCommand(c, op, actor)
		return
	}
	if a.operatorOperationsCommand(c, op, actor, resource) {
		return
	}
	if resource == "valuations" {
		var in struct {
			Currency    string         `json:"currency"`
			USDPerUnit  ledger.Decimal `json:"usd_per_unit"`
			Source      string         `json:"source"`
			EffectiveAt time.Time      `json:"effective_at"`
			ExpiresAt   time.Time      `json:"expires_at"`
		}
		if !body(c, &in) {
			return
		}
		raw, e := a.command(c, "valuation:"+op.ID, in, func(tx *gorm.DB) (any, error) {
			if in.Currency == "USD" || strings.TrimSpace(in.Currency) == "" || in.USDPerUnit.Sign() <= 0 || in.Source == "" || !in.ExpiresAt.After(in.EffectiveAt) {
				return nil, payments.ErrInvalid
			}
			row := valuationRow{ID: uuid.NewString(), OperatorID: op.ID, Currency: strings.ToUpper(in.Currency), USDPerUnit: in.USDPerUnit, Source: in.Source, ActorID: actor, EffectiveAt: in.EffectiveAt, ExpiresAt: in.ExpiresAt, CreatedAt: time.Now().UTC()}
			return commandResult(row.ID, "published"), tx.Create(&row).Error
		})
		jsonResponse(c, raw, e)
		return
	}
	if resource == "staff" {
		a.inviteStaff(c, op, actor)
		return
	}
	if resource == "domains" {
		a.domainRequest(c, op, actor)
		return
	}
	if resource == "coverage" {
		a.coverageCommand(c, op, actor)
		return
	}
	config, ok := decodeConfiguration(c)
	if !ok {
		return
	}
	raw, e := a.command(c, "operator:"+op.ID+":"+resource+":"+c.Query("target_operator_id"), config, func(tx *gorm.DB) (any, error) {
		switch resource {
		case "plans":
			var in commerce.PublishPackageInput
			if e := json.Unmarshal(config, &in); e != nil {
				return nil, commerce.ErrInvalidInput
			}
			in.ActorStaffID = actor
			in.OperatorID = op.ID

			out, e := a.Commerce.PublishPackage(ctx, tx, in)
			if e == nil && in.PackageID != "" {
				e = a.enqueuePackageChange(ctx, tx, out, "")
			}
			return gin.H{"id": out.PackageID, "status": "published", "version": out.Version}, e
		case "payments":
			var in manualReceiptInput
			if e := json.Unmarshal(config, &in); e != nil {
				return nil, commerce.ErrInvalidInput
			}
			fact, e := in.verified(op.ID)
			if e != nil {
				return nil, e
			}
			if strings.TrimSpace(fact.ProviderReceiptID) == "" {
				businessKey, _ := key(c)
				fact.ProviderReceiptID = payments.ManualReference("receipt", op.ID, businessKey)
			}
			out, e := a.Commerce.RecordManualReceipt(ctx, tx, actor, fact)
			if e == nil && in.OrderID != "" {
				businessKey, _ := key(c)
				var allocation salesReceiptAllocation
				matching := tx.Transaction(func(match *gorm.DB) error {
					var err error
					allocation, err = a.allocateWholeSalesReceipt(ctx, match, actor, op.ID, out.ID, in.OrderID, "manual:"+businessKey)
					return err
				})
				if matching != nil {
					return gin.H{"id": out.ID, "status": "recorded", "message": "真实收款已登记，订单尚未关联；请在收款记录核对金额和原订单，原付款未知时先核实原交易"}, nil
				}
				if allocation.DeliveryPending {
					return gin.H{"id": out.ID, "status": "recorded", "message": "收款已关联，采购额度待交付；请在原订单重试交付。"}, nil
				}
			}
			return commandResult(out.ID, "recorded"), e
		case "payment-methods":
			var in payments.MerchantInput
			if e := json.Unmarshal(config, &in); e != nil {
				return nil, payments.ErrInvalid
			}
			m := in.Configuration()
			if m.Credentials == nil {
				m.Credentials = map[string]string{}
			}
			m.OperatorID = op.ID
			if m.ID == "" {
				m.ID = uuid.NewString()
			}
			svc := *a.Payments
			svc.DB = tx
			if m.Driver == "kvoucher" {
				if strings.TrimSpace(m.Credentials["webhook_secret"]) == "" {
					delete(m.Credentials, "webhook_secret")
				}
				if strings.TrimSpace(m.Credentials["webhook_id"]) == "" {
					delete(m.Credentials, "webhook_id")
				}
			}
			if in.ExpectedVersion > 0 {
				old, e := svc.Merchant(ctx, op.ID, m.ID)
				if e != nil {
					return nil, e
				}
				if m.APIBaseURL == "" {
					m.APIBaseURL = old.APIBaseURL
				}
				if old.Driver != m.Driver {
					return nil, payments.ErrForbidden
				}
				for name, value := range old.Credentials {
					if _, provided := m.Credentials[name]; !provided {
						m.Credentials[name] = value
					}
				}
			}
			out, e := svc.SaveMerchant(ctx, actor, m, in.ExpectedVersion)
			if e == nil && m.Driver == "kvoucher" && out.Merchant.Enabled && len(out.MissingFields) == 0 {
				beforeID, beforeSecret := out.Merchant.Credentials["webhook_id"], out.Merchant.Credentials["webhook_secret"]
				updated, hookErr := svc.EnsureKVoucherWebhook(ctx, out.Merchant, a.Config.PublicBaseURL)
				if hookErr != nil {
					return nil, hookErr
				}
				if updated.Credentials["webhook_id"] != beforeID || updated.Credentials["webhook_secret"] != beforeSecret {
					out, e = svc.SaveMerchant(ctx, actor, updated, out.Merchant.Version)
				}
			}
			status := "pending_configuration"
			if e == nil && len(out.MissingFields) == 0 && out.Merchant.Enabled {
				status = "ready"
			}
			return gin.H{"id": m.ID, "status": status, "version": out.Merchant.Version, "message": "配置已保存；真实商户收款效果须在材料齐全后验证"}, e
		case "operators":
			var in struct {
				Kind                              organization.OperatorKind `json:"kind"`
				Name, LegalName, BrandName        string
				SupplyTerms, ContractAttachmentID string
			}
			var fields map[string]json.RawMessage
			if json.Unmarshal(config, &fields) != nil {
				return nil, organization.ErrInvalidInput
			}
			_ = json.Unmarshal(fields["kind"], &in.Kind)
			_ = json.Unmarshal(fields["name"], &in.Name)
			_ = json.Unmarshal(fields["legal_name"], &in.LegalName)
			_ = json.Unmarshal(fields["brand_name"], &in.BrandName)
			_ = json.Unmarshal(fields["supply_terms"], &in.SupplyTerms)
			_ = json.Unmarshal(fields["contract_attachment_id"], &in.ContractAttachmentID)
			out, e := a.Organization.CreateOperator(ctx, tx, organization.CreateOperatorInput{ActorStaffID: actor, ParentOperatorID: op.ID, Kind: in.Kind, Name: in.Name, LegalName: in.LegalName, BrandName: in.BrandName, SupplyTerms: in.SupplyTerms, ContractAttachmentID: in.ContractAttachmentID})
			return gin.H{"id": out.Operator.ID, "status": "configured", "message": "经营方已建立。供货关系已经生效，请继续邀请负责人，并配置价格、收款与开业检查"}, e
		case "marketing":
			var action struct {
				Action, CustomerID, Status, Evidence, MerchantCampaignID, KOLCampaignID string
				Blocked                                                                 bool `json:"release_blocked"`
				Enabled                                                                 bool `json:"enabled"`
			}
			var fields map[string]json.RawMessage
			if json.Unmarshal(config, &fields) != nil {
				return nil, incentives.ErrInvalidInput
			}
			_ = json.Unmarshal(config, &action)
			for k, v := range map[string]*string{"customer_id": &action.CustomerID, "merchant_campaign_id": &action.MerchantCampaignID, "kol_campaign_id": &action.KOLCampaignID} {
				_ = json.Unmarshal(fields[k], v)
			}
			if action.Action == "function" {
				out, e := a.Incentives.SetFunctionState(ctx, tx, actor, op.ID, action.CustomerID, action.Status, action.Evidence, action.Blocked)
				return commandResult(out.ID, out.Status), e
			}
			if action.Action == "combination" {
				e := a.Incentives.AllowCombination(ctx, tx, actor, op.ID, action.MerchantCampaignID, action.KOLCampaignID, action.Enabled)
				return commandResult(action.MerchantCampaignID, "combination_configured"), e
			}
			var in incentives.CampaignInput
			if json.Unmarshal(config, &in) != nil {
				return nil, incentives.ErrInvalidInput
			}
			in.ActorStaffID = actor
			in.OperatorID = op.ID
			in.Kind = "merchant"
			in.OwnerCustomerID = ""
			out, e := a.Incentives.PublishCampaign(ctx, tx, in)
			return gin.H{"id": out.CampaignID, "status": "published", "version": out.Version}, e
		case "commissions":
			var in struct {
				incentives.PolicyInput
				ConfirmationDays   *int64          `json:"confirmation_days"`
				RewardValidityDays *int64          `json:"reward_validity_days"`
				DirectPercent      *ledger.Decimal `json:"direct_percent"`
				IndirectPercent    *ledger.Decimal `json:"indirect_percent"`
			}
			if json.Unmarshal(config, &in) != nil || in.ConfirmationDays == nil || in.RewardValidityDays == nil || in.DirectPercent == nil || in.IndirectPercent == nil {
				return nil, incentives.ErrInvalidInput
			}
			confirmation, ok := policySecondsFromDays(*in.ConfirmationDays, 0, 365)
			reward, rewardOK := policySecondsFromDays(*in.RewardValidityDays, 1, 3650)
			if !ok || !rewardOK {
				return nil, incentives.ErrInvalidInput
			}
			direct, rateErr := percentageRate(*in.DirectPercent)
			indirect, indirectErr := percentageRate(*in.IndirectPercent)
			if rateErr != nil || indirectErr != nil {
				return nil, incentives.ErrInvalidInput
			}
			in.DirectRate, in.IndirectRate = direct, indirect
			in.ConfirmationSeconds = confirmation
			in.RewardValiditySeconds = reward
			out, e := a.Incentives.SetPolicy(ctx, tx, actor, op.ID, in.PolicyInput)
			return gin.H{"id": out.ID, "status": "published", "version": out.Version}, e
		case "branding", "channel-themes":
			target, targetErr := a.brandingTarget(c, op)
			if targetErr != nil {
				return nil, targetErr
			}
			var in struct {
				ExpectedVersion int64                   `json:"expected_version"`
				Values          branding.ThemeOverrides `json:"values"`
			}
			if json.Unmarshal(config, &in) != nil {
				return nil, branding.ErrInvalidInput
			}
			input := branding.SetThemeInput{ActorStaffID: actor, TargetID: target.ID, ExpectedVersion: in.ExpectedVersion, Values: in.Values}
			var out branding.ThemeConfig
			var e error
			if target.Kind == organization.Channel {
				out, e = a.Branding.SetChannelTheme(ctx, tx, input)
			} else {
				input.TargetID = target.BrandID
				out, e = a.Branding.SetBrandTheme(ctx, tx, input)
			}
			return gin.H{"id": target.ID, "status": "updated", "version": out.Version}, e
		case "business-status":
			var in struct {
				Feature         organization.BusinessFeature `json:"feature"`
				Enabled         bool                         `json:"enabled"`
				ExpectedVersion int64                        `json:"expected_version"`
				Evidence        string                       `json:"evidence_reference"`
			}
			if json.Unmarshal(config, &in) != nil {
				return nil, organization.ErrInvalidInput
			}
			if strings.TrimSpace(in.Evidence) == "" {
				in.Evidence = "business settings: " + string(in.Feature)
			}
			out, e := a.Organization.SetBusinessStatus(ctx, tx, organization.SetBusinessStatusInput{ActorStaffID: actor, OperatorID: op.ID, Feature: in.Feature, Enabled: in.Enabled, ExpectedVersion: in.ExpectedVersion, EvidenceReference: in.Evidence}, a)
			return gin.H{"id": op.ID, "status": "updated", "version": out.Version}, e
		case "catalog", "providers", "routes", "pricing":
			return a.catalogCommand(ctx, tx, op, actor, resource, config)
		}
		return nil, commerce.ErrNotReady
	})
	jsonResponse(c, raw, e)
}
func (a *Application) operatorRowCommand(c *gin.Context) {
	resource := c.Param("resource")
	scopeResource := resource
	if resource == "orders" && c.Param("action") == "fulfill_supply" {
		scopeResource = "payments"
	}
	op, actor, ok := a.staffScope(c, scopeResource)
	if !ok {
		return
	}
	ctx := c.Request.Context()
	action := c.Param("action")
	if resource == "receipt-valuations" && action == "confirm" {
		a.confirmReceiptValuation(c, op, actor)
		return
	}
	if resource == "catalog" && action == "availability" {
		a.publicModelAvailability(c, op)
		return
	}
	if resource == "staff" && action == "membership" {
		a.staffMembershipAction(c, op, actor)
		return
	}
	if resource == "exits" {
		a.exitsAction(c, op, actor, action)
		return
	}
	if resource == "disputes" {
		a.disputesAction(c, op, actor, action)
		return
	}
	if resource == "procurement" {
		a.procurementAction(c, op, actor, action)
		return
	}
	if resource == "takeovers" {
		a.takeoverAction(c, op, actor, action)
		return
	}
	if resource == "refunds" {
		a.refundStaffAction(c, op, actor, action)
		return
	}
	if resource == "jobs" && action == "redrive" {
		a.redrive(c, op)
		return
	}

	if resource == "domains" {
		a.domainAction(c, op, actor, action)
		return
	}
	if resource == "operators" && action == "invite_owner" {
		child, e := a.Organization.GetOperator(ctx, nil, c.Param("id"))
		if e != nil {
			handleError(c, e)
			return
		}
		if child.ParentOperatorID == nil || *child.ParentOperatorID != op.ID {
			handleError(c, organization.ErrForbidden)
			return
		}
		a.inviteStaff(c, child, actor)
		return
	}
	if resource == "withdrawals" {
		a.withdrawalStaffAction(c, op, actor, action)
		return
	}
	if a.operatorOperationsRowCommand(c, op, actor, resource, action) {
		return
	}
	switch {
	case resource == "orders" && action == "fulfill_supply":
		var empty struct{}
		if !body(c, &empty) {
			return
		}
		raw, e := a.command(c, "supply-fulfill:"+op.ID+":"+c.Param("id"), empty, func(tx *gorm.DB) (any, error) {
			var supply cooperation.PurchaseOrder
			if err := tx.Where("id=? AND supplier_id=?", c.Param("id"), op.ID).Take(&supply).Error; err != nil {
				return nil, err
			}
			_, err := a.Cooperation.FulfillPurchase(ctx, tx, supply.ID, actor)
			return commandResult(supply.ID, "completed"), err
		})
		jsonResponse(c, raw, e)
	case resource == "orders" && action == "fulfill":
		var empty struct{}
		if !body(c, &empty) {
			return
		}
		raw, e := a.command(c, "fulfill:"+op.ID+":"+c.Param("id"), empty, func(tx *gorm.DB) (any, error) {
			orders, e := a.Commerce.ListOperatorOrders(ctx, tx, op.ID, "", c.Param("id"), 2)
			if e != nil {
				return nil, e
			}
			found := false
			for _, o := range orders {
				if o.ID == c.Param("id") {
					found = true
				}
			}
			if !found {
				return nil, commerce.ErrNotFound
			}
			d, e := a.Commerce.Fulfill(ctx, tx, c.Param("id"), actor)
			return commandResult(d.OrderID, "delivered"), e
		})
		jsonResponse(c, raw, e)
	case resource == "payments" && action == "allocate":
		var in struct {
			OrderID string `json:"order_id"`
		}
		if !body(c, &in) {
			return
		}
		businessKey, _ := key(c)
		raw, e := a.command(c, "allocate:"+op.ID, in, func(tx *gorm.DB) (any, error) {
			out, e := a.allocateWholeSalesReceipt(ctx, tx, actor, op.ID, c.Param("id"), in.OrderID, businessKey)
			return out.result("allocated"), e
		})
		jsonResponse(c, raw, e)
	case resource == "payments" && action == "accounting":
		a.receiptAccountingCommand(c, op, actor)
	default:
		fail(c, 404, "not_found", "经营命令不存在", false)
	}
}
func (a *Application) publicModelAvailability(c *gin.Context, op organization.Operator) {
	var in struct {
		Enabled  bool   `json:"enabled"`
		Expected uint64 `json:"expected_version"`
	}
	if !body(c, &in) {
		return
	}
	ctx := c.Request.Context()
	raw, e := a.command(c, "catalog-availability:"+op.ID+":"+c.Param("id"), in, func(tx *gorm.DB) (any, error) {
		e := a.Catalog.WithDB(tx).SetPublicModelEnabled(ctx, op.ID, c.Param("id"), in.Enabled, in.Expected)
		return commandResult(c.Param("id"), "saved"), e
	})
	jsonResponse(c, raw, e)
}

func (a *Application) catalogResource(c *gin.Context, op organization.Operator, actor, resource string, p gin.H, base string) bool {
	ctx := c.Request.Context()
	items := []any{}
	if resource == "catalog" || resource == "providers" || resource == "routes" {
		p["status_supported"] = false
	}
	switch resource {
	case "catalog":
		rows, e := a.Catalog.AdminModels(ctx, a.Installation.OperatorID)
		if e != nil {
			handleError(c, e)
			return true
		}
		routedIDs, routeErr := a.Catalog.EnabledRouteModelIDs(ctx)
		if routeErr != nil {
			handleError(c, routeErr)
			return true
		}
		routed := map[string]bool{}
		for _, id := range routedIDs {
			routed[id] = true
		}
		for _, row := range rows {
			item := entity(row.ID, "configured", int64(row.Version), row)
			item["values"].(gin.H)["route_configured"] = routed[row.ID]
			items = append(items, item)
		}
		openColumn := col("enabled", "开放模型", "boolean")
		openColumn["help"] = "在列表里打开后，客户才能调用。没有配置路由时不能打开。"
		p["columns"] = []gin.H{col("id", "公开模型标识", "text"), col("capabilities", "能力", "text"), openColumn}
		p["operations"] = []operation{{Name: "save", Label: "添加公开模型", Method: "POST", Path: base, Description: "填写公开模型标识，并勾选可以调用的能力。客户请求和页面都使用这个标识。是否对客户开放在列表里切换。没有配置路由时不能打开。计费模板和零售、批发 ATG 价格在「模型与定价」设置；供应商模型上的参考价目仅用于核算上游成本。", Fields: []field{jsonField("model 对象：id、capabilities（勾选的调用能力）、expected_version。是否开放不在这里修改。")}}}
	case "providers":
		rows, e := a.Catalog.AdminCredentials(ctx, a.Installation.OperatorID)
		if e != nil {
			handleError(c, e)
			return true
		}
		labels := supplierLabels(rows)
		for _, row := range rows {
			item := entity(row.ID, "configured", int64(row.Version), row)
			values := item["values"].(gin.H)
			values["record_kind"] = "credential"
			item["allowed_actions"] = []string{"credential"}
			items = append(items, item)
		}
		models, e := a.Catalog.AdminSupplierModels(ctx, a.Installation.OperatorID)
		if e != nil {
			handleError(c, e)
			return true
		}
		for _, row := range models {
			item := entity(row.ID, "configured", int64(row.Version), row)
			values := item["values"].(gin.H)
			values["record_kind"] = "supplier_model"
			values["name"] = row.Model
			if label := labels[row.CredentialID]; label != "" {
				values["supplier_name"] = label
			}
			item["allowed_actions"] = []string{"model"}
			items = append(items, item)
		}
		p["description"] = "上半部分管理供应商连接，下半部分维护供应商模型与能力。上游成本优先采用可靠的供应商费用，否则由 Bifrost 参考价目计算。"
		p["columns"] = []gin.H{col("record_kind", "类型", "text"), col("name", "名称", "text"), col("supplier_name", "供应商", "text"), col("provider", "协议", "text"), col("model", "供应商模型", "text"), col("capabilities", "能力", "text"), col("enabled", "启用", "boolean")}
		name := f("name", "凭据名称", "text")
		name.Help = "例如「OpenAI 生产」。以后可以只改名称，不必重新填写密钥。"
		createSecret := f("secret", "供应商密钥", "password")
		createSecret.Help = "粘贴供应商提供的密钥。保存后页面不再显示原文。"
		editSecret := optional("secret", "新的供应商密钥", "password")
		editSecret.Help = "留空表示继续使用原来的密钥。只有要更换时才填写。"
		enabled := defaultField("enabled", "启用", "boolean", true)
		version := f("expected_version", "当前版本（首次0）", "integer")
		// The credential id and provider id stay on the saved record so routes keep
		// using the same key. People do not type them.
		modelHelp := jsonField("supplier_model：凭据、协议、供应商模型名称、能力与参考成本模型、expected_version")
		p["operations"] = []operation{{Name: "credential", Label: "添加供应商密钥", Method: "POST", Path: base, Description: "填写名称和密钥。名称用来在列表里辨认这把密钥。", Fields: []field{name, createSecret, enabled, version}}, {Name: "model", Label: "添加供应商模型", Method: "POST", Path: base, Description: "选择供应商连接、接口协议和供应商模型，选择参考计价模型并确认能力。", Fields: []field{modelHelp}}}
		p["row_operations"] = []operation{{Name: "credential", Label: "修改密钥", Method: "POST", Path: base, Description: "可以修改名称或启用状态。密钥留空表示不更换。", Fields: []field{name, editSecret, enabled, optional("id", "凭据", "hidden"), optional("provider_id", "供应商", "hidden"), version}}, {Name: "model", Label: "编辑供应商模型", Method: "POST", Path: base, Description: "修改这个供应商模型的协议、能力和参考成本模型。已经绑定的路由会跟着更新。", Fields: []field{modelHelp}}}
	case "routes":
		models, e := a.Catalog.AdminModels(ctx, a.Installation.OperatorID)
		if e != nil {
			handleError(c, e)
			return true
		}
		credentials, e := a.Catalog.AdminCredentials(ctx, a.Installation.OperatorID)
		if e != nil {
			handleError(c, e)
			return true
		}
		suppliers := supplierLabels(credentials)
		for _, m := range models {
			policy, e := a.Catalog.RoutingPolicy(ctx, m.ID)
			if e != nil {
				handleError(c, e)
				return true
			}
			upstreams, e := a.Catalog.AdminUpstreams(ctx, a.Installation.OperatorID, m.ID)
			if e != nil {
				handleError(c, e)
				return true
			}
			health, _, e := a.Catalog.RouteHealth(ctx, m.ID)
			if e != nil {
				handleError(c, e)
				return true
			}
			bindings := []gin.H{}
			names := []string{}
			active, ready := 0, 0
			for _, row := range upstreams {
				state := health[row.ID]
				if row.Enabled && row.Healthy {
					active++
					if state.OpenUntil == nil || !state.OpenUntil.After(time.Now()) {
						ready++
					}
				}
				if row.Model != "" {
					names = append(names, humanJoin(row.Model, suppliers[row.CredentialID]))
				}
				bindings = append(bindings, gin.H{"id": row.ID, "supplier_model_id": row.SupplierModelID, "model": row.Model, "credential_id": row.CredentialID, "enabled": row.Enabled, "circuit": state.Circuit})
			}
			summary := "未配置可用线路"
			if ready > 0 {
				summary = "可接单"
			} else if active > 0 {
				summary = "线路暂停，等待恢复"
			}
			items = append(items, entity(m.ID, "configured", int64(policy.Version), gin.H{"public_model_id": m.ID, "name": m.ID, "strategy": policy.Strategy, "supplier_models": strings.Join(names, "、"), "health_summary": summary, "bindings": bindings}))
		}
		p["description"] = "为公开模型选择供应商模型和选路策略。系统自动暂停故障线路并探测恢复，其他正常线路继续服务。"
		p["columns"] = []gin.H{col("strategy", "选路策略", "text"), col("supplier_models", "供应商模型", "text"), col("health_summary", "线路状态", "text")}
		p["operations"] = []operation{{Name: "save", Label: "配置路由", Method: "POST", Path: base, Description: "选择公开模型和供应商模型，按列表顺序优先、最低延迟或预估最低成本选路。", Fields: []field{jsonField("route：public_model_id、strategy、bindings（供应商模型、启用，数组顺序即优先顺序）、expected_version")}}}
	case "pricing":
		var e error
		items, e = a.pricingWorkspaceItems(ctx, op, c.Query("q"))
		if e != nil {
			handleError(c, e)
			return true
		}
		p["description"] = "选择模型，设置客户零售价或直属下级供货价。计费模板决定计量方式，ATG 单价决定扣多少额度，供应商成本独立核算。首次定价直接发布；后续变更由系统通知并安排生效。"
		p["search_supported"] = true
		p["columns"] = []gin.H{col("name", "模型", "text"), col("prices", "客户当前调用价", "text"), col("status", "状态", "status")}
		listingLabel := "授权与上架"
		if op.Kind == organization.Channel {
			listingLabel = "上架与下架"
		}
		p["operations"] = []operation{
			{Name: "price", Label: "设置模型价格", Method: "POST", Path: base, Description: "选择模型和定价对象，填写 ATG 单价。系统自动生成价格版本并处理调价通知，无需填写通知编号或依据。", Fields: []field{jsonField("action:price、price:{scope,model_id,layer,rule,ends_at?}；版本、发布状态与普通调价时间由系统生成。")}},
			{Name: "listing", Label: listingLabel, Method: "POST", Path: base, Description: "上架和授权分别管理，正常变更由系统自动通知并安排生效。", Fields: []field{jsonField("action:listing 或 authorization、model_id、buyer_id（授权时）、enabled、expected_version")}},
		}
	default:
		return false
	}
	if resource == "pricing" && op.ID == a.Installation.OperatorID && a.Organization.RequirePermission(ctx, nil, actor, op.ID, "catalog.manage") == nil {
		operations := p["operations"].([]operation)
		p["operations"] = append(operations, operation{Name: "template", Label: "设置计费模板", Method: "POST", Path: base, Description: "按能力分别设置输入与输出的计量方式，按模态组合且不重复计费。已启用模型的变更由系统自动通知。", Fields: []field{jsonField("action:billing、model_id、operation、billing、expected_version")}})
	}
	p["items"] = items
	a.resourceResponse(c, p)
	return true
}
func (a *Application) catalogCommand(ctx context.Context, tx *gorm.DB, op organization.Operator, actor, resource string, raw json.RawMessage) (any, error) {
	cat := a.Catalog.WithDB(tx)
	cat.Operators = cooperation.CatalogDirectory{Organization: a.Organization, Tx: tx}
	controller := *a.CatalogChanges
	controller.Catalog = cat
	switch resource {
	case "catalog":
		var in struct {
			Model       catalog.PublicModel `json:"model"`
			Expected    uint64              `json:"expected_version"`
			EffectiveAt time.Time           `json:"effective_at"`
			Evidence    string              `json:"evidence"`
			Emergency   bool                `json:"emergency"`
		}
		if json.Unmarshal(raw, &in) != nil {
			return nil, catalog.ErrInvalidInput
		}
		if in.Emergency {
			out, e := controller.EmergencyDisablePublicModel(ctx, op.ID, in.Model.ID, in.Evidence, in.Expected)
			return commandResult(out.ID, out.State), e
		}
		// Opening and closing happen on the list switch. Saving the form keeps
		// the current availability, and a new model stays closed.
		if in.Expected == 0 {
			in.Model.Enabled = false
		} else {
			old, e := cat.PublicModel(ctx, in.Model.ID)
			if e != nil {
				return nil, e
			}
			if old.Version != in.Expected {
				return nil, catalog.ErrCatalogVersion
			}
			if in.Model.Billing == nil {
				prepared, err := a.preparedPricingModel(ctx, old)
				if err != nil {
					return nil, err
				}
				in.Model.Billing = prepared.Billing
			}
			in.Model.Enabled = old.Enabled
		}
		if in.Expected > 0 && in.EffectiveAt.IsZero() {
			e := cat.SavePublicModel(ctx, op.ID, in.Model, in.Expected)
			if e == nil {
				return commandResult(in.Model.ID, "saved"), nil
			}
			if !errors.Is(e, catalog.ErrChangeNotice) {
				return nil, e
			}
		}
		if in.Expected == 0 {
			e := cat.SavePublicModel(ctx, op.ID, in.Model, in.Expected)
			return commandResult(in.Model.ID, "saved"), e
		}
		out, e := controller.SchedulePublicModel(ctx, op.ID, in.Model, in.Expected, in.EffectiveAt, in.Evidence)
		return commandResult(out.ID, out.State), e
	case "providers":
		var supplier struct {
			Model    catalog.SupplierModel `json:"supplier_model"`
			Expected uint64                `json:"expected_version"`
		}
		if json.Unmarshal(raw, &supplier) == nil && supplier.Model.CredentialID != "" {
			e := cat.SaveSupplierModel(ctx, op.ID, supplier.Model, supplier.Expected)
			return commandResult(supplier.Model.ID, "saved"), e
		}
		var in struct {
			ID         string `json:"id"`
			ProviderID string `json:"provider_id"`
			Name       string `json:"name"`
			Secret     string `json:"secret"`
			Enabled    bool   `json:"enabled"`
			Expected   uint64 `json:"expected_version"`
		}
		if json.Unmarshal(raw, &in) != nil {
			return nil, catalog.ErrInvalidInput
		}
		if in.ID == "" {
			in.ID = uuid.NewString()
		}
		if in.ProviderID == "" {
			in.ProviderID = uuid.NewString()
		}
		out, e := cat.SaveCredential(ctx, op.ID, in.ID, in.ProviderID, in.Name, in.Secret, in.Enabled, in.Expected)
		return gin.H{"id": out.ID, "status": "saved", "version": out.Version}, e
	case "routes":
		var in struct {
			Route    catalog.RouteConfiguration `json:"route"`
			Expected uint64                     `json:"expected_version"`
		}
		if json.Unmarshal(raw, &in) != nil || in.Route.PublicModelID == "" {
			return nil, catalog.ErrInvalidInput
		}
		e := cat.SaveRouteConfiguration(ctx, op.ID, in.Route, in.Expected)
		return commandResult(in.Route.PublicModelID, "saved"), e
	case "pricing":
		var in struct {
			Action      string                      `json:"action"`
			Operation   modelruntime.Operation      `json:"operation"`
			Billing     modelruntime.BillingProfile `json:"billing"`
			Price       catalog.PriceVersion        `json:"price"`
			ModelID     string                      `json:"model_id"`
			BuyerID     string                      `json:"buyer_id"`
			Enabled     bool                        `json:"enabled"`
			Expected    uint64                      `json:"expected_version"`
			EffectiveAt time.Time                   `json:"effective_at"`
			Evidence    string                      `json:"evidence"`
		}
		if json.Unmarshal(raw, &in) != nil {
			return nil, catalog.ErrInvalidInput
		}
		switch in.Action {
		case "billing":
			if op.ID != a.Installation.OperatorID || a.Organization.RequirePermission(ctx, tx, actor, op.ID, "catalog.manage") != nil {
				return nil, catalog.ErrCatalogForbidden
			}
			if !in.Billing.Valid(in.Operation) {
				return nil, catalog.ErrInvalidInput
			}
			m, e := cat.PublicModel(ctx, in.ModelID)
			if e != nil {
				return nil, e
			}
			if m.Version != in.Expected {
				return nil, catalog.ErrCatalogVersion
			}
			prepared, e := a.preparedPricingModel(ctx, m)
			if e != nil {
				return nil, e
			}
			m = prepared
			found := false
			for _, cap := range m.Capabilities {
				if cap.Operation == in.Operation {
					found = true
				}
			}
			if !found {
				return nil, catalog.ErrInvalidInput
			}
			if m.Billing == nil {
				m.Billing = map[modelruntime.Operation]modelruntime.BillingProfile{}
			}
			m.Billing[in.Operation] = in.Billing
			if in.EffectiveAt.IsZero() {
				e = cat.SavePublicModel(ctx, op.ID, m, in.Expected)
				if e == nil || !errors.Is(e, catalog.ErrChangeNotice) {
					return commandResult(m.ID, "saved"), e
				}
			}
			out, e := controller.SchedulePublicModel(ctx, op.ID, m, in.Expected, in.EffectiveAt, in.Evidence)
			return commandResult(out.ID, out.State), e
		case "listing":
			if in.Expected == 0 && in.Enabled && in.EffectiveAt.IsZero() {
				e := cat.SetListing(ctx, op.ID, in.ModelID, true, 0)
				if e == nil || !errors.Is(e, catalog.ErrChangeNotice) {
					return commandResult(in.ModelID, "configured"), e
				}
			}
			out, e := controller.ScheduleListing(ctx, op.ID, in.ModelID, in.Enabled, in.Expected, in.EffectiveAt, in.Evidence)
			return commandResult(out.ID, out.State), e
		case "authorization":
			if in.Expected == 0 && in.Enabled && in.EffectiveAt.IsZero() {
				e := cat.SetAuthorization(ctx, op.ID, in.BuyerID, in.ModelID, true, 0)
				return commandResult(in.ModelID, "configured"), e
			}
			out, e := controller.ScheduleAuthorization(ctx, op.ID, in.BuyerID, in.ModelID, in.Enabled, in.Expected, in.EffectiveAt, in.Evidence)
			return commandResult(out.ID, out.State), e
		case "price", "bootstrap_price":
			if in.Price.Scope.SellerID == "" {
				in.Price.Scope.SellerID = op.ID
			}
			if in.Price.Scope.SellerID != op.ID || in.Price.Scope.Kind == "supplier" {
				return nil, catalog.ErrCatalogForbidden
			}
			if in.Price.ID == "" {
				in.Price.ID = uuid.NewString()
			}
			if in.Price.Rule.Mode == "follow" {
				if op.ParentOperatorID == nil {
					return nil, catalog.ErrCatalogForbidden
				}
				in.Price.Rule.Follow = &catalog.PriceScope{Kind: "supply", SellerID: *op.ParentOperatorID, BuyerID: op.ID}
			}
			automaticTime := in.Price.EffectiveAt.IsZero()
			if automaticTime {
				in.Price.EffectiveAt = time.Now().UTC()
			}
			in.Price.Published = true
			in.Price.NoticeID = ""
			// One save operation: first prices publish immediately, revisions are
			// automatically announced. The caller never manages notice records.
			e := cat.PublishPrice(ctx, op.ID, in.Price, a.pricePublished)
			if e == nil || !errors.Is(e, catalog.ErrChangeNotice) {
				return commandResult(in.Price.ID, "published"), e
			}
			if automaticTime {
				in.Price.EffectiveAt = time.Time{}
			}
			out, e := controller.SchedulePrice(ctx, op.ID, in.Price, in.Evidence)
			return commandResult(out.ID, out.State), e
		}
	}
	return nil, catalog.ErrInvalidInput
}
func (a *Application) Check(ctx context.Context, tx *gorm.DB, op organization.Operator, feature organization.BusinessFeature) ([]organization.Requirement, error) {
	requirements := []organization.Requirement{}
	add := func(code, description string, ready bool) {
		requirements = append(requirements, organization.Requirement{Code: code, Description: description, Ready: ready})
	}
	policy, e := a.Incentives.CurrentPolicy(ctx, tx, op.ID)
	add("incentives_policy", "已发布资格、分佣与注册奖励规则", e == nil)
	if feature == organization.Registration {
		add("verification_mail", "可发送验证邮件", a.Mail.Available())
		if e == nil && policy.InviteeATG.Add(policy.InviterATG).Sign() > 0 {
			coverage, err := a.Cooperation.Coverage(ctx, tx, op.ID, time.Now())
			add("reward_coverage", "奖励有真实覆盖承诺空间", err == nil && (!coverage.HasUpstream || coverage.Space.Sign() > 0))
		}
		return requirements, nil
	}
	if feature == organization.NewPurchase || feature == organization.Renewal {
		pkgs, err := a.Commerce.ListPackages(ctx, tx, op.ID)
		enabled := false
		for _, p := range pkgs {
			enabled = enabled || p.NewPurchaseEnabled || p.RenewalEnabled
		}
		add("package", "已发布可售套餐", err == nil && enabled)
		methods, err := a.Payments.PublicMethods(ctx, op.ID)
		add("payment", "经营方真实收款材料已配置", err == nil && len(methods) > 0)
	}
	if feature == organization.ModelUsage || feature == organization.NewPurchase || feature == organization.Renewal || feature == organization.Activity {
		models, err := a.Catalog.PublicCatalog(ctx, op.ID, time.Now())
		ready := false
		for _, m := range models {
			ready = ready || m.Status == "available"
		}
		add("catalog_pricing", "公开模型、完整价格及经过验证的上游能力", err == nil && ready)
		coverage, err := a.Cooperation.Coverage(ctx, tx, op.ID, time.Now())
		add("coverage", "采购与已售责任覆盖可用", err == nil && (!coverage.HasUpstream || coverage.Space.Sign() >= 0))
	}
	return requirements, nil
}
func (a *Application) domainRequest(c *gin.Context, op organization.Operator, actor string) {
	var in struct {
		Action           string `json:"action"`
		Hostname         string
		TargetOperatorID string `json:"target_operator_id"`
		ClientID         string `json:"client_id"`
		ClientSecret     string `json:"client_secret"`
		Enabled          bool   `json:"enabled"`
		ExpectedVersion  int64  `json:"config_version"`
	}
	if !body(c, &in) {
		return
	}
	if in.Action == "google" || in.Action == "google_test" {
		a.googleBrandCommand(c, op, actor, in.Action, in.TargetOperatorID, in.Hostname, in.ClientID, in.ClientSecret, in.Enabled, in.ExpectedVersion)
		return
	}
	target, e := a.brandingTarget(c, op, in.TargetOperatorID)
	if e != nil {
		handleError(c, e)
		return
	}
	if target.Kind == organization.Channel {
		handleError(c, organization.ErrForbidden)
		return
	}
	raw, e := a.command(c, "domain:"+target.BrandID, in, func(tx *gorm.DB) (any, error) {
		out, e := a.Branding.RequestDomain(c.Request.Context(), tx, actor, target.BrandID, in.Hostname)
		return gin.H{"id": out.Domain.ID, "status": out.Domain.Status, "message": "请设置 TXT " + out.TXTName + " = " + out.TXTValue + " 并确保有效 TLS"}, e
	})
	jsonResponse(c, raw, e)
}
