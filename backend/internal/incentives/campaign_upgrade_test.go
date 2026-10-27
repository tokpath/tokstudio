package incentives

import (
	"io/fs"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestPostgresCampaignUpgradePreservesPublishedFacts(t *testing.T) {
	const migration = "0006_campaign_simplicity.sql"
	f := setupOnDB(t, integrationDBBefore(t, migration))
	campaign, version := uuid.NewString(), uuid.NewString()
	if err := f.db.Exec(`INSERT INTO incentives_campaigns(id,operator_id,kind,current_version) VALUES(?,?,'merchant',1)`, campaign, f.operator).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.db.Exec(`INSERT INTO incentives_campaign_versions
		(id,campaign_id,version,package_id,enabled,starts_at,ends_at,eligibility,final_price_usd,gift_atg,concession_rate,per_customer_limit,total_use_limit,gift_budget_atg,discount_budget_usd,created_at)
		VALUES(?,?,1,?,true,?,?,'any',80,10,0,2,20,1000,1000,?)`, version, campaign, f.quote.PackageID, time.Now().Add(-time.Hour), time.Now().Add(time.Hour), time.Now()).Error; err != nil {
		t.Fatal(err)
	}
	var before string
	if err := f.db.Raw(`SELECT (to_jsonb(v)-'gift_budget_atg'-'discount_budget_usd')::text FROM incentives_campaign_versions v WHERE id=?`, version).Scan(&before).Error; err != nil {
		t.Fatal(err)
	}
	_, migrations := Migrations()
	body, err := fs.ReadFile(migrations, migration)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.db.Transaction(func(tx *gorm.DB) error { return tx.Exec(string(body)).Error }); err != nil {
		t.Fatalf("upgrade with an existing immutable campaign failed: %v", err)
	}
	var after string
	if err = f.db.Raw(`SELECT (to_jsonb(v)-'name')::text FROM incentives_campaign_versions v WHERE id=?`, version).Scan(&after).Error; err != nil || after != before {
		t.Fatalf("published economic facts changed: before=%s after=%s err=%v", before, after, err)
	}
	rows, err := f.s.CampaignsForOperator(f.ctx, nil, f.owner, f.operator)
	if err != nil || len(rows) != 1 || rows[0].Version.Name != "Monthly" {
		t.Fatalf("package-name fallback missing: %+v %v", rows, err)
	}
	customer := f.register(t, "upgrade-customer@example.test", "")
	offers, err := f.s.EligibleActivities(f.ctx, nil, f.operator, customer.Customer.ID, f.quote.PackageID)
	if err != nil || len(offers) != 1 || offers[0].Name != "Monthly" || !offers[0].Eligible {
		t.Fatalf("customer cannot use the existing offer after upgrade: %+v %v", offers, err)
	}
	for _, sql := range []string{`UPDATE incentives_campaign_versions SET gift_atg=99 WHERE id=?`, `DELETE FROM incentives_campaign_versions WHERE id=?`} {
		if err = f.db.Exec(sql, version).Error; err == nil || !strings.Contains(err.Error(), "immutable") {
			t.Fatalf("published-version protection lost: %v", err)
		}
	}
	if _, err = f.s.PublishCampaign(f.ctx, nil, CampaignInput{ActorStaffID: f.owner, OperatorID: f.operator, Kind: "merchant", Name: "New offer", PackageID: f.quote.PackageID, Eligibility: "any", Enabled: true, StartsAt: time.Now().Add(-time.Minute), EndsAt: time.Now().Add(time.Hour), GiftATG: d("5"), PerCustomerLimit: 1}); err != nil {
		t.Fatalf("new offers cannot be published after upgrade: %v", err)
	}
}
