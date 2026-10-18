package operations

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
)

func TestPostgresRiskRuleGeneratesNameAndIdentityAndKeepsRevisionBoundary(t *testing.T) {
	s, actor, _, operator, recipient := opsFixture(t)
	ctx := context.Background()
	recipients, _ := json.Marshal([]string{recipient})
	in := RiskRule{OperatorID: operator, Metric: "sales_space_atg", Warning: ledger.FromInt64(10), Critical: ledger.FromInt64(2), Direction: "below", Recipients: recipients, EscalationRecipients: json.RawMessage(`[]`), ReminderSeconds: 60, EscalateSeconds: 60, Enabled: true}
	saved, err := s.SaveRiskRule(ctx, actor.actor, in, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = uuid.Parse(saved.ID); err != nil || saved.Name != "剩余销售空间 ATG预警" || saved.Version != 1 {
		t.Fatalf("rule was not generated with a useful name: %+v %v", saved, err)
	}
	saved.Name = "应用额度补货提醒"
	revised, err := s.SaveRiskRule(ctx, actor.actor, saved, 1)
	if err != nil || revised.ID != saved.ID || revised.Version != 2 {
		t.Fatalf("revision lost rule identity: %+v %v", revised, err)
	}
	revised.Name = ""
	retained, err := s.SaveRiskRule(ctx, actor.actor, revised, 2)
	if err != nil || retained.Name != "剩余销售空间 ATG预警" || retained.ID != saved.ID || retained.Version != 3 {
		t.Fatalf("blank name did not restore the documented metric default: %+v %v", retained, err)
	}
	if _, err = s.SaveRiskRule(ctx, actor.actor, saved, 1); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale edit accepted: %v", err)
	}
	if _, err = s.SaveRiskRule(ctx, actor.actor, in, 2); !errors.Is(err, ErrInvalid) {
		t.Fatalf("edit without a selected rule created another rule: %v", err)
	}
	s.Source = &metricSource{value: ledger.FromInt64(9)}
	if _, err = s.RunRisks(ctx, operator); err != nil {
		t.Fatal(err)
	}
	var notification Notification
	if err = s.DB.Where("operator_id=?", operator).Take(&notification).Error; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(notification.Title, retained.Name) || strings.Contains(notification.Title, retained.Metric) {
		t.Fatalf("notification did not use the readable rule name: %+v", notification)
	}
}

func TestRiskThresholdsIncludeTheConfiguredBoundary(t *testing.T) {
	for _, tc := range []struct {
		direction, value, warning, critical, want string
	}{
		{"below", "10", "10", "2", "warning"},
		{"below", "2", "10", "2", "critical"},
		{"below", "10.000000000000000001", "10", "2", "normal"},
		{"above", "10", "10", "20", "warning"},
		{"above", "20", "10", "20", "critical"},
		{"above", "9.999999999999999999", "10", "20", "normal"},
	} {
		got := riskLevel(RiskRule{Direction: tc.direction, Warning: ledger.MustDecimal(tc.warning), Critical: ledger.MustDecimal(tc.critical)}, ledger.MustDecimal(tc.value))
		if got != tc.want {
			t.Fatalf("%s value %s: %s want %s", tc.direction, tc.value, got, tc.want)
		}
	}
}
