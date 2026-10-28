package portal

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/branding"
	"github.com/tokpath/tokstudio/backend/internal/organization"
)

func TestPostgresChannelInvitationUsesOwnBrandAndPreservesDirectRegistration(t *testing.T) {
	s := stockSetup(t)
	f, ctx := s.f, context.Background()
	base := "/v1/operators/" + s.channel + "/customer-invitation"
	status, first, _ := f.call(t, "GET", base, nil, "")
	mustOK(t, status, first)
	code := first["invite_code"].(string)
	if code == "" || first["invite_url"] != "" || first["registration_open"] != false {
		t.Fatal("unconfigured OEM used another brand's domain or claimed registration open", first)
	}
	channel, _ := f.a.Organization.GetOperator(ctx, nil, s.channel)
	domain, err := f.a.Branding.RequestDomain(ctx, nil, f.a.Installation.OwnerStaffID, channel.BrandID, "invite.oem.example.test")
	if err != nil {
		t.Fatal(err)
	}
	active, err := f.a.Branding.VerifyAndActivateDomain(ctx, f.a.Installation.OwnerStaffID, domain.Domain.ID, branding.DomainVerifierFunc(func(context.Context, string, string) (branding.DomainVerification, error) {
		return branding.DomainVerification{OwnershipVerified: true, TLSReady: true, ProviderReference: "isolated DNS and TLS verification"}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	if err = f.a.DB.Exec("UPDATE organization_operators SET lifecycle='active' WHERE id IN ?", []string{s.channel, s.oem}).Error; err != nil {
		t.Fatal(err)
	}
	if err = f.a.DB.Exec("UPDATE organization_operator_statuses SET registration=true WHERE operator_id IN ?", []string{s.channel, s.oem, f.a.Installation.OperatorID}).Error; err != nil {
		t.Fatal(err)
	}
	status, ready, _ := f.call(t, "GET", base, nil, "")
	mustOK(t, status, ready)
	want := "https://invite.oem.example.test/register?invite=" + url.QueryEscape(code)
	if ready["invite_code"] != code || ready["invite_url"] != want || ready["registration_open"] != true {
		t.Fatal(ready)
	}
	for _, tc := range []struct{ brand, code, operator, email string }{{channel.BrandID, code, s.channel, "channel-direct@invite.test"}, {channel.BrandID, "", s.oem, "oem-direct@invite.test"}, {f.a.Installation.BrandID, "", f.a.Installation.OperatorID, "platform-direct@invite.test"}} {
		challenge, err := f.a.Organization.BeginRegistration(ctx, nil, organization.RegistrationIntentRequest{BrandID: tc.brand, InviteCode: tc.code, Email: tc.email, Password: "isolated-invite-password"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = f.a.Organization.VerifyRegistration(ctx, nil, challenge.Intent.ID, challenge.VerificationToken); err != nil {
			t.Fatal(err)
		}
		registered, err := f.a.Organization.CompleteRegistration(ctx, nil, organization.CompleteRegistrationInput{IntentID: challenge.Intent.ID, VerificationToken: challenge.VerificationToken})
		if err != nil || registered.Ownership.OperatorID != tc.operator || registered.InviterID != nil {
			t.Fatal("wrong customer ownership or fabricated user referral", registered, err)
		}
	}
	if _, err = f.a.Organization.ResolveRegistrationIntent(ctx, nil, f.a.Installation.BrandID, code); !errors.Is(err, organization.ErrInvalidInviteCode) {
		t.Fatal("OEM code crossed brand boundary", err)
	}
	status, _, _ = f.call(t, "GET", "/v1/operators/"+s.oem+"/customer-invitation", nil, "")
	if status != 404 {
		t.Fatal("brand operator was given a channel invite", status)
	}
	if _, err = f.a.Branding.SetDomainEnabled(ctx, nil, f.a.Installation.OwnerStaffID, active.ID, false, active.Version); err != nil {
		t.Fatal(err)
	}
	status, disabledDomain, _ := f.call(t, "GET", base, nil, "")
	mustOK(t, status, disabledDomain)
	if disabledDomain["invite_url"] != "" || disabledDomain["invite_code"] != code {
		t.Fatal("disabled domain changed code or fell back to official domain", disabledDomain)
	}
	if err = f.a.DB.Exec("UPDATE organization_invite_codes SET enabled=false WHERE operator_id=? AND kind='channel'", s.channel).Error; err != nil {
		t.Fatal(err)
	}
	status, disabledCode, _ := f.call(t, "GET", base, nil, "")
	mustOK(t, status, disabledCode)
	if disabledCode["enabled"] != false || disabledCode["registration_open"] != false || disabledCode["invite_code"] != code {
		t.Fatal(disabledCode)
	}
	if err = f.a.DB.Exec("UPDATE organization_staff_memberships SET status='suspended' WHERE operator_id=? AND staff_id=?", s.channel, f.a.Installation.OwnerStaffID).Error; err != nil {
		t.Fatal(err)
	}
	status, _, _ = f.call(t, "GET", base, nil, "")
	if status != 403 {
		t.Fatal("parent owner gained implicit channel customer access", status)
	}
}

func TestExternalChannelInvitationAcceptanceServer(t *testing.T) {
	ready := os.Getenv("TOKENHUB_INVITATION_ACCEPTANCE_FILE")
	if ready == "" {
		t.Skip("explicit isolated channel invitation browser acceptance only")
	}
	f := setupPortal(t)
	ctx := context.Background()
	owner := f.a.Installation.OwnerStaffID
	channel, err := f.a.Organization.CreateOperator(ctx, nil, organization.CreateOperatorInput{ActorStaffID: owner, ParentOperatorID: f.a.Installation.OperatorID, Kind: organization.Channel, Name: "Invite channel"})
	if err != nil {
		t.Fatal(err)
	}
	in, err := f.a.Organization.InviteStaff(ctx, nil, owner, channel.Operator.ID, "owner@example.test", "owner")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.a.Organization.AcceptStaffInvitation(ctx, nil, in.Token, organization.VerifiedStaff{ID: owner, Email: "owner@example.test", EmailVerified: true}); err != nil {
		t.Fatal(err)
	}
	if err = f.a.DB.Exec("UPDATE organization_operators SET lifecycle='active' WHERE id=?", channel.Operator.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err = f.a.DB.Exec("UPDATE organization_operator_statuses SET registration=true WHERE operator_id IN ?", []string{channel.Operator.ID, f.a.Installation.OperatorID}).Error; err != nil {
		t.Fatal(err)
	}
	f.server.Close()
	f.a.Config.TrustedProxies = []string{"127.0.0.1", "::1"}
	f.a.Config.WebOrigin = "http://127.0.0.1:13003"
	f.a.Config.PublicBaseURL = f.a.Config.WebOrigin
	listener, err := net.Listen("tcp", "0.0.0.0:18081")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(f.a.Router())
	server.Listener.Close()
	server.Listener = listener
	server.Start()
	f.server = server
	t.Cleanup(server.Close)
	data, _ := json.Marshal(map[string]string{"operator_id": channel.Operator.ID, "platform_id": f.a.Installation.OperatorID, "invite_code": channel.InviteCode, "owner_email": "owner@example.test", "owner_password": "owner-password-test"})
	if err = os.WriteFile(ready, data, 0600); err != nil {
		t.Fatal(err)
	}
	for {
		if _, err = os.Stat(ready); os.IsNotExist(err) {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
}
