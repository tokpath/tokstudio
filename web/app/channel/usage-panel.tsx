"use client";

import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Card, CardTitle } from "@/components/ui/card";
import { apiBase } from "@/lib/api";

type Usage = {
  usage_minor?: number;
  prompt_tokens?: number;
  completion_tokens?: number;
  video_seconds?: number;
  image_count?: number;
};

export default function ChannelUsage() {
  const [usage, setUsage] = useState<Usage>({});
  const [message, setMessage] = useState("渠道用量只汇总本渠道已确认 usage，不含其他渠道或 prompt。");

  async function refresh() {
    const response = await fetch(`${apiBase}/channel/usage`, { credentials: "include" });
    const body = await response.json();
    if (!response.ok) {
      setMessage(body.error?.message || "未登录渠道管理员");
      return;
    }
    setUsage((body.usage || {}) as Usage);
    setMessage("已刷新本渠道用量");
  }

  return (
    <Card className="rounded-card border border-hairline bg-canvas-raised  p-6">
      <CardTitle className="mb-3 text-xl font-medium">本渠道用量</CardTitle>
      <p className="mb-3 text-sm text-ink-secondary">Token 和媒体秒数按账务 usage 汇总，不是估算。</p>
      <Button variant="outline" onClick={refresh}>
        刷新用量
      </Button>
      <section className="mt-4 grid gap-3 sm:grid-cols-2 lg:grid-cols-4" aria-label="本渠道用量">
        {[
          { t: "批发", v: String(usage.usage_minor ?? 0), d: "micro-USD" },
          { t: "Prompt", v: String(usage.prompt_tokens ?? 0), d: "tokens" },
          { t: "Completion", v: String(usage.completion_tokens ?? 0), d: "tokens" },
          { t: "媒体", v: `${usage.video_seconds ?? 0}s / ${usage.image_count ?? 0}`, d: "视频秒 · 图片张" },
        ].map((card) => (
          <div key={card.t} className="rounded-card border border-hairline bg-canvas p-4">
            <p className="th-eyebrow text-ink-mute">{card.t}</p>
            <p className="mt-2 font-mono text-[22px] font-medium leading-none tabular-nums tracking-tight">{card.v}</p>
            <p className="mt-2 text-sm text-ink-secondary">{card.d}</p>
          </div>
        ))}
      </section>
      <p className="mt-3 text-sm text-ink-secondary">{message}</p>
    </Card>
  );
}
