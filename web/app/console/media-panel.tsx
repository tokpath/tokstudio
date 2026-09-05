"use client";

import { zodResolver } from "@hookform/resolvers/zod";
import { useEffect, useMemo, useState } from "react";
import { useForm } from "react-hook-form";
import { useTranslations } from "next-intl";
import { z } from "zod";
import { EmptyLedger } from "@/components/console/empty-ledger";
import { TextField } from "@/components/text-field";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
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

function splitRefs(raw: string) {
  return raw
    .split(/[\n,]/)
    .map((item) => item.trim())
    .filter(Boolean);
}

function jobStatusTone(status: string): "success" | "warn" | "neutral" {
  if (status === "completed" || status === "succeeded") {
    return "success";
  }
  if (status === "failed" || status === "cancelled" || status === "expired") {
    return "warn";
  }
  return "neutral";
}

export default function MediaPanel() {
  const t = useTranslations("user");
  const tc = useTranslations("common");
  const tCat = useTranslations("catalog");
  const [items, setItems] = useState<Job[]>([]);
  const [kind, setKind] = useState("");
  const [loaded, setLoaded] = useState(false);
  const [message, setMessage] = useState(t("mediaHint"));
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
  const videoModes = useMemo(
    () => [
      { value: "t2v", label: t("t2v") },
      { value: "i2v", label: t("i2v") },
      { value: "first_frame", label: t("firstFrame") },
      { value: "first_last_frame", label: t("firstLast") },
      { value: "reference", label: t("reference") },
      { value: "extend", label: t("extend") },
      { value: "edit", label: t("edit") },
    ],
    [t],
  );
  const imageModes = useMemo(
    () => [
      { value: "generate", label: t("imgGen") },
      { value: "edit", label: t("imgEdit") },
    ],
    [t],
  );
  const modes = currentKind === "image" ? imageModes : videoModes;

  async function refresh(nextKind = kind) {
    const query = nextKind ? `?kind=${encodeURIComponent(nextKind)}` : "";
    const response = await fetch(`${apiBase}/v1/me/media${query}`, { credentials: "include" });
    const body = await response.json();
    if (!response.ok) {
      setLoaded(true);
      setMessage(body.error?.message || tc("notLoggedIn"));
      return;
    }
    setItems(body.items || []);
    setLoaded(true);
    setMessage(t("mediaRefreshed"));
  }

  useEffect(() => {
    void refresh(kind);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [kind]);

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
      setMessage(body.error?.message || tc("createFailed"));
      return;
    }
    setMessage(t("createdJob", { id: body.id || "", type: body.task_type || values.task_type }));
    await refresh();
  }

  return (
    <Card>
      <p className="mb-4 text-sm text-ink-secondary">{t("mediaLead")}</p>
      <Form {...form}>
        <form className="mb-4 space-y-3" onSubmit={form.handleSubmit(createJob)}>
          <TextField control={form.control} name="prompt" label="prompt" placeholder="prompt" showLabel={false} />
          <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
            <FormField
              control={form.control}
              name="kind"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t("kind")}</FormLabel>
                  <FormControl>
                    <select
                      className="h-10 w-full rounded-control border border-hairline bg-canvas px-3 text-sm"
                      {...field}
                      onChange={(event) => {
                        field.onChange(event);
                        form.setValue("task_type", event.target.value === "image" ? "generate" : "t2v");
                      }}
                    >
                      <option value="video">{tCat("video")}</option>
                      <option value="image">{tCat("image")}</option>
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
                  <FormLabel>{t("mode")}</FormLabel>
                  <FormControl>
                    <select className="h-10 w-full rounded-control border border-hairline bg-canvas px-3 text-sm" aria-label={t("mode")} {...field}>
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
            <TextField control={form.control} name="duration" label={t("duration")} type="number" />
            <TextField control={form.control} name="resolution" label={t("resolution")} />
            <TextField control={form.control} name="aspect_ratio" label={t("aspect")} />
            <TextField control={form.control} name="fps" label={t("fps")} type="number" />
            <TextField control={form.control} name="first_frame" label={t("firstFrame")} />
            <TextField control={form.control} name="last_frame" label={t("lastFrame")} />
            <TextField control={form.control} name="images" label={t("images")} placeholder={t("imagesPh")} />
            <TextField control={form.control} name="reference_video" label={t("refVideo")} />
            <TextField control={form.control} name="reference_audio" label={t("refAudio")} />
            <TextField control={form.control} name="source_job_id" label={t("sourceJob")} />
            <FormField
              control={form.control}
              name="generate_audio"
              render={({ field }) => (
                <FormItem className="flex items-end gap-2 pb-2">
                  <FormControl>
                    <input
                      type="checkbox"
                      className="h-4 w-4 accent-brand"
                      aria-label={t("nativeAudio")}
                      checked={field.value}
                      onChange={(event) => field.onChange(event.target.checked)}
                    />
                  </FormControl>
                  <FormLabel className="!mt-0">{t("nativeAudio")}</FormLabel>
                </FormItem>
              )}
            />
          </div>
          <Button type="submit">{t("createVideo")}</Button>
        </form>
      </Form>
      <div className="mb-4 flex flex-wrap gap-3">
        <select
          className="h-9 rounded-md border border-hairline bg-canvas px-3 text-sm"
          value={kind}
          onChange={(e) => setKind(e.target.value)}
          aria-label={t("mediaFilterKind")}
        >
          <option value="">{tc("all")}</option>
          <option value="video">{tCat("video")}</option>
          <option value="image">{tCat("image")}</option>
        </select>
        <Button variant="outline" onClick={() => void refresh()}>
          {t("refreshJobs")}
        </Button>
      </div>
      {loaded && items.length === 0 ? (
        <EmptyLedger title={t("mediaEmpty")} detail={t("mediaEmptyDetail")} />
      ) : (
        <ul className="space-y-2">
          {items.map((item) => (
            <li
              key={item.id}
              className="flex flex-wrap items-center gap-x-3 gap-y-2 rounded-control border border-hairline bg-canvas px-3 py-2.5 text-sm text-ink"
            >
              <Badge tone={jobStatusTone(item.status)}>{item.status}</Badge>
              <span className="text-ink-secondary">{item.kind || "—"}</span>
              <span>{item.task_type || "t2v"}</span>
              {item.resolution ? <span className="text-ink-mute">{item.resolution}</span> : null}
              {item.duration ? <span className="text-ink-mute">{item.duration}s</span> : null}
              <span className="font-mono text-xs text-ink-secondary">{item.model}</span>
              <span className="font-mono text-xs text-ink-mute">{item.id}</span>
            </li>
          ))}
        </ul>
      )}
      <p className="mt-3 text-sm text-ink-secondary">{message}</p>
    </Card>
  );
}
