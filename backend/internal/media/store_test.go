package media

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

type fakeS3 struct {
	objects map[string][]byte
	putErr  error
	headErr error
	puts    int
}

func (f *fakeS3) PutObject(_ context.Context, params *s3.PutObjectInput, _ ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	f.puts++
	if f.putErr != nil {
		return nil, f.putErr
	}
	if f.objects == nil {
		f.objects = map[string][]byte{}
	}
	body, _ := io.ReadAll(params.Body)
	f.objects[awsString(params.Key)] = body
	return &s3.PutObjectOutput{}, nil
}

func (f *fakeS3) GetObject(_ context.Context, params *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	data, ok := f.objects[awsString(params.Key)]
	if !ok {
		return nil, &s3types.NoSuchKey{}
	}
	return &s3.GetObjectOutput{Body: io.NopCloser(bytes.NewReader(data))}, nil
}

func (f *fakeS3) DeleteObject(_ context.Context, params *s3.DeleteObjectInput, _ ...func(*s3.Options)) (*s3.DeleteObjectOutput, error) {
	delete(f.objects, awsString(params.Key))
	return &s3.DeleteObjectOutput{}, nil
}

func (f *fakeS3) HeadBucket(context.Context, *s3.HeadBucketInput, ...func(*s3.Options)) (*s3.HeadBucketOutput, error) {
	if f.headErr != nil {
		return nil, f.headErr
	}
	return &s3.HeadBucketOutput{}, nil
}

type fakePresign struct {
	url string
	err error
}

func (f fakePresign) PresignGetObject(context.Context, *s3.GetObjectInput, ...func(*s3.PresignOptions)) (*v4Presigned, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &v4Presigned{URL: f.url}, nil
}

func awsString(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func TestResolveSettingsNoKeyUsesMinIO(t *testing.T) {
	got := ResolveSettings(Settings{})
	if got.Endpoint != defaultMinIOHost || got.AccessKey != defaultMinIOUser || got.SecretKey != defaultMinIOPass {
		t.Fatalf("no-key must select MinIO defaults: %+v", got)
	}
	if got.Bucket != defaultBucket || !got.ForcePathStyle {
		t.Fatalf("minio bucket/path-style: %+v", got)
	}
	if sourceOf(got) != SourceMinIO {
		t.Fatalf("source: %s", sourceOf(got))
	}
}

func TestResolveSettingsProductionNoKeyStaysUnavailable(t *testing.T) {
	got := ResolveSettings(Settings{Production: true})
	if got.AccessKey != "" || got.SecretKey != "" {
		t.Fatalf("production must not invent minioadmin: %+v", got)
	}
	store := NewStore(got)
	if err := store.Put("brand/x/logo.png", "image/png", []byte("x")); !errors.Is(err, ErrStoreUnavailable) {
		t.Fatalf("production no-key put: %v", err)
	}
	st := store.Status(context.Background())
	if st.OK || st.Label != LabelUnavailable || st.Source != SourceUnavailable {
		t.Fatalf("production status: %+v", st)
	}
	assertNoLocalMasquerade(t)
}

func TestStoreMissingBucketNeverWritesLocal(t *testing.T) {
	api := &fakeS3{putErr: &s3types.NoSuchBucket{}, headErr: &s3types.NotFound{}}
	store := newTestStore(api, fakePresign{err: errors.New("no bucket")}, "missing-bucket")
	root := t.TempDir()
	t.Setenv("TOKENHUB_MEDIA_STORE_PATH", root)

	err := store.Put("jobs/vid_1/output.bin", "video/mp4", []byte("should-not-land"))
	if !errors.Is(err, ErrStoreUnavailable) {
		t.Fatalf("missing bucket put: %v", err)
	}
	if api.puts != 1 {
		t.Fatalf("put must attempt S3 once, got %d", api.puts)
	}
	if _, err := store.Read("jobs/vid_1/output.bin"); !errors.Is(err, ErrStoreUnavailable) {
		t.Fatalf("missing bucket read: %v", err)
	}
	if _, _, err := store.Sign("jobs/vid_1/output.bin", time.Minute); !errors.Is(err, ErrStoreUnavailable) {
		t.Fatalf("missing bucket sign: %v", err)
	}
	st := store.Status(context.Background())
	if st.OK || st.Label != LabelUnavailable {
		t.Fatalf("missing bucket status: %+v", st)
	}
	if st.Detail != "missing bucket" {
		t.Fatalf("status detail: %+v", st)
	}
	assertDirEmpty(t, root)
	assertNoLocalMasquerade(t)
}

func TestStoreFailureNeverWritesLocalOrReportsOK(t *testing.T) {
	api := &fakeS3{putErr: errors.New("connection refused"), headErr: errors.New("connection refused")}
	store := newTestStore(api, fakePresign{err: errors.New("down")}, "tokenhub")
	root := t.TempDir()

	err := store.Put("oem/logo.png", "image/png", []byte("logo"))
	if !errors.Is(err, ErrStoreUnavailable) {
		t.Fatalf("failure put: %v", err)
	}
	entries, _ := os.ReadDir(root)
	if len(entries) != 0 {
		t.Fatalf("failed put wrote local files: %v", names(entries))
	}
	st := store.Status(context.Background())
	if st.OK || st.Label != LabelUnavailable || strings.Contains(st.Label, "✓") {
		t.Fatalf("failure must not look like success: %+v", st)
	}
	assertNoLocalMasquerade(t)
}

func TestStorePutGetDeleteAndPresign(t *testing.T) {
	api := &fakeS3{}
	store := newTestStore(api, fakePresign{url: "http://127.0.0.1:9000/tokenhub/jobs/a.bin?X-Amz-Algorithm=AWS4-HMAC-SHA256"}, "tokenhub")
	if err := store.Put("jobs/a.bin", "video/mp4", []byte("tokenhub-sandbox-media")); err != nil {
		t.Fatal(err)
	}
	got, err := store.Read("jobs/a.bin")
	if err != nil || string(got) != "tokenhub-sandbox-media" {
		t.Fatalf("read: %s %v", got, err)
	}
	url, exp, err := store.Sign("jobs/a.bin", time.Minute)
	if err != nil || !strings.Contains(url, "X-Amz-") || exp.Before(time.Now()) {
		t.Fatalf("presign: %s %v %v", url, exp, err)
	}
	if err := store.Delete("jobs/a.bin"); err != nil {
		t.Fatal(err)
	}
	st := store.Status(context.Background())
	if !st.OK || st.Label != LabelS3 || st.Source != SourceMinIO {
		t.Fatalf("ok status: %+v", st)
	}
}

func TestStoreMinIOPathWithoutCloudKeys(t *testing.T) {
	store := NewStore(Settings{SignKey: "k"})
	if store.source != SourceMinIO || store.disabled {
		t.Fatalf("no-key store should be live MinIO client: source=%s disabled=%v", store.source, store.disabled)
	}
	if store.client == nil || store.bucket != defaultBucket {
		t.Fatalf("minio client/bucket: %+v", store)
	}
}

func TestStoreLiveMinIORoundTrip(t *testing.T) {
	store := NewStore(Settings{SignKey: "k"})
	st := store.Status(context.Background())
	if !st.OK {
		t.Skip("live MinIO not available: " + st.Detail)
	}
	key := fmt.Sprintf("ci/%d.bin", time.Now().UnixNano())
	if err := store.Put(key, "text/plain", []byte("minio-ci")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Delete(key) })
	got, err := store.Read(key)
	if err != nil || string(got) != "minio-ci" {
		t.Fatalf("live read: %s %v", got, err)
	}
	url, _, err := store.Sign(key, time.Minute)
	if err != nil || !strings.Contains(url, "X-Amz-") {
		t.Fatalf("live presign: %s %v", url, err)
	}
}

func TestCallbackHMACStillIndependentOfS3(t *testing.T) {
	store := NewStore(Settings{Production: true, SignKey: "cb-key"})
	sig := store.CallbackSign("evt", "job")
	if !store.CallbackValid("evt", "job", sig) || store.CallbackValid("evt", "job", "nope") {
		t.Fatal("callback hmac")
	}
}

func assertDirEmpty(t *testing.T, root string) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return fmt.Errorf("local file %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("store must not write local disk: %v", err)
	}
}

func assertNoLocalMasquerade(t *testing.T) {
	t.Helper()
	for _, root := range LocalWriteProbePaths() {
		if _, err := os.Stat(root); err == nil {
			entries, _ := os.ReadDir(root)
			if len(entries) > 0 {
				t.Fatalf("legacy local store path %s has files after S3 failure: %v", root, names(entries))
			}
		}
	}
}

func names(entries []os.DirEntry) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}
