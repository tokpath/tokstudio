package ledger

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

//go:embed migrate/*.sql
var migrationFiles embed.FS

func Migrations() (string, fs.FS) { f, _ := fs.Sub(migrationFiles, "migrate"); return "ledger", f }

var (
	ErrInsufficient = errors.New("insufficient available asset")
	ErrConflict     = errors.New("idempotency key used for a different command")
	ErrScope        = errors.New("asset scope or unit mismatch")
	ErrHoldState    = errors.New("hold is not active")
	ErrUnbalanced   = errors.New("posting must balance per unit")
)

type Account struct {
	ID            string    `json:"id"`
	OperatorID    string    `json:"operator_id"`
	OwnerID       string    `json:"owner_id"`
	Purpose       string    `json:"purpose"`
	Unit          string    `json:"unit"`
	AllowNegative bool      `json:"-"`
	Balance       Decimal   `json:"balance"`
	Held          Decimal   `json:"held"`
	Version       int64     `json:"version"`
	CreatedAt     time.Time `json:"created_at"`
}

func (Account) TableName() string    { return "ledger_accounts" }
func (a Account) Available() Decimal { return a.Balance.Sub(a.Held) }

type Line struct {
	AccountID string  `json:"account_id"`
	Amount    Decimal `json:"amount"`
}
type Posting struct {
	OperatorID  string  `json:"operator_id"`
	BusinessKey string  `json:"business_key"`
	Kind        string  `json:"kind"`
	SourceID    string  `json:"source_id"`
	ActorID     string  `json:"actor_id"`
	Reason      string  `json:"reason"`
	OriginalID  *string `json:"original_id,omitempty"`
	Lines       []Line  `json:"lines"`
}
type Journal struct {
	ID          string    `json:"id"`
	OperatorID  string    `json:"operator_id"`
	BusinessKey string    `json:"business_key"`
	Kind        string    `json:"kind"`
	SourceID    string    `json:"source_id"`
	Fingerprint string    `json:"-"`
	ActorID     string    `json:"actor_id"`
	Reason      string    `json:"reason"`
	OriginalID  *string   `json:"original_id,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

func (Journal) TableName() string { return "ledger_journals" }

type Entry struct {
	ID           string
	JournalID    string
	AccountID    string
	Unit         string
	Amount       Decimal
	BalanceAfter Decimal
	CreatedAt    time.Time
}

func (Entry) TableName() string { return "ledger_entries" }

type Service struct{ db *gorm.DB }

func New(db *gorm.DB) *Service { return &Service{db: db} }
func (s *Service) within(ctx context.Context, tx *gorm.DB, fn func(*gorm.DB) error) error {
	if tx != nil {
		return fn(tx.WithContext(ctx))
	}
	return s.db.WithContext(ctx).Transaction(fn)
}
func fingerprint(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func (s *Service) EnsureAccount(ctx context.Context, tx *gorm.DB, a Account) (Account, error) {
	if _, e := uuid.Parse(a.OperatorID); e != nil {
		return Account{}, ErrScope
	}
	if _, e := uuid.Parse(a.OwnerID); e != nil {
		return Account{}, ErrScope
	}
	if a.Purpose == "" || (a.Unit != "USD" && a.Unit != "ATG") || a.AllowNegative != strings.HasPrefix(a.Purpose, "control:") {
		return Account{}, ErrScope
	}
	a.ID = uuid.NewString()
	a.Balance = Zero()
	a.Held = Zero()
	a.Version = 0
	a.CreatedAt = time.Now().UTC()
	err := s.within(ctx, tx, func(t *gorm.DB) error {
		if err := t.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "operator_id"}, {Name: "owner_id"}, {Name: "purpose"}, {Name: "unit"}}, DoNothing: true}).Create(&a).Error; err != nil {
			return err
		}
		var existing Account
		if err := t.Where("operator_id=? AND owner_id=? AND purpose=? AND unit=?", a.OperatorID, a.OwnerID, a.Purpose, a.Unit).First(&existing).Error; err != nil {
			return err
		}
		if existing.AllowNegative != a.AllowNegative {
			return ErrConflict
		}
		a = existing
		return nil
	})
	return a, err
}

func (s *Service) GetAccount(ctx context.Context, id string) (Account, error) {
	var a Account
	err := s.db.WithContext(ctx).First(&a, "id=?", id).Error
	return a, err
}

func normalizePosting(p Posting) (Posting, error) {
	if p.OperatorID == "" || p.BusinessKey == "" || len(p.BusinessKey) > 200 || p.Kind == "" || p.SourceID == "" || p.ActorID == "" || p.Reason == "" {
		return p, ErrScope
	}
	values := map[string]Decimal{}
	for _, l := range p.Lines {
		if _, e := uuid.Parse(l.AccountID); e != nil || !l.Amount.Valid() {
			return p, ErrAmount
		}
		values[l.AccountID] = values[l.AccountID].Add(l.Amount)
	}
	p.Lines = nil
	for id, v := range values {
		if !v.Valid() {
			return p, ErrAmount
		}
		if !v.IsZero() {
			p.Lines = append(p.Lines, Line{id, v})
		}
	}
	sort.Slice(p.Lines, func(i, j int) bool { return p.Lines[i].AccountID < p.Lines[j].AccountID })
	if len(p.Lines) < 2 {
		return p, ErrUnbalanced
	}
	return p, nil
}

func lockAccounts(t *gorm.DB, ids []string) (map[string]Account, error) {
	unique := map[string]bool{}
	for _, id := range ids {
		unique[id] = true
	}
	ids = ids[:0]
	for id := range unique {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var rows []Account
	if err := t.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id IN ?", ids).Order("id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) != len(ids) {
		return nil, ErrScope
	}
	out := map[string]Account{}
	for _, a := range rows {
		out[a.ID] = a
	}
	return out, nil
}

func (s *Service) Post(ctx context.Context, tx *gorm.DB, p Posting) (Journal, error) {
	p, err := normalizePosting(p)
	if err != nil {
		return Journal{}, err
	}
	var result Journal
	err = s.within(ctx, tx, func(t *gorm.DB) error { var e error; result, e = s.post(t, p); return e })
	return result, err
}

// post requires one transaction. The idempotency insert and all balances commit together.
func (s *Service) post(t *gorm.DB, p Posting) (Journal, error) {
	if e := lockPosting(t, p); e != nil {
		return Journal{}, e
	}
	var recordedAt time.Time
	if e := t.Raw("SELECT clock_timestamp()").Row().Scan(&recordedAt); e != nil {
		return Journal{}, e
	}
	j := Journal{ID: uuid.NewString(), OperatorID: p.OperatorID, BusinessKey: p.BusinessKey, Kind: p.Kind, SourceID: p.SourceID, Fingerprint: fingerprint(p), ActorID: p.ActorID, Reason: p.Reason, OriginalID: p.OriginalID, CreatedAt: recordedAt.UTC()}
	created := t.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "operator_id"}, {Name: "business_key"}}, DoNothing: true}).Create(&j)
	if created.Error != nil {
		return Journal{}, created.Error
	}
	if created.RowsAffected == 0 {
		var old Journal
		if e := t.Where("operator_id=? AND business_key=?", p.OperatorID, p.BusinessKey).First(&old).Error; e != nil {
			return old, e
		}
		if old.Fingerprint != j.Fingerprint {
			return old, ErrConflict
		}
		return old, nil
	}
	if p.OriginalID != nil {
		var original Journal
		if e := t.First(&original, "id=?", *p.OriginalID).Error; e != nil {
			return Journal{}, e
		}
		if original.OperatorID != p.OperatorID {
			return Journal{}, ErrScope
		}
	}
	ids := make([]string, 0, len(p.Lines))
	for _, l := range p.Lines {
		ids = append(ids, l.AccountID)
	}
	accounts, e := lockAccounts(t, ids)
	if e != nil {
		return Journal{}, e
	}
	totals := map[string]Decimal{}
	for _, l := range p.Lines {
		a := accounts[l.AccountID]
		if a.OperatorID != p.OperatorID {
			return Journal{}, ErrScope
		}
		totals[a.Unit] = totals[a.Unit].Add(l.Amount)
		after := a.Balance.Add(l.Amount)
		if !after.Valid() {
			return Journal{}, ErrAmount
		}
		if !a.AllowNegative && after.Cmp(a.Held) < 0 {
			return Journal{}, ErrInsufficient
		}
	}
	for _, total := range totals {
		if !total.IsZero() {
			return Journal{}, ErrUnbalanced
		}
	}
	for _, l := range p.Lines {
		a := accounts[l.AccountID]
		after := a.Balance.Add(l.Amount)
		if e := t.Model(&Account{}).Where("id=?", a.ID).Updates(map[string]any{"balance": after, "version": gorm.Expr("version+1")}).Error; e != nil {
			return Journal{}, e
		}
		entry := Entry{ID: uuid.NewString(), JournalID: j.ID, AccountID: a.ID, Unit: a.Unit, Amount: l.Amount, BalanceAfter: after, CreatedAt: j.CreatedAt}
		if e := t.Create(&entry).Error; e != nil {
			return Journal{}, e
		}
	}
	return j, nil
}

func lockPosting(t *gorm.DB, p Posting) error {
	return t.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", p.OperatorID+":journal:"+p.BusinessKey).Error
}

// Transaction supplies a common transaction to cross-module application workflows.
func (s *Service) Transaction(ctx context.Context, fn func(*gorm.DB) error) error {
	return s.within(ctx, nil, fn)
}

func (s *Service) Reconcile(ctx context.Context, tx *gorm.DB, id string) error {
	return s.within(ctx, tx, func(t *gorm.DB) error {
		accounts, err := lockAccounts(t, []string{id})
		if err != nil {
			return err
		}
		a := accounts[id]
		var total Decimal
		if err = t.Raw("SELECT COALESCE(sum(amount),0)::text FROM ledger_entries WHERE account_id=?", id).Row().Scan(&total); err != nil {
			return err
		}
		if total.Cmp(a.Balance) != 0 {
			return fmt.Errorf("ledger balance differs for %s", id)
		}
		var held Decimal
		if err = t.Raw("SELECT COALESCE(sum(a.amount),0)::text FROM ledger_hold_allocations a JOIN ledger_holds h ON h.id=a.hold_id WHERE a.account_id=? AND h.status='active'", id).Row().Scan(&held); err != nil {
			return err
		}
		if held.Cmp(a.Held) != 0 {
			return fmt.Errorf("ledger holds differ for %s", id)
		}
		return nil
	})
}
