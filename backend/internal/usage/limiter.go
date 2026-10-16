package usage

import (
	"context"
	"errors"
	"github.com/redis/go-redis/v9"
	"time"
)

var ErrAdmissionUnavailable = errors.New("request admission service is temporarily unavailable")

type AdmissionLimiter interface {
	Check(context.Context, string, string, *int64, int64) error
}

// RedisLimiter is a fast atomic gate backed by the durable count obtained while
// the Key row is locked in Postgres. A Redis restart cannot reset actual use.
type RedisLimiter struct{ Client *redis.Client }

var rateScript = redis.NewScript(`
local t=redis.call('TIME')
local now=t[1]*1000+math.floor(t[2]/1000)
redis.call('ZREMRANGEBYSCORE',KEYS[1],'-inf',now-60000)
if redis.call('ZSCORE',KEYS[1],ARGV[1]) then return 1 end
local count=math.max(redis.call('ZCARD',KEYS[1]),tonumber(ARGV[3]))
if count>=tonumber(ARGV[2]) then return 0 end
redis.call('ZADD',KEYS[1],now,ARGV[1])
redis.call('PEXPIRE',KEYS[1],61000)
return 1`)

func (r RedisLimiter) Check(ctx context.Context, key, request string, limit *int64, durable int64) error {
	if r.Client == nil {
		return ErrAdmissionUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if limit == nil {
		if e := r.Client.Ping(ctx).Err(); e != nil {
			return ErrAdmissionUnavailable
		}
		return nil
	}
	allowed, e := rateScript.Run(ctx, r.Client, []string{"tokenhub:usage:rate:" + key}, request, *limit, durable).Int()
	if e != nil {
		return ErrAdmissionUnavailable
	}
	if allowed != 1 {
		return ErrRate
	}
	return nil
}
