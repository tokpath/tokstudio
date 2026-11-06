package operations

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"testing"
	"time"
)

func TestPostgresFixedRiskDefaultsRunWithoutOpeningSettings(t *testing.T) {
	s, actor, _, operator, owner := opsFixture(t)
	ctx := context.Background()
	source := &metricSource{value: ledger.FromInt64(90)}
	s.Source = source
	states, err := s.RunRisks(ctx, operator)
	if err != nil || len(states) != 2 {
		t.Fatal(states, err)
	}
	rules, err := s.ListRiskRules(ctx, actor.actor, operator)
	if err != nil || len(rules) != 2 {
		t.Fatal(rules, err)
	}
	var sales RiskRule
	for _, r := range rules {
		if r.Warning.String() != "100" || r.Critical.Sign() != 0 || !r.Enabled || r.ReminderSeconds != 3600 || string(r.Recipients) != "[]" {
			t.Fatal(r)
		}
		if r.Metric == "sales_space_atg" {
			sales = r
		}
	}
	var notices []Notification
	if err = s.DB.Find(&notices).Error; err != nil || len(notices) != 1 || notices[0].RecipientID != owner || notices[0].Severity != "warning" {
		t.Fatal(notices, err)
	}
	_, err = s.RunRisks(ctx, operator)
	if err != nil {
		t.Fatal(err)
	}
	var count int64
	s.DB.Model(&Notification{}).Count(&count)
	if count != 1 {
		t.Fatal("duplicate", count)
	}
	// Warning age does not invent a critical event; zero is the critical boundary.
	s.DB.Model(&RiskState{}).Where("rule_id=?", sales.ID).Update("opened_at", time.Now().Add(-48*time.Hour))
	states, err = s.RunRisks(ctx, operator)
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range states {
		if state.RuleID == sales.ID && state.Status != "warning" {
			t.Fatal(state)
		}
	}
	s.DB.Model(&RiskState{}).Where("rule_id=?", sales.ID).Update("last_notified_at", time.Now().Add(-2*time.Hour))
	if _, err = s.RunRisks(ctx, operator); err != nil {
		t.Fatal(err)
	}
	source.value = ledger.Zero()
	if _, err = s.RunRisks(ctx, operator); err != nil {
		t.Fatal(err)
	}
	source.value = ledger.FromInt64(101)
	if _, err = s.RunRisks(ctx, operator); err != nil {
		t.Fatal(err)
	}
	s.DB.Model(&Notification{}).Count(&count)
	if count != 4 {
		t.Fatal("initial,repeat,critical,recovery", count)
	}
	sales.Warning = ledger.MustDecimal("10.000000000000000001")
	saved, err := s.SaveRiskRule(ctx, actor.actor, sales, sales.Version)
	if err != nil || saved.ID != sales.ID || saved.Version != 2 {
		t.Fatal(saved, err)
	}
	var state RiskState
	if err = s.DB.Where("rule_id=?", sales.ID).Take(&state).Error; err != nil || state.LastNotifiedAt == nil {
		t.Fatal("saving settings erased the last actual notification", state, err)
	}
	if _, err = s.SaveRiskRule(ctx, actor.actor, sales, sales.Version); !errors.Is(err, ErrConflict) {
		t.Fatal("stale revision", err)
	}
	saved.Recipients = json.RawMessage(`["` + uuid.NewString() + `"]`)
	if _, err = s.SaveRiskRule(ctx, actor.actor, saved, saved.Version); !errors.Is(err, ErrInvalid) {
		t.Fatal("foreign recipient", err)
	}
	sales.Metric = "pending_usage"
	if _, err = s.SaveRiskRule(ctx, actor.actor, sales, sales.Version); !errors.Is(err, ErrInvalid) {
		t.Fatal("removed metric", err)
	}
	if _, err = s.RunRisks(ctx, uuid.NewString()); err != nil {
		t.Fatal(err)
	}
}
func TestFixedRiskThresholdBoundaries(t *testing.T) {
	r := RiskRule{Direction: "below", Warning: ledger.MustDecimal("10"), Critical: ledger.Zero()}
	for _, tc := range []struct{ value, want string }{{"10", "warning"}, {"0", "critical"}, {"-1", "critical"}, {"10.000000000000000001", "normal"}} {
		if got := RiskLevel(r, ledger.MustDecimal(tc.value)); got != tc.want {
			t.Fatal(tc, got)
		}
	}
}

func TestFixedRiskMigrationClearsOnlyObsoleteRiskData(t *testing.T) {
	db := operationsDBBefore(t, "0011_fixed_risk_settings.sql")
	operator, actor, ruleID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	rule := RiskRule{OperatorID: operator, ID: ruleID, Name: "Old technical rule", Metric: "pending_usage", Warning: ledger.FromInt64(10), Critical: ledger.FromInt64(20), Direction: "above", Recipients: json.RawMessage(`[]`), EscalationRecipients: json.RawMessage(`[]`), ReminderSeconds: 60, EscalateSeconds: 60, Enabled: true, Version: 1, UpdatedBy: actor, UpdatedAt: time.Now()}
	if err := db.Create(&rule).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&RiskState{OperatorID: operator, RuleID: ruleID, Status: "warning", Value: ledger.FromInt64(15)}).Error; err != nil {
		t.Fatal(err)
	}
	for _, reference := range []string{"risk:" + ruleID, "purchase:retained"} {
		id := uuid.NewString()
		if err := db.Exec("INSERT INTO operations_notifications(id,operator_id,recipient_id,business_key,category,severity,title,body,reference,fingerprint,created_at) VALUES(?,?,?,?,'mandatory','warning','notice','body',?,'test',clock_timestamp())", id, operator, actor, reference, reference).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Exec("INSERT INTO operations_deliveries(notification_id,message_ciphertext,state,next_attempt_at) VALUES(?,'test','pending',clock_timestamp())", id).Error; err != nil {
			t.Fatal(err)
		}
	}
	sql, err := migrations.ReadFile("migrate/0011_fixed_risk_settings.sql")
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Exec(string(sql)).Error; err != nil {
		t.Fatal(err)
	}
	for table, want := range map[string]int64{"operations_risk_rules": 0, "operations_risk_states": 0, "operations_notifications": 1, "operations_deliveries": 1} {
		var count int64
		if err = db.Table(table).Count(&count).Error; err != nil || count != want {
			t.Fatal(table, count, want, err)
		}
	}
	var notice Notification
	if err = db.Take(&notice).Error; err != nil || notice.Reference != "purchase:retained" {
		t.Fatal(notice, err)
	}
}
