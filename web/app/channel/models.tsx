"use client";

import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Card, CardTitle } from "@/components/ui/card";
import { apiBase } from "@/lib/api";

type ChannelModel = { public_id?: string; display_name?: string; vendor?: string; status?: string; enabled?: boolean };

export default function ChannelModels() {
  const [items, setItems] = useState<ChannelModel[]>([]);
  const [message, setMessage] = useState("本渠道只能使用平台已授权的模型，不能自己添加提供商和模型。");

  async function refresh() {
    const response = await fetch(`${apiBase}/channel/models`, { credentials: "include" });
    const body = await response.json();
    if (!response.ok) {
      setMessage(body.error?.message || "未登录渠道管理员");
      return;
    }
    const next = (body.items || []) as ChannelModel[];
    setItems(next);
    setMessage(`平台已授权 ${next.filter((item) => item.enabled).length} 个模型`);
  }

  return (
    <Card className="rounded-stamp border border-hairline bg-canvas-raised p-6">
      <CardTitle className="mb-3 text-xl font-medium">本渠道模型</CardTitle>
      <p className="mb-3 text-sm text-ink-secondary">
        所有租户的模型资源都只能从平台目录出发。渠道不能自建提供商或模型，也不能引入目录外的模型。
      </p>
      <Button variant="outline" onClick={refresh}>
        刷新模型
      </Button>
      <ul className="mt-4 grid gap-2 text-sm">
        {items.map((item) => (
          <li key={item.public_id} className="flex flex-wrap justify-between gap-2 border-b border-hairline py-2">
            <span>
              {item.display_name || item.public_id} <span className="font-mono text-ink-secondary">{item.public_id}</span>
            </span>
            <span className="text-ink-secondary">
              {item.vendor} · {item.status} · {item.enabled ? "已授权" : "未授权"}
            </span>
          </li>
        ))}
      </ul>
      <p className="mt-3 text-sm text-ink-secondary">{message}</p>
    </Card>
  );
}
