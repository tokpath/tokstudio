package notifications

import "strings"

func noticeText(kind, locale, detail string) (title, body, href string) {
	zh := map[string][3]string{
		"balance_insufficient": {"可用额度不足，请补充额度", "本次请求因可用额度不足已被拒绝，没有产生本次请求的消费扣款。可用额度包含未到期的套餐、赠送额度和账户余额；补充足够额度后即可继续使用。", "/app/wallet"},
		"balance_low":          {"可用额度即将用尽", "当前可用的套餐、赠送额度及账户余额合计不足 1 美元。请及时补充额度；可用额度不足以覆盖请求时会停止该请求。", "/app/wallet"},
		"expiring_3d":          {"套餐将在 3 天内到期", "当前套餐将在 3 天内到期，到期后剩余额度失效。请检查续费安排或购买新套餐；续费失败没有宽限期。", "/app/plans"},
		"expiring_1d":          {"套餐将在 24 小时内到期", "当前套餐将在 24 小时内到期，到期后剩余额度失效。请及时检查续费安排；续费失败没有宽限期。", "/app/plans"},
		"expired":              {"套餐已到期", "当前套餐已到期，剩余额度已失效，没有宽限期。有其他有效套餐、赠送额度或账户余额时仍可使用；全部可用额度不足时，请求会被拒绝。", "/app/plans"},
		"renewal_failed":       {"套餐续费失败", "套餐续费未成功，本期套餐已到期，剩余额度失效，没有宽限期。后续付款重试不会延长服务期限；有其他有效额度时仍可使用，全部额度不足时请求会被拒绝。请检查付款方式或重新购买套餐。", "/app/plans"},
		"renewed":              {"套餐续费成功", "新一期套餐已生效，新的套餐额度已到账。请求将按当前可用额度和模型权限进行校验。", "/app/plans"},
		"quota_insufficient":   {"服务额度不足", "本品牌的一次请求因服务额度不足被拒绝。请检查品牌服务额度及用户已分配额度，及时补充以恢复使用。", "/channel/ledger"},
		"quota_low":            {"品牌服务额度即将用尽", "品牌可发放服务额度不足 1 美元，请及时补充。已经分配给用户的有效额度仍按现有计费规则使用。", "/channel/ledger"},
		"quota_recovered":      {"品牌服务额度已恢复", "品牌可发放服务额度已恢复到提醒阈值以上。", "/channel/ledger"},
		"alert_opened":         {"平台运营告警", "平台出现新的运营告警，请查看告警详情并处理。", "/admin/alerts"},
		"alert_resolved":       {"平台运营告警已恢复", "此前的运营告警已恢复，请查看处理记录。", "/admin/alerts"},
		"smtp_test":            {"邮件通知测试", "这是一封平台邮件通知测试邮件。如果你收到此邮件，说明当前 SMTP 已完成这次邮件传输。", "/app/notifications"},
	}
	en := map[string][3]string{
		"balance_insufficient": {"Insufficient available credit", "Your request was rejected for insufficient credit and incurred no usage charge. Credit includes unexpired plan and gift credit plus your balance. Add sufficient credit to continue.", "/app/wallet"},
		"balance_low":          {"Available credit is running low", "Your unexpired plan credit, gift credit and balance total less than USD 1. Add credit soon. Requests are rejected when available credit cannot cover the request.", "/app/wallet"},
		"expiring_3d":          {"Your plan expires within 3 days", "Unused plan credit expires at the end of this period. Check renewal or buy another plan. Failed renewals have no grace period.", "/app/plans"},
		"expiring_1d":          {"Your plan expires within 24 hours", "Unused plan credit expires at the end of this period. Check renewal now. Failed renewals have no grace period.", "/app/plans"},
		"expired":              {"Your plan has expired", "Unused credit for this plan is no longer valid. There is no grace period. Other valid credit remains usable; requests stop when all eligible credit is insufficient.", "/app/plans"},
		"renewal_failed":       {"Plan renewal failed", "Renewal failed and this plan period has ended. Unused credit has expired, with no grace period. Payment retries do not extend service. Other valid credit remains usable; requests stop when all eligible credit is insufficient. Check your payment method or purchase a plan.", "/app/plans"},
		"renewed":              {"Plan renewal succeeded", "Your new plan period is active and new credit has been granted. Requests are checked against available credit and model permissions.", "/app/plans"},
		"quota_insufficient":   {"Insufficient service credit", "A request for your brand was rejected for insufficient service credit. Check the brand pool and allocated user credit and replenish as needed.", "/channel/ledger"},
		"quota_low":            {"Brand service credit is running low", "Your brand's available credit pool is below USD 1. Replenish it soon. Valid credit already allocated to users remains subject to the existing billing rules.", "/channel/ledger"},
		"quota_recovered":      {"Brand service credit recovered", "Your brand's available credit pool is above the notification threshold again.", "/channel/ledger"},
		"alert_opened":         {"Platform operations alert", "A new operations alert requires attention. Open the alert details to investigate.", "/admin/alerts"},
		"alert_resolved":       {"Platform operations alert resolved", "A previous operations alert has recovered. Open the alert history for details.", "/admin/alerts"},
		"smtp_test":            {"Email notification test", "This is a platform email notification test. Receiving it confirms SMTP transmitted this message.", "/app/notifications"},
	}
	ja := map[string][3]string{
		"balance_insufficient": {"利用可能な残高が不足しています", "有効なプラン・付与枠・残高が不足しているため、リクエストを拒否しました。このリクエストの利用料金は発生していません。残高を補充して再開してください。", "/app/wallet"},
		"balance_low":          {"利用可能な残高が少なくなっています", "有効なプラン・付与枠・残高の合計が 1 米ドル未満です。残高がリクエストを満たせない場合は利用を停止します。", "/app/wallet"},
		"expiring_3d":          {"プランは 3 日以内に終了します", "期間終了時に未使用の枠は失効します。更新または新規購入をご確認ください。更新失敗時の猶予期間はありません。", "/app/plans"},
		"expiring_1d":          {"プランは 24 時間以内に終了します", "期間終了時に未使用の枠は失効します。更新をご確認ください。更新失敗時の猶予期間はありません。", "/app/plans"},
		"expired":              {"プランが終了しました", "未使用の枠は失効し、猶予期間はありません。他の有効な枠は利用できますが、利用可能な枠が不足するとリクエストを拒否します。", "/app/plans"},
		"renewal_failed":       {"プランの更新に失敗しました", "更新に失敗し、現在の期間が終了しました。未使用の枠は失効し、猶予期間はありません。支払いの再試行でも利用期間は延長されません。他の有効な枠が不足するとリクエストを拒否します。お支払い方法またはプラン購入をご確認ください。", "/app/plans"},
		"renewed":              {"プランを更新しました", "新しい期間と利用枠が有効になりました。利用可能な枠とモデル権限に従ってリクエストを処理します。", "/app/plans"},
		"quota_insufficient":   {"サービス利用枠が不足しています", "利用枠不足でブランドのリクエストを拒否しました。ブランドの在庫とユーザーへの割当を確認し、補充してください。", "/channel/ledger"},
		"quota_low":            {"ブランドのサービス利用枠が少なくなっています", "ブランドの割当可能な枠が 1 米ドル未満です。補充をご検討ください。すでにユーザーへ割り当てた有効な枠は既存の課金ルールに従って利用できます。", "/channel/ledger"},
		"quota_recovered":      {"ブランドのサービス利用枠が回復しました", "ブランドの割当可能な枠が通知の閾値以上に回復しました。", "/channel/ledger"},
		"alert_opened":         {"プラットフォーム運用アラート", "新しい運用アラートを確認し、対応してください。", "/admin/alerts"},
		"alert_resolved":       {"運用アラートが解消しました", "以前の運用アラートが解消しました。処理記録をご確認ください。", "/admin/alerts"},
		"smtp_test":            {"メール通知テスト", "プラットフォームのメール通知テストです。受信できた場合、今回の SMTP 転送は完了しています。", "/app/notifications"},
	}
	value, ok := zh[kind]
	if !ok {
		return "", "", ""
	}
	if locale == "en" {
		value = en[kind]
	}
	if locale == "ja" {
		value = ja[kind]
	}
	if strings.HasPrefix(kind, "alert_") && detail != "" {
		value[1] += "\n" + detail
	}
	return value[0], value[1], value[2]
}
