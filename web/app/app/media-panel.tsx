"use client";

import { zodResolver } from "@hookform/resolvers/zod";
import { useState } from "react";
import { useForm } from "react-hook-form";
import { z } from "zod";
import { TextField } from "@/components/text-field";
import { Button } from "@/components/ui/button";
import { Card, CardTitle } from "@/components/ui/card";
import { Form, FormControl, FormField, FormItem, FormLabel } from "@/components/ui/form";
import { apiBase } from "@/lib/api";

type Job = {
  id: string;
  kind?: string;
  task_type?: string;
  status: string;
  model: string;
  duration?: number;
  resolution?: string;
};

const schema = z.object({
  prompt: z.string().min(1),
  kind: z.enum(["video", "image"]),
  task_type: z.string().min(1),
  duration: z.coerce.number().int().min(1).max(60),
  resolution: z.string().min(1),
  aspect_ratio: z.string(),
  fps: z.coerce.number().int().min(0).max(60),
  generate_audio: z.boolean(),
  first_frame: z.string(),
  last_frame: z.string(),
  images: z.string(),
  reference_video: z.string(),
  reference_audio: z.string(),
  source_job_id: z.string(),
});

const videoModes = [
  { value: "t2v", label: "文生视频" },
  { value: "i2v", label: "图生视频" },
  { value: "first_frame", label: "首帧" },
  { value: "first_last_frame", label: "首尾帧" },
  { value: "reference", label: "参考素材" },
  { value: "extend", label: "延长" },
  { value: "edit", label: "编辑" },
];

const imageModes = [
  { value: "generate", label: "图像生成" },
  { value: "edit", label: "图像编辑" },
];

function splitRefs(raw: string) {
  return raw
    .split(/[\n,]/)
    .map((item) => item.trim())
    .filter(Boolean);
}

export default function MediaPanel() {
  const [items, setItems] = useState<Job[]>([]);
  const [kind, setKind] = useState("");
  const [message, setMessage] = useState("登录后可查看自己的视频和图像任务。");
  const form = useForm<z.infer<typeof schema>>({
    resolver: zodResolver(schema),
    defaultValues: {
      prompt: "a river at dusk",
      kind: "video",
      task_type: "t2v",
      duration: 5,
      resolution: "720p",
      aspect_ratio: "16:9",
      fps: 24,
      generate_audio: false,
      first_frame: "",
      last_frame: "",
      images: "",
      reference_video: "",
      reference_audio: "",
      source_job_id: "",
    },
  });
  const currentKind = form.watch("kind");
  const modes = currentKind === "image" ? imageModes : videoModes;

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
    const images = splitRefs(values.images);
    const payload: Record<string, unknown> = {
      prompt: values.prompt,
      task_type: values.task_type,
      duration: values.duration,
      resolution: values.resolution,
      aspect_ratio: values.aspect_ratio,
      fps: values.fps,
      generate_audio: values.generate_audio,
      first_frame: values.first_frame,
      last_frame: values.last_frame,
      images,
      reference_video: values.reference_video,
      reference_audio: values.reference_audio,
      source_job_id: values.source_job_id,
    };
    let path = "/v1/videos";
    if (values.kind === "image") {
      path = values.task_type === "edit" ? "/v1/images/edits" : "/v1/images/generations";
    } else if (values.task_type === "extend" && values.source_job_id) {
      path = `/v1/videos/${encodeURIComponent(values.source_job_id)}/extend`;
    }
    const response = await fetch(`${apiBase}${path}`, {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json", "Idempotency-Key": `console-${Date.now()}` },
      body: JSON.stringify(payload),
    });
    const body = await response.json();
    if (!response.ok) {
      setMessage(body.error?.message || "创建失败");
      return;
    }
    setMessage(`已创建 ${body.id || ""} · ${body.task_type || values.task_type}`);
    await refresh();
  }

  return (
    <Card>
      <CardTitle>媒体任务</CardTitle>
      <p className="mb-4 text-sm text-ink-secondary">
        支持文生、图生、首帧/首尾帧、参考素材和延长/编辑。结果只能通过签名 URL 下载，列表不含其他用户的任务。登录会话即可创建，不必再贴 API Key。
      </p>
      <Form {...form}>
        <form className="mb-4 space-y-3" onSubmit={form.handleSubmit(createJob)}>
          <TextField control={form.control} name="prompt" label="prompt" placeholder="prompt" showLabel={false} />
          <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
            <FormField
              control={form.control}
              name="kind"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>媒体类型</FormLabel>
                  <FormControl>
                    <select
                      className="h-10 w-full rounded-control border border-hairline bg-canvas px-3 text-sm"
                      aria-label="媒体类型"
                      {...field}
                      onChange={(event) => {
                        field.onChange(event);
                        form.setValue("task_type", event.target.value === "image" ? "generate" : "t2v");
                      }}
                    >
                      <option value="video">视频</option>
                      <option value="image">图像</option>
                    </select>
                  </FormControl>
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name="task_type"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>生成模式</FormLabel>
                  <FormControl>
                    <select className="h-10 w-full rounded-control border border-hairline bg-canvas px-3 text-sm" aria-label="生成模式" {...field}>
                      {modes.map((mode) => (
                        <option key={mode.value} value={mode.value}>
                          {mode.label}
                        </option>
                      ))}
                    </select>
                  </FormControl>
                </FormItem>
              )}
            />
            <TextField control={form.control} name="duration" label="时长" type="number" />
            <TextField control={form.control} name="resolution" label="分辨率" />
            <TextField control={form.control} name="aspect_ratio" label="宽高比" />
            <TextField control={form.control} name="fps" label="帧率" type="number" />
            <TextField control={form.control} name="first_frame" label="首帧" />
            <TextField control={form.control} name="last_frame" label="尾帧" />
            <TextField control={form.control} name="images" label="参考图" placeholder="逗号分隔 URL" />
            <TextField control={form.control} name="reference_video" label="参考视频" />
            <TextField control={form.control} name="reference_audio" label="参考音频" />
            <TextField control={form.control} name="source_job_id" label="源任务" />
            <FormField
              control={form.control}
              name="generate_audio"
              render={({ field }) => (
                <FormItem className="flex items-end gap-2 pb-2">
                  <FormControl>
                    <input
                      type="checkbox"
                      className="h-4 w-4 accent-sky-400"
                      aria-label="原生音频"
                      checked={field.value}
                      onChange={(event) => field.onChange(event.target.checked)}
                    />
                  </FormControl>
                  <FormLabel className="!mt-0">原生音频</FormLabel>
                </FormItem>
              )}
            />
          </div>
          <Button type="submit">创建视频任务</Button>
        </form>
      </Form>
      <div className="mb-4 flex flex-wrap gap-3">
        <select className="h-9 rounded-md border border-hairline bg-canvas px-3 text-sm" value={kind} onChange={(e) => setKind(e.target.value)}>
          <option value="">全部</option>
          <option value="video">视频</option>
          <option value="image">图像</option>
        </select>
        <Button variant="outline" onClick={refresh}>
          刷新任务
        </Button>
      </div>
      <ul className="space-y-2 text-sm text-ink">
        {items.map((item) => (
          <li key={item.id}>
            {item.kind || item.status} · {item.task_type || "t2v"} · {item.resolution || ""} · {item.duration || ""}s · {item.model} · {item.status} · {item.id}
          </li>
        ))}
      </ul>
      <p className="mt-3 text-sm text-ink-secondary">{message}</p>
    </Card>
  );
}
