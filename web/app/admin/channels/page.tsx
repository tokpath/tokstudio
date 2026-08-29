"use client";

import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { AdminListPanel } from "../list-panel";
import { AdminShell } from "../shell";
import { apiBase } from "@/lib/api";

type Channel = { id: string; code: string; type: string; status: string; brand_id: string };

export default function AdminChannelsPage() {
  const [channelID, setChannelID] = useState("chn_reseller_b");
  const [amount, setAmount] = useState("1000000");
  const [message, setMessage] = useState("渠道额度按 micro-USD。发放和扣减都要二次确认。");

  async function loadQuota() {
    const res = await fetch(`${apiBase}/admin/channel-quotas/${channelID}`, { credentials: "include" });
    const body = await res.json();
    setMessage(res.ok ? `可用额度 ${body.quota?.available_minor}` : body.error?.message || "读取额度失败");
  }

  async function grant() {
    const res = await fetch(`${apiBase}/admin/channel-quotas/grant`, {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json", "X-Tokenhub-Confirm": "1" },
      body: JSON.stringify({ channel_org_id: channelID, amount_minor: Number(amount) }),
    });
    const body = await res.json();
    setMessage(res.ok ? `已调整 ${channelID}，可用 ${body.quota?.available_minor}` : body.error?.message || "调整失败");
  }

  return (
    <AdminShell>
      <section className="rounded-2xl border border-slate-800 bg-slate-900/70 p-6">
        <h2 className="mb-3 text-xl font-medium">渠道额度</h2>
        <p className="mb-3 text-sm text-slate-400">B/C 渠道累计消耗不能超过额度。正数发放，负数扣减。额度不足时预授权失败。</p>
        <div className="mb-3 flex flex-wrap gap-2">
          <Input className="w-56" value={channelID} onChange={(e) => setChannelID(e.target.value)} aria-label="渠道 ID" placeholder="chn_..." />
          <Input className="w-40" value={amount} onChange={(e) => setAmount(e.target.value)} aria-label="额度 micro-USD" placeholder="amount_minor" />
          <Button size="sm" variant="outline" onClick={loadQuota}>
            读取额度
          </Button>
          <Button size="sm" onClick={grant}>
            调整额度
          </Button>
        </div>
        <p className="text-sm text-slate-300">{message}</p>
      </section>
      <AdminListPanel<Channel>
        path="/admin/channels"
        title="渠道 / 代理商"
        columns={[
          { accessorKey: "code", header: "Code" },
          { accessorKey: "type", header: "Type" },
          { accessorKey: "status", header: "Status" },
          { accessorKey: "brand_id", header: "Brand" },
        ]}
      />
    </AdminShell>
  );
}
