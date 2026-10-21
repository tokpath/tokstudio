package portal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
			return gin.H{"id": out.PackageID, "status": "published", "version": out.Version}, e
		case "payments":
			var in commerce.VerifiedReceipt
			if e := json.Unmarshal(config, &in); e != nil {
				return nil, commerce.ErrInvalidInput
			}
			in.OperatorID = op.ID
			out, e := a.Commerce.RecordManualReceipt(ctx, tx, actor, in)
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
				Kind                                          organization.OperatorKind `json:"kind"`
				Name, LegalName, BrandName                    string
				SupplyTerms, ExitPolicy, ContractAttachmentID string
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
			_ = json.Unmarshal(fields["exit_policy"], &in.ExitPolicy)
			_ = json.Unmarshal(fields["contract_attachment_id"], &in.ContractAttachmentID)
			out, e := a.Organization.CreateOperator(ctx, tx, organization.CreateOperatorInput{ActorStaffID: actor, ParentOperatorID: op.ID, Kind: in.Kind, Name: in.Name, LegalName: in.LegalName, BrandName: in.BrandName, SupplyTerms: in.SupplyTerms, ExitPolicy: in.ExitPolicy, ContractAttachmentID: in.ContractAttachmentID})
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
				ConfirmationDays   *int64 `json:"confirmation_days"`
				RewardValidityDays *int64 `json:"reward_validity_days"`
			}
			if json.Unmarshal(config, &in) != nil || in.ConfirmationDays == nil || in.RewardValidityDays == nil {
				return nil, incentives.ErrInvalidInput
			}
			confirmation, ok := policySecondsFromDays(*in.ConfirmationDays, 0, 365)
			reward, rewardOK := policySecondsFromDays(*in.RewardValidityDays, 1, 3650)
			if !ok || !rewardOK {
				return nil, incentives.ErrInvalidInput
			}
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
			out, e := a.Organization.SetBusinessStatus(ctx, tx, organization.SetBusinessStatusInput{ActorStaffID: actor, OperatorID: op.ID, Feature: in.Feature, Enabled: in.Enabled, ExpectedVersion: in.ExpectedVersion, EvidenceReference: in.Evidence}, a)
			return gin.H{"id": op.ID, "status": "updated", "version": out.Version}, e
		case "catalog", "providers", "routes", "pricing", "catalog-changes":
			return a.catalogCommand(ctx, tx, op, actor, resource, config)
		}
		return nil, commerce.ErrNotReady
	})
	jsonResponse(c, raw, e)
}
func (a *Application) operatorRowCommand(c *gin.Context) {
	resource := c.Param("resource")
	op, actor, ok := a.staffScope(c, resource)
	if !ok {
		return
	}
	ctx := c.Request.Context()
	action := c.Param("action")
	if resource == "receipt-valuations" && action == "confirm" {
		a.confirmReceiptValuation(c, op, actor)
		return
	}
	if resource == "routes" && action == "limits" {
		a.routeLimitsAction(c, op, actor)
		return
	}
	if resource == "routing" && action == "policy" {
		a.routingAction(c, op, actor)
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
	if resource == "takeovers" || resource == "takeover-settlements" {
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
	if resource == "retention" && action == "hold" {
		a.holdTemporary(c, op, actor)
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
			OrderID string         `json:"order_id"`
			Amount  ledger.Decimal `json:"amount_usd"`
		}
		if !body(c, &in) {
			return
		}
		businessKey, _ := key(c)
		raw, e := a.command(c, "allocate:"+op.ID, in, func(tx *gorm.DB) (any, error) {
			out, e := a.Commerce.AllocateReceipt(ctx, tx, commerce.AllocateReceiptInput{ActorStaffID: actor, OperatorID: op.ID, BusinessKey: businessKey, ReceiptID: c.Param("id"), OrderID: in.OrderID, AmountUSD: in.Amount})
			return commandResult(out.ID, "allocated"), e
		})
		jsonResponse(c, raw, e)
	default:
		fail(c, 404, "not_found", "经营命令不存在", false)
	}
}
func (a *Application) catalogResource(c *gin.Context, op organization.Operator, actor, resource string, p gin.H, base string) bool {
	ctx := c.Request.Context()
	items := []any{}
	switch resource {
	case "catalog":
		rows, e := a.Catalog.AdminModels(ctx, a.Installation.OperatorID)
		if e != nil {
			handleError(c, e)
			return true
		}
		for _, row := range rows {
			items = append(items, entity(row.ID, "configured", int64(row.Version), row))
		}
		p["columns"] = []gin.H{col("id", "公开 ID", "text"), col("display_name", "名称", "text"), col("identity", "事实身份", "text"), col("capabilities", "验证能力", "text"), col("enabled", "启用", "boolean")}
		p["operations"] = []operation{{Name: "save", Label: "维护公开模型与能力证据", Method: "POST", Path: base, Fields: []field{jsonField("model 对象：id、identity、display_name、enabled、capabilities（必须有真实参数/用量/上界/恢复证据）、expected_version")}}}
	case "providers":
		rows, e := a.Catalog.AdminCredentials(ctx, a.Installation.OperatorID)
		if e != nil {
			handleError(c, e)
			return true
		}
		for _, row := range rows {
			items = append(items, entity(row.ID, "configured", int64(row.Version), row))
		}
		p["description"] = "供应商凭据经加密保存，只返回凭据引用与版本。"
		p["columns"] = []gin.H{col("name", "凭据名称", "text"), col("id", "凭据引用", "text"), col("provider_id", "供应商", "text"), col("enabled", "启用", "boolean")}
		name := f("name", "凭据名称", "text")
		name.Help = "给这把密钥起一个便于辨认的名称，例如「OpenAI 生产」。之后可以只改名称，不必重新填写密钥。"
		secret := optional("secret", "新的供应商密钥（只更换时填写）", "password")
		secret.Help = "首次创建必须填写。以后只修改名称或启用状态时留空，已保存的密钥保持不变。"
		p["operations"] = []operation{{Name: "credential", Label: "创建或轮换凭据", Method: "POST", Path: base, Fields: []field{name, optional("id", "现有凭据引用（首次留空）", "text"), optional("provider_id", "现有供应商引用（首次留空）", "text"), secret, f("enabled", "启用", "boolean"), f("expected_version", "当前版本（首次0）", "integer")}}}
	case "routes":
		models, e := a.Catalog.AdminModels(ctx, a.Installation.OperatorID)
		if e != nil {
			handleError(c, e)
			return true
		}
		for _, m := range models {
			rows, e := a.Catalog.AdminUpstreams(ctx, a.Installation.OperatorID, m.ID)
			if e != nil {
				handleError(c, e)
				return true
			}
			for _, row := range rows {
				items = append(items, entity(row.ID, "configured", int64(row.Version), row))
			}
		}
		p["columns"] = []gin.H{col("public_model_id", "公开模型", "text"), col("provider", "协议", "text"), col("model", "上游模型", "text"), col("credential_id", "凭据引用", "text"), col("priority", "优先级", "text"), col("healthy", "健康", "boolean")}
		p["row_operations"] = []operation{{Name: "limits", Label: "设置并发、熔断与恢复", Method: "POST", Path: base + "/{id}/limits", IncludeVersion: true, Fields: []field{defaultField("public_model_id", "公开模型", "hidden", ""), f("routing.max_concurrent", "此上游最大同时在途（0不限）", "integer"), f("routing.failure_threshold", "连续失败熔断阈值（0用默认3）", "integer"), f("routing.unknown_usage_threshold", "未知用量熔断阈值（0用默认2）", "integer"), f("routing.recovery_successes", "恢复所需成功探测次数（0用默认2）", "integer"), f("routing.cooldown_seconds", "熔断冷却秒数（0用默认60）", "integer")}, Description: "保留原供应商模型与能力证据。真实占用、故障、探测与恢复状态在动态选路页面查看。"}}
		p["operations"] = []operation{{Name: "save", Label: "维护已验证路由", Method: "POST", Path: base, Fields: []field{jsonField("upstream 对象 id、public_model_id、identity、provider_id、provider、model、credential_id、endpoints、capabilities、enabled、healthy、priority、expected_version。视频还必须配置 endpoints.video_retrieve 为 https://供应商/v1/videos/{task_id}、video_download 为 https://供应商/v1/videos/{task_id}/content；填路径模板而非具体任务ID。能力证据不足不能开放。")}}}
	case "catalog-changes":
		rows, e := a.Catalog.CatalogChanges(ctx, op.ID)
		if e != nil {
			handleError(c, e)
			return true
		}
		for _, row := range rows {
			items = append(items, entity(row.ID, row.State, int64(row.ExpectedVersion), row))
		}
		p["columns"] = []gin.H{col("kind", "变更类型", "text"), col("resource_id", "模型/资源", "text"), col("effective_at", "计划生效", "datetime"), col("announced_at", "实际通知", "datetime"), col("applied_at", "实际应用", "datetime"), col("error_code", "待复核原因", "text")}
		policy, e := a.Catalog.NoticePolicy(ctx, op.ID)
		if e != nil && !errors.Is(e, gorm.ErrRecordNotFound) {
			handleError(c, e)
			return true
		}
		p["summary"] = []gin.H{{"label": "通知提前秒数", "value": fmt.Sprint(policy.LeadSeconds)}, {"label": "规则版本", "value": fmt.Sprint(policy.Version)}, {"label": "通知规则依据", "value": policy.RulesReference}}
		p["operations"] = []operation{{Name: "policy", Label: "设置提前通知规则", Method: "POST", Path: base, Fields: []field{defaultField("lead_seconds", "提前通知秒数（至少60）", "integer", policy.LeadSeconds), defaultField("rules_reference", "对客户公开的通知条款依据", "text", policy.RulesReference), defaultField("expected_version", "当前规则版本（自动带入）", "integer", policy.Version)}}}
	case "pricing":
		models, e := a.Catalog.PublicCatalog(ctx, op.ID, time.Now())
		if e != nil {
			handleError(c, e)
			return true
		}
		for _, m := range models {
			items = append(items, entity(m.ID, m.Status, 1, m))
		}
		p["columns"] = []gin.H{col("name", "模型", "text"), col("prices", "精确组件价格", "text"), col("status", "状态", "status")}
		p["operations"] = []operation{{Name: "price", Label: "发布价格版本", Method: "POST", Path: base, Fields: []field{jsonField("action:'price'、price:{id,scope:{kind,seller_id,buyer_id?},model_id,layer,effective_at,ends_at?,rule,notice_id,published:true}。调价后将重算已售责任覆盖。")}}, {Name: "listing", Label: "授权或上架模型", Method: "POST", Path: base, Fields: []field{jsonField("action:'listing' 或 'authorization'、model_id、buyer_id（授权时）、enabled、expected_version")}}}
	default:
		return false
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
			Upstream catalog.UpstreamModel `json:"upstream"`
			Expected uint64                `json:"expected_version"`
		}
		if json.Unmarshal(raw, &in) != nil {
			return nil, catalog.ErrInvalidInput
		}
		e := cat.SaveUpstream(ctx, op.ID, in.Upstream, in.Expected)
		return commandResult(in.Upstream.ID, "saved"), e
	case "catalog-changes":
		var in struct {
			LeadSeconds    int64  `json:"lead_seconds"`
			RulesReference string `json:"rules_reference"`
			Expected       uint64 `json:"expected_version"`
		}
		if json.Unmarshal(raw, &in) != nil {
			return nil, catalog.ErrInvalidInput
		}
		out, e := cat.SaveNoticePolicy(ctx, op.ID, catalog.NoticePolicy{OperatorID: op.ID, LeadSeconds: in.LeadSeconds, RulesReference: in.RulesReference}, in.Expected)
		return gin.H{"id": op.ID, "status": "saved", "version": out.Version}, e
	case "pricing":
		var in struct {
			Action      string               `json:"action"`
			Price       catalog.PriceVersion `json:"price"`
			ModelID     string               `json:"model_id"`
			BuyerID     string               `json:"buyer_id"`
			Enabled     bool                 `json:"enabled"`
			Expected    uint64               `json:"expected_version"`
			EffectiveAt time.Time            `json:"effective_at"`
			Evidence    string               `json:"evidence"`
		}
		if json.Unmarshal(raw, &in) != nil {
			return nil, catalog.ErrInvalidInput
		}
		switch in.Action {
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
			if in.Price.Scope.SellerID != op.ID && !(op.Kind == organization.Platform && in.Price.Scope.Kind == "supplier") {
				return nil, catalog.ErrCatalogForbidden
			}
			if in.Action == "bootstrap_price" || in.Price.Scope.Kind == "supplier" {
				e := cat.PublishPrice(ctx, op.ID, in.Price, a.pricePublished)
				return commandResult(in.Price.ID, "published"), e
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
