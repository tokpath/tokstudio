// Package outbox 实现 PostgreSQL Outbox。业务模块只能依赖 Publisher 接口。
package outbox

import (
	"context"
	"embed"
	"encoding/json"
	"io/fs"
	"time"

	"gorm.io/gorm"

	"github.com/tokpath/tokstudio/backend/internal/platform/id"
)

//go:embed migrate/*.sql
var migrationFS embed.FS

// Publisher 是跨模块唯一允许的写入入口。
type Publisher interface {
	Enqueue(ctx context.Context, eventType, aggregateType, aggregateID string, payload any) (string, error)
}

type eventRow struct {
	ID            string     `gorm:"column:id;primaryKey"`
	EventType     string     `gorm:"column:event_type"`
	AggregateType string     `gorm:"column:aggregate_type"`
	AggregateID   string     `gorm:"column:aggregate_id"`
	PayloadJSON   []byte     `gorm:"column:payload_json"`
	Status        string     `gorm:"column:status"`
	Attempts      int        `gorm:"column:attempts"`
	AvailableAt   time.Time  `gorm:"column:available_at"`
	PublishedAt   *time.Time `gorm:"column:published_at"`
	LastError     *string    `gorm:"column:last_error"`
	CreatedAt     time.Time  `gorm:"column:created_at"`
}

func (eventRow) TableName() string { return "outbox_events" }

// Service 提供入队和查询。
type Service struct {
	db *gorm.DB
}

func New(db *gorm.DB) *Service {
	return &Service{db: db}
}

func Migrations() (string, fs.FS) {
	sub, err := fs.Sub(migrationFS, "migrate")
	if err != nil {
		panic(err)
	}
	return "outbox", sub
}

// Enqueue 写入一条 pending 事件。账务模块后续应把该调用放进同一业务事务。
func (s *Service) Enqueue(ctx context.Context, eventType, aggregateType, aggregateID string, payload any) (string, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	row := eventRow{
		ID:            id.New("evt"),
		EventType:     eventType,
		AggregateType: aggregateType,
		AggregateID:   aggregateID,
		PayloadJSON:   body,
		Status:        "pending",
		AvailableAt:   time.Now().UTC(),
		CreatedAt:     time.Now().UTC(),
	}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return "", err
	}
	return row.ID, nil
}

// Stats 供健康检查和验收脚本使用。
type Stats struct {
	Pending   int64 `json:"pending"`
	Published int64 `json:"published"`
	Failed    int64 `json:"failed"`
}

func (s *Service) Stats(ctx context.Context) (Stats, error) {
	var stats Stats
	err := s.db.WithContext(ctx).Model(&eventRow{}).
		Select(`COUNT(*) FILTER (WHERE status = 'pending') AS pending,
		        COUNT(*) FILTER (WHERE status = 'published') AS published,
		        COUNT(*) FILTER (WHERE status = 'failed') AS failed`).
		Scan(&stats).Error
	return stats, err
}
