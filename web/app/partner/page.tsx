"use client";

import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Card, CardTitle } from "@/components/ui/card";
import { apiBase } from "@/lib/api";

type PartnerMe = {
  role_type?: string;
  channel_org_id?: string;
  sees_downline?: boolean;
};

type PartnerUser = { email?: string; source_code?: string; status?: string };
type Commission = { id?: string; kind?: string; status?: string; amount_minor?: number };
type Settlement = { id?: string; status?: string; amount_minor?: number };

export default function PartnerConsole() {
  const [me, setMe] = useState<PartnerMe>({});
  const [users, setUsers] = useState<PartnerUser[]>([]);
  const [comms, setComms] = useState<Commission[]>([]);
  const [settlements, setSettlements] = useState<Settlement[]>([]);
  const [message, setMessage] = useState("代理商看整棵树，1 级 KOL 看自己和 2 级，2 级只看直接引流。邮箱已脱敏。");

  async function refresh() {
    const [meRes, userRes, commRes, setRes] = await Promise.all([
      fetch(`${apiBase}/v1/partner/me`, { credentials: "include" }),
      fetch(`${apiBase}/v1/partner/users`, { credentials: "include" }),
      fetch(`${apiBase}/v1/partner/commissions`, { credentials: "include" }),
      fetch(`${apiBase}/v1/partner/settlements`, { credentials: "include" }),
    ]);
    const meBody = await meRes.json();
    if (!meRes.ok) {
      setMessage(meBody.error?.message || "不是推广主体");
      return;
    }
    setMe(meBody as PartnerMe);
    const userBody = await userRes.json();
    const commBody = await commRes.json();
    const setBody = await setRes.json();
    setUsers((userBody.items || []) as PartnerUser[]);
    setComms((commBody.items || []) as Commission[]);
    setSettlements((setBody.items || []) as Settlement[]);
    setMessage(`层级 ${meBody.role_type || "—"} · 用户 ${userBody.items?.length ?? 0} · 佣金 ${commBody.items?.length ?? 0} · 结算 ${setBody.items?.length ?? 0}`);
  }

  return (
    <main className="mx-auto flex max-w-5xl flex-col gap-6">
      <header>
        <p className="text-sm uppercase tracking-[0.2em] text-slate-400">分销控制台</p>
        <h1 className="mt-2 text-3xl font-semibold tracking-tight">我的推广范围</h1>
        <p className="mt-2 max-w-2xl text-slate-300">
          这里按代理商 / 1 级 KOL / 2 级 KOL 分层。后端再校验角色树，前端隐藏不是安全边界。看不到 prompt，也不能改佣金比例。
        </p>
      </header>
      <Card id="scope">
        <CardTitle>我的层级</CardTitle>
        <p className="mb-3 text-sm text-slate-400">
          当前 {me.role_type || "未登录"} · 渠道 {me.channel_org_id || "—"} ·{" "}
          {me.sees_downline ? "可看下级汇总" : "只看直接引流"}
        </p>
        <Button variant="outline" onClick={refresh}>
          刷新范围
        </Button>
        <p className="mt-3 text-sm text-slate-300">{message}</p>
      </Card>
      <Card id="users">
        <CardTitle>范围内用户</CardTitle>
        <ul className="space-y-2 text-sm text-slate-200">
          {users.map((item) => (
            <li key={`${item.email}-${item.source_code}`} className="rounded-lg border border-white/5 bg-black/20 px-3 py-2">
              {item.email} · {item.source_code || "—"} · {item.status}
            </li>
          ))}
        </ul>
      </Card>
      <Card id="commissions">
        <CardTitle>范围内佣金</CardTitle>
        <ul className="space-y-2 text-sm text-slate-200">
          {comms.map((item) => (
            <li key={item.id} className="rounded-lg border border-white/5 bg-black/20 px-3 py-2">
              {item.kind} · {item.status} · {item.amount_minor ?? 0} micro-USD
            </li>
          ))}
        </ul>
      </Card>
      <Card id="settlements">
        <CardTitle>范围内结算</CardTitle>
        <ul className="space-y-2 text-sm text-slate-200">
          {settlements.map((item) => (
            <li key={item.id} className="rounded-lg border border-white/5 bg-black/20 px-3 py-2">
              {item.id} · {item.status} · {item.amount_minor ?? 0} micro-USD
            </li>
          ))}
        </ul>
      </Card>
    </main>
  );
}
