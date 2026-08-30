"use client";

import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Card, CardTitle } from "@/components/ui/card";
import { apiBase } from "@/lib/api";

type ChannelUser = { id?: string; email?: string; status?: string; source_code?: string };

export default function ChannelUsers() {
  const [items, setItems] = useState<ChannelUser[]>([]);
  const [message, setMessage] = useState("渠道管理员只能看到本渠道用户，邮箱可能已脱敏。");

  async function refresh() {
    const response = await fetch(`${apiBase}/channel/users`, { credentials: "include" });
    const body = await response.json();
    if (!response.ok) {
      setMessage(body.error?.message || "未登录渠道管理员");
      return;
    }
    const next = (body.items || []) as ChannelUser[];
    setItems(next);
    setMessage(`本渠道用户 ${next.length} 人`);
  }

  return (
    <Card className="rounded-2xl border border-white/10 bg-white/[0.035] shadow-glow p-6">
      <CardTitle className="mb-3 text-xl font-medium">本渠道用户</CardTitle>
      <p className="mb-3 text-sm text-slate-400">归因在注册时写死。这里只列本渠道 scope，不含其他渠道或平台成本。</p>
      <Button variant="outline" onClick={refresh}>
        刷新用户
      </Button>
      <ul className="mt-3 space-y-2 text-sm text-slate-200">
        {items.map((item) => (
          <li key={item.id}>
            {item.email} · {item.status} · {item.source_code || "—"}
          </li>
        ))}
      </ul>
      <p className="mt-3 text-sm text-slate-300">{message}</p>
    </Card>
  );
}
