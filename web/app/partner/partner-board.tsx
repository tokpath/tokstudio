"use client";

import { useState } from "react";
import { LedgerTable } from "@/components/console/ledger-table";
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

export type PartnerSection = "all" | "scope" | "users" | "commissions" | "settlements";

export function PartnerBoard({ section = "all" }: { section?: PartnerSection }) {
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

  const show = (id: PartnerSection) => section === "all" || section === id;

  return (
    <div className="flex flex-col gap-6">
      {section !== "all" ? (
        <Button variant="outline" className="self-start" onClick={() => void refresh()}>
          刷新范围
        </Button>
      ) : null}
      {show("scope") ? (
        <Card id="scope">
          <CardTitle>我的层级</CardTitle>
          <p className="mb-3 text-sm text-ink-secondary">
            当前 {me.role_type || "未登录"} · 渠道 {me.channel_org_id || "—"} ·{" "}
            {me.sees_downline ? "可看下级汇总" : "只看直接引流"}
          </p>
          <Button variant="outline" onClick={() => void refresh()}>
            刷新范围
          </Button>
          <p className="mt-3 text-sm text-ink-secondary">{message}</p>
        </Card>
      ) : null}
      {show("users") ? (
        <Card id="users">
          <CardTitle>范围内用户</CardTitle>
          <LedgerTable
            columns={["邮箱", "推广码", "状态"]}
            emptyTitle="暂无用户"
            emptyDetail="登录推广主体后刷新，邮箱已脱敏。"
            rows={users.map((item) => ({
              key: `${item.email}-${item.source_code}`,
              cells: [item.email || "—", item.source_code || "—", item.status || "—"],
            }))}
          />
        </Card>
      ) : null}
      {show("commissions") ? (
        <Card id="commissions">
          <CardTitle>范围内佣金</CardTitle>
          <LedgerTable
            columns={["类型", "状态", "金额"]}
            emptyTitle="暂无佣金"
            emptyDetail="冻结期满前不会出现可结算金额。"
            rows={comms.map((item) => ({
              key: item.id || `${item.kind}-${item.status}`,
              cells: [item.kind || "—", item.status || "—", `${item.amount_minor ?? 0} micro-USD`],
            }))}
          />
        </Card>
      ) : null}
      {show("settlements") ? (
        <Card id="settlements">
          <CardTitle>范围内结算</CardTitle>
          <LedgerTable
            columns={["结算单", "状态", "金额"]}
            emptyTitle="暂无结算单"
            emptyDetail="平台财务打款后才会出现在这里。"
            rows={settlements.map((item) => ({
              key: item.id || "settlement",
              cells: [item.id || "—", item.status || "—", `${item.amount_minor ?? 0} micro-USD`],
            }))}
          />
        </Card>
      ) : null}
    </div>
  );
}
