"use client";

import Link from "next/link";
import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { ConfirmButton } from "@/components/confirm-button";
import { AdminShell } from "../shell";
import { apiBase } from "@/lib/api";
import { apiClient } from "@/lib/client";
import { confirmHeaders, confirmNetworkUnavailable } from "@/lib/confirm";
import { AdminH2 } from "@/components/admin-h2";
import { IfCan } from "@/components/rbac/if-can";

type Alert = { id: string; kind: string; severity: string; status: string; message: string };
type ListResponse = { items?: Alert[]; error?: { message?: string } };

export default function AdminAlertsPage() {
  const [message, setMessage] = useState("按当前成功率、待对账和熔断状态生成告警。");
  const [error,setError]=useState("");
  const queryClient = useQueryClient();
  const query = useQuery({
    queryKey: ["/admin/ops/alerts"],
    queryFn: () => apiClient<ListResponse>("GET", "/admin/ops/alerts"),
  });
  const items = query.data?.items ?? [];

  async function evaluate(): Promise<boolean> {
    setError("");
    try {
    const res = await fetch(`${apiBase}/admin/ops/alerts/evaluate`, {
      method: "POST",
      credentials: "include",
      headers: confirmHeaders,
    });
    const body = await res.json();
    setMessage(res.ok ? `已评估 ${body.items?.length ?? 0} 条告警` : body.error?.message || "评估失败");
    if(!res.ok)setError(body.error?.message||"评估失败，请重试。");
    const __ok = res.ok;
    await queryClient.invalidateQueries({ queryKey: ["/admin/ops/alerts"] });
    return __ok;
    } catch {
      setError(confirmNetworkUnavailable);
      return false;
    }
}

  return (
    <AdminShell>
      <section className="rounded-card border border-hairline bg-canvas-raised  p-6">
        <AdminH2 level={1} k="alerts" className="mb-4 text-lg font-semibold tracking-tight" />
        <p className="mb-3 text-sm text-ink-secondary">按系统设置中的阈值检查当前运行情况，生成告警并保留审计记录。</p>
        <IfCan action="alerts.write">
        <ConfirmButton size="sm" title="确认评估告警" description="将按当前阈值重新评估运行情况并生成告警，不会自动修改上游配置或资金记录。" error={error} onConfirm={evaluate}>
          评估告警
        </ConfirmButton>
        </IfCan>
        {query.data?.error ? <p className="mt-3 text-sm text-ink-secondary">{query.data.error.message}</p> : null}
        {query.isPending && <p role="status">正在读取告警…</p>}
        {query.isError && <p role="alert">读取告警失败，请刷新页面重试。</p>}
        {!query.isPending && !query.isError && !query.data?.error && items.length===0 && <p className="mt-3">暂无告警记录；可点击评估检查当前运行情况。</p>}
        <Link href="/admin/runbooks" className="mt-3 block text-brand-emphasis">查看应急处置手册</Link>
        <table className="mt-4 min-w-full text-left text-sm">
          <thead>
            <tr className="border-b border-hairline text-ink-secondary">
              <th className="px-2 py-2">类型</th>
              <th className="px-2 py-2">级别</th>
              <th className="px-2 py-2">状态</th>
              <th className="px-2 py-2">说明</th>
            </tr>
          </thead>
          <tbody>
            {items.map((item) => (
              <tr key={item.id} className="border-b border-hairline/80">
                <td className="px-2 py-2 text-ink">{({backup_drill_missing:"缺少备份恢复演练",low_success_rate:"成功率偏低",pending_reconciliation:"待对账",circuit_open:"上游熔断"} as Record<string,string>)[item.kind]||item.kind}</td>
                <td className="px-2 py-2 text-ink-secondary">{({critical:"严重",high:"高",medium:"中",low:"低"} as Record<string,string>)[item.severity]||item.severity}</td>
                <td className="px-2 py-2 text-ink-secondary">{({open:"待处理",resolved:"已恢复",closed:"已关闭"} as Record<string,string>)[item.status]||item.status}</td>
                <td className="px-2 py-2 text-ink-secondary">{item.message}</td>
              </tr>
            ))}
          </tbody>
        </table>
        <p className="mt-3 text-sm text-ink-secondary">{message}</p>
      </section>
    </AdminShell>
  );
}
