package payment

import "context"

type CustomerOrderSummary struct {
	Count    int64 `json:"count"`
	Paid     int64 `json:"paid"`
	Pending  int64 `json:"pending"`
	Refunded int64 `json:"refunded"`
}

func (s *Service) CustomerOrders(ctx context.Context, userID, payeeID string) (*CustomerOrderSummary, error) {
	out := &CustomerOrderSummary{}
	err := s.db.WithContext(ctx).Model(&orderRow{}).Where("user_id=? AND payee_channel_org_id=?", userID, payeeID).Select("COUNT(*) AS count,COUNT(*) FILTER (WHERE status='paid') AS paid,COUNT(*) FILTER (WHERE status='pending') AS pending,COUNT(*) FILTER (WHERE status='refunded') AS refunded").Scan(out).Error
	return out, err
}

func (s *Service) CustomerOrdersInScope(ctx context.Context, userID, payeeID string) (bool, error) {
	var count int64
	err := s.db.WithContext(ctx).Model(&orderRow{}).Where("user_id=? AND payee_channel_org_id<>?", userID, payeeID).Count(&count).Error
	return count == 0, err
}
