package identity

import (
	"context"

	"gorm.io/gorm/clause"
)

// ResolvePaymentOwnerID resolves the brand that owns the customer's funds.
// Attribution stays on B; B never owns a merchant or a service quota pool.
func (s *Service) ResolvePaymentOwnerID(ctx context.Context, channelID string) (string, error) {
	if channelID == "" || channelID == OfficialChannelID {
		return OfficialChannelID, nil
	}
	row, err := s.lookupChannel(ctx, channelID)
	if err != nil {
		return "", err
	}
	if row.Type == ChannelTypeC {
		return row.ID, nil
	}
	if row.Type != ChannelTypeB {
		return "", ErrChannelImmutable
	}
	if row.ParentID == nil || *row.ParentID == "" || *row.ParentID == OfficialChannelID {
		return OfficialChannelID, nil
	}
	parent, err := s.lookupChannel(ctx, *row.ParentID)
	if err != nil {
		return "", err
	}
	if parent.Type != ChannelTypeC {
		return "", ErrChannelImmutable
	}
	return parent.ID, nil
}

// BillingCustomerForOwner must be called on the transaction used for allocation.
// It excludes employees and locks attribution until the receipt is committed.
func (s *Service) BillingCustomerForOwner(ctx context.Context, userID, ownerID string) (string, error) {
	var row userRow
	if err := s.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ? AND id NOT IN (SELECT user_id FROM identity_staff_members)", userID).First(&row).Error; err != nil {
		return "", ErrNotFound
	}
	channelID := ""
	if row.ChannelOrgID != nil {
		channelID = *row.ChannelOrgID
	}
	owner, err := s.ResolvePaymentOwnerID(ctx, channelID)
	if err != nil {
		return "", err
	}
	if owner != ownerID || row.Status != "active" {
		return "", ErrChannelImmutable
	}
	return channelID, nil
}

func (s *Service) SearchBrandBillingRecipients(ctx context.Context, query, ownerID string) ([]BillingRecipient, error) {
	return s.searchBillingRecipients(ctx, query, ownerID)
}

// BrandChannelIDs is the funding/settlement scope, including direct B attribution.
func (s *Service) BrandChannelIDs(ctx context.Context, ownerID string) ([]string, error) {
	owner, err := s.ResolvePaymentOwnerID(ctx, ownerID)
	if err != nil || owner != ownerID {
		return nil, ErrChannelImmutable
	}
	q := s.db.WithContext(ctx).Model(&channelRow{}).Where("type = ?", ChannelTypeB)
	if ownerID == OfficialChannelID {
		q = q.Where("parent_id = ? OR parent_id IS NULL", ownerID)
	} else {
		q = q.Where("parent_id = ?", ownerID)
	}
	var children []channelRow
	err = q.Find(&children).Error
	if err != nil {
		return nil, err
	}
	ids := []string{ownerID}
	if ownerID == OfficialChannelID {
		ids = append(ids, "")
	}
	for _, child := range children {
		ids = append(ids, child.ID)
	}
	return ids, nil
}
