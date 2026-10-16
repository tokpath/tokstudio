package operations

import (
	"context"
	"github.com/tokpath/tokstudio/backend/internal/incentives"
	"gorm.io/gorm"
)

type IncentiveAudience interface {
	NotificationEventAudience(context.Context, *gorm.DB, incentives.Event) ([]incentives.NotificationAudience, error)
}
type IncentiveNotifications struct {
	Operations *Service
	Audience   IncentiveAudience
}

func (c IncentiveNotifications) EnqueueIncentiveEvent(ctx context.Context, tx *gorm.DB, event incentives.Event) error {
	if c.Operations == nil || c.Audience == nil {
		return ErrNotReady
	}
	messages := map[string][2]string{
		"withdrawal.requested":                 {"佣金提现已申请", "申请金额已占用，审核不表示已实际打款。请查看提现状态。"},
		"withdrawal.approved":                  {"佣金提现已审核", "提现已审核，仍须核实实际付款后才结清。"},
		"withdrawal.payment_intent_recorded":   {"提现进入付款环节", "已记录唯一付款指令，结果未核实前不会重复发起。"},
		"withdrawal.outcome_unknown":           {"提现付款待核实", "原付款结果暂不能核实，已保留占用，请等待原付款核查。"},
		"withdrawal.payment_verified":          {"佣金提现付款已核实", "已核实实际提现付款，请查看提现明细与金额。"},
		"withdrawal.cancelled":                 {"佣金提现已取消", "本次提现已取消，资金处理结果请查看明细。"},
		"withdrawal.unpaid_verified":           {"提现未付款已核实", "已确认本次未实际付款，请查看占用释放结果。"},
		"withdrawal.policy_changed":            {"提现办理规则已更新", "本经营方后续提现办理规则已更新，已申请条款保持原快照。"},
		"qualification.acquired":               {"推广资格已取得", "已确认金额达到本经营方资格条件，请查看资格及推广规则。"},
		"commission.function_changed":          {"推广功能状态已变更", "推广功能状态已记录变更。此前正确佣金继续按原规则结清。"},
		"commission.pending_offset_recovered":  {"佣金待抵扣已处理", "新确认佣金已按规则抵扣待处理金额，请查看佣金明细。"},
		"consumption.corrected":                {"消费账务已冲正", "已通过关联调整修正原消费，请查看账单、资格及佣金净额。"},
		"confirmation.dispute_changed":         {"消费确认复核状态已变更", "相关金额已进入或完成复核，请查看处理状态。"},
		"commission.source_restricted":         {"佣金来源待核实", "相关消费来源正在核实，涉及佣金的释放和使用按当前复核状态处理。"},
		"commission.source_released":           {"佣金来源限制已解除", "相关消费来源的复核限制已解除，请查看消费与佣金的实际处理状态。"},
		"registration.reward_promised":         {"注册奖励承诺已记录", "已记录注册奖励承诺，正式条件满足后按原规则交付。"},
		"registration.reward_delivery_pending": {"注册奖励交付待处理", "已成立的注册奖励责任仍保留，正在恢复原交付。"},
		"registration.rewards_delivered":       {"注册奖励已交付", "注册奖励已交付，请查看来源和有效期。"},
		"policy.changed":                       {"推广经营规则已更新", "推广与奖励经营规则已更新，请查看新版本与适用范围。"},
		"campaign.version_published":           {"活动版本已发布", "活动条款已发布，请检查适用条件与预算。已成交订单继续按原快照处理。"},
	}
	message, ok := messages[event.Kind]
	if !ok {
		return nil
	}
	audience, e := c.Audience.NotificationEventAudience(ctx, tx, event)
	if e != nil {
		return e
	}
	for _, recipient := range audience {
		if recipient.OperatorID != event.OperatorID || recipient.Kind != event.Kind || !validID(recipient.RecipientID) {
			return ErrInvalid
		}
		body := message[1]
		if recipient.AmountUSD != nil {
			body += " 本次申请金额 " + recipient.AmountUSD.String() + " USD。"
		}
		if _, e = c.Operations.Emit(ctx, tx, Notification{OperatorID: recipient.OperatorID, RecipientID: recipient.RecipientID, BusinessKey: "incentive:" + event.ID, Category: "mandatory", Severity: "info", Title: message[0], Body: body, Reference: "incentive-event:" + recipient.Reference}); e != nil {
			return e
		}
	}
	return nil
}
