"use client";

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
  const [message, setMessage] = useState("评估成功率、待对账和熔断后写入 ops_alerts。");
  const queryClient = useQueryClient();
  const query = useQuery({
    queryKey: ["/admin/ops/alerts"],
    queryFn: () => apiClient<ListResponse>("GET", "/admin/ops/alerts"),
  });
  const items = query.data?.items ?? [];

  async function evaluate(): Promise<boolean> {
    try {
    const res = await fetch(`${apiBase}/admin/ops/alerts/evaluate`, {
      method: "POST",
      credentials: "include",
      headers: confirmHeaders,
    });
    const body = await res.json();
    setMessage(res.ok ? `已评估 ${body.items?.length ?? 0} 条告警` : body.error?.message || "评估失败");
    const __ok = res.ok;
    await queryClient.invalidateQueries({ queryKey: ["/admin/ops/alerts"] });
    return __ok;
    } catch {
      setMessage(confirmNetworkUnavailable);
      return false;
    }
}

  return (
    <AdminShell>
      <section className="rounded-card border border-hairline bg-canvas-raised  p-6">
        <AdminH2 k="alerts" className="mb-4 text-lg font-semibold tracking-tight" />
        <p className="mb-3 text-sm text-ink-secondary">阈值在系统设置里改。评估会写审计 ops.alerts.evaluate。</p>
        <IfCan action="alerts.write">
        <ConfirmButton size="sm" title="确认评估告警" description="评估会按阈值写入 ops_alerts，并记审计。" onConfirm={evaluate}>
          评估告警
        </ConfirmButton>
        </IfCan>
        {query.data?.error ? <p className="mt-3 text-sm text-ink-secondary">{query.data.error.message}</p> : null}
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
                <td className="px-2 py-2 text-ink">{item.kind}</td>
                <td className="px-2 py-2 text-ink-secondary">{item.severity}</td>
                <td className="px-2 py-2 text-ink-secondary">{item.status}</td>
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
