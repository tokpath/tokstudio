package redisx

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

const IdempotencyTTL = 24 * time.Hour

var ErrIdempotencyConflict = errors.New("idempotency conflict")

type IdemRecord struct {
	Hash   string `json:"hash"`
	Status int    `json:"status"`
	Body   []byte `json:"body"`
}

func HashBody(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func IdempotencyKey(actor, key string) string {
	return "tokenhub:idem:" + actor + ":" + key
}

func RecallIdempotency(ctx context.Context, client *redis.Client, actor, key, bodyHash string) (*IdemRecord, error) {
	if client == nil || key == "" {
		return nil, nil
	}
	raw, err := client.Get(ctx, IdempotencyKey(actor, key)).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var rec IdemRecord
	if err := json.Unmarshal(raw, &rec); err != nil {
		return nil, err
	}
	if rec.Hash != bodyHash {
		return nil, ErrIdempotencyConflict
	}
	return &rec, nil
}

func RememberIdempotency(ctx context.Context, client *redis.Client, actor, key, bodyHash string, status int, body []byte) error {
	if client == nil || key == "" {
		return nil
	}
	payload, err := json.Marshal(IdemRecord{Hash: bodyHash, Status: status, Body: body})
	if err != nil {
		return err
	}
	return client.Set(ctx, IdempotencyKey(actor, key), payload, IdempotencyTTL).Err()
}
