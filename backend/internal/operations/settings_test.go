package operations

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestManagedNotificationsSearchPaginationReadStateAndAuthorization(t *testing.T) {
	s, auth, _, operator, customer := opsFixture(t)
	ctx := context.Background()
	var target Notification
	for i := 0; i < 53; i++ {
		n, err := s.Emit(ctx, nil, Notification{OperatorID: operator, RecipientID: customer, BusinessKey: fmt.Sprintf("settings-%d", i), Category: "mandatory", Severity: "info", Title: fmt.Sprintf("Notice %d", i), Body: "Readable content", Reference: "order"})
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			target = n
		}
	}
	if err := s.ReadNotification(ctx, customer, operator, target.ID); err != nil {
		t.Fatal(err)
	}
	if unread, err := s.UnreadNotificationCount(ctx, customer, operator); err != nil || unread != 52 {
		t.Fatal("unread count must exclude the one already read notice", unread, err)
	}
	rows, cursor, err := s.ListOperatorNotifications(ctx, auth.actor, operator, "", "", "", 50)
	if err != nil || len(rows) != 50 || cursor == "" {
		t.Fatal(len(rows), cursor, err)
	}
	seen := map[string]bool{}
	for _, row := range rows {
		seen[row.ID] = true
	}
	more, next, err := s.ListOperatorNotifications(ctx, auth.actor, operator, cursor, "", "", 50)
	if err != nil || len(more) != 3 || next != "" {
		t.Fatal(more, next, err)
	}
	for _, row := range more {
		if seen[row.ID] {
			t.Fatal("duplicate notification across pages")
		}
	}
	rows, _, err = s.ListOperatorNotifications(ctx, auth.actor, operator, "", "Notice 0", "pending", 50)
	if err != nil || len(rows) != 1 || rows[0].ID != target.ID || rows[0].ReadAt == nil || rows[0].Body != "Readable content" || rows[0].RecipientID != customer {
		t.Fatal(rows, err)
	}
	rows, _, err = s.ListOperatorNotifications(ctx, auth.actor, uuid.NewString(), "", "", "", 50)
	if err != nil || len(rows) != 0 {
		t.Fatal("notification leaked to another workspace", rows, err)
	}
	if _, _, err = s.ListOperatorNotifications(ctx, auth.actor, operator, "invalid", "", "", 50); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	mine := uuid.NewString()
	shared := "catalog:" + uuid.NewString()
	onlyMine := "risk:" + uuid.NewString()
	if _, err = s.Emit(ctx, nil, Notification{OperatorID: operator, RecipientID: customer, BusinessKey: shared, Category: "mandatory", Severity: "info", Title: "模型与计价计划变更", Body: "shared body", Reference: "catalog-change:shared"}); err != nil {
		t.Fatal(err)
	}
	other := uuid.NewString()
	readAt := time.Now()
	for _, row := range []Notification{
		{ID: uuid.NewString(), OperatorID: operator, RecipientID: other, BusinessKey: shared, Category: "mandatory", Severity: "info", Title: "模型与计价计划变更", Body: "shared body", Reference: "catalog-change:shared", Fingerprint: "synthetic", CreatedAt: time.Now(), ReadAt: &readAt},
		{ID: mine, OperatorID: operator, RecipientID: auth.actor, BusinessKey: onlyMine, Category: "mandatory", Severity: "info", Title: "只发给我", Body: "personal body", Reference: "risk:mine", Fingerprint: "synthetic", CreatedAt: time.Now()},
	} {
		if err = s.DB.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
		if err = s.DB.Table("operations_deliveries").Create(map[string]any{"notification_id": row.ID, "message_ciphertext": "synthetic", "state": "pending", "next_attempt_at": time.Now(), "error_code": ""}).Error; err != nil {
			t.Fatal(err)
		}
	}
	s.Recipients = operationRecipients{operator: operator, customer: auth.actor}
	personal, events, groupCursor, err := s.ListNotificationGroups(ctx, auth.actor, operator, "", "", "", 50)
	if err != nil || personal.UnreadCount != 1 || personal.TotalCount != 1 || len(personal.Notices) != 0 || groupCursor == "" {
		t.Fatal(personal, len(events), groupCursor, err)
	}
	var sharedGroup NotificationGroup
	for _, event := range events {
		if event.Key == shared {
			sharedGroup = event
		}
		if event.Key == onlyMine {
			t.Fatal("a notice sent only to the signed-in person is not a separate event")
		}
	}
	if sharedGroup.TotalCount != 2 || sharedGroup.UnreadCount != 1 || len(sharedGroup.Notices) != 0 {
		t.Fatal(sharedGroup)
	}
	if _, _, _, err = s.ListNotificationGroups(ctx, auth.actor, operator, "invalid", "", "", 50); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	auth.deny.Store(true)
	if _, _, err = s.ListOperatorNotifications(ctx, auth.actor, operator, "", "", "", 50); !errors.Is(err, ErrForbidden) {
		t.Fatal("revoked operator read", err)
	}
	if mine, groups, _, err := s.ListNotificationGroups(ctx, auth.actor, operator, "", "", "", 50); err != nil || mine.TotalCount != 1 || len(groups) != 0 {
		t.Fatal("personal notification access must not grant management access", err)
	}
}

func TestRemoveUnusedContentRepositoryWithExistingData(t *testing.T) {
	db := operationsDBBefore(t, "0005")
	op, recipient, item, notice := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	if err := db.Exec(`INSERT INTO operations_retention_items(id,operator_id,kind,reference,payload_ciphertext,expires_at,created_at) VALUES (?,?,'temporary_content','unused','synthetic',?,?)`, item, op, time.Now().Add(time.Hour), time.Now()).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO operations_retention_access(id,item_id,actor_id,action,reason,at) VALUES (?,?,?,'hold','synthetic',?)`, uuid.NewString(), item, recipient, time.Now()).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&Notification{ID: notice, OperatorID: op, RecipientID: recipient, BusinessKey: "keep", Category: "mandatory", Severity: "info", Title: "Receipt", Body: "Keep this fact", Reference: "order", Fingerprint: "synthetic", CreatedAt: time.Now()}).Error; err != nil {
		t.Fatal(err)
	}
	data, err := migrations.ReadFile("migrate/0005_remove_unused_content_repository.sql")
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Exec(string(data)).Error; err != nil {
		t.Fatal("upgrade with existing content", err)
	}
	if db.Migrator().HasTable("operations_retention_items") || db.Migrator().HasTable("operations_retention_access") {
		t.Fatal("unused repository survived")
	}
	var kept Notification
	if err = db.First(&kept, "id=?", notice).Error; err != nil || kept.Body != "Keep this fact" {
		t.Fatal("upgrade changed real business facts", kept, err)
	}
}
