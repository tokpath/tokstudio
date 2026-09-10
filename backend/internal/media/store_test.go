package media

import (
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestDiskStorePutReadDeleteSign(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root, "sign-secret", "http://localhost:8080")
	if store.UsingS3() {
		t.Fatal("disk store should not use s3")
	}
	if err := store.Put("vid_1/output.bin", "video/mp4", []byte("payload")); err != nil {
		t.Fatal(err)
	}
	got, err := store.Read("vid_1/output.bin")
	if err != nil || string(got) != "payload" {
		t.Fatalf("read %q %v", got, err)
	}
	signed, exp, err := store.Sign("vid_1/output.bin", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(signed, "/v1/media/objects") {
		t.Fatalf("sign %s", signed)
	}
	parsed, err := url.Parse(signed)
	if err != nil {
		t.Fatal(err)
	}
	q := parsed.Query()
	expUnix, _ := strconv.ParseInt(q.Get("exp"), 10, 64)
	if expUnix != exp.Unix() {
		t.Fatalf("exp %d vs %d", expUnix, exp.Unix())
	}
	if !store.Verify(q.Get("key"), q.Get("sig"), expUnix) {
		t.Fatalf("verify %s", signed)
	}
	if err := store.Delete("vid_1/output.bin"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "vid_1", "output.bin")); !os.IsNotExist(err) {
		t.Fatalf("expected delete: %v", err)
	}
}

func TestS3OptionsReady(t *testing.T) {
	if (S3Options{Bucket: "b", AccessKey: "a"}).Ready() {
		t.Fatal("secret missing")
	}
	if !(S3Options{Bucket: "b", AccessKey: "a", SecretKey: "s"}).Ready() {
		t.Fatal("bucket+keys should be ready")
	}
}

func TestS3StoreRoundTrip(t *testing.T) {
	endpoint := os.Getenv("TOKENHUB_S3_ENDPOINT")
	bucket := os.Getenv("TOKENHUB_S3_BUCKET")
	ak := os.Getenv("TOKENHUB_S3_ACCESS_KEY")
	sk := os.Getenv("TOKENHUB_S3_SECRET_KEY")
	if endpoint == "" || bucket == "" || ak == "" || sk == "" {
		t.Skip("S3/MinIO not configured")
	}
	store, err := OpenStore(StoreOptions{
		Secret: "sign-secret",
		S3:     S3Options{Endpoint: endpoint, Bucket: bucket, AccessKey: ak, SecretKey: sk, Region: "us-east-1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !store.UsingS3() {
		t.Fatal("expected s3 backend")
	}
	key := "itest/" + t.Name() + ".bin"
	if err := store.Put(key, "text/plain", []byte("minio-bytes")); err != nil {
		t.Fatal(err)
	}
	got, err := store.Read(key)
	if err != nil || string(got) != "minio-bytes" {
		t.Fatalf("s3 read %q %v", got, err)
	}
	if err := store.Delete(key); err != nil {
		t.Fatal(err)
	}
}
