package operations

import (
	"github.com/google/uuid"
	"testing"
	"time"
)

func TestNotificationSummariesAreBoundedAndPersonalDetailsRemainPrivate(t *testing.T) {
	s, auth, _, operator, _ := opsFixture(t)
	s.Recipients = operationRecipients{operator: operator, customer: auth.actor}
	for i := 0; i < 65; i++ {
		for _, recipient := range []string{auth.actor, uuid.NewString()} {
			group := "catalog:large"
			if recipient == auth.actor {
				group = "personal:" + uuid.NewString()
			}
			n := Notification{ID: uuid.NewString(), OperatorID: operator, RecipientID: recipient, BusinessKey: group, Category: "mandatory", Severity: "info", Title: "Notice", Body: "Body", Fingerprint: "fixture", CreatedAt: time.Now().Add(time.Duration(i) * time.Microsecond)}
			if err := s.DB.Create(&n).Error; err != nil {
				t.Fatal(err)
			}
			if err := s.DB.Table("operations_deliveries").Create(map[string]any{"notification_id": n.ID, "message_ciphertext": "fixture", "state": "pending", "next_attempt_at": time.Now(), "error_code": ""}).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	personal, groups, _, err := s.ListNotificationGroups(t.Context(), auth.actor, operator, "", "", "", 1)
	if err != nil || personal.TotalCount != 65 || len(personal.Notices) != 0 || len(groups) != 1 || len(groups[0].Notices) != 0 || groups[0].TotalCount != 65 {
		t.Fatal("list summaries loaded recipient bodies", personal.TotalCount, len(groups), err)
	}
	auth.deny.Store(true)
	personal, groups, _, err = s.ListNotificationGroups(t.Context(), auth.actor, operator, "", "", "", 1)
	if err != nil || personal.TotalCount != 65 || len(groups) != 0 {
		t.Fatal("staff cannot read personal notices without management permission", err)
	}
	cursor := ""
	seen := map[string]bool{}
	for {
		rows, next, err := s.ListNotificationNotices(t.Context(), auth.actor, operator, "personal", cursor, "", "", 7)
		if err != nil || len(rows) > 7 {
			t.Fatal("unbounded personal details", err)
		}
		for _, row := range rows {
			if row.RecipientID != auth.actor || seen[row.ID] {
				t.Fatal("recipient leaked or repeated")
			}
			seen[row.ID] = true
		}
		if next == "" {
			break
		}
		cursor = next
	}
	if len(seen) != 65 {
		t.Fatal("personal pagination omitted notices", len(seen))
	}
	if _, _, err = s.ListNotificationNotices(t.Context(), auth.actor, operator, "catalog:large", "", "", "", 7); err == nil {
		t.Fatal("personal access granted access to other recipients")
	}
}
