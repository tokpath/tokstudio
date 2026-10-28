"use client";
import { useState } from "react";
import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import { useI18n } from "@/components/i18n";
import { request } from "@/lib/api-client";
import { FinancialReportSchema } from "@/lib/contracts";
import { canReadResource } from "@/lib/navigation";
import { subtractDecimal } from "@/lib/decimal";
import { useProduct } from "./context";
import { Heading, Empty, Loading, ErrorPanel, Stat, Money, When, Table } from "./shared";
import { Card } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { ReportPeriod, monthRange } from "./report-period";
import { resourceValueText } from "./operator-resources";

export function FinancialReportScreen({ operatorID }: { operatorID: string }) {
  const { t, locale } = useI18n();
  const { session } = useProduct();
  const ws = session?.kind === "staff" ? session.workspaces.find(w => w.operator.id === operatorID) : undefined;
  const allowed = !!ws && (ws.permissions.includes("*") || ws.permissions.includes("reports.view"));
  const [range, setRange] = useState(monthRange);
  const base = "/manage/" + encodeURIComponent(operatorID);
  const data = useQuery({ queryKey: ["financial-report", operatorID, range], queryFn: ({ signal }) => request("/api/v1/operators/" + encodeURIComponent(operatorID) + "/reports/financial?" + new URLSearchParams(range), FinancialReportSchema, { signal }), enabled: allowed });
  if (!allowed) return <Empty title={t("没有查看权限")}>{t("需要当前经营方财务报表权限。")}</Empty>;
  const report = data.data;
  const pending = report ? [
    { label: "未核实供应成本", count: report.costs.pending_supplier_costs, path: "requests", permission: "requests" },
    { label: "待确认手续费", count: report.costs.pending_fees, path: "payments", permission: "payments" },
    { label: "待核实实际收款", count: report.payments?.pending_receipt_facts || 0, path: "payments?section=pending", permission: "payments" },
    { label: "待处理退款", count: report.refunds.pending_executions + report.refunds.mismatch_count, path: "refunds", permission: "refunds" },
    { label: "待交付客户订单", count: report.sales.pending_deliveries, path: "orders", permission: "orders" },
  ].filter(item => item.count > 0) : [];
  const facts = (items: [string, string | number, string?][]) => <dl className="space-y-4">{items.map(([label, value, unit]) => <div key={label} className="flex flex-wrap items-baseline justify-between gap-3 text-sm"><dt className="text-ink-secondary">{t(label)}</dt><dd className="font-medium">{unit ? <Money value={String(value)} unit={unit} /> : String(value)}</dd></div>)}</dl>;
  return <>
    <Heading title={t("经营报表")} description={t("查看销售、调用、资金收支和经营结果。金额以 USD 记账。")} actions={canReadResource(ws, "exports") ? <Button variant="outline" asChild><Link href={base + "/exports?" + new URLSearchParams({ kind: "financial", ...range })}>{t("导出本期数据")}</Link></Button> : undefined} />
    <Card className="mb-6"><ReportPeriod range={range} onApply={setRange} /></Card>
    {data.isPending ? <Loading /> : data.isError ? <ErrorPanel error={data.error} retry={() => void data.refetch()} /> : report && <>
      <p className="mb-4 text-xs text-ink-mute">{t("更新于")} <When value={report.captured_at} /></p>
      <div className="mb-6 grid gap-4 sm:grid-cols-3">
        <Stat label={t("收入合计")} value={report.revenue.total_usd} unit="USD" help={t("按有偿调用、购买额度到期及供货履约确认；销售额、充值和收款不会重复计入收入。")} />
        <Stat label={t("成本合计")} value={report.costs.total_usd} unit="USD" help={t("包含已确认履约、佣金、手续费和异常损失；赠送履约成本已包含其中。")} />
        <Stat label={t("经营结果")} value={report.operating_result_usd} unit="USD" help={t("已确认收入减去已确认成本。有待核实成本时，结果尚未最终确认。")} />
      </div>
      {pending.length > 0 && <Card className="mb-6"><h2 className="mb-4 font-semibold">{t("待处理事项")}</h2><div className="flex flex-wrap gap-x-6 gap-y-3">{pending.map(item => <div key={item.label} className="text-sm">{canReadResource(ws, item.permission) ? <Link className="text-brand-emphasis underline" href={base + "/" + item.path}>{t(item.label)} · {item.count}</Link> : <span>{t(item.label)} · {item.count}</span>}</div>)}</div>{report.costs.pending_supplier_costs + report.costs.pending_fees > 0 && <p className="mt-3 text-xs text-ink-mute">{t("有成本待核实，当前经营结果尚未最终确认。")}</p>}{report.payments?.original_money.length ? <div className="mt-3 text-sm">{report.payments.original_money.map(item => <span key={item.currency} className="mr-4"><Money value={item.amount} unit={item.currency} /></span>)}</div> : null}</Card>}
      <div className="mb-6 grid gap-6 xl:grid-cols-2">
        <Card><h2 className="mb-5 text-lg font-semibold">{t("销售与调用")}</h2>{facts([["套餐销售额", report.sales.sales_usd, "USD"], ["套餐销售笔数", report.sales.completed_sales], ["批发销售额", report.procurement.wholesale_sales_usd, "USD"], ["批发销售笔数", report.procurement.wholesale_orders], ["支付余额充值", report.sales.wallet_top_up_usd, "USD"], ["已完成调用", report.usage.completed_requests], ["净扣费", subtractDecimal(report.usage.charged_atg, report.usage.correction_atg), "ATG"]])}<p className="mt-5 text-xs text-ink-mute">{t("销售额按已交付订单统计；销售、充值与确认收入分别展示。")}</p></Card>
        <Card><h2 className="mb-5 text-lg font-semibold">{t("资金收支")}</h2>{facts([["已核实流入", report.cash.recorded_in_usd, "USD"], ["已核实流出", report.cash.recorded_out_usd, "USD"], ["采购付款", report.cash.procurement_payments_usd, "USD"], ["佣金提现付款", report.cash.commission_payouts_usd, "USD"], ["通道未结应收", report.cash.provider_receivable_usd, "USD"]])}</Card>
      </div>
      <details className="mb-6 rounded-card border border-hairline bg-canvas-raised p-5"><summary className="cursor-pointer font-medium">{t("收入与成本明细")}</summary><div className="mt-5 grid gap-8 xl:grid-cols-2">
        <div><h3 className="mb-4 font-semibold">{t("已确认收入")}</h3>{facts([["有偿调用确认", report.revenue.paid_invocation_usd, "USD"], ["购买权益到期确认", report.revenue.expired_unused_paid_usd, "USD"], ["已消费供货收入", report.revenue.wholesale_consumed_usd, "USD"], ["提现手续费保留", report.revenue.withdrawal_fees_retained_usd, "USD"]])}</div>
        <div><h3 className="mb-4 font-semibold">{t("已确认成本")}</h3>{facts([["全部履约成本", report.costs.fulfillment_usd, "USD"], ["其中赠送履约成本（子集）", report.costs.retail_gift_subset_usd, "USD"], ["推荐佣金成本", report.costs.commission_usd, "USD"], ["已核实收付手续费", report.costs.payment_fees_usd, "USD"], ["异常收款与履约损失", report.costs.exception_loss_usd, "USD"]])}</div>
      </div></details>
      <details className="mb-6 rounded-card border border-hairline bg-canvas-raised p-5"><summary className="cursor-pointer font-medium">{t("期末余额与服务责任")}</summary><div className="mt-5 max-w-3xl">{facts([["客户USD余额责任", report.liabilities.customer_wallet_usd, "USD"], ["购买权益剩余成交价值", report.liabilities.outstanding_grant_paid_usd, "USD"], ["待服务ATG", report.liabilities.outstanding_grant_atg, "ATG"], ["佣金责任", report.liabilities.commission_usd, "USD"], ["未交付订单资金", report.liabilities.unfulfilled_order_funds_usd, "USD"], ["未履约供货预收款", report.liabilities.wholesale_prepayment_usd, "USD"], ["供货退款待付", report.liabilities.supplier_payable_usd, "USD"]])}</div></details>
      {!!report.takeover_flows?.length && <details className="mb-6 rounded-card border border-hairline bg-canvas-raised p-5"><summary className="cursor-pointer font-medium">{t("接管资金明细")}</summary><div className="mt-5"><Table rows={report.takeover_flows} getID={row => row.purpose + ":" + row.kind + ":" + row.unit} columns={[{ label: t("账务事项"), cell: row => resourceValueText(row.purpose, "purpose", locale) + " · " + resourceValueText(row.kind, "kind", locale) }, { label: t("实际流入"), cell: row => <Money value={row.increase} unit={row.unit} /> }, { label: t("实际流出"), cell: row => <Money value={row.decrease} unit={row.unit} /> }]} /></div></details>}
      <p className="text-xs leading-6 text-ink-mute">{t("余额按所选结束时点计算；待核实、退款等事项按本次查询时点列出。")}</p>
    </>}
  </>;
}
