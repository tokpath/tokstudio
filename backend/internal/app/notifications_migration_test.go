package app_test

import (
	"errors"
	"fmt"
	"io/fs"
	"testing"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/outbox"
	"github.com/tokpath/tokstudio/backend/internal/plans"
	"gorm.io/gorm"
)

func TestNotificationMigrationPreservesLegacyReceiptsAndRemovesGrace(t *testing.T) {
	a, _ := newOAuthEnv(t)
	_, outboxFS := outbox.Migrations()
	_, plansFS := plans.Migrations()
	rollback := errors.New("rollback isolated migration fixture")
	err := a.DB.Transaction(func(tx *gorm.DB) error {
		schema := fmt.Sprintf("notification_migration_%d", time.Now().UnixNano())
		if err := tx.Exec(`CREATE SCHEMA "` + schema + `"`).Error; err != nil {
			return err
		}
		if err := tx.Exec(`SET LOCAL search_path TO "` + schema + `"`).Error; err != nil {
			return err
		}
		apply := func(source fs.FS, name string) error {
			body, err := fs.ReadFile(source, name)
			if err != nil {
				return err
			}
			return tx.Exec(string(body)).Error
		}
		if err := apply(outboxFS, "0001_init.sql"); err != nil {
			return err
		}
		if err := tx.Exec(`INSERT INTO outbox_events(id,event_type,aggregate_type,aggregate_id,payload_json) VALUES ('legacy-event','legacy.kind','legacy','legacy','{}');
            INSERT INTO outbox_consumptions(event_id,consumer) VALUES ('legacy-event','legacy-reader')`).Error; err != nil {
			return err
		}
		if err := apply(outboxFS, "0002_consumers.sql"); err != nil {
			return err
		}
		if err := tx.Exec(`INSERT INTO outbox_consumptions(event_id,consumer) VALUES ('legacy-event','notifications-v1')`).Error; err != nil {
			return err
		}
		var count int64
		if err := tx.Raw("SELECT COUNT(*) FROM outbox_consumptions").Scan(&count).Error; err != nil {
			return err
		}
		if count != 2 {
			return errors.New("legacy receipt lost or independent consumer rejected")
		}
		if err := tx.Exec(`CREATE TABLE plans_subscriptions(id TEXT PRIMARY KEY,status TEXT,renewal_policy TEXT,payment_adapter TEXT,retry_count INT,next_retry_at TIMESTAMPTZ,grace_until TIMESTAMPTZ);
            INSERT INTO plans_subscriptions VALUES
              ('manual','past_due','manual','alipay',0,now()+interval '1 day',now()+interval '7 days'),
              ('exhausted','past_due','auto','stripe',4,now()+interval '1 day',now()+interval '7 days'),
              ('retry','past_due','auto','stripe',1,now()+interval '1 day',now()+interval '7 days')`).Error; err != nil {
			return err
		}
		if err := apply(plansFS, "0006_no_grace.sql"); err != nil {
			return err
		}
		if err := tx.Raw("SELECT COUNT(*) FROM plans_subscriptions WHERE grace_until IS NOT NULL OR (id<>'retry' AND next_retry_at IS NOT NULL)").Scan(&count).Error; err != nil {
			return err
		}
		if count != 0 {
			return errors.New("migration retained a grace date or invalid payment retry")
		}
		if err := tx.Raw("SELECT COUNT(*) FROM plans_subscriptions WHERE id='retry' AND next_retry_at IS NOT NULL").Scan(&count).Error; err != nil {
			return err
		}
		if count != 1 {
			return errors.New("migration removed a scheduled legitimate payment retry")
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal(err)
	}
}
