package objectstore

import (
	"context"
	"io"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// Model outputs and regenerated report exports are deliberately absent from
// long-lived backups. Private invoice/evidence/branding objects are immutable.
func durableKey(key string) bool {
	return validKey(key) && (strings.HasPrefix(key, "branding/") || strings.HasPrefix(key, "invoices/") || strings.HasPrefix(key, "finance-evidence/"))
}
func (s *Store) VisitDurable(ctx context.Context, visit func(key, contentType string, data []byte) error) error {
	if s == nil || s.client == nil {
		return ErrUnavailable
	}
	for _, prefix := range []string{"branding/", "finance-evidence/", "invoices/"} {
		pages := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{Bucket: aws.String(s.bucket), Prefix: aws.String(prefix)})
		for pages.HasMorePages() {
			page, e := pages.NextPage(ctx)
			if e != nil {
				return ErrUnavailable
			}
			for _, item := range page.Contents {
				key := aws.ToString(item.Key)
				if !durableKey(key) {
					return ErrKey
				}
				if aws.ToInt64(item.Size) > MaxObjectBytes {
					return ErrSize
				}
				readCtx, cancel := timeout(ctx)
				obj, e := s.client.GetObject(readCtx, &s3.GetObjectInput{Bucket: aws.String(s.bucket), Key: item.Key})
				if e != nil {
					cancel()
					return ErrUnavailable
				}
				data, e := io.ReadAll(io.LimitReader(obj.Body, MaxObjectBytes+1))
				obj.Body.Close()
				cancel()
				if e != nil {
					return ErrUnavailable
				}
				if len(data) > MaxObjectBytes {
					return ErrSize
				}
				if e = visit(key, aws.ToString(obj.ContentType), data); e != nil {
					return e
				}
			}
		}
	}
	return nil
}
func (s *Store) RequireEmpty(ctx context.Context) error {
	if s == nil || s.client == nil {
		return ErrUnavailable
	}
	ctx, cancel := timeout(ctx)
	defer cancel()
	r, e := s.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{Bucket: aws.String(s.bucket), MaxKeys: aws.Int32(1)})
	if e != nil {
		return ErrUnavailable
	}
	if len(r.Contents) > 0 {
		return ErrKey
	}
	return nil
}
func (s *Store) RestoreDurable(ctx context.Context, key, contentType string, data []byte) error {
	if !durableKey(key) {
		return ErrKey
	}
	return s.Put(ctx, key, data, contentType, time.Time{})
}
