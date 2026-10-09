package billing

import "context"

type UsageFacets struct {
	Keys   []string `json:"keys"`
	Models []string `json:"models"`
}

// Key and model options retain the same authorized customer/date/state scope,
// but selecting one option does not remove the other choices in that facet.
func (s *Service) UsageFacets(ctx context.Context, in QueryUsageInput) (*UsageFacets, error) {
	in.APIKeyID = ""
	in.PublicModelID = ""
	in.Cursor = ""
	in.RequestID = ""
	out := &UsageFacets{Keys: []string{}, Models: []string{}}
	for _, dim := range []struct {
		column string
		target *[]string
	}{{"api_key_id", &out.Keys}, {"public_model_id", &out.Models}} {
		q := applyUsageFilters(s.db.WithContext(ctx).Model(&usageRow{}), in)
		if in.State != "" {
			q = q.Where("state = ?", in.State)
		}
		if err := q.Distinct(dim.column).Where(dim.column+" IS NOT NULL AND "+dim.column+" <> ''").Order(dim.column).Pluck(dim.column, dim.target).Error; err != nil {
			return nil, err
		}
	}
	return out, nil
}
