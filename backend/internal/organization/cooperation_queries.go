package organization

import (
	"context"
	"gorm.io/gorm"
)

// Children is an internal hierarchy query. The application must authorize the
// parent cooperation scope before exposing this result to a caller.
func (s *Service) Children(ctx context.Context, tx *gorm.DB, parent string) ([]Operator, error) {
	var rows []operatorRow
	e := s.dbFor(ctx, tx).Where("parent_operator_id=?", parent).Order("created_at,id").Find(&rows).Error
	out := make([]Operator, len(rows))
	for i, r := range rows {
		out[i] = Operator(r)
	}
	return out, e
}
