"use client";

import { zodResolver } from "@hookform/resolvers/zod";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useForm } from "react-hook-form";
import { useTranslations } from "next-intl";
import { z } from "zod";
import { ActionRow, LeadActions } from "@/components/console/action-row";
import { ListResourceView } from "@/components/console/list-resource-view";
import { TextField } from "@/components/text-field";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Form, FormControl, FormField, FormItem, FormLabel, FormMessage } from "@/components/ui/form";
import { StorageSourceBadge } from "@/components/storage-source-badge";
import { SubmitStatus } from "@/components/console/submit-status";
import { useListResource } from "@/hooks/use-list-resource";
import { apiBase } from "@/lib/api";
import {
  buildMediaPayload,
  completedJobsOfKind,
  defaultMediaForm,
  defaultTaskForKind,
  isMediaFailed,
  isMediaSuccess,
  isMediaTerminal,
  jobToFormValues,
  mediaContentPath,
  mediaCreatePath,
  mediaModeFields,
  mediaStatusPath,
  mergeMediaJobs,
  type MediaFormValues,
  type MediaJob,
} from "@/lib/media-job";
import { applyStorageFact, type StorageSource } from "@/lib/storage-source";
import { errorMessageFromBody, readResponseBody } from "@/lib/submit-result";

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

const POLL_MS = 2500;

function jobStatusTone(status: string): "success" | "warn" | "neutral" {
  if (isMediaSuccess(status)) {
    return "success";
  }
  if (isMediaFailed(status)) {
    return "warn";
  }
  return "neutral";
}

function jobLabel(job: MediaJob): string {
  const prompt = job.prompt?.trim();
  if (prompt) {
    return prompt.length > 48 ? `${prompt.slice(0, 48)}…` : prompt;
  }
  return job.id;
}

/** 媒体任务：默认先看列表；空态引导创建；顶栏筛选 / 刷新 / 新建（表单进 Dialog，对齐 Keys）。 */
export default function MediaPanel() {
  const t = useTranslations("user");
  const tc = useTranslations("common");
  const tCat = useTranslations("catalog");
  const [kind, setKind] = useState("");
  const [createOpen, setCreateOpen] = useState(false);
  const [advancedOpen, setAdvancedOpen] = useState(false);
  const [storage, setStorage] = useState<StorageSource | undefined>();
  const [message, setMessage] = useState(t("mediaHint"));
  const [createError, setCreateError] = useState("");
  const [creating, setCreating] = useState(false);
  const [overlays, setOverlays] = useState<Record<string, MediaJob>>({});
  const [previews, setPreviews] = useState<Record<string, string>>({});
  const [previewErrors, setPreviewErrors] = useState<Record<string, string>>({});
  const list = useListResource<MediaJob>({
    queryKey: kind,
    load: async () => {
      const query = kind ? `?kind=${encodeURIComponent(kind)}` : "";
      try {
        const response = await fetch(`${apiBase}/v1/me/media${query}`, { credentials: "include" });
        const body = await response.json().catch(() => ({}));
        const nextStorage = applyStorageFact(body, response.ok);
        if (nextStorage) {
          setStorage(nextStorage);
        }
        if (!response.ok) {
          return { ok: false, status: response.status, items: [], message: body.error?.message, code: body.error?.code };
        }
        return { ok: true, status: response.status, items: (body.items || []) as MediaJob[] };
      } catch {
        return { ok: false, network: true, items: [] };
      }
    },
  });
  const form = useForm<MediaFormValues>({
    resolver: zodResolver(schema),
    defaultValues: defaultMediaForm,
  });
  const currentKind = form.watch("kind");
  const currentTask = form.watch("task_type");
  const modeFields = mediaModeFields(currentKind, currentTask);
  const jobs = mergeMediaJobs(list.snapshot.items, overlays);
  const videoSources = useMemo(() => completedJobsOfKind(jobs, "video"), [jobs]);
  const imageSources = useMemo(() => completedJobsOfKind(jobs, "image"), [jobs]);
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
  const jobsRef = useRef(jobs);
  jobsRef.current = jobs;
  const completedIds = jobs
    .filter((item) => isMediaSuccess(item.status))
    .map((item) => item.id)
    .join(",");
  const previewInflight = useRef(new Set<string>());
  const liveIds = jobs
    .filter((item) => !isMediaTerminal(item.status))
    .map((item) => item.id)
    .join(",");

  function handleCreateOpenChange(open: boolean) {
    setCreateOpen(open);
    setCreateError("");
    if (!open) {
      form.reset(defaultMediaForm);
      setAdvancedOpen(false);
    }
  }

  function openWithJob(job: MediaJob) {
    form.reset(jobToFormValues(job));
    setAdvancedOpen(false);
    setCreateError("");
    setCreateOpen(true);
  }

  async function createJob(values: MediaFormValues) {
    const payload = buildMediaPayload(values);
    const path = mediaCreatePath(values);
    setCreating(true);
    setCreateError("");
    try {
      const response = await fetch(`${apiBase}${path}`, {
        method: "POST",
        credentials: "include",
        headers: { "Content-Type": "application/json", "Idempotency-Key": `console-${Date.now()}` },
        body: JSON.stringify(payload),
      });
      const body = await readResponseBody(response);
      const nextStorage = applyStorageFact(body, response.ok);
      if (nextStorage) {
        setStorage(nextStorage);
      }
      if (!response.ok) {
        setCreateError(errorMessageFromBody(body, tc("createFailed")));
        return;
      }
      const created = body as MediaJob;
      setMessage(t("createdJob", { id: created.id || "", type: created.task_type || values.task_type }));
      if (created.id) {
        setOverlays((current) => ({
          ...current,
          [created.id]: {
            ...created,
            prompt: created.prompt || values.prompt,
            kind: created.kind || values.kind,
            task_type: created.task_type || values.task_type,
          },
        }));
      }
      handleCreateOpenChange(false);
      await list.reload();
    } catch {
      setCreateError(tc("listNetwork"));
    } finally {
      setCreating(false);
    }
  }

  async function downloadJob(id: string, jobKind?: string) {
    const url = previews[id] || (await loadPreview(id, jobKind));
    if (!url) {
      return;
    }
    window.open(url, "_blank", "noopener,noreferrer");
  }

  const loadPreview = useCallback(
    async (id: string, jobKind?: string) => {
      try {
        const response = await fetch(`${apiBase}${mediaContentPath(jobKind, id)}`, { credentials: "include" });
        const body = await readResponseBody(response);
        const nextStorage = applyStorageFact(body, response.ok);
        if (nextStorage) {
          setStorage(nextStorage);
        }
        if (!response.ok) {
          const reason = errorMessageFromBody(body, "存储不可用");
          setPreviewErrors((current) => ({ ...current, [id]: reason }));
          setMessage(reason);
          return "";
        }
        const url = typeof (body as { url?: string }).url === "string" ? (body as { url: string }).url : "";
        if (!url) {
          setPreviewErrors((current) => ({ ...current, [id]: "存储不可用" }));
          setMessage("存储不可用");
          return "";
        }
        setPreviews((current) => ({ ...current, [id]: url }));
        setPreviewErrors((current) => {
          const next = { ...current };
          delete next[id];
          return next;
        });
        return url;
      } catch {
        setMessage(tc("listNetwork"));
        return "";
      }
    },
    [tc],
  );

  const applyAssetUrl = useCallback(
    async (job: MediaJob, field: "images" | "first_frame" | "last_frame" | "reference_video" | "reference_audio") => {
      const url = await loadPreview(job.id, job.kind);
      if (!url) {
        return;
      }
      if (field === "images") {
        const current = form.getValues("images");
        form.setValue("images", current.trim() ? `${current.trim()}\n${url}` : url);
        return;
      }
      form.setValue(field, url);
    },
    [form, loadPreview],
  );

  useEffect(() => {
    for (const job of jobsRef.current) {
      if (!isMediaSuccess(job.status) || previews[job.id] || previewErrors[job.id] || previewInflight.current.has(job.id)) {
        continue;
      }
      previewInflight.current.add(job.id);
      void loadPreview(job.id, job.kind).finally(() => {
        previewInflight.current.delete(job.id);
      });
    }
  }, [completedIds, loadPreview, previewErrors, previews]);

  const pollRef = useRef(false);
  useEffect(() => {
    if (!liveIds) {
      return;
    }
    let cancelled = false;
    async function tick() {
      if (pollRef.current || cancelled) {
        return;
      }
      pollRef.current = true;
      try {
        const active = jobsRef.current.filter((item) => !isMediaTerminal(item.status));
        for (const job of active) {
          const response = await fetch(`${apiBase}${mediaStatusPath(job.kind, job.id)}`, { credentials: "include" });
          const body = await readResponseBody(response);
          if (!response.ok || !body || typeof body !== "object") {
            continue;
          }
          const next = body as MediaJob;
          if (!next.id) {
            continue;
          }
          setOverlays((current) => ({ ...current, [next.id]: { ...job, ...next } }));
        }
      } finally {
        pollRef.current = false;
      }
    }
    const timer = window.setInterval(() => {
      void tick();
    }, POLL_MS);
    void tick();
    return () => {
      cancelled = true;
      window.clearInterval(timer);
    };
  }, [liveIds]);

  function statusLabel(status: string): string {
    if (status === "queued") {
      return t("mediaQueued");
    }
    if (status === "in_progress") {
      return t("mediaInProgress");
    }
    if (status === "completed" || status === "succeeded") {
      return t("mediaCompleted");
    }
    if (status === "failed") {
      return t("mediaFailedStatus");
    }
    if (status === "cancelled") {
      return t("mediaCancelled");
    }
    if (status === "expired") {
      return t("mediaExpired");
    }
    return status;
  }

  function modeLabel(job: MediaJob): string {
    const task = job.task_type || defaultTaskForKind(job.kind === "image" ? "image" : "video");
    const found = (job.kind === "image" ? imageModes : videoModes).find((mode) => mode.value === task);
    return found?.label || task;
  }

  return (
    <Card>
      <LeadActions
        lead={
          <div className="flex flex-wrap items-center gap-3">
            <p className="text-sm text-ink-secondary">{t("mediaLead")}</p>
            <StorageSourceBadge storage={storage} />
          </div>
        }
        actions={
          <>
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
            <Button type="button" variant="outline" onClick={() => void list.reload()}>
              {t("refreshJobs")}
            </Button>
            <Button type="button" onClick={() => setCreateOpen(true)}>
              {t("createJob")}
            </Button>
          </>
        }
      />

      <ListResourceView
        snapshot={list.snapshot}
        emptyTitle={t("mediaEmpty")}
        emptyDetail={t("mediaEmptyDetail")}
        emptyAction={
          <Button type="button" onClick={() => setCreateOpen(true)}>
            {t("createJob")}
          </Button>
        }
        onRetry={() => void list.reload()}
      >
        <ul className="space-y-2">
          {jobs.map((item) => {
            const preview = previews[item.id];
            const failed = isMediaFailed(item.status);
            return (
              <li
                key={item.id}
                className="rounded-control border border-hairline bg-canvas px-3 py-2.5 text-sm text-ink"
                data-testid={`media-job-${item.id}`}
                data-status={item.status}
              >
                <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
                  <Badge tone={jobStatusTone(item.status)}>{statusLabel(item.status)}</Badge>
                  <span className="text-ink-secondary">{item.kind === "image" ? tCat("image") : tCat("video")}</span>
                  <span>{modeLabel(item)}</span>
                  {item.resolution ? <span className="text-ink-mute">{item.resolution}</span> : null}
                  {item.duration ? <span className="text-ink-mute">{item.duration}s</span> : null}
                  {!isMediaTerminal(item.status) ? (
                    <span className="text-ink-mute">{t("mediaAutoUpdate")}</span>
                  ) : null}
                </div>
                {item.prompt ? <p className="mt-2 text-sm text-ink">{item.prompt}</p> : null}
                {preview ? (
                  item.kind === "image" ? (
                    // eslint-disable-next-line @next/next/no-img-element
                    <img src={preview} alt="" className="mt-2 max-h-40 rounded-control border border-hairline object-contain" />
                  ) : (
                    <video src={preview} controls className="mt-2 max-h-40 w-full rounded-control border border-hairline" />
                  )
                ) : null}
                {failed ? (
                  <p className="mt-2 text-sm text-hold">
                    {item.error || previewErrors[item.id] || t("mediaFailedHint")}
                  </p>
                ) : null}
                <div className="mt-2 flex flex-wrap items-center gap-2">
                  {isMediaSuccess(item.status) ? (
                    <>
                      <StorageSourceBadge storage={storage} />
                      <Button type="button" size="sm" variant="outline" onClick={() => void downloadJob(item.id, item.kind)}>
                        {t("mediaDownload")}
                      </Button>
                      <Button type="button" size="sm" variant="outline" onClick={() => openWithJob(item)}>
                        {t("mediaReuse")}
                      </Button>
                    </>
                  ) : null}
                  {failed ? (
                    <Button type="button" size="sm" onClick={() => openWithJob(item)}>
                      {t("mediaRetry")}
                    </Button>
                  ) : null}
                </div>
              </li>
            );
          })}
        </ul>
      </ListResourceView>
      <p className="mt-3 text-sm text-ink-secondary">{message}</p>

      <Dialog open={createOpen} onOpenChange={handleCreateOpenChange}>
        <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>{t("createJob")}</DialogTitle>
            <DialogDescription>{t("createJobLead")}</DialogDescription>
          </DialogHeader>
          <Form {...form}>
            <form className="grid gap-3" onSubmit={form.handleSubmit(createJob, () => setAdvancedOpen(true))}>
              <FormField
                control={form.control}
                name="kind"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t("kind")}</FormLabel>
                    <FormControl>
                      <div className="grid grid-cols-2 gap-2" role="radiogroup" aria-label={t("kind")}>
                        <Button
                          type="button"
                          variant={field.value === "image" ? "default" : "outline"}
                          aria-pressed={field.value === "image"}
                          onClick={() => {
                            field.onChange("image");
                            form.setValue("task_type", defaultTaskForKind("image"));
                          }}
                        >
                          {t("mediaGenImage")}
                        </Button>
                        <Button
                          type="button"
                          variant={field.value === "video" ? "default" : "outline"}
                          aria-pressed={field.value === "video"}
                          onClick={() => {
                            field.onChange("video");
                            form.setValue("task_type", defaultTaskForKind("video"));
                          }}
                        >
                          {t("mediaGenVideo")}
                        </Button>
                      </div>
                    </FormControl>
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name="prompt"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t("mediaPrompt")}</FormLabel>
                    <FormControl>
                      <textarea
                        className="min-h-24 w-full rounded-control border border-hairline bg-canvas-raised px-3 py-2 text-sm leading-normal text-ink placeholder:text-ink-mute focus:border-brand-emphasis"
                        placeholder={t("mediaPromptPh")}
                        aria-label={t("mediaPrompt")}
                        {...field}
                      />
                    </FormControl>
                    <FormMessage />
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
                      <select
                        className="h-10 w-full rounded-control border border-hairline bg-canvas px-3 text-sm"
                        aria-label={t("mode")}
                        {...field}
                      >
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
              {modeFields.duration ? <TextField control={form.control} name="duration" label={t("duration")} type="number" /> : null}
              {modeFields.sourceJob ? (
                <FormField
                  control={form.control}
                  name="source_job_id"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t("mediaPickSource")}</FormLabel>
                      <FormControl>
                        <select
                          className="h-10 w-full rounded-control border border-hairline bg-canvas px-3 text-sm"
                          aria-label={t("mediaPickSource")}
                          value={field.value}
                          onChange={(event) => field.onChange(event.target.value)}
                        >
                          <option value="">{t("mediaPickSourcePh")}</option>
                          {videoSources.map((job) => (
                            <option key={job.id} value={job.id}>
                              {jobLabel(job)}
                            </option>
                          ))}
                        </select>
                      </FormControl>
                    </FormItem>
                  )}
                />
              ) : null}
              {modeFields.images ? (
                <AssetField
                  control={form.control}
                  name="images"
                  label={t("images")}
                  placeholder={t("imagesPh")}
                  jobs={imageSources}
                  pickLabel={t("mediaPickCompleted")}
                  emptyLabel={t("mediaPickNone")}
                  onPick={(job) => void applyAssetUrl(job, "images")}
                />
              ) : null}
              {modeFields.firstFrame ? (
                <AssetField
                  control={form.control}
                  name="first_frame"
                  label={t("firstFrame")}
                  jobs={imageSources}
                  pickLabel={t("mediaPickCompleted")}
                  emptyLabel={t("mediaPickNone")}
                  onPick={(job) => void applyAssetUrl(job, "first_frame")}
                />
              ) : null}
              {modeFields.lastFrame ? (
                <AssetField
                  control={form.control}
                  name="last_frame"
                  label={t("lastFrame")}
                  jobs={imageSources}
                  pickLabel={t("mediaPickCompleted")}
                  emptyLabel={t("mediaPickNone")}
                  onPick={(job) => void applyAssetUrl(job, "last_frame")}
                />
              ) : null}
              {modeFields.referenceVideo ? (
                <AssetField
                  control={form.control}
                  name="reference_video"
                  label={t("refVideo")}
                  jobs={videoSources}
                  pickLabel={t("mediaPickCompleted")}
                  emptyLabel={t("mediaPickNone")}
                  onPick={(job) => void applyAssetUrl(job, "reference_video")}
                />
              ) : null}
              {modeFields.referenceAudio ? <TextField control={form.control} name="reference_audio" label={t("refAudio")} /> : null}
              {modeFields.images || modeFields.firstFrame || modeFields.referenceVideo ? (
                <p className="text-xs text-ink-mute">{t("mediaNoUpload")}</p>
              ) : null}
              <button
                type="button"
                className="justify-self-start text-sm text-brand underline-offset-2 hover:underline"
                aria-expanded={advancedOpen}
                onClick={() => setAdvancedOpen((current) => !current)}
              >
                {t("advanced")}
              </button>
              {advancedOpen ? (
                <div className="grid gap-3 rounded-control border border-hairline bg-canvas p-3 md:grid-cols-2">
                  <TextField control={form.control} name="resolution" label={t("resolution")} />
                  <TextField control={form.control} name="aspect_ratio" label={t("aspect")} />
                  {currentKind === "video" ? (
                    <>
                      <TextField control={form.control} name="fps" label={t("fps")} type="number" />
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
                    </>
                  ) : null}
                </div>
              ) : null}
              <SubmitStatus error={createError} />
              <DialogFooter>
                <ActionRow className="gap-2">
                  <Button type="button" variant="outline" onClick={() => handleCreateOpenChange(false)}>
                    {tc("cancel")}
                  </Button>
                  <Button type="submit" disabled={creating}>
                    {creating ? tc("submitting") : t("createJob")}
                  </Button>
                </ActionRow>
              </DialogFooter>
            </form>
          </Form>
        </DialogContent>
      </Dialog>
    </Card>
  );
}

function AssetField({
  control,
  name,
  label,
  placeholder,
  jobs,
  pickLabel,
  emptyLabel,
  onPick,
}: {
  control: ReturnType<typeof useForm<MediaFormValues>>["control"];
  name: "images" | "first_frame" | "last_frame" | "reference_video" | "reference_audio";
  label: string;
  placeholder?: string;
  jobs: MediaJob[];
  pickLabel: string;
  emptyLabel: string;
  onPick: (job: MediaJob) => void;
}) {
  return (
    <div className="grid gap-2">
      <TextField control={control} name={name} label={label} placeholder={placeholder} />
      <select
        className="h-10 w-full rounded-control border border-hairline bg-canvas px-3 text-sm"
        aria-label={pickLabel}
        defaultValue=""
        onChange={(event) => {
          const job = jobs.find((item) => item.id === event.target.value);
          event.target.value = "";
          if (job) {
            onPick(job);
          }
        }}
      >
        <option value="">{jobs.length ? pickLabel : emptyLabel}</option>
        {jobs.map((job) => (
          <option key={job.id} value={job.id}>
            {jobLabel(job)}
          </option>
        ))}
      </select>
    </div>
  );
}
