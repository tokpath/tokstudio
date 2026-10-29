// Package remediation coordinates financial exceptions. Original receipts,
// orders and usage remain immutable evidence; all adjustments add new facts.
package remediation

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/cooperation"
	"github.com/tokpath/tokstudio/backend/internal/incentives"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"github.com/tokpath/tokstudio/backend/internal/payments"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

//go:embed migrate/*.sql
var migrations embed.FS

func Migrations() (string, fs.FS) { f, _ := fs.Sub(migrations, "migrate"); return "remediation", f }

var (
	ErrInvalid         = errors.New("invalid financial exception input")
	ErrConflict        = errors.New("financial exception conflicts with its original facts")
	ErrState           = errors.New("financial exception cannot transition from its current state")
	ErrScope           = errors.New("financial exception is outside the responsible operator or customer")
	ErrNotReady        = errors.New("financial exception dependency is unavailable")
	ErrReview          = errors.New("financial exception needs independent review")
	ErrPaymentMismatch = errors.New("actual external settlement requires reconciliation")
)

type Dependencies struct {
	Ledger       *ledger.Service
	Organization *organization.Service
	Commerce     *commerce.Service
	Cooperation  *cooperation.Service
	Incentives   *incentives.Service
	Payments     *payments.Service
}
type Service struct {
	DB *gorm.DB
	Dependencies
}

func New(db *gorm.DB, d Dependencies) *Service { return &Service{db, d} }
func (s *Service) within(ctx context.Context, tx *gorm.DB, fn func(*gorm.DB) error) error {
	if tx != nil {
		return fn(tx.WithContext(ctx))
	}
	return s.DB.WithContext(ctx).Transaction(fn)
}
func fingerprint(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func instant(t *gorm.DB) (time.Time, error) {
	var at time.Time
	e := t.Raw("SELECT clock_timestamp()").Row().Scan(&at)
	return at.UTC(), e
}
func validID(id string) bool            { _, e := uuid.Parse(id); return e == nil }
func validAmount(d ledger.Decimal) bool { return d.Valid() && d.Sign() > 0 }
func (s *Service) account(ctx context.Context, t *gorm.DB, op, owner, purpose string) (ledger.Account, error) {
	return s.Ledger.EnsureAccount(ctx, t, ledger.Account{OperatorID: op, OwnerID: owner, Purpose: purpose, Unit: "MONEY", AllowNegative: strings.HasPrefix(purpose, "control:")})
}
func lock(t *gorm.DB, key string) error {
	return t.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", "remediation:"+key).Error
}
func (s *Service) event(t *gorm.DB, op, subject, action, actor, evidence string, v any) error {
	b, e := json.Marshal(v)
	if e != nil {
		return e
	}
	at, e := instant(t)
	if e != nil {
		return e
	}
	return t.Exec("INSERT INTO remediation_events(id,operator_id,subject_id,action,actor_id,evidence,data,created_at) VALUES(?,?,?,?,?,?,?::jsonb,?)", uuid.NewString(), op, subject, action, actor, evidence, string(b), at).Error
}

type Policy struct {
	OperatorID        string          `json:"operator_id"`
	SecondReviewAbove *ledger.Decimal `json:"second_review_above"`
	Version           int64           `json:"version"`
	UpdatedBy         string          `json:"updated_by"`
	UpdatedAt         time.Time       `json:"updated_at"`
}

func (Policy) TableName() string { return "remediation_policies" }
func (s *Service) SetPolicy(ctx context.Context, actor, op string, above *ledger.Decimal, expected int64) (Policy, error) {
	out := Policy{}
	if above != nil && (!above.Valid() || above.Sign() < 0) {
		return out, ErrInvalid
	}
	e := s.within(ctx, nil, func(t *gorm.DB) error {
		if e := s.Organization.RequirePermission(ctx, t, actor, op, "refunds.manage"); e != nil {
			return e
		}
		if e := lock(t, op+":policy"); e != nil {
			return e
		}
		e := t.First(&out, "operator_id=?", op).Error
		if e != nil && !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		if out.Version != expected {
			return ErrConflict
		}
		at, e := instant(t)
		if e != nil {
			return e
		}
		out = Policy{op, above, expected + 1, actor, at}
		if e = t.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "operator_id"}}, UpdateAll: true}).Create(&out).Error; e != nil {
			return e
		}
		return s.event(t, op, op, "refund_policy.changed", actor, "operator review policy", out)
	})
	return out, e
}
func (s *Service) reviews(t *gorm.DB, op string, amount ledger.Decimal) (int, error) {
	var p Policy
	e := t.First(&p, "operator_id=?", op).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return 1, nil
	}
	if e != nil {
		return 0, e
	}
	if p.SecondReviewAbove != nil && amount.Cmp(*p.SecondReviewAbove) >= 0 {
		return 2, nil
	}
	return 1, nil
}

type Case struct {
	ID              string         `json:"id"`
	OperatorID      string         `json:"operator_id"`
	CustomerID      *string        `json:"customer_id,omitempty"`
	BusinessKey     string         `json:"business_key"`
	Fingerprint     string         `json:"-"`
	Kind            string         `json:"kind"`
	OrderID         *string        `json:"order_id,omitempty"`
	GrantID         *string        `json:"grant_id,omitempty"`
	GrantHoldID     *string        `json:"grant_hold_id,omitempty"`
	AmountMoney     ledger.Decimal `json:"amount_money"`
	CreditATG       ledger.Decimal `json:"credit_atg"`
	InternalMoney   ledger.Decimal `json:"internal_money"`
	CommissionMoney ledger.Decimal `json:"commission_money"`
	State           string         `json:"state"`
	Reason          string         `json:"reason"`
	Evidence        string         `json:"evidence"`
	RequestedBy     string         `json:"requested_by"`
	RequiredReviews int            `json:"required_reviews"`
	CreatedAt       time.Time      `json:"created_at"`
	CompletedAt     *time.Time     `json:"completed_at,omitempty"`
}

func (Case) TableName() string { return "remediation_cases" }

type RefundLeg struct {
	ID                 string         `json:"id"`
	CaseID             string         `json:"case_id"`
	OperatorID         string         `json:"operator_id"`
	SourceOperatorID   string         `json:"source_operator_id"`
	ReceiptID          string         `json:"receipt_id"`
	AmountMoney        ledger.Decimal `json:"amount_money"`
	SettledMoney       ledger.Decimal `json:"settled_money"`
	ExcessMoney        ledger.Decimal `json:"excess_money"`
	MisdirectedMoney   ledger.Decimal `json:"misdirected_money"`
	HoldID             *string        `json:"hold_id,omitempty"`
	SourceAccountID    *string        `json:"source_account_id,omitempty"`
	State              string         `json:"state"`
	ExecutionKey       string         `json:"execution_key"`
	RouteVersion       int64          `json:"route_version" gorm:"default:1"`
	ExecutionMode      string         `json:"execution_mode"`
	ExecutionReference string         `json:"execution_reference"`
	PaymentRefundID    string         `json:"payment_refund_id"`
	CompletedAt        *time.Time     `json:"completed_at,omitempty"`
	CreatedAt          time.Time      `json:"created_at"`
}

func (RefundLeg) TableName() string { return "remediation_refund_legs" }
func (s *Service) GetCase(ctx context.Context, tx *gorm.DB, op, id string) (Case, error) {
	var out Case
	t := tx
	if t == nil {
		t = s.DB
	}
	e := t.WithContext(ctx).First(&out, "id=? AND operator_id=?", id, op).Error
	return out, e
}
func (s *Service) Cases(ctx context.Context, op, customer string) ([]Case, error) {
	q := s.DB.WithContext(ctx).Where("operator_id=?", op)
	if customer != "" {
		q = q.Where("customer_id=?", customer)
	}
	var out []Case
	e := q.Order("created_at DESC,id").Limit(200).Find(&out).Error
	return out, e
}
func (s *Service) Legs(ctx context.Context, tx *gorm.DB, op, id string) ([]RefundLeg, error) {
	t := tx
	if t == nil {
		t = s.DB
	}
	var out []RefundLeg
	e := t.WithContext(ctx).Where("operator_id=? AND case_id=?", op, id).Order("created_at,id").Find(&out).Error
	return out, e
}
