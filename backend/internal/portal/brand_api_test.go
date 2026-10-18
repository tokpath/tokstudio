package portal

import (
	"context"
	"testing"

	"github.com/tokpath/tokstudio/backend/internal/branding"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"github.com/tokpath/tokstudio/backend/internal/platform/config"
)

func TestOfficialAPIEndpointUsesCanonicalV1AndConfiguredHost(t *testing.T) {
	for _, tc := range []struct {
		name, env, host, public, want string
	}{
		{"production", "production", "test.tokpath.com", "https://test.tokpath.com/api/v1", "https://test.tokpath.com/v1"},
		{"different host", "production", "official.example.test", "https://unrelated.example.test", "https://official.example.test/v1"},
		{"production requires TLS", "production", "official.example.test", "http://official.example.test", "https://official.example.test/v1"},
		{"development", "development", "127.0.0.1", "http://127.0.0.1:13003/api/v1", "http://127.0.0.1:13003/v1"},
		{"missing host", "development", "", "https://example.test", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := officialAPIEndpoint(Config{Config: &config.Config{Env: tc.env, PublicBaseURL: tc.public}, OfficialHost: tc.host})
			if got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestPostgresBrandAPIEndpointIgnoresLegacyInputAndChannelsInheritVerifiedDomain(t *testing.T) {
	f := setupPortal(t)
	ctx := context.Background()
	owner, parent := f.a.Installation.OwnerStaffID, f.a.Installation.OperatorID
	oem, err := f.a.Organization.CreateOperator(ctx, nil, organization.CreateOperatorInput{ActorStaffID: owner, ParentOperatorID: parent, Kind: organization.OEM, Name: "OEM entry", BrandName: "OEM entry"})
	if err != nil {
		t.Fatal(err)
	}
	invitation, err := f.a.Organization.InviteStaff(ctx, nil, owner, oem.Operator.ID, "owner@example.test", "owner")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.a.Organization.AcceptStaffInvitation(ctx, nil, invitation.Token, organization.VerifiedStaff{ID: owner, Email: "owner@example.test", EmailVerified: true}); err != nil {
		t.Fatal(err)
	}
	// This fixture isolates domain derivation from business opening checks.
	if err = f.a.DB.Exec("UPDATE organization_operators SET lifecycle='active' WHERE id=?", oem.Operator.ID).Error; err != nil {
		t.Fatal(err)
	}
	channel, err := f.a.Organization.CreateOperator(ctx, nil, organization.CreateOperatorInput{ActorStaffID: owner, ParentOperatorID: oem.Operator.ID, Kind: organization.Channel, Name: "OEM channel"})
	if err != nil {
		t.Fatal(err)
	}
	if got := f.a.brandAPIEndpoint(ctx, oem.Brand.ID); got != "" {
		t.Fatalf("manual URL escaped into customer entry: %q", got)
	}
	d, err := f.a.Branding.RequestDomain(ctx, nil, owner, oem.Brand.ID, "entry.example.test")
	if err != nil {
		t.Fatal(err)
	}
	f.login(t, "staff", "owner@example.test", "owner-password-test")
	assertProjection := func(endpoint string) {
		t.Helper()
		if got := f.a.brandDTO(ctx, oem.Brand)["api_base_url"]; got != endpoint {
			t.Fatalf("brand DTO did not use the verified domain: %v", got)
		}
		status, page, _ := f.call(t, "GET", "/v1/operators/"+parent+"/domains", nil, "")
		mustOK(t, status, page)
		for _, item := range page["items"].([]any) {
			row := item.(map[string]any)
			if row["id"] != d.Domain.ID {
				continue
			}
			v := row["values"].(map[string]any)
			if v["txt_record_name"] != d.TXTName || v["txt_record_value"] != d.TXTValue {
				t.Fatal("domain instructions omitted the required TXT prefix or record name", v)
			}
			url, _ := v["api_base_url"].(string)
			if url != endpoint {
				t.Fatal("domain page exposed an unusable API endpoint", v)
			}
			return
		}
		t.Fatal("configured OEM domain missing from parent domain workspace", page)
	}
	assertProjection("")
	if got := f.a.brandAPIEndpoint(ctx, channel.Brand.ID); got != "" {
		t.Fatalf("channel exposed unverified domain: %q", got)
	}
	active, err := f.a.Branding.VerifyAndActivateDomain(ctx, owner, d.Domain.ID, branding.DomainVerifierFunc(func(context.Context, string, string) (branding.DomainVerification, error) {
		return branding.DomainVerification{OwnershipVerified: true, TLSReady: true, ProviderReference: "isolated-provider-evidence"}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	assertProjection("https://entry.example.test/v1")
	for _, id := range []string{oem.Brand.ID, channel.Brand.ID} {
		if got := f.a.brandAPIEndpoint(ctx, id); got != "https://entry.example.test/v1" {
			t.Fatalf("brand/channel endpoint %q", got)
		}
	}
	if _, err = f.a.Branding.SetDomainEnabled(ctx, nil, owner, active.ID, false, active.Version); err != nil {
		t.Fatal(err)
	}
	if got := f.a.brandAPIEndpoint(ctx, oem.Brand.ID); got != "" {
		t.Fatalf("disabled entry remains copyable: %q", got)
	}
	assertProjection("")
}
