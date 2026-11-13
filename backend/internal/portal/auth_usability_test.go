package portal

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/branding"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	pc "github.com/tokpath/tokstudio/backend/internal/platform/crypto"
	"github.com/tokpath/tokstudio/backend/internal/platform/mail"
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

func TestPostgresReplacingFirstOwnerInvitationLeavesOneUsableLink(t *testing.T) {
	f := setupPortal(t)
	ctx := t.Context()
	parent, actor := f.a.Installation.OperatorID, f.a.Installation.OwnerStaffID
	child, err := f.a.Organization.CreateOperator(ctx, nil, organization.CreateOperatorInput{ActorStaffID: actor, ParentOperatorID: parent, Kind: organization.Channel, Name: "New channel", LegalName: "Channel legal"})
	if err != nil {
		t.Fatal(err)
	}
	f.login(t, "staff", "owner@example.test", "owner-password-test")
	path := "/v1/operators/" + parent + "/operators/" + child.Operator.ID + "/invite_owner"
	invite := func(email string) string {
		t.Helper()
		status, out, _ := f.call(t, "POST", path, map[string]string{"email": email, "delivery_mode": "email"}, "")
		mustOK(t, status, out)
		if err := f.a.RunMailJobs(ctx, "first-owner-mail", 5); err != nil {
			t.Fatal(err)
		}
		f.box.mu.Lock()
		defer f.box.mu.Unlock()
		for _, message := range f.box.messages {
			if message.To == email {
				for _, line := range strings.Split(message.Text, "\n") {
					if strings.HasPrefix(line, "http") {
						return queryValue(t, line, "token")
					}
				}
			}
		}
		t.Fatal("expected an owner invitation link")
		return ""
	}
	old := invite("first@example.test")
	newOwner := "replacement@example.test"
	current := invite(newOwner)
	accept := func(token string, want int) {
		t.Helper()
		status, out, _ := f.call(t, "POST", "/v1/auth/staff-invitations/accept", map[string]string{"token": token, "password": "new-owner-password-test"}, "")
		if status != want {
			t.Fatal(status, out)
		}
	}
	accept(old, 400)
	accept(current, 200)
	accept(old, 400)
	status, out, _ := f.call(t, "POST", path, map[string]string{"email": "another@example.test", "delivery_mode": "email"}, "")
	if status != 403 {
		t.Fatal("parent can still open a first owner", status, out)
	}
	f.login(t, "staff", newOwner, "new-owner-password-test")
	status, out, _ = f.call(t, "POST", "/v1/operators/"+child.Operator.ID+"/staff", map[string]string{"role": "owner", "email": "another@example.test", "initial_password": "additional-owner-test"}, "")
	mustOK(t, status, out)
}

func TestPostgresOEMPasswordRecoveryIsLimitedToItsBrandCustomer(t *testing.T) {
	f := setupPortal(t)
	ctx := t.Context()
	owner, platform := f.a.Installation.OwnerStaffID, f.a.Installation.OperatorID
	const email, original, updated = "owner@example.test", "customer-password-test", "new-oem-password-test"
	const host = "reset.oem.example.test"
	oem, err := f.a.Organization.CreateOperator(ctx, nil, organization.CreateOperatorInput{ActorStaffID: owner, ParentOperatorID: platform, Kind: organization.OEM, Name: "OEM merchant", BrandName: "OEM brand", LegalName: "OEM legal"})
	if err != nil {
		t.Fatal(err)
	}
	inv, err := f.a.Organization.InviteFirstOwner(ctx, nil, owner, oem.Operator.ID, email)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.a.Organization.AcceptAuthenticatedStaffInvitation(ctx, nil, inv.Token, owner, email); err != nil {
		t.Fatal(err)
	}
	domain, err := f.a.Branding.RequestDomain(ctx, nil, owner, oem.Brand.ID, host)
	if err != nil {
		t.Fatal(err)
	}
	verified, err := f.a.Branding.VerifyDomain(ctx, owner, domain.Domain.ID, branding.DomainVerifierFunc(func(context.Context, string, string) (branding.DomainVerification, error) {
		return branding.DomainVerification{OwnershipVerified: true, TLSReady: true, RouteReady: true, ProviderReference: "isolated DNS and TLS verification"}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.a.Branding.SetDomainEnabled(ctx, nil, owner, verified.ID, true, verified.Version); err != nil {
		t.Fatal(err)
	}
	for _, op := range []string{platform, oem.Operator.ID} {
		state, err := f.a.Organization.GetBusinessStatus(ctx, nil, op)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = f.a.Organization.SetBusinessStatus(ctx, nil, organization.SetBusinessStatusInput{ActorStaffID: owner, OperatorID: op, Feature: organization.Registration, Enabled: true, ExpectedVersion: state.Version, EvidenceReference: "test mail and default policy"}, f.a); err != nil {
			t.Fatal(err)
		}
	}
	f.registered(t, email)
	status, out, _ := f.callOnHost(t, host, "GET", "/v1/public/context", nil, "")
	mustOK(t, status, out)
	if out["staff_recovery_url"] != "http://127.0.0.1/forgot-password" {
		t.Fatal("missing platform recovery entry", out["staff_recovery_url"])
	}
	status, out, _ = f.callOnHost(t, host, "POST", "/v1/auth/registrations", map[string]string{"email": email, "password": original, "display_name": "OEM customer"}, "")
	mustOK(t, status, out)
	if err = f.a.RunMailJobs(ctx, "oem-registration", 5); err != nil {
		t.Fatal(err)
	}
	readToken := func(message mail.Message) string {
		t.Helper()
		for _, line := range strings.Split(message.Text, "\n") {
			if strings.HasPrefix(line, "http") {
				link, err := url.Parse(line)
				if err != nil || link.Host != host || message.BrandID != oem.Brand.ID || message.To != email {
					t.Fatalf("mail attribution: brand=%s recipient=%s error=%v", message.BrandID, message.To, err)
				}
				return link.Query().Get("token")
			}
		}
		t.Fatal("mail has no verification link")
		return ""
	}
	f.box.mu.Lock()
	message := f.box.messages[len(f.box.messages)-1]
	f.box.messages = nil
	f.box.mu.Unlock()
	status, out, _ = f.callOnHost(t, host, "POST", "/v1/auth/verify", map[string]string{"token": readToken(message)}, "")
	mustOK(t, status, out)
	f.login(t, "staff", email, "owner-password-test")
	status, out, _ = f.callOnHost(t, host, "POST", "/v1/auth/password-resets", map[string]string{"email": email}, "")
	mustOK(t, status, out)
	if err = f.a.RunMailJobs(ctx, "oem-password-reset", 5); err != nil {
		t.Fatal(err)
	}
	f.box.mu.Lock()
	messages := append([]mail.Message(nil), f.box.messages...)
	f.box.mu.Unlock()
	if len(messages) != 1 {
		t.Fatal("expected one customer reset message", len(messages))
	}
	input := map[string]string{"token": readToken(messages[0]), "password": updated}
	status, out, _ = f.call(t, "POST", "/v1/auth/password-resets/complete", input, "")
	if status != 400 {
		t.Fatal("OEM reset was accepted by platform", status, out)
	}
	status, out, _ = f.callOnHost(t, host, "POST", "/v1/auth/password-resets/complete", input, "")
	mustOK(t, status, out)
	// A staff reset issued before the scope fix must also be rejected.
	now := time.Now().UTC()
	proof := "legacy-test-proof"
	legacy := challenge{ID: uuid.NewString(), IntentID: owner, BrandID: oem.Brand.ID, Kind: "staff_password_reset", DisplayName: "1", ProofHash: pc.HashToken(proof), ExpiresAt: now.Add(time.Minute), CreatedAt: now}
	if err = f.a.DB.Create(&legacy).Error; err != nil {
		t.Fatal(err)
	}
	status, out, _ = f.callOnHost(t, host, "POST", "/v1/auth/password-resets/complete", map[string]string{"token": legacy.ID + "." + proof + ".staff", "password": updated}, "")
	if status != 400 {
		t.Fatal("legacy staff reset still usable", status, out)
	}
	status, out, _ = f.call(t, "GET", "/v1/auth/me", nil, "")
	mustOK(t, status, out)
	if out["kind"] != "staff" {
		t.Fatal("OEM recovery revoked the platform staff session", out)
	}
	f.login(t, "staff", email, "owner-password-test")
	f.login(t, "customer", email, original)
	status, out, _ = f.callOnHost(t, host, "POST", "/v1/auth/login", map[string]string{"kind": "customer", "email": email, "password": original}, "")
	if status != 401 {
		t.Fatal("old OEM customer password remains valid", status, out)
	}
	status, out, _ = f.callOnHost(t, host, "POST", "/v1/auth/login", map[string]string{"kind": "customer", "email": email, "password": updated}, "")
	mustOK(t, status, out)
}
