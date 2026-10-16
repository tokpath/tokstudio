package branding

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"io/fs"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"github.com/tokpath/tokstudio/backend/internal/platform/objectstore"
	"gorm.io/gorm"
)

//go:embed migrate/*.sql
var migrationFS embed.FS

func Migrations() (string, fs.FS) {
	sub, err := fs.Sub(migrationFS, "migrate")
	if err != nil {
		panic(err)
	}
	return "branding", sub
}

type Service struct {
	db        *gorm.DB
	directory Directory
	now       func() time.Time
	assets    objectstore.BlobStore
}

// SetAssetStore is configured once at startup. Objects remain private; callers
// retrieve logo bytes through the scoped methods in this package.
func (s *Service) SetAssetStore(store objectstore.BlobStore) { s.assets = store }

func New(db *gorm.DB) *Service { return NewWithDirectory(db, organization.New(db)) }
func NewWithDirectory(db *gorm.DB, d Directory) *Service {
	return &Service{db: db, directory: d, now: func() time.Time { return time.Now().UTC() }}
}
func (s *Service) dbFor(ctx context.Context, tx *gorm.DB) *gorm.DB {
	if tx == nil {
		tx = s.db
	}
	return tx.WithContext(ctx)
}
func (s *Service) withTx(ctx context.Context, tx *gorm.DB, f func(*gorm.DB) error) error {
	if tx != nil {
		return classify(f(tx.WithContext(ctx)))
	}
	return classify(s.db.WithContext(ctx).Transaction(f))
}
func classify(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	var p *pgconn.PgError
	if errors.As(err, &p) && p.Code == "23505" {
		if p.ConstraintName == "branding_domains_hostname_key" {
			return ErrDomainBound
		}
		return ErrConflict
	}
	return err
}
func validID(id string) bool { _, err := uuid.Parse(id); return err == nil }
func (s *Service) audit(tx *gorm.DB, actor, brand, targetType, targetID, action string, before, after any) error {
	a, err := json.Marshal(before)
	if err != nil {
		return err
	}
	b, err := json.Marshal(after)
	if err != nil {
		return err
	}
	return tx.Exec(`INSERT INTO branding_changes(id,brand_id,actor_id,target_type,target_id,action,before_json,after_json,created_at) VALUES(?,?,?,?,?,?,?::jsonb,?::jsonb,?)`, uuid.NewString(), brand, actor, targetType, targetID, action, string(a), string(b), s.now()).Error
}
