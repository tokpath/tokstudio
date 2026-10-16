package incentives

import (
	"context"
	"gorm.io/gorm"
)

func (s *Service) HoldEventDispute(ctx context.Context, tx *gorm.DB, actor, operator, sourceEvent, reference, evidence string) error {
	return s.setEventDisputeReference(ctx, tx, actor, operator, sourceEvent, reference, evidence, true)
}
func (s *Service) ReleaseEventDispute(ctx context.Context, tx *gorm.DB, actor, operator, sourceEvent, reference, evidence string) error {
	return s.setEventDisputeReference(ctx, tx, actor, operator, sourceEvent, reference, evidence, false)
}
func (s *Service) setEventDisputeReference(ctx context.Context, tx *gorm.DB, actor, operator, sourceEvent, reference, evidence string, active bool) error {
	if !validID(sourceEvent) || reference == "" || len(reference) > 200 || evidence == "" {
		return ErrInvalidInput
	}
	return s.within(ctx, tx, func(t *gorm.DB) error {
		if e := s.org.RequirePermission(ctx, t, actor, operator, "refunds.manage"); e != nil {
			return e
		}
		if e := s.LockBusiness(ctx, t, operator); e != nil {
			return e
		}
		var event confirmationRow
		if e := t.First(&event, "source_event_id=? AND operator_id=?", sourceEvent, operator).Error; e != nil {
			return e
		}
		if event.Status == "confirmed" {
			if active {
				return ErrConflict
			}
			return nil
		}
		if e := t.Exec(`INSERT INTO incentives_event_dispute_holders(event_id,reference,active,evidence,created_at,updated_at) VALUES(?,?,?,?,clock_timestamp(),clock_timestamp()) ON CONFLICT(event_id,reference) DO UPDATE SET active=EXCLUDED.active,evidence=EXCLUDED.evidence,updated_at=EXCLUDED.updated_at`, event.ID, reference, active, evidence).Error; e != nil {
			return e
		}
		var remaining int64
		if e := t.Table("incentives_event_dispute_holders").Where("event_id=? AND active=true", event.ID).Count(&remaining).Error; e != nil {
			return e
		}
		status := "pending"
		if remaining > 0 {
			status = "disputed"
		}
		if e := t.Model(&confirmationRow{}).Where("id=? OR original_event_id=?", event.ID, event.ID).Update("status", status).Error; e != nil {
			return e
		}
		return s.event(t, operator, event.ID, "confirmation.dispute_changed", map[string]any{"reference": reference, "active": active, "remaining_holders": remaining, "evidence": evidence})
	})
}
