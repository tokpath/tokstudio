"use client";

import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { ConfirmButton } from "@/components/confirm-button";
import { formatUsdMinor, parseUsdToMinor } from "@/lib/money";
import { Label } from "@/components/ui/label";
import { Input } from "@/components/ui/input";
import { AdminShell } from "../shell";
import { apiBase } from "@/lib/api";
import { apiClient } from "@/lib/client";
import { confirmHeaders, confirmNetworkUnavailable } from "@/lib/confirm";
import { AdminH2 } from "@/components/admin-h2";
import { IfCan } from "@/components/rbac/if-can";
import { AdminSupplierPanel } from "./supplier-panel";

export default function AdminBillingPage() {
  const [requestID, setRequestID] = useState("");
  const [topupID, setTopupID] = useState("");
  const [userID, setUserID] = useState("");
  const [bonus, setBonus] = useState("1");
  const [message, setMessage] = useState("退款、确认入账和赠送额度都要二次确认，并写入审计。");
  const query = useQuery({
    queryKey: ["billing-report"],
    queryFn: () => apiClient<{ report?: Record<string, number>; error?: { message?: string } }>("GET", "/admin/billing/report"),
  });
  const report = query.data?.report;
  const bonusMinor = parseUsdToMinor(bonus);
  const validBonus = bonusMinor !== null && bonusMinor > 0 && Boolean(userID.trim());

  async function post(path: string, body: Record<string, string | number>, okText: string): Promise<boolean> {
    try {
    const res = await fetch(`${apiBase}${path}`, {
      method: "POST",
      credentials: "include",
      headers: confirmHeaders,
      body: JSON.stringify(body),
    });
    const payload = await res.json();
    setMessage(res.ok ? okText : payload.error?.message || "操作失败");
    const __ok = res.ok;
    await query.refetch();
    return __ok;
    } catch {
      setMessage(confirmNetworkUnavailable);
      return false;
    }
}

  return (
    <AdminShell>
      <section className="rounded-card border border-hairline bg-canvas-raised  p-6">
        <AdminH2 k="billing" className="mb-4 text-lg font-semibold tracking-tight" />
        <p className="mb-3 text-sm text-ink-secondary">按请求编号退消费账单会冲正佣金；按充值单退未使用充值。赠送金额按美元填写，有效期为发放后 24 小时。</p>
        <IfCan action="billing.refund">
          <div className="mb-3 flex flex-wrap gap-2">
            <Input className="w-64" value={requestID} onChange={(e) => setRequestID(e.target.value)} aria-label="账单 request_id" placeholder="request_id" />
            <ConfirmButton size="sm" variant="outline" title="确认退消费账单" description="会冲正对应佣金，并写入审计。" onConfirm={() => post("/admin/refunds", { request_id: requestID }, `已退账单 ${requestID}`)}>
              退消费账单
            </ConfirmButton>
          </div>
          <div className="mb-3 flex flex-wrap gap-2">
            <Input className="w-64" value={topupID} onChange={(e) => setTopupID(e.target.value)} aria-label="充值单 ID" placeholder="top_..." />
            <ConfirmButton size="sm" title="确认入账" description="确认后用户额度才会到账。" onConfirm={() => post(`/admin/topups/${topupID}/confirm`, {}, `已确认入账 ${topupID}`)}>
              确认入账
            </ConfirmButton>
            <ConfirmButton size="sm" variant="outline" title="确认退充值" description="只退未使用的充值额度。" onConfirm={() => post("/admin/refunds", { topup_id: topupID }, `已退充值 ${topupID}`)}>
              退充值
            </ConfirmButton>
          </div>
        </IfCan>
        <IfCan action="billing.bonus">
          <div className="mb-3 flex flex-wrap gap-2">
            <Input className="w-64" value={userID} onChange={(e) => setUserID(e.target.value)} aria-label="用户 ID" placeholder="usr_..." />
            <div>
              <Label htmlFor="bonus-usd">赠送金额（USD）</Label>
              <Input id="bonus-usd" className="w-40" inputMode="decimal" value={bonus} onChange={(e) => setBonus(e.target.value)} aria-describedby="bonus-help" />
              <p id="bonus-help" className="mt-1 text-xs text-ink-secondary">大于 0，最多 6 位小数；24 小时后到期。</p>
            </div>
            <ConfirmButton
              size="sm"
              title="确认赠送额度"
              disabled={!validBonus}
              description={`向用户 ${userID.trim()} 赠送 ${formatUsdMinor(bonusMinor)} USD，有效期 24 小时。操作会写入审计。`}
              onConfirm={() =>
                post("/admin/entitlements/bonus", { user_id: userID.trim(), unit_type: "usd_credit", amount: bonusMinor!, expires_in_seconds: 86400 }, `已向 ${userID.trim()} 赠送 ${formatUsdMinor(bonusMinor)} USD，24 小时后到期`)
              }
            >
              赠送额度
            </ConfirmButton>
          </div>
        </IfCan>
        <p className="text-sm text-ink-secondary">{message}</p>
        <h3 className="mt-6 text-base font-semibold">账务汇总</h3>
        {query.isLoading ? <p role="status">正在加载账务汇总…</p> : query.isError || !report ? (
          <p role="alert" className="mt-3 text-danger">{query.data?.error?.message || "账务汇总加载失败，请刷新页面重试。"}</p>
        ) : <dl className="mt-3 grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
          {([
            ["revenue_minor", "客户收入"], ["upstream_cost_minor", "上游成本"],
            ["wholesale_minor", "渠道批发金额"], ["commission_liability_minor", "待结算佣金"],
            ["refund_minor", "退款金额"], ["gross_profit_minor", "毛利"],
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
