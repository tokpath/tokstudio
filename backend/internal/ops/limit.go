package ops

import (
	"context"
	"fmt"
	"time"
)

func rpmKey(apiKeyID string) string {
	return "tokenhub:rpm:" + apiKeyID
}

func concKey(apiKeyID string) string {
	return "tokenhub:conc:" + apiKeyID
}

func failKey(providerID string) string {
	return "tokenhub:cb:fail:" + providerID
}

func openKey(providerID string) string {
	return "tokenhub:cb:open:" + providerID
}

func (s *Service) AllowRPM(ctx context.Context, apiKeyID string, limit int) error {
	if apiKeyID == "" || s.redis == nil {
		return nil
	}
	if limit <= 0 {
		limit = DefaultRPM
	}
	n, err := s.redis.Incr(ctx, rpmKey(apiKeyID)).Result()
	if err != nil {
		return err
	}
	if n == 1 {
		_ = s.redis.Expire(ctx, rpmKey(apiKeyID), time.Minute).Err()
	}
	if n > int64(limit) {
		return ErrRateLimited
	}
	return nil
}

func (s *Service) AcquireConcurrency(ctx context.Context, apiKeyID string, limit int) (func(), error) {
	release := func() {}
	if apiKeyID == "" || s.redis == nil {
		return release, nil
	}
	if limit <= 0 {
		limit = 5
	}
	n, err := s.redis.Incr(ctx, concKey(apiKeyID)).Result()
	if err != nil {
		return release, err
	}
	release = func() {
		_ = s.redis.Decr(context.Background(), concKey(apiKeyID)).Err()
	}
	if n > int64(limit) {
		release()
		return func() {}, ErrRateLimited
	}
	return release, nil
}

func (s *Service) CircuitOpen(ctx context.Context, providerID string) bool {
	if providerID == "" || s.redis == nil {
		return false
	}
	n, err := s.redis.Exists(ctx, openKey(providerID)).Result()
	return err == nil && n > 0
}

func (s *Service) RecordAttempt(ctx context.Context, providerID string, success bool) {
	if providerID == "" || s.redis == nil {
		return
	}
	if success {
		_ = s.redis.Del(ctx, failKey(providerID), openKey(providerID)).Err()
		if s.health != nil {
			_ = s.health.MarkHealth(ctx, providerID, "available")
		}
		_ = s.resolveAlert(ctx, AlertCircuit)
		return
	}
	n, err := s.redis.Incr(ctx, failKey(providerID)).Result()
	if err != nil {
		return
	}
	_ = s.redis.Expire(ctx, failKey(providerID), 5*time.Minute).Err()
	if n >= CircuitThreshold {
		_ = s.TripCircuit(ctx, providerID, "consecutive_failures")
	}
}

func (s *Service) TripCircuit(ctx context.Context, providerID, reason string) error {
	if providerID == "" {
		return ErrInvalid
	}
	if s.redis != nil {
		_ = s.redis.Set(ctx, openKey(providerID), reason, 15*time.Minute).Err()
	}
	if s.health != nil {
		_ = s.health.MarkHealth(ctx, providerID, "unavailable")
	}
	return s.openAlert(ctx, AlertCircuit, SeverityHigh, "熔断已打开："+providerID, map[string]string{"provider_id": providerID, "reason": reason})
}

func (s *Service) ResetCircuit(ctx context.Context, providerID string) error {
	if providerID == "" {
		return ErrInvalid
	}
	if s.redis != nil {
		_ = s.redis.Del(ctx, failKey(providerID), openKey(providerID)).Err()
	}
	if s.health != nil {
		_ = s.health.MarkHealth(ctx, providerID, "available")
	}
	return s.resolveAlert(ctx, AlertCircuit)
}

func (s *Service) PingRedis(ctx context.Context) error {
	if s.redis == nil {
		return fmt.Errorf("redis unavailable")
	}
	return s.redis.Ping(ctx).Err()
}

