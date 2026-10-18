package portal

import (
	"context"
	"net/url"
	"testing"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/organization"
)

func queryValue(t *testing.T, raw, name string) string {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u.Query().Get(name)
}

func TestPostgresOfficialGoogleEnvIsAdoptedForItsRedirect(t *testing.T) {
	f := setupPortal(t)
	status, out, _ := f.call(t, "GET", "/v1/public/context", nil, "")
	mustOK(t, status, out)
	if out["google_sign_in"] != true {
		t.Fatal("official brand hid Google sign-in before a configuration existed", out["google_sign_in"])
	}
	redirect := f.server.URL + "/login/oauth/google"
	if err := f.a.Organization.AdoptOfficialGoogleEnv(context.Background(), f.a.Installation.BrandID, f.a.Config.AppKey, "official-env.apps.googleusercontent.com", "env-secret", redirect); err != nil {
		t.Fatal(err)
	}
	status, out, _ = f.call(t, "GET", "/v1/public/context", nil, "")
	mustOK(t, status, out)
	if out["google_sign_in"] != true {
		t.Fatal("official environment configuration was not offered", out["google_sign_in"])
	}
	if err := f.a.Organization.AdoptOfficialGoogleEnv(context.Background(), f.a.Installation.BrandID, f.a.Config.AppKey, "replacement.apps.googleusercontent.com", "other-secret", redirect); err != nil {
		t.Fatal(err)
	}
	view, err := f.a.Organization.GoogleConfig(context.Background(), nil, f.a.Installation.BrandID)
	if err != nil || view.ClientID != "official-env.apps.googleusercontent.com" || !view.Enabled {
		t.Fatal("restart replaced the adopted configuration", view, err)
	}
}

func TestPostgresGoogleSignInRegistersAndStaysInsideTheBrand(t *testing.T) {
	f := setupPortal(t)
	f.openRegistration(t)
	f.login(t, "staff", "owner@example.test", "owner-password-test")
	clientID := "brand-" + uuid.NewString() + ".apps.googleusercontent.com"
	status, out, _ := f.call(t, "POST", "/v1/operators/"+f.a.Installation.OperatorID+"/domains", map[string]any{"action": "google", "target_operator_id": f.a.Installation.OperatorID, "client_id": clientID, "client_secret": "secret-value", "enabled": true, "config_version": 0}, "")
	if status != 409 || out["error"].(map[string]any)["code"] != "google_not_verified" {
		t.Fatalf("untested google config enabled: %d %#v", status, out)
	}
	status, out, _ = f.call(t, "POST", "/v1/operators/"+f.a.Installation.OperatorID+"/domains", map[string]any{"action": "google", "target_operator_id": f.a.Installation.OperatorID, "client_id": clientID, "client_secret": "secret-value", "enabled": false, "config_version": 0}, "")
	mustOK(t, status, out)
	status, out, _ = f.call(t, "POST", "/v1/operators/"+f.a.Installation.OperatorID+"/domains", map[string]any{"action": "google_test", "target_operator_id": f.a.Installation.OperatorID}, "")
	mustOK(t, status, out)
	ticket := queryValue(t, out["access_url"].(string), "ticket")
	f.a.GoogleExchanger = func(_ context.Context, _ organization.GoogleCredential, code string) (organization.GoogleProfile, error) {
		if code == "test-code" {
			return organization.GoogleProfile{Subject: "google-subject-1", Email: "person@example.test", Name: "Google Person"}, nil
		}
		return organization.GoogleProfile{}, organization.ErrGoogleUnavailable
	}
	status, out, _ = f.call(t, "POST", "/v1/auth/google/test", map[string]any{"ticket": ticket}, "")
	mustOK(t, status, out)
	state := queryValue(t, out["url"].(string), "state")
	status, out, _ = f.call(t, "POST", "/v1/auth/google/finish", map[string]any{"code": "test-code", "state": state}, "")
	mustOK(t, status, out)
	if out["kind"] != "google_test" {
		t.Fatalf("test created a session: %#v", out)
	}
	status, out, _ = f.call(t, "POST", "/v1/operators/"+f.a.Installation.OperatorID+"/domains", map[string]any{"action": "google", "target_operator_id": f.a.Installation.OperatorID, "client_id": clientID, "enabled": true, "config_version": 1}, "")
	mustOK(t, status, out)
	status, out, _ = f.call(t, "GET", "/v1/public/context", nil, "")
	mustOK(t, status, out)
	if out["google_sign_in"] != true {
		t.Fatal("enabled google sign-in was hidden", out["google_sign_in"])
	}
	status, out, _ = f.call(t, "POST", "/v1/auth/google/start", map[string]any{}, "")
	mustOK(t, status, out)
	state = queryValue(t, out["url"].(string), "state")
	status, out, _ = f.call(t, "POST", "/v1/auth/google/finish", map[string]any{"code": "test-code", "state": state}, "")
	mustOK(t, status, out)
	if out["kind"] != "customer" || out["customer"].(map[string]any)["email"] != "person@example.test" {
		t.Fatalf("google registration: %#v", out)
	}
	first := out["customer"].(map[string]any)["id"].(string)
	status, out, _ = f.call(t, "POST", "/v1/auth/google/start", map[string]any{}, "")
	mustOK(t, status, out)
	state = queryValue(t, out["url"].(string), "state")
	status, out, _ = f.call(t, "POST", "/v1/auth/google/finish", map[string]any{"code": "test-code", "state": state}, "")
	mustOK(t, status, out)
	if out["customer"].(map[string]any)["id"] != first {
		t.Fatal("google login created a second customer", out)
	}
	f.registered(t, "password-owner@example.test")
	f.a.GoogleExchanger = func(_ context.Context, _ organization.GoogleCredential, code string) (organization.GoogleProfile, error) {
		return organization.GoogleProfile{Subject: "other-subject", Email: "password-owner@example.test", Name: "Taken"}, nil
	}
	status, out, _ = f.call(t, "POST", "/v1/auth/google/start", map[string]any{}, "")
	mustOK(t, status, out)
	state = queryValue(t, out["url"].(string), "state")
	status, out, _ = f.call(t, "POST", "/v1/auth/google/finish", map[string]any{"code": "taken", "state": state}, "")
	if status != 409 || out["error"].(map[string]any)["code"] != "google_email_exists" {
		t.Fatalf("password account was attached to Google: %d %#v", status, out)
	}
}
