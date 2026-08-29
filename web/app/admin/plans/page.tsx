"use client";

import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { AdminShell } from "../shell";
import { apiBase } from "@/lib/api";
import { apiClient } from "@/lib/client";

type Plan = {
  id: string;
  name: string;
  owner_type: string;
  owner_id: string;
  price_minor: number;
  status: string;
  review_reason?: string;
};

type ListResponse = { items?: Plan[]; error?: { message?: string } };

export default function AdminPlansPage() {
  const [status, setStatus] = useState("pending_review");
  const [reason, setReason] = useState("promo");
  const [message, setMessage] = useState("渠道低价或高风险媒体配额会进入待审核。通过或拒绝都会写审计。");
  const [writeMessage, setWriteMessage] = useState("平台套餐满 1 USD 会直接发布。不要下架 pln_echo_month，那是公共站演示套餐。");
  const queryClient = useQueryClient();
  const path = status ? `/admin/plans?status=${encodeURIComponent(status)}` : "/admin/plans";
  const query = useQuery({
    queryKey: [path],
    queryFn: () => apiClient<ListResponse>("GET", path),
  });
  const items = query.data?.items ?? [];

  async function review(id: string, action: "approve" | "reject") {
    const res = await fetch(`${apiBase}/admin/plans/${id}/review`, {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json", "X-Tokenhub-Confirm": "1" },
      body: JSON.stringify({ action, reason }),
    });
    const body = await res.json();
    setMessage(res.ok ? `已${action === "approve" ? "通过" : "拒绝"} ${body.item?.id}` : body.error?.message || "审核失败");
    await queryClient.invalidateQueries({ queryKey: [path] });
  }

  return (
    <AdminShell>
      <section className="rounded-2xl border border-slate-800 bg-slate-900/70 p-6">
        <h2 className="mb-3 text-xl font-medium">套餐审核</h2>
        <p className="mb-3 text-sm text-slate-400">低于 1 USD、超额权益或高风险视频秒数的渠道套餐会停在 pending_review。</p>
        <div className="mb-3 flex flex-wrap items-center gap-2">
          <Button size="sm" variant={status === "pending_review" ? "default" : "outline"} onClick={() => setStatus("pending_review")}>
            待审核
          </Button>
          <Button size="sm" variant={status === "" ? "default" : "outline"} onClick={() => setStatus("")}>
            全部套餐
          </Button>
          <Input className="w-48" value={reason} onChange={(e) => setReason(e.target.value)} aria-label="审核原因" placeholder="审核原因" />
        </div>
        {query.data?.error ? <p className="text-sm text-slate-400">{query.data.error.message}</p> : null}
        <table className="min-w-full text-left text-sm">
          <thead>
            <tr className="border-b border-slate-800 text-slate-400">
              <th className="px-2 py-2">名称</th>
              <th className="px-2 py-2">归属</th>
              <th className="px-2 py-2">价格</th>
              <th className="px-2 py-2">状态</th>
              <th className="px-2 py-2">原因</th>
              <th className="px-2 py-2">操作</th>
            </tr>
          </thead>
          <tbody>
            {items.map((item) => (
              <tr key={item.id} className="border-b border-slate-800/80">
                <td className="px-2 py-2 text-slate-200">{item.name}</td>
                <td className="px-2 py-2 text-slate-300">
                  {item.owner_type} / {item.owner_id}
                </td>
                <td className="px-2 py-2 text-slate-300">{item.price_minor}</td>
                <td className="px-2 py-2 text-slate-300">{item.status}</td>
                <td className="px-2 py-2 text-slate-400">{item.review_reason || "-"}</td>
                <td className="px-2 py-2">
                  {item.status === "pending_review" ? (
                    <div className="flex flex-wrap gap-2">
                      <Button size="sm" onClick={() => review(item.id, "approve")}>
                        通过
                      </Button>
                      <Button size="sm" variant="outline" onClick={() => review(item.id, "reject")}>
                        拒绝
                      </Button>
                    </div>
                  ) : (
                    item.id
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        <p className="mt-3 text-sm text-slate-300">{message}</p>
      </section>
      <form
        className="rounded-2xl border border-slate-800 bg-slate-900/70 p-6"
        onSubmit={async (event) => {
          event.preventDefault();
          const form = event.currentTarget;
          const data = new FormData(form);
          const res = await fetch(`${apiBase}/admin/plans`, {
            method: "POST",
            credentials: "include",
            headers: { "Content-Type": "application/json", "X-Tokenhub-Confirm": "1" },
            body: JSON.stringify({
              name: String(data.get("name") || "").trim(),
              owner_type: String(data.get("owner_type") || "platform").trim() || "platform",
              price_minor: Number(data.get("price_minor") || 0),
              items: [
                {
                  unit_type: String(data.get("unit_type") || "usd_credit").trim() || "usd_credit",
                  included_amount: Number(data.get("included_amount") || 0),
                },
              ],
            }),
          });
          const body = await res.json();
          if (!res.ok) {
            setWriteMessage(body.error?.message || "创建失败");
            return;
          }
          form.reset();
          setWriteMessage(`已创建 ${body.item?.id} ${body.item?.name} → ${body.item?.status}`);
          await queryClient.invalidateQueries();
        }}
      >
        <h2 className="mb-3 text-xl font-medium">创建套餐</h2>
        <p className="mb-3 text-sm text-slate-400">价格单位是 micro-USD。渠道套餐低于 1 USD 会进 pending_review；平台套餐会直接 published。</p>
        <div className="mb-3 grid max-w-xl gap-2">
          <Input name="name" aria-label="创建用套餐名" placeholder="创建用套餐名" />
          <Input name="owner_type" aria-label="创建用归属" placeholder="创建用归属 platform" defaultValue="platform" />
          <Input name="price_minor" aria-label="创建用价格" placeholder="创建用价格 1000000" defaultValue="1000000" />
          <Input name="unit_type" aria-label="创建用权益单位" placeholder="创建用权益单位 usd_credit" defaultValue="usd_credit" />
          <Input name="included_amount" aria-label="创建用权益数量" placeholder="创建用权益数量" defaultValue="1000000" />
        </div>
        <Button size="sm" type="submit">
          创建套餐
        </Button>
      </form>
      <form
        className="rounded-2xl border border-slate-800 bg-slate-900/70 p-6"
        onSubmit={async (event) => {
          event.preventDefault();
          const form = event.currentTarget;
          const data = new FormData(form);
          const id = String(data.get("plan_id") || "").trim();
          const res = await fetch(`${apiBase}/admin/plans/${id}`, {
            method: "PATCH",
            credentials: "include",
            headers: { "Content-Type": "application/json", "X-Tokenhub-Confirm": "1" },
            body: JSON.stringify({ status: "archived" }),
          });
          const body = await res.json();
          if (!res.ok) {
            setWriteMessage(body.error?.message || "下架失败");
            return;
          }
          setWriteMessage(`已下架 ${body.item?.id} → ${body.item?.status}`);
          await queryClient.invalidateQueries();
        }}
      >
        <h2 className="mb-3 text-xl font-medium">下架套餐</h2>
        <p className="mb-3 text-sm text-slate-400">只改成 archived，不删历史订阅。不要下架 pln_echo_month。</p>
        <div className="mb-3 grid max-w-xl gap-2">
          <Input name="plan_id" aria-label="下架用套餐 ID" placeholder="下架用套餐 ID" />
        </div>
        <Button size="sm" type="submit">
          下架套餐
        </Button>
      </form>
      <p className="text-sm text-slate-300">{writeMessage}</p>
    </AdminShell>
  );
}
