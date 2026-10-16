package operations

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"strings"
	"time"
)

type RetentionItem struct {
	ID                string     `json:"id"`
	OperatorID        string     `json:"operator_id"`
	Kind              string     `json:"kind"`
	Reference         string     `json:"reference"`
	PayloadCiphertext string     `json:"-"`
	ExpiresAt         time.Time  `json:"expires_at"`
	LegalHoldUntil    *time.Time `json:"legal_hold_until,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	PurgedAt          *time.Time `json:"purged_at,omitempty"`
}

func (RetentionItem) TableName() string { return "operations_retention_items" }

// StoreTemporary is a trusted hook for explicitly time-bounded content. No
// request-body logging endpoint is offered. Callers must already have the
// customer's temporary debugging authorization before storing content.
func (s *Service) StoreTemporary(ctx context.Context, tx *gorm.DB, operator, kind, reference string, payload json.RawMessage, expires time.Time) (RetentionItem, error) {
	out := RetentionItem{}
	now := time.Now().UTC()
	if !validID(operator) || (kind != "temporary_content" && kind != "technical_record") || strings.TrimSpace(reference) == "" || !json.Valid(payload) || len(payload) > 1<<20 || !expires.After(now) || expires.Sub(now) > 30*24*time.Hour {
		return out, ErrInvalid
	}
	cipher, e := s.seal(payload)
	if e != nil {
		return out, e
	}
	out = RetentionItem{ID: uuid.NewString(), OperatorID: operator, Kind: kind, Reference: reference, PayloadCiphertext: cipher, ExpiresAt: expires.UTC(), CreatedAt: now}
	e = s.within(ctx, tx, func(t *gorm.DB) error { return t.Create(&out).Error })
	return out, e
}
func (s *Service) HoldTemporary(ctx context.Context, actor, operator, id, reason string, until time.Time) error {
	if reason == "" || !until.After(time.Now()) || until.Sub(time.Now()) > 90*24*time.Hour {
		return ErrInvalid
	}
	return s.within(ctx, nil, func(t *gorm.DB) error {
		if e := s.require(ctx, t, actor, operator, "operations.manage"); e != nil {
			return e
		}
		r := t.Model(&RetentionItem{}).Where("id=? AND operator_id=? AND purged_at IS NULL", id, operator).Update("legal_hold_until", until.UTC())
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 {
			return ErrExpired
		}
		return t.Table("operations_retention_access").Create(map[string]any{"id": uuid.NewString(), "item_id": id, "actor_id": actor, "action": "legal_hold", "reason": reason, "at": time.Now().UTC()}).Error
	})
}
func (s *Service) ReadTemporary(ctx context.Context, actor, operator, id string) (json.RawMessage, error) {
	var out json.RawMessage
	e := s.within(ctx, nil, func(t *gorm.DB) error {
		if e := s.require(ctx, t, actor, operator, "security.manage"); e != nil {
			return e
		}
		var row RetentionItem
		if e := t.First(&row, "id=? AND operator_id=?", id, operator).Error; e != nil {
			return e
		}
		now := time.Now().UTC()
		if row.PurgedAt != nil || (!now.Before(row.ExpiresAt) && (row.LegalHoldUntil == nil || !now.Before(*row.LegalHoldUntil))) {
			return ErrExpired
		}
		if e := s.open(row.PayloadCiphertext, &out); e != nil {
			return e
		}
		return t.Table("operations_retention_access").Create(map[string]any{"id": uuid.NewString(), "item_id": id, "actor_id": actor, "action": "read", "reason": "authorized technical review", "at": time.Now().UTC()}).Error
	})
	return out, e
}

type RetentionResult struct {
	TemporaryPurged    int64 `json:"temporary_purged"`
	ExportsPurged      int64 `json:"exports_purged"`
	ModelResultsPurged int64 `json:"model_results_purged"`
}

func (s *Service) RunRetention(ctx context.Context, at time.Time, limit int) (RetentionResult, error) {
	if at.IsZero() || at.After(time.Now().Add(time.Minute)) {
		return RetentionResult{}, ErrInvalid
	}
	if limit < 1 || limit > 1000 {
		limit = 100
	}
	var out RetentionResult
	r := s.DB.WithContext(ctx).Exec(`UPDATE operations_retention_items SET payload_ciphertext='',purged_at=? WHERE id IN(SELECT id FROM operations_retention_items WHERE purged_at IS NULL AND expires_at<=? AND (legal_hold_until IS NULL OR legal_hold_until<=?) ORDER BY expires_at LIMIT ? FOR UPDATE SKIP LOCKED)`, at, at, at, limit)
	out.TemporaryPurged = r.RowsAffected
	if r.Error != nil {
		return out, r.Error
	}
	r = s.DB.WithContext(ctx).Model(&Export{}).Where("expires_at<=? AND state<>'expired'", at).Updates(map[string]any{"content_ciphertext": "", "state": "expired", "lease_until": nil})
	out.ExportsPurged = r.RowsAffected
	if r.Error != nil {
		return out, r.Error
	}
	if s.Results == nil {
		return out, ErrNotReady
	}
	var e error
	out.ModelResultsPurged, e = s.Results.PurgeExpiredResults(ctx, at, limit)
	return out, e
}

type RetentionPolicy struct {
	TemporaryMaxDays int `json:"temporary_max_days"`
	LegalHoldMaxDays int `json:"legal_hold_max_days"`
	ExportHours      int `json:"export_hours"`
}

func (s *Service) RetentionPolicy() RetentionPolicy {
	return RetentionPolicy{TemporaryMaxDays: 30, LegalHoldMaxDays: 90, ExportHours: 24}
}
func (s *Service) ListRetention(ctx context.Context, actor, operator string) ([]RetentionItem, error) {
	out := []RetentionItem{}
	e := s.within(ctx, nil, func(t *gorm.DB) error {
		if e := s.require(ctx, t, actor, operator, "security.manage"); e != nil {
			return e
		}
		return t.Where("operator_id=?", operator).Order("created_at DESC").Limit(100).Find(&out).Error
	})
	return out, e
}
