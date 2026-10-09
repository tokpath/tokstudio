package identity

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/tokpath/tokstudio/backend/internal/platform/crypto"
	"gorm.io/gorm"
	"sort"
	"strings"
	"time"
)

var ErrCustomerCursor = errors.New("invalid customer cursor")

type CustomerQuery struct {
	Q, Status, ChannelID, BrandID, Cursor string
	Limit                                 int
}

// CustomerView contains no password, token, OAuth identifier or login methods.
type CustomerView struct {
	ID                 string    `json:"id"`
	Email              string    `json:"email"`
	DisplayName        string    `json:"display_name"`
	Status             string    `json:"status"`
	ChannelOrgID       string    `json:"channel_org_id"`
	ChannelCode        string    `json:"channel_code"`
	BrandID            string    `json:"brand_id"`
	BrandName          string    `json:"brand_name"`
	SourceCode         string    `json:"source_code,omitempty"`
	AcquisitionRoleID  string    `json:"acquisition_role_id,omitempty"`
	ProfessionalRoleID string    `json:"professional_role_id,omitempty"`
	Roles              []string  `json:"roles,omitempty" gorm:"-"`
	CreatedAt          time.Time `json:"created_at"`
}
type CustomerPage struct {
	Items      []CustomerView `json:"items"`
	Total      int64          `json:"total"`
	Limit      int            `json:"limit"`
	NextCursor string         `json:"next_cursor"`
}
type CustomerScope struct {
	ID                    string `json:"id"`
	Code                  string `json:"code"`
	BrandID               string `json:"brand_id"`
	BrandName             string `json:"brand_name"`
	CanCreateProfessional bool   `json:"can_create_professional"`
}

func (s *Service) CustomerScopes(ctx context.Context, p Principal) ([]CustomerScope, error) {
	ids, err := s.CustomerChannels(ctx, p, "")
	if err != nil {
		return nil, err
	}
	out := []CustomerScope{}
	err = s.db.WithContext(ctx).Table("identity_channel_orgs c").Select("c.id,c.code,c.brand_id,b.name AS brand_name").Joins("JOIN identity_brands b ON b.id=c.brand_id").Where("c.id IN ?", ids).Order("b.name,c.code,c.id").Scan(&out).Error
	for i := range out {
		owner, e := s.ResolvePaymentOwnerID(ctx, out[i].ID)
		out[i].CanCreateProfessional = e == nil && ((p.IsPlatformAdmin() && owner == OfficialChannelID) || (p.HasRole("channel_admin") && owner == p.ChannelOrgID))
	}
	return out, err
}

func customerScopeHash(p Principal, parts ...string) string {
	roles := append([]string{}, p.Roles...)
	sort.Strings(roles)
	return crypto.HashToken(p.UserID + "\n" + strings.Join(roles, ",") + "\n" + p.ChannelOrgID + "\n" + strings.Join(parts, "\n"))
}

type customerCursor struct {
	At    time.Time `json:"at"`
	ID    string    `json:"id"`
	Scope string    `json:"scope"`
}

// CustomerChannels enforces persisted ownership for every query, including
// details and selectors. Client supplied scope can only narrow that ownership.
func (s *Service) CustomerChannels(ctx context.Context, p Principal, selected string) ([]string, error) {
	var ids []string
	if p.IsPlatformAdmin() {
		if err := s.db.WithContext(ctx).Model(&channelRow{}).Pluck("id", &ids).Error; err != nil {
			return nil, err
		}
		ids = append(ids, "")
	} else if p.HasRole("finance_admin", "ops_admin", "audit_readonly") {
		var err error
		ids, err = s.BrandChannelIDs(ctx, OfficialChannelID)
		if err != nil {
			return nil, err
		}
	} else if p.IsChannelStaff() {
		ch, err := s.lookupChannel(ctx, p.ChannelOrgID)
		if err != nil {
			return nil, err
		}
		if ch.Type == ChannelTypeC {
			ids, err = s.BrandChannelIDs(ctx, ch.ID)
			if err != nil {
				return nil, err
			}
		} else if ch.Type == ChannelTypeB {
			ids = []string{ch.ID}
		} else {
			return nil, ErrChannelImmutable
		}
	} else {
		return nil, ErrChannelImmutable
	}
	if selected != "" {
		for _, id := range ids {
			if id == selected {
				return []string{id}, nil
			}
		}
		return nil, ErrChannelImmutable
	}
	return ids, nil
}
func customerOperations(p Principal) bool {
	return p.HasRole("platform_admin", "ops_admin", "channel_admin", "oem_ops", "audit_readonly", "oem_audit")
}
func (s *Service) customerQuery(ctx context.Context, p Principal, in CustomerQuery) (*gorm.DB, error) {
	ids, err := s.CustomerChannels(ctx, p, in.ChannelID)
	if err != nil {
		return nil, err
	}
	q := s.db.WithContext(ctx).Table("identity_users u").Joins("LEFT JOIN identity_channel_orgs c ON c.id=u.channel_org_id").Joins("LEFT JOIN identity_brands b ON b.id=u.brand_id").Joins("LEFT JOIN identity_attributions a ON a.user_id=u.id").
		Where("u.id NOT IN (SELECT user_id FROM identity_staff_members)").Where("COALESCE(u.channel_org_id, '') IN ?", ids)
	if in.BrandID != "" {
		q = q.Where("u.brand_id = ?", in.BrandID)
	}
	if in.Status != "" {
		q = q.Where("u.status = ?", in.Status)
	}
	if text := strings.TrimSpace(in.Q); text != "" {
		if len(text) > 200 {
			return nil, ErrInvalidProfile
		}
		pattern := "%" + strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(text) + "%"
		if customerOperations(p) {
			q = q.Where("u.id = ? OR u.email ILIKE ? OR u.display_name ILIKE ? OR c.code ILIKE ? OR a.source_code ILIKE ?", text, pattern, pattern, pattern, pattern)
		} else {
			q = q.Where("u.id = ? OR u.email ILIKE ? OR u.display_name ILIKE ? OR c.code ILIKE ?", text, pattern, pattern, pattern)
		}
	}
	return q, nil
}
func customerSelect(operations bool) string {
	base := "u.id,u.email,u.display_name,u.status,COALESCE(u.channel_org_id,'') AS channel_org_id,COALESCE(c.code,'') AS channel_code,COALESCE(u.brand_id,'') AS brand_id,COALESCE(b.name,'') AS brand_name,u.created_at"
	if operations {
		base += ",COALESCE(a.source_code,'') AS source_code,COALESCE(a.acquisition_role_id,'') AS acquisition_role_id"
	}
	return base
}
func (s *Service) SearchCustomers(ctx context.Context, p Principal, in CustomerQuery) (*CustomerPage, error) {
	if in.Limit < 1 {
		in.Limit = 25
	}
	if in.Limit > 100 {
		in.Limit = 100
	}
	q, err := s.customerQuery(ctx, p, in)
	if err != nil {
		return nil, err
	}
	out := &CustomerPage{Items: []CustomerView{}, Limit: in.Limit}
	if err := q.Count(&out.Total).Error; err != nil {
		return nil, err
	}
	scope := customerScopeHash(p, "customers", in.ChannelID, in.BrandID, strings.TrimSpace(in.Q), in.Status)
	if in.Cursor != "" {
		raw, err := base64.RawURLEncoding.DecodeString(in.Cursor)
		var cursor customerCursor
		if err != nil || json.Unmarshal(raw, &cursor) != nil || cursor.ID == "" || cursor.At.IsZero() || cursor.Scope != scope {
			return nil, ErrCustomerCursor
		}
		q = q.Where("(u.created_at,u.id) < (?,?)", cursor.At, cursor.ID)
	}
	if err := q.Select(customerSelect(customerOperations(p))).Order("u.created_at DESC,u.id DESC").Limit(in.Limit + 1).Scan(&out.Items).Error; err != nil {
		return nil, err
	}
	if len(out.Items) > in.Limit {
		out.Items = out.Items[:in.Limit]
		last := out.Items[len(out.Items)-1]
		raw, _ := json.Marshal(customerCursor{last.CreatedAt, last.ID, scope})
		out.NextCursor = base64.RawURLEncoding.EncodeToString(raw)
	}
	if customerOperations(p) {
		if err := s.customerRoles(ctx, out.Items); err != nil {
			return nil, err
		}
	}
	return out, nil
}
func (s *Service) customerRoles(ctx context.Context, items []CustomerView) error {
	if len(items) == 0 {
		return nil
	}
	ids := make([]string, 0, len(items))
	for _, u := range items {
		ids = append(ids, u.ID)
	}
	var rows []struct {
		UserID string
		Code   string
	}
	if err := s.db.WithContext(ctx).Table("identity_user_roles ur").Select("ur.user_id,r.code").Joins("JOIN identity_roles r ON r.id=ur.role_id").Where("ur.user_id IN ?", ids).Scan(&rows).Error; err != nil {
		return err
	}
	roles := map[string][]string{}
	for _, row := range rows {
		roles[row.UserID] = append(roles[row.UserID], row.Code)
	}
	for i := range items {
		items[i].Roles = roles[items[i].ID]
	}
	var professional []struct {
		UserID string
		RoleID string
	}
	if err := s.db.WithContext(ctx).Table("identity_role_members m").Select("m.user_id,r.id AS role_id").Joins("JOIN identity_acquisition_roles r ON r.id=m.acquisition_role_id").Where("m.user_id IN ? AND r.type IN ?", ids, []string{AcqAgent, AcqKOL1, AcqKOL2}).Order("m.created_at,m.acquisition_role_id").Scan(&professional).Error; err != nil {
		return err
	}
	for i := range items {
		for _, row := range professional {
			if row.UserID == items[i].ID {
				items[i].ProfessionalRoleID = row.RoleID
				break
			}
		}
	}
	return nil
}
func (s *Service) GetCustomer(ctx context.Context, p Principal, id string) (*CustomerView, error) {
	q, err := s.customerQuery(ctx, p, CustomerQuery{})
	if err != nil {
		return nil, err
	}
	var items []CustomerView
	if err := q.Select(customerSelect(customerOperations(p))).Where("u.id=?", id).Limit(1).Scan(&items).Error; err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, ErrNotFound
	}
	if customerOperations(p) {
		if err := s.customerRoles(ctx, items); err != nil {
			return nil, err
		}
	}
	return &items[0], nil
}
