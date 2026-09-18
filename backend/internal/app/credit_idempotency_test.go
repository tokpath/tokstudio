package app_test

import (
	"context"
	"errors"
	"net/http/httptest"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/billing"
	"github.com/tokpath/tokstudio/backend/internal/platform/config"
	"gorm.io/gorm"
)

func TestWalletCreditIdempotency(t *testing.T) {
	if os.Getenv("TOKENHUB_DATABASE_URL") == "" || os.Getenv("TOKENHUB_REDIS_URL") == "" {
		t.Skip("requires postgres and redis")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.BootstrapAdmin = "credit_idem_admin"
	cfg.BootstrapUser = "credit_idem_user"
	cfg.EncryptionKey = "dev-only-32-byte-key-change-me!!"
	application := mustApp(t, cfg)
	server := httptest.NewServer(application.Router())
	defer server.Close()
	ctx := context.Background()
	users := []string{}
	for _, prefix := range []string{"first", "second"} {
		reg := postBody(t, server.URL+"/v1/auth/register", "", map[string]string{"email": "credit-" + prefix + strconv.FormatInt(time.Now().UnixNano(), 10) + "@example.test", "password": "password1", "promotion_code": "THA1"})
		users = append(users, userIDOf(reg))
	}
	balance := func(uid string) int64 {
		b, err := application.Billing.Balance(ctx, uid, "")
		if err != nil {
			t.Fatal(err)
		}
		return b.PurchasedMinor
	}
	before := balance(users[0])
	otherBefore := balance(users[1])
	key := "credit-retry-" + users[0]
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := application.Billing.Credit(ctx, users[0], key, 2500000, "test"); err != nil {
				t.Errorf("credit retry: %v", err)
			}
		}()
	}
	wg.Wait()
	if got := balance(users[0]); got != before+2500000 {
		t.Fatalf("duplicate balance credit: %d", got-before)
	}
	for _, input := range []struct {
		user   string
		amount int64
	}{{users[0], 3000000}, {users[1], 2500000}} {
		if err := application.Billing.Credit(ctx, input.user, key, input.amount, "test"); !errors.Is(err, billing.ErrConflict) {
			t.Fatalf("changed payload should conflict: %v", err)
		}
	}
	if balance(users[0]) != before+2500000 || balance(users[1]) != otherBefore {
		t.Fatal("conflicting replay changed balance")
	}
	var count int64
	if err := application.DB.Table("billing_ledger").Where("idempotency_key = ?", "pay:"+key).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected one ledger row, got %d", count)
	}
	// A failed ledger write must roll back the balance and allow the same key to retry.
	injected := errors.New("credit ledger failure")
	callback := "credit_idempotency_failure"
	if err := application.DB.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "billing_ledger" {
			tx.AddError(injected)
		}
	}); err != nil {
		t.Fatal(err)
	}
	err = application.Billing.Credit(ctx, users[0], key+"-fail", 1000000, "test")
	application.DB.Callback().Create().Remove(callback)
	if !errors.Is(err, injected) || balance(users[0]) != before+2500000 {
		t.Fatalf("failed credit must roll back: %v", err)
	}
	if err := application.Billing.Credit(ctx, users[0], key+"-fail", 1000000, "test"); err != nil {
		t.Fatal(err)
	}
	if balance(users[0]) != before+3500000 {
		t.Fatal("retry after rollback did not credit exactly once")
	}
	// Competing users cannot consume the same global operation key.
	raceKey := key + "-race"
	results := make(chan error, 2)
	for _, uid := range users {
		wg.Add(1)
		go func(uid string) {
			defer wg.Done()
			results <- application.Billing.Credit(ctx, uid, raceKey, 1000000, "test")
		}(uid)
	}
	wg.Wait()
	close(results)
	succeeded, conflicted := 0, 0
	for err := range results {
		if err == nil {
			succeeded++
		} else if errors.Is(err, billing.ErrConflict) {
			conflicted++
		} else {
			t.Fatal(err)
		}
	}
	if succeeded != 1 || conflicted != 1 {
		t.Fatalf("global key race: success=%d conflict=%d", succeeded, conflicted)
	}
	if balance(users[0])+balance(users[1]) != before+otherBefore+4500000 {
		t.Fatal("cross-user key race created excess credit")
	}
}
