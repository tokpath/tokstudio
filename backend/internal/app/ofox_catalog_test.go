package app_test

import (
	"context"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"

	"github.com/tokpath/tokstudio/backend/internal/app"
	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"github.com/tokpath/tokstudio/backend/internal/platform/config"
	"github.com/tokpath/tokstudio/backend/internal/platform/db"
	"github.com/tokpath/tokstudio/backend/internal/platform/logx"
	"github.com/tokpath/tokstudio/backend/internal/platform/redisx"
)

func TestOfoxCatalogRequiresRouteAndKeepsOEMIsolated(t *testing.T) {
	if os.Getenv("TOKENHUB_DATABASE_URL") == "" || os.Getenv("TOKENHUB_REDIS_URL") == "" {
		t.Skip("integration test requires TOKENHUB_DATABASE_URL and TOKENHUB_REDIS_URL")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Env = "test"
	gdb, err := db.Open(cfg.DatabaseURL)
	if err != nil {
		t.Fatal(err)
	}
	rdb, err := redisx.Open(cfg.RedisURL)
	if err != nil {
		t.Fatal(err)
	}
	application := app.New(cfg, gdb, rdb, logx.New("error", os.Stdout))
	if err := application.Migrate(); err != nil {
		t.Fatal(err)
	}
	if err := application.Bootstrap(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer application.Close()
	server := httptest.NewServer(application.Router())
	defer server.Close()

	const ofoxID = "z-ai/glm-5.3-flash"
	adminItems, err := application.Catalog.ListAdminModels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(adminItems) < 100 {
		t.Fatalf("admin catalog should retain ofox dump, got %d", len(adminItems))
	}
	adminFound := false
	for _, row := range adminItems {
		if row.ID == ofoxID {
			adminFound = true
			if row.Kind == "" && row.DisplayName == "" {
				t.Fatalf("ofox row incomplete: %+v", row)
			}
		}
	}
	if !adminFound {
		t.Fatalf("admin catalog missing %s", ofoxID)
	}

	official := getAuthJSON(t, server.URL+"/v1/public/models", "")
	items, _ := official["items"].([]any)
	for _, raw := range items {
		row := raw.(map[string]any)
		if _, ok := row["providers"]; ok {
			t.Fatalf("public models must not leak providers: %+v", row)
		}
		if row["id"] == ofoxID {
			t.Fatalf("unrouted ofox model must stay out of public catalog: %+v", row)
		}
	}

	oem := getAuthJSON(t, server.URL+"/v1/public/models?host=oem.localhost", "")
	for _, raw := range oem["items"].([]any) {
		row := raw.(map[string]any)
		if row["id"] == ofoxID {
			t.Fatalf("OEM catalog must not include ofox dump %s", ofoxID)
		}
	}

	site := getAuthJSON(t, server.URL+"/v1/public/site", "")
	payload, _ := site["site"].(map[string]any)
	if payload["leaderboards"] == nil || payload["blog"] == nil {
		t.Fatalf("public site snapshot missing: %+v", site)
	}

	filtered := getAuthJSON(t, server.URL+"/v1/public/models?vendor=tokenhub&kind=text", "")
	filteredItems, _ := filtered["items"].([]any)
	if len(filteredItems) == 0 {
		t.Fatalf("vendor=tokenhub&kind=text returned no items: %+v", filtered)
	}
	for _, raw := range filteredItems {
		row := raw.(map[string]any)
		if row["vendor"] != "tokenhub" {
			t.Fatalf("vendor filter leaked %v", row["vendor"])
		}
		if row["kind"] != "text" {
			t.Fatalf("kind filter leaked %v for %v", row["kind"], row["id"])
		}
	}
	facets, _ := filtered["facets"].(map[string]any)
	if facets["kinds"] == nil || facets["vendors"] == nil {
		t.Fatalf("filtered catalog missing facets: %+v", filtered)
	}
	if n, _ := filtered["total"].(float64); int(n) != len(filteredItems) {
		t.Fatalf("total=%v items=%d", filtered["total"], len(filteredItems))
	}

	one := getAuthJSON(t, server.URL+"/v1/public/models?id="+url.QueryEscape(catalog.EchoModelID), "")
	oneItems, _ := one["items"].([]any)
	if len(oneItems) != 1 || oneItems[0].(map[string]any)["id"] != catalog.EchoModelID {
		t.Fatalf("id filter: %+v", one)
	}
	unrouted := getAuthJSON(t, server.URL+"/v1/public/models?id="+url.QueryEscape(ofoxID), "")
	if rows, _ := unrouted["items"].([]any); len(rows) != 0 {
		t.Fatalf("id filter exposed unrouted model: %+v", unrouted)
	}
}
