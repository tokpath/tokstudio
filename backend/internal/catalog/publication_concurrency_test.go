package catalog

import (
	"context"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"testing"
	"time"
)

type blockedPublicationSink struct {
	CatalogNoticeSink
	entered, release chan struct{}
}

func (s blockedPublicationSink) RecordCatalogNotice(ctx context.Context, tx *gorm.DB, c CatalogChange, operator, recipient string) error {
	close(s.entered)
	select {
	case <-s.release:
	case <-ctx.Done():
		return ctx.Err()
	}
	return s.CatalogNoticeSink.RecordCatalogNotice(ctx, tx, c, operator, recipient)
}
func TestPublicationAndAdmissionsDoNotWaitForNotificationFanout(t *testing.T) {
	s := newCommerceTestStore(t)
	seedCommerceModel(t, s)
	c, now, sink := testController(t, s, "platform")
	blocked := blockedPublicationSink{CatalogNoticeSink: sink, entered: make(chan struct{}), release: make(chan struct{})}
	c.Notices = blocked
	price := priceAt(uuid.NewString(), PriceScope{Kind: "retail", SellerID: "platform"}, now.Add(time.Minute), "2")
	change, err := c.SchedulePrice(t.Context(), "platform", price, "")
	if err != nil {
		t.Fatal(err)
	}
	if change.State != "applied" {
		t.Fatal("the price and its notification plan must commit together", change.State)
	}
	done := make(chan error, 1)
	go func() { _, err := c.Run(context.Background(), 20); done <- err }()
	select {
	case <-blocked.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("worker did not start fanout")
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error { _, err := s.WithDB(tx).LockRevision(ctx); return err })
	close(blocked.release)
	if err != nil {
		t.Fatal("a slow notification blocked request admission", err)
	}
	select {
	case err = <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("worker did not complete")
	}
}
