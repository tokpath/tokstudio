package portal

import (
	"context"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"github.com/tokpath/tokstudio/backend/internal/modelruntime"
)

func TestPostgresTechnologyRoutesLoadOrderRemoveAndProtectCapabilities(t *testing.T) {
	f := setupPortal(t)
	ctx := context.Background()
	operator := f.a.Installation.OperatorID
	base := "/v1/operators/" + operator
	f.login(t, "staff", "owner@example.test", "owner-password-test")
	for _, id := range []string{"tech-chat", "tech-empty"} {
		if e := f.a.Catalog.SavePublicModel(ctx, operator, catalog.PublicModel{ID: id, Capabilities: []modelruntime.Capability{{Operation: modelruntime.Chat}}}, 0); e != nil {
			t.Fatal(e)
		}
	}
	for i, id := range []string{"primary", "backup", "embedding-only"} {
		if _, e := f.a.Catalog.SaveCredential(ctx, operator, id, id, "Supplier "+id, "synthetic-key", true, 0); e != nil {
			t.Fatal(e)
		}
		op := modelruntime.Chat
		if i == 2 {
			op = modelruntime.Embedding
		}
		if e := f.a.Catalog.SaveSupplierModel(ctx, operator, catalog.SupplierModel{ID: id, CredentialID: id, Provider: "openai", Model: "vendor-" + id, Enabled: true, Capabilities: []modelruntime.Capability{{Operation: op}}}, 0); e != nil {
			t.Fatal(e)
		}
	}
	read := func(path string) map[string]any {
		t.Helper()
		status, out, _ := f.call(t, "GET", base+path, nil, "")
		mustOK(t, status, out)
		return out
	}
	refs := read("/references/supplier-models?public_model_id=tech-chat")["items"].([]any)
	if len(refs) != 2 {
		t.Fatal("picker offered an incompatible supplier model", refs)
	}
	save := func(version int, ids ...string) {
		t.Helper()
		bindings := []map[string]any{}
		for _, id := range ids {
			bindings = append(bindings, map[string]any{"supplier_model_id": id, "enabled": true})
		}
		status, out, _ := f.call(t, "POST", base+"/routes", map[string]any{"route": map[string]any{"public_model_id": "tech-chat", "strategy": "priority", "bindings": bindings}, "expected_version": version}, uuid.NewString())
		mustOK(t, status, out)
	}
	defaults := func() map[string]any {
		t.Helper()
		out := read("/references/models?purpose=routing&q=" + url.QueryEscape("tech-chat"))
		return out["items"].([]any)[0].(map[string]any)["defaults"].(map[string]any)
	}
	save(0, "backup", "primary")
	d := defaults()
	if d["routing_version"] != float64(1) || d["route_bindings"].([]any)[0].(map[string]any)["supplier_model_id"] != "backup" {
		t.Fatal("route picker lost order or used the public model revision", d)
	}
	save(1, "primary", "backup")
	rows, e := f.a.Catalog.AdminUpstreams(ctx, operator, "tech-chat")
	if e != nil || len(rows) != 2 || rows[0].SupplierModelID != "primary" || rows[1].Priority != 1 {
		t.Fatal("array order did not become route priority", rows, e)
	}
	if e = f.a.DB.Create(&catalog.RouteHealth{UpstreamID: rows[0].ID, Circuit: "open", OpenUntil: timePointer(time.Now().Add(time.Minute))}).Error; e != nil {
		t.Fatal(e)
	}
	page := read("/routes")
	for _, item := range page["items"].([]any) {
		row := item.(map[string]any)
		values := row["values"].(map[string]any)
		if row["id"] == "tech-chat" && values["health_summary"] != "可接单" {
			t.Fatal("healthy backup marked unavailable", values)
		}
		if row["id"] == "tech-empty" && values["health_summary"] == "可接单" {
			t.Fatal("empty route marked ready", values)
		}
	}
	save(2, "backup")
	if bindings := defaults()["route_bindings"].([]any); len(bindings) != 1 || bindings[0].(map[string]any)["supplier_model_id"] != "backup" {
		t.Fatal("removed route returned to configuration", bindings)
	}
	status, out, _ := f.call(t, "POST", base+"/routes", map[string]any{"route": map[string]any{"public_model_id": "tech-chat", "bindings": []map[string]any{{"supplier_model_id": "embedding-only", "enabled": true}}}, "expected_version": 3}, uuid.NewString())
	if status < 400 || defaults()["routing_version"] != float64(3) {
		t.Fatal("incompatible binding was accepted or mutated the valid policy", status, out)
	}
}

func TestPostgresTechnologySettingsExposeOnlyNecessaryControls(t *testing.T) {
	f := setupPortal(t)
	f.login(t, "staff", "owner@example.test", "owner-password-test")
	base := "/v1/operators/" + f.a.Installation.OperatorID
	status, page, _ := f.call(t, "GET", base+"/bifrost", nil, "")
	mustOK(t, status, page)
	ops := page["operations"].([]any)
	if len(ops) != 1 || ops[0].(map[string]any)["name"] != "sync" {
		t.Fatal("Bifrost still exposes editable feed policies", ops)
	}
	status, out, _ := f.call(t, "POST", base+"/bifrost", map[string]any{"action": "configure", "source_url": "https://invalid.example/feed"}, uuid.NewString())
	if status < 400 {
		t.Fatal("removed configuration command remained active", status, out)
	}
	status, out, _ = f.call(t, "POST", base+"/bifrost", map[string]any{"action": "sync"}, uuid.NewString())
	mustOK(t, status, out)
	var obsolete int64
	if e := f.a.DB.Raw("SELECT count(*) FROM information_schema.columns WHERE table_schema=current_schema() AND table_name='model_pricing_settings' AND column_name IN ('source_url','interval_hours','enabled')").Scan(&obsolete).Error; e != nil || obsolete != 0 {
		t.Fatal("obsolete feed settings remain in storage", obsolete, e)
	}
}

func timePointer(value time.Time) *time.Time { return &value }
