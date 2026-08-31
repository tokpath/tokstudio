package identity

import (
	"context"
	"strings"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/platform/id"
)

const (
	AcqAgent = "agent"
	AcqKOL1  = "kol_l1"
	AcqKOL2  = "kol_l2"

	AgentBRoleID = "acr_b_agent"
	KOL1BRoleID  = "acr_b_kol1"
	KOL2BRoleID  = "acr_b_kol2"
	PromoAgentB  = "THB-AGENT"
	PromoKOL1B   = "THB-KOL1"
	PromoKOL2B   = "THB-KOL2"
)

type acquisitionRow struct {
	ID           string    `gorm:"column:id;primaryKey"`
	ChannelOrgID string    `gorm:"column:channel_org_id"`
	Type         string    `gorm:"column:type"`
	ParentID     *string   `gorm:"column:parent_id"`
	Level        int       `gorm:"column:level"`
	Status       string    `gorm:"column:status"`
	CreatedAt    time.Time `gorm:"column:created_at"`
}

func (acquisitionRow) TableName() string { return "identity_acquisition_roles" }

type roleMemberRow struct {
	UserID            string    `gorm:"column:user_id;primaryKey"`
	AcquisitionRoleID string    `gorm:"column:acquisition_role_id;primaryKey"`
	CreatedAt         time.Time `gorm:"column:created_at"`
}

func (roleMemberRow) TableName() string { return "identity_role_members" }

type AcquisitionRoleView struct {
	ID           string `json:"id"`
	ChannelOrgID string `json:"channel_org_id"`
	Type         string `json:"type"`
	ParentID     string `json:"parent_id,omitempty"`
	Level        int    `json:"level"`
	Status       string `json:"status"`
}

type AttributionView struct {
	UserID            string `json:"user_id"`
	ChannelOrgID      string `json:"channel_org_id"`
	AcquisitionRoleID string `json:"acquisition_role_id,omitempty"`
	RoleType          string `json:"role_type,omitempty"`
	ParentRoleID      string `json:"parent_role_id,omitempty"`
	SourceCode        string `json:"source_code"`
}

// ChannelAttributionBucket 是渠道控制台的归因汇总：按推广码和角色点数，不含其他渠道。
type ChannelAttributionBucket struct {
	ChannelOrgID      string `json:"channel_org_id"`
	SourceCode        string `json:"source_code"`
	AcquisitionRoleID string `json:"acquisition_role_id,omitempty"`
	RoleType          string `json:"role_type,omitempty"`
	UserCount         int64  `json:"user_count"`
}

type PromotionView struct {
	ID                string `json:"id"`
	Code              string `json:"code"`
	ChannelOrgID      string `json:"channel_org_id"`
	AcquisitionRoleID string `json:"acquisition_role_id,omitempty"`
	Status            string `json:"status"`
}

func (s *Service) CreateAcquisitionRole(ctx context.Context, channelID, typ, parentID string) (*AcquisitionRoleView, error) {
	if channelID == "" || !validAcqType(typ) {
		return nil, ErrPromotionInvalid
	}
	level := acqLevel(typ)
	if typ != AcqAgent && parentID == "" {
		return nil, ErrPromotionInvalid
	}
	if typ == AcqKOL2 && parentID != "" {
		var parent acquisitionRow
		if err := s.db.WithContext(ctx).Where("id = ?", parentID).First(&parent).Error; err != nil {
			return nil, ErrPromotionInvalid
		}
		if parent.Type != AcqKOL1 {
			return nil, ErrPromotionInvalid
		}
	}
	now := time.Now().UTC()
	row := acquisitionRow{
		ID: id.New("acr"), ChannelOrgID: channelID, Type: typ, Level: level, Status: "active", CreatedAt: now,
	}
	if parentID != "" {
		row.ParentID = &parentID
	}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return nil, err
	}
	return acqView(row), nil
}

func (s *Service) CreatePromotionCode(ctx context.Context, channelID, roleID, code string) (*PromotionView, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	if channelID == "" || code == "" {
		return nil, ErrPromotionInvalid
	}
	row := promotionRow{ID: id.New("prm"), Code: code, ChannelOrgID: channelID, Status: "active"}
	if roleID != "" {
		row.AcquisitionRoleID = &roleID
	}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return nil, ErrPromotionInvalid
	}
	return promoView(row), nil
}

func (s *Service) ListAcquisitionRoles(ctx context.Context, channelID, listType string) ([]AcquisitionRoleView, error) {
	var rows []acquisitionRow
	q := s.db.WithContext(ctx).Order("level, created_at")
	if channelID != "" {
		q = q.Where("channel_org_id = ?", channelID)
	}
	if types := AcqTypeFilter(listType); len(types) == 1 {
		q = q.Where("type = ?", types[0])
	} else if len(types) > 1 {
		q = q.Where("type IN ?", types)
	}
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]AcquisitionRoleView, 0, len(rows))
	for _, row := range rows {
		out = append(out, *acqView(row))
	}
	return out, nil
}

func (s *Service) GetAcquisitionRole(ctx context.Context, viewer Principal, roleID string) (*AcquisitionRoleView, error) {
	var row acquisitionRow
	if err := s.db.WithContext(ctx).Where("id = ?", roleID).First(&row).Error; err != nil {
		return nil, mapNotFound(err)
	}
	if scoped := viewer.VisibleChannelID(); scoped != "" && scoped != row.ChannelOrgID {
		return nil, ErrChannelImmutable
	}
	return acqView(row), nil
}

func (s *Service) PatchAcquisitionRole(ctx context.Context, viewer Principal, roleID, status string) (*AcquisitionRoleView, error) {
	item, err := s.GetAcquisitionRole(ctx, viewer, roleID)
	if err != nil {
		return nil, err
	}
	status = strings.TrimSpace(status)
	if status == "" {
		return item, nil
	}
	if status != "active" && status != "disabled" {
		return nil, ErrPromotionInvalid
	}
	if err := s.db.WithContext(ctx).Model(&acquisitionRow{}).Where("id = ?", roleID).Update("status", status).Error; err != nil {
		return nil, err
	}
	return s.GetAcquisitionRole(ctx, viewer, roleID)
}

func (s *Service) ListPromotionCodes(ctx context.Context, channelID string) ([]PromotionView, error) {
	var rows []promotionRow
	q := s.db.WithContext(ctx).Order("code")
	if channelID != "" {
		q = q.Where("channel_org_id = ?", channelID)
	}
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]PromotionView, 0, len(rows))
	for _, row := range rows {
		out = append(out, *promoView(row))
	}
	return out, nil
}

func (s *Service) BindRoleMember(ctx context.Context, userID, roleID string) error {
	return s.db.WithContext(ctx).Where("user_id = ? AND acquisition_role_id = ?", userID, roleID).
		FirstOrCreate(&roleMemberRow{UserID: userID, AcquisitionRoleID: roleID, CreatedAt: time.Now().UTC()}).Error
}

func (s *Service) MemberRole(ctx context.Context, userID string) (*AcquisitionRoleView, error) {
	var mem roleMemberRow
	if err := s.db.WithContext(ctx).Where("user_id = ?", userID).First(&mem).Error; err != nil {
		return nil, err
	}
	var row acquisitionRow
	if err := s.db.WithContext(ctx).Where("id = ?", mem.AcquisitionRoleID).First(&row).Error; err != nil {
		return nil, err
	}
	return acqView(row), nil
}

func (s *Service) GetAttribution(ctx context.Context, userID string) (*AttributionView, error) {
	var attr attributionRow
	if err := s.db.WithContext(ctx).Where("user_id = ?", userID).First(&attr).Error; err != nil {
		return nil, err
	}
	view := &AttributionView{
		UserID: userID, ChannelOrgID: attr.ChannelOrgID, SourceCode: attr.SourceCode,
	}
	if attr.AcquisitionRoleID != nil {
		view.AcquisitionRoleID = *attr.AcquisitionRoleID
		var role acquisitionRow
		if err := s.db.WithContext(ctx).Where("id = ?", *attr.AcquisitionRoleID).First(&role).Error; err == nil {
			view.RoleType = role.Type
			if role.ParentID != nil {
				view.ParentRoleID = *role.ParentID
			}
		}
	}
	return view, nil
}

func (s *Service) ListChannelAttribution(ctx context.Context, viewer Principal) ([]ChannelAttributionBucket, error) {
	q := s.db.WithContext(ctx).Model(&attributionRow{})
	if channelID := viewer.VisibleChannelID(); channelID != "" {
		q = q.Where("channel_org_id = ?", channelID)
	}
	var rows []attributionRow
	if err := q.Order("source_code").Find(&rows).Error; err != nil {
		return nil, err
	}
	roleIDs := make([]string, 0, len(rows))
	for _, row := range rows {
		if row.AcquisitionRoleID != nil && *row.AcquisitionRoleID != "" {
			roleIDs = append(roleIDs, *row.AcquisitionRoleID)
		}
	}
	roleTypes := map[string]string{}
	if len(roleIDs) > 0 {
		var roles []acquisitionRow
		if err := s.db.WithContext(ctx).Where("id IN ?", roleIDs).Find(&roles).Error; err != nil {
			return nil, err
		}
		for _, role := range roles {
			roleTypes[role.ID] = role.Type
		}
	}
	type key struct{ channel, source, role string }
	counts := map[key]int64{}
	order := make([]key, 0)
	for _, row := range rows {
		roleID := ""
		if row.AcquisitionRoleID != nil {
			roleID = *row.AcquisitionRoleID
		}
		k := key{row.ChannelOrgID, row.SourceCode, roleID}
		if _, seen := counts[k]; !seen {
			order = append(order, k)
		}
		counts[k]++
	}
	out := make([]ChannelAttributionBucket, 0, len(order))
	for _, k := range order {
		out = append(out, ChannelAttributionBucket{
			ChannelOrgID:      k.channel,
			SourceCode:        k.source,
			AcquisitionRoleID: k.role,
			RoleType:          roleTypes[k.role],
			UserCount:         counts[k],
		})
	}
	return out, nil
}

func (s *Service) MapUserAcquisitionRoles(ctx context.Context, userIDs []string) (map[string]string, error) {
	out := map[string]string{}
	if len(userIDs) == 0 {
		return out, nil
	}
	var rows []attributionRow
	if err := s.db.WithContext(ctx).Where("user_id IN ?", userIDs).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		if row.AcquisitionRoleID != nil && *row.AcquisitionRoleID != "" {
			out[row.UserID] = *row.AcquisitionRoleID
		}
	}
	return out, nil
}

func (s *Service) RoleIDsInScope(ctx context.Context, roleID string) ([]string, error) {
	var root acquisitionRow
	if err := s.db.WithContext(ctx).Where("id = ?", roleID).First(&root).Error; err != nil {
		return nil, err
	}
	ids := []string{root.ID}
	if root.Type == AcqKOL2 {
		return ids, nil
	}
	var children []acquisitionRow
	if err := s.db.WithContext(ctx).Where("parent_id = ?", root.ID).Find(&children).Error; err != nil {
		return nil, err
	}
	for _, child := range children {
		ids = append(ids, child.ID)
		if child.Type == AcqKOL1 {
			var grand []acquisitionRow
			if err := s.db.WithContext(ctx).Where("parent_id = ?", child.ID).Find(&grand).Error; err != nil {
				return nil, err
			}
			for _, g := range grand {
				ids = append(ids, g.ID)
			}
		}
	}
	return ids, nil
}

func (s *Service) ListScopedUsers(ctx context.Context, viewer Principal, maskEmail bool) ([]UserView, error) {
	q := s.db.WithContext(ctx).Model(&userRow{})
	if channelID := viewer.VisibleChannelID(); channelID != "" {
		q = q.Where("channel_org_id = ?", channelID)
	}
	roleIDs := []string{}
	if !viewer.IsPlatformAdmin() && !viewer.HasRole("channel_admin", "finance_admin", "ops_admin") {
		mem, err := s.MemberRole(ctx, viewer.UserID)
		if err != nil {
			return nil, ErrChannelImmutable
		}
		roleIDs, err = s.RoleIDsInScope(ctx, mem.ID)
		if err != nil {
			return nil, err
		}
	}
	var rows []userRow
	if err := q.Order("created_at DESC").Limit(200).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]UserView, 0, len(rows))
	for _, row := range rows {
		var attr attributionRow
		_ = s.db.WithContext(ctx).Where("user_id = ?", row.ID).First(&attr).Error
		if len(roleIDs) > 0 {
			if attr.AcquisitionRoleID == nil || !contains(roleIDs, *attr.AcquisitionRoleID) {
				continue
			}
		}
		email := row.Email
		if maskEmail {
			email = MaskEmail(email)
		}
		view := viewFromUser(row, nil, attr.SourceCode)
		view.Email = email
		out = append(out, view)
	}
	return out, nil
}

func MaskEmail(email string) string {
	parts := strings.Split(email, "@")
	if len(parts) != 2 || parts[0] == "" {
		return "***"
	}
	local := parts[0]
	if len(local) == 1 {
		return local[:1] + "***@" + parts[1]
	}
	return local[:1] + "***@" + parts[1]
}

func validAcqType(t string) bool {
	return t == AcqAgent || t == AcqKOL1 || t == AcqKOL2
}

// AcqTypeFilter 把列表类型收成 SQL type 条件。空表示不过滤；kol 覆盖 1 级和 2 级。
func AcqTypeFilter(listType string) []string {
	switch strings.TrimSpace(listType) {
	case "":
		return nil
	case "kol":
		return []string{AcqKOL1, AcqKOL2}
	default:
		return []string{listType}
	}
}

func acqLevel(t string) int {
	switch t {
	case AcqKOL1:
		return 1
	case AcqKOL2:
		return 2
	default:
		return 0
	}
}

func acqView(row acquisitionRow) *AcquisitionRoleView {
	view := &AcquisitionRoleView{
		ID: row.ID, ChannelOrgID: row.ChannelOrgID, Type: row.Type, Level: row.Level, Status: row.Status,
	}
	if row.ParentID != nil {
		view.ParentID = *row.ParentID
	}
	return view
}

func promoView(row promotionRow) *PromotionView {
	view := &PromotionView{ID: row.ID, Code: row.Code, ChannelOrgID: row.ChannelOrgID, Status: row.Status}
	if row.AcquisitionRoleID != nil {
		view.AcquisitionRoleID = *row.AcquisitionRoleID
	}
	return view
}

func contains(ids []string, id string) bool {
	for _, item := range ids {
		if item == id {
			return true
		}
	}
	return false
}
