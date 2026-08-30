"use client";

import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Card, CardTitle } from "@/components/ui/card";
import { apiBase } from "@/lib/api";

type Settlement = { id?: string; status?: string; amount_minor?: number; channel_org_id?: string };

export default function ChannelSettlements() {
  const [items, setItems] = useState<Settlement[]>([]);
  const [message, setMessage] = useState("渠道只能看本渠道结算单。P0 打款由平台财务人工完成。");

  async function refresh() {
    const response = await fetch(`${apiBase}/channel/settlements`, { credentials: "include" });
    const body = await response.json();
    if (!response.ok) {
      setMessage(body.error?.message || "未登录渠道管理员");
      return;
    }
    const next = (body.items || []) as Settlement[];
    setItems(next);
    setMessage(`本渠道结算单 ${next.length} 张`);
  }

  return (
    <Card className="rounded-2xl border border-white/10 bg-white/[0.035] shadow-glow p-6">
      <CardTitle className="mb-3 text-xl font-medium">本渠道结算</CardTitle>
      <p className="mb-3 text-sm text-slate-400">佣金冻结期满后按月出结算单。这里不含其他渠道，也不含 prompt。</p>
      <Button variant="outline" onClick={refresh}>
        刷新结算
      </Button>
      <ul className="mt-3 space-y-2 text-sm text-slate-200">
        {items.map((item) => (
          <li key={item.id}>
            {item.id} · {item.status} · {item.amount_minor ?? 0} micro-USD
          </li>
        ))}
      </ul>
      <p className="mt-3 text-sm text-slate-300">{message}</p>
    </Card>
  );
}
