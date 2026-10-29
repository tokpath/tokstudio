package ledger

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/text/currency"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const MoneyUnit = "MONEY"

var (
	ErrMonetaryLocked   = errors.New("monetary foundation is permanently locked")
	ErrMonetaryNotReady = errors.New("monetary foundation is not effective")
)

// MoneyUnit names the one system settlement asset. Currency is immutable once
// effective; original external currencies are recorded separately, never valued
// by this ATG ratio. There is deliberately no foreign-exchange rate here.
type MonetarySettings struct {
	ID           string     `json:"-"`
	Currency     string     `json:"currency"`
	AtgPerUnit   Decimal    `json:"atg_per_unit"`
	EffectiveAt  *time.Time `json:"effective_at,omitempty"`
	Version      int64      `json:"version"`
	ConfiguredBy string     `json:"configured_by,omitempty"`
	ConfirmedAt  *time.Time `json:"confirmed_at,omitempty"`
	Status       string     `json:"status" gorm:"-"`
}

func (MonetarySettings) TableName() string { return "ledger_monetary_settings" }

func GetMonetary(ctx context.Context, tx *gorm.DB) (MonetarySettings, error) {
	var out MonetarySettings
	if tx == nil {
		return out, ErrScope
	}
	var now time.Time
	if e := tx.WithContext(ctx).Raw("SELECT clock_timestamp()").Scan(&now).Error; e != nil {
		return out, e
	}
	if e := tx.WithContext(ctx).First(&out, "id='platform'").Error; e != nil {
		return out, e
	}
	out.Status = "unconfigured"
	if out.EffectiveAt != nil {
		out.Status = "scheduled"
		if !now.Before(*out.EffectiveAt) {
			out.Status = "locked"
		}
	}
	return out, nil
}

func RequireMonetary(ctx context.Context, tx *gorm.DB) (MonetarySettings, error) {
	out, err := GetMonetary(ctx, tx)
	if err == nil && out.Status != "locked" {
		err = ErrMonetaryNotReady
	}
	return out, err
}

type MonetaryInput struct {
	Currency        string    `json:"currency"`
	AtgPerUnit      Decimal   `json:"atg_per_unit"`
	EffectiveAt     time.Time `json:"effective_at"`
	ExpectedVersion int64     `json:"expected_version"`
	Confirmed       bool      `json:"confirmed"`
}

// ConfigureMonetary must be called only after a platform-owner check, in the
// same transaction. The row lock and DB clock serialize cancellation and expiry.
func ConfigureMonetary(ctx context.Context, tx *gorm.DB, actor string, in MonetaryInput) (MonetarySettings, error) {
	if !in.Confirmed || actor == "" || in.Currency != strings.ToUpper(in.Currency) || len(in.Currency) != 3 || in.AtgPerUnit.Sign() <= 0 || !in.AtgPerUnit.Valid() {
		return MonetarySettings{}, ErrAmount
	}
	if _, e := currency.ParseISO(in.Currency); e != nil || in.Currency == "XXX" || in.Currency == "XTS" {
		return MonetarySettings{}, ErrAmount
	}
	var row MonetarySettings
	if e := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, "id='platform'").Error; e != nil {
		return row, e
	}
	var now time.Time
	if e := tx.Raw("SELECT clock_timestamp()").Scan(&now).Error; e != nil {
		return row, e
	}
	if row.EffectiveAt != nil && !now.Before(*row.EffectiveAt) {
		return row, ErrMonetaryLocked
	}
	if row.Version != in.ExpectedVersion || row.EffectiveAt != nil {
		return row, ErrConflict
	}
	if in.EffectiveAt.IsZero() || !in.EffectiveAt.After(now) {
		return row, ErrAmount
	}
	row.Currency, row.AtgPerUnit = in.Currency, in.AtgPerUnit
	at := in.EffectiveAt.UTC()
	row.EffectiveAt, row.ConfirmedAt, row.ConfiguredBy = &at, &now, actor
	row.Version++
	if e := tx.Save(&row).Error; e != nil {
		return row, e
	}
	if e := monetaryAudit(tx, actor, "scheduled", row, now); e != nil {
		return row, e
	}
	return GetMonetary(ctx, tx)
}

func CancelMonetary(ctx context.Context, tx *gorm.DB, actor string, expected int64) (MonetarySettings, error) {
	var row MonetarySettings
	if e := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, "id='platform'").Error; e != nil {
		return row, e
	}
	var now time.Time
	if e := tx.Raw("SELECT clock_timestamp()").Scan(&now).Error; e != nil {
		return row, e
	}
	if row.EffectiveAt != nil && !now.Before(*row.EffectiveAt) {
		return row, ErrMonetaryLocked
	}
	if row.Version != expected || row.EffectiveAt == nil {
		return row, ErrConflict
	}
	if e := monetaryAudit(tx, actor, "cancelled", row, now); e != nil {
		return row, e
	}
	row.EffectiveAt = nil
	row.Version++
	if e := tx.Save(&row).Error; e != nil {
		return row, e
	}
	return GetMonetary(ctx, tx)
}

func monetaryAudit(tx *gorm.DB, actor, action string, row MonetarySettings, at time.Time) error {
	return tx.Table("ledger_monetary_events").Create(map[string]any{"id": uuid.NewString(), "actor_id": actor, "action": action, "currency": row.Currency, "atg_per_unit": row.AtgPerUnit, "effective_at": row.EffectiveAt, "version": row.Version, "created_at": at}).Error
}

type MonetarySnapshot struct {
	Currency   string  `json:"currency"`
	AtgPerUnit Decimal `json:"atg_per_unit"`
	Version    int64   `json:"version"`
}

func (m MonetarySettings) Snapshot() MonetarySnapshot {
	return MonetarySnapshot{m.Currency, m.AtgPerUnit, m.Version}
}
