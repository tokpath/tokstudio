package jobs

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"regexp"
)

// View omits payload and payload hashes. Operational access must be authorized
// by the application; this is a trusted system queue, never a customer list.
type View struct {
	ID          string     `json:"id"`
	Queue       string     `json:"queue"`
	Kind        string     `json:"kind"`
	BusinessKey string     `json:"business_key"`
	Status      string     `json:"status"`
	Attempts    int        `json:"attempts"`
	MaxAttempts int        `json:"max_attempts"`
	AvailableAt time.Time  `json:"available_at"`
	LeaseUntil  *time.Time `json:"lease_until"`
	LastError   string     `json:"last_error"`
	CreatedAt   time.Time  `json:"created_at"`
}

func (s *Service) List(ctx context.Context, cursor, queue, status string, limit int) ([]View, error) {
	if limit < 1 || limit > 100 {
		return nil, errors.New("invalid job page limit")
	}
	q := s.db.WithContext(ctx).Table("durable_jobs")
	if cursor != "" {
		if !jobIDPattern.MatchString(cursor) {
			return nil, errors.New("invalid job cursor")
		}
		q = q.Where("id>?", cursor)
	}
	if queue != "" {
		q = q.Where("queue=?", queue)
	}
	if status != "" {
		q = q.Where("status=?", status)
	}
	rows := []View{}
	e := q.Select("id,queue,kind,business_key,status,attempts,max_attempts,available_at,lease_until,last_error,created_at").Order("id").Limit(limit).Find(&rows).Error
	return rows, e
}

// Redrive preserves the original business identity and immutable payload. It
// grants a bounded number of additional attempts and never resets attempt/fence
// history. Callers must record the human reason in an audit transaction.
func (s *Service) Redrive(ctx context.Context, id string, additional int) error {
	return s.RedriveTx(s.db.WithContext(ctx), id, additional)
}
func (s *Service) RedriveTx(tx *gorm.DB, id string, additional int) error {
	if !jobIDPattern.MatchString(id) || additional < 1 || additional > 10 {
		return errors.New("invalid redrive")
	}
	result := tx.Exec(`UPDATE durable_jobs SET status='pending',max_attempts=max_attempts+?,available_at=now(),lease_until=NULL,lease_owner='',fence=fence+1,updated_at=now() WHERE id=? AND status='dead' AND max_attempts+?<=100`, additional, id, additional)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrLeaseLost
	}
	return nil
}
func (s *Service) StatusTx(tx *gorm.DB, id string) (string, error) {
	var result struct{ Status string }
	e := tx.Model(&Job{}).Select("status").Where("id=?", id).Take(&result).Error
	return result.Status, e
}

var jobIDPattern = regexp.MustCompile(`^job_[0-9a-f]{32}$`)
