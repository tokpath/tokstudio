"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useForm, type Resolver } from "react-hook-form";
import { useTranslations } from "next-intl";
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
  applyMediaMode,
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
  mediaFormIssues,
  mediaIssueNeedsAdvanced,
  mediaListPath,
  mediaModeFields,
  mediaStatusPath,
  mergeMediaJobs,
  parseSignedMedia,
  readMediaListPage,
  signedMediaUsable,
  type MediaFormValues,
  type MediaJob,
  type MediaKind,
  type SignedMediaUrl,
} from "@/lib/media-job";
import { applyStorageFact, type StorageSource } from "@/lib/storage-source";
import { errorMessageFromBody, readResponseBody } from "@/lib/submit-result";
import { EmptyLedger } from "@/components/console/empty-ledger";
import Link from "next/link";
import { publicModelsPath } from "@/lib/catalog";
import { catalogModelUsable } from "@/lib/model-use";

const POLL_MS = 2500;

function mediaIssueMessage(
  path: keyof MediaFormValues,
  t: (key: string) => string,
): string {
  if (path === "prompt") {
    return t("mediaNeedPrompt");
  }
  if (path === "model") {
    return t("mediaNeedModel");
  }
  if (path === "duration") {
    return t("mediaNeedDuration");
  }
  if (path === "source_job_id") {
    return t("mediaNeedSource");
  }
  if (path === "images") {
    return t("mediaNeedImages");
  }
  if (path === "fps") {
    return t("mediaNeedFps");
  }
  if (path === "resolution") {
    return t("mediaNeedResolution");
  }
  return t("mediaNeedAsset");
}

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

function useCompletedMediaAssets(kind: MediaKind, enabled: boolean) {
  const [items, setItems] = useState<MediaJob[]>([]);
  const [cursor, setCursor] = useState("");
  const [loading, setLoading] = useState(false);

  const loadPage = useCallback(
    async (from = "") => {
      setLoading(true);
      try {
        const response = await fetch(`${apiBase}${mediaListPath({ kind, status: "completed", cursor: from || undefined })}`, {
          credentials: "include",
        });
        const body = await response.json().catch(() => ({}));
        if (!response.ok) {
          return;
        }
        const page = readMediaListPage(body);
        setItems((current) => (from ? [...current, ...page.items] : page.items));
        setCursor(page.nextCursor);
      } catch {
        return;
      } finally {
        setLoading(false);
      }
    },
    [kind],
  );

  useEffect(() => {
    if (!enabled) {
      setItems([]);
      setCursor("");
      return;
    }
    void loadPage("");
  }, [enabled, loadPage]);

  return {
    items: completedJobsOfKind(items, kind),
    hasMore: Boolean(cursor),
    loading,
    loadMore: () => {
      if (cursor && !loading) {
        void loadPage(cursor);
      }
    },
  };
}

/** 媒体任务：默认先看列表；空态引导创建；顶栏筛选 / 刷新 / 新建（表单进 Dialog，对齐 Keys）。 */
export default function MediaPanel({
  initialKind,
  initialModel,
  catalogHref = "/app/catalog",
  catalogOk = true,
  catalogMessage,
  modelError,
}: {
  initialKind?: string;
  initialModel?: string;
  catalogHref?: string;
  catalogOk?: boolean;
  catalogMessage?: string;
  modelError?: "missing" | "unavailable";
} = {}) {
  const t = useTranslations("user");
  const tc = useTranslations("common");
  const tCat = useTranslations("catalog");
  const seededKind: MediaFormValues["kind"] = initialKind === "video" ? "video" : "image";
  const entryBlocked = Boolean(initialModel) && (!catalogOk || Boolean(modelError));
  const [kind, setKind] = useState(initialKind === "video" || initialKind === "image" ? initialKind : "");
  const [createOpen, setCreateOpen] = useState(false);
  const [advancedOpen, setAdvancedOpen] = useState(false);
  const [storage, setStorage] = useState<StorageSource | undefined>();
  const [message, setMessage] = useState(t("mediaHint"));
  const [createError, setCreateError] = useState("");
  const [creating, setCreating] = useState(false);
  const [overlays, setOverlays] = useState<Record<string, MediaJob>>({});
  const [moreItems, setMoreItems] = useState<MediaJob[]>([]);
  const [listCursor, setListCursor] = useState("");
  const [previews, setPreviews] = useState<Record<string, SignedMediaUrl>>({});
  const [previewErrors, setPreviewErrors] = useState<Record<string, string>>({});
  const [pollFailed, setPollFailed] = useState(false);
  const list = useListResource<MediaJob>({
    queryKey: kind,
    load: async () => {
      try {
        const response = await fetch(`${apiBase}${mediaListPath({ kind: kind || undefined })}`, { credentials: "include" });
        const body = await response.json().catch(() => ({}));
        const page = readMediaListPage(body);
        const extras = { storage: applyStorageFact(body, response.ok), next_cursor: page.nextCursor };
        if (!response.ok) {
          return { ok: false, status: response.status, items: [], message: body.error?.message, code: body.error?.code, extras };
        }
        return { ok: true, status: response.status, items: page.items, extras };
      } catch {
        return { ok: false, network: true, items: [] };
      }
    },
    onAccepted: (result) => {
      const extras = result.extras as { storage?: StorageSource; next_cursor?: string } | undefined;
      if (extras?.storage) {
        setStorage(extras.storage);
      }
      setListCursor(result.ok ? extras?.next_cursor || "" : "");
      setMoreItems([]);
    },
  });
  function seedFormValues(): MediaFormValues {
    return {
      ...defaultMediaForm,
      kind: seededKind,
      task_type: defaultTaskForKind(seededKind),
      model: initialModel || "",
    };
  }
  const formResolver = useMemo<Resolver<MediaFormValues>>(
    () => (values) => {
      const issues = mediaFormIssues(values);
      if (issues.length === 0) {
        return { values, errors: {} };
      }
      return {
        values: {},
        errors: Object.fromEntries(
          issues.map((issue) => [
            issue.path,
            { type: issue.code, message: mediaIssueMessage(issue.path, t) },
          ]),
        ),
      };
    },
    [t],
  );
  const form = useForm<MediaFormValues>({
    resolver: formResolver,
    defaultValues: seedFormValues(),
  });
  const currentKind = form.watch("kind");
  const currentTask = form.watch("task_type");
  const modeFields = mediaModeFields(currentKind, currentTask);
  const jobs = mergeMediaJobs([...list.snapshot.items, ...moreItems], overlays);
  const imageAssets = useCompletedMediaAssets("image", createOpen);
  const videoAssets = useCompletedMediaAssets("video", createOpen);
  const videoSources = videoAssets.items;
  const imageSources = imageAssets.items;
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
  const previewsRef = useRef(previews);
  previewsRef.current = previews;
  const completedIds = jobs
    .filter((item) => isMediaSuccess(item.status))
    .map((item) => item.id)
    .join(",");
  const previewInflight = useRef(new Set<string>());
  const liveIds = jobs
    .filter((item) => !isMediaTerminal(item.status))
    .map((item) => item.id)
    .join(",");

  useEffect(() => {
    if (initialModel && !entryBlocked) {
      form.reset(seedFormValues());
      setCreateOpen(true);
    }
  }, [initialModel, entryBlocked]);

  function handleCreateOpenChange(open: boolean) {
    if (open && entryBlocked) {
      return;
    }
    setCreateOpen(open);
    setCreateError("");
    if (!open) {
      form.reset(seedFormValues());
      setAdvancedOpen(false);
    }
  }

  async function openWithJob(job: MediaJob) {
    const values = jobToFormValues(job);
    form.reset(values);
    setAdvancedOpen(false);
    setCreateError("");
    setCreateOpen(true);
    if (!values.model) {
      form.setError("model", { type: "required", message: t("mediaNeedModel") });
      return;
    }
    try {
      const response = await fetch(`${apiBase}${publicModelsPath({ id: values.model })}`, { credentials: "include" });
      const body = (await response.json().catch(() => ({}))) as { items?: { id: string; status?: string }[] };
      if (!response.ok) {
        form.setError("model", { type: "catalog", message: t("examplesFail") });
        return;
      }
      const found = (body.items || []).find((item) => item.id === values.model);
      if (!found) {
        form.setError("model", { type: "missing", message: t("pgModelMissing") });
        return;
      }
      if (!catalogModelUsable(found)) {
        form.setError("model", { type: "unavailable", message: t("pgModelUnavailable") });
      }
    } catch {
      form.setError("model", { type: "catalog", message: t("examplesFail") });
    }
  }

  async function loadMoreJobs() {
    if (!listCursor) {
      return;
    }
    try {
      const response = await fetch(`${apiBase}${mediaListPath({ kind: kind || undefined, cursor: listCursor })}`, {
        credentials: "include",
      });
      const body = await response.json().catch(() => ({}));
      const nextStorage = applyStorageFact(body, response.ok);
      if (nextStorage) {
        setStorage(nextStorage);
      }
      if (!response.ok) {
        setMessage(body.error?.message || tc("listFailed"));
        return;
      }
      const page = readMediaListPage(body);
      setMoreItems((current) => [...current, ...page.items]);
      setListCursor(page.nextCursor);
    } catch {
      setMessage(tc("listNetwork"));
    }
  }

  async function createJob(values: MediaFormValues) {
    if (entryBlocked) {
      setCreateError(
        !catalogOk ? catalogMessage || tc("listFailed") : modelError === "unavailable" ? t("pgModelUnavailable") : t("pgModelMissing"),
      );
      return;
    }
    if (!values.model.trim()) {
      setCreateError(t("mediaNeedModel"));
      return;
    }
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
            model: created.model || values.model,
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
    if (!signedMediaUsable(previewsRef.current[id])) {
      await loadPreview(id, jobKind, true);
    }
    if (!signedMediaUsable(previewsRef.current[id])) {
      await loadPreview(id, jobKind, true);
    }
    const url = previewsRef.current[id]?.url;
    if (!url) {
      return;
    }
    window.open(url, "_blank", "noopener,noreferrer");
  }

  const loadPreview = useCallback(
    async (id: string, jobKind?: string, force = false) => {
      const cached = previewsRef.current[id];
      if (!force && signedMediaUsable(cached)) {
        return cached.url;
      }
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
        const signed = parseSignedMedia(body);
        if (!signed) {
          setPreviewErrors((current) => ({ ...current, [id]: "存储不可用" }));
          setMessage("存储不可用");
          return "";
        }
        previewsRef.current = { ...previewsRef.current, [id]: signed };
        setPreviews((current) => ({ ...current, [id]: signed }));
        setPreviewErrors((current) => {
          const next = { ...current };
          delete next[id];
          return next;
        });
        return signed.url;
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
      if (!isMediaSuccess(job.status) || signedMediaUsable(previewsRef.current[job.id]) || previewErrors[job.id] || previewInflight.current.has(job.id)) {
        continue;
      }
      previewInflight.current.add(job.id);
      void loadPreview(job.id, job.kind).finally(() => {
        previewInflight.current.delete(job.id);
      });
    }
  }, [completedIds, loadPreview, previewErrors]);

  const pollRef = useRef(false);
  useEffect(() => {
    if (!liveIds) {
      setPollFailed(false);
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
        let failed = false;
        for (const job of active) {
          try {
            const response = await fetch(`${apiBase}${mediaStatusPath(job.kind, job.id)}`, { credentials: "include" });
            const body = await readResponseBody(response);
            if (!response.ok || !body || typeof body !== "object") {
              failed = true;
              continue;
            }
            const next = body as MediaJob;
            if (!next.id) {
              failed = true;
              continue;
            }
            setOverlays((current) => ({ ...current, [next.id]: { ...job, ...next } }));
          } catch {
            failed = true;
          }
        }
        if (!cancelled) {
          setPollFailed(failed);
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
      {entryBlocked ? (
        <div className="mb-3" data-testid="model-entry-error" data-reason={!catalogOk ? "catalog" : modelError} role="alert">
          <EmptyLedger
            title={!catalogOk ? tc("listFailed") : modelError === "unavailable" ? t("pgModelUnavailable") : t("pgModelMissing")}
            detail={!catalogOk ? catalogMessage || tc("listNetwork") : modelError === "unavailable" ? t("pgModelUnavailableDetail", { id: initialModel || "" }) : t("pgModelMissingDetail")}
          />
          <Button asChild variant="outline" size="sm" className="mt-2">
            <Link href={catalogHref}>{t("pgBackCatalog")}</Link>
          </Button>
        </div>
      ) : null}
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
            <Button type="button" disabled={entryBlocked} onClick={() => handleCreateOpenChange(true)}>
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
          <Button type="button" disabled={entryBlocked} onClick={() => handleCreateOpenChange(true)}>
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
                    <span className="text-ink-mute">{pollFailed ? t("mediaPollInterrupted") : t("mediaAutoUpdate")}</span>
                  ) : null}
                </div>
                {item.prompt ? <p className="mt-2 text-sm text-ink">{item.prompt}</p> : null}
                {preview?.url ? (
                  item.kind === "image" ? (
                    // eslint-disable-next-line @next/next/no-img-element
                    <img src={preview.url} alt="" className="mt-2 max-h-40 rounded-control border border-hairline object-contain" />
                  ) : (
                    <video src={preview.url} controls className="mt-2 max-h-40 w-full rounded-control border border-hairline" />
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
      {listCursor ? (
        <Button type="button" variant="outline" size="sm" className="mt-2" onClick={() => void loadMoreJobs()}>
          {t("mediaLoadMore")}
        </Button>
      ) : null}
      <p className="mt-3 text-sm text-ink-secondary">{message}</p>

      <Dialog open={createOpen} onOpenChange={handleCreateOpenChange}>
        <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>{t("createJob")}</DialogTitle>
            <DialogDescription>{t("createJobLead")}</DialogDescription>
          </DialogHeader>
          <Form {...form}>
            <form
              className="grid gap-3"
              onSubmit={form.handleSubmit(createJob, (errors) => {
                if (Object.keys(errors).some((key) => mediaIssueNeedsAdvanced(key as keyof MediaFormValues))) {
                  setAdvancedOpen(true);
                }
              })}
            >
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
                            form.reset(applyMediaMode(form.getValues(), "image"));
                          }}
                        >
                          {t("mediaGenImage")}
                        </Button>
                        <Button
                          type="button"
                          variant={field.value === "video" ? "default" : "outline"}
                          aria-pressed={field.value === "video"}
                          onClick={() => {
                            form.reset(applyMediaMode(form.getValues(), "video"));
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
                name="model"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t("mediaModel")}</FormLabel>
                    <FormControl>
                      <input
                        className="h-10 w-full rounded-control border border-hairline bg-canvas px-3 text-sm text-ink"
                        aria-label={t("mediaModel")}
                        readOnly={Boolean(initialModel) && !modelError}
                        {...field}
                      />
                    </FormControl>
                    <FormMessage />
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
                        value={field.value}
                        onChange={(event) => {
                          form.reset(applyMediaMode(form.getValues(), currentKind, event.target.value));
                        }}
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
                      <FormMessage />
                      {videoAssets.hasMore ? (
                        <Button type="button" variant="outline" size="sm" onClick={videoAssets.loadMore} disabled={videoAssets.loading}>
                          {t("mediaLoadMoreAssets")}
                        </Button>
                      ) : null}
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
                  hasMore={imageAssets.hasMore}
                  loadingMore={imageAssets.loading}
                  loadMoreLabel={t("mediaLoadMoreAssets")}
                  onLoadMore={imageAssets.loadMore}
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
                  hasMore={imageAssets.hasMore}
                  loadingMore={imageAssets.loading}
                  loadMoreLabel={t("mediaLoadMoreAssets")}
                  onLoadMore={imageAssets.loadMore}
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
                  hasMore={imageAssets.hasMore}
                  loadingMore={imageAssets.loading}
                  loadMoreLabel={t("mediaLoadMoreAssets")}
                  onLoadMore={imageAssets.loadMore}
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
                  hasMore={videoAssets.hasMore}
                  loadingMore={videoAssets.loading}
                  loadMoreLabel={t("mediaLoadMoreAssets")}
                  onLoadMore={videoAssets.loadMore}
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
  hasMore,
  loadingMore,
  loadMoreLabel,
  onLoadMore,
  onPick,
}: {
  control: ReturnType<typeof useForm<MediaFormValues>>["control"];
  name: "images" | "first_frame" | "last_frame" | "reference_video" | "reference_audio";
  label: string;
  placeholder?: string;
  jobs: MediaJob[];
  pickLabel: string;
  emptyLabel: string;
  hasMore?: boolean;
  loadingMore?: boolean;
  loadMoreLabel?: string;
  onLoadMore?: () => void;
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
      {hasMore ? (
        <Button type="button" variant="outline" size="sm" onClick={onLoadMore} disabled={loadingMore}>
          {loadMoreLabel}
        </Button>
      ) : null}
    </div>
  );
}
