package operations

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"slices"
	"strconv"
	"strings"
	"time"
)

type RiskRule struct {
	OperatorID           string          `gorm:"primaryKey" json:"operator_id"`
	ID                   string          `gorm:"primaryKey" json:"id"`
	Name                 string          `json:"name"`
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

var RiskMetrics = []string{"sales_space_atg", "procurement_available_atg"}

type RiskScope struct {
	Eligible      bool
	Owners, Staff []string
}
type RiskDirectory interface {
	RiskScope(context.Context, *gorm.DB, string) (RiskScope, error)
}

func (s *Service) riskScope(ctx context.Context, tx *gorm.DB, operator string) (RiskScope, error) {
	d, ok := s.Recipients.(RiskDirectory)
	if !ok {
		return RiskScope{}, ErrNotReady
	}
	return d.RiskScope(ctx, tx, operator)
}

// Empty recipients follows the current active owners; it does not freeze a
// departed owner's identity into a new operator's default configuration.
func (s *Service) EnsureRiskRules(ctx context.Context, tx *gorm.DB, operator string) error {
	return s.within(ctx, tx, func(t *gorm.DB) error {
		scope, err := s.riskScope(ctx, t, operator)
		if err != nil {
			return err
		}
		if !scope.Eligible {
			return ErrForbidden
		}
		for _, metric := range RiskMetrics {
			r := RiskRule{OperatorID: operator, ID: uuid.NewString(), Name: RiskMetricName(metric) + "预警", Metric: metric, Warning: ledger.FromInt64(100), Critical: ledger.Zero(), Direction: "below", Recipients: json.RawMessage(`[]`), EscalationRecipients: json.RawMessage(`[]`), ReminderSeconds: 3600, EscalateSeconds: 3600, Enabled: true, Version: 1, UpdatedBy: uuid.Nil.String(), UpdatedAt: time.Now().UTC()}
			if err := t.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "operator_id"}, {Name: "metric"}}, DoNothing: true}).Create(&r).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// RiskMetricName keeps the default rule name and notifications understandable
// without exposing the internal metric key to the person responding to it.
func RiskMetricName(metric string) string {
	switch metric {
	case "sales_space_atg":
		return "剩余销售空间ATG（不足时，继续卖可能超出履约能力，需要补货或限制新增销售）"
	case "procurement_available_atg":
		return "剩余采购余额ATG（不足时，实际调用可能停服，需要补货）"
	case "oversell_used_atg":
		return "已用超卖额度 ATG"
	case "pending_usage":
		return "在途及待结算请求数"
	case "unverified_costs":
		return "待核实上游成本数"
	default:
		return metric
	}
}

func (r RiskRule) DisplayName() string {
	if name := strings.TrimSpace(r.Name); name != "" {
		return name
	}
	return RiskMetricName(r.Metric) + "预警"
}

func (s *Service) SaveRiskRule(ctx context.Context, actor string, r RiskRule, expected int64) (RiskRule, error) {
	return s.SaveRiskRuleTx(ctx, nil, actor, r, expected)
}

func (s *Service) SaveRiskRuleTx(ctx context.Context, tx *gorm.DB, actor string, r RiskRule, expected int64) (RiskRule, error) {
	if expected < 1 || !slices.Contains(RiskMetrics, r.Metric) || !r.Warning.Valid() || r.Warning.Sign() <= 0 || r.ReminderSeconds < 60 || r.ReminderSeconds > 86400*30 {
		return r, ErrInvalid
	}
	var recipients []string
	if len(r.Recipients) == 0 {
		r.Recipients = json.RawMessage(`[]`)
	}
	if json.Unmarshal(r.Recipients, &recipients) != nil || len(recipients) > 50 {
		return r, ErrInvalid
	}
	r.Name, r.Direction, r.Critical = RiskMetricName(r.Metric)+"预警", "below", ledger.Zero()
	r.EscalationRecipients, r.EscalateSeconds = json.RawMessage(`[]`), 3600
	e := s.within(ctx, tx, func(t *gorm.DB) error {
		if e := s.require(ctx, t, actor, r.OperatorID, "operations.manage"); e != nil {
			return e
		}
		scope, e := s.riskScope(ctx, t, r.OperatorID)
		if e != nil {
			return e
		}
		if !scope.Eligible {
			return ErrForbidden
		}
		for _, id := range recipients {
			if !slices.Contains(scope.Staff, id) {
				return ErrInvalid
			}
		}
		if e = lock(t, "risks:"+r.OperatorID); e != nil {
			return e
		}
		if e = s.EnsureRiskRules(ctx, t, r.OperatorID); e != nil {
			return e
		}
		var old RiskRule
		if e = t.Where("operator_id=? AND metric=?", r.OperatorID, r.Metric).Take(&old).Error; e != nil {
			return e
		}
		if old.Version != expected || r.ID != "" && r.ID != old.ID {
			return ErrConflict
		}
		r.ID, r.Version, r.UpdatedBy, r.UpdatedAt = old.ID, expected+1, actor, time.Now().UTC()
		return t.Save(&r).Error
	})
	return r, e
}

func (s *Service) ListRiskRules(ctx context.Context, actor, operator string) ([]RiskRule, error) {
	out := []RiskRule{}
	e := s.within(ctx, nil, func(t *gorm.DB) error {
		if e := s.require(ctx, t, actor, operator, "operations.manage"); e != nil {
			return e
		}
		if e := s.EnsureRiskRules(ctx, t, operator); e != nil {
			return e
		}
		return t.Where("operator_id=?", operator).Order("metric DESC").Find(&out).Error
	})
	return out, e
}
func (s *Service) ListRisks(ctx context.Context, actor, operator string) ([]RiskState, error) {
	out := []RiskState{}
	e := s.within(ctx, nil, func(t *gorm.DB) error {
		if e := s.require(ctx, t, actor, operator, "operations.manage"); e != nil {
			if e = s.require(ctx, t, actor, operator, "reports.view"); e != nil {
				return e
			}
		}
		return t.Where("operator_id=?", operator).Order("rule_id").Find(&out).Error
	})
	return out, e
}
func RiskLevel(r RiskRule, v ledger.Decimal) string {
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
		scope, e := s.riskScope(ctx, t, operator)
		if e != nil {
			return e
		}
		if !scope.Eligible {
			return nil
		}
		if e = s.EnsureRiskRules(ctx, t, operator); e != nil {
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
			next := RiskLevel(r, v)
			if next != "normal" && previous == "normal" {
				state.Incident++
				state.OpenedAt = &now
			}
			notify := next != previous || (next != "normal" && (state.LastNotifiedAt == nil || now.Sub(*state.LastNotifiedAt) >= time.Duration(r.ReminderSeconds)*time.Second))
			state.Status = next
			state.Value = v
			if notify {
				state.Sequence++
				recipients := []string{}
				_ = json.Unmarshal(r.Recipients, &recipients)
				recipients = slices.DeleteFunc(recipients, func(id string) bool { return !slices.Contains(scope.Staff, id) })
				if len(recipients) == 0 {
					recipients = scope.Owners
				}
				if len(recipients) == 0 {
					continue
				}

				seen := map[string]bool{}
				severity := next
				if severity == "normal" {
					severity = "info"
				}
				title := "经营预警：" + r.DisplayName()
				if next == "normal" {
					title = "经营预警已恢复：" + r.DisplayName()
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
