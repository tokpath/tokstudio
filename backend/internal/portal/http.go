package portal

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
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
	"github.com/tokpath/tokstudio/backend/internal/modelpricing"
	"github.com/tokpath/tokstudio/backend/internal/modelruntime"
	"github.com/tokpath/tokstudio/backend/internal/operations"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"github.com/tokpath/tokstudio/backend/internal/payments"
	pc "github.com/tokpath/tokstudio/backend/internal/platform/crypto"
	"github.com/tokpath/tokstudio/backend/internal/platform/db"
	"github.com/tokpath/tokstudio/backend/internal/platform/httpx"
	"github.com/tokpath/tokstudio/backend/internal/platform/mail"
	"github.com/tokpath/tokstudio/backend/internal/platform/objectstore"
	"github.com/tokpath/tokstudio/backend/internal/remediation"
	"github.com/tokpath/tokstudio/backend/internal/usage"
	"gorm.io/gorm"
)

type browserSession struct {
	TokenHash, Kind, BrandID, SubjectID string
	CredentialVersion                   int64
	ExpiresAt, CreatedAt                time.Time
}

func (browserSession) TableName() string { return "portal_sessions" }

type identity struct {
	Session  browserSession
	Token    string
	Customer organization.Principal
	Staff    Staff
}

func fail(c *gin.Context, status int, code, message string, retry bool) {
	message = httpx.LocalizedErrorMessage(httpx.RequestLocale(c.Request), code, message)
	c.AbortWithStatusJSON(status, gin.H{"error": gin.H{"code": code, "message": message, "retryable": retry, "request_id": c.GetString("request_id")}})
}
func handleError(c *gin.Context, err error) {
	if err == nil {
		return
	}
	var readiness *organization.ReadinessError
	if errors.As(err, &readiness) {
		messages := []string{}
		for _, r := range readiness.Missing {
			messages = append(messages, r.Description)
		}
		fail(c, 409, "unmet_requirements", "仍有未满足事项："+strings.Join(messages, "；"), false)
		return
	}
	status, code, msg, retry := 500, "internal_error", "处理失败，请沿用本次操作编号重试", true
	switch {
	case errors.Is(err, mail.ErrSettings):
		status, code, msg, retry = 400, "invalid_mail_settings", "邮件配置不完整或格式不正确，请核对 SMTP 主机、端口、发信地址及认证信息。", false
	case errors.Is(err, mail.ErrUnavailable):
		status, code, msg, retry = 409, "mail_unavailable", "邮件服务尚未就绪，请由当前品牌负责人检查配置；使用默认服务时请等待平台配置。", false
	case errors.Is(err, ledger.ErrMonetaryLocked):
		status, code, msg, retry = 409, "monetary_locked", "清算币种和 ATG 基础比例已正式生效，永久锁定；暂停营业也不能修改。", false
	case errors.Is(err, ledger.ErrMonetaryNotReady):
		status, code, msg, retry = 409, "monetary_not_ready", "全系统清算币种和 ATG 基础比例尚未正式生效，请由平台负责人完成营业前设置。", false
	case errors.Is(err, branding.ErrNotFound), errors.Is(err, gorm.ErrRecordNotFound), errors.Is(err, organization.ErrNotFound), errors.Is(err, commerce.ErrNotFound):
		status, code, msg, retry = 404, "not_found", "记录不存在或不属于当前范围", false
	case errors.Is(err, catalog.ErrResolutionPrice):
		status, code, msg, retry = 403, "resolution_owner", "只有平台经营方可以为图片或视频新增分辨率，其它经营方只能为已有分辨率标价。", false
	case errors.Is(err, remediation.ErrScope), errors.Is(err, operations.ErrForbidden), errors.Is(err, payments.ErrForbidden), errors.Is(err, catalog.ErrCatalogForbidden), errors.Is(err, incentives.ErrNotEligible), errors.Is(err, organization.ErrForbidden), errors.Is(err, commerce.ErrScope), errors.Is(err, ledger.ErrScope):
		status, code, msg, retry = 403, "forbidden", "当前身份没有此经营范围的操作权限", false
	case errors.Is(err, errStaffPasswordLength):
		status, code, msg, retry = 400, "password_length", "初始密码长度需为 10 至 72 字节，建议使用英文字母、数字和符号", false
	case errors.Is(err, organization.ErrInvalidCredentials):
		status, code, msg, retry = 401, "invalid_credentials", "邮箱或密码错误", false
	case errors.Is(err, organization.ErrEmailAlreadyRegistered):
		status, code, msg, retry = 409, "email_already_registered", "此邮箱已在当前品牌注册，请登录原账号；邀请码不能改变已有归属", false
	case errors.Is(err, organization.ErrInvalidInviteCode):
		status, code, msg, retry = 400, "invalid_invite_code", "邀请码无效，请更正或显式移除；不会自动更改经营归属", false
	case errors.Is(err, organization.ErrInvalidVerification):
		status, code, msg, retry = 400, "invalid_verification", "验证凭据无效或已过期，请重新发起验证", false
	case errors.Is(err, modelpricing.ErrInput), errors.Is(err, ledger.ErrAmount), errors.Is(err, cooperation.ErrExitTerms), errors.Is(err, branding.ErrInvalidInput), errors.Is(err, remediation.ErrInvalid), errors.Is(err, operations.ErrInvalid), errors.Is(err, incentives.ErrInvalidInput), errors.Is(err, catalog.ErrInvalidInput), errors.Is(err, payments.ErrInvalid), errors.Is(err, organization.ErrInvalidInput), errors.Is(err, organization.ErrInvalidRelationship), errors.Is(err, commerce.ErrInvalidInput), errors.Is(err, modelruntime.ErrInvalidRequest), errors.Is(err, usage.ErrKey):
		status, code, msg, retry = 400, "invalid_input", "输入不完整、不符合限制或身份凭据不可用", false
	case errors.Is(err, usage.ErrSecretDisplayed):
		status, code, msg, retry = 409, "secret_already_displayed", "此 Key 操作已完成；完整密钥只显示一次，请从列表重置以获得新密钥", false
	case errors.Is(err, modelpricing.ErrVersion), errors.Is(err, cooperation.ErrConflict), errors.Is(err, organization.ErrConflict), errors.Is(err, remediation.ErrConflict), errors.Is(err, ledger.ErrConflict), errors.Is(err, catalog.ErrCatalogVersion), errors.Is(err, incentives.ErrConflict), errors.Is(err, payments.ErrConflict), errors.Is(err, operations.ErrConflict), errors.Is(err, commerce.ErrConflict), errors.Is(err, usage.ErrConflict), errors.Is(err, branding.ErrConflict):
		status, code, msg, retry = 409, "version_conflict", "内容已变化或操作编号已被其它内容使用，请刷新后重新确认", false
	case errors.Is(err, usage.ErrBudget):
		status, code, msg, retry = 429, "request_limit", "API Key 预算已用尽", false
	case errors.Is(err, usage.ErrRate):
		status, code, msg, retry = 429, "request_limit", "API Key 请求频率已达到上限", false
	case errors.Is(err, usage.ErrConcurrency):
		status, code, msg, retry = 429, "request_limit", "API Key 同时请求数已达到上限", false
	case errors.Is(err, usage.ErrInProgress):
		status, code, msg, retry = 409, "in_progress", "原请求仍在处理中，请查询原请求状态", true
	case errors.Is(err, remediation.ErrTakeoverBlocked):
		status, code, msg, retry = 409, "takeover_awaits_drain", "接管正在等待已受理请求、订单、退款和补货排空，请查看原事项后刷新快照", true
	case errors.Is(err, catalog.ErrRouteUnavailable):
		status, code, msg, retry = 503, "route_unavailable", "已验证候选暂不可用或达到容量，请稍后重试；未扩大候选或更改价格", true
	case errors.Is(err, objectstore.ErrUnavailable):
		status, code, msg, retry = 503, "object_store_unavailable", "私有对象存储暂不可用，请保留当前事项稍后重试", true
	case errors.Is(err, usage.ErrAdmissionUnavailable):
		status, code, msg, retry = 503, "admission_unavailable", "请求限流依赖暂不可用，已受理事项继续结算，请稍后重试新请求", true
	case errors.Is(err, usage.ErrResultUnavailable):
		status, code, msg, retry = 410, "result_unavailable", "临时结果已到期或暂不可回读，请查看保留状态", false
	case errors.Is(err, catalog.ErrEffectiveTimeRequired):
		status, code, msg, retry = 400, "effective_time_required", "请填写生效时间。为预留系统通知时间，最好比当前时间晚五分钟，系统不强制。", false
	case errors.Is(err, catalog.ErrChangeNotice):
		status, code, msg, retry = 409, "advance_notice_required", "系统暂时无法安排变更通知，请稍后重试；当前有效配置不变。", false
	case errors.Is(err, catalog.ErrPublicRoute):
		status, code, msg, retry = 409, "public_route_required", "还没有可用的路由，不能对客户开放这个模型", false
	case errors.Is(err, catalog.ErrPublicCapability):
		status, code, msg, retry = 409, "public_capability_required", "请先勾选至少一项能力，再开放这个模型", false
	case errors.Is(err, catalog.ErrPriceUnavailable), errors.Is(err, catalog.ErrUnboundedCoverage):
		status, code, msg, retry = 409, "pending_configuration", "请先补齐模型及各层有效价格，系统才能核实采购与履约责任。", false
	case errors.Is(err, remediation.ErrNotReady), errors.Is(err, operations.ErrNotReady), errors.Is(err, incentives.ErrNotReady), errors.Is(err, modelruntime.ErrUnsupported), errors.Is(err, commerce.ErrNotReady), errors.Is(err, payments.ErrNotConfigured):
		status, code, msg, retry = 409, "pending_configuration", "相关能力尚未完成配置或验证，请联系当前经营方", false
	case errors.Is(err, remediation.ErrPaymentMismatch):
		status, code, msg, retry = 409, "refund_execution_mismatch", "实际退款事实已保留；金额或收款方不符，剩余责任与资金占用须在原记录复核", false
	case errors.Is(err, payments.ErrUnsupported):
		status, code, msg, retry = 400, "payment_currency_unsupported", "当前付款通道不支持本订单清算币种或操作，请选择其它方式。", false
	case errors.Is(err, payments.ErrAwaitingSettlement):
		status, code, msg, retry = 409, "receipt_awaiting_settlement", "原币实际款已保留，等待财务核实本笔收款对应的订单清算金额；请勿重复付款", false
	case errors.Is(err, remediation.ErrReview):
		status, code, msg, retry = 409, "independent_review_required", "此事项需要另一位有权限的人员独立复核", false
	case errors.Is(err, commerce.ErrWholePayment):
		status, code, msg, retry = 409, "whole_payment_required", "一个订单只关联一笔足额外部付款；金额不符的款项请单独登记并按异常处理", false
	case errors.Is(err, commerce.ErrExcessAllocation):
		status, code, msg, retry = 409, "receipt_amount_conflict", "收款金额不足、已被使用或超出订单应付，请核对原收款和订单", false
	case errors.Is(err, cooperation.ErrCoverage):
		status, code, msg, retry = 409, "sales_coverage_insufficient", "供货方或当前经营方剩余销售空间不足以承接这笔订单。请核对已有权益、未付款订单占用和超卖额度，补货或释放占用后重试。", false
	case errors.Is(err, cooperation.ErrLiability), errors.Is(err, remediation.ErrState), errors.Is(err, commerce.ErrOrderState), errors.Is(err, commerce.ErrNotPaid), errors.Is(err, incentives.ErrWithdrawalState):
		status, code, msg, retry = 409, "state_restricted", "当前业务状态或资金条件不允许此操作，请查询原记录", false
	case errors.Is(err, ledger.ErrInsufficient):
		status, code, msg, retry = 409, "insufficient_asset", "可用资产不足；占用与各类资产不能互相替代", false
	case errors.Is(err, incentives.ErrLimit):
		status, code, msg, retry = 409, "activity_limit", "活动名额或客户使用次数已达上限", false
	case errors.Is(err, payments.ErrSettlementReview):
		status, code, msg, retry = 409, "refund_valuation_mismatch", "真实退款事实已保留，本次原币付款或清偿金额差异需要核实；未清偿责任仍保留占用", false
	case errors.Is(err, commerce.ErrReceiptRestricted):
		status, code, msg, retry = 409, "receipt_restricted", "原收款存在限制，真实收款事实已保留，不能继续新分配或重复支付", false
	case errors.Is(err, payments.ErrOutcomeUnknown):
		status, code, msg, retry = 409, "payment_outcome_unknown", "原支付结果尚待核实，请查询原交易，重试将沿用原业务编号", true
	case errors.Is(err, commerce.ErrPaymentUnknown):
		status, code, msg, retry = 409, "original_payment_pending", "原订单付款尚待核实，本次操作未执行；请先查询原交易，避免重复付款", false
	case errors.Is(err, organization.ErrFrozen):
		status, code, msg, retry = 403, "action_frozen", "当前范围的此项操作已冻结，请联系经营方核实", false
	case errors.Is(err, organization.ErrBusinessClosed):
		status, code, msg, retry = 409, "business_restricted", "当前经营方尚未开放此项业务", false
	case errors.Is(err, organization.ErrGoogleUnavailable):
		status, code, msg, retry = 409, "google_unavailable", "当前品牌域名尚未启用 Google 登录", false
	case errors.Is(err, organization.ErrGoogleNotVerified):
		status, code, msg, retry = 409, "google_not_verified", "请先在品牌域名上完成 Google 回调测试，再启用", false
	case errors.Is(err, organization.ErrGoogleNeedsPassword):
		status, code, msg, retry = 409, "google_email_exists", "这个邮箱已在本品牌用密码注册。请使用密码登录；Google 不会自动并入已有账号", false
	case errors.Is(err, organization.ErrGoogleState):
		status, code, msg, retry = 409, "google_state", "Google 登录已过期或已使用，请从登录页重新开始", false
	case errors.Is(err, organization.ErrGoogleClientTaken):
		status, code, msg, retry = 409, "google_client_taken", "这个 Google 客户端已经配置给其他品牌", false
	case errors.Is(err, organization.ErrGoogleConfig):
		status, code, msg, retry = 400, "google_config", "请填写以 .apps.googleusercontent.com 结尾的客户端编号；首次配置必须填写密钥", false
	}
	fail(c, status, code, msg, retry)
}
func body(c *gin.Context, v any) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<20)
	dec := json.NewDecoder(c.Request.Body)
	dec.DisallowUnknownFields()
	dec.UseNumber()
	if err := dec.Decode(v); err != nil {
		fail(c, 400, "invalid_input", "请求内容格式错误", false)
		return false
	}
	var extra any
	if dec.Decode(&extra) != io.EOF {
		fail(c, 400, "invalid_input", "请求只能包含一个 JSON 对象", false)
		return false
	}
	return true
}
func key(c *gin.Context) (string, bool) {
	v := c.GetHeader("Idempotency-Key")
	if _, err := uuid.Parse(v); err != nil {
		fail(c, 400, "idempotency_required", "本次操作必须携带 UUID 格式的 Idempotency-Key", false)
		return "", false
	}
	return v, true
}
func commandResult(id, status string) gin.H { return gin.H{"id": id, "status": status} }
func subject(c *gin.Context) *identity {
	v, _ := c.Get("identity")
	if v == nil {
		return nil
	}
	return v.(*identity)
}
func (a *Application) Router() *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery(), a.Metrics.HTTP())
	_ = r.SetTrustedProxies(a.Config.TrustedProxies)
	r.Use(func(c *gin.Context) {
		if c.GetString("request_id") == "" {
			c.Set("request_id", uuid.NewString())
		}
		c.Header("X-Request-ID", c.GetString("request_id"))
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("Cache-Control", "no-store")
		c.Next()
	})
	r.GET("/metrics", a.metricsAccess, a.Metrics.Handler())
	r.GET("/healthz", func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok"}) })
	r.GET("/readyz", func(c *gin.Context) {
		if err := db.Readiness(c.Request.Context(), a.DB, AllMigrations()); err != nil {
			fail(c, 503, "schema_not_ready", "数据库版本未就绪，请执行显式迁移", true)
			return
		}
		if a.Redis == nil || a.Redis.Ping(c.Request.Context()).Err() != nil {
			fail(c, 503, "redis_not_ready", "请求限流依赖尚未配置或不可用", true)
			return
		}
		if a.ObjectStore == nil || a.ObjectStore.Ready(c.Request.Context()) != nil {
			fail(c, 503, "object_store_not_ready", "私有对象存储尚未配置或不可用", true)
			return
		}
		c.JSON(200, gin.H{"status": "ready", "dependencies": gin.H{"postgres": "ready", "object_store": "ready", "redis": "ready"}})
	})
	r.GET("/v1/public/tls-check", a.tlsAsk)
	v := r.Group("/v1")
	v.Use(a.resolveBrand)
	v.GET("/branding/assets/:id/content", a.publicLogo)
	v.GET("/public/context", a.loadIdentity(false), a.publicContext)
	v.GET("/monetary/settings", a.monetarySettings)
	v.POST("/monetary/settings", a.loadIdentity(true), a.requireStaff, a.requireCSRF, a.monetaryCommand)
	v.POST("/monetary/cancel", a.loadIdentity(true), a.requireStaff, a.requireCSRF, a.monetaryCommand)
	v.GET("/public/models", a.loadIdentity(false), a.models)
	v.GET("/public/plans", a.loadIdentity(false), a.plans)
	v.GET("/public/payment-assets", a.publicPaymentAssets)
	v.GET("/auth/me", a.loadIdentity(false), a.sessionResponse)
	v.POST("/auth/staff-invitations/accept", a.originGuard, a.acceptStaff)
	v.POST("/auth/login", a.originGuard, a.login)
	v.POST("/auth/google/start", a.originGuard, a.googleStart)
	v.POST("/auth/google/test", a.originGuard, a.googleTestStart)
	v.POST("/auth/google/finish", a.originGuard, a.googleFinish)
	v.POST("/auth/registrations", a.originGuard, a.monetaryTransactionGuard, a.register)
	v.POST("/auth/verify", a.originGuard, a.monetaryTransactionGuard, a.verify)
	v.POST("/auth/password-resets", a.originGuard, a.passwordReset)
	v.POST("/auth/password-resets/complete", a.originGuard, a.passwordResetComplete)
	v.POST("/auth/staff-password", a.loadIdentity(true), a.requireCSRF, a.changeStaffPassword)
	v.POST("/auth/logout", a.loadIdentity(true), a.requireCSRF, a.logout)
	me := v.Group("/me", a.loadIdentity(true), a.requireCustomer, a.requireCSRF, a.monetaryTransactionGuard)
	a.customerRoutes(me)
	staff := v.Group("/operators/:operatorId", a.loadIdentity(true), a.requireStaff, a.requireCSRF, a.monetaryTransactionGuard)
	a.operatorRoutes(staff)
	a.modelRoutes(v)
	v.POST("/payments/webhooks/kvoucher/:operatorId", a.kvoucherWebhook)
	v.POST("/payments/webhooks/:merchantId", a.paymentWebhook)
	r.NoRoute(func(c *gin.Context) { fail(c, 404, "not_found", "接口不存在", false) })
	return r
}
func (a *Application) requestHost(c *gin.Context) string {
	host := c.Request.Host
	remote, _, _ := net.SplitHostPort(c.Request.RemoteAddr)
	ip := net.ParseIP(remote)
	trusted := false
	for _, p := range a.Config.TrustedProxies {
		if pip := net.ParseIP(p); pip != nil && pip.Equal(ip) {
			trusted = true
		}
		if _, network, e := net.ParseCIDR(p); e == nil && network.Contains(ip) {
			trusted = true
		}
	}
	if trusted {
		if forwarded := c.GetHeader("X-Forwarded-Host"); forwarded != "" && !strings.Contains(forwarded, ",") {
			host = forwarded
		}
	}
	return strings.ToLower(strings.TrimSuffix(host, "."))
}
func bareHost(host string) string {
	if h, _, e := net.SplitHostPort(host); e == nil {
		return h
	}
	return host
}
func (a *Application) resolveBrand(c *gin.Context) {
	host := a.requestHost(c)
	var b organization.Brand
	var err error
	if a.officialHostname(bareHost(host)) {
		b, err = a.Organization.GetBrand(c.Request.Context(), nil, a.Installation.BrandID)
	} else {
		b, err = a.Branding.ResolveHost(c.Request.Context(), nil, host)
	}
	if err != nil || b.Status != "active" {
		fail(c, 404, "unknown_brand", "当前域名没有已验证并启用的品牌", false)
		return
	}
	c.Set("brand", b)
	c.Set("request_host", host)
	c.Next()
}
func brand(c *gin.Context) organization.Brand { v, _ := c.Get("brand"); return v.(organization.Brand) }
func (a *Application) originGuard(c *gin.Context) {
	if origin := c.GetHeader("Origin"); origin != "" {
		u, e := url.Parse(origin)
		if e != nil || !strings.EqualFold(u.Host, a.requestHost(c)) || (u.Scheme != "https" && u.Scheme != "http") {
			fail(c, 403, "origin_mismatch", "请求来源与当前品牌域名不一致", false)
			return
		}
	}
	if c.GetHeader("Sec-Fetch-Site") == "cross-site" {
		fail(c, 403, "origin_mismatch", "不接受跨站身份操作", false)
		return
	}
}
func (a *Application) csrf(token, brandID string) string {
	h := hmac.New(sha256.New, []byte(a.Config.AppKey))
	h.Write([]byte("csrf:" + brandID + ":" + token))
	return hex.EncodeToString(h.Sum(nil))
}
func (a *Application) requireCSRF(c *gin.Context) {
	if c.Request.Method == http.MethodGet || c.Request.Method == http.MethodHead {
		c.Next()
		return
	}
	a.originGuard(c)
	if c.IsAborted() {
		return
	}
	id := subject(c)
	if id == nil || subtle.ConstantTimeCompare([]byte(c.GetHeader("X-CSRF-Token")), []byte(a.csrf(id.Token, id.Session.BrandID))) != 1 {
		fail(c, 403, "csrf_required", "会话校验失败，请刷新后重试", false)
		return
	}
	if _, ok := key(c); !ok {
		return
	}
	c.Next()
}
func (a *Application) loadIdentity(required bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, e := c.Cookie("tokenhub_session")
		if e != nil || token == "" {
			if required {
				fail(c, 401, "unauthenticated", "请先登录", false)
			}
			return
		}
		var session browserSession
		e = a.DB.WithContext(c.Request.Context()).First(&session, "token_hash=? AND brand_id=? AND expires_at>?", pc.HashToken(token), brand(c).ID, time.Now()).Error
		if e != nil {
			if required {
				fail(c, 401, "session_expired", "会话已失效，请重新登录", false)
			}
			return
		}
		id := &identity{Session: session, Token: token}
		if session.Kind == "customer" {
			id.Customer, e = a.Organization.AuthenticateBrandToken(c.Request.Context(), nil, session.BrandID, token)
		} else {
			e = a.DB.WithContext(c.Request.Context()).First(&id.Staff, "id=? AND status='active' AND version=?", session.SubjectID, session.CredentialVersion).Error
		}
		if e != nil {
			if required {
				fail(c, 401, "session_expired", "会话已失效，请重新登录", false)
			}
			return
		}
		c.Set("identity", id)
	}
}
func (a *Application) requireCustomer(c *gin.Context) {
	id := subject(c)
	if id == nil || id.Session.Kind != "customer" {
		fail(c, 403, "customer_required", "此操作需要客户身份", false)
	}
}
func (a *Application) requireStaff(c *gin.Context) {
	id := subject(c)
	if id == nil || id.Session.Kind != "staff" {
		fail(c, 403, "staff_required", "此操作需要独立的经营人员身份", false)
		return
	}
	if id.Staff.MustChangePassword {
		fail(c, 403, "password_change_required", "请先修改初始密码，再进入经营空间", false)
	}
}
func (a *Application) setCookie(c *gin.Context, token string, expires time.Time) {
	secure := a.Config.IsProduction() || c.Request.TLS != nil
	// Forwarded protocol is trusted only when its forwarded host was accepted.
	if a.requestHost(c) != c.Request.Host && c.GetHeader("X-Forwarded-Proto") == "https" {
		secure = true
	}
	http.SetCookie(c.Writer, &http.Cookie{Name: "tokenhub_session", Value: token, Path: "/", HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode, Expires: expires, MaxAge: int(time.Until(expires).Seconds())})
}

type commandRow struct {
	Scope, BusinessKey, Fingerprint string
	Response                        json.RawMessage
	CreatedAt                       time.Time
}

func (commandRow) TableName() string { return "portal_commands" }
func (a *Application) command(c *gin.Context, scope string, input any, fn func(*gorm.DB) (any, error)) (json.RawMessage, error) {
	k, ok := key(c)
	if !ok {
		return nil, errors.New("invalid idempotency key")
	}
	raw, _ := json.Marshal(input)
	mac := hmac.New(sha256.New, []byte(a.Config.AppKey))
	mac.Write(raw)
	fingerprint := hex.EncodeToString(mac.Sum(nil))
	var out json.RawMessage
	err := a.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		if e := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", scope+":"+k).Error; e != nil {
			return e
		}
		var old commandRow
		e := tx.First(&old, "scope=? AND business_key=?", scope, k).Error
		if e == nil {
			if old.Fingerprint != fingerprint {
				return commerce.ErrConflict
			}
			out = old.Response
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		result, e := fn(tx)
		if e != nil {
			return e
		}
		out, e = json.Marshal(result)
		if e != nil {
			return e
		}
		return tx.Create(&commandRow{Scope: scope, BusinessKey: k, Fingerprint: fingerprint, Response: out, CreatedAt: time.Now().UTC()}).Error
	})
	return out, err
}
func jsonResponse(c *gin.Context, raw json.RawMessage, err error) {
	if err != nil {
		handleError(c, err)
		return
	}
	c.Data(200, "application/json", raw)
}
func (a *Application) guardEndpoint(ctx context.Context, raw string) error {
	return a.guardURL(ctx, raw, true)
}
func (a *Application) guardResource(ctx context.Context, raw string) error {
	return a.guardURL(ctx, raw, false)
}
func (a *Application) guardURL(ctx context.Context, raw string, endpoint bool) error {
	u, e := url.Parse(raw)
	if e != nil || u.Hostname() == "" || u.User != nil || u.Fragment != "" {
		return errors.New("invalid external URL")
	}
	host := u.Hostname()
	if a.Config.TestLoopback && (host == "127.0.0.1" || host == "localhost" || host == "::1") && (u.Scheme == "http" || u.Scheme == "https") {
		return nil
	}
	if u.Scheme != "https" {
		return errors.New("external URL requires HTTPS")
	}
	if endpoint && len(a.Config.UpstreamURLAllowlist) > 0 {
		allowed := false
		for _, h := range a.Config.UpstreamURLAllowlist {
			if strings.EqualFold(h, host) || strings.EqualFold(h, u.Host) {
				allowed = true
			}
		}
		if !allowed {
			return errors.New("upstream hostname is outside allowlist")
		}
	}
	ips, e := net.DefaultResolver.LookupIPAddr(ctx, host)
	if e != nil || len(ips) == 0 {
		return errors.New("external hostname cannot be resolved")
	}
	for _, ip := range ips {
		if ip.IP.IsPrivate() || ip.IP.IsLoopback() || ip.IP.IsUnspecified() || ip.IP.IsLinkLocalUnicast() || ip.IP.IsLinkLocalMulticast() || ip.IP.IsMulticast() {
			return fmt.Errorf("external hostname resolves to a restricted address")
		}
	}
	return nil
}

func (a *Application) metricsAccess(c *gin.Context) {
	if a.Config.MetricsToken != "" {
		provided := strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
		if subtle.ConstantTimeCompare([]byte(provided), []byte(a.Config.MetricsToken)) != 1 {
			c.AbortWithStatus(403)
			return
		}
	} else {
		host, _, e := net.SplitHostPort(c.Request.RemoteAddr)
		ip := net.ParseIP(host)
		if e != nil || ip == nil || !ip.IsLoopback() {
			c.AbortWithStatus(403)
			return
		}
	}
}
