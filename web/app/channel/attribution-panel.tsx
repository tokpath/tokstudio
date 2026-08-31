"use client";

import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Card, CardTitle } from "@/components/ui/card";
import { apiBase } from "@/lib/api";

type Bucket = {
  source_code?: string;
  role_type?: string;
  user_count?: number;
};

export default function ChannelAttribution() {
  const [items, setItems] = useState<Bucket[]>([]);
  const [message, setMessage] = useState("归因在注册时写死。这里只汇总本渠道推广码，不含其他渠道。");

  async function refresh() {
    const response = await fetch(`${apiBase}/channel/attribution`, { credentials: "include" });
    const body = await response.json();
    if (!response.ok) {
      setMessage(body.error?.message || "未登录渠道管理员");
      return;
    }
    const next = (body.items || []) as Bucket[];
    setItems(next);
    setMessage(`本渠道归因 ${next.length} 组`);
  }

  return (
    <Card className="rounded-card border border-hairline bg-canvas-raised  p-6">
      <CardTitle className="mb-3 text-xl font-medium">本渠道归因</CardTitle>
      <p className="mb-3 text-sm text-ink-secondary">按推广码和代理层级点数。渠道不能改别人的归属。</p>
      <Button variant="outline" onClick={refresh}>
        刷新归因
      </Button>
      <ul className="mt-3 space-y-2 text-sm text-ink">
        {items.map((item) => (
          <li key={`${item.source_code}-${item.role_type}`}>
            {item.source_code || "—"} · {item.role_type || "无层级"} · {item.user_count ?? 0} 人
          </li>
        ))}
      </ul>
      <p className="mt-3 text-sm text-ink-secondary">{message}</p>
    </Card>
  );
}
