package app_test

import (
	"context"
	"errors"
	"gorm.io/gorm"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/platform/config"
)

func TestPersonalReferralRegistrationAndIsolation(t *testing.T) {
	if os.Getenv("TOKENHUB_DATABASE_URL") == "" || os.Getenv("TOKENHUB_REDIS_URL") == "" {
		t.Skip("requires postgres and redis")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.BootstrapAdmin = "referral_admin"
	cfg.BootstrapUser = "referral_user"
	cfg.EncryptionKey = "dev-only-32-byte-key-change-me!!"
	application := mustApp(t, cfg)
	server := httptest.NewServer(application.Router())
	defer server.Close()
	register := func(promo string) map[string]any {
		return postBody(t, server.URL+"/v1/auth/register", "", map[string]string{"email": "ref-" + strconv.FormatInt(time.Now().UnixNano(), 10) + "@example.test", "password": "password1", "promotion_code": promo})
	}
	profile := func(token string) map[string]any {
		return getAuthJSON(t, server.URL+"/v1/me/referral", token)["item"].(map[string]any)
	}
	if code, _ := doJSON(t, http.MethodGet, server.URL+"/v1/me/referral", "", false, nil); code != 401 && code != 403 {
		t.Fatalf("anonymous status %d", code)
	}
	first := register("THA1")
	a := profile(tokenOf(first))
	if a["can_commission"] != false || len(a["codes"].([]any)) != 1 || a["can_create"] != false {
		t.Fatalf("new profile: %+v", a)
	}
	code := a["codes"].([]any)[0].(string)
	second := register(code)
	attr, err := application.Identity.GetAttribution(context.Background(), userIDOf(second))
	if err != nil {
		t.Fatal(err)
	}
	if attr.SourceCode != code || attr.AcquisitionRoleID == "" {
		t.Fatalf("lost invitation: %+v", attr)
	}
	rule, err := application.Identity.EffectiveEligibility(context.Background(), identity.OfficialChannelID)
	if err != nil {
		t.Fatal(err)
	}
	bal := getAuthJSON(t, server.URL+"/v1/balance", tokenOf(second))["balance"].(map[string]any)
	if asInt(bal["gift_minor"]) != rule.GiftMinor {
		t.Fatalf("new invitee gift: %+v", bal)
	}
	if asInt(profile(tokenOf(first))["invited_count"]) != 1 {
		t.Fatal("direct invite count missing")
	}
	b := profile(tokenOf(second))
	if asInt(b["invited_count"]) != 0 || b["codes"].([]any)[0] == code {
		t.Fatalf("personal isolation: %+v", b)
	}
	// Query parameters cannot select another user's or channel's data.
	foreign := getAuthJSON(t, server.URL+"/v1/me/referral?user_id="+userIDOf(first)+"&channel_id="+identity.OfficialChannelID, tokenOf(second))["item"].(map[string]any)
	if foreign["codes"].([]any)[0] != b["codes"].([]any)[0] {
		t.Fatal("query changed referral owner")
	}
	repeat := postBody(t, server.URL+"/v1/me/referral", tokenOf(first), map[string]string{})["item"].(map[string]any)
	if len(repeat["codes"].([]any)) != 1 || repeat["codes"].([]any)[0] != code {
		t.Fatal("repeat created a new code")
	}
	// Seed distinct commission facts to prove personal results exclude the invited promoter.
	for _, reg := range []map[string]any{first, second} {
		role, err := application.Identity.MemberRole(context.Background(), userIDOf(reg))
		if err != nil {
			t.Fatal(err)
		}
		entryID := "ref-test-" + userIDOf(reg)
		if err := application.DB.Exec(`INSERT INTO commission_entries (id,usage_event_id,beneficiary_role_id,kind,policy_version,base_amount_minor,raw_amount_minor,amount_minor,status,idempotency_key) VALUES (?,?,?,'direct','test',100,15,15,'frozen',?)`, entryID, entryID, role.ID, entryID).Error; err != nil {
			t.Fatal(err)
		}
	}
	ownRewards := profile(tokenOf(first))["rewards"].([]any)
	if len(ownRewards) != 1 || ownRewards[0].(map[string]any)["id"] != "ref-test-"+userIDOf(first) {
		t.Fatalf("commission owner isolation: %+v", ownRewards)
	}
	legacy := register("THA1")
	if err := application.DB.Exec("DELETE FROM identity_role_members WHERE user_id = ?", userIDOf(legacy)).Error; err != nil {
		t.Fatal(err)
	}
	if got := profile(tokenOf(legacy)); len(got["rewards"].([]any)) != 0 || got["can_create"] != true {
		t.Fatalf("legacy profile: %+v", got)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- application.Identity.CreatePersonalReferral(context.Background(), userIDOf(legacy))
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	repaired := profile(tokenOf(legacy))
	if len(repaired["codes"].([]any)) != 1 {
		t.Fatalf("concurrent creation: %+v", repaired)
	}
	role, err := application.Identity.MemberRole(context.Background(), userIDOf(legacy))
	if err != nil {
		t.Fatal(err)
	}
	if err := application.DB.Exec("UPDATE identity_acquisition_roles SET status = 'disabled' WHERE id = ?", role.ID).Error; err != nil {
		t.Fatal(err)
	}
	disabled := postBody(t, server.URL+"/v1/me/referral", tokenOf(legacy), map[string]string{})["item"].(map[string]any)
	if len(disabled["codes"].([]any)) != 0 || disabled["can_create"] != false {
		t.Fatalf("disabled membership bypass: %+v", disabled)
	}
	if _, err := application.Identity.Register(context.Background(), identity.RegisterInput{Email: "disabled-" + userIDOf(legacy) + "@example.test", Password: "password1", PromotionCode: repaired["codes"].([]any)[0].(string)}); !errors.Is(err, identity.ErrPromotionInvalid) {
		t.Fatalf("disabled invitation accepted: %v", err)
	}
	// A promo creation failure must roll back the whole new account, allowing a clean retry.
	injected := errors.New("test promo creation failure")
	callback := "referral-test-fail-promo"
	if err := application.DB.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "identity_promotion_codes" {
			tx.AddError(injected)
		}
	}); err != nil {
		t.Fatal(err)
	}
	email := "rollback-" + userIDOf(first) + "@example.test"
	_, regErr := application.Identity.Register(context.Background(), identity.RegisterInput{Email: email, Password: "password1", PromotionCode: "THA1"})
	application.DB.Callback().Create().Remove(callback)
	if regErr == nil {
		t.Fatal("promo failure must fail registration")
	}
	var count int64
	if err := application.DB.Table("identity_users").Where("email = ?", email).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("partial account persisted: count=%d err=%v", count, err)
	}
	if _, err := application.Identity.Register(context.Background(), identity.RegisterInput{Email: email, Password: "password1", PromotionCode: "THA1"}); err != nil {
		t.Fatalf("retry after rollback: %v", err)
	}
	// Accounts without a membership must not receive the global commission list.
	old := profile("referral_user")
	if len(old["codes"].([]any)) == 0 && len(old["rewards"].([]any)) != 0 {
		t.Fatal("non-member leaked commissions")
	}
}
