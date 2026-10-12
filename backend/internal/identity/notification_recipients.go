package identity

import (
	"context"

	"gorm.io/gorm"
)

// NotificationRecipient is an identity-owned projection. Notification consumers
// never resolve recipients by email alone, nor inspect identity tables directly.
type NotificationRecipient struct {
	UserID, BrandID, Email, Locale, BrandName, PrimaryDomain, AdminDomain string
	Verified                                                              bool
	Portal                                                                string
}

func (s *Service) notificationQuery(ctx context.Context) *gorm.DB {
	return s.db.WithContext(ctx).Table("identity_users AS u").
		Select(`u.id AS user_id, u.brand_id, u.email, u.locale, u.email_verified_at IS NOT NULL AS verified,
            b.name AS brand_name, b.primary_domain, b.admin_domain`).
		Joins("JOIN identity_brands AS b ON b.id = u.brand_id").
		Where(`u.status = 'active' AND NOT EXISTS
            (SELECT 1 FROM identity_staff_members sm WHERE sm.user_id = u.id AND sm.status <> 'active')`)
}
func (s *Service) NotificationUser(ctx context.Context, userID string) (*NotificationRecipient, error) {
	var row NotificationRecipient
	err := s.notificationQuery(ctx).Where("u.id = ?", userID).Take(&row).Error
	if err != nil {
		return nil, err
	}
	row.Portal = "app"
	return &row, nil
}
func (s *Service) NotificationStaff(ctx context.Context, scopeType, scopeID string, roles []string) ([]NotificationRecipient, error) {
	rows := []NotificationRecipient{}
	err := s.notificationQuery(ctx).Where(`EXISTS (SELECT 1 FROM identity_user_roles ur
        JOIN identity_roles r ON r.id = ur.role_id WHERE ur.user_id = u.id
        AND ur.scope_type = ? AND ur.scope_id = ? AND r.code IN ?)`, scopeType, scopeID, roles).
		Order("u.id").Scan(&rows).Error
	for i := range rows {
		rows[i].Portal = "channel"
		if scopeType == "platform" {
			rows[i].Portal = "admin"
		}
	}
	return rows, err
}
