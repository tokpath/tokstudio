package identity

import (
	"context"
	"encoding/json"
	"time"

	"gorm.io/gorm"
)

const (
	OfficialPromotionCode = "THA1"
	OfficialPrimaryDomain = "localhost"
	OfficialChannelCode   = "official-a"
	ResellerChannelCode   = "reseller-b"
	OEMChannelCode        = "oem-c"
	OfficialBrandID       = "brd_official"
	OEMBrandID            = "brd_oem"
	OfficialChannelID     = "chn_official_a"
	ResellerChannelID     = "chn_reseller_b"
	OEMChannelID          = "chn_oem_c"
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
		if err := upsertBootUser(tx, "agent.b@tokenhub.local", "end_user", adminToken+"-agent", "thagb_", ResellerChannelID, OfficialBrandID, "channel", ResellerChannelID); err != nil {
			return err
		}
		var agentUser userRow
		if err := tx.Where("email = ?", "agent.b@tokenhub.local").First(&agentUser).Error; err == nil {
			_ = tx.Where("user_id = ? AND acquisition_role_id = ?", agentUser.ID, AgentBRoleID).
				FirstOrCreate(&roleMemberRow{UserID: agentUser.ID, AcquisitionRoleID: AgentBRoleID, CreatedAt: time.Now().UTC()}).Error
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
	agent := AgentBRoleID
	kol1 := KOL1BRoleID
	kol2 := KOL2BRoleID
	roles := []acquisitionRow{
		{ID: AgentBRoleID, ChannelOrgID: ResellerChannelID, Type: AcqAgent, Level: 0, Status: "active", CreatedAt: now},
		{ID: KOL1BRoleID, ChannelOrgID: ResellerChannelID, Type: AcqKOL1, ParentID: &agent, Level: 1, Status: "active", CreatedAt: now},
		{ID: KOL2BRoleID, ChannelOrgID: ResellerChannelID, Type: AcqKOL2, ParentID: &kol1, Level: 2, Status: "active", CreatedAt: now},
	}
	for i := range roles {
		if err := tx.Where("id = ?", roles[i].ID).FirstOrCreate(&roles[i]).Error; err != nil {
			return err
		}
	}
	promos := []promotionRow{
		{ID: "promo_tha1", Code: OfficialPromotionCode, ChannelOrgID: OfficialChannelID, Status: "active"},
		{ID: "promo_thb1", Code: "THB1", ChannelOrgID: ResellerChannelID, AcquisitionRoleID: &agent, Status: "active"},
		{ID: "promo_thc1", Code: "THC1", ChannelOrgID: OEMChannelID, Status: "active"},
		{ID: "promo_thb_agent", Code: PromoAgentB, ChannelOrgID: ResellerChannelID, AcquisitionRoleID: &agent, Status: "active"},
		{ID: "promo_thb_kol1", Code: PromoKOL1B, ChannelOrgID: ResellerChannelID, AcquisitionRoleID: &kol1, Status: "active"},
		{ID: "promo_thb_kol2", Code: PromoKOL2B, ChannelOrgID: ResellerChannelID, AcquisitionRoleID: &kol2, Status: "active"},
	}
	for _, promo := range promos {
		var existing promotionRow
		err := tx.Where("code = ?", promo.Code).First(&existing).Error
		if err == nil {
			if existing.AcquisitionRoleID == nil && promo.AcquisitionRoleID != nil {
				existing.AcquisitionRoleID = promo.AcquisitionRoleID
				if err := tx.Save(&existing).Error; err != nil {
					return err
				}
			}
			continue
		}
		if err := tx.Create(&promo).Error; err != nil {
			return err
		}
	}
	return nil
}
