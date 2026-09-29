package app_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/platform/config"
)

func TestHierarchicalModelAuthorization(t *testing.T) {
	if os.Getenv("TOKENHUB_DATABASE_URL") == "" {
		t.Skip("postgres required")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.BootstrapAdmin = "model_chain_admin"
	cfg.BootstrapUser = "model_chain_user"
	cfg.EncryptionKey = "dev-only-32-byte-key-change-me!!"
	a := mustApp(t, cfg)
	server := httptest.NewServer(a.Router())
	defer server.Close()
	ctx := context.Background()
	suffix := strconv.FormatInt(time.Now().UnixNano(), 10)
	brandID := postJSONRaw(t, server.URL+"/admin/brands", cfg.BootstrapAdmin, map[string]any{
		"name":           "Model OEM " + suffix,
		"primary_domain": "model-oem-" + suffix + ".localhost",
		"api_domain":     "api-model-oem-" + suffix + ".localhost",
		"admin_domain":   "admin-model-oem-" + suffix + ".localhost",
	})["item"].(map[string]any)["id"].(string)
	newOEM := postJSONRaw(t, server.URL+"/admin/channels", cfg.BootstrapAdmin, map[string]any{
		"code": "model-oem-" + suffix, "type": "C", "brand_id": brandID,
	})["item"].(map[string]any)["id"].(string)
	if code, _ := doJSON(t, http.MethodPost, server.URL+"/admin/channels", cfg.BootstrapAdmin, true, map[string]any{
		"code": "duplicate-brand-" + suffix, "type": "C", "brand_id": brandID,
	}); code != http.StatusBadRequest {
		t.Fatalf("one brand cannot belong to two OEM platforms: %d", code)
	}
	checkVisible := func(channelID string, want bool) {
		t.Helper()
		_, err := a.Catalog.GetVisibleModel(ctx, channelID, catalog.EchoModelID, nil)
		if want && err != nil {
			t.Fatalf("%s should see echo: %v", channelID, err)
		}
		if !want && !errors.Is(err, catalog.ErrModelNotVisible) {
			t.Fatalf("%s should not see echo: %v", channelID, err)
		}
	}
	checkVisible(newOEM, false)
	parent := identity.OEMChannelID
	grant := func(target, token string, enabled bool) {
		t.Helper()
		patchJSONRaw(t, server.URL+"/admin/channels/"+target+"/models", token, map[string]any{
			"items": []map[string]any{{"public_id": catalog.EchoModelID, "enabled": enabled,
				"wholesale": map[string]string{"input": "0.0000007", "output": "0.0000014"}}},
		})
	}
	grant(parent, cfg.BootstrapAdmin, false)
	checkVisible(parent, false)
	grant(parent, cfg.BootstrapAdmin, true)
	checkVisible(parent, true)
	child := postJSONRaw(t, server.URL+"/admin/channels", cfg.BootstrapAdmin+"-c", map[string]any{
		"code": "model-b-" + suffix, "type": "B", "parent_id": parent,
	})["item"].(map[string]any)["id"].(string)
	checkVisible(child, false)
	if models := getAuthJSON(t, server.URL+"/admin/channels/"+child+"/models", cfg.BootstrapAdmin+"-c"); models["items"] == nil {
		t.Fatalf("OEM must see grant candidates: %+v", models)
	}
	if code, _ := doJSON(t, http.MethodPatch, server.URL+"/admin/channels/"+child+"/models", cfg.BootstrapAdmin+"-b", true,
		map[string]any{"items": []map[string]any{{"public_id": catalog.EchoModelID, "enabled": true}}}); code != http.StatusForbidden {
		t.Fatalf("unrelated channel could grant child: %d", code)
	}
	if code, _ := doJSON(t, http.MethodPatch, server.URL+"/admin/channels/"+child+"/models", cfg.BootstrapAdmin, true,
		map[string]any{"items": []map[string]any{{"public_id": catalog.EchoModelID, "enabled": true}}}); code != http.StatusForbidden {
		t.Fatalf("platform granted OEM grandchild: %d", code)
	}
	grant(child, cfg.BootstrapAdmin+"-c", true)
	checkVisible(child, true)
	if code, _ := doJSON(t, http.MethodPatch, server.URL+"/channel/model-prices", cfg.BootstrapAdmin+"-b", true, map[string]any{
		"public_id": catalog.EchoModelID, "customer_price": map[string]string{"input": "0.000003", "output": "0.000005"},
	}); code != http.StatusForbidden {
		t.Fatalf("B channel changed brand customer price: %d", code)
	}
	if code, _ := doJSON(t, http.MethodPatch, server.URL+"/admin/channels/"+child+"/models", cfg.BootstrapAdmin+"-c", true, map[string]any{
		"items": []map[string]any{{"public_id": catalog.EchoModelID, "enabled": true, "customer_override": map[string]string{"input": "0.000004", "output": "0.000006"}}},
	}); code != http.StatusBadRequest {
		t.Fatalf("OEM gave B a separate customer price: %d", code)
	}
	if code, _ := doJSON(t, http.MethodPatch, server.URL+"/channel/model-prices", cfg.BootstrapAdmin+"-c", true, map[string]any{
		"public_id": catalog.EchoModelID, "customer_price": map[string]string{"input": "0.000003", "output": "0.000005"},
	}); code != http.StatusOK {
		t.Fatalf("OEM could not set brand customer price: %d", code)
	}
	quote, err := a.Catalog.PriceForChannel(ctx, child, catalog.EchoModelID, json.RawMessage(`{"input":"0.000001","output":"0.000002"}`))
	if err != nil {
		t.Fatal(err)
	}
	var prices map[string]string
	if err := json.Unmarshal(quote, &prices); err != nil {
		t.Fatal(err)
	}
	if prices["input"] != "0.000003" || prices["output"] != "0.000005" || prices["wholesale_input"] != "0.0000007" {
		t.Fatalf("B did not inherit OEM customer price while keeping its settlement terms: %+v", prices)
	}
	grant(parent, cfg.BootstrapAdmin, false)
	checkVisible(parent, false)
	checkVisible(child, false)
	if err := a.Catalog.SetOwnChannelModelEnabled(ctx, child, catalog.EchoModelID, true); !errors.Is(err, catalog.ErrModelNotVisible) {
		t.Fatalf("child must not override parent revocation: %v", err)
	}
	checkVisible(child, false)
	grant(parent, cfg.BootstrapAdmin, true)
	checkVisible(child, true)
	patchJSONRaw(t, server.URL+"/channel/models", cfg.BootstrapAdmin+"-c", map[string]any{"public_id": catalog.EchoModelID, "enabled": false})
	checkVisible(parent, false)
	checkVisible(child, false)
	patchJSONRaw(t, server.URL+"/channel/models", cfg.BootstrapAdmin+"-c", map[string]any{"public_id": catalog.EchoModelID, "enabled": true})
	checkVisible(child, true)
	if err := a.Catalog.SetOwnChannelModelEnabled(ctx, child, catalog.EchoModelID, false); err != nil {
		t.Fatal(err)
	}
	checkVisible(child, false)
	checkVisible(parent, true)
	if err := a.Catalog.SetOwnChannelModelEnabled(ctx, child, catalog.EchoModelID, true); err != nil {
		t.Fatal(err)
	}
	grant(child, cfg.BootstrapAdmin+"-c", false)
	checkVisible(child, false)
	if err := a.Catalog.SetOwnChannelModelEnabled(ctx, child, catalog.EchoModelID, true); !errors.Is(err, catalog.ErrModelNotVisible) {
		t.Fatalf("child restored revoked grant: %v", err)
	}
	grant(child, cfg.BootstrapAdmin+"-c", true)
	checkVisible(child, true)
	t.Cleanup(func() {
		_ = a.DB.Exec("UPDATE catalog_public_models SET status = 'published' WHERE public_id = ?", catalog.EchoModelID).Error
		_ = a.DB.Exec("UPDATE catalog_route_groups SET status = 'active' WHERE public_model_id IN (SELECT id FROM catalog_public_models WHERE public_id = ?)", catalog.EchoModelID).Error
	})
	if _, err := a.Catalog.DeprecateModel(ctx, catalog.EchoModelID); err != nil {
		t.Fatal(err)
	}
	checkVisible(parent, false)
	checkVisible(child, false)
}
