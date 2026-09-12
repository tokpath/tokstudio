package app

import (
	"context"
	"encoding/json"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/platform/crypto"
)

const (
	googleOAuthDoneTTL     = 5 * time.Minute
	googleOAuthInflightTTL = 60 * time.Second
	googleOAuthWaitBudget  = 3 * time.Second
	googleOAuthWaitStep    = 50 * time.Millisecond
)

func googleOAuthDoneKey(state string) string {
	return "tokenhub:oauth:done:" + crypto.HashToken(state)
}

func googleOAuthInflightKey(state string) string {
	return "tokenhub:oauth:inflight:" + crypto.HashToken(state)
}

func (a *App) rememberGoogleOAuth(ctx context.Context, state string, session *identity.Session) {
	if a.Redis == nil || state == "" || session == nil {
		return
	}
	raw, err := json.Marshal(session)
	if err != nil {
		return
	}
	_ = a.Redis.Set(ctx, googleOAuthDoneKey(state), raw, googleOAuthDoneTTL).Err()
}

func (a *App) recallGoogleOAuth(ctx context.Context, state string) *identity.Session {
	if a.Redis == nil || state == "" {
		return nil
	}
	raw, err := a.Redis.Get(ctx, googleOAuthDoneKey(state)).Bytes()
	if err == redis.Nil || err != nil || len(raw) == 0 {
		return nil
	}
	var session identity.Session
	if err := json.Unmarshal(raw, &session); err != nil || session.Token == "" {
		return nil
	}
	return &session
}

func (a *App) waitGoogleOAuth(ctx context.Context, state string, budget time.Duration) *identity.Session {
	deadline := time.Now().Add(budget)
	for {
		if session := a.recallGoogleOAuth(ctx, state); session != nil {
			return session
		}
		if time.Now().After(deadline) {
			return nil
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(googleOAuthWaitStep):
		}
	}
}

// beginGoogleOAuthFlight 用 Redis SETNX 串行化同 state 的兑码；false 表示已有同伴在飞。
func (a *App) beginGoogleOAuthFlight(ctx context.Context, state string) bool {
	if a.Redis == nil || state == "" {
		return true
	}
	ok, err := a.Redis.SetNX(ctx, googleOAuthInflightKey(state), "1", googleOAuthInflightTTL).Result()
	if err != nil {
		return true
	}
	return ok
}

func (a *App) endGoogleOAuthFlight(ctx context.Context, state string) {
	if a.Redis == nil || state == "" {
		return
	}
	_ = a.Redis.Del(ctx, googleOAuthInflightKey(state)).Err()
}
