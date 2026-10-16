package usage

import (
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/redis/go-redis/v9"
)

func TestExternalRedisAdmissionConcurrentResetAndFailureKeepDurableLimits(t *testing.T) {
	endpoint := os.Getenv("TOKENHUB_TEST_REDIS_URL")
	if endpoint == "" {
		t.Skip("isolated real Redis required")
	}
	options, e := redis.ParseURL(endpoint)
	if e != nil {
		t.Fatal(e)
	}
	client := redis.NewClient(options)
	defer client.Close()
	f := setup(t, "100", nil)
	if e = client.Ping(f.ctx).Err(); e != nil {
		t.Fatal(e)
	}
	rpm := int64(3)
	key, e := f.s.UpdateKey(f.ctx, f.customer, f.key.Key.ID, f.key.Key.Version, KeyInput{Name: "three-per-minute", Period: f.key.Key.Period, Timezone: "UTC", RateLimitRPM: &rpm})
	if e != nil {
		t.Fatal(e)
	}
	f.s.AdmissionLimiter = RedisLimiter{client}
	redisKey := "tokenhub:usage:rate:" + key.ID
	defer client.Del(f.ctx, redisKey)
	var admitted atomic.Int64
	var wg sync.WaitGroup
	failures := make(chan error, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, created, e := f.s.Admit(f.ctx, key, fmtKey(i), request(t, false))
			if e == nil && created {
				admitted.Add(1)
			} else if !errors.Is(e, ErrRate) {
				failures <- e
			}
		}(i)
	}
	wg.Wait()
	close(failures)
	for e := range failures {
		t.Errorf("unexpected admission result %v", e)
	}
	if admitted.Load() != 3 {
		t.Fatalf("accepted %d", admitted.Load())
	}
	if e = client.Del(f.ctx, redisKey).Err(); e != nil {
		t.Fatal(e)
	}
	if _, _, e = f.s.Admit(f.ctx, key, "after-redis-state-loss", request(t, false)); !errors.Is(e, ErrRate) {
		t.Fatal("Redis reset bypassed durable rate", e)
	}
	// A separate unlimited Key isolates dependency failure from rate exhaustion.
	second, e := f.s.CreateKey(f.ctx, f.customer, "outage-key", KeyInput{Name: "outage", Period: key.Period, Timezone: "UTC"})
	if e != nil {
		t.Fatal(e)
	}
	down := redis.NewClient(options)
	if e = down.Close(); e != nil {
		t.Fatal(e)
	}
	f.s.AdmissionLimiter = RedisLimiter{down}
	grants, e := f.l.CustomerGrants(f.ctx, nil, f.operator, f.customer)
	if e != nil {
		t.Fatal(e)
	}
	before, e := f.l.AccountInTransaction(f.ctx, nil, grants[0].AccountID)
	if e != nil {
		t.Fatal(e)
	}
	if _, _, e = f.s.Admit(f.ctx, second.Key, "redis-unavailable", request(t, false)); !errors.Is(e, ErrAdmissionUnavailable) {
		t.Fatal(e)
	}
	after, e := f.l.AccountInTransaction(f.ctx, nil, grants[0].AccountID)
	if e != nil || after.Held.Cmp(before.Held) != 0 || after.Balance.Cmp(before.Balance) != 0 {
		t.Fatal("outage leaked customer reservation", before, after, e)
	}
	var accepted []Request
	if e = f.db.Where("key_id=?", key.ID).Find(&accepted).Error; e != nil || len(accepted) != 3 {
		t.Fatal(accepted, e)
	}
	// Finishing an already accepted call never requires Redis to be available.
	if _, e = f.s.Settle(f.ctx, nil, accepted[0].ID, measured("12", "5")); e != nil {
		t.Fatal("outage blocked earned settlement", e)
	}
}

func fmtKey(i int) string { return "concurrent-rate-" + string(rune('a'+i)) }
