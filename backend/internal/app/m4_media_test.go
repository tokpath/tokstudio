package app_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/billing"
	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"github.com/tokpath/tokstudio/backend/internal/platform/config"
)

func TestM4MediaJobs(t *testing.T) {
	if os.Getenv("TOKENHUB_DATABASE_URL") == "" || os.Getenv("TOKENHUB_REDIS_URL") == "" {
		t.Skip("integration test requires postgres and redis")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.BootstrapAdmin = "m4_admin"
	cfg.BootstrapUser = "m4_user"
	cfg.EncryptionKey = "dev-only-32-byte-key-change-me!!"
	application := mustApp(t, cfg)
	server := httptest.NewServer(application.Router())
	defer server.Close()

	reg := postBody(t, server.URL+"/v1/auth/register", "", map[string]string{
		"email":    "media-" + t.Name() + "-" + strconv.FormatInt(time.Now().UnixNano(), 10) + "@example.test",
		"password": "password1", "promotion_code": "THA1",
	})
	session := tokenOf(reg)
	apiKey := postJSONRaw(t, server.URL+"/v1/me/api-keys", session, map[string]any{"name": "m4"})["item"].(map[string]any)["key"].(string)
	_ = postJSONRaw(t, server.URL+"/v1/topups/redeem", session, map[string]any{"code": billing.RedeemE2E})

	before := application.Media.TestCreates()
	first := postAccepted(t, server.URL+"/v1/videos", apiKey, "idem-m4-1", map[string]any{
		"model": catalog.SeedanceModelID, "prompt": "a cat walks", "duration": 5, "resolution": "720p",
	})
	if first["status"] != "completed" || first["upstream_job_id"] == "" {
		t.Fatalf("create: %+v", first)
	}
	second := postAccepted(t, server.URL+"/v1/videos", apiKey, "idem-m4-1", map[string]any{
		"model": catalog.SeedanceModelID, "prompt": "a cat walks", "duration": 5,
	})
	if first["id"] != second["id"] {
		t.Fatalf("idempotency created a new job: %v %v", first["id"], second["id"])
	}
	mine := getAuthJSON(t, server.URL+"/v1/me/media", session)
	found := false
	for _, raw := range mine["items"].([]any) {
		if raw.(map[string]any)["id"] == first["id"] {
			found = true
			if raw.(map[string]any)["kind"] != "video" {
				t.Fatalf("media list kind: %+v", raw)
			}
		}
	}
	if !found {
		t.Fatalf("user media list missing job: %+v", mine)
	}
	adminCSV := getBytesAuth(t, server.URL+"/admin/media?format=csv", "m4_admin")
	if !strings.Contains(string(adminCSV), first["id"].(string)) || strings.Contains(string(adminCSV), "a cat walks") {
		t.Fatalf("admin media csv should list id without prompt: %s", adminCSV)
	}
	if application.Media.TestCreates() != before+1 {
		t.Fatalf("adapter resubmitted after upstream id was stored: %d -> %d", before, application.Media.TestCreates())
	}

	content := getAuthJSON(t, server.URL+"/v1/videos/"+first["id"].(string)+"/content", apiKey)
	url, _ := content["url"].(string)
	if url == "" || !strings.Contains(url, "/v1/media/objects") {
		t.Fatalf("expected signed url, got %+v", content)
	}
	if !strings.HasPrefix(url, "http") {
		url = server.URL + url
	}
	raw := getBytes(t, url)
	if !bytes.Contains(raw, []byte("tokenhub-sandbox-media")) {
		t.Fatalf("signed download missing payload")
	}

	asyncJob := postAccepted(t, server.URL+"/v1/videos", apiKey, "idem-async", map[string]any{
		"model": catalog.SeedanceModelID, "prompt": "force-async", "duration": 5,
	})
	if asyncJob["status"] != "in_progress" {
		t.Fatalf("async: %+v", asyncJob)
	}
	event := map[string]any{
		"event_id": "evt-dup-" + t.Name() + "-" + strconv.FormatInt(time.Now().UnixNano(), 10),
		"job_id":   asyncJob["id"],
		"usage":    map[string]int{"video_seconds": 5},
	}
	sig := application.Media.SignCallback(event["event_id"].(string), asyncJob["id"].(string))
	cb1 := postCallback(t, server.URL+"/v1/media/callbacks", sig, event)
	cb2 := postCallback(t, server.URL+"/v1/media/callbacks", sig, event)
	if cb1["ok"] != true || cb2["ok"] != true {
		t.Fatalf("callback should be idempotent 2xx: %+v %+v", cb1, cb2)
	}
	got := getAuthJSON(t, server.URL+"/v1/videos/"+asyncJob["id"].(string), apiKey)
	if got["status"] != "completed" {
		t.Fatalf("after callback: %+v", got)
	}

	failJob := postAccepted(t, server.URL+"/v1/videos", apiKey, "idem-fail", map[string]any{
		"model": catalog.SeedanceModelID, "prompt": "force-fail",
	})
	if failJob["status"] != "failed" {
		t.Fatalf("fail: %+v", failJob)
	}

	cancelSrc := postAccepted(t, server.URL+"/v1/videos", apiKey, "idem-cancel", map[string]any{
		"model": catalog.SeedanceModelID, "prompt": "force-async",
	})
	beforeBal := num(getAuthJSON(t, server.URL+"/v1/me/balance", session)["balance"].(map[string]any)["reserved_minor"])
	cancel := postJSONRaw(t, server.URL+"/v1/videos/"+cancelSrc["id"].(string)+"/cancel", apiKey, map[string]any{})
	if cancel["status"] != "cancelled" {
		t.Fatalf("cancel: %+v", cancel)
	}
	afterBal := num(getAuthJSON(t, server.URL+"/v1/me/balance", session)["balance"].(map[string]any)["reserved_minor"])
	if afterBal >= beforeBal && beforeBal > 0 {
		t.Fatalf("cancel should release reserve: before=%v after=%v", beforeBal, afterBal)
	}

	img := postAccepted(t, server.URL+"/v1/images/generations", apiKey, "idem-img", map[string]any{
		"model": catalog.ImageModelID, "prompt": "logo",
	})
	if img["object"] != "image" || img["status"] != "completed" {
		t.Fatalf("image: %+v", img)
	}
}

func postAccepted(t *testing.T, url, token, idem string, payload map[string]any) map[string]any {
	t.Helper()
	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", idem)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
		t.Fatalf("POST %s %d %v", url, resp.StatusCode, out)
	}
	return out
}

func postCallback(t *testing.T, url, sig string, payload map[string]any) map[string]any {
	t.Helper()
	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tokenhub-Signature", sig)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if resp.StatusCode >= 300 {
		t.Fatalf("callback %d %v", resp.StatusCode, out)
	}
	return out
}

func getBytesAuth(t *testing.T, url, token string) []byte {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		t.Fatalf("GET %s %d %s", url, resp.StatusCode, body)
	}
	return body
}

func getBytes(t *testing.T, url string) []byte {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		t.Fatalf("GET %s %d %s", url, resp.StatusCode, body)
	}
	return body
}

func num(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	default:
		return 0
	}
}
