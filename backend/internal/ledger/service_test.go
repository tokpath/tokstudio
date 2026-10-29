package ledger

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func testDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("TOKENHUB_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TOKENHUB_TEST_DATABASE_URL required for PostgreSQL invariants")
	}
	root, e := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if e != nil {
		t.Fatal(e)
	}
	schema := "ledger_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if e = root.Exec("CREATE SCHEMA " + schema).Error; e != nil {
		t.Fatal(e)
	}
	u, e := url.Parse(dsn)
	if e != nil {
		t.Fatal(e)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	db, e := gorm.Open(postgres.Open(u.String()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if e != nil {
		t.Fatal(e)
	}
	files, e := migrationFiles.ReadDir("migrate")
	if e != nil {
		t.Fatal(e)
	}
	for _, file := range files {
		body, e := migrationFiles.ReadFile("migrate/" + file.Name())
		if e != nil {
			t.Fatal(e)
		}
		if e = db.Exec(string(body)).Error; e != nil {
			t.Fatal(e)
		}
	}
	t.Cleanup(func() {
		sql, _ := db.DB()
		_ = sql.Close()
		root.Exec("DROP SCHEMA " + schema + " CASCADE")
		r, _ := root.DB()
		_ = r.Close()
	})
	return db
}
func fixture(t *testing.T) (*Service, string, Account, Account) {
	t.Helper()
	s := New(testDB(t))
	op := uuid.NewString()
	ctx := context.Background()
	control, e := s.EnsureAccount(ctx, nil, Account{OperatorID: op, OwnerID: op, Purpose: "control:issued", Unit: "ATG", AllowNegative: true})
	if e != nil {
		t.Fatal(e)
	}
	customer, e := s.EnsureAccount(ctx, nil, Account{OperatorID: op, OwnerID: uuid.NewString(), Purpose: "customer:atg", Unit: "ATG"})
	if e != nil {
		t.Fatal(e)
	}
	_, e = s.Post(ctx, nil, Posting{OperatorID: op, BusinessKey: "initial", Kind: "grant", SourceID: "order1", ActorID: "test", Reason: "paid grant", Lines: []Line{{control.ID, FromInt64(-20)}, {customer.ID, FromInt64(20)}}})
	if e != nil {
		t.Fatal(e)
	}
	return s, op, customer, control
}
func debit(op, customer, control, key string, n Decimal) Posting {
	return Posting{OperatorID: op, BusinessKey: key, Kind: "usage", SourceID: key, ActorID: "test", Reason: "confirmed usage", Lines: []Line{{customer, n.Neg()}, {control, n}}}
}
func TestPostConcurrentNoDoubleSpend(t *testing.T) {
	s, op, c, k := fixture(t)
	var success atomic.Int64
	var wg sync.WaitGroup
	errs := make(chan error, 40)
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, e := s.Post(context.Background(), nil, debit(op, c.ID, k.ID, fmt.Sprint(i), MustDecimal("0.7")))
			if e == nil {
				success.Add(1)
			} else if !errors.Is(e, ErrInsufficient) {
				errs <- e
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Error(e)
	}
	if success.Load() != 28 {
		t.Fatal(success.Load())
	}
	a, e := s.GetAccount(context.Background(), c.ID)
	if e != nil || a.Balance.String() != "0.4" {
		t.Fatal(a, e)
	}
	if e = s.Reconcile(context.Background(), nil, c.ID); e != nil {
		t.Fatal(e)
	}
}
func TestIdempotentPostingAndImmutableFacts(t *testing.T) {
	s, op, c, k := fixture(t)
	p := debit(op, c.ID, k.ID, "same", FromInt64(3))
	one, e := s.Post(context.Background(), nil, p)
	if e != nil {
		t.Fatal(e)
	}
	two, e := s.Post(context.Background(), nil, p)
	if e != nil || one.ID != two.ID {
		t.Fatal(two, e)
	}
	p.Lines[0].Amount = FromInt64(-4)
	p.Lines[1].Amount = FromInt64(4)
	if _, e = s.Post(context.Background(), nil, p); !errors.Is(e, ErrConflict) {
		t.Fatal(e)
	}
	if e = s.db.Exec("UPDATE ledger_entries SET amount=0 WHERE journal_id=?", one.ID).Error; e == nil {
		t.Fatal("mutable fact")
	}
	if e = s.Reconcile(context.Background(), nil, c.ID); e != nil {
		t.Fatal(e)
	}
}
func TestHoldsCaptureAndRollback(t *testing.T) {
	s, op, c, k := fixture(t)
	ctx := context.Background()
	h, e := s.Reserve(ctx, nil, Reservation{op, "request1", "model", time.Now().Add(time.Hour), []Line{{c.ID, FromInt64(12)}}})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Post(ctx, nil, debit(op, c.ID, k.ID, "competing", FromInt64(9))); !errors.Is(e, ErrInsufficient) {
		t.Fatal(e)
	}
	if _, e = s.Capture(ctx, nil, h.ID, debit(op, c.ID, k.ID, "capture", FromInt64(13)), nil); !errors.Is(e, ErrInsufficient) {
		t.Fatal(e)
	}
	a, _ := s.GetAccount(ctx, c.ID)
	if a.Held.String() != "12" || a.Balance.String() != "20" {
		t.Fatal(a)
	}
	one, e := s.Capture(ctx, nil, h.ID, debit(op, c.ID, k.ID, "capture", FromInt64(10)), nil)
	if e != nil {
		t.Fatal(e)
	}
	two, e := s.Capture(ctx, nil, h.ID, debit(op, c.ID, k.ID, "capture", FromInt64(10)), nil)
	if e != nil || one.ID != two.ID {
		t.Fatal(e)
	}
	a, _ = s.GetAccount(ctx, c.ID)
	if a.Held.Sign() != 0 || a.Balance.String() != "10" {
		t.Fatal(a)
	}
	if _, e = s.Release(ctx, nil, op, h.ID, "cancel"); !errors.Is(e, ErrHoldState) {
		t.Fatal(e)
	}
	if e = s.Reconcile(ctx, nil, c.ID); e != nil {
		t.Fatal(e)
	}
}
func TestCustomerOverageCannotStealOtherHold(t *testing.T) {
	s, op, c, k := fixture(t)
	ctx := context.Background()
	expiry := time.Now().Add(time.Hour)
	h, e := s.Reserve(ctx, nil, Reservation{op, "one", "model", expiry, []Line{{c.ID, FromInt64(10)}}})
	if e != nil {
		t.Fatal(e)
	}
	_, e = s.Reserve(ctx, nil, Reservation{op, "two", "model", expiry, []Line{{c.ID, FromInt64(8)}}})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Capture(ctx, nil, h.ID, debit(op, c.ID, k.ID, "settle", FromInt64(13)), map[string]bool{c.ID: true}); !errors.Is(e, ErrInsufficient) {
		t.Fatal(e)
	}
	if _, e = s.Capture(ctx, nil, h.ID, debit(op, c.ID, k.ID, "settle", FromInt64(12)), map[string]bool{c.ID: true}); e != nil {
		t.Fatal(e)
	}
	a, _ := s.GetAccount(ctx, c.ID)
	if a.Balance.String() != "8" || a.Held.String() != "8" {
		t.Fatal(a)
	}
}
func TestUnitAndScopeAreNotInterchangeable(t *testing.T) {
	s, op, c, _ := fixture(t)
	ctx := context.Background()
	usd, e := s.EnsureAccount(ctx, nil, Account{OperatorID: op, OwnerID: c.OwnerID, Purpose: "customer:money", Unit: "MONEY"})
	if e != nil {
		t.Fatal(e)
	}
	_, e = s.Post(ctx, nil, Posting{OperatorID: op, BusinessKey: "bad-unit", Kind: "usage", SourceID: "bad", ActorID: "test", Reason: "bad", Lines: []Line{{c.ID, FromInt64(-1)}, {usd.ID, FromInt64(1)}}})
	if !errors.Is(e, ErrUnbalanced) {
		t.Fatal(e)
	}
	_, e = s.EnsureAccount(ctx, nil, Account{OperatorID: op, OwnerID: c.OwnerID, Purpose: "customer:money", Unit: "MONEY", AllowNegative: true})
	if !errors.Is(e, ErrScope) {
		t.Fatal(e)
	}
}
