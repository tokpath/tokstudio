"use client";

import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { ConfirmButton } from "@/components/confirm-button";
import { AdminShell } from "../shell";
import { apiBase } from "@/lib/api";
import { apiClient } from "@/lib/client";
import { confirmHeaders } from "@/lib/confirm";

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

  async function evaluate() {
    const res = await fetch(`${apiBase}/admin/ops/alerts/evaluate`, {
      method: "POST",
      credentials: "include",
      headers: confirmHeaders,
    });
    const body = await res.json();
    setMessage(res.ok ? `已评估 ${body.items?.length ?? 0} 条告警` : body.error?.message || "评估失败");
    await queryClient.invalidateQueries({ queryKey: ["/admin/ops/alerts"] });
  }

  return (
    <AdminShell>
      <section className="rounded-2xl border border-white/10 bg-white/[0.035] shadow-glow p-6">
        <h2 className="mb-3 text-xl font-medium">告警</h2>
        <p className="mb-3 text-sm text-slate-400">阈值在系统设置里改。评估会写审计 ops.alerts.evaluate。</p>
        <ConfirmButton size="sm" title="确认评估告警" description="评估会按阈值写入 ops_alerts，并记审计。" onConfirm={evaluate}>
          评估告警
        </ConfirmButton>
        {query.data?.error ? <p className="mt-3 text-sm text-slate-400">{query.data.error.message}</p> : null}
        <table className="mt-4 min-w-full text-left text-sm">
          <thead>
            <tr className="border-b border-white/10 text-slate-400">
              <th className="px-2 py-2">类型</th>
              <th className="px-2 py-2">级别</th>
              <th className="px-2 py-2">状态</th>
              <th className="px-2 py-2">说明</th>
            </tr>
          </thead>
          <tbody>
            {items.map((item) => (
              <tr key={item.id} className="border-b border-white/10/80">
                <td className="px-2 py-2 text-slate-200">{item.kind}</td>
                <td className="px-2 py-2 text-slate-300">{item.severity}</td>
                <td className="px-2 py-2 text-slate-300">{item.status}</td>
                <td className="px-2 py-2 text-slate-400">{item.message}</td>
              </tr>
            ))}
          </tbody>
        </table>
        <p className="mt-3 text-sm text-slate-300">{message}</p>
      </section>
    </AdminShell>
  );
}
