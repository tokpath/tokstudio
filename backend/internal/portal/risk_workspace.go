package portal

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"github.com/tokpath/tokstudio/backend/internal/cooperation"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/operations"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"gorm.io/gorm"
)

func (d recipientDirectory) RiskScope(ctx context.Context, tx *gorm.DB, operator string) (operations.RiskScope, error) {
	op, err := d.Application.Organization.GetOperator(ctx, tx, operator)
	if err != nil {
		return operations.RiskScope{}, err
	}
	scope := operations.RiskScope{Eligible: op.Kind != organization.Platform}
	if !scope.Eligible {
		return scope, nil
	}
	if tx == nil {
		tx = d.Application.DB
	}
	var rows []struct{ ID, Role string }
	err = tx.WithContext(ctx).Table("portal_staff s").Select("s.id,m.role").Joins("JOIN organization_staff_memberships m ON m.staff_id=s.id").Where("m.operator_id=? AND m.status='active' AND s.status='active'", operator).Order("s.id").Scan(&rows).Error
	for _, row := range rows {
		scope.Staff = append(scope.Staff, row.ID)
		if row.Role == "owner" {
			scope.Owners = append(scope.Owners, row.ID)
		}
	}
	return scope, err
}

// Two fixed settings and their current measurement share one page. Reading
// never emits notifications; the regular worker evaluates them independently.
func (a *Application) riskWorkspace(c *gin.Context, op organization.Operator, actor, resource, base string, p gin.H) {
	ctx := c.Request.Context()
	rules, err := a.Operations.ListRiskRules(ctx, actor, op.ID)
	if err != nil {
		handleError(c, err)
		return
	}
	choices, _, err := a.riskStaffChoices(ctx, op.ID)
	if err != nil {
		handleError(c, err)
		return
	}
	scope, err := (recipientDirectory{a}).RiskScope(ctx, nil, op.ID)
	if err != nil {
		handleError(c, err)
		return
	}
	coverage, err := a.Cooperation.Coverage(ctx, nil, op.ID, time.Now())
	unavailable := errors.Is(err, cooperation.ErrCoverage) || errors.Is(err, catalog.ErrPriceUnavailable) || errors.Is(err, catalog.ErrUnboundedCoverage)
	if err != nil && !unavailable {
		handleError(c, err)
		return
	}
	metrics := map[string]ledger.Decimal{"sales_space_atg": coverage.Space, "procurement_available_atg": coverage.ProcurementAvailable}
	states, err := a.Operations.ListRisks(ctx, actor, op.ID)
	if err != nil {
		handleError(c, err)
		return
	}
	items := []any{}
	for _, rule := range rules {
		v := gin.H{"metric": rule.Metric, "warning": rule.Warning, "recipients": rule.Recipients, "reminder_minutes": riskMinutes(rule.ReminderSeconds), "enabled": rule.Enabled, "staff_choices": choices, "default_owner_ids": scope.Owners, "observed_at": time.Now().UTC()}
		status := "pending_configuration"
		if !unavailable {
			v["value"] = metrics[rule.Metric]
			status = operations.RiskLevel(rule, metrics[rule.Metric])
		}
		if !rule.Enabled {
			status = "disabled"
		}
		for _, state := range states {
			if state.RuleID == rule.ID {
				v["last_notified_at"] = state.LastNotifiedAt
			}
		}
		items = append(items, entity(rule.ID, status, rule.Version, v))
	}
	p["title"] = "风险预警"
	p["description"] = "两项补货预警默认已启用，可直接调整。仅提醒本经营方，不会自动补货或限制销售。"
	p["items"], p["columns"], p["operations"] = items, []gin.H{}, []operation{}
	p["row_operations"] = []operation{{Name: "risk_settings", Label: "保存预警设置", Method: "POST", Path: base, Permission: "operations.manage", Fields: businessFields(operation{Name: "risk_settings", Path: base, Fields: []field{jsonField("预警设置")}})}}
	p["search_supported"], p["status_supported"] = false, false
	a.resourceResponse(c, p)
}
func riskMinutes(seconds int64) string { return strconv.FormatFloat(float64(seconds)/60, 'f', -1, 64) }
func riskSeconds(minutes ledger.Decimal) (int64, bool) {
	if minutes.Cmp(ledger.FromInt64(1)) < 0 || minutes.Cmp(ledger.FromInt64(43200)) > 0 {
		return 0, false
	}
	value, _ := minutes.Rat().Float64()
	return int64(math.Round(value * 60)), true
}

type riskRuleInput struct {
	Metric          string          `json:"metric"`
	Warning         ledger.Decimal  `json:"warning"`
	Recipients      json.RawMessage `json:"recipients"`
	ReminderMinutes ledger.Decimal  `json:"reminder_minutes"`
	Enabled         bool            `json:"enabled"`
	ExpectedVersion int64           `json:"expected_version"`
}

func (in riskRuleInput) rule(operator string) (operations.RiskRule, bool) {
	reminder, ok := riskSeconds(in.ReminderMinutes)
	return operations.RiskRule{OperatorID: operator, Metric: in.Metric, Warning: in.Warning, Recipients: in.Recipients, ReminderSeconds: reminder, Enabled: in.Enabled}, ok
}

func (a *Application) riskStaffChoices(ctx context.Context, operator string) ([]gin.H, map[string]string, error) {
	var rows []struct {
		ID, DisplayName, Email, Status, MembershipStatus string
	}
	err := a.DB.WithContext(ctx).Table("portal_staff AS s").Select("s.id,s.display_name,s.email,s.status,m.status AS membership_status").Joins("JOIN organization_staff_memberships AS m ON m.staff_id=s.id").Where("m.operator_id=?", operator).Order("s.display_name,s.email,s.id").Scan(&rows).Error
	if err != nil {
		return nil, nil, err
	}
	choices := []gin.H{}
	names := map[string]string{}
	for _, row := range rows {
		label := strings.TrimSpace(row.DisplayName)
		if label == "" {
			label = row.Email
		} else if label != row.Email && row.Email != "" {
			label += " · " + row.Email
		}
		names[row.ID] = label
		if row.Status == "active" && row.MembershipStatus == "active" {
			choices = append(choices, gin.H{"value": row.ID, "label": label})
		}
	}
	return choices, names, nil
}
