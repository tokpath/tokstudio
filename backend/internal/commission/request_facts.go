package commission

import "context"

// RequestFacts returns the original commission facts for an already authorized
// request. The caller must authorize the request and its brand before reading.
func (s *Service) RequestFacts(ctx context.Context, requestID string) ([]EntryView, error) {
	var rows []entryRow
	if err := s.db.WithContext(ctx).Where("request_id = ?", requestID).Order("created_at ASC, id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]EntryView, 0, len(rows))
	for _, row := range rows {
		out = append(out, *entryView(row))
	}
	return out, nil
}
