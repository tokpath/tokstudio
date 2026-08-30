"use client";

import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Card, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { apiBase } from "@/lib/api";

type Promo = { id?: string; code?: string; status?: string; acquisition_role_id?: string };

export default function ChannelPromos() {
  const [items, setItems] = useState<Promo[]>([]);
  const [code, setCode] = useState("");
  const [message, setMessage] = useState("推广链接只属于本渠道。用户注册时服务端会固化归因。");

  async function refresh() {
    const response = await fetch(`${apiBase}/channel/promotion-codes`, { credentials: "include" });
    const body = await response.json();
    if (!response.ok) {
      setMessage(body.error?.message || "未登录渠道管理员");
      return;
    }
    const next = (body.items || []) as Promo[];
    setItems(next);
    setMessage(`本渠道推广码 ${next.length} 个`);
  }

  async function createPromo() {
    const response = await fetch(`${apiBase}/channel/promotion-codes`, {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json", "X-Tokenhub-Confirm": "1" },
      body: JSON.stringify({ code }),
    });
    const body = await response.json();
    if (!response.ok) {
      setMessage(body.error?.message || "创建失败");
      return;
    }
    setCode("");
    await refresh();
    setMessage(`已创建推广码 ${body.item?.code || ""}`);
  }

  const origin = typeof window === "undefined" ? "" : window.location.origin;

  return (
    <Card className="rounded-2xl border border-slate-800 bg-slate-900/70 p-6">
      <CardTitle className="mb-3 text-xl font-medium">推广链接</CardTitle>
      <p className="mb-3 text-sm text-slate-400">把推广码发给用户，或复制带 promo 参数的登录链接。</p>
      <div className="mb-3 flex flex-wrap gap-2">
        <Input className="max-w-xs" placeholder="新推广码 THB-SALE" value={code} onChange={(e) => setCode(e.target.value)} />
        <Button variant="outline" onClick={createPromo}>
          创建推广码
        </Button>
        <Button variant="outline" onClick={refresh}>
          刷新推广码
        </Button>
      </div>
      <ul className="mt-3 space-y-2 text-sm text-slate-200">
        {items.map((item) => (
          <li key={item.id}>
            {item.code} · {item.status} · {origin}/login?promo={item.code}
          </li>
        ))}
      </ul>
      <p className="mt-3 text-sm text-slate-300">{message}</p>
    </Card>
  );
}
