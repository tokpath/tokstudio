"use client";

import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { ConfirmButton } from "@/components/confirm-button";
import { Input } from "@/components/ui/input";
import { AdminShell } from "../shell";
import { apiBase } from "@/lib/api";
import { apiClient } from "@/lib/client";
import { confirmHeaders, confirmNetworkUnavailable } from "@/lib/confirm";
import { AdminH2 } from "@/components/admin-h2";
import { IfCan } from "@/components/rbac/if-can";

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

  async function postAction(path: string, body: Record<string, string>, okText: string): Promise<boolean> {
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
    await queryClient.invalidateQueries({ queryKey: ["/admin/users"] });
    return __ok;
    } catch {
      setMessage(confirmNetworkUnavailable);
      return false;
    }
}

  return (
    <AdminShell>
      <section className="rounded-card border border-hairline bg-canvas-raised  p-6">
        <AdminH2 k="users" className="mb-4 text-lg font-semibold tracking-tight" />
        <p className="mb-3 text-sm text-ink-secondary">封禁、解封和人工改归因都要二次确认，并写入审计。平台管理员不能封自己。</p>
        <div className="mb-3 flex flex-wrap items-center gap-2">
          <Input className="w-40" value={reason} onChange={(e) => setReason(e.target.value)} aria-label="操作原因" placeholder="操作原因" />
          <Input className="w-32" value={promo} onChange={(e) => setPromo(e.target.value)} aria-label="推广码" placeholder="推广码" />
        </div>
        {query.data?.error ? <p className="text-sm text-ink-secondary">{query.data.error.message}</p> : null}
        <table className="min-w-full text-left text-sm">
          <thead>
            <tr className="border-b border-hairline text-ink-secondary">
              <th className="px-2 py-2">Email</th>
              <th className="px-2 py-2">渠道</th>
              <th className="px-2 py-2">归因</th>
              <th className="px-2 py-2">状态</th>
              <th className="px-2 py-2">操作</th>
            </tr>
          </thead>
          <tbody>
            {items.map((item) => (
              <tr key={item.id} className="border-b border-hairline/80">
                <td className="px-2 py-2 text-ink">{item.email}</td>
                <td className="px-2 py-2 text-ink-secondary">{item.channel_org_id}</td>
                <td className="px-2 py-2 text-ink-secondary">{item.source_code || "-"}</td>
                <td className="px-2 py-2 text-ink-secondary">{item.status}</td>
                <td className="px-2 py-2">
                  <IfCan action="users.write">
                    <div className="flex flex-wrap gap-2">
                      {item.status === "banned" ? (
                        <ConfirmButton size="sm" title="确认解封用户" description={`将解封 ${item.email}，并写入审计。`} onConfirm={() => postAction(`/admin/users/${item.id}/unban`, { reason }, `已解封 ${item.email}`)}>
                          解封
                        </ConfirmButton>
                      ) : (
                        <ConfirmButton size="sm" variant="outline" title="确认封禁用户" description={`将封禁 ${item.email}，停用登录和旧 API Key。`} onConfirm={() => postAction(`/admin/users/${item.id}/ban`, { reason }, `已封禁 ${item.email}`)}>
                          封禁
                        </ConfirmButton>
                      )}
                      <ConfirmButton
                        size="sm"
                        variant="outline"
                        title="确认改归因"
                        description={`将把 ${item.email} 的归因改成 ${promo}。`}
                        onConfirm={() => postAction(`/admin/users/${item.id}/attribution`, { promotion_code: promo, reason }, `已改归因 ${item.email} → ${promo}`)}
                      >
                        改归因
                      </ConfirmButton>
                    </div>
                  </IfCan>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        <p className="mt-3 text-sm text-ink-secondary">{message}</p>
      </section>
    </AdminShell>
  );
}
