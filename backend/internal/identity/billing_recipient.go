package identity

import (
	"context"
	"strings"
)

// BillingRecipient deliberately excludes login methods, roles and attribution history.
type BillingRecipient struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	ChannelCode string `json:"channel_code"`
	Status      string `json:"status"`
}

func (s *Service) SearchBillingRecipients(ctx context.Context, query string) ([]BillingRecipient, error) {
	query = strings.TrimSpace(query)
	out := []BillingRecipient{}
	if len([]rune(query)) < 2 || len(query) > 200 {
		return out, nil
	}
	// Literal substring search: '%' and '_' must not enumerate all users.
	pattern := "%" + strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(query) + "%"
	err := s.db.WithContext(ctx).Table("identity_users AS u").
		Select("u.id, u.email, u.display_name, u.status, COALESCE(c.code, '') AS channel_code").
		Joins("LEFT JOIN identity_channel_orgs AS c ON c.id = u.channel_org_id").
		Where("u.id = ? OR u.email ILIKE ? OR u.display_name ILIKE ?", query, pattern, pattern).
		Order("u.email ASC, u.id ASC").Limit(20).Scan(&out).Error
	return out, err
}

// MatchBillingUserIDs is used only by role-protected administrative order search.
func (s *Service) MatchBillingUserIDs(ctx context.Context, query string) ([]string, error) {
	ids := []string{}
	pattern := "%" + strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(strings.TrimSpace(query)) + "%"
	err := s.db.WithContext(ctx).Model(&userRow{}).Where("email ILIKE ? OR display_name ILIKE ?", pattern, pattern).Pluck("id", &ids).Error
	return ids, err
}

func (s *Service) BillingRecipientsByID(ctx context.Context, ids []string) (map[string]BillingRecipient, error) {
	out := map[string]BillingRecipient{}
	if len(ids) == 0 {
		return out, nil
	}
	var rows []BillingRecipient
	err := s.db.WithContext(ctx).Table("identity_users AS u").
		Select("u.id, u.email, u.display_name, u.status, COALESCE(c.code, '') AS channel_code").
		Joins("LEFT JOIN identity_channel_orgs AS c ON c.id = u.channel_org_id").Where("u.id IN ?", ids).Scan(&rows).Error
	for _, row := range rows {
		out[row.ID] = row
	}
	return out, err
}

func (s *Service) BillingChannelCodes(ctx context.Context, ids []string) (map[string]string, error) {
	out := map[string]string{}
	if len(ids) == 0 {
		return out, nil
	}
	var rows []channelRow
	err := s.db.WithContext(ctx).Where("id IN ?", ids).Find(&rows).Error
	for _, row := range rows {
		out[row.ID] = row.Code
	}
	return out, err
}
