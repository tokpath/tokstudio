package app_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/billing"
	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/ops"
	"github.com/tokpath/tokstudio/backend/internal/plans"
	"github.com/tokpath/tokstudio/backend/internal/platform/mail"
	"gorm.io/gorm"
)

func TestRenewalFailureHasNoGraceAndOtherCreditStillWorks(t *testing.T) {
	t.Setenv("TOKENHUB_ENCRYPTION_KEY", "synthetic-notification-encryption-key")
	a, server := newOAuthEnv(t)
	resetNotificationFixture(t, a.DB)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	session, err := a.Identity.Register(ctx, identity.RegisterInput{BrandID: identity.OfficialBrandID, Email: fmt.Sprintf("no-grace-%d@example.test", now.UnixNano()), Password: "synthetic-password"})
	if err != nil {
		t.Fatal(err)
	}
	sub, err := a.Plans.CreateSubscription(ctx, session.User.ID, identity.OfficialChannelID, "pln_echo_month", "stripe", "pm_fail", identity.OfficialChannelID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Plans.ActivatePaid(ctx, sub.ID, now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	end := now.Add(-time.Second)
	if err := a.Plans.ForcePeriodEnd(ctx, sub.ID, end); err != nil {
		t.Fatal(err)
	}
	if err := a.DB.Exec("UPDATE plans_subscriptions SET grace_until=? WHERE id=?", now.Add(7*24*time.Hour), sub.ID).Error; err != nil {
		t.Fatal(err)
	}
	reserve := func(request string, amount int64) error {
		_, err := a.Billing.Reserve(ctx, billing.ReserveInput{UserID: session.User.ID, RequestID: request + session.User.ID, PublicModelID: catalog.EchoModelID, ReserveMinor: amount, UnitPrices: []byte(`{}`)})
		return err
	}
	// Admission is stopped at expiry even before the renewal worker runs.
	if err := reserve("expired", 1); !errors.Is(err, billing.ErrInsufficientBalance) {
		t.Fatalf("expired plan funded request: %v", err)
	}
	apiKey := postJSONRaw(t, server.URL+"/v1/me/api-keys", session.Token, map[string]any{"name": "no-grace-admission"})["item"].(map[string]any)["key"].(string)
	chatBody := map[string]any{"model": catalog.EchoModelID, "messages": []map[string]string{{"role": "user", "content": "synthetic-no-credit"}}}
	if code, _ := doJSON(t, "POST", server.URL+"/v1/chat/completions", apiKey, false, chatBody); code != http.StatusPaymentRequired {
		t.Fatalf("expired credit not rejected by gateway: %d", code)
	}
	var calls int
	fail := func(id, _, _ string) error {
		if id == sub.ID {
			calls++
		}
		return errors.New("synthetic decline")
	}
	if _, err := a.Plans.ProcessRenewals(ctx, now, fail); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Plans.ProcessRenewals(ctx, now.Add(time.Minute), fail); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Plans.ProcessRenewals(ctx, now.Add(time.Hour), fail); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("payment retried before next_retry_at: %d", calls)
	}
	var state struct {
		GraceUntil  *time.Time
		RetryCount  int
		NextRetryAt *time.Time
	}
	if err := a.DB.Table("plans_subscriptions").Where("id=?", sub.ID).Take(&state).Error; err != nil {
		t.Fatal(err)
	}
	if state.GraceUntil != nil || state.RetryCount != 1 || state.NextRetryAt == nil || !state.NextRetryAt.Equal(end.Add(24*time.Hour)) {
		t.Fatalf("wrong no-grace retry state: %+v", state)
	}
	subscriptions := getAuthJSON(t, server.URL+"/v1/me/subscriptions", session.Token)
	encoded, _ := json.Marshal(subscriptions)
	if strings.Contains(string(encoded), "grace_until") {
		t.Fatal("API exposes a grace date")
	}
	if err := reserve("still-expired", 1); !errors.Is(err, billing.ErrInsufficientBalance) {
		t.Fatalf("failed renewal extended access: %v", err)
	}
	if _, err := a.Plans.GrantBonus(ctx, session.User.ID, "valid-credit", session.User.ID, plans.UnitUSDCredit, 2*billing.MinorPerUSD, 24*time.Hour); err != nil {
		t.Fatal(err)
	}
	if err := reserve("other-valid-credit", billing.MinorPerUSD); err != nil {
		t.Fatalf("other valid credit blocked: %v", err)
	}
	if code, _ := doJSON(t, "POST", server.URL+"/v1/chat/completions", apiKey, false, chatBody); code != http.StatusOK {
		t.Fatalf("gateway blocked valid fallback credit: %d", code)
	}
	if err := a.Notifications.Consume(ctx); err != nil {
		t.Fatal(err)
	}
	page, err := a.Notifications.List(ctx, session.User.ID, identity.OfficialBrandID, "", false)
	if err != nil {
		t.Fatal(err)
	}
	failures := 0
	for _, item := range page.Items {
		if item.Kind == "renewal_failed" {
			failures++
			if !strings.Contains(item.Body, "没有宽限期") {
				t.Fatal("missing no-grace explanation")
			}
		}
	}
	if failures != 1 {
		t.Fatalf("failure notice repeated: %d", failures)
	}
	if _, err := a.Plans.ProcessRenewals(ctx, now.Add(24*time.Hour), fail); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("scheduled retry missed: %d", calls)
	}
	if err := a.Notifications.Consume(ctx); err != nil {
		t.Fatal(err)
	}
	page, err = a.Notifications.List(ctx, session.User.ID, identity.OfficialBrandID, "", false)
	if err != nil {
		t.Fatal(err)
	}
	failures = 0
	for _, item := range page.Items {
		if item.Kind == "renewal_failed" {
			failures++
		}
	}
	if failures != 1 {
		t.Fatal("scheduled retry duplicated customer notice")
	}
}

func TestOpsNotificationsRespectStaffScopeAndRecoveredAlerts(t *testing.T) {
	t.Setenv("TOKENHUB_ENCRYPTION_KEY", "synthetic-notification-encryption-key")
	a, _ := newOAuthEnv(t)
	resetNotificationFixture(t, a.DB)
	ctx := context.Background()
	admin, err := a.Identity.Authenticate(ctx, "Bearer oauth_admin")
	if err != nil {
		t.Fatal(err)
	}
	finance, err := a.Identity.Authenticate(ctx, "Bearer oauth_admin-finance")
	if err != nil {
		t.Fatal(err)
	}
	oem, err := a.Identity.Authenticate(ctx, "Bearer oauth_admin-c")
	if err != nil {
		t.Fatal(err)
	}
	first, second := "synthetic-provider-one-"+fmt.Sprint(time.Now().UnixNano()), "synthetic-provider-two-"+fmt.Sprint(time.Now().UnixNano())
	t.Cleanup(func() { a.Ops.ResetCircuit(ctx, first); a.Ops.ResetCircuit(ctx, second) })
	for _, provider := range []string{first, first, second} {
		if err := a.Ops.TripCircuit(ctx, provider, "synthetic failure"); err != nil {
			t.Fatal(err)
		}
	}
	var rows []struct{ ID, ScopeKey string }
	if err := a.DB.Table("ops_alerts").Where("kind=? AND scope_key IN ? AND status=?", ops.AlertCircuit, []string{first, second}, ops.StatusOpen).Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("provider incidents merged or repeated: %d", len(rows))
	}
	if err := a.Notifications.Consume(ctx); err != nil {
		t.Fatal(err)
	}
	for _, actor := range []struct {
		UserID, BrandID string
		Want            int
	}{{admin.UserID, admin.BrandID, 2}, {finance.UserID, finance.BrandID, 0}, {oem.UserID, oem.BrandID, 0}} {
		page, err := a.Notifications.List(ctx, actor.UserID, actor.BrandID, "", false)
		if err != nil {
			t.Fatal(err)
		}
		if len(page.Items) != actor.Want {
			t.Fatalf("staff scope leaked: %s wants %d got %d", actor.UserID, actor.Want, len(page.Items))
		}
	}
	if err := a.Ops.ResetCircuit(ctx, first); err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		open, err := a.Ops.NotificationAlertOpen(ctx, row.ID)
		if err != nil {
			t.Fatal(err)
		}
		if open != (row.ScopeKey == second) {
			t.Fatal("resolving one provider affected another")
		}
	}
	if err := a.Notifications.Consume(ctx); err != nil {
		t.Fatal(err)
	}
	if err := a.Notifications.Deliver(ctx); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := a.DB.Table("notifications_email_deliveries d").Joins("JOIN notifications_inbox n ON n.id=d.notification_id").Where("n.user_id=? AND n.kind=? AND n.body LIKE ? AND d.status=?", admin.UserID, "alert_opened", "%"+first+"%", "cancelled").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatal("resolved alert still queued for delivery")
	}
}

func TestBusinessEmailLeaseRecoveryExhaustionAndPreferences(t *testing.T) {
	t.Setenv("TOKENHUB_ENCRYPTION_KEY", "synthetic-notification-encryption-key")
	a, _ := newOAuthEnv(t)
	resetNotificationFixture(t, a.DB)
	ctx := context.Background()
	user, err := a.Identity.Register(ctx, identity.RegisterInput{BrandID: identity.OfficialBrandID, Email: fmt.Sprintf("lease-test-%d@example.test", time.Now().UnixNano()), Password: "synthetic-password"})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.DB.Exec("UPDATE identity_users SET email_verified_at=now() WHERE id=?", user.User.ID).Error; err != nil {
		t.Fatal(err)
	}
	mailer := &syntheticMailer{fail: true}
	a.Mail.SetSender(mailer)
	noticeID, err := a.Notifications.QueueTest(ctx, user.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	for range 5 {
		if err := a.Notifications.Deliver(ctx); err != nil {
			t.Fatal(err)
		}
		if err := a.DB.Exec("UPDATE notifications_email_deliveries SET available_at=now()-interval '1 minute' WHERE notification_id=?", noticeID).Error; err != nil {
			t.Fatal(err)
		}
	}
	var row struct {
		ID, Status, PayloadCiphertext string
		Attempts                      int
	}
	if err := a.DB.Table("notifications_email_deliveries").Where("notification_id=?", noticeID).Take(&row).Error; err != nil {
		t.Fatal(err)
	}
	if row.Status != "failed" || row.Attempts != 5 || row.PayloadCiphertext != "" {
		t.Fatal("exhausted attempts not terminal or payload retained")
	}
	if err := a.Notifications.Retry(ctx, row.ID); err != nil {
		t.Fatal(err)
	}
	// Simulate a crashed worker; a new worker must reclaim the expired lease.
	if err := a.DB.Exec("UPDATE notifications_email_deliveries SET status='sending',lease_id='synthetic-crashed-worker',available_at=now()-interval '1 minute' WHERE id=?", row.ID).Error; err != nil {
		t.Fatal(err)
	}
	mailer.mu.Lock()
	mailer.fail = false
	mailer.mu.Unlock()
	if err := a.Notifications.Deliver(ctx); err != nil {
		t.Fatal(err)
	}
	if err := a.DB.Table("notifications_email_deliveries").Where("id=?", row.ID).Take(&row).Error; err != nil {
		t.Fatal(err)
	}
	if row.Status != "accepted" {
		t.Fatal("expired lease not recovered")
	}
	if err := a.Notifications.SetEmailEnabled(ctx, user.User.ID, identity.OfficialBrandID, false); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Outbox.Enqueue(ctx, "billing.balance.insufficient", "preauth_failure", user.User.ID, map[string]any{"user_id": user.User.ID, "required_minor": 100}); err != nil {
		t.Fatal(err)
	}
	if err := a.Notifications.Consume(ctx); err != nil {
		t.Fatal(err)
	}
	page, err := a.Notifications.List(ctx, user.User.ID, identity.OfficialBrandID, "", false)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range page.Items {
		if item.Kind == "balance_insufficient" {
			found = true
			if item.EmailStatus != "cancelled" {
				t.Fatal("email opt-out ignored")
			}
		}
	}
	if !found {
		t.Fatal("opt-out removed inbox notice")
	}
	expiredID, err := a.Notifications.QueueTest(ctx, user.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.DB.Exec("UPDATE notifications_email_deliveries SET expires_at=now()-interval '1 minute' WHERE notification_id=?", expiredID).Error; err != nil {
		t.Fatal(err)
	}
	if err := a.Notifications.Deliver(ctx); err != nil {
		t.Fatal(err)
	}
	var status string
	if err := a.DB.Table("notifications_email_deliveries").Where("notification_id=?", expiredID).Pluck("status", &status).Error; err != nil {
		t.Fatal(err)
	}
	if status != "expired" {
		t.Fatal("expired mail sent")
	}
}

func TestNotificationsAreDurableBrandScopedAndRetrySafe(t *testing.T) {
	t.Setenv("TOKENHUB_ENCRYPTION_KEY", "synthetic-notification-encryption-key")
	a, server := newOAuthEnv(t)
	resetNotificationFixture(t, a.DB)
	ctx := context.Background()
	email := fmt.Sprintf("notification-brand-%d@example.test", time.Now().UnixNano())
	first, err := a.Identity.Register(ctx, identity.RegisterInput{BrandID: identity.OfficialBrandID, Email: email, Password: "synthetic-password"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := a.Identity.Register(ctx, identity.RegisterInput{BrandID: identity.OEMBrandID, Email: email, Password: "synthetic-password"})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.DB.Exec("UPDATE identity_users SET email_verified_at=now() WHERE id IN ?", []string{first.User.ID, second.User.ID}).Error; err != nil {
		t.Fatal(err)
	}
	for _, user := range []string{first.User.ID, second.User.ID} {
		if _, err := a.Outbox.Enqueue(ctx, "billing.balance.insufficient", "preauth_failure", user, map[string]any{"user_id": user, "required_minor": 100}); err != nil {
			t.Fatal(err)
		}
	}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- a.Notifications.Consume(ctx) }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	page, err := a.Notifications.List(ctx, first.User.ID, identity.OfficialBrandID, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Unread != 1 || page.Items[0].EmailStatus != "blocked_smtp" {
		t.Fatalf("wrong official inbox: %+v", page)
	}
	other, err := a.Notifications.List(ctx, second.User.ID, identity.OEMBrandID, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(other.Items) != 1 {
		t.Fatalf("OEM inbox: %+v", other)
	}
	if code, _ := doJSON(t, "POST", server.URL+"/v1/me/notifications/"+other.Items[0].ID+"/read", first.Token, false, map[string]any{}); code != 404 {
		t.Fatalf("foreign notice writable: %d", code)
	}
	if err := a.Notifications.Read(ctx, first.User.ID, identity.OfficialBrandID, page.Items[0].ID); err != nil {
		t.Fatal(err)
	}
	count, err := a.Notifications.Unread(ctx, first.User.ID, identity.OfficialBrandID)
	if err != nil || count != 0 {
		t.Fatalf("read count: %d %v", count, err)
	}
	var stored struct{ PayloadCiphertext string }
	if err := a.DB.Table("notifications_email_deliveries").Where("notification_id=?", page.Items[0].ID).Take(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if stored.PayloadCiphertext == "" || strings.Contains(stored.PayloadCiphertext, email) {
		t.Fatal("plaintext or missing encrypted payload")
	}
	mailer := &syntheticMailer{fail: true}
	a.Mail.SetSender(mailer)
	if err := a.Notifications.Deliver(ctx); err != nil {
		t.Fatal(err)
	}
	var delivery struct {
		ID, Status, LastError, PayloadCiphertext string
		Attempts                                 int
	}
	if err := a.DB.Table("notifications_email_deliveries").Where("notification_id=?", page.Items[0].ID).Take(&delivery).Error; err != nil {
		t.Fatal(err)
	}
	if delivery.Status != "pending" || delivery.Attempts != 1 || strings.Contains(delivery.LastError, "credentials") {
		t.Fatalf("unsafe failure state: %+v", delivery)
	}
	mailer.mu.Lock()
	mailer.fail = false
	mailer.mu.Unlock()
	if err := a.DB.Exec("UPDATE notifications_email_deliveries SET available_at=now()-interval '1 minute' WHERE notification_id IN ?", []string{page.Items[0].ID, other.Items[0].ID}).Error; err != nil {
		t.Fatal(err)
	}
	errs = make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- a.Notifications.Deliver(ctx) }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	mailer.mu.Lock()
	defer mailer.mu.Unlock()
	seen := map[string]bool{}
	for _, message := range mailer.messages {
		if message.To == email {
			if message.ID == "" {
				t.Fatal("missing stable message id")
			}
			seen[message.Brand] = true
		}
	}
	if len(seen) != 2 {
		t.Fatalf("same-email brands merged: %d", len(seen))
	}
	if err := a.DB.Table("notifications_email_deliveries").Where("notification_id=?", page.Items[0].ID).Take(&delivery).Error; err != nil {
		t.Fatal(err)
	}
	if delivery.Status != "accepted" || delivery.PayloadCiphertext != "" {
		t.Fatalf("SMTP result incorrectly recorded: %+v", delivery)
	}
	if err := a.Notifications.Retry(ctx, delivery.ID); err == nil {
		t.Fatal("accepted mail can be resent")
	}
}

func TestNotificationRemindersAndRecoveredConditions(t *testing.T) {
	t.Setenv("TOKENHUB_ENCRYPTION_KEY", "synthetic-notification-encryption-key")
	a, _ := newOAuthEnv(t)
	resetNotificationFixture(t, a.DB)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	session, err := a.Identity.Register(ctx, identity.RegisterInput{BrandID: identity.OfficialBrandID, Email: fmt.Sprintf("reminders-%d@example.test", now.UnixNano()), Password: "synthetic-password"})
	if err != nil {
		t.Fatal(err)
	}
	sub, err := a.Plans.CreateSubscription(ctx, session.User.ID, identity.OfficialChannelID, "pln_echo_month", "alipay", "", identity.OfficialChannelID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Plans.ActivatePaid(ctx, sub.ID, now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := a.Plans.ForcePeriodEnd(ctx, sub.ID, now.Add(48*time.Hour)); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := a.Plans.ProcessExpiryReminders(ctx, now); err != nil {
			t.Fatal(err)
		}
	}
	if err := a.Notifications.Consume(ctx); err != nil {
		t.Fatal(err)
	}
	page, err := a.Notifications.List(ctx, session.User.ID, identity.OfficialBrandID, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].Kind != "expiring_3d" || page.Items[0].EmailStatus != "blocked_unverified" {
		t.Fatalf("reminder dedupe/unverified state: %+v", page)
	}
	// Renewed before the queued reminder can be delivered: cancel it.
	if err := a.Plans.ForcePeriodEnd(ctx, sub.ID, now.Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Plans.ActivatePaid(ctx, sub.ID, now); err != nil {
		t.Fatal(err)
	}
	if err := a.DB.Exec("UPDATE identity_users SET email_verified_at=now() WHERE id=?", session.User.ID).Error; err != nil {
		t.Fatal(err)
	}
	a.Mail.SetSender(&syntheticMailer{})
	if err := a.Notifications.Deliver(ctx); err != nil {
		t.Fatal(err)
	}
	var status string
	if err := a.DB.Table("notifications_email_deliveries").Where("notification_id=?", page.Items[0].ID).Pluck("status", &status).Error; err != nil {
		t.Fatal(err)
	}
	if status != "cancelled" {
		t.Fatalf("obsolete reminder sent: %s", status)
	}
	// No SMTP publication dependency: consume a domain event still pending.
	if _, err := a.Outbox.Enqueue(ctx, "billing.balance.insufficient", "preauth_failure", session.User.ID, map[string]any{"user_id": session.User.ID, "required_minor": 1}); err != nil {
		t.Fatal(err)
	}
	if err := a.Notifications.Consume(ctx); err != nil {
		t.Fatal(err)
	}
	if err := a.Notifications.Deliver(ctx); err != nil {
		t.Fatal(err)
	}
	var n int64
	if err := a.DB.Table("notifications_email_deliveries d").Joins("JOIN notifications_inbox n ON n.id=d.notification_id").Where("n.user_id=? AND n.kind=? AND d.status=?", session.User.ID, "balance_insufficient", "cancelled").Count(&n).Error; err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatal("recovered credit reminder not cancelled")
	}
}

func TestPlatformSMTPSettingsAreRestrictedAndSecretsNeverEcho(t *testing.T) {
	t.Setenv("TOKENHUB_ENCRYPTION_KEY", "synthetic-notification-encryption-key")
	a, server := newOAuthEnv(t)
	resetNotificationFixture(t, a.DB)
	t.Cleanup(func() { a.DB.Exec("DELETE FROM mail_settings WHERE id='platform'") })
	body := map[string]any{"enabled": true, "host": "smtp.example.test", "port": 587, "from": "notifications@example.test", "security": "starttls", "username": "synthetic-user", "password": "synthetic-private-smtp-password"}
	if code, _ := doJSON(t, "PUT", server.URL+"/admin/notifications/smtp", "oauth_user", true, body); code != http.StatusForbidden {
		t.Fatalf("user configured SMTP: %d", code)
	}
	if code, _ := doJSON(t, "PUT", server.URL+"/admin/notifications/smtp", "oauth_admin", false, body); code != 409 {
		t.Fatalf("configuration lacked confirmation: %d", code)
	}
	code, result := doJSON(t, "PUT", server.URL+"/admin/notifications/smtp", "oauth_admin-tech", true, body)
	if code != 200 {
		t.Fatalf("technician could not configure SMTP: %d", code)
	}
	encoded, _ := json.Marshal(result)
	if strings.Contains(string(encoded), body["password"].(string)) {
		t.Fatal("SMTP secret echoed")
	}
	settings, err := a.Mail.Settings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !settings.Available || !settings.PasswordSet || settings.Source != "database" {
		t.Fatalf("settings not shared dynamically: %+v", settings)
	}
	delete(body, "password")
	if code, _ := doJSON(t, "PUT", server.URL+"/admin/notifications/smtp", "oauth_admin", true, body); code != 200 {
		t.Fatalf("preserve password failed: %d", code)
	}
	if _, err := a.Mail.Update(context.Background(), mail.SettingsInput{Enabled: true, Host: "smtp.example.test", Port: 587, From: "notifications@example.test", Security: "plain"}); !errors.Is(err, mail.ErrSettings) {
		t.Fatalf("unsafe SMTP accepted: %v", err)
	}
	// Test sending is limited to the actor's own verified address.
	if code, _ := doJSON(t, "POST", server.URL+"/admin/notifications/smtp/test", "oauth_admin", false, map[string]any{}); code != 409 {
		t.Fatalf("unverified test target allowed: %d", code)
	}
	if code, _ := doJSON(t, "GET", server.URL+"/admin/notifications/deliveries", "oauth_admin-audit", false, nil); code != 200 {
		t.Fatalf("auditor cannot inspect safe delivery metadata: %d", code)
	}
}

// Start each scenario after unrelated historical fixtures, as a fresh consumer
// deployment does, and remove notification fixtures from earlier scenarios.
func resetNotificationFixture(t *testing.T, db *gorm.DB) {
	t.Helper()
	for _, query := range []string{
		"DELETE FROM notifications_email_deliveries", "DELETE FROM notifications_inbox", "DELETE FROM notifications_preferences",
		"UPDATE outbox_consumers SET started_at=now() WHERE name='notifications-v1'",
	} {
		if err := db.Exec(query).Error; err != nil {
			t.Fatal(err)
		}
	}
}

func TestOEMQuotaNotificationsUseAdminDomainAndRejectDisabledStaff(t *testing.T) {
	t.Setenv("TOKENHUB_ENCRYPTION_KEY", "synthetic-notification-encryption-key")
	a, _ := newOAuthEnv(t)
	resetNotificationFixture(t, a.DB)
	ctx := context.Background()
	actor, err := a.Identity.Authenticate(ctx, "Bearer oauth_admin-c")
	if err != nil {
		t.Fatal(err)
	}
	var previous struct{ EmailVerifiedAt *time.Time }
	if err := a.DB.Table("identity_users").Where("id=?", actor.UserID).Take(&previous).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		a.DB.Exec("UPDATE identity_users SET email_verified_at=? WHERE id=?", previous.EmailVerifiedAt, actor.UserID)
		a.DB.Exec("UPDATE identity_staff_members SET status='active' WHERE user_id=?", actor.UserID)
	})
	if err := a.DB.Exec("UPDATE identity_users SET email_verified_at=now() WHERE id=?", actor.UserID).Error; err != nil {
		t.Fatal(err)
	}
	quota, err := a.Billing.ChannelQuota(ctx, identity.OEMChannelID)
	if err != nil {
		t.Fatal(err)
	}
	original := quota.AvailableMinor
	t.Cleanup(func() {
		if current, err := a.Billing.ChannelQuota(ctx, identity.OEMChannelID); err == nil && current.AvailableMinor != original {
			a.Billing.GrantChannelQuota(ctx, identity.OEMChannelID, original-current.AvailableMinor, actor.UserID)
		}
	})
	if original > 0 {
		if _, err := a.Billing.GrantChannelQuota(ctx, identity.OEMChannelID, -original, actor.UserID); err != nil {
			t.Fatal(err)
		}
	}
	if err := a.DB.Exec("DELETE FROM billing_quota_notification_state WHERE owner_id=?", identity.OEMChannelID).Error; err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := a.Billing.ProcessQuotaNotifications(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if err := a.Notifications.Consume(ctx); err != nil {
		t.Fatal(err)
	}
	page, err := a.Notifications.List(ctx, actor.UserID, actor.BrandID, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].Kind != "quota_low" {
		t.Fatalf("quota transition repeated or missed: %+v", page)
	}
	mailer := &syntheticMailer{}
	a.Mail.SetSender(mailer)
	if err := a.Notifications.Deliver(ctx); err != nil {
		t.Fatal(err)
	}
	brand, err := a.Identity.BrandByID(ctx, actor.BrandID)
	if err != nil {
		t.Fatal(err)
	}
	matched := false
	mailer.mu.Lock()
	for _, message := range mailer.messages {
		if message.To == actor.Email {
			matched = true
			if !strings.Contains(message.Text, "https://"+brand.AdminDomain+"/channel/ledger") {
				t.Error("OEM staff email points to the wrong brand portal")
			}
		}
	}
	mailer.mu.Unlock()
	if !matched {
		t.Fatal("OEM quota email not sent")
	}
	if _, err := a.Billing.GrantChannelQuota(ctx, identity.OEMChannelID, 2*billing.MinorPerUSD, actor.UserID); err != nil {
		t.Fatal(err)
	}
	if err := a.Billing.ProcessQuotaNotifications(ctx); err != nil {
		t.Fatal(err)
	}
	if err := a.Notifications.Consume(ctx); err != nil {
		t.Fatal(err)
	}
	if err := a.DB.Exec("UPDATE identity_staff_members SET status='disabled' WHERE user_id=?", actor.UserID).Error; err != nil {
		t.Fatal(err)
	}
	if err := a.Notifications.Deliver(ctx); err != nil {
		t.Fatal(err)
	}
	page, err = a.Notifications.List(ctx, actor.UserID, actor.BrandID, "", false)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range page.Items {
		if item.Kind == "quota_recovered" {
			found = true
			if item.EmailStatus != "cancelled" {
				t.Fatal("disabled staff received queued email")
			}
		}
	}
	if !found {
		t.Fatal("quota recovery not recorded")
	}
}
