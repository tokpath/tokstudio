"use client";

import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { AdminShell } from "../shell";
import { apiBase } from "@/lib/api";
import { apiClient } from "@/lib/client";

type User = {
  id: string;
  email: string;
  channel_org_id: string;
  status: string;
  source_code?: string;
};

type ListResponse = { items?: User[]; error?: { message?: string } };

export default function AdminUsersPage() {
  const [reason, setReason] = useState("abuse");
  const [promo, setPromo] = useState("THB1");
  const [message, setMessage] = useState("封禁会立刻停用登录和旧 API Key，并冻结该用户未结算佣金。改归因必须写原因。");
  const queryClient = useQueryClient();
  const query = useQuery({
    queryKey: ["/admin/users"],
    queryFn: () => apiClient<ListResponse>("GET", "/admin/users"),
  });
  const items = query.data?.items ?? [];

  async function postAction(path: string, body: Record<string, string>, okText: string) {
    const res = await fetch(`${apiBase}${path}`, {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json", "X-Tokenhub-Confirm": "1" },
      body: JSON.stringify(body),
    });
    const payload = await res.json();
    setMessage(res.ok ? okText : payload.error?.message || "操作失败");
    await queryClient.invalidateQueries({ queryKey: ["/admin/users"] });
  }

  return (
    <AdminShell>
      <section className="rounded-2xl border border-slate-800 bg-slate-900/70 p-6">
        <h2 className="mb-3 text-xl font-medium">用户/项目</h2>
        <p className="mb-3 text-sm text-slate-400">封禁、解封和人工改归因都要二次确认，并写入审计。平台管理员不能封自己。</p>
        <div className="mb-3 flex flex-wrap items-center gap-2">
          <Input className="w-40" value={reason} onChange={(e) => setReason(e.target.value)} aria-label="操作原因" placeholder="操作原因" />
          <Input className="w-32" value={promo} onChange={(e) => setPromo(e.target.value)} aria-label="推广码" placeholder="推广码" />
        </div>
        {query.data?.error ? <p className="text-sm text-slate-400">{query.data.error.message}</p> : null}
        <table className="min-w-full text-left text-sm">
          <thead>
            <tr className="border-b border-slate-800 text-slate-400">
              <th className="px-2 py-2">Email</th>
              <th className="px-2 py-2">渠道</th>
              <th className="px-2 py-2">归因</th>
              <th className="px-2 py-2">状态</th>
              <th className="px-2 py-2">操作</th>
            </tr>
          </thead>
          <tbody>
            {items.map((item) => (
              <tr key={item.id} className="border-b border-slate-800/80">
                <td className="px-2 py-2 text-slate-200">{item.email}</td>
                <td className="px-2 py-2 text-slate-300">{item.channel_org_id}</td>
                <td className="px-2 py-2 text-slate-300">{item.source_code || "-"}</td>
                <td className="px-2 py-2 text-slate-300">{item.status}</td>
                <td className="px-2 py-2">
                  <div className="flex flex-wrap gap-2">
                    {item.status === "banned" ? (
                      <Button size="sm" onClick={() => postAction(`/admin/users/${item.id}/unban`, { reason }, `已解封 ${item.email}`)}>
                        解封
                      </Button>
                    ) : (
                      <Button size="sm" variant="outline" onClick={() => postAction(`/admin/users/${item.id}/ban`, { reason }, `已封禁 ${item.email}`)}>
                        封禁
                      </Button>
                    )}
                    <Button
                      size="sm"
                      variant="outline"
                      onClick={() =>
                        postAction(`/admin/users/${item.id}/attribution`, { promotion_code: promo, reason }, `已改归因 ${item.email} → ${promo}`)
                      }
                    >
                      改归因
                    </Button>
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        <p className="mt-3 text-sm text-slate-300">{message}</p>
      </section>
    </AdminShell>
  );
}
