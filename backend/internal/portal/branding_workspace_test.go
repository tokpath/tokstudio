package portal

import (
	"context"
	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/branding"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPostgresOwnAppearanceAndPlatformDomainActivation(t *testing.T) {
	f := setupPortal(t)
	ctx := context.Background()
	roles := f.seedPaymentOperatorRoles(t)
	platform := f.a.Installation.OperatorID
	entries := append([]map[string]string{{"kind": "platform", "operator": platform, "email": "owner@example.test", "password": "owner-password-test"}}, roles...)
	for _, role := range entries {
		f.login(t, "staff", role["email"], role["password"])
		resource := "branding"
		wrong := "channel-themes"
		if role["kind"] == "channel" {
			resource, wrong = wrong, resource
		}
		base := "/v1/operators/" + role["operator"]
		status, page, _ := f.call(t, "GET", base+"/"+resource, nil, "")
		mustOK(t, status, page)
		rows := page["items"].([]any)
		if len(rows) != 1 || rows[0].(map[string]any)["id"] != role["operator"] {
			t.Fatal("appearance leaked child targets", page)
		}
		status, _, _ = f.call(t, "GET", base+"/"+wrong, nil, "")
		if status != 404 {
			t.Fatal("wrong appearance workspace", status)
		}
		input := map[string]any{"expected_version": 0, "values": map[string]any{"primary_color": "#123456"}}
		key := uuid.NewString()
		status, out, _ := f.call(t, "POST", base+"/"+resource, input, key)
		mustOK(t, status, out)
		status, out, _ = f.call(t, "POST", base+"/"+resource, input, key)
		mustOK(t, status, out)
		if out["version"] != float64(1) {
			t.Fatal("theme replay", out)
		}
		status, _, _ = f.call(t, "POST", base+"/"+resource, input, uuid.NewString())
		if status != 409 {
			t.Fatal("stale theme", status)
		}
		status, _, _ = f.call(t, "POST", base+"/"+resource+"?target_operator_id="+uuid.NewString(), input, uuid.NewString())
		if status != 403 {
			t.Fatal("foreign theme target", status)
		}
	}
	oem, _ := f.a.Organization.GetOperator(ctx, nil, roles[0]["operator"])
	f.login(t, "staff", roles[0]["email"], roles[0]["password"])
	base := "/v1/operators/" + oem.ID + "/domains"
	status, out, _ := f.call(t, "POST", base, map[string]any{"hostname": "own.example.test"}, uuid.NewString())
	mustOK(t, status, out)
	id := out["id"].(string)
	status, page, _ := f.call(t, "GET", base, nil, "")
	mustOK(t, status, page)
	for _, op := range page["row_operations"].([]any) {
		if op.(map[string]any)["name"] == "enable" { // metadata must also respect scope
			t.Fatal("OEM received platform activation control")
		}
	}
	f.a.DomainVerifier = ownershipVerifier{f: f, t: t}
	status, out, _ = f.call(t, "POST", base+"/"+id+"/verify", map[string]any{"version": 1}, uuid.NewString())
	mustOK(t, status, out)
	if out["status"] != "verified" {
		t.Fatal("verification activated business", out)
	}
	status, _, _ = f.call(t, "POST", base+"/"+id+"/enable", map[string]any{"version": out["version"]}, uuid.NewString())
	if status != 403 {
		t.Fatal("OEM enabled domain", status)
	}
	if _, e := f.a.Branding.ResolveHost(ctx, nil, "own.example.test"); e == nil {
		t.Fatal("unapproved hostname resolved")
	}
	f.login(t, "staff", "owner@example.test", "owner-password-test")
	status, page, _ = f.call(t, "GET", "/v1/operators/"+platform+"/domains", nil, "")
	mustOK(t, status, page)
	if len(page["items"].([]any)) < 1 {
		t.Fatal("OEM missing in platform domains")
	}
	status, _, _ = f.call(t, "POST", "/v1/operators/"+platform+"/domains", map[string]any{"target_operator_id": oem.ID, "hostname": "parent-edit.example.test"}, uuid.NewString())
	if status != 403 {
		t.Fatal("parent registered OEM hostname", status)
	}
	status, out, _ = f.call(t, "POST", "/v1/operators/"+platform+"/domains/"+id+"/enable", map[string]any{"version": out["version"]}, uuid.NewString())
	mustOK(t, status, out)
	if _, e := f.a.Branding.ResolveHost(ctx, nil, "own.example.test"); e != nil {
		t.Fatal(e)
	}
}

type ownershipVerifier struct {
	f *testApp
	t *testing.T
}

func (v ownershipVerifier) VerifyOwnership(context.Context, string, string) (bool, error) {
	return true, nil
}
func (v ownershipVerifier) Verify(ctx context.Context, host, token string) (branding.DomainVerification, error) {
	router := v.f.a.Router()
	r := httptest.NewRecorder()
	router.ServeHTTP(r, httptest.NewRequest("GET", "http://internal/v1/public/tls-check?domain="+host, nil))
	if r.Code != 200 {
		v.t.Fatal("first certificate permission depends on existing TLS", r.Code)
	}
	nonce := strings.Repeat("a", 64)
	r = httptest.NewRecorder()
	router.ServeHTTP(r, httptest.NewRequest("GET", "https://"+host+"/.well-known/tokenhub-domain?nonce="+nonce, nil))
	if r.Code != 200 || r.Body.String() != branding.DomainProof(v.f.a.Config.AppKey, host, token, nonce) {
		v.t.Fatal("preactivation brand routing proof", r.Code, r.Body.String())
	}
	r = httptest.NewRecorder()
	router.ServeHTTP(r, httptest.NewRequest("GET", "https://"+host+"/v1/public/context", nil))
	if r.Code == 200 {
		v.t.Fatal("ownership alone opened business access")
	}
	return branding.DomainVerification{OwnershipVerified: true, TLSReady: true, RouteReady: true, ProviderReference: "isolated-http-proof"}, nil
}

func operationFields(t *testing.T, page map[string]any, name string) map[string]map[string]any {
	t.Helper()
	for _, raw := range page["operations"].([]any) {
		action := raw.(map[string]any)
		if action["name"] != name {
			continue
		}
		fields := map[string]map[string]any{}
		for _, fieldRaw := range action["fields"].([]any) {
			field := fieldRaw.(map[string]any)
			fields[field["name"].(string)] = field
		}
		return fields
	}
	t.Fatal("operation missing", name)
	return nil
}
