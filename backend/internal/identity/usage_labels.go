package identity

import "context"

// APIKeyLabels returns display names only. Callers must first scope the usage
// records that supplied these IDs; it never reveals key material or limits.
func (s *Service) APIKeyLabels(ctx context.Context, ids []string) (map[string]string, error) {
	out := map[string]string{}
	if len(ids) == 0 {
		return out, nil
	}
	var rows []struct{ ID, Name string }
	if err := s.db.WithContext(ctx).Model(&apiKeyRow{}).Select("id,name").Where("id IN ?", ids).Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.ID] = row.Name
	}
	return out, nil
}
