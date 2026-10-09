package identity

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/tokpath/tokstudio/backend/internal/platform/crypto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strings"
	"time"
)

type ProfessionalCustomer struct {
	Role         *AcquisitionRoleView `json:"role"`
	Members      []CustomerView       `json:"members"`
	Codes        []ReferralLink       `json:"codes"`
	InvitedCount int64                `json:"invited_count"`
	ParentUserID string               `json:"parent_user_id"`
	ParentName   string               `json:"parent_name"`
}

func (s *Service) CreateProfessionalCustomerTx(tx *gorm.DB, p Principal, userID, typ, parentID string) (*AcquisitionRoleView, error) {
	scoped := *s
	scoped.db = tx
	user, err := scoped.GetCustomer(tx.Statement.Context, p, userID)
	if err != nil {
		return nil, err
	}
	for _, role := range user.Roles {
		if role != "end_user" {
			return nil, ErrAdminProtected
		}
	}
	owner, err := scoped.ResolvePaymentOwnerID(tx.Statement.Context, user.ChannelOrgID)
	if err != nil {
		return nil, err
	}
	if (p.IsPlatformAdmin() && owner != OfficialChannelID) || (!p.IsPlatformAdmin() && (!p.HasRole("channel_admin") || owner != p.ChannelOrgID)) {
		return nil, ErrChannelImmutable
	}
	var locked userRow
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND status='active'", userID).First(&locked).Error; err != nil {
		return nil, ErrPromotionInvalid
	}
	user, err = scoped.GetCustomer(tx.Statement.Context, p, userID)
	if err != nil {
		return nil, err
	}
	for _, role := range user.Roles {
		if role != "end_user" {
			return nil, ErrAdminProtected
		}
	}
	if typ != AcqAgent && typ != AcqKOL1 && typ != AcqKOL2 {
		return nil, ErrPromotionInvalid
	}
	parentID = strings.TrimSpace(parentID)
	if typ == AcqAgent {
		if parentID != "" {
			return nil, ErrPromotionInvalid
		}
	} else {
		var parent acquisitionRow
		if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).Where("id=? AND channel_org_id=? AND status='active'", parentID, user.ChannelOrgID).First(&parent).Error; err != nil {
			return nil, ErrPromotionInvalid
		}
		if (typ == AcqKOL1 && parent.Type != AcqAgent) || (typ == AcqKOL2 && parent.Type != AcqKOL1) {
			return nil, ErrPromotionInvalid
		}
	}
	// Repeated creation keeps the original professional membership; it never
	// changes qualification, fees, attribution or an existing promoter's facts.
	var existing acquisitionRow
	err = tx.Table("identity_acquisition_roles r").Select("r.*").Joins("JOIN identity_role_members m ON m.acquisition_role_id=r.id").Where("m.user_id=? AND r.type IN ?", userID, []string{AcqAgent, AcqKOL1, AcqKOL2}).First(&existing).Error
	if err == nil {
		if existing.Type == typ && deref(existing.ParentID) == parentID {
			return acqView(existing), nil
		}
		return nil, ErrPromotionInvalid
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	role, err := scoped.CreateAcquisitionRole(tx.Statement.Context, user.ChannelOrgID, typ, parentID)
	if err != nil {
		return nil, err
	}
	if err := scoped.BindRoleMember(tx.Statement.Context, userID, role.ID); err != nil {
		return nil, err
	}
	code := "THP" + strings.ToUpper(crypto.HashToken(role.ID)[:12])
	if _, err := scoped.CreatePromotionCode(tx.Statement.Context, user.ChannelOrgID, role.ID, code); err != nil {
		return nil, err
	}
	return role, nil
}

type ProfessionalChoice struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Type         string `json:"type"`
	ChannelOrgID string `json:"channel_org_id"`
}

type ProfessionalCustomerRow struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Email       string    `json:"email"`
	UserID      string    `json:"user_id"`
	Type        string    `json:"type"`
	Status      string    `json:"status"`
	ChannelName string    `json:"channel_name"`
	ParentName  string    `json:"parent_name"`
	CreatedAt   time.Time `json:"created_at"`
}
type ProfessionalCustomerPage struct {
	Items      []ProfessionalCustomerRow `json:"items"`
	Total      int64                     `json:"total"`
	NextCursor string                    `json:"next_cursor"`
}

func (s *Service) SearchProfessionalCustomers(ctx context.Context, p Principal, in CustomerQuery) (*ProfessionalCustomerPage, error) {
	ids, err := s.CustomerChannels(ctx, p, in.ChannelID)
	if err != nil {
		return nil, err
	}
	out := &ProfessionalCustomerPage{Items: []ProfessionalCustomerRow{}}
	q := s.db.WithContext(ctx).Table("identity_acquisition_roles r").Joins("JOIN identity_channel_orgs c ON c.id=r.channel_org_id").Joins("LEFT JOIN LATERAL (SELECT u.id,u.display_name,u.email FROM identity_role_members m JOIN identity_users u ON u.id=m.user_id WHERE m.acquisition_role_id=r.id ORDER BY m.created_at,m.user_id LIMIT 1) u ON true").Joins("LEFT JOIN LATERAL (SELECT u.display_name,u.email FROM identity_role_members m JOIN identity_users u ON u.id=m.user_id WHERE m.acquisition_role_id=r.parent_id ORDER BY m.created_at,m.user_id LIMIT 1) parent ON true").Where("r.channel_org_id IN ? AND r.type IN ?", ids, []string{AcqAgent, AcqKOL1, AcqKOL2})
	if in.Status != "" {
		q = q.Where("r.status=?", in.Status)
	}
	search := strings.TrimSpace(in.Q)
	if len(search) > 200 {
		return nil, ErrInvalidProfile
	}
	if search != "" {
		pattern := "%" + strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(search) + "%"
		q = q.Where("r.id=? OR u.email ILIKE ? OR u.display_name ILIKE ? OR c.code ILIKE ?", search, pattern, pattern, pattern)
	}
	if err := q.Count(&out.Total).Error; err != nil {
		return nil, err
	}
	scope := customerScopeHash(p, "professionals", in.ChannelID, search, in.Status)
	if in.Cursor != "" {
		raw, err := base64.RawURLEncoding.DecodeString(in.Cursor)
		var cursor customerCursor
		if err != nil || json.Unmarshal(raw, &cursor) != nil || cursor.Scope != scope || cursor.At.IsZero() || cursor.ID == "" {
			return nil, ErrCustomerCursor
		}
		q = q.Where("(r.created_at,r.id)<(?,?)", cursor.At, cursor.ID)
	}
	if err := q.Select("r.id,r.type,r.status,r.created_at,COALESCE(NULLIF(u.display_name,''),u.email,'') AS name,COALESCE(u.email,'') AS email,COALESCE(u.id,'') AS user_id,c.code AS channel_name,COALESCE(NULLIF(parent.display_name,''),parent.email,'') AS parent_name").Order("r.created_at DESC,r.id DESC").Limit(26).Scan(&out.Items).Error; err != nil {
		return nil, err
	}
	if len(out.Items) > 25 {
		out.Items = out.Items[:25]
		last := out.Items[24]
		raw, _ := json.Marshal(customerCursor{last.CreatedAt, last.ID, scope})
		out.NextCursor = base64.RawURLEncoding.EncodeToString(raw)
	}
	return out, nil
}

func (s *Service) ProfessionalChoices(ctx context.Context, p Principal, channelID, typ, search string) ([]ProfessionalChoice, error) {
	ids, err := s.CustomerChannels(ctx, p, channelID)
	if err != nil {
		return nil, err
	}
	out := []ProfessionalChoice{}
	q := s.db.WithContext(ctx).Table("identity_acquisition_roles r").Select("r.id,r.type,r.channel_org_id,COALESCE(NULLIF(u.display_name,''),NULLIF(u.email,''),'未绑定客户') AS name").Joins("LEFT JOIN LATERAL (SELECT u.email,u.display_name FROM identity_role_members m JOIN identity_users u ON u.id=m.user_id WHERE m.acquisition_role_id=r.id ORDER BY m.created_at,m.user_id LIMIT 1) u ON true").Where("r.channel_org_id IN ? AND r.type IN ? AND r.status='active'", ids, []string{AcqAgent, AcqKOL1, AcqKOL2})
	if typ != "" {
		q = q.Where("r.type=?", typ)
	}
	if strings.TrimSpace(search) != "" {
		pattern := "%" + strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(strings.TrimSpace(search)) + "%"
		q = q.Where("u.email ILIKE ? OR u.display_name ILIKE ? OR r.id=?", pattern, pattern, strings.TrimSpace(search))
	}
	err = q.Order("r.created_at DESC,r.id DESC").Limit(100).Scan(&out).Error
	return out, err
}
func (s *Service) ProfessionalCustomer(ctx context.Context, p Principal, roleID string) (*ProfessionalCustomer, error) {
	var row acquisitionRow
	if err := s.db.WithContext(ctx).Where("id=?", roleID).First(&row).Error; err != nil {
		return nil, mapNotFound(err)
	}
	role := acqView(row)
	if _, err := s.CustomerChannels(ctx, p, role.ChannelOrgID); err != nil {
		return nil, err
	}
	out := &ProfessionalCustomer{Role: role, Members: []CustomerView{}, Codes: []ReferralLink{}}
	var members []roleMemberRow
	if err := s.db.WithContext(ctx).Where("acquisition_role_id=?", roleID).Order("created_at,user_id").Find(&members).Error; err != nil {
		return nil, err
	}
	for _, member := range members {
		user, err := s.GetCustomer(ctx, p, member.UserID)
		if err != nil {
			return nil, err
		}
		out.Members = append(out.Members, *user)
		links, err := s.PersonalReferral(ctx, member.UserID)
		if err != nil {
			return nil, err
		}
		for _, link := range links.Links {
			for _, promo := range links.Codes {
				if promo.Code == link.Code && promo.AcquisitionRoleID == roleID {
					out.Codes = append(out.Codes, link)
				}
			}
		}
	}
	if err := s.db.WithContext(ctx).Model(&attributionRow{}).Where("acquisition_role_id=?", roleID).Count(&out.InvitedCount).Error; err != nil {
		return nil, err
	}
	if role.ParentID != "" {
		var parent struct {
			Email  string
			UserID string
		}
		if err := s.db.WithContext(ctx).Table("identity_role_members m").Select("u.email,u.id AS user_id").Joins("JOIN identity_users u ON u.id=m.user_id").Where("m.acquisition_role_id=?", role.ParentID).Order("m.created_at,m.user_id").Limit(1).Scan(&parent).Error; err != nil {
			return nil, err
		}
		out.ParentName = parent.Email
		out.ParentUserID = parent.UserID
	}
	return out, nil
}
