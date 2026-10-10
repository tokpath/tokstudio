package app_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/billing"
	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/platform/id"
)

func accountingUser(t *testing.T, fx *wmeterEnv, brand, promo string, limit int64) (*identity.Principal, *identity.APIKeyView, string) {
	t.Helper()
	session, err := fx.app.Identity.Register(fx.ctx, identity.RegisterInput{Email: id.New("actual") + "@example.test", Password: "password1", BrandID: brand, PromotionCode: promo})
	if err != nil {
		t.Fatal(err)
	}
	p, err := fx.app.Identity.Authenticate(fx.ctx, session.Token)
	if err != nil {
		t.Fatal(err)
	}
	key, err := fx.app.Identity.CreateAPIKeyWithLimits(fx.ctx, *p, fx.app.Config.EncryptionKey, identity.APIKeyLimits{Name: "actual", ModelMode: "all", BudgetLimitMinor: &limit})
	if err != nil {
		t.Fatal(err)
	}
	return p, key, session.Token
}
func accountingReserve(t *testing.T, fx *wmeterEnv, p *identity.Principal, key *identity.APIKeyView, amount int64, prices json.RawMessage) string {
	t.Helper()
	request := id.New("req")
	if _, err := fx.app.Billing.Reserve(fx.ctx, billing.ReserveInput{UserID: p.UserID, ChannelOrgID: p.ChannelOrgID, APIKeyID: key.ID, PublicModelID: catalog.EchoModelID, RequestID: request, ReserveMinor: amount, UnitPrices: prices}); err != nil {
		t.Fatal(err)
	}
	return request
}
func accountingBalance(t *testing.T, fx *wmeterEnv, p *identity.Principal, available, gift, reserved int64) {
	t.Helper()
	b, err := fx.app.Billing.Balance(fx.ctx, p.UserID, p.ChannelOrgID)
	if err != nil || b.AvailableMinor != available || b.GiftMinor != gift || b.ReservedMinor != reserved || b.PurchasedMinor != available-gift {
		t.Fatalf("balance %+v want %d/%d/%d: %v", b, available, gift, reserved, err)
	}
}

var accountingPrices = json.RawMessage(`{"currency":"USD","input":"0.000001","output":"0.000002","wholesale_input":"0.0000005","wholesale_output":"0.000001"}`)

func TestActualKeyCashExcessAdmissionAndRefund(t *testing.T) {
	fx := newWMeterEnv(t)
	p, key, session := accountingUser(t, fx, "", identity.PromoKOL2B, 100000)
	if err := fx.app.Billing.Credit(fx.ctx, p.UserID, id.New("credit"), 100000, ""); err != nil {
		t.Fatal(err)
	}
	request := accountingReserve(t, fx, p, key, 100000, accountingPrices)
	// Changing the limit/model/name while in flight only changes later admissions.
	low := int64(50000)
	if _, err := fx.app.Identity.UpdateAPIKeyLimits(fx.ctx, *p, key.ID, identity.APIKeyLimits{Name: "lowered", ModelMode: "selected", Allowlist: []string{catalog.OEMModelID}, BudgetLimitMinor: &low}); err != nil {
		t.Fatal(err)
	}
	in := billing.SettleInput{RequestID: request, Usage: map[string]int{"prompt_tokens": 120000, "completion_tokens": 0}, PublicModelID: catalog.EchoModelID}
	var wg sync.WaitGroup
	results := make(chan *billing.Settlement, 2)
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, e := fx.app.Billing.Settle(fx.ctx, in)
			if e != nil {
				errs <- e
			} else {
				results <- r
			}
		}()
	}
	wg.Wait()
	close(errs)
	close(results)
	for err := range errs {
		t.Fatal(err)
	}
	charge := ""
	for r := range results {
		if r.State != billing.UsageConfirmed || r.AmountMinor != 120000 || (charge != "" && charge != r.ChargeID) {
			t.Fatalf("actual/replay %+v", r)
		}
		charge = r.ChargeID
	}
	accountingBalance(t, fx, p, -20000, 0, 0)
	assertBudget(t, fx.app.Identity, fx.ctx, *p, fx.app.Config.EncryptionKey, key.ID, 120000, 0)
	var entries int64
	if err := fx.app.DB.Table("commission_entries").Where("request_id=?", request).Count(&entries).Error; err != nil {
		t.Fatal(err)
	}
	if entries == 0 {
		t.Fatal("reliable excess did not accrue normal commission")
	}
	code, body := doJSON(t, http.MethodGet, fx.server.URL+"/v1/me/balance", session, false, nil)
	if code != 200 || body["balance"].(map[string]any)["available"] != "-0.02" {
		t.Fatalf("negative HTTP balance %d %+v", code, body)
	}
	in.Usage["prompt_tokens"] = 120001
	if _, err := fx.app.Billing.Settle(fx.ctx, in); !errors.Is(err, billing.ErrConflict) {
		t.Fatalf("conflicting actual usage %v", err)
	}
	high := int64(200000)
	if _, err := fx.app.Identity.UpdateAPIKeyLimits(fx.ctx, *p, key.ID, identity.APIKeyLimits{Name: "editable after excess", ModelMode: "all", BudgetLimitMinor: &high}); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.app.Billing.Reserve(fx.ctx, billing.ReserveInput{UserID: p.UserID, ChannelOrgID: p.ChannelOrgID, APIKeyID: key.ID, RequestID: id.New("req"), PublicModelID: catalog.EchoModelID, ReserveMinor: 1, UnitPrices: accountingPrices}); !errors.Is(err, billing.ErrInsufficientBalance) {
		t.Fatalf("cash debt admitted call: %v", err)
	}
	if err := fx.app.Billing.Credit(fx.ctx, p.UserID, id.New("credit"), 30000, ""); err != nil {
		t.Fatal(err)
	}
	accountingBalance(t, fx, p, 10000, 0, 0)
	assertBudget(t, fx.app.Identity, fx.ctx, *p, fx.app.Config.EncryptionKey, key.ID, 120000, 0)
	held := accountingReserve(t, fx, p, key, 10000, accountingPrices)
	if _, err := fx.app.Billing.Reserve(fx.ctx, billing.ReserveInput{UserID: p.UserID, ChannelOrgID: p.ChannelOrgID, APIKeyID: key.ID, RequestID: id.New("req"), PublicModelID: catalog.EchoModelID, ReserveMinor: 1, UnitPrices: accountingPrices}); !errors.Is(err, billing.ErrInsufficientBalance) {
		t.Fatalf("occupied balance reused: %v", err)
	}
	if err := fx.app.Billing.Release(fx.ctx, held); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err := fx.app.Billing.RefundCharge(fx.ctx, request); err != nil {
			t.Fatal(err)
		}
	}
	accountingBalance(t, fx, p, 130000, 0, 0)
	assertBudget(t, fx.app.Identity, fx.ctx, *p, fx.app.Config.EncryptionKey, key.ID, 0, 0)
}

func TestActualKeyGiftExcessDoesNotStealOtherReservations(t *testing.T) {
	fx := newWMeterEnv(t)
	for _, concurrent := range []bool{false, true} {
		t.Run(map[bool]string{false: "all remaining gift", true: "another occupied gift"}[concurrent], func(t *testing.T) {
			p, key, _ := accountingUser(t, fx, "", "THA1", 2000000)
			if err := fx.app.Billing.GrantGift(fx.ctx, p.UserID, id.New("gift"), 1000000); err != nil {
				t.Fatal(err)
			}
			request := accountingReserve(t, fx, p, key, 100000, accountingPrices)
			other := ""
			if concurrent {
				other = accountingReserve(t, fx, p, key, 300000, accountingPrices)
			}
			result, err := fx.app.Billing.Settle(fx.ctx, billing.SettleInput{RequestID: request, Usage: map[string]int{"prompt_tokens": 1100000, "completion_tokens": 0}})
			if err != nil || result.State != billing.UsageConfirmed {
				t.Fatalf("gift actual %+v %v", result, err)
			}
			var auth struct{ GiftSettledMinor int64 }
			if err := fx.app.DB.Table("billing_authorizations").Where("request_id=?", request).First(&auth).Error; err != nil {
				t.Fatal(err)
			}
			if concurrent {
				accountingBalance(t, fx, p, -400000, 0, 300000)
				if auth.GiftSettledMinor != 700000 {
					t.Fatalf("stole occupied gift %d", auth.GiftSettledMinor)
				}
				if err := fx.app.Billing.Release(fx.ctx, other); err != nil {
					t.Fatal(err)
				}
				accountingBalance(t, fx, p, -100000, 300000, 0)
			} else {
				accountingBalance(t, fx, p, -100000, 0, 0)
				if auth.GiftSettledMinor != 1000000 {
					t.Fatalf("did not consume free gift %d", auth.GiftSettledMinor)
				}
			}
		})
	}
}

func TestActualKeyEntitlementExcessAndDebtNetAdmission(t *testing.T) {
	fx := newWMeterEnv(t)
	p, key, _ := accountingUser(t, fx, "", "THA1", 3000000)
	grant := func(unit string, amount int64) {
		t.Helper()
		if _, err := fx.app.Plans.GrantBonus(fx.ctx, "actual", id.New("bonus"), p.UserID, unit, amount, time.Hour); err != nil {
			t.Fatal(err)
		}
	}
	grant("usd_credit", 1000000)
	grant("token", 100000000)
	expired, err := fx.app.Plans.GrantBonus(fx.ctx, "actual", id.New("bonus"), p.UserID, "usd_credit", 1000000, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if err := fx.app.DB.Exec("UPDATE plans_entitlement_accounts SET expires_at=NOW()-INTERVAL '1 minute' WHERE id=?", expired.ID).Error; err != nil {
		t.Fatal(err)
	}
	request := accountingReserve(t, fx, p, key, 100000, accountingPrices)
	in := billing.SettleInput{RequestID: request, Usage: map[string]int{"prompt_tokens": 1100000, "completion_tokens": 0}}
	for i := 0; i < 2; i++ {
		r, err := fx.app.Billing.Settle(fx.ctx, in)
		if err != nil || r.State != billing.UsageConfirmed || r.AmountMinor != 1100000 {
			t.Fatalf("plan actual %+v %v", r, err)
		}
	}
	accountingBalance(t, fx, p, -100000, 0, 0)
	avail, err := fx.app.Plans.AvailableUSD(fx.ctx, p.UserID)
	if err != nil || avail != 0 {
		t.Fatalf("plan unused/invalid coverage %d %v", avail, err)
	}
	if err := fx.app.Billing.Credit(fx.ctx, p.UserID, id.New("credit"), 50000, ""); err != nil {
		t.Fatal(err)
	}
	grant("usd_credit", 40000)
	if _, err := fx.app.Billing.Reserve(fx.ctx, billing.ReserveInput{UserID: p.UserID, ChannelOrgID: p.ChannelOrgID, APIKeyID: key.ID, RequestID: id.New("req"), PublicModelID: catalog.EchoModelID, ReserveMinor: 1, UnitPrices: accountingPrices}); !errors.Is(err, billing.ErrInsufficientBalance) {
		t.Fatalf("negative cash was clamped before plan coverage: %v", err)
	}
	grant("usd_credit", 20000)
	held := accountingReserve(t, fx, p, key, 5000, accountingPrices)
	accountingBalance(t, fx, p, -50000, 0, 0)
	if err := fx.app.Billing.Release(fx.ctx, held); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err := fx.app.Billing.RefundCharge(fx.ctx, request); err != nil {
			t.Fatal(err)
		}
	}
	accountingBalance(t, fx, p, 50000, 0, 0)
	avail, err = fx.app.Plans.AvailableUSD(fx.ctx, p.UserID)
	if err != nil || avail != 1060000 {
		t.Fatalf("actual plan coverage refund %d %v", avail, err)
	}
	assertBudget(t, fx.app.Identity, fx.ctx, *p, fx.app.Config.EncryptionKey, key.ID, 0, 0)
}

func TestActualKeyOEMBrandPriceAndUnknownUsage(t *testing.T) {
	fx := newWMeterEnv(t)
	p, key, _ := accountingUser(t, fx, identity.OEMBrandID, "THC1", 100000)
	snapshot, err := fx.app.Catalog.PriceSnapshot(fx.ctx, catalog.OEMModelID)
	if err != nil {
		t.Fatal(err)
	}
	var original struct{ CustomerOverrideJSON json.RawMessage }
	if err := fx.app.DB.Table("catalog_channel_model_policies").Select("customer_override_json").Where("channel_org_id=? AND public_model_id=(SELECT id FROM catalog_public_models WHERE public_id=?)", p.ChannelOrgID, catalog.OEMModelID).Scan(&original).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		fx.app.DB.Exec("UPDATE catalog_channel_model_policies SET customer_override_json=? WHERE channel_org_id=? AND public_model_id=(SELECT id FROM catalog_public_models WHERE public_id=?)", func() any {
			if len(original.CustomerOverrideJSON) == 0 {
				return nil
			}
			return string(original.CustomerOverrideJSON)
		}(), p.ChannelOrgID, catalog.OEMModelID)
	})
	if err := fx.app.Catalog.SetOwnChannelCustomerPrices(fx.ctx, p.ChannelOrgID, catalog.OEMModelID, map[string]string{"input": "0.000002", "output": "0.000004"}); err != nil {
		t.Fatal(err)
	}
	prices, err := fx.app.Catalog.PriceForChannel(fx.ctx, p.ChannelOrgID, catalog.OEMModelID, snapshot.Raw)
	if err != nil {
		t.Fatal(err)
	}
	topup, err := fx.app.Billing.CreateTopup(fx.ctx, p.UserID, p.ChannelOrgID, 100000, "manual")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fx.app.Billing.ConfirmTopup(fx.ctx, topup.ID, "actual-finance"); err != nil {
		t.Fatal(err)
	}
	var source struct {
		ID                          string
		GrantedMinor, ConsumedMinor int64
	}
	if err := fx.app.DB.Table("billing_quota_allocations").Where("source_id=?", topup.ID).First(&source).Error; err != nil {
		t.Fatal(err)
	}
	request := id.New("req")
	if _, err := fx.app.Billing.Reserve(fx.ctx, billing.ReserveInput{UserID: p.UserID, ChannelOrgID: p.ChannelOrgID, APIKeyID: key.ID, PublicModelID: catalog.OEMModelID, RequestID: request, ReserveMinor: 100000, UnitPrices: prices, PriceVersionID: snapshot.VersionID}); err != nil {
		t.Fatal(err)
	}
	// Unknown keeps its reservation, then late reliable facts settle at original
	// brand terms despite a changed current selling price and a forged late user.
	for i := 0; i < 2; i++ {
		r, err := fx.app.Billing.Settle(fx.ctx, billing.SettleInput{RequestID: request, MissingUsage: true})
		if err != nil || r.State != billing.UsagePending {
			t.Fatalf("unknown %+v %v", r, err)
		}
	}
	accountingBalance(t, fx, p, 0, 0, 100000)
	assertBudget(t, fx.app.Identity, fx.ctx, *p, fx.app.Config.EncryptionKey, key.ID, 0, 100000)
	if err := fx.app.Catalog.SetOwnChannelCustomerPrices(fx.ctx, p.ChannelOrgID, catalog.OEMModelID, map[string]string{"input": "100", "output": "100"}); err != nil {
		t.Fatal(err)
	}
	r, err := fx.app.Billing.Settle(fx.ctx, billing.SettleInput{RequestID: request, UserID: "other", ChannelOrgID: identity.OfficialChannelID, APIKeyID: "other", Usage: map[string]int{"prompt_tokens": 60000, "completion_tokens": 0}, UnitPrices: json.RawMessage(`{"input":"100","output":"100"}`)})
	if err != nil || r.State != billing.UsageConfirmed || r.AmountMinor != 120000 {
		t.Fatalf("original OEM charge %+v %v", r, err)
	}
	accountingBalance(t, fx, p, -20000, 0, 0)
	assertBudget(t, fx.app.Identity, fx.ctx, *p, fx.app.Config.EncryptionKey, key.ID, 120000, 0)
	facts, err := fx.app.Billing.QueryUsage(fx.ctx, billing.QueryUsageInput{RequestID: request})
	if err != nil || len(facts) != 2 {
		t.Fatalf("unknown history %+v %v", facts, err)
	}
	for _, fact := range facts {
		if fact.ChannelOrgID != p.ChannelOrgID || fact.UserID != p.UserID || fact.APIKeyID != key.ID || fact.PublicModelID != catalog.OEMModelID {
			t.Fatalf("late fact escaped original brand %+v", fact)
		}
	}
	var pending int64
	fx.app.DB.Table("billing_usage_events").Where("request_id=? AND state=?", request, billing.UsagePending).Count(&pending)
	if pending != 0 {
		t.Fatal("reliable late fact still in pending queue")
	}
	if err := fx.app.DB.Table("billing_quota_allocations").Where("id=?", source.ID).First(&source).Error; err != nil {
		t.Fatal(err)
	}
	if source.GrantedMinor != 100000 || source.ConsumedMinor != 100000 {
		t.Fatalf("actual excess invented source allocation %+v", source)
	}
	for i := 0; i < 2; i++ {
		if _, err := fx.app.Billing.RefundCharge(fx.ctx, request); err != nil {
			t.Fatal(err)
		}
	}
	if err := fx.app.DB.Table("billing_quota_allocations").Where("id=?", source.ID).First(&source).Error; err != nil {
		t.Fatal(err)
	}
	if source.ConsumedMinor != 0 || source.GrantedMinor != 100000 {
		t.Fatalf("refund restored nonexistent allocation %+v", source)
	}
	accountingBalance(t, fx, p, 100000, 0, 0)
}

func TestActualKeyProtocolEstimatesIncludingVisionAndStream(t *testing.T) {
	fx := newWMeterEnv(t)
	p, key, _ := accountingUser(t, fx, "", "THA1", 1000000)
	if err := fx.app.Billing.Credit(fx.ctx, p.UserID, id.New("credit"), 1000000, ""); err != nil {
		t.Fatal(err)
	}
	for _, endpoint := range []string{"/v1/chat/completions", "/v1/responses", "/v1/messages"} {
		var payload any = map[string]any{"model": catalog.EchoModelID, "max_tokens": 32, "reasoning_effort": "high", "messages": []map[string]any{{"role": "user", "content": []map[string]any{{"type": "text", "text": "vision accounting"}, {"type": "image_url", "image_url": map[string]string{"url": "https://example.test/image.png"}}}}}}
		if endpoint == "/v1/responses" {
			payload = map[string]any{"model": catalog.EchoModelID, "max_output_tokens": 32, "input": "response accounting"}
		}
		if endpoint == "/v1/messages" {
			payload = map[string]any{"model": catalog.EchoModelID, "max_tokens": 32, "messages": []map[string]string{{"role": "user", "content": "message accounting"}}}
		}
		code, body := doJSON(t, http.MethodPost, fx.server.URL+endpoint, key.Secret, false, payload)
		if code != 200 {
			t.Fatalf("finite estimate rejected %s: %d %+v", endpoint, code, body)
		}
	}
	reqBody := strings.NewReader(`{"model":"` + catalog.EchoModelID + `","max_tokens":1,"stream":true,"messages":[{"role":"user","content":"stream real usage may exceed its output estimate"}]}`)
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, fx.server.URL+"/v1/chat/completions", reqBody)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key.Secret)
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	streamBody, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != 200 {
		t.Fatalf("finite stream %d %s", response.StatusCode, streamBody)
	}
	if !strings.Contains(string(streamBody), "data: [DONE]") {
		t.Fatalf("stream body = %s", streamBody)
	}
	facts, err := fx.app.Billing.QueryUsage(fx.ctx, billing.QueryUsageInput{UserID: p.UserID})
	if err != nil || len(facts) != 4 {
		t.Fatalf("protocol facts %+v %v", facts, err)
	}
	var total int64
	for _, fact := range facts {
		if fact.State != billing.UsageConfirmed {
			t.Fatalf("reliable protocol remains pending %+v", fact)
		}
		total += fact.CustomerMinor
	}
	assertBudget(t, fx.app.Identity, fx.ctx, *p, fx.app.Config.EncryptionKey, key.ID, total, 0)
}

func TestActualKeyMediaExcessAndMissingUsage(t *testing.T) {
	fx := newWMeterEnv(t)
	requireObjectStore(t, fx.app)
	for _, tc := range []struct {
		name           string
		missing, mixed bool
	}{
		{"actual excess", false, false}, {"missing remains held", true, false}, {"mixed price missing token usage", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snapshot, err := fx.app.Catalog.PriceSnapshot(fx.ctx, catalog.SeedanceModelID)
			if err != nil {
				t.Fatal(err)
			}
			if tc.mixed {
				original := append(json.RawMessage(nil), snapshot.Raw...)
				var rates map[string]any
				if err := json.Unmarshal(snapshot.Raw, &rates); err != nil {
					t.Fatal(err)
				}
				rates["input"], rates["output"], rates["reasoning"] = "0.000001", "0.000002", "0.000003"
				snapshot.Raw, _ = json.Marshal(rates)
				if err := fx.app.DB.Exec("UPDATE catalog_price_versions SET unit_prices_json=? WHERE id=?", string(snapshot.Raw), snapshot.VersionID).Error; err != nil {
					t.Fatal(err)
				}
				defer fx.app.DB.Exec("UPDATE catalog_price_versions SET unit_prices_json=? WHERE id=?", string(original), snapshot.VersionID)
			}
			quote, err := billing.ParseQuote(snapshot.VersionID, snapshot.Raw)
			if err != nil {
				t.Fatal(err)
			}
			reserve := billing.EstimateMediaReserveMinor(quote, 5, 0, "720p", true)
			reserve += quote.Charge(map[string]int{"prompt_tokens": (len("force-async") + 2) / 3, "completion_tokens": 256, "reasoning_tokens": 256}, "")
			p, key, _ := accountingUser(t, fx, "", "THA1", reserve)
			if err := fx.app.Billing.Credit(fx.ctx, p.UserID, id.New("credit"), reserve, ""); err != nil {
				t.Fatal(err)
			}
			job := postAccepted(t, fx.server.URL+"/v1/videos", key.Secret, id.New("media-op"), map[string]any{"model": catalog.SeedanceModelID, "prompt": "force-async", "duration": 5, "generate_audio": true})
			var saved struct {
				RequestID       string
				SupplierCosts   json.RawMessage `gorm:"column:supplier_costs_json"`
				UpstreamModelID string
			}
			if err := fx.app.DB.Table("media_jobs").Where("id=?", job["id"]).First(&saved).Error; err != nil {
				t.Fatal(err)
			}
			if len(saved.SupplierCosts) == 0 || string(saved.SupplierCosts) == "{}" || saved.UpstreamModelID == "" {
				t.Fatalf("accepted candidate terms missing %+v", saved)
			}
			// Remove the current price after admission: callbacks must use authorization
			// prices, never fail because the catalog no longer has a published price.
			if err := fx.app.DB.Exec("UPDATE catalog_price_versions SET status='retired' WHERE id=?", snapshot.VersionID).Error; err != nil {
				t.Fatal(err)
			}
			defer fx.app.DB.Exec("UPDATE catalog_price_versions SET status='published' WHERE id=?", snapshot.VersionID)
			var usage map[string]int
			if !tc.missing || tc.mixed {
				usage = map[string]int{"video_seconds": 7, "audio_seconds": 7}
			}
			eventID := id.New("evt")
			payload := map[string]any{"event_id": eventID, "job_id": job["id"], "usage": usage}
			sig := fx.app.Media.SignCallback(eventID, job["id"].(string))
			for i := 0; i < 2; i++ {
				result := postCallback(t, fx.server.URL+"/v1/media/callbacks", sig, payload)
				if result["ok"] != true {
					t.Fatalf("callback %+v", result)
				}
			}
			facts, err := fx.app.Billing.QueryUsage(fx.ctx, billing.QueryUsageInput{RequestID: saved.RequestID})
			if err != nil || len(facts) != 1 {
				t.Fatalf("media facts %+v %v", facts, err)
			}
			if tc.missing {
				if facts[0].State != billing.UsagePending || facts[0].CustomerMinor != 0 || string(facts[0].UnitUsage) == "{}" {
					t.Fatalf("missing media was estimated %+v", facts[0])
				}
				accountingBalance(t, fx, p, 0, 0, reserve)
				assertBudget(t, fx.app.Identity, fx.ctx, *p, fx.app.Config.EncryptionKey, key.ID, 0, reserve)
			} else {
				actual := quote.Charge(usage, "720p")
				if actual <= reserve || facts[0].State != billing.UsageConfirmed || facts[0].CustomerMinor != actual || facts[0].UpstreamMinor <= 0 {
					t.Fatalf("actual media charge/cost %+v estimate=%d actual=%d", facts[0], reserve, actual)
				}
				accountingBalance(t, fx, p, reserve-actual, 0, 0)
				assertBudget(t, fx.app.Identity, fx.ctx, *p, fx.app.Config.EncryptionKey, key.ID, actual, 0)
				if _, err := fx.app.Billing.RefundCharge(fx.ctx, saved.RequestID); err != nil {
					t.Fatal(err)
				}
				accountingBalance(t, fx, p, reserve, 0, 0)
				var costs struct {
					Count  int64
					Amount int64
				}
				if err := fx.app.DB.Table("billing_cost_entries").Select("COUNT(*) AS count, COALESCE(SUM(amount_minor),0) AS amount").Where("request_id=?", saved.RequestID).Scan(&costs).Error; err != nil {
					t.Fatal(err)
				}
				if costs.Count != 1 || costs.Amount != facts[0].UpstreamMinor {
					t.Fatalf("media cost missing, duplicate or removed by refund %+v usage=%d", costs, facts[0].UpstreamMinor)
				}
			}
		})
	}
}

func TestActualKeyUnknownPriceIsConfigurationErrorForEveryKey(t *testing.T) {
	fx := newWMeterEnv(t)
	p, key, _ := accountingUser(t, fx, "", "THA1", 1000000)
	if err := fx.app.Billing.Credit(fx.ctx, p.UserID, id.New("credit"), 1000000, ""); err != nil {
		t.Fatal(err)
	}
	unlimited, err := fx.app.Identity.CreateAPIKeyWithLimits(fx.ctx, *p, fx.app.Config.EncryptionKey, identity.APIKeyLimits{Name: "unlimited", ModelMode: "all"})
	if err != nil {
		t.Fatal(err)
	}
	var original struct{ CustomerOverrideJSON json.RawMessage }
	where := "channel_org_id=? AND public_model_id=(SELECT id FROM catalog_public_models WHERE public_id=?)"
	if err := fx.app.DB.Table("catalog_channel_model_policies").Select("customer_override_json").Where(where, p.ChannelOrgID, catalog.EchoModelID).Scan(&original).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		var value any
		if len(original.CustomerOverrideJSON) > 0 {
			value = string(original.CustomerOverrideJSON)
		}
		fx.app.DB.Table("catalog_channel_model_policies").Where(where, p.ChannelOrgID, catalog.EchoModelID).Update("customer_override_json", value)
	})
	if err := fx.app.DB.Table("catalog_channel_model_policies").Where(where, p.ChannelOrgID, catalog.EchoModelID).Update("customer_override_json", `{"cache_read":"1"}`).Error; err != nil {
		t.Fatal(err)
	}
	for _, item := range []*identity.APIKeyView{key, unlimited} {
		code, body := doJSON(t, http.MethodPost, fx.server.URL+"/v1/chat/completions", item.Secret, false, map[string]any{"model": catalog.EchoModelID, "messages": []map[string]string{{"role": "user", "content": "unknown price"}}})
		if code != 400 || body["error"].(map[string]any)["code"] != "price_estimate_unavailable" {
			t.Fatalf("unknown price key=%s %d %+v", item.Name, code, body)
		}
	}
	accountingBalance(t, fx, p, 1000000, 0, 0)
	assertBudget(t, fx.app.Identity, fx.ctx, *p, fx.app.Config.EncryptionKey, key.ID, 0, 0)
	model, err := fx.app.Catalog.GetVisibleModel(fx.ctx, p.ChannelOrgID, catalog.EchoModelID, nil)
	if err != nil || model.Capabilities["budget_estimate_supported"] != false {
		t.Fatalf("unknown estimate capability %+v %v", model, err)
	}
}

func TestActualKeyAccountReservationsAcrossKeysAndPlanSources(t *testing.T) {
	fx := newWMeterEnv(t)
	p, key, _ := accountingUser(t, fx, "", "THA1", 1000000)
	second, err := fx.app.Identity.CreateAPIKeyWithLimits(fx.ctx, *p, fx.app.Config.EncryptionKey, identity.APIKeyLimits{Name: "second", ModelMode: "all"})
	if err != nil {
		t.Fatal(err)
	}
	if err := fx.app.Billing.Credit(fx.ctx, p.UserID, id.New("credit"), 100000, ""); err != nil {
		t.Fatal(err)
	}
	requests := []string{id.New("req"), id.New("req")}
	keys := []*identity.APIKeyView{key, second}
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for i := range keys {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, err := fx.app.Billing.Reserve(fx.ctx, billing.ReserveInput{UserID: p.UserID, ChannelOrgID: p.ChannelOrgID, APIKeyID: keys[i].ID, RequestID: requests[i], PublicModelID: catalog.EchoModelID, ReserveMinor: 60000, UnitPrices: accountingPrices})
			errs <- err
		}(i)
	}
	close(start)
	wg.Wait()
	close(errs)
	success, denied := 0, 0
	for err := range errs {
		if err == nil {
			success++
		} else if errors.Is(err, billing.ErrInsufficientBalance) {
			denied++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || denied != 1 {
		t.Fatalf("account concurrency success=%d denied=%d", success, denied)
	}
	accountingBalance(t, fx, p, 40000, 0, 60000)
	for _, request := range requests {
		if err := fx.app.Billing.Release(fx.ctx, request); err != nil {
			t.Fatal(err)
		}
	}
	accountingBalance(t, fx, p, 100000, 0, 0)

	if _, err := fx.app.Plans.GrantBonus(fx.ctx, "test", id.New("bonus"), p.UserID, "usd_credit", 1000000, time.Hour); err != nil {
		t.Fatal(err)
	}
	a := accountingReserve(t, fx, p, key, 100000, accountingPrices)
	b := accountingReserve(t, fx, p, second, 300000, accountingPrices)
	if _, err := fx.app.Billing.Settle(fx.ctx, billing.SettleInput{RequestID: a, Usage: map[string]int{"prompt_tokens": 1100000, "completion_tokens": 0}}); err != nil {
		t.Fatal(err)
	}
	// A may extend to the free 600000 credits, but B's 300000 stays its own.
	accountingBalance(t, fx, p, -300000, 0, 0)
	var source struct{ EntitlementSettledMinor int64 }
	if err := fx.app.DB.Table("billing_authorizations").Where("request_id=?", a).First(&source).Error; err != nil {
		t.Fatal(err)
	}
	if source.EntitlementSettledMinor != 700000 {
		t.Fatalf("stole other request's plan coverage %+v", source)
	}
	if err := fx.app.Billing.Release(fx.ctx, b); err != nil {
		t.Fatal(err)
	}
	if remaining, err := fx.app.Plans.AvailableUSD(fx.ctx, p.UserID); err != nil || remaining != 300000 {
		t.Fatalf("other plan reservation lost %d %v", remaining, err)
	}
	if _, err := fx.app.Billing.RefundCharge(fx.ctx, a); err != nil {
		t.Fatal(err)
	}
	accountingBalance(t, fx, p, 100000, 0, 0)
	if remaining, err := fx.app.Plans.AvailableUSD(fx.ctx, p.UserID); err != nil || remaining != 1000000 {
		t.Fatalf("actual plan refund %d %v", remaining, err)
	}
}
