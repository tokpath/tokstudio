"use client";

import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Card, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { apiBase } from "@/lib/api";

type Plan = { id?: string; name?: string; status?: string; owner_type?: string; owner_id?: string; price_minor?: number; review_reason?: string };

export default function ChannelPlans() {
  const [items, setItems] = useState<Plan[]>([]);
  const [message, setMessage] = useState("渠道管理员可看本渠道套餐和平台套餐，看不到其他渠道的销售组合。");
  const [createMessage, setCreateMessage] = useState("低于 1 USD 或高风险配额会进 pending_review。归属会被后端写成当前渠道，改不了官方渠道。");

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

  async function create(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = event.currentTarget;
    const data = new FormData(form);
    const response = await fetch(`${apiBase}/channel/plans`, {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        name: String(data.get("name") || "").trim(),
        price_minor: Number(data.get("price_minor") || 0),
        items: [
          {
            unit_type: String(data.get("unit_type") || "usd_credit").trim() || "usd_credit",
            included_amount: Number(data.get("included_amount") || 0),
          },
        ],
      }),
    });
    const body = await response.json();
    if (!response.ok) {
      setCreateMessage(body.error?.message || "创建失败");
      return;
    }
    form.reset();
    setCreateMessage(`已创建 ${body.item?.id} ${body.item?.name} → ${body.item?.status}${body.item?.review_reason ? ` (${body.item.review_reason})` : ""}`);
    await refresh();
  }

  return (
    <Card className="rounded-2xl border border-white/10 bg-white/[0.035] shadow-glow p-6">
      <CardTitle className="mb-3 text-xl font-medium">本渠道套餐</CardTitle>
      <p className="mb-3 text-sm text-slate-400">低价或高风险配额会进平台审核。这里只列本渠道 scope。</p>
      <Button variant="outline" onClick={refresh}>
        刷新套餐
      </Button>
      <form className="mt-4 grid max-w-xl gap-2" onSubmit={create}>
        <h3 className="text-lg font-medium">创建渠道套餐</h3>
        <Input name="name" aria-label="渠道套餐名" placeholder="渠道套餐名" />
        <Input name="price_minor" aria-label="渠道套餐价格" placeholder="价格 micro-USD，低于 1000000 会审核" defaultValue="1000" />
        <Input name="unit_type" aria-label="渠道套餐权益单位" placeholder="usd_credit" defaultValue="usd_credit" />
        <Input name="included_amount" aria-label="渠道套餐权益数量" placeholder="included_amount" defaultValue="1" />
        <Button size="sm" type="submit">
          创建渠道套餐
        </Button>
        <p className="text-sm text-slate-300">{createMessage}</p>
      </form>
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
