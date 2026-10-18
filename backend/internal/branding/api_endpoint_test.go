package branding

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/organization"
)

func TestPostgresAPIEndpointUsesFirstAvailableVerifiedDomainAndRespectsDisable(t *testing.T) {
	f := setup(t)
	oem, _ := f.child(t, f.platform, f.owner, organization.OEM)
	f.s.now = func() time.Time { return time.Date(2026, 10, 18, 1, 0, 0, 0, time.UTC) }
	first, err := f.s.RequestDomain(f.ctx, nil, f.owner, oem.Brand.ID, "first.example.test")
	if err != nil {
		t.Fatal(err)
	}
	f.s.now = func() time.Time { return time.Date(2026, 10, 18, 2, 0, 0, 0, time.UTC) }
	second, err := f.s.RequestDomain(f.ctx, nil, f.owner, oem.Brand.ID, "second.example.test")
	if err != nil {
		t.Fatal(err)
	}
	assertEndpoint := func(want string) {
		t.Helper()
		got, err := f.s.APIEndpoint(f.ctx, nil, oem.Brand.ID)
		if err != nil || got != want {
			t.Fatalf("endpoint=%q want=%q err=%v", got, want, err)
		}
	}
	// Legacy free-text URLs and pending domain evidence cannot become a usable
	// customer address. Verifier doubles here assert persistence boundaries only.
	if err = f.db.Exec("UPDATE organization_brands SET api_base_url=? WHERE id=?", "https://unverified.invalid/api/v1", oem.Brand.ID).Error; err != nil {
		t.Fatal(err)
	}
	assertEndpoint("")
	ready := DomainVerifierFunc(func(context.Context, string, string) (DomainVerification, error) {
		return DomainVerification{OwnershipVerified: true, TLSReady: true, ProviderReference: "isolated-provider-evidence"}, nil
	})
	secondActive, err := f.s.VerifyAndActivateDomain(f.ctx, f.owner, second.Domain.ID, ready)
	if err != nil {
		t.Fatal(err)
	}
	assertEndpoint("https://second.example.test/v1")
	firstActive, err := f.s.VerifyAndActivateDomain(f.ctx, f.owner, first.Domain.ID, ready)
	if err != nil {
		t.Fatal(err)
	}
	assertEndpoint("https://first.example.test/v1")
	stopped, err := f.s.SetDomainEnabled(f.ctx, nil, f.owner, firstActive.ID, false, firstActive.Version)
	if err != nil {
		t.Fatal(err)
	}
	assertEndpoint("https://second.example.test/v1")
	rechecked, err := f.s.VerifyAndActivateDomain(f.ctx, f.owner, stopped.ID, ready)
	if err != nil || rechecked.Status != "disabled" || rechecked.Version != stopped.Version+1 {
		t.Fatalf("checking a stopped domain changed enablement: %+v %v", rechecked, err)
	}
	if _, err = f.s.ResolveHost(f.ctx, nil, stopped.Hostname); !errors.Is(err, ErrNotFound) {
		t.Fatalf("stopped host resolved after recheck: %v", err)
	}
	assertEndpoint("https://second.example.test/v1")
	if _, err = f.s.SetDomainEnabled(f.ctx, nil, f.owner, secondActive.ID, false, secondActive.Version); err != nil {
		t.Fatal(err)
	}
	assertEndpoint("")
	if _, err = f.s.SetDomainEnabled(f.ctx, nil, f.owner, rechecked.ID, true, rechecked.Version); err != nil {
		t.Fatal(err)
	}
	assertEndpoint("https://first.example.test/v1")
	if endpoint, err := f.s.APIEndpoint(f.ctx, nil, f.platform.Brand.ID); err != nil || endpoint != "" {
		t.Fatalf("another brand inherited the OEM endpoint: %q %v", endpoint, err)
	}
	if err = f.db.Exec("UPDATE organization_brands SET status='suspended' WHERE id=?", oem.Brand.ID).Error; err != nil {
		t.Fatal(err)
	}
	assertEndpoint("")
}
