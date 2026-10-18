package portal

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"github.com/tokpath/tokstudio/backend/internal/platform/mail"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"testing"
)

func TestPostgresAutomaticLoginAndAuthenticatedWorkspaceChoice(t *testing.T) {
	f := setupPortal(t)
	status, out, _ := f.call(t, "POST", "/v1/auth/login", map[string]string{"email": "owner@example.test", "password": "owner-password-test"}, "")
	mustOK(t, status, out)
	if out["kind"] != "staff" {
		t.Fatal(out)
	}
	f.openRegistration(t)
	f.registered(t, "owner@example.test")
	// A different password unambiguously identifies the customer.
	status, out, _ = f.call(t, "POST", "/v1/auth/login", map[string]string{"email": "owner@example.test", "password": "customer-password-test"}, "")
	mustOK(t, status, out)
	if out["kind"] != "customer" {
		t.Fatal(out)
	}
	hash, e := organization.HashPassword("owner-password-test")
	if e != nil {
		t.Fatal(e)
	}
	if e = f.a.DB.Exec(`UPDATE organization_customer_credentials SET password_hash=?`, hash).Error; e != nil {
		t.Fatal(e)
	}
	jar, _ := cookiejar.New(nil)
	f.client = &http.Client{Jar: jar}
	var before, after int64
	f.a.DB.Table("portal_sessions").Count(&before)
	status, out, headers := f.call(t, "POST", "/v1/auth/login", map[string]string{"email": "owner@example.test", "password": "owner-password-test"}, "")
	mustOK(t, status, out)
	f.a.DB.Table("portal_sessions").Count(&after)
	if out["kind"] != "workspace_choice" || headers.Get("Set-Cookie") != "" || after != before {
		t.Fatal("choice issued a session", out)
	}
	status, out, _ = f.call(t, "POST", "/v1/auth/login", map[string]string{"email": "owner@example.test", "password": "wrong"}, "")
	if status != 401 || out["choices"] != nil {
		t.Fatal("unauthenticated identity disclosure", status, out)
	}
	f.login(t, "customer", "owner@example.test", "owner-password-test")
	f.login(t, "staff", "owner@example.test", "owner-password-test")
}
func TestPostgresManualStaffOnboardingAndInitialPassword(t *testing.T) {
	f := setupPortal(t)
	ctx := context.Background()
	f.login(t, "staff", "owner@example.test", "owner-password-test")
	op := f.a.Installation.OperatorID
	input := map[string]any{"email": "managed@example.test", "role": "support", "display_name": "Managed colleague", "initial_password": "initial-password-test", "delivery_mode": "manual"}
	key := uuid.NewString()
	status, out, _ := f.call(t, "POST", "/v1/operators/"+op+"/staff", input, key)
	mustOK(t, status, out)
	if out["requires_password_change"] != true || out["status"] != "account_created" {
		t.Fatal(out)
	}
	staffID := out["id"].(string)
	status, replay, _ := f.call(t, "POST", "/v1/operators/"+op+"/staff", input, key)
	mustOK(t, status, replay)
	if replay["id"] != staffID {
		t.Fatal("retry created a second account")
	}
	var staff Staff
	f.a.DB.First(&staff, "id=?", staffID)
	if staff.EmailVerified || !staff.MustChangePassword {
		t.Fatal("manual provisioning must not pretend to verify mailbox", staff.EmailVerified)
	}
	var messages int64
	f.a.DB.Table("portal_messages").Count(&messages)
	if messages != 0 {
		t.Fatal("manual creation requires mail", messages)
	}
	var commands []commandRow
	f.a.DB.Find(&commands)
	saved, _ := json.Marshal(commands)
	if strings.Contains(string(saved), "initial-password-test") {
		t.Fatal("password persisted in command response")
	}
	f.login(t, "staff", "managed@example.test", "initial-password-test")
	status, out, _ = f.call(t, "GET", "/v1/operators/"+op+"/customers", nil, "")
	if status != 403 {
		t.Fatal("initial password bypass", status, out)
	}
	key = uuid.NewString()
	passwordInput := map[string]string{"current_password": "initial-password-test", "new_password": "private-password-test"}
	status, out, _ = f.call(t, "POST", "/v1/auth/staff-password", passwordInput, key)
	mustOK(t, status, out)
	status, out, _ = f.call(t, "POST", "/v1/auth/staff-password", passwordInput, key)
	mustOK(t, status, out)
	status, out, _ = f.call(t, "GET", "/v1/operators/"+op+"/customers", nil, "")
	mustOK(t, status, out)
	status, out, _ = f.call(t, "POST", "/v1/auth/login", map[string]string{"email": "managed@example.test", "password": "initial-password-test"}, "")
	if status != 401 {
		t.Fatal("old password remained valid", status, out)
	}
	f.login(t, "staff", "managed@example.test", "private-password-test")
	// The same global staff email joining another operator is invited, never reset.
	f.login(t, "staff", "owner@example.test", "owner-password-test")
	child, e := f.a.Organization.CreateOperator(ctx, nil, organization.CreateOperatorInput{ActorStaffID: f.a.Installation.OwnerStaffID, ParentOperatorID: op, Kind: organization.Channel, Name: "New channel", LegalName: "New channel legal"})
	if e != nil {
		t.Fatal(e)
	}
	input["role"] = "owner"
	input["initial_password"] = "replacement-password-test"
	status, out, _ = f.call(t, "POST", "/v1/operators/"+op+"/operators/"+child.Operator.ID+"/invite_owner", input, "")
	mustOK(t, status, out)
	if out["status"] != "invitation_created" || out["requires_password_change"] == true {
		t.Fatal("existing account replaced", out)
	}
	link, e := url.Parse(out["access_url"].(string))
	if e != nil {
		t.Fatal(e)
	}
	status, out, _ = f.call(t, "POST", "/v1/auth/staff-invitations/accept", map[string]string{"token": link.Query().Get("token"), "password": "replacement-password-test"}, "")
	if status != 401 {
		t.Fatal(status, out)
	}
	status, out, _ = f.call(t, "POST", "/v1/auth/staff-invitations/accept", map[string]string{"token": link.Query().Get("token"), "password": "private-password-test"}, "")
	mustOK(t, status, out)
	if e = f.a.Organization.RequirePermission(ctx, nil, staffID, child.Operator.ID, "*"); e != nil {
		t.Fatal(e)
	}
	f.a.DB.First(&staff, "id=?", staffID)
	if !organization.CheckPassword(staff.PasswordHash, "private-password-test") || staff.EmailVerified {
		t.Fatal("existing credentials or verification changed")
	}
}

func TestPostgresStaffPasswordRecoveryWithoutIdentitySelection(t *testing.T) {
	f := setupPortal(t)
	f.login(t, "staff", "owner@example.test", "owner-password-test")
	status, out, _ := f.call(t, "POST", "/v1/auth/password-resets", map[string]string{"email": "owner@example.test"}, "")
	mustOK(t, status, out)
	if e := f.a.RunMailJobs(context.Background(), "staff-reset-mail", 5); e != nil {
		t.Fatal(e)
	}
	f.box.mu.Lock()
	messages := append([]mail.Message{}, f.box.messages...)
	f.box.mu.Unlock()
	var token string
	for _, message := range messages {
		for _, line := range strings.Split(message.Text, "\n") {
			if strings.HasPrefix(line, "http") {
				u, e := url.Parse(line)
				if e == nil {
					token = u.Query().Get("token")
				}
			}
		}
	}
	if token == "" {
		t.Fatal("staff-only account did not receive reset mail")
	}
	key := uuid.NewString()
	input := map[string]string{"token": token, "password": "recovered-password-test"}
	status, out, _ = f.call(t, "POST", "/v1/auth/password-resets/complete", input, key)
	mustOK(t, status, out)
	status, out, _ = f.call(t, "GET", "/v1/auth/me", nil, "")
	mustOK(t, status, out)
	if out["kind"] != "anonymous" {
		t.Fatal("old staff session survived reset", out)
	}
	status, out, _ = f.call(t, "POST", "/v1/auth/password-resets/complete", input, key)
	mustOK(t, status, out)
	status, out, _ = f.call(t, "POST", "/v1/auth/password-resets/complete", input, "")
	if status != 400 {
		t.Fatal("reset link reusable", status, out)
	}
	f.login(t, "staff", "owner@example.test", "recovered-password-test")
}
