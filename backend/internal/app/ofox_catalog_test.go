package app_test

import (
	"context"
	"os"
	"testing"

	"github.com/tokpath/tokstudio/backend/internal/app"
	"github.com/tokpath/tokstudio/backend/internal/platform/config"
	"github.com/tokpath/tokstudio/backend/internal/platform/db"
	"github.com/tokpath/tokstudio/backend/internal/platform/logx"
	"github.com/tokpath/tokstudio/backend/internal/platform/redisx"
	"net/http/httptest"
)

func TestOfoxCatalogDumpIsServedAndOEMIsolated(t *testing.T) {
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
	official := getAuthJSON(t, server.URL+"/v1/public/models", "")
	items, _ := official["items"].([]any)
	if len(items) < 100 {
		t.Fatalf("official catalog should include ofox dump, got %d", len(items))
	}
	found := false
	for _, raw := range items {
		row := raw.(map[string]any)
		if _, ok := row["providers"]; ok {
			t.Fatalf("public models must not leak providers: %+v", row)
		}
		if row["id"] == ofoxID {
			found = true
			if row["kind"] == "" && row["display_name"] == "" {
				t.Fatalf("ofox row incomplete: %+v", row)
			}
		}
	}
	if !found {
		t.Fatalf("official /v1/public/models missing %s", ofoxID)
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
}
