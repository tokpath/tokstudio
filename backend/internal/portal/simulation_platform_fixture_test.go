package portal

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"github.com/tokpath/tokstudio/backend/internal/organization"
)

// Only identities and empty appearance targets are prerequisites. Catalog,
// routes, prices, staff invitations and business settings are exercised through
// the real browser/API in simulation-platform.spec.ts.
func (f *testApp) seedPlatformSimulation(t *testing.T, lab string) map[string]any {
	t.Helper()
	ctx := context.Background()
	op, owner := f.a.Installation.OperatorID, f.a.Installation.OwnerStaffID
	password := "simulation-platform-password"
	staff := func(email, name string, grants []string) map[string]any {
		t.Helper()
		hash, err := organization.HashPassword(password)
		if err != nil {
			t.Fatal(err)
		}
		row := Staff{ID: uuid.NewString(), Email: email, DisplayName: name, PasswordHash: hash, Version: 1, Status: "active", EmailVerified: true, CreatedAt: time.Now()}
		if err = f.a.DB.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
		inv, err := f.a.Organization.InviteStaff(ctx, nil, owner, op, email, "readonly")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = f.a.Organization.AcceptStaffInvitation(ctx, nil, inv.Token, organization.VerifiedStaff{ID: row.ID, Email: email, EmailVerified: true}); err != nil {
			t.Fatal(err)
		}
		if err = f.a.Organization.SetMembership(ctx, nil, organization.SetMembershipInput{ActorStaffID: owner, OperatorID: op, StaffID: row.ID, Role: "custom", Status: "active", Grants: grants}); err != nil {
			t.Fatal(err)
		}
		return map[string]any{"id": row.ID, "email": email, "password": password}
	}
	tech := staff("platform-tech@simulation.test", "技术管理员仿真", []string{"organization.read", "catalog.manage"})
	brand := staff("platform-brand@simulation.test", "品牌管理员仿真", []string{"organization.read", "branding.manage_oem", "branding.manage_channels"})
	restricted := staff("platform-restricted@simulation.test", "受限员工仿真", []string{"organization.read", "staff.read", "reports.summary"})
	create := func(kind organization.OperatorKind, name string) organization.Operator {
		t.Helper()
		out, err := f.a.Organization.CreateOperator(ctx, nil, organization.CreateOperatorInput{ActorStaffID: owner, ParentOperatorID: op, Kind: kind, Name: name, LegalName: "合成仿真主体", BrandName: name, APIBaseURL: "https://platform-simulation.example.test/api/v1", QuoteReference: "isolated simulation terms", ExitTermsReference: "isolated simulation exit"})
		if err != nil {
			t.Fatal(err)
		}
		return out.Operator
	}
	oem := create(organization.OEM, "平台品牌仿真OEM")
	channel := create(organization.Channel, "平台品牌仿真渠道")
	f.registered(t, "platform-customer@simulation.test")
	f.login(t, "customer", "platform-customer@simulation.test", "customer-password-test")
	status, me, _ := f.call(t, "GET", "/v1/auth/me", nil, "")
	mustOK(t, status, me)
	customer := map[string]any{"id": me["customer"].(map[string]any)["id"], "email": "platform-customer@simulation.test", "password": "customer-password-test"}
	return map[string]any{"operator_id": op, "tech": tech, "brand": brand, "restricted": restricted, "customer": customer, "oem_id": oem.ID, "oem_brand_id": oem.BrandID, "channel_id": channel.ID, "model_id": "simulation-chat-" + uuid.NewString(), "route_id": uuid.NewString(), "lab_url": lab, "invite_email": "platform-invited@simulation.test"}
}

func TestPostgresStaffWorkspaceIdentifiesPeopleAndOffersNamedInvitationRoles(t *testing.T) {
	f := setupPortal(t)
	f.login(t, "staff", "owner@example.test", "owner-password-test")
	status, page, _ := f.call(t, "GET", "/v1/operators/"+f.a.Installation.OperatorID+"/staff", nil, "")
	mustOK(t, status, page)
	rows := page["items"].([]any)
	if len(rows) != 1 {
		t.Fatal("owner membership missing")
	}
	person := rows[0].(map[string]any)["values"].(map[string]any)
	if person["display_name"] != "Owner" || person["email"] != "owner@example.test" {
		t.Fatal("staff cannot be identified by name and email", person)
	}
	operation := page["operations"].([]any)[0].(map[string]any)
	var role map[string]any
	for _, value := range operation["fields"].([]any) {
		field := value.(map[string]any)
		if field["name"] == "role" {
			role = field
		}
	}
	if role["kind"] != "select" || len(role["options"].([]any)) != 5 {
		t.Fatal("invitation requires guessing role enum instead of a named choice", role)
	}
}

func TestPostgresNewModelListingAndAuthorizationHonorPlannedActivation(t *testing.T) {
	f := setupPortal(t)
	ctx := context.Background()
	op, owner := f.a.Installation.OperatorID, f.a.Installation.OwnerStaffID
	first := f.configureExternalCatalog(t, "http://127.0.0.1:18090")
	model, err := f.a.Catalog.PublicModel(ctx, first)
	if err != nil {
		t.Fatal(err)
	}
	upstreams, err := f.a.Catalog.AdminUpstreams(ctx, op, first)
	if err != nil || len(upstreams) != 1 {
		t.Fatal("source route", err)
	}
	model.ID, model.Identity, model.Enabled, model.Version = "scheduled-"+uuid.NewString(), "scheduled-revision", false, 0
	if err = f.a.Catalog.SavePublicModel(ctx, op, model, 0); err != nil {
		t.Fatal(err)
	}
	upstream := upstreams[0]
	upstream.ID, upstream.PublicModelID, upstream.Identity, upstream.Version = uuid.NewString(), model.ID, model.Identity, 0
	if err = f.a.Catalog.SaveUpstream(ctx, op, upstream, 0); err != nil {
		t.Fatal(err)
	}
	model.Enabled = true
	if err = f.a.Catalog.SavePublicModel(ctx, op, model, 1); err != nil {
		t.Fatal(err)
	}
	child, err := f.a.Organization.CreateOperator(ctx, nil, organization.CreateOperatorInput{ActorStaffID: owner, ParentOperatorID: op, Kind: organization.Channel, Name: "scheduled child", LegalName: "synthetic", QuoteReference: "terms", ExitTermsReference: "exit"})
	if err != nil {
		t.Fatal(err)
	}
	f.login(t, "staff", "owner@example.test", "owner-password-test")
	base := "/v1/operators/" + op
	status, out, _ := f.call(t, "POST", base+"/catalog-changes", map[string]any{"lead_seconds": 60, "rules_reference": "synthetic notice terms", "expected_version": 0}, uuid.NewString())
	mustOK(t, status, out)
	for _, action := range []string{"listing", "authorization"} {
		input := map[string]any{"action": action, "model_id": model.ID, "enabled": true, "expected_version": 0, "effective_at": time.Now().Add(3 * time.Minute), "evidence": "actual scheduled release, never immediate bootstrap"}
		if action == "authorization" {
			input["buyer_id"] = child.Operator.ID
		}
		status, out, _ = f.call(t, "POST", base+"/pricing", input, uuid.NewString())
		mustOK(t, status, out)
		if out["status"] != "announcing" || out["id"] == model.ID {
			t.Fatal("planned first activation bypassed its notification plan", action, out)
		}
	}
	status, page, _ := f.call(t, "GET", base+"/catalog-changes", nil, "")
	mustOK(t, status, page)
	fields := page["operations"].([]any)[0].(map[string]any)["fields"].([]any)
	for _, item := range fields {
		field := item.(map[string]any)
		if field["name"] == "expected_version" && field["default_value"] != float64(1) {
			t.Fatal("reopening notice policy lost its saved concurrency version", field)
		}
	}
	var changes int64
	if err = f.a.DB.Model(&catalog.CatalogChange{}).Where("operator_id=? AND resource_id=? AND state='announcing'", op, model.ID).Count(&changes).Error; err != nil || changes != 2 {
		t.Fatal("missing planned activations", changes, err)
	}
}
