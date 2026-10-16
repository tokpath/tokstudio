package operations

import (
	"context"
	"encoding/json"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strconv"
	"time"
)

type RiskRule struct {
	OperatorID           string          `gorm:"primaryKey" json:"operator_id"`
	ID                   string          `gorm:"primaryKey" json:"id"`
	Metric               string          `json:"metric"`
	Warning              ledger.Decimal  `json:"warning"`
	Critical             ledger.Decimal  `json:"critical"`
	Direction            string          `json:"direction"`
	Recipients           json.RawMessage `gorm:"type:jsonb" json:"recipients"`
	EscalationRecipients json.RawMessage `gorm:"type:jsonb" json:"escalation_recipients"`
	ReminderSeconds      int64           `json:"reminder_seconds"`
	EscalateSeconds      int64           `json:"escalate_seconds"`
	Enabled              bool            `json:"enabled"`
	Version              int64           `json:"version"`
	UpdatedBy            string          `json:"updated_by"`
	UpdatedAt            time.Time       `json:"updated_at"`
}

func (RiskRule) TableName() string { return "operations_risk_rules" }

type RiskState struct {
	OperatorID     string         `gorm:"primaryKey" json:"operator_id"`
	RuleID         string         `gorm:"primaryKey" json:"rule_id"`
	Status         string         `json:"status"`
	Value          ledger.Decimal `json:"value"`
	Incident       int64          `json:"incident"`
	OpenedAt       *time.Time     `json:"opened_at,omitempty"`
	LastNotifiedAt *time.Time     `json:"last_notified_at,omitempty"`
	Sequence       int64          `json:"sequence"`
}

func (RiskState) TableName() string { return "operations_risk_states" }

var riskMetrics = map[string]bool{"sales_space_atg": true, "procurement_available_atg": true, "oversell_used_atg": true, "pending_usage": true, "unverified_costs": true}

func (s *Service) SaveRiskRule(ctx context.Context, actor string, r RiskRule, expected int64) (RiskRule, error) {
	if r.ID == "" || len(r.ID) > 100 || !riskMetrics[r.Metric] || r.ReminderSeconds < 60 || r.ReminderSeconds > 86400*30 || r.EscalateSeconds < 60 || r.EscalateSeconds > 86400*30 || !r.Warning.Valid() || !r.Critical.Valid() {
		return r, ErrInvalid
	}
	if (r.Direction == "below" && r.Critical.Cmp(r.Warning) > 0) || (r.Direction == "above" && r.Critical.Cmp(r.Warning) < 0) || (r.Direction != "below" && r.Direction != "above") {
		return r, ErrInvalid
	}
	recipients := []string{}
	escalation := []string{}
	if json.Unmarshal(r.Recipients, &recipients) != nil || json.Unmarshal(r.EscalationRecipients, &escalation) != nil || len(recipients) == 0 || len(recipients) > 50 || len(escalation) > 50 {
		return r, ErrInvalid
	}
	e := s.within(ctx, nil, func(t *gorm.DB) error {
		if e := s.require(ctx, t, actor, r.OperatorID, "operations.manage"); e != nil {
			return e
		}
		for _, id := range append(recipients, escalation...) {
			if _, e := s.self(ctx, t, id, r.OperatorID); e != nil {
				return e
			}
		}
		if e := lock(t, "risk:"+r.OperatorID+r.ID); e != nil {
			return e
		}
		var old RiskRule
		q := t.Where("operator_id=? AND id=?", r.OperatorID, r.ID).Limit(1).Find(&old)
		if q.Error != nil {
			return q.Error
		}
		if (q.RowsAffected == 0 && expected != 0) || (q.RowsAffected > 0 && old.Version != expected) {
			return ErrConflict
		}
		r.Version = expected + 1
		r.UpdatedBy = actor
		r.UpdatedAt = time.Now().UTC()
		return t.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "operator_id"}, {Name: "id"}}, UpdateAll: true}).Create(&r).Error
	})
	return r, e
}
func (s *Service) ListRiskRules(ctx context.Context, actor, operator string) ([]RiskRule, error) {
	out := []RiskRule{}
	e := s.within(ctx, nil, func(t *gorm.DB) error {
		if e := s.require(ctx, t, actor, operator, "operations.manage"); e != nil {
			return e
		}
		return t.Where("operator_id=?", operator).Order("id").Find(&out).Error
	})
	return out, e
}
func (s *Service) ListRisks(ctx context.Context, actor, operator string) ([]RiskState, error) {
	out := []RiskState{}
	e := s.within(ctx, nil, func(t *gorm.DB) error {
		if e := s.require(ctx, t, actor, operator, "reports.view"); e != nil {
			return e
		}
		return t.Where("operator_id=?", operator).Order("rule_id").Find(&out).Error
	})
	return out, e
}
func riskLevel(r RiskRule, v ledger.Decimal) string {
	if r.Direction == "below" {
		if v.Cmp(r.Critical) <= 0 {
			return "critical"
		}
		if v.Cmp(r.Warning) <= 0 {
			return "warning"
		}
	} else {
		if v.Cmp(r.Critical) >= 0 {
			return "critical"
		}
		if v.Cmp(r.Warning) >= 0 {
			return "warning"
		}
	}
	return "normal"
}

// RunRisks takes actual current coverage and metering metrics. Rule thresholds
// never modify the underlying sales/call authorization or customer assets.
func (s *Service) RunRisks(ctx context.Context, operator string) ([]RiskState, error) {
	if s.Source == nil {
		return nil, ErrNotReady
	}
	out := []RiskState{}
	e := s.within(ctx, nil, func(t *gorm.DB) error {
		if e := lock(t, "risks:"+operator); e != nil {
			return e
		}
		now := time.Now().UTC()
		metrics, e := s.Source.Metrics(ctx, t, operator, now)
		if e != nil {
			return e
		}
		rules := []RiskRule{}
		if e = t.Where("operator_id=? AND enabled", operator).Order("id").Find(&rules).Error; e != nil {
			return e
		}
		for _, r := range rules {
			v, ok := metrics[r.Metric]
			if !ok {
				return ErrNotReady
			}
			var state RiskState
			q := t.Where("operator_id=? AND rule_id=?", operator, r.ID).Limit(1).Find(&state)
			if q.Error != nil {
				return q.Error
			}
			if q.RowsAffected == 0 {
				state = RiskState{OperatorID: operator, RuleID: r.ID, Status: "normal"}
			}
			previous := state.Status
			next := riskLevel(r, v)
			if next != "normal" && previous == "normal" {
				state.Incident++
				state.OpenedAt = &now
			}
			if next == "warning" && state.OpenedAt != nil && now.Sub(*state.OpenedAt) >= time.Duration(r.EscalateSeconds)*time.Second {
				next = "critical"
			}
			notify := next != previous || (next != "normal" && (state.LastNotifiedAt == nil || now.Sub(*state.LastNotifiedAt) >= time.Duration(r.ReminderSeconds)*time.Second))
			state.Status = next
			state.Value = v
			if notify {
				state.Sequence++
				recipients := []string{}
				_ = json.Unmarshal(r.Recipients, &recipients)
				if next == "critical" || previous == "critical" {
					extra := []string{}
					_ = json.Unmarshal(r.EscalationRecipients, &extra)
					recipients = append(recipients, extra...)
				}
				seen := map[string]bool{}
				severity := next
				if severity == "normal" {
					severity = "info"
				}
				title := "经营预警：" + r.Metric
				if next == "normal" {
					title = "经营预警已恢复：" + r.Metric
				}
				for _, id := range recipients {
					if seen[id] {
						continue
					}
					seen[id] = true
					_, e = s.Emit(ctx, t, Notification{OperatorID: operator, RecipientID: id, BusinessKey: "risk:" + r.ID + ":" + strconv.FormatInt(state.Incident, 10) + ":" + strconv.FormatInt(state.Sequence, 10), Category: "mandatory", Severity: severity, Title: title, Body: "当前值：" + v.String() + "；状态：" + next + "。请核对经营资源与待办事项。", Reference: "risk:" + r.ID})
					if e != nil {
						return e
					}
				}
				state.LastNotifiedAt = &now
			}
			if next == "normal" {
				state.OpenedAt = nil
			}
			if e = t.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "operator_id"}, {Name: "rule_id"}}, UpdateAll: true}).Create(&state).Error; e != nil {
				return e
			}
			out = append(out, state)
		}
		return nil
	})
	return out, e
}
