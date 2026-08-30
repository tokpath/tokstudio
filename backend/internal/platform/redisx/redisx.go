// Package redisx 封装 Redis。Redis 只做限流、缓存和心跳，不是账务事实源。
package redisx

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const WorkerHeartbeatKey = "tokenhub:worker:heartbeat"

// Open 解析 REDIS URL 并 ping。
func Open(redisURL string) (*redis.Client, error) {
	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("parse redis url: %w", err)
	}
	client := redis.NewClient(opt)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("ping redis: %w", err)
	}
	return client, nil
}

// Ping 检查 Redis。
func Ping(ctx context.Context, client *redis.Client) error {
	return client.Ping(ctx).Err()
}

// TouchWorkerHeartbeat 由 Outbox Worker 定期写入。
func TouchWorkerHeartbeat(ctx context.Context, client *redis.Client) error {
	return client.Set(ctx, WorkerHeartbeatKey, time.Now().UTC().Format(time.RFC3339Nano), 30*time.Second).Err()
}

// WorkerHeartbeatAge 返回心跳年龄。没有心跳时返回 false。
func WorkerHeartbeatAge(ctx context.Context, client *redis.Client) (time.Duration, bool, error) {
	value, err := client.Get(ctx, WorkerHeartbeatKey).Result()
	if err == redis.Nil {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	ts, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return 0, false, err
	}
	return time.Since(ts), true, nil
}
