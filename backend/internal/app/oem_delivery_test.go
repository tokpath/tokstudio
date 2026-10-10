package app_test

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/tokpath/tokstudio/backend/internal/audit"
	"github.com/tokpath/tokstudio/backend/internal/billing"
	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/payment"
	"github.com/tokpath/tokstudio/backend/internal/platform/config"
	"gorm.io/gorm"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestOverhaulOEMDeliveryAndBrandPriceFacts(t *testing.T) {
	if os.Getenv("TOKENHUB_DATABASE_URL") == "" || os.Getenv("TOKENHUB_REDIS_URL") == "" {
		t.Skip("isolated postgres and redis required")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.EncryptionKey = "dev-only-32-byte-key-change-me!!"
	a := mustApp(t, cfg)
	ctx := context.Background()
	server := httptest.NewServer(a.Router())
	defer server.Close()
	login, err := a.Identity.Login(ctx, identity.LoginInput{Email: "admin@tokenhub.local", Password: "password1"})
	if err != nil {
		t.Fatal(err)
	}
	actor := identity.Principal{UserID: login.User.ID, Roles: []string{"platform_admin"}}
	marker := "oem-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	input := identity.OEMCreateInput{OperationID: marker, Brand: &identity.BrandInput{Name: marker, PrimaryDomain: marker + ".test", APIDomain: "api." + marker + ".test", AdminDomain: "admin." + marker + ".test"}, SalesMode: "offline"}
	results := make(chan *identity.DeliveryView, 2)
	failures := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			item, e := a.Identity.CreateOEM(ctx, actor, input)
			if e != nil {
				failures <- e
			} else {
				results <- item
			}
		}()
	}
	wg.Wait()
	close(results)
	close(failures)
	for e := range failures {
		t.Fatal(e)
	}
	var delivery *identity.DeliveryView
	for item := range results {
		if delivery != nil && delivery.ChannelOrgID != item.ChannelOrgID {
			t.Fatal("duplicate OEM on retry")
		}
		delivery = item
	}
	if delivery == nil {
		t.Fatal("no delivery")
	}
	changed := input
	changed.SalesMode = "online"
	if _, err := a.Identity.CreateOEM(ctx, actor, changed); !errors.Is(err, identity.ErrDeliveryConflict) {
		t.Fatalf("changed operation: %v", err)
	}
	channel, err := a.Identity.GetChannel(ctx, actor, delivery.ChannelOrgID)
	if err != nil {
		t.Fatal(err)
	}
	code, body := doJSON(t, http.MethodGet, server.URL+"/admin/oem-deliveries/"+channel.ID, login.Token, false, nil)
	if code != 200 {
		t.Fatalf("projection: %d %+v", code, body)
	}
	projection := body["item"].(map[string]any)
	if projection["ready"] != false {
		t.Fatal("new OEM falsely ready")
	}
	registration, err := url.Parse(projection["registration_url"].(string))
	if err != nil || registration.Host != input.Brand.PrimaryDomain || registration.Path != "/login" || registration.Query().Get("next") != "/enter" {
		t.Fatalf("registration contract: %v %v", registration, err)
	}
	receiver, err := a.Identity.Register(ctx, identity.RegisterInput{Email: marker + "@example.test", Password: "password1", PromotionCode: registration.Query().Get("promotion_code")})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		_, e := a.Identity.SetChannelAdminTx(tx, actor, channel.ID, receiver.User.Email, true)
		return e
	}); err != nil {
		t.Fatal(err)
	}
	admins, err := a.Identity.OEMAdminEvidence(ctx, channel.ID)
	if err != nil || len(admins) != 1 || admins[0].LoginAt != nil {
		t.Fatalf("registration should not count as acceptance: %+v %v", admins, err)
	}
	if _, err := a.Identity.CompleteOEMDelivery(ctx, actor, channel.ID, receiver.User.ID, delivery.Version, map[string]any{"fixture": true}); !errors.Is(err, identity.ErrDeliveryIncomplete) {
		t.Fatalf("pre-login handoff: %v", err)
	}
	receiver, err = a.Identity.Login(ctx, identity.LoginInput{Email: receiver.User.Email, Password: "password1"})
	if err != nil {
		t.Fatal(err)
	}
	admins, err = a.Identity.OEMAdminEvidence(ctx, channel.ID)
	if err != nil || len(admins) != 1 || admins[0].LoginAt == nil {
		t.Fatalf("actual post-grant login missing: %+v %v", admins, err)
	}
	wholesale := map[string]string{"input": "0.0000003", "output": "0.0000005"}
	if err := a.Catalog.SetChannelModels(ctx, channel.ID, []catalog.ChannelModelGrant{{PublicID: catalog.EchoModelID, Enabled: true, Wholesale: wholesale}}); err != nil {
		t.Fatal(err)
	}
	sell := map[string]string{"input": "0.000003", "output": "0.000005"}
	if err := a.Catalog.SetOwnChannelCustomerPrices(ctx, channel.ID, catalog.EchoModelID, sell); err != nil {
		t.Fatal(err)
	}
	child, err := a.Identity.CreateChannel(ctx, identity.Principal{UserID: receiver.User.ID, Roles: []string{"channel_admin"}, ChannelOrgID: channel.ID}, identity.ChannelInput{Code: marker + "-child", Type: "B"})
	if err != nil {
		t.Fatal(err)
	}
	// New channel grants need no procurement price; funds remain with the OEM.
	code, grantBody := doJSON(t, http.MethodPatch, server.URL+"/admin/channels/"+child.ID+"/models", receiver.Token, true, map[string]any{"items": []map[string]any{{"public_id": catalog.EchoModelID, "enabled": true}}})
	if code != 200 {
		t.Fatalf("channel authorization required a procurement price: %d %+v", code, grantBody)
	}
	if err := a.Catalog.SetDelegatedChannelModels(ctx, child.ID, channel.ID, []catalog.ChannelModelGrant{{PublicID: catalog.EchoModelID, Enabled: true, Wholesale: wholesale, CustomerOverride: map[string]string{"input": "0.1", "output": "0.2"}}}); err != nil {
		t.Fatal(err)
	}
	if err := a.Catalog.SetOwnChannelCustomerPrices(ctx, child.ID, catalog.EchoModelID, sell); !errors.Is(err, catalog.ErrModelNotVisible) {
		t.Fatalf("channel price write bypassed service: %v", err)
	}
	snapshot, err := a.Catalog.PriceSnapshot(ctx, catalog.EchoModelID)
	if err != nil {
		t.Fatal(err)
	}
	original := string(snapshot.Raw)
	for _, scope := range []string{channel.ID, child.ID} {
		raw, err := a.Catalog.PriceForChannel(ctx, scope, catalog.EchoModelID, snapshot.Raw)
		if err != nil {
			t.Fatal(err)
		}
		var price map[string]any
		if json.Unmarshal(raw, &price) != nil {
			t.Fatal("price json")
		}
		nested := price["customer_sell"].(map[string]any)
		if price["input"] != sell["input"] || nested["input"] != price["input"] || nested["output"] != price["output"] {
			t.Fatalf("inconsistent brand price for %s: %s", scope, raw)
		}
		quote, err := billing.ParseQuote(snapshot.VersionID, raw)
		if err != nil || quote.InputSell != 3 || quote.OutputSell != 5 {
			t.Fatalf("quote: %+v %v", quote, err)
		}
	}
	for _, scope := range []string{channel.ID, child.ID} {
		model, err := a.Catalog.GetVisibleModel(ctx, scope, catalog.EchoModelID, nil)
		if err != nil || model.SellPrice["input"] != sell["input"] || model.SellPrice["output"] != sell["output"] {
			t.Fatalf("public brand price: %+v %v", model, err)
		}
		for key := range model.SellPrice {
			if strings.Contains(key, "wholesale") || strings.Contains(key, "upstream") {
				t.Fatal("public cost disclosure")
			}
		}
	}
	if string(snapshot.Raw) != original {
		t.Fatal("original price snapshot changed")
	}
	if _, err := a.Billing.AdjustQuota(ctx, actor.UserID, channel.ID, marker+"-quota", 10*billing.MinorPerUSD); err != nil {
		t.Fatal(err)
	}
	customer, err := a.Identity.Register(ctx, identity.RegisterInput{Email: marker + "-customer@example.test", Password: "password1", PromotionCode: registration.Query().Get("promotion_code")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Payment.RecordOfflineReceipt(ctx, channel.ID, payment.OfflineReceiptInput{OperationID: marker + "-op", OccurredAt: time.Now().UTC(), ExpectedIssueRatioBPS: billing.DefaultIssueRatioBPS, UserID: customer.User.ID, AmountMinor: billing.MinorPerUSD, CreditMinor: billing.MinorPerUSD, Currency: "USD", Reference: marker + "-controlled-receipt"}, a.Audit, audit.RecordInput{ActorUserID: actor.UserID}); err != nil {
		t.Fatal(err)
	}
	principal := identity.Principal{UserID: customer.User.ID, ChannelOrgID: channel.ID, BrandID: channel.BrandID, Roles: customer.User.Roles}
	key, err := a.Identity.CreateAPIKey(ctx, principal, "delivery-fixture", cfg.EncryptionKey, nil, 60, 5)
	if err != nil {
		t.Fatal(err)
	}
	response := postJSONRaw(t, server.URL+"/v1/chat/completions", key.Secret, map[string]any{"model": catalog.EchoModelID, "messages": []map[string]string{{"role": "user", "content": "isolated acceptance fixture"}}, "max_tokens": 8})
	requestID, _ := response["request_id"].(string)
	if requestID == "" {
		t.Fatalf("successful request missing: %+v", response)
	}
	// These are controlled evidence fixtures, never a claim that real public DNS/TLS was verified.
	now := time.Now().UTC()
	evidence := map[string]any{}
	for name, host := range map[string]string{"primary": input.Brand.PrimaryDomain, "api": input.Brand.APIDomain, "admin": input.Brand.AdminDomain} {
		evidence[name] = map[string]any{"host": host, "ok": true, "checked_at": now, "expires_at": now.Add(time.Hour)}
	}
	if err := a.Identity.RecordOEMDomainEvidence(ctx, actor, channel.ID, evidence); err != nil {
		t.Fatal(err)
	}
	code, body = doJSON(t, http.MethodGet, server.URL+"/admin/oem-deliveries/"+channel.ID, login.Token, false, nil)
	if code != 200 {
		t.Fatalf("ready projection: %d %+v", code, body)
	}
	projection = body["item"].(map[string]any)
	if projection["ready"] != true {
		t.Fatalf("fixture not ready: %+v", projection["checks"])
	}
	// An otherwise successful billed request is insufficient when its user is
	// a staff/diagnostic account. This mutation is only an isolated evidence fixture.
	if err := a.DB.Table("gateway_requests").Where("request_id=?", requestID).Update("user_id", receiver.User.ID).Error; err != nil {
		t.Fatal(err)
	}
	code, staffProof := doJSON(t, http.MethodGet, server.URL+"/admin/oem-deliveries/"+channel.ID, login.Token, false, nil)
	if code != 200 || staffProof["item"].(map[string]any)["request_evidence"] != nil {
		t.Fatalf("staff call counted as customer evidence: %+v", staffProof)
	}
	if err := a.DB.Table("gateway_requests").Where("request_id=?", requestID).Update("user_id", customer.User.ID).Error; err != nil {
		t.Fatal(err)
	}
	version := int64(projection["delivery"].(map[string]any)["version"].(float64))
	code, body = doJSON(t, http.MethodPost, server.URL+"/admin/oem-deliveries/"+channel.ID+"/handoff", login.Token, true, map[string]any{"expected_version": version, "receiver_user_id": receiver.User.ID})
	if code != 200 {
		t.Fatalf("handoff: %d %+v", code, body)
	}
	saved, err := a.Identity.OEMDelivery(ctx, actor, channel.ID)
	if err != nil || saved.HandedOverAt == nil || saved.HandoffEvidence["request_id"] != requestID {
		t.Fatalf("handoff evidence: %+v %v", saved, err)
	}
	evidence["api"] = map[string]any{"host": input.Brand.APIDomain, "ok": true, "checked_at": now.Add(-25 * time.Hour), "expires_at": now.Add(time.Hour)}
	if err := a.Identity.RecordOEMDomainEvidence(ctx, actor, channel.ID, evidence); err != nil {
		t.Fatal(err)
	}
	code, body = doJSON(t, http.MethodGet, server.URL+"/admin/oem-deliveries/"+channel.ID, login.Token, false, nil)
	if code != 200 || body["item"].(map[string]any)["ready"] != false {
		t.Fatalf("stale domains remain green: %d %+v", code, body)
	}
	saved, err = a.Identity.OEMDelivery(ctx, actor, channel.ID)
	if err != nil || saved.HandedOverAt == nil || saved.HandoffEvidence["request_id"] != requestID {
		t.Fatal("current failure rewrote historical acceptance")
	}
}

func TestOverhaulQuotaAdjustmentAtomicRetries(t *testing.T) {
	if os.Getenv("TOKENHUB_DATABASE_URL") == "" || os.Getenv("TOKENHUB_REDIS_URL") == "" {
		t.Skip("isolated postgres and redis required")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	a := mustApp(t, cfg)
	ctx := context.Background()
	marker := "quota-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	before, err := a.Billing.ChannelQuota(ctx, identity.OEMChannelID)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	ids := make(chan string, 4)
	failures := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			operation, e := a.Billing.AdjustQuota(ctx, "fixture-actor", identity.OEMChannelID, marker, billing.MinorPerUSD)
			if e != nil {
				failures <- e
			} else {
				ids <- operation.ID
			}
		}()
	}
	wg.Wait()
	close(ids)
	close(failures)
	for e := range failures {
		t.Fatal(e)
	}
	first := ""
	for result := range ids {
		if first != "" && first != result {
			t.Fatal("duplicate adjustment")
		}
		first = result
	}
	after, err := a.Billing.ChannelQuota(ctx, identity.OEMChannelID)
	if err != nil || after.AvailableMinor != before.AvailableMinor+billing.MinorPerUSD {
		t.Fatalf("quota adjusted more than once: %v", err)
	}
	if _, err := a.Billing.AdjustQuota(ctx, "other-actor", identity.OEMChannelID, marker, billing.MinorPerUSD); !errors.Is(err, billing.ErrConflict) {
		t.Fatalf("actor conflict: %v", err)
	}
	if _, err := a.Billing.AdjustQuota(ctx, "fixture-actor", identity.OEMChannelID, marker, 2*billing.MinorPerUSD); !errors.Is(err, billing.ErrConflict) {
		t.Fatalf("payload conflict: %v", err)
	}
	if _, err := a.Billing.QuotaOperation(ctx, "other-actor", identity.OEMChannelID, marker); !errors.Is(err, billing.ErrNotFound) {
		t.Fatalf("operation leaked: %v", err)
	}
	var ledgerCount int64
	if err := a.DB.Table("billing_quota_ledger").Where("reference_type='quota_operation' AND reference_id=?", first).Count(&ledgerCount).Error; err != nil || ledgerCount != 1 {
		t.Fatalf("operation ledger count %d %v", ledgerCount, err)
	}
}
