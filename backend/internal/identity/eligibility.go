package identity

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/tokpath/tokstudio/backend/internal/platform/id"
)

const (
	EligibilityScopePlatform = "platform"
	EligibilityScopeChannel  = "channel"
	// DefaultEligibilitySpendMinor 默认累计消费达线（10 USD）。0 表示关闭该路径。
	DefaultEligibilitySpendMinor int64 = 10_000_000
	DefaultEligibilityTopupMinor int64 = 10_000_000
	DefaultEligibilityGiftMinor  int64 = 1_000_000
)

type eligibilityRow struct {
	ID         string    `gorm:"column:id;primaryKey"`
	ScopeType  string    `gorm:"column:scope_type"`
	ScopeID    string    `gorm:"column:scope_id"`
	SpendMinor int64     `gorm:"column:spend_minor"`
	TopupMinor int64     `gorm:"column:topup_minor"`
	GiftMinor  int64     `gorm:"column:gift_minor"`
	CreatedAt  time.Time `gorm:"column:created_at"`
	UpdatedAt  time.Time `gorm:"column:updated_at"`
}

func (eligibilityRow) TableName() string { return "identity_eligibility_rules" }

type EligibilityView struct {
	ID         string `json:"id"`
	ScopeType  string `json:"scope_type"`
	ScopeID    string `json:"scope_id"`
	SpendMinor int64  `json:"spend_minor"`
	TopupMinor int64  `json:"topup_minor"`
	GiftMinor  int64  `json:"gift_minor"`
	Inherited  bool   `json:"inherited"`
}

func eligibilityView(row eligibilityRow, inherited bool) *EligibilityView {
	return &EligibilityView{
		ID: row.ID, ScopeType: row.ScopeType, ScopeID: row.ScopeID,
		SpendMinor: row.SpendMinor, TopupMinor: row.TopupMinor, GiftMinor: row.GiftMinor, Inherited: inherited,
	}
}

func seedEligibility(tx *gorm.DB) error {
	now := time.Now().UTC()
	row := eligibilityRow{
		ID: "elg_platform", ScopeType: EligibilityScopePlatform, ScopeID: "*",
		SpendMinor: DefaultEligibilitySpendMinor, TopupMinor: DefaultEligibilityTopupMinor, GiftMinor: DefaultEligibilityGiftMinor,
		CreatedAt: now, UpdatedAt: now,
	}
	return tx.Where("scope_type = ? AND scope_id = ?", EligibilityScopePlatform, "*").FirstOrCreate(&row).Error
}

func (s *Service) PlatformEligibility(ctx context.Context) (*EligibilityView, error) {
	return s.platformEligibility(ctx, false)
}

func (s *Service) EffectiveEligibility(ctx context.Context, channelID string) (*EligibilityView, error) {
	market := ""
	if channelID != "" {
		if resolved, err := s.ResolveMarketChannelID(ctx, channelID); err == nil {
			market = resolved
		}
	}
	if market != "" {
		ch, err := s.lookupChannel(ctx, market)
		if err == nil && ch.Type == ChannelTypeC {
			var row eligibilityRow
			err := s.db.WithContext(ctx).Where("scope_type = ? AND scope_id = ?", EligibilityScopeChannel, market).First(&row).Error
			if err == nil {
				return eligibilityView(row, false), nil
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, err
			}
		}
	}
	return s.platformEligibility(ctx, true)
}

func (s *Service) platformEligibility(ctx context.Context, inherited bool) (*EligibilityView, error) {
	var row eligibilityRow
	err := s.db.WithContext(ctx).Where("scope_type = ? AND scope_id = ?", EligibilityScopePlatform, "*").First(&row).Error
	if err == nil {
		return eligibilityView(row, inherited), nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	return &EligibilityView{
		ScopeType: EligibilityScopePlatform, ScopeID: "*",
		SpendMinor: DefaultEligibilitySpendMinor, TopupMinor: DefaultEligibilityTopupMinor, GiftMinor: DefaultEligibilityGiftMinor,
		Inherited: true,
	}, nil
}

func (s *Service) UpdatePlatformEligibility(ctx context.Context, spend, topup, gift int64) (*EligibilityView, error) {
	if spend < 0 || topup < 0 || gift < 0 {
		return nil, ErrPromotionInvalid
	}
	now := time.Now().UTC()
	var row eligibilityRow
	err := s.db.WithContext(ctx).Where("scope_type = ? AND scope_id = ?", EligibilityScopePlatform, "*").First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		row = eligibilityRow{
			ID: "elg_platform", ScopeType: EligibilityScopePlatform, ScopeID: "*",
			SpendMinor: spend, TopupMinor: topup, GiftMinor: gift, CreatedAt: now, UpdatedAt: now,
		}
		if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
			return nil, err
		}
		return eligibilityView(row, false), nil
	}
	if err != nil {
		return nil, err
	}
	row.SpendMinor = spend
	row.TopupMinor = topup
	row.GiftMinor = gift
	row.UpdatedAt = now
	if err := s.db.WithContext(ctx).Save(&row).Error; err != nil {
		return nil, err
	}
	return eligibilityView(row, false), nil
}

func (s *Service) UpdateChannelEligibility(ctx context.Context, viewer Principal, spend, topup, gift int64) (*EligibilityView, error) {
	if spend < 0 || topup < 0 || gift < 0 {
		return nil, ErrPromotionInvalid
	}
	channelID := viewer.ChannelOrgID
	if channelID == "" {
		return nil, ErrChannelImmutable
	}
	ch, err := s.lookupChannel(ctx, channelID)
	if err != nil {
		return nil, err
	}
	if ch.Type != ChannelTypeC {
		return nil, ErrChannelImmutable
	}
	now := time.Now().UTC()
	row := eligibilityRow{
		ID: id.New("elg"), ScopeType: EligibilityScopeChannel, ScopeID: ch.ID,
		SpendMinor: spend, TopupMinor: topup, GiftMinor: gift, CreatedAt: now, UpdatedAt: now,
	}
	err = s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "scope_type"}, {Name: "scope_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"spend_minor", "topup_minor", "gift_minor", "updated_at"}),
	}).Create(&row).Error
	if err != nil {
		return nil, err
	}
	var out eligibilityRow
	if err := s.db.WithContext(ctx).Where("scope_type = ? AND scope_id = ?", EligibilityScopeChannel, ch.ID).First(&out).Error; err != nil {
		return nil, err
	}
	return eligibilityView(out, false), nil
}

// Consider 在充值或消费后尝试达线赋权。已有资格不撤销。
func (s *Service) Consider(ctx context.Context, userID, channelOrgID string, singleTopup, lifetimeSpend int64) error {
	if userID == "" {
		return nil
	}
	var user userRow
	if err := s.db.WithContext(ctx).Where("id = ?", userID).First(&user).Error; err != nil {
		return nil
	}
	if user.CanCommission {
		return nil
	}
	channelID := channelOrgID
	if channelID == "" && user.ChannelOrgID != nil {
		channelID = *user.ChannelOrgID
	}
	rule, err := s.EffectiveEligibility(ctx, channelID)
	if err != nil || rule == nil {
		return err
	}
	ok := false
	if rule.TopupMinor > 0 && singleTopup >= rule.TopupMinor {
		ok = true
	}
	if rule.SpendMinor > 0 && lifetimeSpend >= rule.SpendMinor {
		ok = true
	}
	if !ok {
		return nil
	}
	return s.SetCommissionEligible(ctx, userID, true)
}
