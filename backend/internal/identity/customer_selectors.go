package identity

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
)

type SubchannelPage struct {
	Items      []ChannelView `json:"items"`
	Total      int64         `json:"total"`
	NextCursor string        `json:"next_cursor"`
}

func (s *Service) SearchCustomerSubchannels(ctx context.Context, p Principal, search, cursor string) (*SubchannelPage, error) {
	owner, err := s.lookupChannel(ctx, p.ChannelOrgID)
	if err != nil || owner.Type != ChannelTypeC || !p.HasRole("channel_admin", "oem_ops", "oem_audit") {
		return nil, ErrChannelImmutable
	}
	search = strings.TrimSpace(search)
	if len(search) > 200 {
		return nil, ErrInvalidProfile
	}
	q := s.db.WithContext(ctx).Model(&channelRow{}).Where("parent_id=? AND type=?", owner.ID, ChannelTypeB)
	if search != "" {
		q = q.Where("code ILIKE ? OR id=?", "%"+strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(search)+"%", search)
	}
	out := &SubchannelPage{Items: []ChannelView{}}
	if err := q.Count(&out.Total).Error; err != nil {
		return nil, err
	}
	scope := customerScopeHash(p, "subchannels", owner.ID, search)
	if cursor != "" {
		raw, err := base64.RawURLEncoding.DecodeString(cursor)
		var value customerCursor
		if err != nil || json.Unmarshal(raw, &value) != nil || value.ID == "" || value.At.IsZero() || value.Scope != scope {
			return nil, ErrCustomerCursor
		}
		q = q.Where("(created_at,id)<(?,?)", value.At, value.ID)
	}
	var rows []channelRow
	if err := q.Order("created_at DESC,id DESC").Limit(26).Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) > 25 {
		rows = rows[:25]
		last := rows[24]
		raw, _ := json.Marshal(customerCursor{last.CreatedAt, last.ID, scope})
		out.NextCursor = base64.RawURLEncoding.EncodeToString(raw)
	}
	for _, row := range rows {
		out.Items = append(out.Items, channelViewFrom(row))
	}
	return out, nil
}

type CustomerPromotionChoice struct {
	Code         string `json:"code"`
	ChannelName  string `json:"channel_name"`
	CustomerName string `json:"customer_name"`
}

// The attribution selector has exactly the existing platform-owned write scope.
func (s *Service) CustomerPromotionChoices(ctx context.Context, p Principal, search string) ([]CustomerPromotionChoice, error) {
	if !p.IsPlatformAdmin() {
		return nil, ErrChannelImmutable
	}
	ids, err := s.BrandChannelIDs(ctx, OfficialChannelID)
	if err != nil {
		return nil, err
	}
	out := []CustomerPromotionChoice{}
	q := s.db.WithContext(ctx).Table("identity_promotion_codes p").Select("p.code,c.code AS channel_name,COALESCE(NULLIF(u.display_name,''),u.email,'') AS customer_name").Joins("JOIN identity_channel_orgs c ON c.id=p.channel_org_id").Joins("LEFT JOIN identity_acquisition_roles r ON r.id=p.acquisition_role_id").Joins("LEFT JOIN LATERAL (SELECT u.email,u.display_name FROM identity_role_members m JOIN identity_users u ON u.id=m.user_id WHERE m.acquisition_role_id=r.id ORDER BY m.created_at,m.user_id LIMIT 1) u ON true").Where("p.channel_org_id IN ? AND c.status='active' AND p.status='active' AND (p.acquisition_role_id IS NULL OR (r.status='active' AND r.channel_org_id=p.channel_org_id))", ids)
	search = strings.TrimSpace(search)
	if len(search) > 200 {
		return nil, ErrInvalidProfile
	}
	if search != "" {
		pattern := "%" + strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(search) + "%"
		q = q.Where("p.code ILIKE ? OR c.code ILIKE ? OR u.email ILIKE ? OR u.display_name ILIKE ?", pattern, pattern, pattern, pattern)
	}
	err = q.Order("c.code,p.code").Limit(100).Scan(&out).Error
	return out, err
}
