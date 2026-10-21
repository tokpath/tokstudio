package portal

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/incentives"
	"github.com/tokpath/tokstudio/backend/internal/jobs"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/modelruntime"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"github.com/tokpath/tokstudio/backend/internal/payments"
	"github.com/tokpath/tokstudio/backend/internal/platform/config"
	"github.com/tokpath/tokstudio/backend/internal/platform/db"
	"github.com/tokpath/tokstudio/backend/internal/platform/db/dbtest"
	"github.com/tokpath/tokstudio/backend/internal/platform/mail"
	"github.com/tokpath/tokstudio/backend/internal/remediation"
	"gorm.io/gorm"
)

type mailbox struct {
	mu       sync.Mutex
	messages []mail.Message
}

func TestVerifiedPartialRefundIsAReviewableFact(t *testing.T) {
	gin.SetMode(gin.TestMode)
	writer := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(writer)
	c.Request = httptest.NewRequest("POST", "/v1/operators/operator/refunds/leg/verify_manual", nil)
	handleError(c, remediation.ErrPaymentMismatch)
	if writer.Code != http.StatusConflict || !strings.Contains(writer.Body.String(), "refund_execution_mismatch") || strings.Contains(writer.Body.String(), `"retryable":true`) {
		t.Fatal(writer.Code, writer.Body.String())
	}
}

func (m *mailbox) Send(_ context.Context, v mail.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, v)
	return nil
}

type testApp struct {
	a      *Application
	server *httptest.Server
	client *http.Client
	box    *mailbox
	csrf   string
}

func setupPortal(t *testing.T) *testApp {
	t.Helper()
	gin.SetMode(gin.TestMode)
	gdb := dbtest.New(t)
	if e := db.Apply(gdb, AllMigrations()); e != nil {
		t.Fatal(e)
	}
	cfg := Config{Config: &config.Config{Env: "development", PublicBaseURL: "http://127.0.0.1", WebOrigin: "http://127.0.0.1", EncryptionKey: strings.Repeat("k", 32)}, OfficialHost: "127.0.0.1", AppKey: strings.Repeat("k", 32), BootstrapEmail: "owner@example.test", BootstrapPassword: "owner-password-test", BootstrapName: "Owner", BrandName: "Test Brand", OperatorName: "Test Operator", LegalName: "Test Legal", TestLoopback: true}
	cfg.RedisURL = os.Getenv("TOKENHUB_TEST_REDIS_URL")
	cfg.S3Endpoint = os.Getenv("TOKENHUB_TEST_S3_ENDPOINT")
	cfg.S3Region = os.Getenv("TOKENHUB_TEST_S3_REGION")
	cfg.S3Bucket = os.Getenv("TOKENHUB_TEST_S3_BUCKET")
	cfg.S3AccessKey = os.Getenv("TOKENHUB_TEST_S3_ACCESS_KEY")
	cfg.S3SecretKey = os.Getenv("TOKENHUB_TEST_S3_SECRET_KEY")
	cfg.S3ForcePathStyle = true
	if _, e := Bootstrap(context.Background(), cfg, gdb); e != nil {
		t.Fatal(e)
	}
	a, e := New(context.Background(), cfg, gdb)
	if e != nil {
		t.Fatal(e)
	}
	box := &mailbox{}
	a.Mail.SetSender(box)
	s := httptest.NewUnstartedServer(a.Router())
	if os.Getenv("TOKENHUB_MOCK_CALLBACK_HOST") != "" {
		listener, e := net.Listen("tcp", "0.0.0.0:0")
		if e != nil {
			t.Fatal(e)
		}
		s.Listener.Close()
		s.Listener = listener
	}
	s.Start()
	if os.Getenv("TOKENHUB_MOCK_CALLBACK_HOST") != "" {
		_, port, _ := net.SplitHostPort(s.Listener.Addr().String())
		s.URL = "http://127.0.0.1:" + port
	}
	t.Cleanup(func() { s.Close(); a.Close() })
	jar, _ := cookiejar.New(nil)
	return &testApp{a: a, server: s, client: &http.Client{Jar: jar}, box: box}
}
func (f *testApp) call(t *testing.T, method, path string, input any, key string) (int, map[string]any, http.Header) {
	t.Helper()
	var body io.Reader
	if input != nil {
		b, _ := json.Marshal(input)
		body = bytes.NewReader(b)
	}
	req, e := http.NewRequest(method, f.server.URL+path, body)
	if e != nil {
		t.Fatal(e)
	}
	req.Header.Set("Content-Type", "application/json")
	if method != "GET" {
		if key == "" {
			key = uuid.NewString()
		}
		req.Header.Set("Idempotency-Key", key)
		req.Header.Set("X-CSRF-Token", f.csrf)
		req.Header.Set("Origin", f.server.URL)
	}
	resp, e := f.client.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	if json.Unmarshal(raw, &out) != nil {
		t.Fatalf("non JSON %d %s", resp.StatusCode, raw)
	}
	return resp.StatusCode, out, resp.Header
}
func mustOK(t *testing.T, status int, out any) {
	t.Helper()
	if status != 200 {
		t.Fatalf("status %d: %#v", status, out)
	}
}
func (f *testApp) login(t *testing.T, kind, email, password string) {
	t.Helper()
	status, out, h := f.call(t, "POST", "/v1/auth/login", map[string]any{"kind": kind, "email": email, "password": password}, "")
	mustOK(t, status, out)
	f.csrf = out["csrf_token"].(string)
	cookie := h.Get("Set-Cookie")
	if !strings.Contains(cookie, "HttpOnly") || !strings.Contains(cookie, "SameSite=Lax") {
		t.Fatal("unsafe session cookie", cookie)
	}
}
func (f *testApp) openRegistration(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	_, e := f.a.Incentives.SetPolicy(ctx, nil, f.a.Installation.OwnerStaffID, f.a.Installation.OperatorID, incentives.PolicyInput{ThresholdUSD: ledger.MustDecimal("1"), DirectRate: ledger.MustDecimal("0.1"), IndirectRate: ledger.MustDecimal("0.02"), RewardValiditySeconds: 86400})
	if e != nil {
		t.Fatal(e)
	}
	st, e := f.a.Organization.GetBusinessStatus(ctx, nil, f.a.Installation.OperatorID)
	if e != nil {
		t.Fatal(e)
	}
	_, e = f.a.Organization.SetBusinessStatus(ctx, nil, organization.SetBusinessStatusInput{ActorStaffID: f.a.Installation.OwnerStaffID, OperatorID: f.a.Installation.OperatorID, Feature: organization.Registration, Enabled: true, ExpectedVersion: st.Version, EvidenceReference: "test policy and mail ready"}, f.a)
	if e != nil {
		t.Fatal(e)
	}
}
func (f *testApp) registered(t *testing.T, email string) {
	t.Helper()
	status, out, _ := f.call(t, "POST", "/v1/auth/registrations", map[string]any{"email": email, "password": "customer-password-test", "display_name": "Customer"}, "")
	mustOK(t, status, out)
	if e := f.a.RunMailJobs(context.Background(), "mail-test", 1); e != nil {
		t.Fatal(e)
	}
	f.box.mu.Lock()
	message := f.box.messages[len(f.box.messages)-1]
	f.box.mu.Unlock()
	var token string
	for _, line := range strings.Split(message.Text, "\n") {
		if strings.HasPrefix(line, "http") {
			u, e := url.Parse(line)
			if e != nil {
				t.Fatal(e)
			}
			token = u.Query().Get("token")
		}
	}
	if token == "" {
		t.Fatal("missing verification message")
	}
	status, out, _ = f.call(t, "POST", "/v1/auth/verify", map[string]string{"token": token}, "")
	mustOK(t, status, out)
}
func TestPostgresBrowserIdentityCSRFAndScope(t *testing.T) {
	f := setupPortal(t)
	f.openRegistration(t)
	f.login(t, "staff", "owner@example.test", "owner-password-test")
	status, out, _ := f.call(t, "GET", "/v1/operators/"+f.a.Installation.OperatorID+"/plans", nil, "")
	mustOK(t, status, out)
	// CSRF must be verified before the action handler executes.
	saved := f.csrf
	f.csrf = "wrong"
	status, out, _ = f.call(t, "POST", "/v1/operators/"+f.a.Installation.OperatorID+"/commissions", map[string]any{"configuration": map[string]any{"threshold_usd": "999", "expected_version": 1}}, "")
	if status != 403 {
		t.Fatal(status, out)
	}
	f.csrf = saved
	policy, e := f.a.Incentives.CurrentPolicy(context.Background(), nil, f.a.Installation.OperatorID)
	if e != nil || policy.Version != 1 {
		t.Fatal("CSRF mutated domain", policy, e)
	}
	f.registered(t, "customer@example.test")
	f.login(t, "customer", "customer@example.test", "customer-password-test")
	status, out, _ = f.call(t, "GET", "/v1/me/credits", nil, "")
	mustOK(t, status, out)
	if out["customer_atg"].(map[string]any)["available"] != "0" {
		t.Fatal(out)
	}
	status, out, _ = f.call(t, "GET", "/v1/operators/"+f.a.Installation.OperatorID+"/plans", nil, "")
	if status != 403 {
		t.Fatal("customer accessed staff", status, out)
	}
	status, out, _ = f.call(t, "POST", "/v1/me/keys", map[string]any{"name": "Primary", "model_ids": nil, "budget": map[string]any{"limit_atg": "0.000000000000000001", "period": "month", "timezone": "Asia/Shanghai"}, "rate_limit_rpm": 30, "concurrency_limit": 3}, "")
	mustOK(t, status, out)
	for _, resource := range []string{"invoices", "refunds", "account", "topups", "activities", "withdrawals", "credit-sources"} {
		status, page, _ := f.call(t, "GET", "/v1/me/"+resource+"/workspace", nil, "")
		mustOK(t, status, page)
		if page["title"] == nil {
			t.Fatal("workspace route was shadowed", resource, page)
		}
	}
	secret := out["secret"].(string)
	key := out["key"].(map[string]any)
	if key["budget"].(map[string]any)["limit_atg"] != "0.000000000000000001" {
		t.Fatal(out)
	}
	status, out, _ = f.call(t, "GET", "/v1/me/keys", nil, "")
	mustOK(t, status, out)
	b, _ := json.Marshal(out)
	if bytes.Contains(b, []byte(secret)) {
		t.Fatal("secret leaked in list")
	}
	req, _ := http.NewRequest("GET", f.server.URL+"/v1/auth/me", nil)
	req.Host = "forged.example.test"
	resp, e := f.client.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatal("unknown host resolved", resp.StatusCode)
	}
}
func TestPostgresRegistrationIdempotencyFinalInviteAndRecovery(t *testing.T) {
	f := setupPortal(t)
	f.openRegistration(t)
	key := uuid.NewString()
	input := map[string]any{"email": "new@example.test", "password": "customer-password-test", "display_name": "Final", "invite_code": "DOES-NOT-EXIST"}
	status, out, _ := f.call(t, "POST", "/v1/auth/registrations", input, key)
	if status == 200 {
		t.Fatal("invalid invite silently fell back", out)
	}
	input["invite_code"] = ""
	key = uuid.NewString()
	status, first, _ := f.call(t, "POST", "/v1/auth/registrations", input, key)
	mustOK(t, status, first)
	status, second, _ := f.call(t, "POST", "/v1/auth/registrations", input, key)
	mustOK(t, status, second)
	if first["registration_id"] != second["registration_id"] {
		t.Fatal("replayed registration duplicated")
	}
	var n int64
	f.a.DB.Model(&queuedMessage{}).Count(&n)
	if n != 1 {
		t.Fatal("mail duplicated", n)
	}
	input["display_name"] = "Changed"
	status, out, _ = f.call(t, "POST", "/v1/auth/registrations", input, key)
	if status != 409 {
		t.Fatal("changed retry accepted", status, out)
	}
	// Bootstrap is resumable and does not reset a live staff password.
	cfg := f.a.Config
	cfg.BootstrapPassword = "changed-password-test"
	installed, e := Bootstrap(context.Background(), cfg, f.a.DB)
	if e != nil || installed != f.a.Installation {
		t.Fatal(installed, e)
	}
	f.login(t, "staff", "owner@example.test", "owner-password-test")
	if e := db.Readiness(context.Background(), f.a.DB, AllMigrations()); e != nil {
		t.Fatal(e)
	}
	_ = time.Now()
}

// This opt-in test reaches the separately deployed protocol service over TCP;
// no httptest replacement stands in for Stripe or its signed callback.
func TestExternalStripeReceiptCallbackTopup(t *testing.T) {
	lab := os.Getenv("TOKENHUB_INTEGRATION_LAB_URL")
	if lab == "" {
		lab = os.Getenv("TOKENHUB_PAYMENT_MOCK_URL")
	}
	if lab == "" {
		t.Skip("requires separately running integration-lab")
	}
	f := setupPortal(t)
	f.a.Config.PublicBaseURL = f.server.URL
	if callbackHost := os.Getenv("TOKENHUB_MOCK_CALLBACK_HOST"); callbackHost != "" {
		u, _ := url.Parse(f.server.URL)
		f.a.Config.PublicBaseURL = "http://" + net.JoinHostPort(callbackHost, u.Port())
	}
	f.openRegistration(t)
	f.registered(t, "paying@example.test")
	f.login(t, "customer", "paying@example.test", "customer-password-test")
	merchant := payments.Merchant{ID: uuid.NewString(), OperatorID: f.a.Installation.OperatorID, Driver: "stripe", Enabled: true, APIBaseURL: lab, Credentials: map[string]string{"secret_key": "sk_test_" + uuid.NewString(), "publishable_key": "pk_test_lab", "webhook_secret": "whsec_integration_test", "currency": "USD"}}
	if _, e := f.a.Payments.SaveMerchant(context.Background(), f.a.Installation.OwnerStaffID, merchant, 0); e != nil {
		t.Fatal(e)
	}
	modelID := f.configureExternalCatalog(t, lab)
	for _, feature := range []organization.BusinessFeature{organization.NewPurchase, organization.ModelUsage} {
		st, e := f.a.Organization.GetBusinessStatus(context.Background(), nil, f.a.Installation.OperatorID)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = f.a.Organization.SetBusinessStatus(context.Background(), nil, organization.SetBusinessStatusInput{ActorStaffID: f.a.Installation.OwnerStaffID, OperatorID: f.a.Installation.OperatorID, Feature: feature, Enabled: true, ExpectedVersion: st.Version, EvidenceReference: "configured real HTTP contract"}, f.a); e != nil {
			t.Fatal(e)
		}
	}
	cmd := uuid.NewString()
	input := map[string]string{"amount_usd": "100", "external_method_id": merchant.ID}
	status, out, _ := f.call(t, "POST", "/v1/me/topups", input, cmd)
	mustOK(t, status, out)
	orderID := out["id"].(string)
	status, replay, _ := f.call(t, "POST", "/v1/me/topups", input, cmd)
	mustOK(t, status, replay)
	if replay["id"] != orderID {
		t.Fatal("topup duplicated")
	}
	intent, e := f.a.Payments.PaymentIntent(context.Background(), merchant.OperatorID, merchant.ID, orderID)
	if e != nil {
		t.Fatal(e)
	}
	if intent.ProviderID == "" {
		t.Fatal("missing external payment")
	}
	control := func(path string, in any) map[string]any {
		t.Helper()
		b, _ := json.Marshal(in)
		resp, e := http.Post(lab+path, "application/json", bytes.NewReader(b))
		if e != nil {
			t.Fatal(e)
		}
		defer resp.Body.Close()
		var out map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&out)
		if resp.StatusCode != 200 {
			t.Fatal(resp.StatusCode, out)
		}
		return out
	}
	settled := control("/__control/payments/"+intent.ProviderID+"/settle", map[string]any{"status": "succeeded", "amount_received": 10000, "paid_at": time.Now().Unix()})
	event := settled["event_id"].(string)
	delivered := control("/__control/webhooks", map[string]any{"url": f.a.Config.PublicBaseURL + "/v1/payments/webhooks/" + merchant.ID, "event_id": event, "repeat": 2})
	for _, v := range delivered["deliveries"].([]any) {
		if v.(map[string]any)["status"] != float64(200) {
			t.Fatal("callback rejected", delivered)
		}
	}
	if _, e = f.a.Payments.RunPending(context.Background(), 10); e != nil {
		t.Fatal(e)
	}
	if e = f.a.RunDeliveryJobs(context.Background(), "delivery", 10); e != nil {
		t.Fatal(e)
	}
	status, out, _ = f.call(t, "GET", "/v1/me/credits", nil, "")
	mustOK(t, status, out)
	if out["usd_balance"].(map[string]any)["available"] != "100" {
		t.Fatal("verified receipt not credited", out)
	}
	if _, e = f.a.Payments.Reconcile(context.Background(), merchant.OperatorID, merchant.ID, orderID); e != nil {
		t.Fatal(e)
	}
	if e = f.a.RunDeliveryJobs(context.Background(), "delivery", 10); e != nil {
		t.Fatal(e)
	}
	status, out, _ = f.call(t, "GET", "/v1/me/credits", nil, "")
	mustOK(t, status, out)
	if out["usd_balance"].(map[string]any)["available"] != "100" {
		t.Fatal("repeat receipt doubled balance", out)
	}
	status, out, _ = f.call(t, "GET", "/v1/me/plans/catalog", nil, "")
	mustOK(t, status, out)
	plan := out["items"].([]any)[0].(map[string]any)
	status, out, _ = f.call(t, "POST", "/v1/me/orders/quotes", map[string]any{"plan_id": plan["id"]}, "")
	mustOK(t, status, out)
	quote := out
	buyKey := uuid.NewString()
	buy := map[string]any{"quote_id": quote["id"], "terms_version": quote["terms_version"], "funding": map[string]string{"usd_balance": "100", "commission_usd": "0"}, "auto_renew": true, "allow_commission": false}
	status, out, _ = f.call(t, "POST", "/v1/me/orders", buy, buyKey)
	mustOK(t, status, out)
	if out["status"] != "completed" {
		t.Fatal("package not delivered", out)
	}
	status, out, _ = f.call(t, "GET", "/v1/me/credits", nil, "")
	mustOK(t, status, out)
	if out["customer_atg"].(map[string]any)["available"] != "120" {
		t.Fatal("package credits missing", out)
	}
	status, out, _ = f.call(t, "POST", "/v1/me/keys", map[string]any{"name": "HTTP test", "budget": map[string]any{"limit_atg": "10", "period": "month", "timezone": "UTC"}}, "")
	mustOK(t, status, out)
	secret := out["secret"].(string)
	callModel := func(key string) map[string]any {
		t.Helper()
		b, _ := json.Marshal(map[string]any{"model": modelID, "messages": []any{map[string]string{"role": "user", "content": "hello"}}, "max_completion_tokens": 512})
		r, _ := http.NewRequest("POST", f.server.URL+"/v1/chat/completions", bytes.NewReader(b))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Authorization", "Bearer "+secret)
		r.Header.Set("Idempotency-Key", key)
		resp, e := http.DefaultClient.Do(r)
		if e != nil {
			t.Fatal(e)
		}
		defer resp.Body.Close()
		var result map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&result)
		mustOK(t, resp.StatusCode, result)
		if result["model"] != modelID {
			t.Fatal("upstream model identity leaked", result)
		}
		return result
	}
	modelKey := uuid.NewString()
	callModel(modelKey)
	callModel(modelKey)
	status, out, _ = f.call(t, "GET", "/v1/me/requests", nil, "")
	mustOK(t, status, out)
	requests := out["items"].([]any)
	if len(requests) != 1 || requests[0].(map[string]any)["charged_atg"] != "0.017" {
		t.Fatal("request billing or replay failed", out)
	}
	status, out, _ = f.call(t, "GET", "/v1/me/credits", nil, "")
	mustOK(t, status, out)
	if out["customer_atg"].(map[string]any)["available"] != "119.983" {
		t.Fatal("request not settled once", out)
	}
}
func (f *testApp) configureExternalCatalog(t *testing.T, lab string) string {
	t.Helper()
	ctx := context.Background()
	cat := f.a.Catalog
	platform := f.a.Installation.OperatorID
	modelID := "contract-" + uuid.NewString()
	upstreamID := uuid.NewString()
	providerID := uuid.NewString()
	credentialID := uuid.NewString()
	cap := modelruntime.Capability{Operation: modelruntime.Chat, InputTypes: []string{"text"}, Parameters: []string{"max_completion_tokens"}, RequiredParameters: []string{"max_completion_tokens"}, OutputLimitParameter: "max_completion_tokens", MaxInputBytes: 10000, MaxOutput: 1000, BillableBounds: map[string]string{"input": "1000", "output": "1000"}, UsageRules: []modelruntime.UsageRule{{Component: "input", Unit: "token", Path: "/prompt_tokens"}, {Component: "output", Unit: "token", Path: "/completion_tokens"}}, EvidenceID: "test-contract-only:" + lab, VerifiedAt: time.Now(), ParametersVerified: true, UsageVerified: true, BoundsVerified: true, RecoveryVerified: true}
	m := catalog.PublicModel{ID: modelID, Identity: modelID, DisplayName: "Independent HTTP contract model", Capabilities: []modelruntime.Capability{cap}}
	if e := cat.SavePublicModel(ctx, platform, m, 0); e != nil {
		t.Fatal(e)
	}
	if _, e := cat.SaveCredential(ctx, platform, credentialID, providerID, "fixture provider key", "fixture-provider-key", true, 0); e != nil {
		t.Fatal(e)
	}
	upstream := catalog.UpstreamModel{ID: upstreamID, PublicModelID: modelID, Identity: modelID, ProviderID: providerID, Provider: "openai", Model: "test-chat", CredentialID: credentialID, Endpoints: map[modelruntime.Operation]string{modelruntime.Chat: lab + "/v1/chat/completions"}, Capabilities: m.Capabilities, Enabled: true, Healthy: true}
	if e := cat.SaveUpstream(ctx, platform, upstream, 0); e != nil {
		t.Fatal(e)
	}
	m.Enabled = true
	if e := cat.SavePublicModel(ctx, platform, m, 1); e != nil {
		t.Fatal(e)
	}
	if e := cat.SetListing(ctx, platform, modelID, true, 0); e != nil {
		t.Fatal(e)
	}
	for _, scope := range []catalog.PriceScope{{Kind: "retail", SellerID: platform}, {Kind: "supplier", SellerID: providerID, BuyerID: upstreamID}} {
		price := catalog.PriceVersion{ID: uuid.NewString(), Scope: scope, ModelID: modelID, Layer: "regular", EffectiveAt: time.Now().Add(-time.Hour), Published: true, NoticeID: "explicit synthetic protocol terms", Rule: catalog.PriceRule{Mode: "manual", Components: []catalog.ComponentRate{{Key: "input", Unit: "token", Per: "1", Rate: "0.001"}, {Key: "output", Unit: "token", Per: "1", Rate: "0.001"}}}}
		if e := cat.PublishPrice(ctx, platform, price, func(ctx context.Context, tx *gorm.DB, _ catalog.PriceVersion) error {
			_, e := f.a.Cooperation.Coverage(ctx, tx, platform, time.Now())
			return e
		}); e != nil {
			t.Fatal(e)
		}
	}
	if _, e := f.a.Commerce.PublishPackage(ctx, nil, commerce.PublishPackageInput{ActorStaffID: f.a.Installation.OwnerStaffID, OperatorID: platform, Name: "Contract monthly", TermsReference: "independent protocol fixture", Timezone: "UTC", PriceUSD: ledger.MustDecimal("100"), PaidATG: ledger.MustDecimal("100"), GiftATG: ledger.MustDecimal("20"), Period: ledger.Month, NewPurchaseEnabled: true, RenewalEnabled: true}); e != nil {
		t.Fatal(e)
	}
	return modelID
}
func TestPostgresDurableScheduleSerializesCyclesAndRecordsFailures(t *testing.T) {
	f := setupPortal(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	errs := make(chan error, 24)
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- f.a.schedule(ctx, "retention", "same-subject", workerPayload{}, time.Minute)
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	var count int64
	f.a.DB.Model(&jobs.Job{}).Where("queue='portal-maintenance' AND kind='retention'").Count(&count)
	if count != 1 {
		t.Fatal("duplicate recurring jobs", count)
	}
	var original jobs.Job
	if e := f.a.DB.Where("queue='portal-maintenance' AND kind='retention'").First(&original).Error; e != nil {
		t.Fatal(e)
	}
	if e := f.a.DB.Model(&scheduledTask{}).Where("id=?", "retention:same-subject").Update("next_due_at", time.Now().Add(-time.Minute)).Error; e != nil {
		t.Fatal(e)
	}
	if e := f.a.schedule(ctx, "retention", "same-subject", workerPayload{}, time.Minute); e != nil {
		t.Fatal(e)
	}
	f.a.DB.Model(&jobs.Job{}).Where("kind='retention'").Count(&count)
	if count != 1 {
		t.Fatal("new cycle overlapped unfinished original", count)
	}
	if e := f.a.RunMaintenanceJobs(ctx, "worker-A", 1); e != nil {
		t.Fatal(e)
	}
	if e := f.a.DB.First(&original, "id=?", original.ID).Error; e != nil {
		t.Fatal(e)
	}
	if original.Status != "completed" || original.Attempts != 1 {
		t.Fatal("maintenance did not complete exactly once", original.Status, original.Attempts)
	}
	if e := f.a.schedule(ctx, "retention", "same-subject", workerPayload{}, time.Minute); e != nil {
		t.Fatal(e)
	}
	f.a.DB.Model(&jobs.Job{}).Where("kind='retention'").Count(&count)
	if count != 2 {
		t.Fatal("next actual cycle not created", count)
	}
	if e := f.a.schedule(ctx, "deliberately_unknown", "failure", workerPayload{}, time.Minute); e != nil {
		t.Fatal(e)
	}
	if e := f.a.RunMaintenanceJobs(ctx, "worker-B", 3); e != nil {
		t.Fatal(e)
	}
	var failed jobs.Job
	if e := f.a.DB.Where("kind='deliberately_unknown'").First(&failed).Error; e != nil {
		t.Fatal(e)
	}
	if failed.Status != "pending" || failed.Attempts != 1 || failed.LastError == "" {
		t.Fatal("worker error was swallowed", failed.Status, failed.Attempts, failed.LastError)
	}
	if e := f.a.DiscoverWorkerTasks(ctx); e != nil {
		t.Fatal("worker discovery does not match migrated schema", e)
	}
}
func TestPostgresLifecycleKeepsStoppedAssetsAndRequiresExplicitATGWaiver(t *testing.T) {
	f := setupPortal(t)
	f.openRegistration(t)
	f.registered(t, "closing@example.test")
	f.login(t, "customer", "closing@example.test", "customer-password-test")
	status, session, _ := f.call(t, "GET", "/v1/auth/me", nil, "")
	mustOK(t, status, session)
	customer := session["customer"].(map[string]any)["id"].(string)
	ctx := context.Background()
	grant, e := f.a.Ledger.IssueGrant(ctx, nil, ledger.GrantInput{OperatorID: f.a.Installation.OperatorID, CustomerID: customer, BusinessKey: "lifecycle-explicit-fixture", ActorID: f.a.Installation.OwnerStaffID, Reason: "isolated lifecycle test funded grant", PaidUSD: ledger.MustDecimal("5"), Sources: []ledger.GrantSource{{Kind: "paid", Amount: ledger.MustDecimal("5")}, {Kind: "package_gift", Amount: ledger.MustDecimal("2")}}, EffectiveAt: time.Now().Add(-time.Minute)})
	if e != nil {
		t.Fatal(e)
	}
	status, out, _ := f.call(t, "POST", "/v1/me/account/lifecycle/stop", map[string]any{"version": 1, "evidence": "customer chooses to stop"}, "")
	mustOK(t, status, out)
	if out["status"] != "stopped" {
		t.Fatal(out)
	}
	g, e := f.a.Ledger.GetGrant(ctx, nil, grant.ID)
	if e != nil || g.Cancelled.Sign() != 0 {
		t.Fatal("stop destroyed assets", g, e)
	}
	status, out, _ = f.call(t, "POST", "/v1/me/account/lifecycle/begin-close", map[string]any{"version": 2, "evidence": "customer closing without waiver"}, "")
	mustOK(t, status, out)
	status, out, _ = f.call(t, "POST", "/v1/me/account/lifecycle/complete-close", map[string]any{"version": 3, "evidence": "attempt while credits remain"}, "")
	if status == 200 {
		t.Fatal("closure bypassed outstanding ATG", out)
	}
	status, out, _ = f.call(t, "POST", "/v1/me/account/waive-credits", map[string]any{"version": 3, "waiver_evidence": "I explicitly waive my remaining purchased and gift ATG for closure"}, "")
	mustOK(t, status, out)
	g, e = f.a.Ledger.GetGrant(ctx, nil, grant.ID)
	if e != nil || g.Cancelled.String() != "7" {
		t.Fatal("explicit waiver not recorded", g, e)
	}
	status, out, _ = f.call(t, "POST", "/v1/me/account/lifecycle/complete-close", map[string]any{"version": 3, "evidence": "customer confirms all obligations settled"}, "")
	mustOK(t, status, out)
	if out["status"] != "closed" {
		t.Fatal(out)
	}
	status, out, _ = f.call(t, "GET", "/v1/me/credits", nil, "")
	if status != 401 {
		t.Fatal("closed identity kept access", status, out)
	}
}

func TestPostgresPublicContextAndManagementActions(t *testing.T) {
	f := setupPortal(t)
	ctx := context.Background()
	status, out, _ := f.call(t, "GET", "/v1/public/context", nil, "")
	mustOK(t, status, out)
	if out["operator"].(map[string]any)["id"] != f.a.Installation.OperatorID {
		t.Fatal("public page lost actual operator", out)
	}
	f.openRegistration(t)
	f.login(t, "staff", "owner@example.test", "owner-password-test")
	scope := "/v1/operators/" + f.a.Installation.OperatorID
	for _, resource := range []string{"overview", "customers", "plans", "orders", "payments", "payment-methods", "valuations", "pricing", "catalog-changes", "marketing", "commissions", "assets", "ledger", "refunds", "withdrawals", "disputes", "invoices", "reconciliation", "reports", "exports", "operators", "procurement", "alerts", "takeovers", "takeover-settlements", "exits", "branding", "domains", "channel-themes", "staff", "approvals", "risk", "notifications", "business-status", "audit", "retention", "providers", "catalog", "routes", "requests", "jobs", "mail"} {
		status, out, _ = f.call(t, "GET", scope+"/"+resource, nil, "")
		mustOK(t, status, out)
		if _, has := out["items"]; !has {
			t.Fatalf("%s has no actual records collection %#v", resource, out)
		}
	}
	// Ordinary package UI submits explicit fields, without constructing JSON.
	action := uuid.NewString()
	in := map[string]any{"name": "HTTP real package", "price_usd": "12.34567890123456789", "paid_atg": "40", "gift_atg": "2", "period": "month", "timezone": "UTC", "terms_reference": "published terms", "new_purchase_enabled": true, "renewal_enabled": true, "expected_version": 0}
	status, out, _ = f.call(t, "POST", scope+"/plans", in, action)
	mustOK(t, status, out)
	packageID := out["id"].(string)
	status, retry, _ := f.call(t, "POST", scope+"/plans", in, action)
	mustOK(t, status, retry)
	if retry["id"] != packageID {
		t.Fatal("package command retry duplicated publication")
	}
	rows, e := f.a.Commerce.ListPackages(ctx, nil, f.a.Installation.OperatorID)
	if e != nil || len(rows) != 1 {
		t.Fatal("package publication duplicated", rows, e)
	}
	status, out, _ = f.call(t, "GET", scope+"/plans", nil, "")
	mustOK(t, status, out)
	operations := out["operations"].([]any)
	fields := operations[0].(map[string]any)["fields"].([]any)
	for _, field := range fields {
		if field.(map[string]any)["kind"] == "json" {
			t.Fatal("ordinary package UI still asks for JSON")
		}
	}
	status, out, _ = f.call(t, "POST", scope+"/catalog-changes", map[string]any{"lead_seconds": 3600, "rules_reference": "customer published lead-time policy", "expected_version": 0}, "")
	mustOK(t, status, out)
	status, out, _ = f.call(t, "POST", scope+"/invoices", map[string]any{"enabled": true, "stage": "receipt", "maximum_ratio": "1", "rules_reference": "invoice contract", "expected_version": 0}, "")
	mustOK(t, status, out)
	status, out, _ = f.call(t, "POST", scope+"/staff/"+uuid.NewString()+"/membership", map[string]any{"role": "owner", "status": "suspended"}, "")
	if status == 200 {
		t.Fatal("unknown staff member was changed")
	}
}
