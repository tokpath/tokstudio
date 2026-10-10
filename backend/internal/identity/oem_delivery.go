package identity

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/tokpath/tokstudio/backend/internal/platform/id"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strings"
	"time"
)

var ErrDeliveryConflict = errors.New("delivery changed or operation conflicts")
var ErrDeliveryIncomplete = errors.New("delivery evidence incomplete")

type OEMCreateInput struct {
	OperationID string      `json:"operation_id"`
	BrandID     string      `json:"brand_id"`
	Brand       *BrandInput `json:"brand,omitempty"`
	SalesMode   string      `json:"sales_mode"`
	SellPlans   bool        `json:"sell_plans"`
}
type deliveryRow struct {
	ChannelOrgID        string     `gorm:"column:channel_org_id;primaryKey"`
	OperationID         string     `gorm:"column:operation_id"`
	CreatedBy           string     `gorm:"column:created_by"`
	InputJSON           []byte     `gorm:"column:input_json"`
	SalesMode           string     `gorm:"column:sales_mode"`
	SellPlans           bool       `gorm:"column:sell_plans"`
	Phase               string     `gorm:"column:phase"`
	Version             int64      `gorm:"column:version"`
	DomainEvidenceJSON  []byte     `gorm:"column:domain_evidence_json"`
	ReceiverUserID      *string    `gorm:"column:receiver_user_id"`
	HandedOverAt        *time.Time `gorm:"column:handed_over_at"`
	HandoffEvidenceJSON []byte     `gorm:"column:handoff_evidence_json"`
	CreatedAt           time.Time  `gorm:"column:created_at"`
	UpdatedAt           time.Time  `gorm:"column:updated_at"`
}

func (deliveryRow) TableName() string { return "identity_oem_deliveries" }

type DeliveryView struct {
	ChannelOrgID    string         `json:"channel_org_id"`
	SalesMode       string         `json:"sales_mode"`
	SellPlans       bool           `json:"sell_plans"`
	Phase           string         `json:"phase"`
	Version         int64          `json:"version"`
	DomainEvidence  map[string]any `json:"domain_evidence"`
	ReceiverUserID  string         `json:"receiver_user_id,omitempty"`
	HandedOverAt    *time.Time     `json:"handed_over_at,omitempty"`
	HandoffEvidence map[string]any `json:"handoff_evidence"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
}

func deliveryView(row deliveryRow) *DeliveryView {
	view := &DeliveryView{ChannelOrgID: row.ChannelOrgID, SalesMode: row.SalesMode, SellPlans: row.SellPlans, Phase: row.Phase, Version: row.Version, DomainEvidence: map[string]any{}, HandoffEvidence: map[string]any{}, HandedOverAt: row.HandedOverAt, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
	_ = json.Unmarshal(row.DomainEvidenceJSON, &view.DomainEvidence)
	_ = json.Unmarshal(row.HandoffEvidenceJSON, &view.HandoffEvidence)
	if row.ReceiverUserID != nil {
		view.ReceiverUserID = *row.ReceiverUserID
	}
	return view
}
func (s *Service) CreateOEM(ctx context.Context, actor Principal, in OEMCreateInput) (*DeliveryView, error) {
	if !actor.IsPlatformAdmin() {
		return nil, ErrChannelImmutable
	}
	in.OperationID = strings.TrimSpace(in.OperationID)
	in.BrandID = strings.TrimSpace(in.BrandID)
	if in.OperationID == "" || (in.BrandID == "") == (in.Brand == nil) {
		return nil, ErrPromotionInvalid
	}
	if in.SalesMode == "" {
		in.SalesMode = "offline"
	}
	if in.SalesMode != "offline" && in.SalesMode != "online" {
		return nil, ErrPromotionInvalid
	}
	payload, err := json.Marshal(in)
	if err != nil {
		return nil, err
	}
	var out *DeliveryView
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", "oem-operation:"+in.OperationID).Error; err != nil {
			return err
		}
		var existing deliveryRow
		err := tx.Where("operation_id = ?", in.OperationID).First(&existing).Error
		if err == nil {
			var old, newPayload any
			_ = json.Unmarshal(existing.InputJSON, &old)
			_ = json.Unmarshal(payload, &newPayload)
			a, _ := json.Marshal(old)
			b, _ := json.Marshal(newPayload)
			if existing.CreatedBy != actor.UserID || string(a) != string(b) {
				return ErrDeliveryConflict
			}
			out = deliveryView(existing)
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		service := *s
		service.db = tx
		brandID := in.BrandID
		if in.Brand != nil {
			brand, err := service.CreateBrand(ctx, actor, *in.Brand)
			if err != nil {
				return err
			}
			brandID = brand.ID
		}
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", "oem-brand:"+brandID).Error; err != nil {
			return err
		}
		channel, err := service.CreateChannel(ctx, actor, ChannelInput{Code: id.New("oem"), Type: ChannelTypeC, BrandID: brandID, Status: "active"})
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		row := deliveryRow{ChannelOrgID: channel.ID, OperationID: in.OperationID, CreatedBy: actor.UserID, InputJSON: payload, SalesMode: in.SalesMode, SellPlans: in.SellPlans, Phase: "configuring", Version: 1, DomainEvidenceJSON: []byte(`{}`), HandoffEvidenceJSON: []byte(`{}`), CreatedAt: now, UpdatedAt: now}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		if _, err := service.CreatePromotionCode(ctx, channel.ID, "", strings.ToUpper(id.New("OEM"))); err != nil {
			return err
		}
		out = deliveryView(row)
		return nil
	})
	return out, err
}
func (s *Service) OEMDelivery(ctx context.Context, actor Principal, channelID string) (*DeliveryView, error) {
	channel, err := s.GetChannel(ctx, actor, channelID)
	if err != nil {
		return nil, err
	}
	if channel.Type != ChannelTypeC {
		return nil, ErrChannelImmutable
	}
	var row deliveryRow
	if err := s.db.WithContext(ctx).Where("channel_org_id = ?", channelID).First(&row).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		var stored channelRow
		if err := s.db.WithContext(ctx).Where("id = ?", channelID).First(&stored).Error; err != nil {
			return nil, err
		}
		row = deliveryRow{ChannelOrgID: channelID, OperationID: "existing:" + channelID, CreatedBy: actor.UserID, InputJSON: []byte(`{}`), SalesMode: "offline", Phase: "configuring", Version: 1, DomainEvidenceJSON: []byte(`{}`), HandoffEvidenceJSON: []byte(`{}`), CreatedAt: stored.CreatedAt, UpdatedAt: time.Now().UTC()}
		if err := s.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "channel_org_id"}}, DoNothing: true}).Create(&row).Error; err != nil {
			return nil, err
		}
		if err := s.db.WithContext(ctx).Where("channel_org_id = ?", channelID).First(&row).Error; err != nil {
			return nil, err
		}
	}
	return deliveryView(row), nil
}

type DeliveryUpdateInput struct {
	ExpectedVersion int64  `json:"expected_version"`
	SalesMode       string `json:"sales_mode"`
	SellPlans       bool   `json:"sell_plans"`
	Phase           string `json:"phase"`
}

func (s *Service) UpdateOEMDelivery(ctx context.Context, actor Principal, channelID string, in DeliveryUpdateInput) (*DeliveryView, error) {
	if !actor.IsPlatformAdmin() {
		return nil, ErrChannelImmutable
	}
	if in.SalesMode != "offline" && in.SalesMode != "online" {
		return nil, ErrPromotionInvalid
	}
	if in.Phase != "configuring" && in.Phase != "awaiting_acceptance" && in.Phase != "paused" {
		return nil, ErrPromotionInvalid
	}
	result := s.db.WithContext(ctx).Model(&deliveryRow{}).Where("channel_org_id = ? AND version = ? AND handed_over_at IS NULL", channelID, in.ExpectedVersion).Updates(map[string]any{"sales_mode": in.SalesMode, "sell_plans": in.SellPlans, "phase": in.Phase, "updated_at": time.Now().UTC(), "version": gorm.Expr("version + 1")})
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected != 1 {
		return nil, ErrDeliveryConflict
	}
	return s.OEMDelivery(ctx, actor, channelID)
}

// AdminLoginEvidence requires a real session issued after this administrator's
// scoped grant. Registration or another organization's login is insufficient.
type AdminLoginEvidence struct {
	UserID    string     `json:"user_id"`
	Email     string     `json:"email"`
	GrantedAt time.Time  `json:"granted_at"`
	LoginAt   *time.Time `json:"login_at,omitempty"`
}

func (s *Service) OEMAdminEvidence(ctx context.Context, channelID string) ([]AdminLoginEvidence, error) {
	var rows []AdminLoginEvidence
	err := s.db.WithContext(ctx).Table("identity_staff_members m").Select("u.id AS user_id,u.email,m.updated_at AS granted_at,MAX(t.created_at) AS login_at").Joins("JOIN identity_users u ON u.id=m.user_id AND u.status='active'").Joins("LEFT JOIN identity_access_tokens t ON t.user_id=u.id AND t.status='active' AND t.created_at>=m.updated_at").Where(`m.scope_type='channel' AND m.scope_id=? AND m.status='active' AND m.roles_json @> '["channel_admin"]'::jsonb AND u.channel_org_id=?`, channelID, channelID).Group("u.id,u.email,m.updated_at").Scan(&rows).Error
	return rows, err
}
func (s *Service) CompleteOEMDelivery(ctx context.Context, actor Principal, channelID, receiver string, version int64, evidence map[string]any) (*DeliveryView, error) {
	if !actor.IsPlatformAdmin() {
		return nil, ErrChannelImmutable
	}
	admins, err := s.OEMAdminEvidence(ctx, channelID)
	if err != nil {
		return nil, err
	}
	valid := false
	for _, admin := range admins {
		if admin.UserID == receiver && admin.LoginAt != nil {
			valid = true
		}
	}
	if !valid || len(evidence) == 0 {
		return nil, ErrDeliveryIncomplete
	}
	body, err := json.Marshal(evidence)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	result := s.db.WithContext(ctx).Model(&deliveryRow{}).Where("channel_org_id=? AND version=? AND handed_over_at IS NULL AND phase <> 'paused'", channelID, version).Updates(map[string]any{"receiver_user_id": receiver, "handed_over_at": now, "handoff_evidence_json": body, "phase": "handed_over", "updated_at": now, "version": gorm.Expr("version + 1")})
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected != 1 {
		return nil, ErrDeliveryConflict
	}
	return s.OEMDelivery(ctx, actor, channelID)
}
func (s *Service) RecordOEMDomainEvidence(ctx context.Context, actor Principal, channelID string, evidence map[string]any) error {
	if !actor.HasRole("platform_admin", "tech_admin") {
		return ErrChannelImmutable
	}
	body, err := json.Marshal(evidence)
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).Model(&deliveryRow{}).Where("channel_org_id=?", channelID).Updates(map[string]any{"domain_evidence_json": body, "updated_at": time.Now().UTC(), "version": gorm.Expr("version + 1")}).Error
}

func (s *Service) OEMOperation(ctx context.Context, actor Principal, operationID string) (*DeliveryView, error) {
	if !actor.IsPlatformAdmin() {
		return nil, ErrChannelImmutable
	}
	var row deliveryRow
	err := s.db.WithContext(ctx).Where("operation_id=? AND created_by=?", strings.TrimSpace(operationID), actor.UserID).First(&row).Error
	if err != nil {
		return nil, err
	}
	return deliveryView(row), nil
}
func (s *Service) ChannelAdminCandidates(ctx context.Context, channelID, search string) ([]ChannelAdminView, error) {
	out := []ChannelAdminView{}
	q := s.db.WithContext(ctx).Table("identity_users u").Select("u.id AS user_id,u.email").Where("u.channel_org_id=? AND u.status='active'", channelID).Where("NOT EXISTS(SELECT 1 FROM identity_user_roles ur JOIN identity_roles r ON r.id=ur.role_id WHERE ur.user_id=u.id AND r.code<>'end_user' AND NOT (r.code='channel_admin' AND ur.scope_type='channel' AND ur.scope_id=?))", channelID)
	if search = strings.TrimSpace(search); search != "" {
		q = q.Where("u.email ILIKE ?", "%"+search+"%")
	}
	err := q.Order("u.email").Limit(100).Scan(&out).Error
	return out, err
}
