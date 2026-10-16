package usage

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"github.com/tokpath/tokstudio/backend/internal/cooperation"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/modelruntime"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"github.com/tokpath/tokstudio/backend/internal/platform/objectstore"
	"gorm.io/gorm"
)

//go:embed migrate/*.sql
var migrations embed.FS

func Migrations() (string, fs.FS) { f, _ := fs.Sub(migrations, "migrate"); return "usage", f }

var (
	ErrKey               = errors.New("API key is invalid or unavailable")
	ErrBudget            = errors.New("API key budget is exhausted")
	ErrRate              = errors.New("API key request rate is exceeded")
	ErrConcurrency       = errors.New("API key concurrency is exceeded")
	ErrConflict          = errors.New("request or key operation conflicts with its original command")
	ErrInProgress        = errors.New("the original request is already being processed")
	ErrNotReady          = errors.New("model request dependencies are not ready")
	ErrSecretDisplayed   = errors.New("key operation already completed; the full secret is only shown once")
	ErrResultUnavailable = errors.New("the original request completed but its temporary result is unavailable")
)

type GrantObserver interface {
	LockBusiness(context.Context, *gorm.DB, string) error
	ObserveGrantEvent(context.Context, *gorm.DB, ledger.Grant, ledger.GrantEvent) error
}
type Dependencies struct {
	Ledger           *ledger.Service
	Organization     *organization.Service
	Cooperation      *cooperation.Service
	Catalog          catalog.CommerceCatalog
	Runtime          modelruntime.Adapter
	GrantObserver    GrantObserver
	EncryptionKey    string
	UsageTimeout     time.Duration
	ResultRetention  time.Duration
	ResultStore      objectstore.BlobStore
	AdmissionLimiter AdmissionLimiter
}
type Service struct {
	DB *gorm.DB
	Dependencies
	now func() time.Time
}

func New(db *gorm.DB, d Dependencies) *Service {
	if d.UsageTimeout <= 0 {
		d.UsageTimeout = 15 * time.Minute
	}
	if d.ResultRetention <= 0 {
		d.ResultRetention = 24 * time.Hour
	}
	if d.ResultRetention > 24*time.Hour {
		d.ResultRetention = 24 * time.Hour
	}
	return &Service{DB: db, Dependencies: d}
}
func (s *Service) within(ctx context.Context, tx *gorm.DB, fn func(*gorm.DB) error) error {
	if tx != nil {
		return fn(tx.WithContext(ctx))
	}
	return s.DB.WithContext(ctx).Transaction(fn)
}
func (s *Service) fingerprint(v any) string {
	b, _ := json.Marshal(v)
	h := hmac.New(sha256.New, []byte(s.EncryptionKey))
	h.Write(b)
	return hex.EncodeToString(h.Sum(nil))
}

type SupplyHold struct {
	BuyerID string         `json:"buyer_id"`
	HoldID  string         `json:"hold_id,omitempty"`
	Maximum ledger.Decimal `json:"maximum"`
}
type Request struct {
	ID                  string                 `json:"id"`
	CustomerID          string                 `json:"customer_id"`
	OperatorID          string                 `json:"operator_id"`
	BrandID             string                 `json:"brand_id"`
	OwnershipVersion    int64                  `json:"ownership_version"`
	KeyID               string                 `json:"key_id"`
	BusinessKey         string                 `json:"business_key"`
	Fingerprint         string                 `json:"-"`
	ModelID             string                 `json:"model_id"`
	Operation           modelruntime.Operation `json:"operation"`
	Streaming           bool                   `json:"streaming"`
	Status              string                 `json:"status"`
	AcceptedAt          time.Time              `json:"accepted_at"`
	UsageDeadline       time.Time              `json:"usage_deadline"`
	WindowStart         time.Time              `json:"window_start"`
	WindowEnd           *time.Time             `json:"window_end,omitempty"`
	BudgetLimit         *ledger.Decimal        `json:"budget_limit,omitempty"`
	CancelRequested     bool                   `json:"cancel_requested"`
	EstimateATG         ledger.Decimal         `json:"estimate_atg"`
	CorrectedATG        ledger.Decimal         `json:"corrected_atg"`
	CorrectedChargedATG ledger.Decimal         `json:"corrected_charged_atg"`
	ActualATG           ledger.Decimal         `json:"actual_atg"`
	ChargedATG          ledger.Decimal         `json:"charged_atg"`
	MerchantShortfall   ledger.Decimal         `json:"merchant_shortfall"`
	BudgetOverage       ledger.Decimal         `json:"budget_overage"`
	CustomerHoldID      *string                `json:"-"`
	SupplyHolds         json.RawMessage        `json:"-" gorm:"type:jsonb"`
	PriceSnapshot       json.RawMessage        `json:"-" gorm:"type:jsonb"`
	Candidates          json.RawMessage        `json:"-" gorm:"type:jsonb"`
	Settlement          json.RawMessage        `json:"settlement,omitempty" gorm:"type:jsonb"`
	Reason              string                 `json:"reason"`
	CompletedAt         *time.Time             `json:"completed_at,omitempty"`
}

func (Request) TableName() string { return "usage_requests" }
func terminal(status string) bool {
	return status == "completed" || status == "failed" || status == "free" || status == "cancelled"
}

// A single database clock orders monetary facts and policy eligibility across
// API and worker processes. Tests may explicitly inject a controlled clock.
func (s *Service) businessTime(t *gorm.DB) (time.Time, error) {
	if s.now != nil {
		return s.now().UTC(), nil
	}
	var at time.Time
	err := t.Raw("SELECT clock_timestamp()").Row().Scan(&at)
	return at.UTC(), err
}
func (s *Service) wallNow() time.Time {
	if s.now != nil {
		return s.now().UTC()
	}
	return time.Now().UTC()
}
