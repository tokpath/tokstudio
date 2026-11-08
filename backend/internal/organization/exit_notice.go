package organization

import (
	"context"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strings"
	"time"
)

type ExitNotice struct {
	ID         string       `json:"id"`
	OperatorID string       `json:"operator_id"`
	ParentID   string       `json:"parent_id"`
	Kind       OperatorKind `json:"kind"`
	Note       string       `json:"note"`
	ActorID    string       `json:"actor_id"`
	CreatedAt  time.Time    `json:"created_at"`
}

func (ExitNotice) TableName() string { return "organization_exit_notices" }
func (s *Service) ReportExit(ctx context.Context, tx *gorm.DB, actor, operator, note string) (ExitNotice, error) {
	var out ExitNotice
	e := s.withTx(ctx, tx, func(t *gorm.DB) error {
		if e := s.RequirePermission(ctx, t, actor, operator, "cooperation.manage"); e != nil {
			return e
		}
		var op operatorRow
		if e := t.Clauses(clause.Locking{Strength: "UPDATE"}).First(&op, "id=?", operator).Error; e != nil {
			return e
		}
		if op.Kind == Platform || op.ParentOperatorID == nil {
			return ErrInvalidRelationship
		}
		if len(note) > 2000 {
			return ErrInvalidInput
		}
		q := t.Where("operator_id=?", operator).Limit(1).Find(&out)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected > 0 {
			return nil
		}
		out = ExitNotice{ID: newID(), OperatorID: operator, ParentID: *op.ParentOperatorID, Kind: op.Kind, Note: strings.TrimSpace(note), ActorID: actor, CreatedAt: s.now()}
		if e := t.Create(&out).Error; e != nil {
			return e
		}
		return s.event(t, operator, actor, "cooperation.exit_reported", out.ID, out)
	})
	return out, e
}
func (s *Service) ExitNotices(ctx context.Context, actor, operator string) ([]ExitNotice, error) {
	if e := s.RequirePermission(ctx, nil, actor, operator, "cooperation.manage"); e != nil {
		return nil, e
	}
	rows := []ExitNotice{}
	e := s.dbFor(ctx, nil).Where("operator_id=? OR parent_id=?", operator, operator).Order("created_at DESC,id").Find(&rows).Error
	return rows, e
}
