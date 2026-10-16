package operations

import (
	"context"
	"github.com/tokpath/tokstudio/backend/internal/cooperation"
	"gorm.io/gorm"
)

func (s *Service) RecordReplenishmentNotice(ctx context.Context, tx *gorm.DB, n cooperation.ReplenishmentNotice) error {
	title := "采购自动补货待办"
	body := "采购订单已创建，请核对付款与交付状态。"
	if n.Kind == "authorization_blocked" {
		title = "自动补货未能发起"
		body = "当前报价、授权或周期限额不允许发起自动补货，请核对经营设置。"
	} else if n.Kind == "pending" {
		title = "自动补货等待核实"
		body = "原采购付款或交付尚未核实，系统持续核对原记录，不会重复发起收款。原因：" + n.Reason
	}
	_, e := s.Emit(ctx, tx, Notification{OperatorID: n.BuyerID, RecipientID: n.RecipientID, BusinessKey: "replenishment:" + n.AttemptID + ":" + n.Kind + ":" + n.Reason, Category: "mandatory", Severity: "warning", Title: title, Body: body, Reference: "purchase-order:" + n.OrderID})
	return e
}
