package identity

import (
	"errors"
	"gorm.io/gorm"
)

// CommissionContextTx keeps attribution and eligibility reads in the usage transaction.
func (s *Service) CommissionContextTx(tx *gorm.DB, userID, channelID string) (*AttributionView, bool, string, error) {
	attr := &AttributionView{UserID: userID, ChannelOrgID: channelID}
	var row attributionRow
	if userID != "" {
		err := tx.Where("user_id = ?", userID).First(&row).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, false, "", err
		}
		if err == nil {
			attr.ChannelOrgID = row.ChannelOrgID
			if row.AcquisitionRoleID != nil {
				attr.AcquisitionRoleID = *row.AcquisitionRoleID
			}
		}
	}
	eligible := false
	if attr.AcquisitionRoleID != "" {
		var role acquisitionRow
		if err := tx.Where("id = ?", attr.AcquisitionRoleID).First(&role).Error; err != nil {
			return nil, false, "", err
		}
		attr.RoleType = role.Type
		if role.ParentID != nil {
			attr.ParentRoleID = *role.ParentID
		}
		var member roleMemberRow
		err := tx.Where("acquisition_role_id = ?", role.ID).First(&member).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, false, "", err
		}
		if err == nil {
			var user userRow
			if err := tx.Where("id = ?", member.UserID).First(&user).Error; err != nil {
				return nil, false, "", err
			}
			eligible = user.CanCommission
		}
	}
	if channelID == "" {
		channelID = attr.ChannelOrgID
	}
	market := ""
	if channelID != "" {
		var ch channelRow
		if err := tx.Where("id = ?", channelID).First(&ch).Error; err != nil {
			return nil, false, "", err
		}
		parentID, parentType := "", ""
		if ch.ParentID != nil && *ch.ParentID != "" {
			var parent channelRow
			if err := tx.Where("id = ?", *ch.ParentID).First(&parent).Error; err != nil {
				return nil, false, "", err
			}
			parentID, parentType = parent.ID, parent.Type
		}
		market = MarketChannelID(ch.Type, ch.ID, parentID, parentType)
	}
	return attr, eligible, market, nil
}
