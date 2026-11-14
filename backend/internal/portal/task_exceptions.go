package portal

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/jobs"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"gorm.io/gorm"
)

// Keep the failed outcome visible during the next automatic attempt. A later
// successful cycle supersedes it; one-off tasks retain their own identity.
type taskException struct {
	jobs.Job
	ScheduleID, CurrentJobID, CurrentStatus string
}

type taskCursor struct {
	At time.Time `json:"at"`
	ID string    `json:"id"`
}

var taskIDPattern = regexp.MustCompile(`^(job|refund)_[0-9a-f]{32}$`)
var taskCycleSuffix = regexp.MustCompile(`:[0-9]+$`)

func (a *Application) taskExceptions(ctx context.Context, cursor string, limit int) ([]taskException, error) {
	if limit < 1 || limit > 100 {
		return nil, organization.ErrInvalidInput
	}
	var after taskCursor
	if cursor != "" {
		raw, err := base64.RawURLEncoding.DecodeString(cursor)
		if err != nil || json.Unmarshal(raw, &after) != nil || after.At.IsZero() || !taskIDPattern.MatchString(after.ID) {
			return nil, organization.ErrInvalidInput
		}
	}
	query := `WITH exceptions AS (
	 SELECT failed.*,s.id AS schedule_id,current.id AS current_job_id,current.status AS current_status
	 FROM portal_schedules s
	 JOIN durable_jobs current ON current.id=s.last_job_id
	 JOIN LATERAL (
	  SELECT j.* FROM durable_jobs j
	  WHERE j.queue='portal-maintenance'
	   AND regexp_replace(j.business_key, ':[0-9]+$', '')=s.id
	   AND (j.status='completed' OR j.exhausted_at IS NOT NULL)
	  ORDER BY j.created_at DESC,j.id DESC LIMIT 1
	 ) failed ON failed.status<>'completed'
	 WHERE NOT (s.kind='risks' AND COALESCE(s.payload->>'operator_id','')=?)
	 UNION ALL
	 SELECT j.*,'' AS schedule_id,j.id AS current_job_id,j.status AS current_status
	 FROM durable_jobs j
	 WHERE j.status<>'completed' AND j.exhausted_at IS NOT NULL
	 AND NOT EXISTS (SELECT 1 FROM portal_schedules s WHERE j.queue='portal-maintenance'
	  AND regexp_replace(j.business_key, ':[0-9]+$', '')=s.id)
	 AND NOT (j.queue='portal-mail' AND EXISTS (SELECT 1 FROM portal_messages m
	  WHERE m.id::text=j.payload->>'message_id' AND (m.expires_at<=now() OR m.status='accepted')))
	) SELECT * FROM (
	 SELECT id,queue,kind,payload,max_attempts,last_error,exhausted_at,current_job_id,current_status FROM exceptions
	 UNION ALL
	 SELECT 'refund_'||md5(i.merchant_id||':'||i.event_id), 'refund-callback', 'refund_callback',
	    jsonb_build_object('operator_id',m.operator_id), i.max_attempts, COALESCE(i.error_code,''),
	    i.exhausted_at, 'refund_'||md5(i.merchant_id||':'||i.event_id), i.state
	 FROM payments_webhook_inbox i JOIN payments_merchants m ON m.id=i.merchant_id
	 WHERE i.exhausted_at IS NOT NULL AND i.state<>'processed'
	) all_exceptions`
	args := []any{a.Installation.OperatorID}
	if cursor != "" {
		query += ` WHERE (exhausted_at,id) < (?,?)`
		args = append(args, after.At, after.ID)
	}
	query += ` ORDER BY exhausted_at DESC,id DESC LIMIT ?`
	args = append(args, limit)
	rows := []taskException{}
	err := a.DB.WithContext(ctx).Raw(query, args...).Scan(&rows).Error
	return rows, err
}

func taskName(kind string) string {
	if name := map[string]string{
		"brand_domain": "品牌域名验证", "catalog_changes": "模型配置生效检查",
		"registration_release": "注册奖励预留清理", "unpaid_orders": "未付款订单检查",
		"kvoucher_payments": "付款报价与 KVoucher 收款核实", "registration_reward": "注册奖励发放",
		"remediation_events": "退款与争议通知", "incentive_events": "邀请与佣金通知", "commerce_notification": "购买与续费通知",
		"refund_callback": "原路退款回调核实",
		"qualification":   "消费确认与 KOL 资格检查", "expiry": "到期额度处理", "renewal": "套餐自动续费",
		"package_change": "套餐变更通知", "usage": "调用结果与用量核实", "delivery": "套餐额度交付",
		"receipt": "到账后的订单交付", "risks": "采购余额与销售空间检查", "restriction_reviews": "账户限制复查",
		"invoice_review": "发票状态检查", "retention": "过期临时数据清理", "coverage": "调价后的履约检查",
		"verification": "账号验证邮件", "staff_invitation": "人员邀请邮件", "staff_password_reset": "密码重置邮件",
	}[kind]; name != "" {
		return name
	}
	return "后台任务"
}

func taskFailureReason(code string) string {
	switch {
	case strings.HasSuffix(code, ": business_closed"):
		return "相关业务已暂停，请检查经营方的营业状态。"
	case strings.HasSuffix(code, ": insufficient_asset"):
		return "可用资产不足，请检查相关余额或额度。"
	case strings.HasSuffix(code, ": original_order_not_fully_paid"):
		return "订单尚未足额付款，请在原订单核实收款。"
	case strings.HasSuffix(code, ": ownership_changed"):
		return "业务归属已变化，请核实当前负责的经营方。"
	case code == "mail delivery unavailable":
		return "邮件发送失败，请检查邮件服务配置与连接。"
	case code == "coverage recheck requires dependency recovery":
		return "履约检查未完成，请检查供货配置和相关服务。"
	case code == "allocation or delivery requires retry/review":
		return "收款后的订单处理未完成，请核实原订单与交付状态。"
	case code == "lease expired on final attempt":
		return "后台执行超时或中断，请检查任务服务。"
	case code == "provider_unavailable" || code == "verification_pending":
		return "原路退款结果待核实，请检查支付通道。"
	case code == "evidence_conflict":
		return "支付通道结果与原退款记录不一致，请核对原执行。"
	default:
		// Never expose arbitrary driver errors, payloads or credentials here.
		return "相关服务或业务状态异常，请由技术人员排查。"
	}
}

// Technical diagnostics identify the operator/domain/public model without
// exposing another brand's customer identity, assets or raw job payload.
func (a *Application) taskImpact(ctx context.Context, j jobs.Job) (string, string, error) {
	var payload struct {
		workerPayload
		OrderID string `json:"order_id"`
		ModelID string `json:"model_id"`
	}
	if json.Unmarshal(j.Payload, &payload) != nil {
		return "", "关联业务", nil
	}
	if payload.OperatorID != "" {
		if _, err := uuid.Parse(payload.OperatorID); err == nil {
			op, err := a.Organization.GetOperator(ctx, nil, payload.OperatorID)
			if err == nil {
				return op.Name, "", nil
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return "", "", err
			}
		}
		return "", "关联经营方", nil
	}
	if j.Kind == "coverage" && payload.ModelID != "" {
		return payload.ModelID, "", nil
	}
	if j.Kind == "brand_domain" {
		var domain struct{ Hostname string }
		if _, err := uuid.Parse(payload.ID); err == nil {
			if err := a.DB.WithContext(ctx).Table("branding_domains").Select("hostname").Where("id=?", payload.ID).Find(&domain).Error; err != nil {
				return "", "", err
			}
		}
		return domain.Hostname, "品牌域名", nil
	}
	if j.Queue == "portal-mail" {
		return "", "相关账号", nil
	}
	if payload.ID != "" || payload.CustomerID != "" || payload.OrderID != "" {
		return "", "关联业务", nil
	}
	return "", "全系统", nil
}
