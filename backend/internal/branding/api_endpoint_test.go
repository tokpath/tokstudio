package branding

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/organization"
)

func TestPostgresAPIEndpointSwitchRequiresPlatformApproval(t *testing.T) {
	f := setup(t)
	oem, oemOwner := f.child(t, f.platform, f.owner, organization.OEM)
	f.s.now = func() time.Time { return time.Date(2026, 10, 18, 1, 0, 0, 0, time.UTC) }
	first, err := f.s.RequestDomain(f.ctx, nil, oemOwner, oem.Brand.ID, "first.example.test")
	if err != nil {
		t.Fatal(err)
	}
	f.s.now = func() time.Time { return time.Date(2026, 10, 18, 2, 0, 0, 0, time.UTC) }
	second, err := f.s.RequestDomain(f.ctx, nil, oemOwner, oem.Brand.ID, "second.example.test")
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
	// A requested domain is not a customer address until ownership and TLS are verified.
	assertEndpoint("")
	ready := DomainVerifierFunc(func(context.Context, string, string) (DomainVerification, error) {
		return DomainVerification{OwnershipVerified: true, TLSReady: true, RouteReady: true, ProviderReference: "isolated-provider-evidence"}, nil
	})
	secondReady, err := f.s.VerifyDomain(f.ctx, oemOwner, second.Domain.ID, ready)
	if err != nil {
		t.Fatal(err)
	}
	assertEndpoint("")
	if _, err = f.s.SetDomainEnabled(f.ctx, nil, oemOwner, secondReady.ID, true, secondReady.Version); !errors.Is(err, organization.ErrForbidden) {
		t.Fatal("OEM self activation", err)
	}
	_, err = f.s.SetDomainEnabled(f.ctx, nil, f.owner, secondReady.ID, true, secondReady.Version)
	if err != nil {
		t.Fatal(err)
	}
	assertEndpoint("https://second.example.test/v1")
	firstReady, err := f.s.VerifyDomain(f.ctx, oemOwner, first.Domain.ID, ready)
	if err != nil {
		t.Fatal(err)
	}
	assertEndpoint("https://second.example.test/v1")
	firstActive, err := f.s.SetDomainEnabled(f.ctx, nil, f.owner, firstReady.ID, true, firstReady.Version)
	if err != nil {
		t.Fatal(err)
	}
	assertEndpoint("https://first.example.test/v1")
	if _, err = f.s.ResolveHost(f.ctx, nil, second.Domain.Hostname); !errors.Is(err, ErrNotFound) {
		t.Fatal("old hostname remained active", err)
	}
	stopped, err := f.s.SetDomainEnabled(f.ctx, nil, f.owner, firstActive.ID, false, firstActive.Version)
	if err != nil {
		t.Fatal(err)
	}
	assertEndpoint("")
	rechecked, err := f.s.VerifyDomain(f.ctx, oemOwner, stopped.ID, ready)
	if err != nil || rechecked.Status != "disabled" {
		t.Fatal(rechecked, err)
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
