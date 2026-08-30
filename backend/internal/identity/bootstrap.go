package identity

import (
	"context"
	"encoding/json"
	"time"

	"gorm.io/gorm"
)

const (
	OfficialPromotionCode  = "THA1"
	OfficialPrimaryDomain  = "localhost"
	OfficialChannelCode    = "official-a"
	ResellerChannelCode    = "reseller-b"
	OEMChannelCode         = "oem-c"
	OfficialBrandID        = "brd_official"
	OEMBrandID             = "brd_oem"
	OfficialChannelID      = "chn_official_a"
	ResellerChannelID      = "chn_reseller_b"
	OEMChannelID           = "chn_oem_c"
)

func (s *Service) Bootstrap(ctx context.Context, adminToken, userToken, channelToken string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := seedCatalog(tx); err != nil {
			return err
		}
		if adminToken == "" || userToken == "" {
			return nil
		}
		if channelToken == "" {
			channelToken = adminToken + "-b"
		}
		if err := upsertBootUser(tx, "admin@tokenhub.local", "platform_admin", adminToken, "thadm_", OfficialChannelID, OfficialBrandID, "platform", "*"); err != nil {
			return err
		}
		if err := upsertBootUser(tx, "channel.b@tokenhub.local", "channel_admin", channelToken, "thchb_", ResellerChannelID, OfficialBrandID, "channel", ResellerChannelID); err != nil {
			return err
		}
		return upsertBootUser(tx, "user@tokenhub.local", "end_user", userToken, "thusr_", OfficialChannelID, OfficialBrandID, "channel", OfficialChannelID)
	})
}

func seedCatalog(tx *gorm.DB) error {
	officialTheme, _ := json.Marshal(map[string]string{
		"primary": "#22d3ee", "background": "#020617", "name": "TokenHub",
	})
	oemTheme, _ := json.Marshal(map[string]string{
		"primary": "#f59e0b", "background": "#111827", "name": "Aurora OEM",
	})
	now := time.Now().UTC()
	brands := []brandRow{
		{ID: OfficialBrandID, Name: "TokenHub", PrimaryDomain: OfficialPrimaryDomain, APIDomain: "localhost", AdminDomain: "admin.localhost", ThemeJSON: officialTheme, CreatedAt: now},
		{ID: OEMBrandID, Name: "Aurora OEM", PrimaryDomain: "oem.localhost", APIDomain: "api.oem.localhost", AdminDomain: "admin.oem.localhost", ThemeJSON: oemTheme, CreatedAt: now},
	}
	for _, brand := range brands {
		if err := tx.Where("id = ?", brand.ID).FirstOrCreate(&brand).Error; err != nil {
			return err
		}
	}
	channels := []channelRow{
		{ID: OfficialChannelID, Code: OfficialChannelCode, Type: "A", Status: "active", BrandID: OfficialBrandID, CreatedAt: now},
		{ID: ResellerChannelID, Code: ResellerChannelCode, Type: "B", Status: "active", BrandID: OfficialBrandID, CreatedAt: now},
		{ID: OEMChannelID, Code: OEMChannelCode, Type: "C", Status: "active", BrandID: OEMBrandID, CreatedAt: now},
	}
	for _, channel := range channels {
		if err := tx.Where("id = ?", channel.ID).FirstOrCreate(&channel).Error; err != nil {
			return err
		}
	}
	promos := []promotionRow{
		{ID: "promo_tha1", Code: OfficialPromotionCode, ChannelOrgID: OfficialChannelID, Status: "active"},
		{ID: "promo_thb1", Code: "THB1", ChannelOrgID: ResellerChannelID, Status: "active"},
		{ID: "promo_thc1", Code: "THC1", ChannelOrgID: OEMChannelID, Status: "active"},
	}
	for _, promo := range promos {
		if err := tx.Where("code = ?", promo.Code).FirstOrCreate(&promo).Error; err != nil {
			return err
		}
	}
	return nil
}
