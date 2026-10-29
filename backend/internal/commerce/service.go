package commerce

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"gorm.io/gorm"
)

//go:embed migrate/*.sql
var migrationFS embed.FS

func Migrations() (string, fs.FS) {
	sub, err := fs.Sub(migrationFS, "migrate")
	if err != nil {
		panic(err)
	}
	return "commerce", sub
}

type Service struct {
	db                       *gorm.DB
	ledger                   *ledger.Service
	org                      *organization.Service
	coverage                 SalesCoverage
	marketing                Marketing
	commissionFunding        CommissionFunding
	receiptGuard             ReceiptAllocationGuard
	now                      func() time.Time
	databaseFulfillmentClock bool
}

func New(db *gorm.DB) *Service {
	return &Service{db: db, ledger: ledger.New(db), org: organization.New(db), databaseFulfillmentClock: true, now: func() time.Time { return time.Now().UTC().Truncate(time.Microsecond) }}
}

// Configure adapters before the service starts serving concurrent requests.
func (s *Service) SetSalesCoverage(c SalesCoverage) { s.coverage = c }
func (s *Service) SetMonetaryReadiness(check func(context.Context, *gorm.DB) error) {
	s.org.SetMonetaryReadiness(check)
}
func (s *Service) SetMarketing(m Marketing) {
	s.marketing = m
	s.commissionFunding, _ = m.(CommissionFunding)
}
func (s *Service) SetCommissionFunding(f CommissionFunding) { s.commissionFunding = f }
func (s *Service) SetReceiptGuard(g ReceiptAllocationGuard) { s.receiptGuard = g }

// Production coverage adapters expose the shared business lock so pricing,
// campaigns, orders and asset mutations acquire it before their own row locks.
func (s *Service) lockBusinessScope(ctx context.Context, tx *gorm.DB, operator string) error {
	if locker, ok := s.coverage.(interface {
		Lock(context.Context, *gorm.DB, string) error
	}); ok {
		return locker.Lock(ctx, tx, operator)
	}
	return nil
}
func (s *Service) within(ctx context.Context, tx *gorm.DB, f func(*gorm.DB) error) error {
	if tx != nil {
		return classify(f(tx.WithContext(ctx)))
	}
	return classify(s.db.WithContext(ctx).Transaction(f))
}
func (s *Service) dbFor(ctx context.Context, tx *gorm.DB) *gorm.DB {
	if tx == nil {
		tx = s.db
	}
	return tx.WithContext(ctx)
}
func classify(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	var p *pgconn.PgError
	if errors.As(err, &p) {
		if p.Code == "23505" {
			return ErrConflict
		}
		if p.Code == "23514" && (p.ConstraintName == "commerce_receipts_allocated_money_check" || p.ConstraintName == "commerce_orders_check1") {
			return ErrExcessAllocation
		}
	}
	return err
}
func validID(id string) bool { _, err := uuid.Parse(id); return err == nil }
func fingerprint(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func (s *Service) event(tx *gorm.DB, operator, id, kind string, payload any) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return tx.Exec(`INSERT INTO commerce_events(id,operator_id,aggregate_id,kind,payload,created_at) VALUES(?,?,?,?,?::jsonb,?)`, uuid.NewString(), operator, id, kind, string(b), s.now()).Error
}
func lockCommand(tx *gorm.DB, scope, key string) error {
	return tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended(?,0))`, "commerce:"+scope+":"+key).Error
}

type packageRow Package

func (packageRow) TableName() string { return "commerce_packages" }

type quoteRow PackageQuote

func (quoteRow) TableName() string { return "commerce_package_quotes" }

type orderRow Order

func (orderRow) TableName() string { return "commerce_orders" }

type receiptRow Receipt

func (receiptRow) TableName() string { return "commerce_receipts" }

type allocationRow Allocation

func (allocationRow) TableName() string { return "commerce_receipt_allocations" }

type deliveryRow Delivery

func (deliveryRow) TableName() string { return "commerce_deliveries" }

type subscriptionRow Subscription

func (subscriptionRow) TableName() string { return "commerce_subscriptions" }
func (s *Service) account(ctx context.Context, tx *gorm.DB, operator, owner, purpose string) (ledger.Account, error) {
	return s.ledger.EnsureAccount(ctx, tx, ledger.Account{OperatorID: operator, OwnerID: owner, Purpose: purpose, Unit: "MONEY", AllowNegative: len(purpose) >= 8 && purpose[:8] == "control:"})
}
func orderTotalATG(o Order) ledger.Decimal {
	return o.PaidATG.Add(o.PackageGiftATG).Add(o.MerchantGiftATG).Add(o.KOLGiftATG)
}
func internalTotal(o Order) ledger.Decimal { return o.InternalMoney.Add(o.CommissionMoney) }
func externalDue(o Order) ledger.Decimal   { return o.DueMoney.Sub(internalTotal(o)) }
