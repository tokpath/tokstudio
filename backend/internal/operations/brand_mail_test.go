package operations

import (
	"context"
	"testing"

	"github.com/tokpath/tokstudio/backend/internal/platform/mail"
	"gorm.io/gorm"
)

type brandedRecipientDirectory struct {
	RecipientDirectory
	brandID string
}

func (d brandedRecipientDirectory) ResolveRecipient(ctx context.Context, tx *gorm.DB, operator, actor string) (Recipient, error) {
	r, err := d.RecipientDirectory.ResolveRecipient(ctx, tx, operator, actor)
	r.BrandID = d.brandID
	return r, err
}

type brandSender struct{ received []mail.Message }

func (s *brandSender) Available() bool                  { return false }
func (s *brandSender) AvailableForBrand(id string) bool { return id == "oem-brand" }
func (s *brandSender) Send(_ context.Context, m mail.Message) error {
	if !s.AvailableForBrand(m.BrandID) {
		return ErrNotReady
	}
	s.received = append(s.received, m)
	return nil
}
func TestBrandNotificationUsesIndependentMailWhenPlatformMailIsUnavailable(t *testing.T) {
	s, _, _, operator, customer := opsFixture(t)
	s.Recipients = brandedRecipientDirectory{s.Recipients, "oem-brand"}
	sender := &brandSender{}
	s.Mail = sender
	ctx := context.Background()
	if _, err := s.Emit(ctx, nil, Notification{OperatorID: operator, RecipientID: customer, BusinessKey: "receipt:brand-mail", Category: "mandatory", Severity: "info", Title: "Receipt", Body: "Review receipt"}); err != nil {
		t.Fatal(err)
	}
	deliveries, err := s.RunNotifications(ctx, 1)
	if err != nil || len(deliveries) != 1 || deliveries[0].State != "accepted" || len(sender.received) != 1 || sender.received[0].BrandID != "oem-brand" {
		t.Fatal("notification used global availability or lost brand", deliveries, sender.received, err)
	}
}
