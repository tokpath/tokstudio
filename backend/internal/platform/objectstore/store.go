// Package objectstore provides private S3-compatible storage. Callers keep
// authorization and retention records in Postgres; objects are never public.
package objectstore

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

const MaxObjectBytes = 64 << 20

var ErrUnavailable = errors.New("private object storage unavailable")
var ErrKey = errors.New("invalid private object key")
var ErrSize = errors.New("private object exceeds size limit")

type Settings struct {
	Endpoint, Region, Bucket, AccessKey, SecretKey string
	ForcePathStyle                                 bool
}
type Store struct {
	client *s3.Client
	bucket string
}
type BlobStore interface {
	Put(context.Context, string, []byte, string, time.Time) error
	Get(context.Context, string) ([]byte, error)
	Delete(context.Context, string) error
}

func New(ctx context.Context, in Settings) (*Store, error) {
	if strings.TrimSpace(in.Bucket) == "" || strings.TrimSpace(in.Region) == "" || (in.AccessKey == "") != (in.SecretKey == "") {
		return nil, ErrUnavailable
	}
	if in.Endpoint != "" {
		u, e := url.Parse(in.Endpoint)
		if e != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return nil, ErrUnavailable
		}
	}
	options := []func(*awsconfig.LoadOptions) error{awsconfig.WithRegion(in.Region)}
	if in.AccessKey != "" {
		options = append(options, awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(in.AccessKey, in.SecretKey, "")))
	}
	cfg, e := awsconfig.LoadDefaultConfig(ctx, options...)
	if e != nil {
		return nil, fmt.Errorf("%w: configuration", ErrUnavailable)
	}
	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.UsePathStyle = in.ForcePathStyle
		if in.Endpoint != "" {
			o.BaseEndpoint = aws.String(in.Endpoint)
		}
	})
	return &Store{client: client, bucket: in.Bucket}, nil
}
func validKey(key string) bool {
	if key == "" || len(key) > 512 || path.Clean(key) != key || strings.HasPrefix(key, "/") || strings.ContainsAny(key, "\\\r\n\x00") {
		return false
	}
	for _, prefix := range []string{"temporary-results/", "branding/", "finance-evidence/", "invoices/", "exports/"} {
		if strings.HasPrefix(key, prefix) && len(key) > len(prefix) {
			return true
		}
	}
	return false
}
func timeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, 20*time.Second)
}
func (s *Store) Put(ctx context.Context, key string, data []byte, contentType string, expiry time.Time) error {
	if !validKey(key) {
		return ErrKey
	}
	if len(data) > MaxObjectBytes {
		return ErrSize
	}
	if s == nil || s.client == nil {
		return ErrUnavailable
	}
	if strings.HasPrefix(key, "temporary-results/") && (expiry.IsZero() || !expiry.After(time.Now())) {
		return ErrKey
	}
	ctx, cancel := timeout(ctx)
	defer cancel()
	metadata := map[string]string{}
	if !expiry.IsZero() {
		metadata["retention-until"] = expiry.UTC().Format(time.RFC3339Nano)
	}
	_, e := s.client.PutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key), Body: bytes.NewReader(data), ContentType: aws.String(contentType), Metadata: metadata, CacheControl: aws.String("private, no-store")})
	if e != nil {
		return fmt.Errorf("%w: write failed", ErrUnavailable)
	}
	return nil
}
func (s *Store) Get(ctx context.Context, key string) ([]byte, error) {
	if !validKey(key) {
		return nil, ErrKey
	}
	if s == nil || s.client == nil {
		return nil, ErrUnavailable
	}
	ctx, cancel := timeout(ctx)
	defer cancel()
	out, e := s.client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key)})
	if e != nil {
		return nil, fmt.Errorf("%w: read failed", ErrUnavailable)
	}
	defer out.Body.Close()
	if expiry := out.Metadata["retention-until"]; expiry != "" {
		at, e := time.Parse(time.RFC3339Nano, expiry)
		if e != nil || !at.After(time.Now()) {
			return nil, ErrUnavailable
		}
	}
	data, e := io.ReadAll(io.LimitReader(out.Body, MaxObjectBytes+1))
	if e != nil {
		return nil, ErrUnavailable
	}
	if len(data) > MaxObjectBytes {
		return nil, ErrSize
	}
	return data, nil
}
func (s *Store) Delete(ctx context.Context, key string) error {
	if !validKey(key) {
		return ErrKey
	}
	if s == nil || s.client == nil {
		return ErrUnavailable
	}
	ctx, cancel := timeout(ctx)
	defer cancel()
	if _, e := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key)}); e != nil {
		return fmt.Errorf("%w: deletion requires retry", ErrUnavailable)
	}
	return nil
}
func (s *Store) Ready(ctx context.Context) error {
	if s == nil || s.client == nil {
		return ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if _, e := s.client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(s.bucket)}); e != nil {
		return ErrUnavailable
	}
	return nil
}

// ConfigureTemporaryLifecycle is an explicit provisioning operation, not an API
// startup mutation. Postgres-enforced expiry is exact; this rule also collects
// orphaned uploads after a crash. Other prefixes/rules remain untouched.
func (s *Store) ConfigureTemporaryLifecycle(ctx context.Context) error {
	if s == nil || s.client == nil {
		return ErrUnavailable
	}
	ctx, cancel := timeout(ctx)
	defer cancel()
	rules := []types.LifecycleRule{}
	existing, e := s.client.GetBucketLifecycleConfiguration(ctx, &s3.GetBucketLifecycleConfigurationInput{Bucket: aws.String(s.bucket)})
	if e != nil {
		var api smithy.APIError
		if !errors.As(e, &api) || api.ErrorCode() != "NoSuchLifecycleConfiguration" {
			return fmt.Errorf("%w: lifecycle read failed", ErrUnavailable)
		}
	} else {
		for _, r := range existing.Rules {
			if aws.ToString(r.ID) != "tokenhub-temporary-results" {
				rules = append(rules, r)
			}
		}
	}
	rules = append(rules, types.LifecycleRule{ID: aws.String("tokenhub-temporary-results"), Status: types.ExpirationStatusEnabled, Filter: &types.LifecycleRuleFilter{Prefix: aws.String("temporary-results/")}, Expiration: &types.LifecycleExpiration{Days: aws.Int32(2)}, NoncurrentVersionExpiration: &types.NoncurrentVersionExpiration{NoncurrentDays: aws.Int32(2)}, AbortIncompleteMultipartUpload: &types.AbortIncompleteMultipartUpload{DaysAfterInitiation: aws.Int32(1)}})
	_, e = s.client.PutBucketLifecycleConfiguration(ctx, &s3.PutBucketLifecycleConfigurationInput{Bucket: aws.String(s.bucket), LifecycleConfiguration: &types.BucketLifecycleConfiguration{Rules: rules}})
	if e != nil {
		return fmt.Errorf("%w: lifecycle configuration failed", ErrUnavailable)
	}
	return nil
}
