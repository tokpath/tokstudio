package portal

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"testing"
)

func TestSafeCustomerNext(t *testing.T) {
	cases := []struct{ raw, want string }{
		{"/workspace/models?model=public-model", "/workspace/models?model=public-model"},
		{"/workspace/plans?kol_activity=kol-one&plan=plan-one&invite=forged", "/workspace/plans?plan=plan-one&kol_activity=kol-one"},
		{"/workspace/playground?model=model-one&next=https%3A%2F%2Fevil.test", "/workspace/playground?model=model-one"},
		{"/workspace/models?plan=unrelated", "/workspace/models"},
		{"/workspace/plans?plan=first&plan=second", "/workspace/plans?plan=first"},
		{"/workspace/models?model=%5C%5Cevil.test", "/workspace/models"},
	}
	for _, raw := range []string{"", "https://evil.test/workspace/models", "//evil.test/workspace/plans", "/\\evil.test/workspace/models", "/workspace/models\\evil", "/workspace/plans/../models", "/workspace%2fmodels", "/workspace/models/", "/workspace/orders", "/manage/operator/plans", "/workspace/models#//evil", "/workspace/models?model=%ZZ", "/workspace/models?model=bad\nvalue"} {
		cases = append(cases, struct{ raw, want string }{raw, ""})
	}
	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			if got := safeCustomerNext(tc.raw); got != tc.want {
				t.Fatalf("safeCustomerNext(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

func TestPortalRegistrationContinuationTravelsInVerifiedMail(t *testing.T) {
	f := setupPortal(t)
	f.openRegistration(t)
	for i, next := range []string{"/workspace/plans?plan=plan-one&kol_activity=kol-one&invite=forged", "//evil.test/workspace/plans"} {
		status, out, _ := f.call(t, "POST", "/v1/auth/registrations", map[string]any{"email": fmt.Sprintf("continuation-%d@example.test", i), "password": "customer-password-test", "display_name": "Customer", "next": next}, "")
		mustOK(t, status, out)
		if err := f.a.RunMailJobs(context.Background(), "continuation-mail", 1); err != nil {
			t.Fatal(err)
		}
		f.box.mu.Lock()
		message := f.box.messages[len(f.box.messages)-1]
		f.box.mu.Unlock()
		var link *url.URL
		for _, line := range strings.Split(message.Text, "\n") {
			if strings.HasPrefix(line, "http") {
				var err error
				link, err = url.Parse(line)
				if err != nil {
					t.Fatal(err)
				}
			}
		}
		if link == nil || link.Query().Get("token") == "" {
			t.Fatal("verification mail missing its token")
		}
		if got, want := link.Query().Get("next"), safeCustomerNext(next); got != want {
			t.Fatalf("mail continuation %q, want %q", got, want)
		}
		// Verification needs no browser-local storage or session from registration.
		status, out, _ = f.call(t, "POST", "/v1/auth/verify", map[string]string{"token": link.Query().Get("token")}, "")
		mustOK(t, status, out)
	}
}
