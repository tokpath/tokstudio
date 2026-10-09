package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/identity"
)

func TestSignupBrandAndOAuthTaskIntent(t *testing.T) {
	application, server := newOAuthEnv(t)
	ctx := context.Background()
	brand, err := application.Identity.BrandByHost(ctx, "oem.localhost")
	if err != nil {
		t.Fatal(err)
	}
	register := func(promo string) (int, map[string]any) {
		email := "brand-signup-" + strconv.FormatInt(time.Now().UnixNano(), 10) + "@example.test"
		raw, _ := json.Marshal(map[string]string{"email": email, "password": "password1", "promotion_code": promo})
		req, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/auth/register", bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Forwarded-Host", brand.PrimaryDomain)
		res, err := server.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var body map[string]any
		if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		return res.StatusCode, body
	}
	status, registered := register("")
	if status != 201 {
		t.Fatalf("brand default registration %d %+v", status, registered)
	}
	user := registered["session"].(map[string]any)["user"].(map[string]any)
	if user["brand_id"] != brand.ID || user["channel_org_id"] != identity.OEMChannelID {
		t.Fatalf("signup lost OEM brand: %+v", user)
	}
	if status, _ := register("THA1"); status != 400 {
		t.Fatalf("cross-brand invitation accepted: %d", status)
	}
	if status, _ := register("THC1"); status != 201 {
		t.Fatalf("same-brand invitation rejected: %d", status)
	}
	application.Config.GoogleClientID = "test-client"
	application.Config.GoogleClientSecret = "test-secret"
	application.Config.GoogleRedirect = "https://" + identity.OfficialPrimaryDomain + "/login/oauth/google"
	request, _ := http.NewRequest(http.MethodGet, server.URL+"/v1/auth/google/status", nil)
	request.Header.Set("X-Forwarded-Host", brand.PrimaryDomain)
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	var state map[string]any
	json.NewDecoder(response.Body).Decode(&state)
	response.Body.Close()
	if state["available"] != false {
		t.Fatalf("cross-domain OAuth advertised: %+v", state)
	}
	for host, available := range map[string]bool{"localhost": true, "admin.localhost": false} {
		req, _ := http.NewRequest(http.MethodGet, server.URL+"/v1/auth/google/status", nil)
		req.Header.Set("X-Forwarded-Host", host)
		res, err := server.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		var body map[string]any
		json.NewDecoder(res.Body).Decode(&body)
		res.Body.Close()
		if body["available"] != available {
			t.Fatalf("host-only OAuth availability host=%s %+v", host, body)
		}
	}
	application.GoogleExchange = func(context.Context, string) (identity.GoogleProfile, error) {
		return identity.GoogleProfile{Subject: "intent-" + strconv.FormatInt(time.Now().UnixNano(), 10), Email: "oauth-intent-" + strconv.FormatInt(time.Now().UnixNano(), 10) + "@example.test"}, nil
	}
	goal := "/app/plans?plan=example&next=%2Fmodels%2Fexample%3Ftab%3Dagent"
	started := startOAuthJSON(t, server.URL+"/v1/auth/google/start?promotion_code=THA1&next="+url.QueryEscape(goal))
	if started["state"] == nil {
		t.Fatalf("Google start unavailable: %+v", started)
	}
	code, finished := callbackOAuthJSON(t, server.URL+"/v1/auth/google/callback", map[string]string{"state": started["state"].(string), "code": "synthetic-code", "next": "//evil.test"})
	if code != 200 || finished["session"].(map[string]any)["next"] != goal {
		t.Fatalf("server OAuth intent lost/spoofed: %d %+v", code, finished)
	}
	attr, err := application.Identity.GetAttribution(ctx, userIDOf(finished))
	if err != nil || attr.SourceCode != "THA1" {
		t.Fatalf("OAuth attribution: %+v %v", attr, err)
	}
	_, replayed := callbackOAuthJSON(t, server.URL+"/v1/auth/google/callback", map[string]string{"state": started["state"].(string), "code": "synthetic-code"})
	if replayed["session"].(map[string]any)["next"] != goal {
		t.Fatal("idempotent OAuth replay lost next")
	}
}
