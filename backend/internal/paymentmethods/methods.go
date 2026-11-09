// Package paymentmethods owns the single fee rule shared by collections and
// both withdrawal wallets. Each accepted transaction retains its own snapshot.
package paymentmethods

import (
	"context"
	"embed"
	"errors"
	"io/fs"
	"math/big"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"golang.org/x/text/currency"
	"gorm.io/gorm"
)

//go:embed migrate/*.sql
var migrations embed.FS

func Migrations() (string, fs.FS) { f, _ := fs.Sub(migrations, "migrate"); return "paymentmethods", f }
func Schema() string {
	a, _ := migrations.ReadFile("migrate/0001_methods.sql")
	b, _ := migrations.ReadFile("migrate/0002_method_catalog.sql")
	c, _ := migrations.ReadFile("migrate/0003_payout_details.sql")
	return string(a) + "\n" + string(b) + "\n" + string(c)
}

var ErrInvalid = errors.New("invalid payment method fee rule")
var ErrChanged = errors.New("payment method fees changed; review the updated amounts")

type Rule struct {
	Kind          string         `json:"method_kind" gorm:"column:method_kind"`
	Paused        bool           `json:"paused"`
	ID            string         `json:"id"`
	MethodID      string         `json:"method_id"`
	OperatorID    string         `json:"operator_id"`
	Version       int64          `json:"version"`
	Name          string         `json:"name"`
	Collection    bool           `json:"collection"`
	PayoutEnabled bool           `json:"payout_enabled"`
	PayoutKind    string         `json:"payout_kind"`
	FlatFeeMoney  ledger.Decimal `json:"flat_fee_money"`
	FeeRate       ledger.Decimal `json:"fee_rate"`
	CreatedBy     string         `json:"created_by"`
	CreatedAt     time.Time      `json:"created_at"`
}

func (Rule) TableName() string { return "payment_method_versions" }
func Current(ctx context.Context, db *gorm.DB, op, method string) (Rule, error) {
	var r Rule
	e := db.WithContext(ctx).Where("operator_id=? AND method_id=?", op, method).Order("version DESC").First(&r).Error
	return r, e
}
func Version(ctx context.Context, db *gorm.DB, op, id string) (Rule, error) {
	var r Rule
	e := db.WithContext(ctx).Where("operator_id=? AND id=?", op, id).First(&r).Error
	return r, e
}
func List(ctx context.Context, db *gorm.DB, op string) ([]Rule, error) {
	rows := []Rule{}
	e := db.WithContext(ctx).Raw("SELECT * FROM (SELECT DISTINCT ON(method_id) * FROM payment_method_versions WHERE operator_id=? ORDER BY method_id,version DESC) m ORDER BY name,method_id", op).Scan(&rows).Error
	return rows, e
}

// Save requires the caller's authorization and transaction. The lock protects
// concurrent edits; quotes reference immutable version IDs, never current fees.
func Save(ctx context.Context, tx *gorm.DB, r Rule, expected int64) (Rule, error) {
	if r.MethodID == "" || r.OperatorID == "" || len(r.MethodID) > 150 || len(r.OperatorID) > 150 {
		return r, ErrInvalid
	}
	if strings.TrimSpace(r.Name) == "" || len(r.Name) > 100 || expected < 0 || !r.FlatFeeMoney.Valid() || r.FlatFeeMoney.Sign() < 0 || !r.FeeRate.Valid() || r.FeeRate.Sign() < 0 || r.FeeRate.Cmp(ledger.FromInt64(1)) >= 0 || r.FlatFeeMoney.Sign() > 0 && r.FeeRate.Sign() > 0 || r.PayoutEnabled && r.PayoutKind == "" {
		return r, ErrInvalid
	}
	if e := tx.WithContext(ctx).Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", "payment-method:"+r.OperatorID+":"+r.MethodID).Error; e != nil {
		return r, e
	}
	old, e := Current(ctx, tx, r.OperatorID, r.MethodID)
	if e != nil && !errors.Is(e, gorm.ErrRecordNotFound) {
		return r, e
	}
	if r.Kind != "" {
		m, ok := Find(r.Kind)
		if !ok || old.Kind != "" && old.Kind != r.Kind {
			return r, ErrInvalid
		}
		r.Name = m.Label
		r.PayoutKind = m.PayoutKind
		if r.PayoutEnabled && r.PayoutKind == "" {
			return r, ErrInvalid
		}
		identity := Identity{OperatorID: r.OperatorID, Kind: r.Kind, MethodID: r.MethodID}
		if e := tx.WithContext(ctx).Exec("INSERT INTO payment_method_identities(operator_id,kind,method_id) VALUES(?,?,?) ON CONFLICT DO NOTHING", identity.OperatorID, identity.Kind, identity.MethodID).Error; e != nil {
			return r, e
		}
		var existing Identity
		if e := tx.Where("operator_id=? AND kind=?", r.OperatorID, r.Kind).First(&existing).Error; e != nil {
			return r, e
		}
		if existing.MethodID != r.MethodID {
			return r, ErrChanged
		}
	}
	if old.Version != expected {
		return r, ErrChanged
	}
	if r.CreatedBy == "" {
		r.CreatedBy = "system:payment-methods"
	}
	r.ID = uuid.NewString()
	r.Version = expected + 1
	r.CreatedAt = time.Now().UTC()
	return r, tx.WithContext(ctx).Create(&r).Error
}

type Quote struct {
	RuleID      string         `json:"rule_id"`
	MethodID    string         `json:"method_id"`
	MethodName  string         `json:"method_name"`
	Currency    string         `json:"currency"`
	AmountMoney ledger.Decimal `json:"amount_money"`
	FeeMoney    ledger.Decimal `json:"fee_money"`
	TotalMoney  ledger.Decimal `json:"total_money"`
}

// Fee rounds once in the settlement currency, on the external principal only.
func Fee(r Rule, amount ledger.Decimal, code string) (ledger.Decimal, error) {
	unit, e := currency.ParseISO(code)
	if e != nil || amount.Sign() < 0 || !amount.Valid() {
		return ledger.Zero(), ErrInvalid
	}
	digits, _ := currency.Standard.Rounding(unit)
	scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(digits)), nil)
	exact := new(big.Rat).Add(r.FlatFeeMoney.Rat(), new(big.Rat).Mul(amount.Rat(), r.FeeRate.Rat()))
	raw := new(big.Rat).Mul(exact, new(big.Rat).SetInt(scale))
	numerator := new(big.Int).Add(new(big.Int).Mul(raw.Num(), big.NewInt(2)), raw.Denom())
	units := new(big.Int).Quo(numerator, new(big.Int).Mul(raw.Denom(), big.NewInt(2)))
	fee, _ := ledger.Quantize(new(big.Rat).SetFrac(units, scale))
	return fee, nil
}
func CollectionQuote(r Rule, amount ledger.Decimal, code string) (Quote, error) {
	if !r.Collection || amount.Sign() <= 0 {
		return Quote{}, ErrInvalid
	}
	fee, e := Fee(r, amount, code)
	return Quote{RuleID: r.ID, MethodID: r.MethodID, MethodName: r.Name, Currency: code, AmountMoney: amount, FeeMoney: fee, TotalMoney: amount.Add(fee)}, e
}
