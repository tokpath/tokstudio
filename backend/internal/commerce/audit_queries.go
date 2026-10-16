package commerce

import (
	"context"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"gorm.io/gorm"
)

type AuditEvent = organization.AuditEvent

func (s *Service) ListAuditEvents(ctx context.Context, tx *gorm.DB, actor, operator, cursor string, limit int) ([]AuditEvent, string, error) {
	if limit < 1 || limit > 200 || !validID(operator) {
		return nil, "", ErrInvalidInput
	}
	at, id, e := organization.ParseAuditCursor(cursor)
	if e != nil {
		return nil, "", e
	}
	if e = s.org.RequirePermission(ctx, tx, actor, operator, "ledger.read"); e != nil {
		return nil, "", e
	}
	q := s.dbFor(ctx, tx).Table("commerce_events").Select("id,operator_id,aggregate_id AS subject_id,kind AS action,COALESCE(payload->>'actor_id',payload->>'actor_staff_id','') AS actor_id,COALESCE(payload->>'evidence',payload->>'evidence_reference',payload->>'verification_evidence',payload->>'terms_reference','') AS evidence,payload->'before' AS before,payload->'after' AS after,payload AS data,created_at").Where("operator_id=?", operator)
	if at != nil {
		q = q.Where("(created_at,id)<(?,?::uuid)", *at, id)
	}
	rows := []AuditEvent{}
	e = q.Order("created_at DESC,id DESC").Limit(limit).Scan(&rows).Error
	return rows, organization.NextAuditCursor(rows, limit), e
}
