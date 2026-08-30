"use client";

import { zodResolver } from "@hookform/resolvers/zod";
import { useState } from "react";
import { useForm } from "react-hook-form";
import { z } from "zod";
import { Button } from "@/components/ui/button";
import { Card, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { apiBase } from "@/lib/api";

type Job = { id: string; kind?: string; status: string; model: string };

const schema = z.object({
  prompt: z.string().min(1),
  kind: z.enum(["video", "image"]),
});

export default function MediaPanel() {
  const [items, setItems] = useState<Job[]>([]);
  const [kind, setKind] = useState("");
  const [message, setMessage] = useState("登录后可查看自己的视频和图像任务。");
  const form = useForm({ resolver: zodResolver(schema), defaultValues: { prompt: "a river at dusk", kind: "video" as const } });

  async function refresh() {
    const query = kind ? `?kind=${encodeURIComponent(kind)}` : "";
    const response = await fetch(`${apiBase}/v1/me/media${query}`, { credentials: "include" });
    const body = await response.json();
    if (!response.ok) {
      setMessage(body.error?.message || "未登录");
      return;
    }
    setItems(body.items || []);
    setMessage("媒体任务已刷新");
  }

  async function createJob(values: z.infer<typeof schema>) {
    const path = values.kind === "image" ? "/v1/images/generations" : "/v1/videos";
    const response = await fetch(`${apiBase}${path}`, {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json", "Idempotency-Key": `console-${Date.now()}` },
      body: JSON.stringify({ prompt: values.prompt }),
    });
    const body = await response.json();
    if (!response.ok) {
      setMessage(body.error?.message || "创建失败");
      return;
    }
    setMessage(`已创建 ${body.id || ""}`);
    await refresh();
  }

  return (
    <Card>
      <CardTitle>媒体任务</CardTitle>
      <p className="mb-4 text-sm text-slate-400">结果只能通过签名 URL 下载，列表不含其他用户的任务。登录会话即可创建，不必再贴 API Key。</p>
      <form className="mb-4 grid gap-3 md:grid-cols-[1fr_auto_auto]" onSubmit={form.handleSubmit(createJob)}>
        <Input placeholder="prompt" {...form.register("prompt")} />
        <select className="h-9 rounded-md border border-slate-700 bg-slate-950 px-3 text-sm" {...form.register("kind")}>
          <option value="video">视频</option>
          <option value="image">图像</option>
        </select>
        <Button type="submit">创建视频任务</Button>
      </form>
      <div className="mb-4 flex flex-wrap gap-3">
        <select className="h-9 rounded-md border border-slate-700 bg-slate-950 px-3 text-sm" value={kind} onChange={(e) => setKind(e.target.value)}>
          <option value="">全部</option>
          <option value="video">视频</option>
          <option value="image">图像</option>
        </select>
        <Button variant="outline" onClick={refresh}>
          刷新任务
        </Button>
      </div>
      <ul className="space-y-2 text-sm text-slate-200">
        {items.map((item) => (
          <li key={item.id}>
            {item.kind || item.status} · {item.model} · {item.status} · {item.id}
          </li>
        ))}
      </ul>
      <p className="mt-3 text-sm text-slate-300">{message}</p>
    </Card>
  );
}
