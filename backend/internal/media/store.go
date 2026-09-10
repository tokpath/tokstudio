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

const (
	SourceS3          = "s3"
	SourceMinIO       = "minio"
	SourceUnavailable = "unavailable"
	LabelS3           = "S3"
	LabelUnavailable  = "存储不可用"
	defaultMinIOHost  = "http://127.0.0.1:9000"
	defaultMinIOUser  = "minioadmin"
	defaultMinIOPass  = "minioadmin"
	defaultBucket     = "tokenhub"
	defaultRegion     = "us-east-1"
	storeOpTimeout    = 15 * time.Second
)

var ErrStoreUnavailable = errors.New("object store unavailable")

// ObjectStore 是媒体与 OEM 品牌资源共用的对象存储。
// 实现必须走 S3 API；失败时返回错误，禁止改写本地盘并伪装成功。
type ObjectStore interface {
	Put(key, contentType string, data []byte) error
	Read(key string) ([]byte, error)
	Delete(key string) error
	Sign(key string, ttl time.Duration) (string, time.Time, error)
	Verify(key, sig string, expUnix int64) bool
	CallbackSign(eventID, jobID string) string
	CallbackValid(eventID, jobID, sig string) bool
	Status(ctx context.Context) Status
}

// Status 是只读「存储源」徽章的事实。ok=false 时文案必须是「存储不可用」。
type Status struct {
	Source string `json:"source"`
	OK     bool   `json:"ok"`
	Label  string `json:"label"`
	Detail string `json:"detail,omitempty"`
}

// Settings 由 TOKENHUB_S3_* / AWS_* 解析。无云 Key 时非生产走 MinIO，生产标不可用。
type Settings struct {
	Endpoint       string
	PublicEndpoint string
	Region         string
	Bucket         string
	AccessKey      string
	SecretKey      string
	ForcePathStyle bool
	Production     bool
	SignKey        string
}

// Store 是 S3 兼容客户端（AWS 或 Compose MinIO）+ 回调 HMAC。
type Store struct {
	client      *s3.Client
	presign     *s3.PresignClient
	bucket      string
	source      string
	signer      hmacSigner
	disabled    bool
	disableWhy  string
	testAPI     objectAPI
	testPresign objectPresigner
}

type hmacSigner struct {
	secret string
}

type objectAPI interface {
	PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	DeleteObject(ctx context.Context, params *s3.DeleteObjectInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectOutput, error)
	HeadBucket(ctx context.Context, params *s3.HeadBucketInput, optFns ...func(*s3.Options)) (*s3.HeadBucketOutput, error)
}

type objectPresigner interface {
	PresignGetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.PresignOptions)) (*v4Presigned, error)
}

// v4Presigned 只取 URL，避免测试绑死 aws 预签名结构。
type v4Presigned struct {
	URL string
}

type sdkPresigner struct {
	inner *s3.PresignClient
}

func (p sdkPresigner) PresignGetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.PresignOptions)) (*v4Presigned, error) {
	out, err := p.inner.PresignGetObject(ctx, params, optFns...)
	if err != nil {
		return nil, err
	}
	return &v4Presigned{URL: out.URL}, nil
}

type liveAPI struct {
	inner *s3.Client
}

func (a liveAPI) PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	return a.inner.PutObject(ctx, params, optFns...)
}
func (a liveAPI) GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	return a.inner.GetObject(ctx, params, optFns...)
}
func (a liveAPI) DeleteObject(ctx context.Context, params *s3.DeleteObjectInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectOutput, error) {
	return a.inner.DeleteObject(ctx, params, optFns...)
}
func (a liveAPI) HeadBucket(ctx context.Context, params *s3.HeadBucketInput, optFns ...func(*s3.Options)) (*s3.HeadBucketOutput, error) {
	return a.inner.HeadBucket(ctx, params, optFns...)
}

var (
	_ ObjectStore = (*Store)(nil)
)

// ResolveSettings 无云 Key 时：非生产默认 MinIO；生产不得假装本地盘成功。
func ResolveSettings(in Settings) Settings {
	in.Endpoint = strings.TrimRight(strings.TrimSpace(in.Endpoint), "/")
	in.PublicEndpoint = strings.TrimRight(strings.TrimSpace(in.PublicEndpoint), "/")
	in.Region = strings.TrimSpace(in.Region)
	in.Bucket = strings.TrimSpace(in.Bucket)
	in.AccessKey = strings.TrimSpace(in.AccessKey)
	in.SecretKey = strings.TrimSpace(in.SecretKey)
	if in.Region == "" {
		in.Region = defaultRegion
	}
	if in.Bucket == "" {
		in.Bucket = defaultBucket
	}
	if in.SignKey == "" {
		in.SignKey = "dev-media-sign-key"
	}
	hasKeys := in.AccessKey != "" && in.SecretKey != ""
	if !hasKeys {
		if in.Production {
			return in
		}
		in.AccessKey = defaultMinIOUser
		in.SecretKey = defaultMinIOPass
		if in.Endpoint == "" {
			in.Endpoint = defaultMinIOHost
		}
		in.ForcePathStyle = true
	}
	if in.Endpoint != "" {
		in.ForcePathStyle = true
	}
	return in
}

func sourceOf(in Settings) string {
	if in.AccessKey == "" || in.SecretKey == "" {
		return SourceUnavailable
	}
	if in.Endpoint == "" {
		return SourceS3
	}
	host := strings.ToLower(in.Endpoint)
	if strings.Contains(host, "minio") || strings.Contains(host, "127.0.0.1:9000") || strings.Contains(host, "localhost:9000") {
		return SourceMinIO
	}
	return SourceS3
}

// NewStore 构造 S3 兼容 Store。不会 MkdirAll 本地目录，也不会在失败时改写磁盘。
func NewStore(in Settings) *Store {
	in = ResolveSettings(in)
	store := &Store{
		bucket: in.Bucket,
		source: sourceOf(in),
		signer: hmacSigner{secret: in.SignKey},
	}
	if in.AccessKey == "" || in.SecretKey == "" {
		store.disabled = true
		store.disableWhy = "missing object-store credentials"
		store.source = SourceUnavailable
		return store
	}
	awsCfg := aws.Config{
		Region:      in.Region,
		Credentials: credentials.NewStaticCredentialsProvider(in.AccessKey, in.SecretKey, ""),
	}
	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if in.Endpoint != "" {
			o.BaseEndpoint = aws.String(in.Endpoint)
		}
		o.UsePathStyle = in.ForcePathStyle || in.Endpoint != ""
	})
	store.client = client
	signClient := client
	if in.PublicEndpoint != "" && in.PublicEndpoint != in.Endpoint {
		signClient = s3.NewFromConfig(awsCfg, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(in.PublicEndpoint)
			o.UsePathStyle = true
		})
	}
	store.presign = s3.NewPresignClient(signClient)
	return store
}

func (s *Store) api() objectAPI {
	if s == nil {
		return nil
	}
	if s.testAPI != nil {
		return s.testAPI
	}
	if s.client == nil {
		return nil
	}
	return liveAPI{inner: s.client}
}

func (s *Store) signerAPI() objectPresigner {
	if s == nil {
		return nil
	}
	if s.testPresign != nil {
		return s.testPresign
	}
	if s.presign == nil {
		return nil
	}
	return sdkPresigner{inner: s.presign}
}

func (s *Store) Put(key, contentType string, data []byte) error {
	if err := s.ready(); err != nil {
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
	if err := s.ready(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), storeOpTimeout)
	defer cancel()
	out, err := s.api().GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrStoreUnavailable, err)
	}
	defer out.Body.Close()
	return io.ReadAll(out.Body)
}

func (s *Store) Delete(key string) error {
	if err := s.ready(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), storeOpTimeout)
	defer cancel()
	_, err := s.api().DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("%w: %v", ErrStoreUnavailable, err)
	}
	return nil
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
