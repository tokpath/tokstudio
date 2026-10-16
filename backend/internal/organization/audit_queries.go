package organization

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"time"

	"gorm.io/gorm"
)

// AuditEvent contains only recorded facts. Missing actor/before/after fields
// remain empty or null; callers must not invent a financial before-image.
type AuditEvent struct {
	ID         string          `json:"id"`
	OperatorID string          `json:"operator_id"`
	SubjectID  string          `json:"subject_id"`
	Action     string          `json:"action"`
	ActorID    string          `json:"actor_id"`
	Evidence   string          `json:"evidence"`
	Before     json.RawMessage `gorm:"type:jsonb" json:"before"`
	After      json.RawMessage `gorm:"type:jsonb" json:"after"`
	Data       json.RawMessage `gorm:"type:jsonb" json:"data"`
	CreatedAt  time.Time       `json:"created_at"`
}
type auditCursor struct {
	At time.Time `json:"at"`
	ID string    `json:"id"`
}

func ParseAuditCursor(cursor string) (*time.Time, string, error) {
	if cursor == "" {
		return nil, "", nil
	}
	if len(cursor) > 512 {
		return nil, "", ErrInvalidInput
	}
	b, e := base64.RawURLEncoding.DecodeString(cursor)
	if e != nil {
		return nil, "", ErrInvalidInput
	}
	var c auditCursor
	if e = json.Unmarshal(b, &c); e != nil || c.At.IsZero() || !validID(c.ID) {
		return nil, "", ErrInvalidInput
	}
	return &c.At, c.ID, nil
}
func NextAuditCursor(rows []AuditEvent, limit int) string {
	if len(rows) < limit || len(rows) == 0 {
		return ""
	}
	last := rows[len(rows)-1]
	b, _ := json.Marshal(auditCursor{At: last.CreatedAt, ID: last.ID})
	return base64.RawURLEncoding.EncodeToString(b)
}
func (s *Service) ListAuditEvents(ctx context.Context, tx *gorm.DB, actor, operator, cursor string, limit int) ([]AuditEvent, string, error) {
	if limit < 1 || limit > 200 || !validID(operator) {
		return nil, "", ErrInvalidInput
	}
	at, id, e := ParseAuditCursor(cursor)
	if e != nil {
		return nil, "", e
	}
	if e = s.RequirePermission(ctx, tx, actor, operator, "reports.view"); e != nil {
		return nil, "", e
	}
	q := s.dbFor(ctx, tx).Table("organization_events").Select("id,operator_id,COALESCE(payload->>'customer_id',payload->>'scope_id',payload->>'CustomerID',operator_id::text,'') AS subject_id,event_type AS action,COALESCE(actor_id::text,'') AS actor_id,evidence_reference AS evidence,payload->'before' AS before,payload->'after' AS after,payload AS data,created_at").Where("operator_id=?", operator)
	if at != nil {
		q = q.Where("(created_at,id)<(?,?::uuid)", *at, id)
	}
	rows := []AuditEvent{}
	e = q.Order("created_at DESC,id DESC").Limit(limit).Scan(&rows).Error
	return rows, NextAuditCursor(rows, limit), e
}
