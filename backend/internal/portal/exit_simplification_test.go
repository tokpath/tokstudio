package portal

import (
	"context"
	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"testing"
)

func TestPostgresExitNoticesRespectScopeWithoutRefundsOrMonetarySetup(t *testing.T) {
	f := setupPortalMonetary(t, false)
	ctx := context.Background()
	parent, owner := f.a.Installation.OperatorID, f.a.Installation.OwnerStaffID
	f.login(t, "staff", "owner@example.test", "owner-password-test")
	base := "/v1/operators/" + parent + "/exits"
	status, out, _ := f.call(t, "GET", base, nil, "")
	mustOK(t, status, out)
	if len(out["operations"].([]any)) != 0 {
		t.Fatal("platform offered its own exit", out)
	}
	status, _, _ = f.call(t, "POST", base, map[string]any{"note": "platform cannot exit"}, uuid.NewString())
	if status == 200 {
		t.Fatal("platform self exit accepted")
	}
	for _, kind := range []organization.OperatorKind{organization.OEM, organization.Channel} {
		child, e := f.a.Organization.CreateOperator(ctx, nil, organization.CreateOperatorInput{ActorStaffID: owner, ParentOperatorID: parent, Kind: kind, Name: string(kind) + " exit", BrandName: "Independent brand"})
		if e != nil {
			t.Fatal(e)
		}
		invitation, e := f.a.Organization.InviteStaff(ctx, nil, owner, child.Operator.ID, "owner@example.test", "owner")
		if e != nil {
			t.Fatal(e)
		}
		if _, e = f.a.Organization.AcceptStaffInvitation(ctx, nil, invitation.Token, organization.VerifiedStaff{ID: owner, Email: "owner@example.test", EmailVerified: true}); e != nil {
			t.Fatal(e)
		}
		path := "/v1/operators/" + child.Operator.ID + "/exits"
		key := uuid.NewString()
		input := map[string]any{"note": "business closure notice"}
		status, out, _ = f.call(t, "POST", path, input, key)
		mustOK(t, status, out)
		id := out["id"]
		status, out, _ = f.call(t, "POST", path, input, key)
		mustOK(t, status, out)
		if out["id"] != id {
			t.Fatal("notice duplicated", out)
		}
		status, out, _ = f.call(t, "GET", path, nil, "")
		mustOK(t, status, out)
		if len(out["items"].([]any)) != 1 || len(out["operations"].([]any)) != 0 {
			t.Fatal("notice workspace", out)
		}
		unchanged, e := f.a.Organization.GetSupplyAgreement(ctx, nil, child.Operator.ID)
		if e != nil || unchanged.Status != "active" {
			t.Fatal("notice changed supply or forced closure", unchanged, e)
		}
		status, _, _ = f.call(t, "POST", path+"/"+uuid.NewString()+"/prepare_refund", map[string]any{}, uuid.NewString())
		if status != 404 {
			t.Fatal("removed refund action", status)
		}
	}
	status, out, _ = f.call(t, "GET", base, nil, "")
	mustOK(t, status, out)
	if len(out["items"].([]any)) != 2 {
		t.Fatal("parent did not receive notices", out)
	}
}
