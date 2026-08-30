"use client";

import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { AdminShell } from "../shell";
import { apiBase } from "@/lib/api";
import { apiClient } from "@/lib/client";

export default function AdminBillingPage() {
  const [requestID, setRequestID] = useState("");
  const [topupID, setTopupID] = useState("");
  const [userID, setUserID] = useState("");
  const [bonus, setBonus] = useState("1000000");
  const [message, setMessage] = useState("退款、确认入账和赠送额度都要二次确认，并写入审计。");
  const query = useQuery({
    queryKey: ["billing-report"],
    queryFn: () => apiClient<{ report?: Record<string, number>; error?: { message?: string } }>("GET", "/admin/billing/report"),
  });
  const report = query.data?.report || {};

  async function post(path: string, body: Record<string, string | number>, okText: string) {
    const res = await fetch(`${apiBase}${path}`, {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json", "X-Tokenhub-Confirm": "1" },
      body: JSON.stringify(body),
    });
    const payload = await res.json();
    setMessage(res.ok ? okText : payload.error?.message || "操作失败");
    await query.refetch();
  }

  return (
    <AdminShell>
      <section className="rounded-2xl border border-white/10 bg-white/[0.035] shadow-glow p-6">
        <h2 className="mb-3 text-xl font-medium">余额 / 充值 / 账务</h2>
        <p className="mb-3 text-sm text-slate-400">按 request_id 退消费账单会冲正佣金；按 topup_id 退未使用充值。赠送额度默认 usd_credit。</p>
        <div className="mb-3 flex flex-wrap gap-2">
          <Input className="w-64" value={requestID} onChange={(e) => setRequestID(e.target.value)} aria-label="账单 request_id" placeholder="request_id" />
          <Button size="sm" variant="outline" onClick={() => post("/admin/refunds", { request_id: requestID }, `已退账单 ${requestID}`)}>
            退消费账单
          </Button>
        </div>
        <div className="mb-3 flex flex-wrap gap-2">
          <Input className="w-64" value={topupID} onChange={(e) => setTopupID(e.target.value)} aria-label="充值单 ID" placeholder="top_..." />
          <Button size="sm" onClick={() => post(`/admin/topups/${topupID}/confirm`, {}, `已确认入账 ${topupID}`)}>
            确认入账
          </Button>
          <Button size="sm" variant="outline" onClick={() => post("/admin/refunds", { topup_id: topupID }, `已退充值 ${topupID}`)}>
            退充值
          </Button>
        </div>
        <div className="mb-3 flex flex-wrap gap-2">
          <Input className="w-64" value={userID} onChange={(e) => setUserID(e.target.value)} aria-label="用户 ID" placeholder="usr_..." />
          <Input className="w-36" value={bonus} onChange={(e) => setBonus(e.target.value)} aria-label="赠送额度" placeholder="amount" />
          <Button
            size="sm"
            onClick={() =>
              post("/admin/entitlements/bonus", { user_id: userID, unit_type: "usd_credit", amount: Number(bonus), expires_in_seconds: 86400 }, `已赠送 ${bonus} 给 ${userID}`)
            }
          >
            赠送额度
          </Button>
        </div>
        <p className="text-sm text-slate-300">{message}</p>
        <pre className="mt-3 overflow-x-auto text-sm text-slate-200">{JSON.stringify(report, null, 2) || query.data?.error?.message}</pre>
      </section>
    </AdminShell>
  );
}
