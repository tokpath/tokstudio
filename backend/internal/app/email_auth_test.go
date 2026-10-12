package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/platform/mail"
)

type syntheticMailer struct {
	mu       sync.Mutex
	messages []mail.Message
	fail     bool
}

func (m *syntheticMailer) Send(_ context.Context, message mail.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fail {
		return errors.New("synthetic provider error containing private credentials")
	}
	m.messages = append(m.messages, message)
	return nil
}
func (m *syntheticMailer) last(t *testing.T, email, host, path string) string {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := len(m.messages) - 1; i >= 0; i-- {
		message := m.messages[i]
		if message.To != email {
			continue
		}
		for _, word := range strings.Fields(message.Text) {
			if !strings.HasPrefix(word, "https://") && !strings.HasPrefix(word, "http://") {
				continue
			}
			u, err := url.Parse(word)
			if err != nil || u.Host != host || u.Path != path {
				continue
			}
			params, _ := url.ParseQuery(u.Fragment)
			return params.Get("token")
		}
	}
	t.Fatalf("no synthetic email for %s %s %s", email, host, path)
	return ""
}

func emailHTTP(t *testing.T, endpoint, host, bearer string, body any) (int, map[string]any, *http.Response) {
	t.Helper()
	data, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", endpoint, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept-Language", "en")
	if host != "" {
		req.Host = host
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var result map[string]any
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, result, response
}

func TestEmailVerificationAndPasswordRecoveryAreBrandScoped(t *testing.T) {
	a, server := newOAuthEnv(t)
	ctx := context.Background()
	if status, _, _ := emailHTTP(t, server.URL+"/v1/auth/password/forgot", "", "", map[string]string{"email": "unavailable@example.test"}); status != 503 {
		t.Fatal("unconfigured email falsely accepted")
	}
	sender := &syntheticMailer{}
	a.Identity.SetEmailSender(sender, "synthetic-email-encryption-key")
	actor, err := a.Identity.Authenticate(ctx, "Bearer "+a.Config.BootstrapAdmin)
	if err != nil || actor == nil {
		t.Fatal(err)
	}
	marker := fmt.Sprint(time.Now().UnixNano())
	email := "recover-" + marker + "@example.test"
	create := func(name string) (string, string) {
		_, err := a.Identity.CreateOEM(ctx, *actor, identity.OEMCreateInput{OperationID: name + marker, SalesMode: "offline", Brand: &identity.BrandInput{Name: name, PrimaryDomain: name + marker + ".example.test", APIDomain: "api." + name + marker + ".example.test", AdminDomain: "admin." + name + marker + ".example.test"}})
		if err != nil {
			t.Fatal(err)
		}
		brand, err := a.Identity.BrandByHost(ctx, name+marker+".example.test")
		if err != nil {
			t.Fatal(err)
		}
		return brand.ID, brand.PrimaryDomain
	}
	brandA, hostA := create("email-a")
	brandB, hostB := create("email-b")
	register := func(host string) map[string]any {
		status, body, _ := emailHTTP(t, server.URL+"/v1/auth/register", host, "", map[string]string{"email": email, "password": "password-before"})
		if status != 201 || body["verification_email_queued"] != true {
			t.Fatalf("registration %d %+v", status, body)
		}
		return body
	}
	first, second := register(hostA), register(hostB)
	tokenA, tokenB := tokenOf(first), tokenOf(second)
	principalA, err := a.Identity.Authenticate(ctx, "Bearer "+tokenA)
	if err != nil || principalA == nil {
		t.Fatal(err)
	}
	apiKey, err := a.Identity.CreateAPIKey(ctx, *principalA, "recovery-preserved-key", "synthetic-email-encryption-key", nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Identity.DeliverEmails(ctx); err != nil {
		t.Fatal(err)
	}
	verifyA := sender.last(t, email, hostA, "/verify-email")
	verifyB := sender.last(t, email, hostB, "/verify-email")
	var clearSecrets int64
	a.DB.Table("identity_email_actions").Where("token_hash = ?", verifyA).Count(&clearSecrets)
	if clearSecrets != 0 {
		t.Fatal("raw recovery token stored")
	}
	if status, _, _ := emailHTTP(t, server.URL+"/v1/auth/email/verify", hostB, "", map[string]string{"token": verifyA}); status != 400 {
		t.Fatal("cross-brand verification accepted")
	}
	status, _, response := emailHTTP(t, server.URL+"/v1/auth/email/verify", hostA, "", map[string]string{"token": verifyA})
	if status != 200 || response.Header.Get("Set-Cookie") != "" {
		t.Fatal("verification created a session or failed")
	}
	if status, _, _ := emailHTTP(t, server.URL+"/v1/auth/email/verify", hostA, "", map[string]string{"token": verifyA}); status != 400 {
		t.Fatal("verification reused")
	}
	if err := a.DB.Table("identity_email_actions").Where("brand_id = ? AND purpose = ?", brandB, identity.EmailVerify).Update("expires_at", time.Now().Add(-time.Minute)).Error; err != nil {
		t.Fatal(err)
	}
	if status, _, _ := emailHTTP(t, server.URL+"/v1/auth/email/verify", hostB, "", map[string]string{"token": verifyB}); status != 400 {
		t.Fatal("expired verification accepted")
	}
	request := func(host, target string) map[string]any {
		status, body, _ := emailHTTP(t, server.URL+"/v1/auth/password/forgot", host, "", map[string]string{"email": target})
		if status != 202 {
			t.Fatalf("forgot %d %+v", status, body)
		}
		delete(body, "request_id")
		return body
	}
	known, unknown := request(hostA, email), request(hostA, "absent-"+marker+"@example.test")
	if fmt.Sprint(known) != fmt.Sprint(unknown) {
		t.Fatal("forgot response exposed account existence")
	}
	request(hostB, email)
	request(hostA, email)
	if err := a.Identity.DeliverEmails(ctx); err != nil {
		t.Fatal(err)
	}
	resetA := sender.last(t, email, hostA, "/reset-password")
	resetB := sender.last(t, email, hostB, "/reset-password")
	if status, _, _ := emailHTTP(t, server.URL+"/v1/auth/password/reset", hostB, "", map[string]string{"token": resetA, "new_password": "password-after"}); status != 400 {
		t.Fatal("cross-brand reset accepted")
	}
	if status, _, _ := emailHTTP(t, server.URL+"/v1/auth/password/reset", hostA, "", map[string]string{"token": resetA, "new_password": "short"}); status != 400 {
		t.Fatal("weak password accepted")
	}
	status, _, response = emailHTTP(t, server.URL+"/v1/auth/password/reset", hostA, "", map[string]string{"token": resetA, "new_password": "password-after"})
	if status != 200 || !strings.Contains(response.Header.Get("Set-Cookie"), "Max-Age=0") {
		t.Fatalf("reset %d %+v", status, response.Header)
	}
	if user, err := a.Identity.Authenticate(ctx, "Bearer "+tokenA); err != nil || user != nil {
		t.Fatal("old session survived password reset")
	}
	if user, err := a.Identity.Authenticate(ctx, "Bearer "+tokenB); err != nil || user == nil {
		t.Fatal("other brand session was revoked")
	}
	var activeKeys int64
	if err := a.DB.Table("identity_api_keys").Where("id = ? AND status = ?", apiKey.ID, "active").Count(&activeKeys).Error; err != nil || activeKeys != 1 {
		t.Fatal("password reset changed API key state")
	}
	if _, err := a.Identity.Login(ctx, identity.LoginInput{BrandID: brandA, Email: email, Password: "password-before"}); err == nil {
		t.Fatal("old password accepted")
	}
	if _, err := a.Identity.Login(ctx, identity.LoginInput{BrandID: brandA, Email: email, Password: "password-after"}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Identity.Login(ctx, identity.LoginInput{BrandID: brandB, Email: email, Password: "password-before"}); err != nil {
		t.Fatal("other brand password changed")
	}
	if _, err := a.Identity.CompleteEmailAction(ctx, brandA, identity.EmailReset, resetA, "password-again"); !errors.Is(err, identity.ErrEmailLink) {
		t.Fatal("reset replay accepted")
	}
	if err := a.Identity.DeliverEmails(ctx); err != nil {
		t.Fatal(err)
	}
	// A newly issued reset supersedes the previous link; concurrent consumption wins once.
	a.DB.Table("identity_email_actions").Where("brand_id = ? AND purpose = ?", brandB, identity.EmailReset).Update("created_at", time.Now().Add(-2*time.Minute))
	if err := a.Identity.RequestEmailAction(ctx, identity.EmailActionInput{BrandID: brandB, Email: email, Purpose: identity.EmailReset, Context: identity.EmailActionContext{Origin: "https://" + hostB}}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Identity.CompleteEmailAction(ctx, brandB, identity.EmailReset, resetB, "password-newer"); !errors.Is(err, identity.ErrEmailLink) {
		t.Fatal("superseded reset accepted")
	}
	if err := a.Identity.DeliverEmails(ctx); err != nil {
		t.Fatal(err)
	}
	newReset := sender.last(t, email, hostB, "/reset-password")
	results := make(chan error, 2)
	for range 2 {
		go func() {
			_, err := a.Identity.CompleteEmailAction(ctx, brandB, identity.EmailReset, newReset, "password-newer")
			results <- err
		}()
	}
	successes := 0
	for range 2 {
		err := <-results
		if err == nil {
			successes++
		} else if !errors.Is(err, identity.ErrEmailLink) {
			t.Fatal(err)
		}
	}
	if successes != 1 {
		t.Fatal("concurrent recovery was not one-use")
	}
}

func TestEmailQueueRetriesAndPasswordChangeInvalidatesReset(t *testing.T) {
	a, _ := newOAuthEnv(t)
	ctx := context.Background()
	sender := &syntheticMailer{fail: true}
	a.Identity.SetEmailSender(sender, "synthetic-email-encryption-key")
	email := fmt.Sprintf("retry-%d@example.test", time.Now().UnixNano())
	session, err := a.Identity.Register(ctx, identity.RegisterInput{BrandID: identity.OfficialBrandID, Email: email, Password: "before-password"})
	if err != nil {
		t.Fatal(err)
	}
	request := identity.EmailActionInput{BrandID: identity.OfficialBrandID, Email: email, Purpose: identity.EmailReset, Context: identity.EmailActionContext{Origin: "http://localhost:3000"}}
	if err := a.Identity.RequestEmailAction(ctx, request); err != nil {
		t.Fatal(err)
	}
	if err := a.Identity.DeliverEmails(ctx); err != nil {
		t.Fatal(err)
	}
	var row struct {
		Status, LastError, PayloadCiphertext string
		Attempts                             int
	}
	a.DB.Table("identity_email_deliveries").Where("user_id = ?", session.User.ID).First(&row)
	if row.Status != "pending" || row.Attempts != 1 || row.LastError != "delivery_failed" || row.PayloadCiphertext == "" {
		t.Fatalf("unsafe retry state %+v", row)
	}
	sender.fail = false
	a.DB.Table("identity_email_deliveries").Where("user_id = ?", session.User.ID).Update("available_at", time.Now().Add(-time.Minute))
	if err := a.Identity.DeliverEmails(ctx); err != nil {
		t.Fatal(err)
	}
	reset := sender.last(t, email, "localhost:3000", "/reset-password")
	a.DB.Table("identity_email_deliveries").Where("user_id = ?", session.User.ID).First(&row)
	if row.Status != "sent" || row.PayloadCiphertext != "" {
		t.Fatal("sent payload retained recovery secret")
	}
	principal, err := a.Identity.Authenticate(ctx, "Bearer "+session.Token)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Identity.ChangePassword(ctx, *principal, "before-password", "updated-password"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Identity.CompleteEmailAction(ctx, identity.OfficialBrandID, identity.EmailReset, reset, "attacker-password"); !errors.Is(err, identity.ErrEmailLink) {
		t.Fatal("reset survived password change")
	}
	a.DB.Table("identity_email_actions").Where("user_id = ?", session.User.ID).Update("created_at", time.Now().Add(-time.Hour))
	if err := a.Identity.RequestEmailAction(ctx, request); err != nil {
		t.Fatal(err)
	}
	sender.fail = true
	for range 5 {
		a.DB.Table("identity_email_deliveries").Where("user_id = ? AND status = ?", session.User.ID, "pending").Update("available_at", time.Now().Add(-time.Minute))
		if err := a.Identity.DeliverEmails(ctx); err != nil {
			t.Fatal(err)
		}
	}
	a.DB.Table("identity_email_deliveries").Where("user_id = ? AND status = ?", session.User.ID, "failed").First(&row)
	if row.Attempts != 5 || row.PayloadCiphertext != "" || row.LastError != "delivery_failed" {
		t.Fatal("final failure kept secret or exposed provider error")
	}
	// Expired payloads are erased even after SMTP has been disabled.
	a.DB.Table("identity_email_deliveries").Where("user_id = ? AND status = ?", session.User.ID, "failed").Updates(map[string]any{"status": "pending", "payload_ciphertext": "synthetic-expired-payload", "expires_at": time.Now().Add(-time.Minute)})
	a.Identity.SetEmailSender(nil, "")
	if err := a.Identity.DeliverEmails(ctx); err != nil {
		t.Fatal(err)
	}
	a.DB.Table("identity_email_deliveries").Where("user_id = ? AND status = ?", session.User.ID, "expired").First(&row)
	if row.Status != "expired" || row.PayloadCiphertext != "" {
		t.Fatal("unconfigured worker retained expired payload")
	}
}
