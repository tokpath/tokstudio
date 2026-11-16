package operations

import (
	"context"
	"errors"
	"io/fs"
	"strings"
	"testing"

	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/incentives"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"github.com/tokpath/tokstudio/backend/internal/paymentmethods"
	"gorm.io/gorm"
)

func TestIncentiveRelayUsesDurableFactsAndSafeAudience(t *testing.T) {
	s, a, _, _, _ := opsFixture(t)
	ctx := context.Background()
	apply := func(_ string, f fs.FS) {
		t.Helper()
		files, e := fs.ReadDir(f, ".")
		if e != nil {
			t.Fatal(e)
		}
		for _, file := range files {
			b, e := fs.ReadFile(f, file.Name())
			if e != nil {
				t.Fatal(e)
			}
			if e = s.DB.Exec(string(b)).Error; e != nil {
				t.Fatal(e)
			}
		}
	}
	apply(organization.Migrations())
	apply(ledger.Migrations())
	apply(commerce.Migrations())
	apply(paymentmethods.Migrations())
	apply(incentives.Migrations())
	org := organization.New(s.DB)
	platform, e := org.BootstrapPlatform(ctx, nil, organization.BootstrapInput{BrandName: "Notice Test", OperatorName: "Notice Operator", OwnerStaffID: a.actor})
	if e != nil {
		t.Fatal(e)
	}
	op := platform.Operator.ID
	inc := incentives.New(s.DB)
	if _, e = inc.SetPolicy(ctx, nil, a.actor, op, incentives.PolicyInput{Evidence: "sensitive raw bank proof never send", ThresholdMoney: ledger.FromInt64(10), RewardValiditySeconds: 3600}); e != nil {
		t.Fatal(e)
	}
	s.Authorize = org
	s.Recipients = nil
	sink := IncentiveNotifications{Operations: s, Audience: inc}
	var event incentives.Event
	if e = s.DB.First(&event).Error; e != nil {
		t.Fatal(e)
	}
	if e := inc.PublishEvent(ctx, nil, event.ID, sink); !errors.Is(e, ErrNotReady) {
		t.Fatalf("partial relay committed without recipient verification: %v", e)
	}
	var published int64
	if e = s.DB.Model(&incentives.Event{}).Where("published_at IS NOT NULL").Count(&published).Error; e != nil || published != 0 {
		t.Fatalf("failed notice lost original retryable event: %d %v", published, e)
	}
	s.Recipients = operationRecipients{operator: op, customer: a.actor}
	if e := inc.PublishEvent(ctx, nil, event.ID, sink); e != nil {
		t.Fatalf("durable relay failed: %v", e)
	}
	if e := inc.PublishEvent(ctx, nil, event.ID, sink); e != nil {
		t.Fatalf("published event replay failed: %v", e)
	}
	notices, e := s.ListNotifications(ctx, a.actor, op, nil, 20)
	if e != nil || len(notices) != 1 {
		t.Fatal(notices, e)
	}
	if strings.Contains(notices[0].Body, "sensitive") || notices[0].RecipientID != a.actor || notices[0].Title != "推广经营规则已更新" {
		t.Fatalf("raw evidence leaked or audience changed: %+v", notices)
	}
	event.Kind = "commission.source_restricted"
	if e = s.DB.Transaction(func(tx *gorm.DB) error { return sink.EnqueueIncentiveEvent(ctx, tx, event) }); !errors.Is(e, incentives.ErrConflict) {
		t.Fatalf("forged kind accepted: %v", e)
	}
}
