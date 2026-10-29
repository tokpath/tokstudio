package portal

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"github.com/tokpath/tokstudio/backend/internal/platform/mail"
)

func TestPostgresBrandMailWorkspaceBoundariesAndReadiness(t *testing.T) {
	f := setupPortalMonetary(t, false)
	ctx := context.Background()
	owner, platform := f.a.Installation.OwnerStaffID, f.a.Installation.OperatorID
	create := func(parent string, kind organization.OperatorKind, name string) organization.Operator {
		t.Helper()
		out, err := f.a.Organization.CreateOperator(ctx, nil, organization.CreateOperatorInput{ActorStaffID: owner, ParentOperatorID: parent, Kind: kind, Name: name, LegalName: name, BrandName: name})
		if err != nil {
			t.Fatal(err)
		}
		inv, err := f.a.Organization.InviteStaff(ctx, nil, owner, out.Operator.ID, "owner@example.test", "owner")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = f.a.Organization.AcceptStaffInvitation(ctx, nil, inv.Token, organization.VerifiedStaff{ID: owner, Email: "owner@example.test", EmailVerified: true}); err != nil {
			t.Fatal(err)
		}
		return out.Operator
	}
	oem := create(platform, organization.OEM, "Mail OEM")
	channel := create(oem.ID, organization.Channel, "OEM Channel")
	other := create(platform, organization.OEM, "Other OEM")
	f.a.Mail.SetSender(nil)
	f.login(t, "staff", "owner@example.test", "owner-password-test")
	post := func(op string, in any) {
		t.Helper()
		status, out, _ := f.call(t, "POST", "/v1/operators/"+op+"/mail", in, uuid.NewString())
		mustOK(t, status, out)
	}
	get := func(op, resource string) map[string]any {
		t.Helper()
		status, out, _ := f.call(t, "GET", "/v1/operators/"+op+"/"+resource, nil, "")
		mustOK(t, status, out)
		return out
	}
	first := func(page map[string]any) map[string]any {
		return page["items"].([]any)[0].(map[string]any)["values"].(map[string]any)
	}
	post(platform, map[string]any{"enabled": true, "host": "smtp.platform.example", "port": 587, "username": "private-platform-account", "password": "private-platform-secret", "from": "platform@example.test", "from_name": "Platform", "security": "starttls"})
	page := get(oem.ID, "mail")
	raw, _ := json.Marshal(page)
	if strings.Contains(string(raw), "private-platform") || strings.Contains(string(raw), "smtp.platform.example") {
		t.Fatal("platform transport exposed to OEM", string(raw))
	}
	if v := first(page); v["mode"] != "platform" || v["available"] != true {
		t.Fatal(v)
	}
	fields := operationFields(t, page, "configure")
	if fields["from"]["default_value"] != "" || fields["mode"]["default_value"] != "platform" || fields["host"]["visible_when"] == nil {
		t.Fatal("custom form inherited platform identity", fields)
	}
	post(oem.ID, map[string]any{"mode": "custom", "enabled": true, "host": "smtp.oem.example", "port": 465, "username": "oem-account", "password": "oem-secret", "from": "oem@example.test", "security": "tls"})
	page = get(channel.ID, "mail")
	raw, _ = json.Marshal(page)
	for _, secret := range []string{"smtp.oem.example", "oem-account", "oem-secret"} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("brand settings exposed to channel", string(raw))
		}
	}
	if len(page["operations"].([]any)) != 0 || first(page)["from"] != "oem@example.test" {
		t.Fatal(page)
	}
	status, _, _ := f.call(t, "POST", "/v1/operators/"+channel.ID+"/mail", map[string]any{"mode": "custom"}, uuid.NewString())
	if status != 403 {
		t.Fatal("channel configured independent mail", status)
	}
	if first(get(other.ID, "mail"))["from"] != "platform@example.test" {
		t.Fatal("OEM override affected sibling")
	}
	// Selected workspace wins even while the browser is on the platform domain.
	post(platform, map[string]any{"enabled": false})
	staffFields := operationFields(t, get(oem.ID, "staff"), "invite")
	options, _ := staffFields["delivery_mode"]["options"].([]any)
	hasEmail := false
	for _, o := range options {
		if o.(map[string]any)["value"] == "email" {
			hasEmail = true
		}
	}
	if !hasEmail {
		t.Fatal("OEM mail not used for workspace invitation", staffFields)
	}
	status, result, _ := f.call(t, "POST", "/v1/operators/"+oem.ID+"/staff", map[string]any{"email": "invited@example.test", "role": "operations", "delivery_mode": "email"}, uuid.NewString())
	mustOK(t, status, result)
	f.a.Mail.SetSender(f.box)
	if err := f.a.RunMailJobs(ctx, "brand-mail-test", 1); err != nil {
		t.Fatal(err)
	}
	f.box.mu.Lock()
	messages := append([]mail.Message{}, f.box.messages...)
	f.box.mu.Unlock()
	f.a.Mail.SetSender(nil)
	if len(messages) != 1 || messages[0].BrandID != oem.BrandID || messages[0].Brand != "Mail OEM" {
		t.Fatal("invitation followed browser host instead of workspace", messages)
	}
	post(platform, map[string]any{"enabled": true, "host": "smtp.platform.example", "port": 587, "username": "private-platform-account", "from": "platform@example.test", "security": "starttls"})
	recipient, err := (recipientDirectory{f.a}).ResolveRecipient(ctx, nil, channel.ID, owner)
	if err != nil || recipient.BrandID != oem.BrandID || recipient.Brand != "Mail OEM" {
		t.Fatal(recipient, err)
	}
	v := first(get(channel.ID, "business-status"))
	hints := map[string]string{}
	for _, feature := range v["readiness"].([]any) {
		for _, check := range feature.(map[string]any)["requirements"].([]any) {
			r := check.(map[string]any)
			if hint, ok := r["action_hint"].(string); ok {
				hints[r["code"].(string)] = hint
			}
		}
	}
	if !strings.Contains(hints["monetary_foundation"], "等待平台负责人") {
		t.Fatal(hints)
	}
	post(oem.ID, map[string]any{"mode": "custom", "enabled": false})
	if f.a.Mail.AvailableForBrand(oem.BrandID) || !f.a.Mail.AvailableForBrand(other.BrandID) {
		t.Fatal("brand availability isolation")
	}
	v = first(get(channel.ID, "mail"))
	if v["configuration_note"] != "等待所属品牌方配置可用的邮件服务。" {
		t.Fatal(v)
	}
	post(oem.ID, map[string]any{"mode": "platform"})
	if first(get(channel.ID, "mail"))["from"] != "platform@example.test" {
		t.Fatal("channel did not follow brand fallback")
	}
	// An unrelated staff role still cannot gain access through the new menu.

	hash, err := organization.HashPassword("finance-password-test")
	if err != nil {
		t.Fatal(err)
	}
	finance := Staff{ID: uuid.NewString(), Email: "finance@example.test", DisplayName: "Finance", PasswordHash: hash, Version: 1, Status: "active"}
	if err = f.a.DB.Create(&finance).Error; err != nil {
		t.Fatal(err)
	}
	invitation, err := f.a.Organization.InviteStaff(ctx, nil, owner, oem.ID, finance.Email, "finance")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.a.Organization.AcceptAuthenticatedStaffInvitation(ctx, nil, invitation.Token, finance.ID, finance.Email); err != nil {
		t.Fatal(err)
	}
	f.login(t, "staff", finance.Email, "finance-password-test")
	status, _, _ = f.call(t, "GET", "/v1/operators/"+oem.ID+"/mail", nil, "")
	if status != 403 {
		t.Fatal("finance read brand SMTP without authority", status)
	}
	status, _, _ = f.call(t, "POST", "/v1/operators/"+oem.ID+"/mail", map[string]any{"mode": "platform"}, uuid.NewString())
	if status != 403 {
		t.Fatal("finance changed brand SMTP without authority", status)
	}
}
