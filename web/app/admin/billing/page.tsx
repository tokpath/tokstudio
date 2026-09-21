"use client";

import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import { formatUsdMinor } from "@/lib/money";
import { ChargeRefundPanel } from "./charge-refund-panel";
import { BonusPanel } from "./bonus-panel";
import { AdminShell } from "../shell";
import { apiClient } from "@/lib/client";
import { AdminH2 } from "@/components/admin-h2";
import { IfCan } from "@/components/rbac/if-can";
import { AdminSupplierPanel } from "./supplier-panel";

export default function AdminBillingPage() {
  const query = useQuery({
    queryKey: ["billing-report"],
    queryFn: () => apiClient<{ report?: Record<string, number>; error?: { message?: string } }>("GET", "/admin/billing/report"),
  });
  const report = query.data?.report;

  return (
    <AdminShell>
      <section className="rounded-card border border-hairline bg-canvas-raised  p-6">
        <AdminH2 k="billing" className="mb-4 text-lg font-semibold tracking-tight" />
        <p className="mb-3 text-sm text-ink-secondary">按请求编号退消费账单会冲正佣金；按充值单退未使用充值。赠送金额按美元填写，有效期为发放后 24 小时。</p>
        <IfCan action="billing.refund">
          <ChargeRefundPanel onRefund={() => { void query.refetch(); }} />
          <p className="my-3 text-sm"><Link href="/admin/payments" className="text-brand-emphasis underline">前往支付订单查询用户充值、确认入账或退款</Link></p>
        </IfCan>
        <IfCan action="billing.bonus"><BonusPanel /></IfCan>

        <h3 className="mt-6 text-base font-semibold">账务汇总</h3>
        {query.isLoading ? <p role="status">正在加载账务汇总…</p> : query.isError || !report ? (
          <p role="alert" className="mt-3 text-danger">{query.data?.error?.message || "账务汇总加载失败，请刷新页面重试。"}</p>
        ) : <dl className="mt-3 grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
          {([
            ["revenue_minor", "客户收入"], ["upstream_cost_minor", "上游成本"],
            ["wholesale_minor", "渠道批发金额"], ["commission_liability_minor", "待结算佣金"], ["commission_expense_minor", "佣金成本（含已打款）"],
            ["refund_minor", "已退消费账单金额"], ["gross_profit_minor", "毛利"],
          ] as const).map(([key, label]) => <div key={key} className="rounded-control border border-hairline p-3">
            <dt className="text-sm text-ink-secondary">{label}</dt>
            <dd className="mt-1 font-mono tabular-nums">{formatUsdMinor(report[key])} USD</dd>
          </div>)}
          <div className="rounded-control border border-hairline p-3"><dt className="text-sm text-ink-secondary">待对账请求</dt><dd className="mt-1 font-mono">{report.pending_reconciliation_count ?? "—"}</dd></div>
        </dl>}
      </section>
      <AdminSupplierPanel />
    </AdminShell>
  );
}
