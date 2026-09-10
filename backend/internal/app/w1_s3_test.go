package app_test

import (
	"context"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/tokpath/tokstudio/backend/internal/media"
	"github.com/tokpath/tokstudio/backend/internal/platform/config"
)

// release/v0.1.0 语义：未配齐 S3 时回落本地目录；配齐但连不上则 OpenStore 失败。
// 与 feature/grokbot「禁止本地回落、徽章标不可用」冲突时以 release 为准。

func TestW1S3LocalFallbackWhenS3NotReady(t *testing.T) {
	if os.Getenv("TOKENHUB_DATABASE_URL") == "" || os.Getenv("TOKENHUB_REDIS_URL") == "" {
		t.Skip("integration test requires postgres and redis")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.BootstrapAdmin = "s3_local_admin"
	cfg.BootstrapUser = "s3_local_user"
	cfg.EncryptionKey = "dev-only-32-byte-key-change-me!!"
	cfg.Env = "test"
	cfg.S3Endpoint = ""
	cfg.S3Bucket = ""
	cfg.S3AccessKey = ""
	cfg.S3SecretKey = ""
	cfg.MediaStorePath = t.TempDir()

	application := mustApp(t, cfg)
	st := application.Media.StoreStatus(context.Background())
	if !st.OK || st.Source != media.SourceLocal {
		t.Fatalf("missing S3 keys must use local store: %+v", st)
	}

	server := httptest.NewServer(application.Router())
	defer server.Close()
	health := getJSON(t, server.URL+"/healthz", "")
	storage, _ := health["storage"].(map[string]any)
	if storage["ok"] != true || storage["source"] != media.SourceLocal {
		t.Fatalf("healthz local storage: %+v", health["storage"])
	}
}

func TestW1S3OpenStoreFailsOnUnreachableBucket(t *testing.T) {
	_, err := media.OpenStore(media.StoreOptions{
		Root:   t.TempDir(),
		Secret: "sign",
		Public: "http://localhost:8080",
		S3: media.S3Options{
			Endpoint:  "http://127.0.0.1:1",
			Bucket:    "missing-tokenhub-bucket",
			AccessKey: "minioadmin",
			SecretKey: "minioadmin",
			Region:    "us-east-1",
			PathStyle: true,
		},
	})
	if err == nil {
		t.Fatal("unreachable S3 with Ready keys must fail OpenStore (release semantics)")
	}
}

func TestW1S3HealthzWhenUsingS3(t *testing.T) {
	if os.Getenv("TOKENHUB_DATABASE_URL") == "" || os.Getenv("TOKENHUB_REDIS_URL") == "" {
		t.Skip("integration test requires postgres and redis")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.S3Bucket == "" || cfg.S3AccessKey == "" || cfg.S3SecretKey == "" {
		t.Skip("S3 not configured in env")
	}
	cfg.BootstrapAdmin = "s3ok_admin"
	cfg.BootstrapUser = "s3ok_user"
	cfg.Env = "test"
	application := mustApp(t, cfg)
	st := application.Media.StoreStatus(context.Background())
	if !st.OK {
		t.Skip("S3/MinIO not reachable: " + st.Detail)
	}
	if st.Label != media.LabelS3 {
		t.Fatalf("configured S3 must show S3 badge: %+v", st)
	}
	server := httptest.NewServer(application.Router())
	defer server.Close()
	health := getJSON(t, server.URL+"/healthz", "")
	storage, _ := health["storage"].(map[string]any)
	if storage["ok"] != true || storage["label"] != "S3" {
		t.Fatalf("healthz s3: %+v", health["storage"])
	}
}
