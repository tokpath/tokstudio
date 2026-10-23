package operations

import (
	"context"
	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"gorm.io/gorm"
	"time"
)

func (s *Service) RecordCatalogNotice(ctx context.Context, tx *gorm.DB, c catalog.CatalogChange, operator, recipient string) error {
	title := "模型与计价计划变更"
	body := "公开模型 " + c.ResourceID + " 的 " + c.Kind + " 变更计划于 " + c.EffectiveAt.UTC().Format(time.RFC3339) + " 生效。请在模型目录或合作报价中查看具体条件；已受理请求继续按原快照结算。"
	if c.Evidence == "system:catalog-publication" {
		body = "公开模型 " + c.ResourceID + " 的价格或使用条件即将更新，最早于 " + c.EffectiveAt.UTC().Format(time.RFC3339) + " 生效；系统完成通知后保留提前通知时间，如通知延迟则顺延。具体状态与价格可在模型目录或合作报价中查看，已受理请求继续按原快照结算。"
	}
	severity := "info"
	if c.Kind == "emergency_model_disable" {
		title = "模型紧急停用"
		body = "公开模型 " + c.ResourceID + " 已紧急停止受理新请求。已受理请求按原规则处理；可在模型目录查看状态。"
		severity = "critical"
	}
	_, e := s.Emit(ctx, tx, Notification{OperatorID: operator, RecipientID: recipient, BusinessKey: "catalog:" + c.ID, Category: "mandatory", Severity: severity, Title: title, Body: body, Reference: "catalog-change:" + c.ID})
	return e
}
