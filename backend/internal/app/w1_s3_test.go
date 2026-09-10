package app_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/media"
	"github.com/tokpath/tokstudio/backend/internal/platform/config"
)

func TestW1S3MissingBucketNeverSilentLocalSuccess(t *testing.T) {
	if os.Getenv("TOKENHUB_DATABASE_URL") == "" || os.Getenv("TOKENHUB_REDIS_URL") == "" {
		t.Skip("integration test requires postgres and redis")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.BootstrapAdmin = "s3_admin"
	cfg.BootstrapUser = "s3_user"
	cfg.BootstrapChannel = "s3_admin-b"
	cfg.EncryptionKey = "dev-only-32-byte-key-change-me!!"
	cfg.Env = "test"
	cfg.S3Endpoint = "http://127.0.0.1:1"
	cfg.S3Bucket = "missing-tokenhub-bucket"
	cfg.S3AccessKey = "minioadmin"
	cfg.S3SecretKey = "minioadmin"
	cfg.S3ForcePathStyle = true
	cfg.S3PublicEndpoint = "http://127.0.0.1:1"
	root := t.TempDir()
	cfg.MediaStorePath = root

	application := mustApp(t, cfg)
	st := application.Media.StoreStatus(context.Background())
	if st.OK || st.Label != media.LabelUnavailable {
		t.Fatalf("dead endpoint must be 存储不可用: %+v", st)
	}

	server := httptest.NewServer(application.Router())
	defer server.Close()

	health := getJSON(t, server.URL+"/healthz", "")
	storage, _ := health["storage"].(map[string]any)
	if storage["ok"] == true || storage["label"] != "存储不可用" {
		t.Fatalf("healthz must not pretend S3 success: %+v", health["storage"])
	}

	reg := postBody(t, server.URL+"/v1/auth/register", "", map[string]string{
		"email":    "s3-fail-" + strconv.FormatInt(time.Now().UnixNano(), 10) + "@example.test",
		"password": "password1", "promotion_code": "THA1",
	})
	session := tokenOf(reg)
	apiKey := postJSONRaw(t, server.URL+"/v1/me/api-keys", session, map[string]any{"name": "s3"})["item"].(map[string]any)["key"].(string)
	_ = postJSONRaw(t, server.URL+"/v1/topups/redeem", session, map[string]any{"code": "THE2E"})

	created := mustStatusBody(t, http.MethodPost, server.URL+"/v1/videos", apiKey, map[string]any{
		"model": catalog.SeedanceModelID, "prompt": "must not land on disk",
	})
	if created.status != http.StatusServiceUnavailable {
		t.Fatalf("store failure must be 503, got %d %+v", created.status, created.body)
	}
	errBody, _ := created.body["error"].(map[string]any)
	if errBody["code"] != "store_unavailable" || errBody["message"] != "存储不可用" {
		t.Fatalf("media error: %+v", created.body)
	}

	cLogin := postBody(t, server.URL+"/v1/auth/login", "", map[string]string{
		"email": "channel.c@tokenhub.local", "password": identity.BootstrapPassword,
	})
	cToken := cLogin["session"].(map[string]any)["token"].(string)
	code, body := postBrandFileStatus(t, server.URL+"/channel/brand/assets", cToken, "logo", "logo.png", pngBytes(t, 128, 128))
	if code != http.StatusServiceUnavailable {
		t.Fatalf("OEM upload must be 503, got %d %+v", code, body)
	}
	errObj, _ := body["error"].(map[string]any)
	if errObj["code"] != "store_unavailable" || errObj["message"] != "存储不可用" {
		t.Fatalf("OEM upload must not look like success: %+v", body)
	}

	assertNoLocalObjectFiles(t, root)
	assertNoLocalObjectFiles(t, filepath.Join(os.TempDir(), "tokenhub-media"))
}

func TestW1S3HealthzMinIOWithoutCloudKeys(t *testing.T) {
	if os.Getenv("TOKENHUB_DATABASE_URL") == "" || os.Getenv("TOKENHUB_REDIS_URL") == "" {
		t.Skip("integration test requires postgres and redis")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.BootstrapAdmin = "s3ok_admin"
	cfg.BootstrapUser = "s3ok_user"
	cfg.S3AccessKey = ""
	cfg.S3SecretKey = ""
	cfg.S3Endpoint = ""
	cfg.Env = "test"
	application := mustApp(t, cfg)
	st := application.Media.StoreStatus(context.Background())
	if !st.OK {
		t.Skip("CI/dev MinIO path requires a running bucket: " + st.Detail)
	}
	if st.Source != media.SourceMinIO || st.Label != media.LabelS3 {
		t.Fatalf("no-key path must be MinIO S3 badge: %+v", st)
	}
	server := httptest.NewServer(application.Router())
	defer server.Close()
	health := getJSON(t, server.URL+"/healthz", "")
	storage, _ := health["storage"].(map[string]any)
	if storage["ok"] != true || storage["label"] != "S3" || storage["source"] != "minio" {
		t.Fatalf("healthz minio: %+v", health["storage"])
	}
}

func assertNoLocalObjectFiles(t *testing.T, root string) {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		t.Fatal(err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			t.Fatalf("S3 failure wrote local file %s", filepath.Join(root, e.Name()))
		}
		nested, _ := os.ReadDir(filepath.Join(root, e.Name()))
		if len(nested) > 0 {
			t.Fatalf("S3 failure wrote local tree %s: %d entries", filepath.Join(root, e.Name()), len(nested))
		}
	}
}
