package operations

import (
	"context"

	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"gorm.io/gorm"
)

type CommerceAudience interface {
	EventAudience(context.Context, *gorm.DB, commerce.Event) (string, string, string, error)
}

// CommerceNotifications composes with other transactional outbox consumers.
// It resolves only the aggregate owner; raw event payloads never become mail.
type CommerceNotifications struct {
	Operations *Service
	Audience   CommerceAudience
}

var commerceMessages = map[string][2]string{
	"order.created":                 {"订单已创建", "请查看订单详情核对当前付款和交付状态。"},
	"order.closed":                  {"订单已关闭", "订单已关闭；已核实但未匹配的款项会单独处理。"},
	"order.receipt_allocated":       {"收款已关联订单", "已核实收款已关联订单，请查看订单详情确认剩余应付与交付状态。"},
	"order.delivered":               {"订单权益已交付", "订单权益已交付，请查看资产明细与有效期。"},
	"manual_gift.delivered":         {"独立赠送已发放", "独立赠送已发放，请在资产来源核对金额与有效期；不计作充值或有偿消费。"},
	"renewal.authorized":            {"自动续费已授权", "自动续费授权已记录，可在续费设置中关闭。"},
	"renewal.cancelled":             {"自动续费已关闭", "已关闭后续自动续费；当前权益继续按原期限有效。"},
	"renewal.reconfirmed":           {"续费条件已确认", "新的续费条件已确认，请在续费详情核对。"},
	"renewal.confirmation_required": {"续费条件需要重新确认", "续费条件发生变更，确认前不会按变更后的条件自动扣款。"},
	"renewal.insufficient_funds":    {"续费资金不足", "自动续费因可用资金不足暂停，请查看续费详情。"},
}

func HasCommerceNotification(kind string) bool {
	_, ok := commerceMessages[kind]
	return ok
}

func (c CommerceNotifications) EnqueueCommerceEvent(ctx context.Context, tx *gorm.DB, e commerce.Event) error {
	if c.Operations == nil || c.Audience == nil {
		return ErrNotReady
	}
	message, ok := commerceMessages[e.Kind]
	if !ok {
		return nil
	}
	customer, operator, reference, err := c.Audience.EventAudience(ctx, tx, e)
	if err != nil {
		return err
	}
	if customer == "" {
		return nil
	}
	if !validID(customer) {
		return ErrInvalid
	}
	_, err = c.Operations.Emit(ctx, tx, Notification{OperatorID: operator, RecipientID: customer, BusinessKey: "commerce:" + e.ID, Category: "mandatory", Severity: "info", Title: message[0], Body: message[1], Reference: reference})
	return err
}
