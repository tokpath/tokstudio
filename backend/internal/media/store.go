package media

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// Store 是受控对象存储。未配置 S3 时用本地目录；配齐后对象进 S3/MinIO。
// 下载仍走 HMAC 签名的 /v1/media/objects，由 API 从后端读出，浏览器不必直连桶。
type Store struct {
	Root   string
	Secret string
	Public string
	blob   blobStore
}

type StoreOptions struct {
	Root   string
	Secret string
	Public string
	S3     S3Options
}

func NewStore(root, secret, publicBase string) *Store {
	store, _ := OpenStore(StoreOptions{Root: root, Secret: secret, Public: publicBase})
	return store
}

func OpenStore(opts StoreOptions) (*Store, error) {
	root := opts.Root
	if root == "" {
		root = filepath.Join(os.TempDir(), "tokenhub-media")
	}
	secret := opts.Secret
	if secret == "" {
		secret = "dev-media-sign-key"
	}
	store := &Store{Root: root, Secret: secret, Public: strings.TrimRight(opts.Public, "/")}
	if opts.S3.Ready() {
		blob, err := newS3Blob(opts.S3)
		if err != nil {
			return nil, err
		}
		store.blob = blob
		return store, nil
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *Store) UsingS3() bool {
	return s != nil && s.blob != nil
}

func (s *Store) Put(key, contentType string, data []byte) error {
	if s.blob != nil {
		return s.blob.Put(key, contentType, data)
	}
	_ = contentType
	path := filepath.Join(s.Root, filepath.FromSlash(key))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if strings.TrimSpace(key) == "" {
		return fmt.Errorf("%w: empty object key", ErrStoreUnavailable)
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	ctx, cancel := context.WithTimeout(context.Background(), storeOpTimeout)
	defer cancel()
	_, err := s.api().PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(data),
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return fmt.Errorf("%w: %v", ErrStoreUnavailable, err)
	}
	return nil
}

func (s *Store) Read(key string) ([]byte, error) {
	if s.blob != nil {
		return s.blob.Read(key)
	}
	return os.ReadFile(filepath.Join(s.Root, filepath.FromSlash(key)))
}

func (s *Store) Delete(key string) error {
	if s.blob != nil {
		return s.blob.Delete(key)
	}
	return os.Remove(filepath.Join(s.Root, filepath.FromSlash(key)))
}

func (s *Store) Sign(key string, ttl time.Duration) (string, time.Time, error) {
	if err := s.ready(); err != nil {
		return "", time.Time{}, err
	}
	if ttl <= 0 {
		ttl = SignTTL
	}
	exp := time.Now().UTC().Add(ttl)
	ctx, cancel := context.WithTimeout(context.Background(), storeOpTimeout)
	defer cancel()
	out, err := s.signerAPI().PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}, func(o *s3.PresignOptions) {
		o.Expires = ttl
	})
	if err != nil {
		return "", time.Time{}, fmt.Errorf("%w: %v", ErrStoreUnavailable, err)
	}
	if out == nil || out.URL == "" {
		return "", time.Time{}, fmt.Errorf("%w: empty presigned url", ErrStoreUnavailable)
	}
	return out.URL, exp, nil
}

func (s *Store) Verify(key, sig string, expUnix int64) bool {
	return s.signer.Verify(key, sig, expUnix)
}

func (s *Store) CallbackSign(eventID, jobID string) string {
	return s.signer.Sign(eventID + "|" + jobID)
}

func (s *Store) CallbackValid(eventID, jobID, sig string) bool {
	return hmac.Equal([]byte(s.CallbackSign(eventID, jobID)), []byte(sig))
}

func (s *Store) Status(ctx context.Context) Status {
	if s == nil {
		return unavailableStatus("store not configured")
	}
	if err := s.ready(); err != nil {
		return unavailableStatus(s.disableWhy)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	probe, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	_, err := s.api().HeadBucket(probe, &s3.HeadBucketInput{Bucket: aws.String(s.bucket)})
	if err != nil {
		detail := "store failure"
		if isMissingBucket(err) {
			detail = "missing bucket"
		}
		return unavailableStatus(detail)
	}
	label := LabelS3
	return Status{Source: s.source, OK: true, Label: label}
}

func (s *Store) ready() error {
	if s == nil || s.disabled || s.api() == nil || s.bucket == "" {
		why := "object store unavailable"
		if s != nil && s.disableWhy != "" {
			why = s.disableWhy
		}
		return fmt.Errorf("%w: %s", ErrStoreUnavailable, why)
	}
	return nil
}

func newTestStore(api objectAPI, presign objectPresigner, bucket string) *Store {
	return &Store{
		bucket:      bucket,
		source:      SourceMinIO,
		signer:      hmacSigner{secret: "test-media-sign-key"},
		testAPI:     api,
		testPresign: presign,
	}
}

func unavailableStatus(detail string) Status {
	return Status{Source: SourceUnavailable, OK: false, Label: LabelUnavailable, Detail: detail}
}

func isMissingBucket(err error) bool {
	if err == nil {
		return false
	}
	var notFound *s3types.NotFound
	var noBucket *s3types.NoSuchBucket
	if errors.As(err, &notFound) || errors.As(err, &noBucket) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "nosuchbucket") || strings.Contains(msg, "not found") || strings.Contains(msg, "404")
}

func (h hmacSigner) Sign(payload string) string {
	mac := hmac.New(sha256.New, []byte(h.secret))
	_, _ = mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}

func (h hmacSigner) Verify(key, sig string, expUnix int64) bool {
	if time.Now().UTC().Unix() > expUnix {
		return false
	}
	want := h.Sign(key + "|" + strconv.FormatInt(expUnix, 10))
	return hmac.Equal([]byte(want), []byte(sig))
}

// LocalWriteProbe 给测试用：S3 实现不得在这些路径落盘。
func LocalWriteProbePaths() []string {
	return []string{
		"/tmp/tokenhub-media",
		"/tmp/tokenhub-media-s3",
	}
}
