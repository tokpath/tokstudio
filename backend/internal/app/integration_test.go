package app_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/app"
	"github.com/tokpath/tokstudio/backend/internal/platform/config"
	"github.com/tokpath/tokstudio/backend/internal/platform/db"
	"github.com/tokpath/tokstudio/backend/internal/platform/logx"
	"github.com/tokpath/tokstudio/backend/internal/platform/redisx"
)

func TestM0Foundation(t *testing.T) {
	if os.Getenv("TOKENHUB_DATABASE_URL") == "" || os.Getenv("TOKENHUB_REDIS_URL") == "" {
		t.Skip("integration test requires TOKENHUB_DATABASE_URL and TOKENHUB_REDIS_URL")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.BootstrapAdmin = "test_admin_token"
	cfg.BootstrapUser = "test_user_token"
	cfg.Env = "test"

	gdb, err := db.Open(cfg.DatabaseURL)
	if err != nil {
		t.Fatal(err)
	}
	rdb, err := redisx.Open(cfg.RedisURL)
	if err != nil {
		t.Fatal(err)
	}
	logger := logx.New("error", os.Stdout)
	application := app.New(cfg, gdb, rdb, logger)
	if err := application.Migrate(); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := application.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	defer application.Close()

	server := httptest.NewServer(application.Router())
	defer server.Close()

	health := getJSON(t, server.URL+"/healthz", "")
	if health["status"] != "ok" {
		t.Fatalf("health: %+v", health)
	}

	unauth := mustStatus(t, http.MethodGet, server.URL+"/admin/audit-logs", "", nil)
	if unauth != http.StatusForbidden {
		t.Fatalf("unauth expected 403, got %d", unauth)
	}
	userDenied := mustStatus(t, http.MethodGet, server.URL+"/admin/audit-logs", "test_user_token", nil)
	if userDenied != http.StatusForbidden {
		t.Fatalf("user expected 403, got %d", userDenied)
	}

	created := postJSON(t, server.URL+"/admin/audit-probes", "test_admin_token")
	if created["item"] == nil {
		t.Fatalf("audit probe: %+v", created)
	}

	listed := getJSON(t, server.URL+"/admin/audit-logs", "test_admin_token")
	items, _ := listed["items"].([]any)
	if len(items) == 0 {
		t.Fatal("expected audit entries")
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		stats := getJSON(t, server.URL+"/admin/outbox/stats", "test_admin_token")
		raw, _ := stats["stats"].(map[string]any)
		if raw != nil && raw["pending"] != nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("outbox stats missing: %+v", stats)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func getJSON(t *testing.T, url, token string) map[string]any {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	return body
}

func postJSON(t *testing.T, url, token string) map[string]any {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, url, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	return body
}

func mustStatus(t *testing.T, method, url, token string, _ any) int {
	t.Helper()
	req, _ := http.NewRequest(method, url, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	return resp.StatusCode
}
