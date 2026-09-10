package media

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

const maxS3Body = 64 << 20

type blobStore interface {
	Put(key, contentType string, data []byte) error
	Read(key string) ([]byte, error)
	Delete(key string) error
}

// S3Options 是 S3 兼容后端（含 MinIO）的连接参数。
type S3Options struct {
	Endpoint  string
	Bucket    string
	AccessKey string
	SecretKey string
	Region    string
	PathStyle bool
}

func (o S3Options) Ready() bool {
	return strings.TrimSpace(o.Bucket) != "" &&
		strings.TrimSpace(o.AccessKey) != "" &&
		strings.TrimSpace(o.SecretKey) != ""
}

type s3Blob struct {
	client *s3.Client
	bucket string
}

func newS3Blob(opts S3Options) (*s3Blob, error) {
	region := strings.TrimSpace(opts.Region)
	if region == "" {
		region = "us-east-1"
	}
	cfg := aws.Config{
		Region:      region,
		Credentials: credentials.NewStaticCredentialsProvider(strings.TrimSpace(opts.AccessKey), strings.TrimSpace(opts.SecretKey), ""),
	}
	if endpoint := strings.TrimSpace(opts.Endpoint); endpoint != "" {
		cfg.BaseEndpoint = aws.String(endpoint)
	}
	pathStyle := opts.PathStyle
	if strings.TrimSpace(opts.Endpoint) != "" {
		pathStyle = true
	}
	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.UsePathStyle = pathStyle
	})
	blob := &s3Blob{client: client, bucket: strings.TrimSpace(opts.Bucket)}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := blob.ensureBucket(ctx); err != nil {
		return nil, err
	}
	return blob, nil
}

func (b *s3Blob) ensureBucket(ctx context.Context) error {
	_, err := b.client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(b.bucket)})
	if err == nil {
		return nil
	}
	_, err = b.client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(b.bucket)})
	if err == nil {
		return nil
	}
	var owned *types.BucketAlreadyOwnedByYou
	var exists *types.BucketAlreadyExists
	if errors.As(err, &owned) || errors.As(err, &exists) {
		return nil
	}
	return fmt.Errorf("s3 bucket %s: %w", b.bucket, err)
}

func (b *s3Blob) Put(key, contentType string, data []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	in := &s3.PutObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(key),
		Body:   bytes.NewReader(data),
	}
	if strings.TrimSpace(contentType) != "" {
		in.ContentType = aws.String(contentType)
	}
	_, err := b.client.PutObject(ctx, in)
	return err
}

func (b *s3Blob) Read(key string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	out, err := b.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, err
	}
	defer out.Body.Close()
	return io.ReadAll(io.LimitReader(out.Body, maxS3Body))
}

func (b *s3Blob) Delete(key string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, err := b.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(key),
	})
	return err
}
