package remediation

import (
	"context"
	"encoding/json"
	"gorm.io/gorm"
	"time"
)

type Event struct {
	ID         string          `json:"id"`
	OperatorID string          `json:"operator_id"`
	SubjectID  string          `json:"subject_id"`
	Action     string          `json:"action"`
	ActorID    string          `json:"actor_id"`
	Evidence   string          `json:"evidence"`
	Data       json.RawMessage `gorm:"type:jsonb" json:"data"`
	CreatedAt  time.Time       `json:"created_at"`
}

// ListEvents exposes the original financial audit facts only to a staff member
// with ledger access at this exact operator. A parent relationship is not access.
// The pair (created_at,id) is a stable descending cursor, including tied times.
func (s *Service) ListEvents(ctx context.Context, actor, operator string, limit int, beforeAt *time.Time, beforeID string) ([]Event, error) {
	if limit < 1 || limit > 200 || (beforeAt == nil) != (beforeID == "") || (beforeID != "" && !validID(beforeID)) {
		return nil, ErrInvalid
	}
	if e := s.Organization.RequirePermission(ctx, nil, actor, operator, "ledger.read"); e != nil {
		return nil, e
	}
	q := s.DB.WithContext(ctx).Table("remediation_events").Where("operator_id=?", operator)
	if beforeAt != nil {
		q = q.Where("(created_at,id)<(?,?::uuid)", *beforeAt, beforeID)
	}
	out := []Event{}
	e := q.Order("created_at DESC,id DESC").Limit(limit).Find(&out).Error
	return out, e
}

func (s *Service) RelayEvents(ctx context.Context, tx *gorm.DB, limit int, publish func(context.Context, *gorm.DB, Event) error) (int, error) {
	if limit < 1 || limit > 1000 || publish == nil {
		return 0, ErrInvalid
	}
	count := 0
	e := s.within(ctx, tx, func(t *gorm.DB) error {
		var events []Event
		if e := t.Raw(`SELECT e.* FROM remediation_events e WHERE NOT EXISTS(SELECT 1 FROM remediation_event_publications p WHERE p.event_id=e.id) ORDER BY e.created_at,e.id LIMIT ? FOR UPDATE OF e SKIP LOCKED`, limit).Scan(&events).Error; e != nil {
			return e
		}
		for _, event := range events {
			if e := publish(ctx, t, event); e != nil {
				return e
			}
			if e := t.Exec("INSERT INTO remediation_event_publications(event_id,published_at) VALUES(?,clock_timestamp())", event.ID).Error; e != nil {
				return e
			}
			count++
		}
		return nil
	})
	return count, e
}
