package branding

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
	op, e := s.directory.GetOperator(ctx, tx, operator)
	if e != nil {
		return nil, "", e
	}
	q := s.dbFor(ctx, tx).Table("branding_changes").Select("id,?::text AS operator_id,target_id AS subject_id,action,actor_id,evidence,before_json AS before,after_json AS after,after_json AS data,created_at", operator)
	if op.Kind == organization.Channel {
		if e = s.directory.RequireChannelBranding(ctx, tx, actor, operator); e != nil {
			return nil, "", e
		}
		q = q.Where("brand_id=? AND ((target_type='channel' AND target_id=?) OR (target_type='logo' AND after_json->>'scope_type'='channel' AND after_json->>'scope_id'=?))", op.BrandID, operator, operator)
	} else {
		if e = s.directory.RequireBrandBranding(ctx, tx, actor, op.BrandID); e != nil {
			return nil, "", e
		}
		q = q.Where("brand_id=? AND (target_type IN('brand','domain') OR (target_type='logo' AND after_json->>'scope_type'='brand'))", op.BrandID)
	}
	if at != nil {
		q = q.Where("(created_at,id)<(?,?::uuid)", *at, id)
	}
	rows := []AuditEvent{}
	e = q.Order("created_at DESC,id DESC").Limit(limit).Scan(&rows).Error
	return rows, organization.NextAuditCursor(rows, limit), e
}
