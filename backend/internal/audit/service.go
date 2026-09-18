// Package audit 提供不可删除的审计账本。其他模块只调用 Recorder。
package audit

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"io/fs"
	"time"

	"gorm.io/gorm"

	"github.com/tokpath/tokstudio/backend/internal/outbox"
	"github.com/tokpath/tokstudio/backend/internal/platform/id"
)

//go:embed migrate/*.sql
var migrationFS embed.FS

type logRow struct {
	ID           string          `gorm:"column:id;primaryKey"`
	ActorUserID  *string         `gorm:"column:actor_user_id"`
	Action       string          `gorm:"column:action"`
	ResourceType string          `gorm:"column:resource_type"`
	ResourceID   *string         `gorm:"column:resource_id"`
	BeforeJSON   json.RawMessage `gorm:"column:before_json"`
	AfterJSON    json.RawMessage `gorm:"column:after_json"`
	IP           *string         `gorm:"column:ip"`
	RequestID    *string         `gorm:"column:request_id"`
	CreatedAt    time.Time       `gorm:"column:created_at"`
}

func (logRow) TableName() string { return "audit_logs" }

// RecordInput 是审计模块的公开写入契约。
type RecordInput struct {
	ActorUserID  string
	Action       string
	ResourceType string
	ResourceID   string
	Before       any
	After        any
	IP           string
	RequestID    string
}

// Entry 是查询返回的公开视图。
type Entry struct {
	ID           string          `json:"id"`
	ActorUserID  string          `json:"actor_user_id,omitempty"`
	Action       string          `json:"action"`
	ResourceType string          `json:"resource_type"`
	ResourceID   string          `json:"resource_id,omitempty"`
	Before       json.RawMessage `json:"before,omitempty"`
	After        json.RawMessage `json:"after,omitempty"`
	IP           string          `json:"ip,omitempty"`
	RequestID    string          `json:"request_id,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
}

type Service struct {
	db     *gorm.DB
	events outbox.Publisher
}

func New(db *gorm.DB, events outbox.Publisher) *Service {
	return &Service{db: db, events: events}
}

func Migrations() (string, fs.FS) {
	sub, err := fs.Sub(migrationFS, "migrate")
	if err != nil {
		panic(err)
	}
	return "audit", sub
}

func (s *Service) Record(ctx context.Context, in RecordInput) (*Entry, error) {
	row := logRow{
		ID:           id.New("aud"),
		Action:       in.Action,
		ResourceType: in.ResourceType,
		CreatedAt:    time.Now().UTC(),
	}
	if in.ActorUserID != "" {
		row.ActorUserID = &in.ActorUserID
	}
	if in.ResourceID != "" {
		row.ResourceID = &in.ResourceID
	}
	if in.IP != "" {
		row.IP = &in.IP
	}
	if in.RequestID != "" {
		row.RequestID = &in.RequestID
	}
	if in.Before != nil {
		body, err := json.Marshal(in.Before)
		if err != nil {
			return nil, err
		}
		row.BeforeJSON = body
	}
	if in.After != nil {
		body, err := json.Marshal(in.After)
		if err != nil {
			return nil, err
		}
		row.AfterJSON = body
	}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return nil, err
	}
	entry := toEntry(row)
	if s.events != nil {
		_, _ = s.events.Enqueue(ctx, "tokenhub.audit.recorded.v1", "audit_log", row.ID, entry)
	}
	return &entry, nil
}

type SearchQuery struct {
	Action       string
	ResourceType string
	ResourceID   string
	ActorUserID  string
	RequestID    string
	Query        string
	Limit        int
}

func (s *Service) Search(ctx context.Context, q SearchQuery) ([]Entry, error) {
	limit := q.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	dbq := s.db.WithContext(ctx).Model(&logRow{}).Order("created_at DESC").Limit(limit)
	if q.Action != "" {
		dbq = dbq.Where("action = ?", q.Action)
	}
	if q.ResourceType != "" {
		dbq = dbq.Where("resource_type = ?", q.ResourceType)
	}
	if q.ResourceID != "" {
		dbq = dbq.Where("resource_id = ?", q.ResourceID)
	}
	if q.ActorUserID != "" {
		dbq = dbq.Where("actor_user_id = ?", q.ActorUserID)
	}
	if q.RequestID != "" {
		dbq = dbq.Where("request_id = ?", q.RequestID)
	}
	if q.Query != "" {
		like := "%" + q.Query + "%"
		dbq = dbq.Where("action ILIKE ? OR resource_type ILIKE ? OR COALESCE(resource_id,'') ILIKE ?", like, like, like)
	}
	var rows []logRow
	if err := dbq.Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]Entry, 0, len(rows))
	for _, row := range rows {
		out = append(out, toEntry(row))
	}
	return out, nil
}

func toEntry(row logRow) Entry {
	entry := Entry{
		ID:           row.ID,
		Action:       row.Action,
		ResourceType: row.ResourceType,
		Before:       row.BeforeJSON,
		After:        row.AfterJSON,
		CreatedAt:    row.CreatedAt,
	}
	if row.ActorUserID != nil {
		entry.ActorUserID = *row.ActorUserID
	}
	if row.ResourceID != nil {
		entry.ResourceID = *row.ResourceID
	}
	if row.IP != nil {
		entry.IP = *row.IP
	}
	if row.RequestID != nil {
		entry.RequestID = *row.RequestID
	}
	return entry
}

// RecordTx persists audit and its outbox event in the caller's transaction.
func (s *Service) RecordTx(tx *gorm.DB, in RecordInput) (*Entry, error) {
	scoped := *s
	scoped.db = tx
	scoped.events = nil
	entry, err := scoped.Record(tx.Statement.Context, in)
	if err != nil {
		return nil, err
	}
	if s.events != nil {
		publisher, ok := s.events.(interface {
			EnqueueTx(*gorm.DB, string, string, string, any) (string, error)
		})
		if !ok {
			return nil, errors.New("transactional audit publisher required")
		}
		if _, err := publisher.EnqueueTx(tx, "tokenhub.audit.recorded.v1", "audit_log", entry.ID, entry); err != nil {
			return nil, err
		}
	}
	return entry, nil
}
