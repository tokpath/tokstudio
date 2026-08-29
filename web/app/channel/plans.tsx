"use client";

import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Card, CardTitle } from "@/components/ui/card";
import { apiBase } from "@/lib/api";

type Plan = { id?: string; name?: string; status?: string; owner_type?: string; price_minor?: number };

export default function ChannelPlans() {
  const [items, setItems] = useState<Plan[]>([]);
  const [message, setMessage] = useState("渠道管理员可看本渠道套餐和平台套餐，看不到其他渠道的销售组合。");

  async function refresh() {
    const response = await fetch(`${apiBase}/channel/plans`, { credentials: "include" });
    const body = await response.json();
    if (!response.ok) {
      setMessage(body.error?.message || "未登录渠道管理员");
      return;
    }
    const next = (body.items || []) as Plan[];
    setItems(next);
    setMessage(`本渠道可见套餐 ${next.length} 个`);
  }

  return (
    <Card className="rounded-2xl border border-slate-800 bg-slate-900/70 p-6">
      <CardTitle className="mb-3 text-xl font-medium">本渠道套餐</CardTitle>
      <p className="mb-3 text-sm text-slate-400">低价或高风险配额会进平台审核。这里只列本渠道 scope。</p>
      <Button variant="outline" onClick={refresh}>
        刷新套餐
      </Button>
      <ul className="mt-3 space-y-2 text-sm text-slate-200">
        {items.map((item) => (
          <li key={item.id}>
            {item.name} · {item.status} · {item.owner_type} · {item.price_minor ?? 0} micro-USD
          </li>
        ))}
      </ul>
      <p className="mt-3 text-sm text-slate-300">{message}</p>
    </Card>
  );
}
