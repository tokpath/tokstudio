package billing

import "context"

// ListPoolAllocations reads original source-pool facts, never the customer's
// current brand or an attribution-wide account.
func (s *Service) ListPoolAllocations(ctx context.Context, ownerID, attribution string) ([]AllocationView, error) {
	if ownerID == "" {
		return nil, ErrInvalidAmount
	}
	q := s.db.WithContext(ctx).Where("pool_channel_org_id=?", ownerID).Order("created_at DESC,id DESC")
	if attribution != "" {
		q = q.Where("channel_org_id=?", attribution)
	}
	var rows []allocationRow
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]AllocationView, 0, len(rows))
	for _, row := range rows {
		out = append(out, allocationView(row))
	}
	return out, nil
}
