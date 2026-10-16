package organization

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/mail"
	"strings"
	"time"

	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

//go:embed migrate/*.sql
var migrationFS embed.FS

func Migrations() (string, fs.FS) {
	sub, err := fs.Sub(migrationFS, "migrate")
	if err != nil {
		panic(err)
	}
	return "organization", sub
}

type Service struct {
	db          *gorm.DB
	enforcer    *casbin.Enforcer
	now         func() time.Time
	exitChecker OperatorExitChecker
}

func New(db *gorm.DB) *Service {
	m, err := model.NewModelFromString(`[request_definition]
r = role, action
[policy_definition]
p = role, action
[policy_effect]
e = some(where (p.eft == allow))
[matchers]
m = r.role == p.role && (p.action == "*" || r.action == p.action)
`)
	if err != nil {
		panic(err)
	}
	e, err := casbin.NewEnforcer(m)
	if err != nil {
		panic(err)
	}
	for role, actions := range rolePermissions {
		for _, action := range actions {
			if _, err = e.AddPolicy(role, action); err != nil {
				panic(err)
			}
		}
	}
	return &Service{db: db, enforcer: e, now: func() time.Time { return time.Now().UTC() }}
}

type brandRow Brand

func (brandRow) TableName() string { return "organization_brands" }

type operatorRow Operator

func (operatorRow) TableName() string { return "organization_operators" }

type supplyRow SupplyAgreement

func (supplyRow) TableName() string { return "organization_supply_agreements" }

type businessRow BusinessStatus

func (businessRow) TableName() string { return "organization_operator_statuses" }

type membershipRow Membership

func (membershipRow) TableName() string { return "organization_staff_memberships" }

type customerRow Customer

func (customerRow) TableName() string { return "organization_customers" }

type ownershipRow Ownership

func (ownershipRow) TableName() string { return "organization_service_ownerships" }

func (s *Service) dbFor(ctx context.Context, tx *gorm.DB) *gorm.DB {
	if tx == nil {
		tx = s.db
	}
	return tx.WithContext(ctx)
}

// A supplied tx remains owned by its caller. The caller must roll it back when
// a participating command fails; nil starts a complete atomic transaction.
func (s *Service) withTx(ctx context.Context, tx *gorm.DB, fn func(*gorm.DB) error) error {
	if tx != nil {
		return classify(fn(tx.WithContext(ctx)))
	}
	return classify(s.db.WithContext(ctx).Transaction(fn))
}
func classify(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		if pg.Code == "23505" {
			if pg.ConstraintName == "organization_brand_email_unique" {
				return ErrEmailAlreadyRegistered
			}
			return ErrConflict
		}
		if pg.Code == "23514" || pg.Code == "23503" {
			return fmt.Errorf("%w: %w", ErrInvalidRelationship, err)
		}
	}
	return err
}
func validID(id string) bool { _, err := uuid.Parse(id); return err == nil }
func newID() string          { return uuid.NewString() }
func token() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
func tokenHash(t string) string { b := sha256.Sum256([]byte(t)); return hex.EncodeToString(b[:]) }
func tokenMatches(raw, hash string) bool {
	return raw != "" && subtle.ConstantTimeCompare([]byte(tokenHash(raw)), []byte(hash)) == 1
}
func inviteCode() (string, error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return strings.ToUpper(hex.EncodeToString(b)), nil
}

// NormalizeEmail deliberately applies no provider-specific dot or plus rules.
func NormalizeEmail(value string) (string, error) {
	v := strings.ToLower(strings.TrimSpace(value))
	a, err := mail.ParseAddress(v)
	if err != nil || a.Address != v || !strings.Contains(v, "@") || len(v) > 254 {
		return "", ErrInvalidInput
	}
	return v, nil
}
func (s *Service) event(tx *gorm.DB, operatorID, actorID, eventType, evidence string, payload any) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	var actor, operator any
	if actorID != "" {
		actor = actorID
	}
	if operatorID != "" {
		operator = operatorID
	}
	return tx.Exec(`INSERT INTO organization_events(id,operator_id,actor_id,event_type,evidence_reference,payload,created_at) VALUES (?,?,?,?,?,?::jsonb,?)`, newID(), operator, actor, eventType, evidence, string(b), s.now()).Error
}
func (s *Service) GetBrand(ctx context.Context, tx *gorm.DB, id string) (Brand, error) {
	if !validID(id) {
		return Brand{}, ErrInvalidInput
	}
	var r brandRow
	err := s.dbFor(ctx, tx).First(&r, "id = ?", id).Error
	return Brand(r), classify(err)
}
func (s *Service) GetOperator(ctx context.Context, tx *gorm.DB, id string) (Operator, error) {
	if !validID(id) {
		return Operator{}, ErrInvalidInput
	}
	var r operatorRow
	err := s.dbFor(ctx, tx).First(&r, "id = ?", id).Error
	return Operator(r), classify(err)
}
func (s *Service) GetCustomer(ctx context.Context, tx *gorm.DB, id string) (Customer, error) {
	if !validID(id) {
		return Customer{}, ErrInvalidInput
	}
	var r customerRow
	err := s.dbFor(ctx, tx).First(&r, "id = ?", id).Error
	return Customer(r), classify(err)
}
