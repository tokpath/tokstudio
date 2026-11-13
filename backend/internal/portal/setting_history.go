package portal

import (
	"context"
	"encoding/json"
	"time"

	"github.com/gin-gonic/gin"
)

type settingChange struct {
	Actor   string    `json:"actor"`
	At      time.Time `json:"at"`
	Summary []string  `json:"summary"`
}

type settingEvent struct {
	Subject, Actor string
	CreatedAt      time.Time
	Payload        json.RawMessage
}

func (a *Application) businessSettingHistory(c *gin.Context, operator string) ([]settingChange, error) {
	var rows []settingEvent
	err := a.DB.WithContext(c.Request.Context()).Table("organization_events e").
		Select("COALESCE(NULLIF(s.display_name,''),s.email,'') AS actor,e.created_at,e.payload").
		Joins("LEFT JOIN portal_staff s ON s.id=e.actor_id").
		Where("e.operator_id=? AND e.event_type='business.status_changed'", operator).
		Order("e.created_at DESC,e.id DESC").Limit(20).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	changes, err := presentSettingChanges(rows, true)
	if err != nil {
		return nil, err
	}
	return append([]settingChange{}, changes[""]...), nil
}

// Called after ListMemberships checks the workspace's read permission.
func (a *Application) staffSettingHistory(ctx context.Context, operator string, staffIDs []string) (map[string][]settingChange, error) {
	var rows []settingEvent
	if len(staffIDs) == 0 {
		return map[string][]settingChange{}, nil
	}
	query := a.DB.WithContext(ctx).Table("organization_events e").
		Select(`e.payload->>'StaffID' AS subject, COALESCE(NULLIF(s.display_name,''),s.email,'') AS actor,
			e.created_at,e.payload,row_number() OVER (PARTITION BY e.payload->>'StaffID' ORDER BY e.created_at DESC,e.id DESC) AS position`).
		Joins("LEFT JOIN portal_staff s ON s.id=e.actor_id").
		Where("e.operator_id=? AND e.event_type='staff.membership_changed' AND e.payload->>'StaffID' IN ?", operator, staffIDs)
	if err := a.DB.WithContext(ctx).Table("(?) AS history", query).Where("position<=20").Order("subject,position").Scan(&rows).Error; err != nil {
		return nil, err
	}
	return presentSettingChanges(rows, false)
}

func presentSettingChanges(rows []settingEvent, business bool) (map[string][]settingChange, error) {
	out := map[string][]settingChange{}
	roles := map[string]string{"owner": "负责人", "operations": "运营", "finance": "财务", "support": "客服", "readonly": "只读", "custom": "明确自定义"}
	features := map[string]string{"registration": "新客户注册", "new_purchase": "套餐新购", "renewal": "套餐续费", "model_usage": "模型调用", "activity": "营销活动", "settlement": "资金结算"}
	for _, row := range rows {
		var payload struct {
			Role, Status, Feature string
			Enabled               bool
			Grants                []string
		}
		if err := json.Unmarshal(row.Payload, &payload); err != nil {
			return nil, err
		}
		summary := []string{}
		if business {
			state := "暂停"
			if payload.Enabled {
				state = "开启"
			}
			summary = []string{features[payload.Feature], state}
		} else {
			state := "停用"
			if payload.Status == "active" {
				state = "启用"
			}
			summary = []string{roles[payload.Role], state}
			if len(payload.Grants) > 0 {
				names := []string{}
				for _, grant := range payload.Grants {
					label := grant
					for _, choice := range staffPermissionLabels {
						if choice[0] == grant {
							label = choice[1]
							break
						}
					}
					names = append(names, label)
				}
				summary = append(summary, names...)
			}
		}
		out[row.Subject] = append(out[row.Subject], settingChange{Actor: row.Actor, At: row.CreatedAt, Summary: summary})
	}
	return out, nil
}

func (a *Application) domainHistory(c *gin.Context, brand, id string) ([]settingChange, error) {
	var rows []struct {
		Actor, Action, Status string
		CreatedAt             time.Time
	}
	err := a.DB.WithContext(c.Request.Context()).Table("branding_changes e").Select("COALESCE(NULLIF(s.display_name,''),s.email,'') AS actor,e.action,e.created_at,e.after_json->>'status' AS status").Joins("LEFT JOIN portal_staff s ON s.id=e.actor_id").Where("e.brand_id=? AND e.target_type='domain' AND e.target_id=?", brand, id).Order("e.created_at DESC,e.id DESC").Limit(20).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := []settingChange{}
	for _, row := range rows {
		label := map[string]string{"domain.requested": "申请域名验证", "domain.ownership_verified": "域名归属已验证", "domain.verified": "域名接入已验证", "domain.replaced": "域名已替换"}[row.Action]
		if row.Action == "domain.enabled_changed" {
			label = "已停用"
			if row.Status == "active" {
				label = "已启用"
			}
		}
		if label == "" {
			continue
		}
		actor := row.Actor
		out = append(out, settingChange{Actor: actor, At: row.CreatedAt, Summary: []string{label}})
	}
	return out, nil
}
