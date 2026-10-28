package operations

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"gorm.io/gorm"
)

type exportColumn struct{ Key, Label string }

var exportColumns = map[string][]exportColumn{
	"financial":   {{"section", "分类"}, {"metric", "指标"}, {"value", "数值"}, {"unit", "单位"}},
	"orders":      {{"created_at", "创建时间"}, {"customer_name", "客户"}, {"kind", "业务类型"}, {"status", "状态"}, {"due_usd", "应付 USD"}, {"paid_atg", "购买 ATG"}, {"gift_atg", "赠送 ATG"}, {"external_allocated_usd", "外部收款 USD"}, {"internal_usd", "USD 余额支付"}, {"commission_usd", "佣金支付"}, {"completed_at", "交付时间"}, {"id", "记录编号"}},
	"payments":    {{"paid_at", "实际付款时间"}, {"counterparty_name", "客户或采购方"}, {"provider", "收款方式"}, {"currency", "实际币种"}, {"original_amount", "原币金额"}, {"gross_usd", "实收 USD"}, {"allocated_usd", "已关联 USD"}, {"fee_status", "手续费状态"}, {"fee_usd", "手续费 USD"}, {"id", "记录编号"}},
	"requests":    {{"accepted_at", "受理时间"}, {"customer_name", "客户"}, {"model_id", "模型"}, {"operation", "调用类型"}, {"status", "状态"}, {"net_charged_atg", "净扣费 ATG"}, {"net_actual_atg", "实际消耗 ATG"}, {"corrected_charged_atg", "已冲正 ATG"}, {"merchant_shortfall", "经营方承担 ATG"}, {"fulfillment_cost_usd", "履约成本 USD"}, {"supplier_cost_usd", "供应商成本 USD"}, {"unverified_attempts", "成本待核实次数"}, {"id", "记录编号"}},
	"procurement": {{"created_at", "创建时间"}, {"supplier_name", "供货方"}, {"buyer_name", "采购方"}, {"name", "报价名称"}, {"status", "状态"}, {"due_usd", "应付 USD"}, {"allocated_usd", "已收款 USD"}, {"paid_atg", "购买 ATG"}, {"gift_atg", "赠送 ATG"}, {"completed_at", "交付时间"}, {"id", "记录编号"}},
	"ledger":      {{"created_at", "入账时间"}, {"owner_id", "主体编号"}, {"kind", "账务事项"}, {"purpose", "账户用途"}, {"unit", "单位"}, {"amount", "变动金额"}, {"balance_after", "变动后余额"}, {"journal_id", "账务记录编号"}, {"source_id", "来源记录编号"}},
}

func exportKind(kind string) bool { _, ok := exportColumns[kind]; return ok }

func (d DomainSource) salesExport(ctx context.Context, tx *gorm.DB, operator string, q ExportQuery) ([]json.RawMessage, error) {
	var rows []map[string]any
	var err error
	if q.Kind == "orders" {
		err = tx.WithContext(ctx).Table("commerce_orders o").Select("o.id,o.customer_id,o.kind,o.status,o.due_usd::text,o.paid_atg::text,(o.package_gift_atg+o.merchant_gift_atg+o.kol_gift_atg)::text AS gift_atg,o.external_allocated_usd::text,o.internal_usd::text,o.commission_usd::text,o.created_at,o.completed_at").Where("o.operator_id=? AND o.created_at>=? AND o.created_at<?", operator, q.From, q.To).Order("o.created_at,o.id").Limit(100001).Find(&rows).Error
	} else {
		err = tx.WithContext(ctx).Table("commerce_receipts r").Select(`r.id,r.provider,r.currency,r.original_amount,r.gross_usd::text,r.allocated_usd::text,r.fee_usd::text,r.fee_status,r.paid_at,
 COALESCE((SELECT string_agg(DISTINCT c.display_name,' / ') FROM commerce_receipt_allocations a JOIN commerce_orders o ON o.id=a.order_id JOIN organization_customers c ON c.id=o.customer_id WHERE a.receipt_id=r.id AND a.operator_id=r.operator_id),
 (SELECT string_agg(DISTINCT b.name,' / ') FROM cooperation_purchase_allocations a JOIN cooperation_purchase_orders o ON o.id=a.order_id JOIN organization_operators b ON b.id=o.buyer_id WHERE a.receipt_id=r.id AND o.supplier_id=r.operator_id),'') AS counterparty_name`).Where("r.operator_id=? AND COALESCE(r.paid_at,r.created_at)>=? AND COALESCE(r.paid_at,r.created_at)<?", operator, q.From, q.To).Order("r.created_at,r.id").Limit(100001).Find(&rows).Error
	}
	if err != nil {
		return nil, err
	}
	return jsonRows(rows)
}

func financialExportRows(r FinancialReport) ([]json.RawMessage, error) {
	rows := []map[string]any{}
	add := func(section, metric string, value any, unit string) {
		rows = append(rows, map[string]any{"section": section, "metric": metric, "value": value, "unit": unit})
	}
	add("销售与调用", "套餐销售额", r.Sales.SalesUSD, "USD")
	add("销售与调用", "套餐销售笔数", r.Sales.CompletedSales, "笔")
	add("销售与调用", "批发销售额", r.Procurement.WholesaleSalesUSD, "USD")
	add("销售与调用", "批发销售笔数", r.Procurement.WholesaleOrders, "笔")
	add("销售与调用", "支付余额充值", r.Sales.WalletTopUpUSD, "USD")
	add("销售与调用", "已完成调用", r.Usage.CompletedRequests, "次")
	add("销售与调用", "净扣费", r.Usage.ChargedATG.Sub(r.Usage.CorrectionATG), "ATG")
	add("经营结果", "收入合计", r.Revenue.TotalUSD, "USD")
	add("经营结果", "成本合计", r.Costs.TotalUSD, "USD")
	add("经营结果", "经营结果（未决项另列）", r.OperatingResultUSD, "USD")
	add("资金收支", "已核实流入", r.Cash.RecordedInUSD, "USD")
	add("资金收支", "已核实流出", r.Cash.RecordedOutUSD, "USD")
	add("经营结果", "全部履约成本", r.Costs.FulfillmentUSD, "USD")
	add("经营结果", "其中赠送履约成本（子集）", r.Costs.RetailGiftSubsetUSD, "USD")
	add("经营结果", "推荐佣金成本", r.Costs.CommissionUSD, "USD")
	add("经营结果", "已核实收付手续费", r.Costs.PaymentFeesUSD, "USD")
	add("经营结果", "有偿调用确认", r.Revenue.PaidInvocationUSD, "USD")
	add("经营结果", "购买权益到期确认", r.Revenue.ExpiredUnusedPaidUSD, "USD")
	add("经营结果", "已消费供货收入", r.Revenue.WholesaleConsumedUSD, "USD")
	add("经营结果", "提现手续费保留", r.Revenue.WithdrawalFeesRetainedUSD, "USD")
	add("经营结果", "异常收款与履约损失", r.Costs.ExceptionLossUSD, "USD")
	add("资金收支", "采购付款", r.Cash.ProcurementPaymentsUSD, "USD")
	add("资金收支", "佣金提现付款", r.Cash.CommissionPayoutsUSD, "USD")
	add("资金收支", "通道未结应收", r.Cash.ProviderReceivableUSD, "USD")
	add("资金收支", "其中接管流入", r.Cash.TakeoverInUSD, "USD")
	add("资金收支", "其中接管流出", r.Cash.TakeoverOutUSD, "USD")
	add("期末余额与服务责任", "待服务ATG", r.Liabilities.OutstandingGrantATG, "ATG")
	add("期末余额与服务责任", "客户USD余额责任", r.Liabilities.CustomerWalletUSD, "USD")
	add("期末余额与服务责任", "佣金责任", r.Liabilities.CommissionUSD, "USD")
	add("期末余额与服务责任", "购买权益剩余成交价值", r.Liabilities.OutstandingGrantPaidUSD, "USD")
	add("期末余额与服务责任", "未交付订单资金", r.Liabilities.UnfulfilledOrderFundsUSD, "USD")
	add("期末余额与服务责任", "未履约供货预收款", r.Liabilities.WholesalePrepaymentUSD, "USD")
	add("期末余额与服务责任", "供货退款待付", r.Liabilities.SupplierPayableUSD, "USD")
	add("待处理事项", "未核实供应成本", r.Costs.PendingSupplierCosts, "笔")
	add("待处理事项", "待确认手续费", r.Costs.PendingFees, "笔")
	add("待处理事项", "待处理退款", r.Refunds.PendingExecutions, "笔")
	add("待处理事项", "退款差额待处理", r.Refunds.MismatchCount, "笔")
	add("待处理事项", "待核实实际收款", r.Payments.PendingReceiptFacts, "笔")
	add("待处理事项", "待交付客户订单", r.Sales.PendingDeliveries, "笔")

	add("统计范围", "开始时间", r.From.Format("2006-01-02T15:04:05Z07:00"), "")
	add("统计范围", "结束时间（不含）", r.To.Format("2006-01-02T15:04:05Z07:00"), "")
	add("统计范围", "生成时间", r.CapturedAt.Format("2006-01-02T15:04:05Z07:00"), "")
	return jsonRows(rows)
}

func buildExportCSV(ctx context.Context, tx *gorm.DB, operator, kind string, raw []json.RawMessage) ([]byte, error) {
	columns, ok := exportColumns[kind]
	if !ok {
		return nil, ErrInvalid
	}
	rows := make([]map[string]any, 0, len(raw))
	ids := map[string]bool{}
	for _, b := range raw {
		var r map[string]any
		decoder := json.NewDecoder(bytes.NewReader(b))
		decoder.UseNumber()
		if err := decoder.Decode(&r); err != nil {
			return nil, err
		}
		rows = append(rows, r)
		for _, key := range []string{"customer_id", "buyer_id", "supplier_id"} {
			if id, ok := r[key].(string); ok && validID(id) {
				ids[id] = true
			}
		}
	}
	names := map[string]string{}
	if len(ids) > 0 {
		keys := make([]string, 0, len(ids))
		for id := range ids {
			keys = append(keys, id)
		}
		sort.Strings(keys)
		// Names can only describe parties present in this operator's own facts.
		// No email, account evidence, customer content, or supplier keys are exported.
		for start := 0; start < len(keys); start += 5000 {
			batch := keys[start:min(start+5000, len(keys))]
			var found []struct{ ID, Name string }
			if err := tx.WithContext(ctx).Raw(`SELECT id::text,name FROM organization_operators WHERE id IN ? AND (id=? OR parent_operator_id=? OR id=(SELECT parent_operator_id FROM organization_operators WHERE id=?)) UNION ALL SELECT c.id::text,c.display_name AS name FROM organization_customers c WHERE c.id IN ? AND (EXISTS(SELECT 1 FROM organization_service_ownerships o WHERE o.customer_id=c.id AND o.operator_id=?) OR EXISTS(SELECT 1 FROM commerce_orders o WHERE o.customer_id=c.id AND o.operator_id=?))`, batch, operator, operator, operator, batch, operator, operator).Scan(&found).Error; err != nil {
				return nil, err
			}
			for _, r := range found {
				names[r.ID] = r.Name
			}
		}
	}
	var content bytes.Buffer
	writer := csv.NewWriter(&content)
	head := make([]string, len(columns))
	for i, c := range columns {
		head[i] = c.Label
	}
	if err := writer.Write(head); err != nil {
		return nil, err
	}
	for _, r := range rows {
		for key, target := range map[string]string{"customer_id": "customer_name", "buyer_id": "buyer_name", "supplier_id": "supplier_name"} {
			if id, ok := r[key].(string); ok {
				r[target] = names[id]
			}
		}
		cells := make([]string, len(columns))
		for i, c := range columns {
			value := ""
			if r[c.Key] != nil {
				value = fmt.Sprint(r[c.Key])
			}
			if strings.HasPrefix(value, "map[") {
				return nil, ErrInvalid
			}
			cells[i] = safeCSVCell(exportValue(c.Key, value))
		}
		if err := writer.Write(cells); err != nil {
			return nil, err
		}
	}
	writer.Flush()
	return content.Bytes(), writer.Error()
}

var csvNumber = regexp.MustCompile(`^-?[0-9]+(?:\.[0-9]+)?$`)

func safeCSVCell(value string) string {
	trimmed := strings.TrimLeft(value, " \t\r\n")
	if trimmed != "" && (strings.ContainsRune("=+-@", rune(trimmed[0])) || strings.ContainsAny(value[:len(value)-len(trimmed)], "\t\r\n")) && !csvNumber.MatchString(value) {
		return "'" + value
	}
	return value
}
func exportValue(key, value string) string {
	if !strings.Contains("|kind|status|fee_status|operation|purpose|provider|", "|"+key+"|") {
		return value
	}
	if key == "provider" {
		value = strings.SplitN(value, ":", 2)[0]
	}
	labels := map[string]string{"pending": "待付款", "awaiting_payment": "待付款", "paid": "已付款待交付", "completed": "已完成", "closed": "已关闭", "confirmed": "已核实", "settled": "已结算", "released": "已释放", "failed": "失败", "unknown": "待核实", "exception": "异常", "manual": "线下人工核实", "stripe": "Stripe", "alipay": "支付宝", "wechat": "微信支付", "topup": "余额充值", "package": "套餐购买", "renewal": "套餐续费", "chat": "文本对话", "responses": "模型响应", "image_generation": "图片生成", "speech": "语音生成", "transcription": "语音转写", "video_generation": "视频生成", "embedding": "文本向量", "customer:usd": "客户 USD 余额", "commission:available": "可用佣金", "commission:frozen": "冻结佣金", "procurement:atg": "采购余额", "control:merchant_fees": "收款手续费", "control:provider_receivable": "通道未结应收", "control:external_cash": "资金往来", "order:funds": "订单待交付资金", "wallet_topup": "余额充值", "package_payment": "套餐购买", "external_receipt": "实际收款", "wholesale_sale": "批发销售", "procurement": "采购入库", "commission_payout": "佣金提现", "request_settlement": "调用结算"}
	if key == "fee_status" && value == "pending" {
		return "待核实"
	}
	if label := labels[value]; label != "" {
		return label
	}
	if strings.HasPrefix(value, "customer:atg:") {
		return "客户 ATG 权益"
	}
	return value
}

// Localize CSV system labels only. Authored customer/model names and money
// strings retain their original values; formula-like text remains escaped.
func LocalizeExportCSV(raw []byte, translate func(string) string) ([]byte, error) {
	r := csv.NewReader(bytes.NewReader(raw))
	records, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, ErrInvalid
	}
	translateColumns := map[string]bool{"分类": true, "指标": true, "单位": true, "业务类型": true, "状态": true, "收款方式": true, "手续费状态": true, "调用类型": true, "账务事项": true, "账户用途": true}
	var content bytes.Buffer
	content.WriteString("\ufeff")
	writer := csv.NewWriter(&content)
	headers := append([]string(nil), records[0]...)
	for i, record := range records {
		for j, value := range record {
			if i == 0 || translateColumns[headers[j]] {
				record[j] = safeCSVCell(translate(value))
			}
		}
		if err := writer.Write(record); err != nil {
			return nil, err
		}
	}
	writer.Flush()
	return content.Bytes(), writer.Error()
}
