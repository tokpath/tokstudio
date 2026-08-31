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
      <p className="mt-3 text-sm text-ink">
        批发 {usage.usage_minor ?? 0} micro-USD · prompt {usage.prompt_tokens ?? 0} · completion {usage.completion_tokens ?? 0} · 视频{" "}
        {usage.video_seconds ?? 0} 秒 · 图片 {usage.image_count ?? 0}
      </p>
      <p className="mt-3 text-sm text-ink-secondary">{message}</p>
    </Card>
  );
}
