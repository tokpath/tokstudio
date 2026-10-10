package app_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/platform/id"
	"gorm.io/gorm"
)

func TestCommissionLegacyChannelIncomeScope(t *testing.T) {
	fx := newWMeterEnv(t)
	a := fx.app
	ctx := context.Background()
	token := a.Config.BootstrapAdmin + "-b"
	var user struct{ ID string }
	if err := a.DB.Table("identity_users").Where("email=?", "channel.b@tokenhub.local").First(&user).Error; err != nil {
		t.Fatal(err)
	}
	type membership struct {
		UserID            string
		AcquisitionRoleID string
		CreatedAt         time.Time
	}
	var prior []membership
	if err := a.DB.Table("identity_role_members").Where("user_id=?", user.ID).Find(&prior).Error; err != nil {
		t.Fatal(err)
	}
	if err := a.DB.Table("identity_role_members").Where("user_id=?", user.ID).Delete(nil).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		a.DB.Table("identity_role_members").Where("user_id=?", user.ID).Delete(nil)
		if len(prior) > 0 {
			if err := a.DB.Table("identity_role_members").Create(&prior).Error; err != nil {
				t.Error(err)
			}
		}
	})
	role, err := a.Identity.CreateAcquisitionRole(ctx, identity.ResellerChannelID, identity.AcqPromoter, "")
	if err != nil {
		t.Fatal(err)
	}
	own, foreign, oem := id.New("ownlegacy"), id.New("foreignlegacy"), id.New("oemlegacy")
	for _, fact := range []struct{ key, channel, role string }{{own, identity.ResellerChannelID, role.ID}, {foreign, identity.ResellerChannelID, identity.KOL2BRoleID}, {oem, identity.OEMChannelID, identity.KOL2BRoleID}} {
		if err := a.DB.Exec("INSERT INTO commission_settlements(id,period_start,period_end,channel_org_id,beneficiary_role_id,amount_minor,status,policy_version) VALUES (?,now(),now(),?,?,100,'settled','original-policy')", fact.key, fact.channel, fact.role).Error; err != nil {
			t.Fatal(err)
		}
		if err := a.DB.Exec("INSERT INTO commission_entries(id,usage_event_id,request_id,channel_org_id,beneficiary_role_id,kind,policy_version,base_amount_minor,raw_amount_minor,amount_minor,status,settlement_id,idempotency_key) VALUES (?,?,?,?,?,'direct','original-policy',1000,100,100,'settled',?,?)", fact.key, fact.key, fact.key, fact.channel, fact.role, fact.key, fact.key).Error; err != nil {
			t.Fatal(err)
		}
	}
	export := func(actor string) (int, string) {
		t.Helper()
		req, err := http.NewRequest(http.MethodGet, fx.server.URL+"/v1/partner/export", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+actor)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		raw, err := io.ReadAll(res.Body)
		if err != nil {
			t.Fatal(err)
		}
		return res.StatusCode, string(raw)
	}
	// Channel staff without a personal membership have an empty income scope.
	for _, path := range []string{"/v1/partner/commissions", "/v1/partner/settlements", "/channel/commissions", "/channel/settlements"} {
		code, body := doJSON(t, http.MethodGet, fx.server.URL+path, token, false, nil)
		if code != 200 || len(body["items"].([]any)) != 0 {
			t.Fatalf("empty own scope widened %s: %d %+v", path, code, body)
		}
	}
	if code, csv := export(token); code != 200 || strings.Count(strings.TrimSpace(csv), "\n") != 0 {
		t.Fatalf("empty export widened %d %s", code, csv)
	}
	if code, _ := doJSON(t, http.MethodGet, fx.server.URL+"/channel/settlements/"+foreign, token, false, nil); code != 404 {
		t.Fatalf("empty own detail exposed foreign recipient %d", code)
	}
	if err := a.Identity.BindRoleMember(ctx, user.ID, role.ID); err != nil {
		t.Fatal(err)
	}
	// Query parameters cannot switch the server-derived original beneficiary scope.
	for _, path := range []string{"/v1/partner/commissions", "/v1/partner/settlements"} {
		code, body := doJSON(t, http.MethodGet, fx.server.URL+path+"?channel_id="+identity.OEMChannelID+"&role_id="+identity.KOL2BRoleID, token, false, nil)
		if code != 200 {
			t.Fatalf("own legacy read %s %d %+v", path, code, body)
		}
		rows := body["items"].([]any)
		if len(rows) != 1 || rows[0].(map[string]any)["id"] != own {
			t.Fatalf("same-B other recipient leaked %s %+v", path, body)
		}
	}
	if code, csv := export(token); code != 200 || !strings.Contains(csv, own) || strings.Contains(csv, foreign) || strings.Contains(csv, oem) {
		t.Fatalf("legacy export scope %d %s", code, csv)
	}
	if code, _ := doJSON(t, http.MethodGet, fx.server.URL+"/channel/settlements/"+own, token, false, nil); code != 200 {
		t.Fatalf("own original detail missing %d", code)
	}
	for _, target := range []string{foreign, oem} {
		if code, _ := doJSON(t, http.MethodGet, fx.server.URL+"/channel/settlements/"+target, token, false, nil); code != 404 {
			t.Fatalf("direct other recipient detail leaked %s %d", target, code)
		}
	}
	for _, path := range []string{"/channel/settlements/manage", "/admin/settlements/" + foreign, "/admin/commissions"} {
		if code, _ := doJSON(t, http.MethodGet, fx.server.URL+path, token, false, nil); code != 403 {
			t.Fatalf("B gained management scope %s %d", path, code)
		}
	}
	for _, path := range []string{"/v1/partner/commissions", "/v1/partner/settlements"} {
		code, body := doJSON(t, http.MethodGet, fx.server.URL+path, a.Config.BootstrapAdmin+"-c", false, nil)
		if code != 200 {
			t.Fatalf("OEM original read rejected %d %+v", code, body)
		}
		found := false
		for _, raw := range body["items"].([]any) {
			key := raw.(map[string]any)["id"]
			if key == own || key == foreign {
				t.Fatalf("OEM read another brand %+v", raw)
			}
			if key == oem {
				found = true
			}
		}
		if !found {
			t.Fatalf("OEM original scope lost %s", path)
		}
	}
	if code, csv := export(a.Config.BootstrapAdmin + "-c"); code != 200 || !strings.Contains(csv, oem) || strings.Contains(csv, foreign) || strings.Contains(csv, own) {
		t.Fatalf("OEM export scope changed %d %s", code, csv)
	}
	callback := "t07_legacy_member_read_failure"
	if err := a.DB.Callback().Query().Before("gorm:query").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "identity_role_members" {
			tx.AddError(errors.New("injected own role read failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer a.DB.Callback().Query().Remove(callback)
	for _, path := range []string{"/v1/partner/commissions", "/v1/partner/settlements"} {
		if code, body := doJSON(t, http.MethodGet, fx.server.URL+path, token, false, nil); code == 200 || body["items"] != nil {
			t.Fatalf("role read failed open %s %d %+v", path, code, body)
		}
	}
	if code, _ := export(token); code == 200 {
		t.Fatal("export role read failed open")
	}
}
