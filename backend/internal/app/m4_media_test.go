package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/app"
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
	cfg.ArkBaseURL = ""
	cfg.ArkAPIKey = ""
	cfg.OpenRouterBaseURL = ""
	application := mustApp(t, cfg)
	requireObjectStore(t, application)
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
	if !presignedDownloadURL(url) {
		t.Fatalf("expected S3 presigned url, got %+v", content)
	}
	storage, _ := content["storage"].(map[string]any)
	if storage["ok"] != true || storage["label"] != "S3" {
		t.Fatalf("content storage badge: %+v", content["storage"])
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

	fromSession := postAccepted(t, server.URL+"/v1/videos", session, "sess-m4", map[string]any{
		"model": catalog.SeedanceModelID, "prompt": "session river",
	})
	if fromSession["id"] == nil {
		t.Fatalf("session should create media: %+v", fromSession)
	}
	sessionGot := getAuthJSON(t, server.URL+"/v1/videos/"+fromSession["id"].(string), session)
	if sessionGot["id"] != fromSession["id"] {
		t.Fatalf("session get video: %+v", sessionGot)
	}

	params := postAccepted(t, server.URL+"/v1/videos", apiKey, "idem-params", map[string]any{
		"model": catalog.SeedanceModelID, "prompt": "params", "duration": 8, "resolution": "1080p",
		"aspect_ratio": "9:16", "fps": 24, "generate_audio": true, "task_type": "t2v",
	})
	if params["task_type"] != "t2v" || params["duration"] != float64(8) || params["resolution"] != "1080p" ||
		params["aspect_ratio"] != "9:16" || params["fps"] != float64(24) || params["generate_audio"] != true {
		t.Fatalf("params echo: %+v", params)
	}

	badI2V := mustStatusBody(t, http.MethodPost, server.URL+"/v1/videos", apiKey, map[string]any{
		"model": catalog.SeedanceModelID, "prompt": "need image", "task_type": "i2v",
	})
	if badI2V.status != http.StatusBadRequest {
		t.Fatalf("i2v without image: %d %+v", badI2V.status, badI2V.body)
	}
	i2v := postAccepted(t, server.URL+"/v1/videos", apiKey, "idem-i2v", map[string]any{
		"model": catalog.SeedanceModelID, "prompt": "from still", "mode": "i2v", "images": []string{"https://example.test/frame.png"},
	})
	if i2v["task_type"] != "i2v" {
		t.Fatalf("i2v: %+v", i2v)
	}

	badFLF := mustStatusBody(t, http.MethodPost, server.URL+"/v1/videos", apiKey, map[string]any{
		"model": catalog.SeedanceModelID, "prompt": "flf", "task_type": "first_last_frame", "first_frame": "https://example.test/a.png",
	})
	if badFLF.status != http.StatusBadRequest {
		t.Fatalf("first_last_frame missing last: %d %+v", badFLF.status, badFLF.body)
	}
	flf := postAccepted(t, server.URL+"/v1/videos", apiKey, "idem-flf", map[string]any{
		"model": catalog.SeedanceModelID, "prompt": "walk", "task_type": "first_last_frame",
		"first_frame": "https://example.test/a.png", "last_frame": "https://example.test/b.png",
	})
	if flf["task_type"] != "first_last_frame" || flf["first_frame"] == "" || flf["last_frame"] == "" {
		t.Fatalf("first_last_frame: %+v", flf)
	}

	ref := postAccepted(t, server.URL+"/v1/videos", apiKey, "idem-ref", map[string]any{
		"model": catalog.SeedanceModelID, "prompt": "ref", "task_type": "reference", "reference_audio": "https://example.test/a.wav",
	})
	if ref["task_type"] != "reference" || ref["reference_audio"] == "" {
		t.Fatalf("reference: %+v", ref)
	}

	badExt := mustStatusBody(t, http.MethodPost, server.URL+"/v1/videos", apiKey, map[string]any{
		"model": catalog.SeedanceModelID, "prompt": "extend", "task_type": "extend",
	})
	if badExt.status != http.StatusBadRequest {
		t.Fatalf("extend without source: %d %+v", badExt.status, badExt.body)
	}
	other := postBody(t, server.URL+"/v1/auth/register", "", map[string]string{
		"email":    "media-other-" + strconv.FormatInt(time.Now().UnixNano(), 10) + "@example.test",
		"password": "password1", "promotion_code": "THA1",
	})
	otherKey := postJSONRaw(t, server.URL+"/v1/me/api-keys", tokenOf(other), map[string]any{"name": "m4-other"})["item"].(map[string]any)["key"].(string)
	stolen := mustStatusBody(t, http.MethodPost, server.URL+"/v1/videos", otherKey, map[string]any{
		"model": catalog.SeedanceModelID, "prompt": "steal", "task_type": "extend", "source_job_id": first["id"],
	})
	if stolen.status != http.StatusBadRequest {
		t.Fatalf("extend other user job: %d %+v", stolen.status, stolen.body)
	}
	extended := postAccepted(t, server.URL+"/v1/videos", apiKey, "idem-ext", map[string]any{
		"model": catalog.SeedanceModelID, "prompt": "longer", "task_type": "extend", "source_job_id": first["id"],
	})
	if extended["task_type"] != "extend" || extended["source_job_id"] != first["id"] {
		t.Fatalf("extend: %+v", extended)
	}
	viaRoute := postAccepted(t, server.URL+"/v1/videos/"+first["id"].(string)+"/extend", apiKey, "idem-ext-route", map[string]any{
		"model": catalog.SeedanceModelID, "prompt": "via route", "duration": 6,
	})
	if viaRoute["task_type"] != "extend" || viaRoute["source_job_id"] != first["id"] {
		t.Fatalf("extend route: %+v", viaRoute)
	}

	badEdit := mustStatusBody(t, http.MethodPost, server.URL+"/v1/images/edits", apiKey, map[string]any{
		"model": catalog.ImageModelID, "prompt": "edit me",
	})
	if badEdit.status != http.StatusBadRequest {
		t.Fatalf("image edit without images: %d %+v", badEdit.status, badEdit.body)
	}
	imgEdit := postAccepted(t, server.URL+"/v1/images/edits", apiKey, "idem-img-edit", map[string]any{
		"model": catalog.ImageModelID, "prompt": "make blue", "images": []string{"https://example.test/logo.png"},
	})
	if imgEdit["object"] != "image" || imgEdit["task_type"] != "edit" {
		t.Fatalf("image edit: %+v", imgEdit)
	}
}

func TestM4PollAndCustomerCallback(t *testing.T) {
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
	cfg.ArkBaseURL = ""
	cfg.ArkAPIKey = ""
	cfg.OpenRouterBaseURL = ""
	application := mustApp(t, cfg)
	server := httptest.NewServer(application.Router())
	defer server.Close()

	reg := postBody(t, server.URL+"/v1/auth/register", "", map[string]string{
		"email":    "media-cb-" + t.Name() + "-" + strconv.FormatInt(time.Now().UnixNano(), 10) + "@example.test",
		"password": "password1", "promotion_code": "THA1",
	})
	session := tokenOf(reg)
	apiKey := postJSONRaw(t, server.URL+"/v1/me/api-keys", session, map[string]any{"name": "m4-cb"})["item"].(map[string]any)["key"].(string)
	_ = postJSONRaw(t, server.URL+"/v1/topups/redeem", session, map[string]any{"code": billing.RedeemE2E})

	var got []map[string]any
	var sigs []string
	cb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sigs = append(sigs, r.Header.Get("X-Tokenhub-Signature"))
		var payload map[string]any
		_ = json.NewDecoder(r.Body).Decode(&payload)
		got = append(got, payload)
		w.WriteHeader(http.StatusOK)
	}))
	defer cb.Close()

	syncJob := postAccepted(t, server.URL+"/v1/videos", apiKey, "idem-cb-sync", map[string]any{
		"model": catalog.SeedanceModelID, "prompt": "callback sync", "duration": 5,
		"callback_url": cb.URL,
	})
	if syncJob["status"] != "completed" {
		t.Fatalf("sync job: %+v", syncJob)
	}
	if len(got) < 1 {
		application.Media.Tick(t.Context())
	}
	if len(got) < 1 {
		t.Fatal("expected customer callback after completed job")
	}
	eventID, _ := got[0]["event_id"].(string)
	if eventID == "" || got[0]["job_id"] != syncJob["id"] || got[0]["status"] != "completed" {
		t.Fatalf("callback payload %+v", got[0])
	}
	want := application.Media.SignCallback(eventID, syncJob["id"].(string))
	if len(sigs) < 1 || sigs[0] != want {
		t.Fatalf("signature %q want %q", sigs, want)
	}

	asyncJob := postAccepted(t, server.URL+"/v1/videos", apiKey, "idem-cb-async", map[string]any{
		"model": catalog.SeedanceModelID, "prompt": "force-async", "duration": 5,
		"callback_url": cb.URL,
	})
	if asyncJob["status"] != "in_progress" {
		t.Fatalf("async: %+v", asyncJob)
	}
	before := len(got)
	application.Media.CompleteTestJob(asyncJob["upstream_job_id"].(string), []byte("polled-bytes"))
	polled := getAuthJSON(t, server.URL+"/v1/videos/"+asyncJob["id"].(string), apiKey)
	if polled["status"] != "completed" {
		t.Fatalf("poll via GET: %+v", polled)
	}
	if len(got) <= before {
		application.Media.Tick(t.Context())
	}
	if len(got) <= before {
		t.Fatal("expected callback after poll complete")
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

func presignedDownloadURL(url string) bool {
	return url != "" && (strings.Contains(url, "X-Amz-") || strings.Contains(url, "/v1/media/objects"))
}

func requireObjectStore(t *testing.T, application *app.App) {
	t.Helper()
	st := application.Media.StoreStatus(context.Background())
	if !st.OK {
		t.Skip("integration test requires MinIO/S3: " + st.Detail)
	}
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
