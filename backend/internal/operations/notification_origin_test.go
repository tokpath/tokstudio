package operations

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"testing"
)

func TestNotificationSystemGeneratedUsesTrustedNamespaces(t *testing.T) {
	for _, prefix := range []string{"commerce:", "incentive:", "catalog:", "renewal-notice:", "replenishment:", "risk:", restrictionReviewKey, "remediation:", "price-check:"} {
		t.Run(prefix, func(t *testing.T) {
			if !(Notification{BusinessKey: prefix + "domain-event", Title: "operator-authored-looking text"}).SystemGenerated() {
				t.Fatal("trusted domain namespace was not recognized")
			}
			if (Notification{BusinessKey: "announcement:" + prefix + "domain-event"}).SystemGenerated() {
				t.Fatal("announcement impersonated a trusted domain namespace")
			}
		})
	}
	for _, key := range []string{"", "commerce:", "commerce", "commerce-custom:event", "prefix:commerce:event", "announcement:event", "arbitrary-event"} {
		if (Notification{BusinessKey: key, Title: "订单已创建", Body: "订单已创建，请按订单详情完成付款。"}).SystemGenerated() {
			t.Fatalf("untrusted key %q classified as system-generated based on content", key)
		}
	}
}

func TestAnnouncementRejectsReservedSystemNamespaces(t *testing.T) {
	s := &Service{}
	for _, prefix := range []string{"commerce:", "incentive:", "catalog:", "renewal-notice:", "replenishment:", "risk:", restrictionReviewKey, "remediation:", "price-check:"} {
		for _, key := range []string{prefix, prefix + "operator-chosen-key"} {
			_, err := s.Announce(context.Background(), "actor", Notification{BusinessKey: key})
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("authored notice used reserved key %q: %v", key, err)
			}
		}
	}
}

func TestAnnouncementCannotImpersonateSystemNoticeAndRetainsRetryIdentity(t *testing.T) {
	s, auth, _, operator, customer := opsFixture(t)
	in := Notification{OperatorID: operator, RecipientID: customer, BusinessKey: uuid.NewString(), Category: "mandatory", Severity: "info", Title: "订单已创建", Body: "Operator-authored evidence remains unchanged", Reference: "operator-authored-reference"}
	first, err := s.Announce(context.Background(), auth.actor, in)
	if err != nil {
		t.Fatal(err)
	}
	if first.SystemGenerated() || first.BusinessKey != in.BusinessKey || first.Title != in.Title || first.Body != in.Body || first.Reference != in.Reference {
		t.Fatalf("announcement origin or authored content changed: %+v", first)
	}
	retry, err := s.Announce(context.Background(), auth.actor, in)
	if err != nil || retry.ID != first.ID {
		t.Fatalf("original request retry created a different notice: %+v %v", retry, err)
	}
	retry, err = s.Announce(context.Background(), auth.actor, first)
	if err != nil || retry.ID != first.ID {
		t.Fatalf("stored-notice retry created a different notice: %+v %v", retry, err)
	}
}
