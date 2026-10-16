// Package jobs implements at-least-once durable business work. A lease only
// fences database completion; external side effects still require their original
// business idempotency key and an intent/query protocol for unknown outcomes.
package jobs

import (
	"bytes"
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/tokpath/tokstudio/backend/internal/platform/id"
)

//go:embed migrate/*.sql
var migrationFS embed.FS

var (
	ErrLeaseLost   = errors.New("job lease expired or fencing token no longer current")
	ErrKeyConflict = errors.New("job business key already has different immutable input")
)

const (
	DefaultMaxAttempts = 8
	MaxAttempts        = 100
	MaxBatch           = 100
)

type Job struct {
	ID                       string `gorm:"primaryKey"`
	Queue, Kind, BusinessKey string
	Payload                  json.RawMessage `gorm:"type:jsonb"`
	PayloadHash              string
	Status                   string
	Attempts, MaxAttempts    int
	AvailableAt              time.Time
	LeaseUntil               *time.Time
	LeaseOwner               string
	Fence                    int64
	LastError                string
	CreatedAt, UpdatedAt     time.Time
	CompletedAt              *time.Time
}

func (Job) TableName() string { return "durable_jobs" }

// Input's queue, kind and business key identify one original business action.
// Re-enqueue returns that action even after completion; it never resets attempts.
// Payload should contain IDs/object references, not long-lived request content.
type Input struct {
	Queue, Kind, BusinessKey string
	Payload                  json.RawMessage
	AvailableAt              time.Time
	MaxAttempts              int
}

type Service struct{ db *gorm.DB }

func New(gdb *gorm.DB) *Service { return &Service{db: gdb} }
func Migrations() (string, fs.FS) {
	sub, err := fs.Sub(migrationFS, "migrate")
	if err != nil {
		panic(err)
	}
	return "jobs", sub
}

func prepare(input Input) (Job, error) {
	if input.Queue == "" {
		input.Queue = "default"
	}
	if strings.TrimSpace(input.Queue) == "" || strings.TrimSpace(input.Kind) == "" || strings.TrimSpace(input.BusinessKey) == "" {
		return Job{}, errors.New("job queue, kind and business key are required")
	}
	if input.MaxAttempts == 0 {
		input.MaxAttempts = DefaultMaxAttempts
	}
	if input.MaxAttempts < 1 || input.MaxAttempts > MaxAttempts {
		return Job{}, fmt.Errorf("job max attempts must be 1..%d", MaxAttempts)
	}
	if len(input.Payload) == 0 {
		input.Payload = json.RawMessage(`{}`)
	}
	decoder := json.NewDecoder(bytes.NewReader(input.Payload))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return Job{}, fmt.Errorf("job payload: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return Job{}, errors.New("job payload must contain exactly one JSON value")
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return Job{}, err
	}
	digest := sha256.Sum256(payload)
	return Job{ID: id.New("job"), Queue: input.Queue, Kind: input.Kind, BusinessKey: input.BusinessKey, Payload: payload, PayloadHash: hex.EncodeToString(digest[:]), Status: "pending", MaxAttempts: input.MaxAttempts, AvailableAt: input.AvailableAt.UTC()}, nil
}

func (s *Service) Enqueue(ctx context.Context, input Input) (Job, error) {
	return s.EnqueueTx(s.db.WithContext(ctx), input)
}

// EnqueueTx participates in the business transaction, so work is not lost in the
// gap between committing an order and publishing a Redis notification.
func (s *Service) EnqueueTx(tx *gorm.DB, input Input) (Job, error) {
	job, err := prepare(input)
	if err != nil {
		return Job{}, err
	}
	query := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "queue"}, {Name: "kind"}, {Name: "business_key"}}, DoNothing: true})
	omitted := []string{"CreatedAt", "UpdatedAt"}
	if input.AvailableAt.IsZero() {
		omitted = append(omitted, "AvailableAt")
	}
	if err := query.Omit(omitted...).Create(&job).Error; err != nil {
		return Job{}, err
	}
	var original Job
	if err := tx.Where("queue = ? AND kind = ? AND business_key = ?", job.Queue, job.Kind, job.BusinessKey).First(&original).Error; err != nil {
		return Job{}, err
	}
	if original.PayloadHash != job.PayloadHash || original.MaxAttempts != job.MaxAttempts {
		return Job{}, ErrKeyConflict
	}
	return original, nil
}

func validateLease(lease time.Duration) error {
	if lease < time.Second || lease > 24*time.Hour {
		return errors.New("lease must be between one second and 24 hours")
	}
	return nil
}

// Claim atomically takes due work with SKIP LOCKED. Expired work increments its
// fencing token on every new claim; a crashed final attempt becomes a dead letter.
func (s *Service) Claim(ctx context.Context, queue, owner string, lease time.Duration, limit int) ([]Job, error) {
	if queue == "" {
		queue = "default"
	}
	if strings.TrimSpace(owner) == "" {
		return nil, errors.New("lease owner is required")
	}
	if err := validateLease(lease); err != nil {
		return nil, err
	}
	if limit < 1 || limit > MaxBatch {
		return nil, fmt.Errorf("claim limit must be 1..%d", MaxBatch)
	}
	var result []Job
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Lock only expired rows that are not held by another claimant.
		if err := tx.Exec(`WITH exhausted AS (
			SELECT id FROM durable_jobs WHERE queue = ? AND status = 'running'
			AND lease_until <= clock_timestamp() AND attempts >= max_attempts
			FOR UPDATE SKIP LOCKED)
			UPDATE durable_jobs j SET status='dead',lease_until=NULL,lease_owner='',
			last_error='lease expired on final attempt',updated_at=clock_timestamp()
			FROM exhausted e WHERE j.id=e.id`, queue).Error; err != nil {
			return err
		}
		return tx.Raw(`WITH candidates AS (
			SELECT id FROM durable_jobs WHERE queue = ? AND attempts < max_attempts
			AND ((status='pending' AND available_at <= clock_timestamp())
			OR (status='running' AND lease_until <= clock_timestamp()))
			ORDER BY available_at,created_at,id FOR UPDATE SKIP LOCKED LIMIT ?)
			UPDATE durable_jobs j SET status='running',attempts=j.attempts+1,fence=j.fence+1,
			lease_owner=?,lease_until=clock_timestamp() + (? * interval '1 microsecond'),updated_at=clock_timestamp()
			FROM candidates c WHERE j.id=c.id RETURNING j.*`, queue, limit, owner, lease.Microseconds()).Scan(&result).Error
	})
	return result, err
}

func leaseQuery(tx *gorm.DB, jobID, owner string, fence int64) *gorm.DB {
	return tx.Model(&Job{}).Where("id=? AND status='running' AND lease_owner=? AND fence=? AND lease_until > clock_timestamp()", jobID, owner, fence)
}
func guarded(result *gorm.DB) error {
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrLeaseLost
	}
	return nil
}

func (s *Service) Complete(ctx context.Context, jobID, owner string, fence int64) error {
	return s.CompleteTx(s.db.WithContext(ctx), jobID, owner, fence)
}

// CompleteTx must share the transaction that writes the business result. Return
// its error from that transaction so a stale worker's business writes roll back.
func (s *Service) CompleteTx(tx *gorm.DB, jobID, owner string, fence int64) error {
	return guarded(leaseQuery(tx, jobID, owner, fence).Updates(map[string]any{
		"status": "completed", "lease_until": nil, "lease_owner": "", "last_error": "",
		"completed_at": gorm.Expr("clock_timestamp()"), "updated_at": gorm.Expr("clock_timestamp()"),
	}))
}

func (s *Service) ExtendLease(ctx context.Context, jobID, owner string, fence int64, lease time.Duration) error {
	if err := validateLease(lease); err != nil {
		return err
	}
	return guarded(leaseQuery(s.db.WithContext(ctx), jobID, owner, fence).Updates(map[string]any{
		"lease_until": gorm.Expr("clock_timestamp() + (? * interval '1 microsecond')", lease.Microseconds()),
		"updated_at":  gorm.Expr("clock_timestamp()"),
	}))
}

func retryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if attempt > 10 {
		return 15 * time.Minute
	}
	delay := time.Second * time.Duration(1<<uint(attempt-1))
	if delay > 15*time.Minute {
		return 15 * time.Minute
	}
	return delay
}

// Fail either schedules a bounded retry or keeps the final attempt in the dead
// letter table. Unknown external outcomes must be retried as verification work,
// never interpreted here as permission to repeat an external payment/submission.
func (s *Service) Fail(ctx context.Context, jobID, owner string, fence int64, reason string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var job Job
		if err := leaseQuery(tx, jobID, owner, fence).Clauses(clause.Locking{Strength: "UPDATE"}).First(&job).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrLeaseLost
			}
			return err
		}
		status := "pending"
		if job.Attempts >= job.MaxAttempts {
			status = "dead"
		}
		if len(reason) > 4096 {
			reason = reason[:4096]
		}
		return guarded(leaseQuery(tx, jobID, owner, fence).Updates(map[string]any{
			"status": status, "lease_until": nil, "lease_owner": "", "last_error": reason,
			"available_at": gorm.Expr("clock_timestamp() + (? * interval '1 microsecond')", retryDelay(job.Attempts).Microseconds()),
			"updated_at":   gorm.Expr("clock_timestamp()"),
		}))
	})
}

func (s *Service) DeadLetters(ctx context.Context, queue string, limit int) ([]Job, error) {
	if queue == "" {
		queue = "default"
	}
	if limit < 1 || limit > MaxBatch {
		return nil, fmt.Errorf("dead letter limit must be 1..%d", MaxBatch)
	}
	var result []Job
	err := s.db.WithContext(ctx).Where("queue=? AND status='dead'", queue).Order("updated_at,id").Limit(limit).Find(&result).Error
	return result, err
}
