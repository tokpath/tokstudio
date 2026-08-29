"use client";

import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { AdminListPanel } from "../list-panel";
import { AdminShell } from "../shell";
import { apiBase } from "@/lib/api";
import { apiClient } from "@/lib/client";

type Policy = {
  id?: string;
  version?: string;
  direct_bps?: number;
  override_bps?: number;
  channel_bps?: number;
  team_bps?: number;
  cap_bps?: number;
  freeze_days?: number;
  min_settle_minor?: number;
};

type Commission = { id: string; kind: string; status: string; amount_minor: number; channel_org_id?: string };
type Settlement = { id: string; status: string; amount_minor: number; channel_org_id?: string };

export default function AdminCommissionPage() {
  const [direct, setDirect] = useState("1500");
  const [overrideBps, setOverrideBps] = useState("500");
  const [channel, setChannel] = useState("500");
  const [team, setTeam] = useState("0");
  const [cap, setCap] = useState("3500");
  const [freeze, setFreeze] = useState("7");
  const [minSettle, setMinSettle] = useState("1000000");
  const [message, setMessage] = useState("BPS 是万分比。各档之和不能超过上限。保存需要二次确认。");

  const policyQuery = useQuery({
    queryKey: ["commission-policy"],
    queryFn: () => apiClient<{ policy?: Policy; error?: { message?: string } }>("GET", "/admin/commission-policy"),
  });

  async function loadPolicy() {
    const res = await fetch(`${apiBase}/admin/commission-policy`, { credentials: "include" });
    const body = await res.json();
    if (!res.ok) {
      setMessage(body.error?.message || "未登录平台管理员");
      return;
    }
    const p = body.policy as Policy;
    setDirect(String(p.direct_bps ?? 1500));
    setOverrideBps(String(p.override_bps ?? 500));
    setChannel(String(p.channel_bps ?? 500));
    setTeam(String(p.team_bps ?? 0));
    setCap(String(p.cap_bps ?? 3500));
    setFreeze(String(p.freeze_days ?? 7));
    setMinSettle(String(p.min_settle_minor ?? 1_000_000));
    setMessage(`已读取策略 ${p.version || p.id}`);
  }

  async function savePolicy() {
    const res = await fetch(`${apiBase}/admin/commission-policy`, {
      method: "PATCH",
      credentials: "include",
      headers: { "Content-Type": "application/json", "X-Tokenhub-Confirm": "1" },
      body: JSON.stringify({
        direct_bps: Number(direct),
        override_bps: Number(overrideBps),
        channel_bps: Number(channel),
        team_bps: Number(team),
        cap_bps: Number(cap),
        freeze_days: Number(freeze),
        min_settle_minor: Number(minSettle),
      }),
    });
    const body = await res.json();
    setMessage(res.ok ? `已保存 ${body.policy?.version}，直接佣金 ${body.policy?.direct_bps} bps` : body.error?.message || "保存失败");
  }

  return (
    <AdminShell>
      <section className="rounded-2xl border border-slate-800 bg-slate-900/70 p-6">
        <h2 className="mb-3 text-xl font-medium">佣金策略</h2>
        <p className="mb-3 text-sm text-slate-400">
          直接 / 管理奖励 / 渠道 / 团队分成与冻结天数。改策略只影响之后的 usage，不改已经入账的明细。
        </p>
        <div className="mb-3 grid max-w-3xl grid-cols-2 gap-2 md:grid-cols-4">
          <Input value={direct} onChange={(e) => setDirect(e.target.value)} aria-label="直接佣金 BPS" placeholder="direct_bps" />
          <Input value={overrideBps} onChange={(e) => setOverrideBps(e.target.value)} aria-label="管理奖励 BPS" placeholder="override_bps" />
          <Input value={channel} onChange={(e) => setChannel(e.target.value)} aria-label="渠道 BPS" placeholder="channel_bps" />
          <Input value={team} onChange={(e) => setTeam(e.target.value)} aria-label="团队 BPS" placeholder="team_bps" />
          <Input value={cap} onChange={(e) => setCap(e.target.value)} aria-label="上限 BPS" placeholder="cap_bps" />
          <Input value={freeze} onChange={(e) => setFreeze(e.target.value)} aria-label="冻结天数" placeholder="freeze_days" />
          <Input value={minSettle} onChange={(e) => setMinSettle(e.target.value)} aria-label="最低结算额" placeholder="min_settle_minor" />
        </div>
        <div className="flex flex-wrap gap-2">
          <Button size="sm" variant="outline" onClick={loadPolicy}>
            读取策略
          </Button>
          <Button size="sm" onClick={savePolicy}>
            保存策略
          </Button>
        </div>
        <p className="mt-3 text-sm text-slate-300">{message}</p>
        {policyQuery.data?.error ? <p className="mt-2 text-sm text-slate-400">{policyQuery.data.error.message}</p> : null}
      </section>
      <AdminListPanel<Commission>
        path="/admin/commissions"
        title="佣金明细"
        columns={[
          { accessorKey: "kind", header: "Kind" },
          { accessorKey: "status", header: "Status" },
          { accessorKey: "amount_minor", header: "Amount" },
          { accessorKey: "channel_org_id", header: "Channel" },
        ]}
      />
      <AdminListPanel<Settlement>
        path="/admin/settlements"
        title="结算单"
        columns={[
          { accessorKey: "id", header: "ID" },
          { accessorKey: "status", header: "Status" },
          { accessorKey: "amount_minor", header: "Amount" },
          { accessorKey: "channel_org_id", header: "Channel" },
        ]}
      />
    </AdminShell>
  );
}
