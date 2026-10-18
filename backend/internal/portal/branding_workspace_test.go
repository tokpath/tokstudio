package portal

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/organization"
)

func TestPostgresParentAppearanceWorkspaceTargetsAndVersions(t *testing.T) {
	f := setupPortal(t)
	ctx := context.Background()
	op, owner := f.a.Installation.OperatorID, f.a.Installation.OwnerStaffID
	create := func(parent string, kind organization.OperatorKind, name string) organization.Operator {
		t.Helper()
		out, err := f.a.Organization.CreateOperator(ctx, nil, organization.CreateOperatorInput{ActorStaffID: owner, ParentOperatorID: parent, Kind: kind, Name: name, LegalName: name, BrandName: name})
		if err != nil {
			t.Fatal(err)
		}
		return out.Operator
	}
	channel := create(op, organization.Channel, "Parent configured channel")
	oem := create(op, organization.OEM, "Parent configured OEM")
	f.login(t, "staff", "owner@example.test", "owner-password-test")
	base := "/v1/operators/" + op
	for _, test := range []struct {
		resource string
		target   organization.Operator
	}{{"channel-themes", channel}, {"branding", oem}} {
		status, out, _ := f.call(t, "GET", base+"/"+test.resource, nil, "")
		mustOK(t, status, out)
		found := false
		for _, raw := range out["items"].([]any) {
			row := raw.(map[string]any)
			if row["id"] == test.target.ID {
				found = true
				if row["version"] != float64(0) {
					t.Fatal("initial version", row)
				}
			}
		}
		if !found {
			t.Fatal("target missing", out)
		}
		path := base + "/" + test.resource + "?target_operator_id=" + test.target.ID
		input := map[string]any{"expected_version": 0, "values": map[string]any{"primary_color": "#123456"}}
		key := uuid.NewString()
		status, out, _ = f.call(t, "POST", path, input, key)
		mustOK(t, status, out)
		status, out, _ = f.call(t, "POST", path, input, key)
		mustOK(t, status, out)
		if out["version"] != float64(1) {
			t.Fatal("replay version", out)
		}
		status, _, _ = f.call(t, "POST", path, input, uuid.NewString())
		if status != 409 {
			t.Fatal("stale version allowed", status)
		}
		status, out, _ = f.call(t, "GET", base+"/"+test.resource, nil, "")
		mustOK(t, status, out)
		for _, raw := range out["items"].([]any) {
			row := raw.(map[string]any)
			if row["id"] == test.target.ID && (row["version"] != float64(1) || row["values"].(map[string]any)["primary_color"] != "#123456") {
				t.Fatal("saved override/version not returned", row)
			}
		}
	}
	status, out, _ := f.call(t, "POST", base+"/domains", map[string]any{"target_operator_id": oem.ID, "hostname": "oem-" + uuid.NewString() + ".example.test"}, uuid.NewString())
	mustOK(t, status, out)
	status, out, _ = f.call(t, "GET", base+"/domains", nil, "")
	mustOK(t, status, out)
	if len(out["items"].([]any)) != 1 || out["items"].([]any)[0].(map[string]any)["values"].(map[string]any)["brand_id"] != oem.BrandID {
		t.Fatal("parent cannot see requested OEM domain", out)
	}
	status, _, _ = f.call(t, "POST", base+"/domains", map[string]any{"target_operator_id": channel.ID, "hostname": "channel.example.test"}, uuid.NewString())
	if status != 403 {
		t.Fatal("channel domain allowed", status)
	}
	status, _, _ = f.call(t, "POST", base+"/branding?target_operator_id="+uuid.NewString(), map[string]any{"expected_version": 0, "values": map[string]any{"primary_color": "#654321"}}, uuid.NewString())
	if status == 200 {
		t.Fatal("unknown target allowed")
	}
	status, _, _ = f.call(t, "GET", "/v1/operators/"+channel.ID+"/customers", nil, "")
	if status != 403 {
		t.Fatal("appearance granted child customer access", status)
	}
}
