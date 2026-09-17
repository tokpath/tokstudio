export type MediaKind = "video" | "image";

export type MediaJob = {
  id: string;
  kind?: string;
  task_type?: string;
  status: string;
  model: string;
  prompt?: string;
  duration?: number;
  resolution?: string;
  aspect_ratio?: string;
  fps?: number;
  generate_audio?: boolean;
  images?: string[];
  first_frame?: string;
  last_frame?: string;
  reference_video?: string;
  reference_audio?: string;
  source_job_id?: string;
  error?: string;
  progress?: number;
};

export type MediaFormValues = {
  prompt: string;
  model: string;
  kind: MediaKind;
  task_type: string;
  duration: number;
  resolution: string;
  aspect_ratio: string;
  fps: number;
  generate_audio: boolean;
  first_frame: string;
  last_frame: string;
  images: string;
  reference_video: string;
  reference_audio: string;
  source_job_id: string;
};

export type MediaModeFields = {
  duration: boolean;
  images: boolean;
  firstFrame: boolean;
  lastFrame: boolean;
  referenceVideo: boolean;
  referenceAudio: boolean;
  sourceJob: boolean;
};

const TERMINAL = new Set(["completed", "succeeded", "failed", "cancelled", "expired"]);

export function isMediaTerminal(status: string): boolean {
  return TERMINAL.has(status);
}

export function isMediaSuccess(status: string): boolean {
  return status === "completed" || status === "succeeded";
}

export function isMediaFailed(status: string): boolean {
  return status === "failed" || status === "cancelled" || status === "expired";
}

export function defaultTaskForKind(kind: MediaKind): string {
  return kind === "image" ? "generate" : "t2v";
}

export function mediaModeFields(kind: string, task: string): MediaModeFields {
  if (kind === "image") {
    return {
      duration: false,
      images: task === "edit",
      firstFrame: false,
      lastFrame: false,
      referenceVideo: false,
      referenceAudio: false,
      sourceJob: false,
    };
  }
  switch (task) {
    case "i2v":
    case "first_frame":
      return {
        duration: true,
        images: true,
        firstFrame: true,
        lastFrame: false,
        referenceVideo: false,
        referenceAudio: false,
        sourceJob: false,
      };
    case "first_last_frame":
      return {
        duration: true,
        images: true,
        firstFrame: true,
        lastFrame: true,
        referenceVideo: false,
        referenceAudio: false,
        sourceJob: false,
      };
    case "reference":
      return {
        duration: true,
        images: true,
        firstFrame: true,
        lastFrame: false,
        referenceVideo: true,
        referenceAudio: true,
        sourceJob: false,
      };
    case "extend":
    case "edit":
      return {
        duration: true,
        images: false,
        firstFrame: false,
        lastFrame: false,
        referenceVideo: false,
        referenceAudio: false,
        sourceJob: true,
      };
    default:
      return {
        duration: true,
        images: false,
        firstFrame: false,
        lastFrame: false,
        referenceVideo: false,
        referenceAudio: false,
        sourceJob: false,
      };
  }
}

export const defaultMediaForm: MediaFormValues = {
  prompt: "",
  model: "",
  kind: "image",
  task_type: "generate",
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
};

export function splitMediaRefs(raw: string): string[] {
  return raw
    .split(/[\n,]/)
    .map((item) => item.trim())
    .filter(Boolean);
}

export function jobToFormValues(job: MediaJob): MediaFormValues {
  const kind: MediaKind = job.kind === "image" ? "image" : "video";
  return {
    prompt: job.prompt?.trim() || "",
    model: job.model?.trim() || "",
    kind,
    task_type: job.task_type || defaultTaskForKind(kind),
    duration: job.duration && job.duration > 0 ? job.duration : 5,
    resolution: job.resolution || "720p",
    aspect_ratio: job.aspect_ratio || "16:9",
    fps: job.fps ?? 24,
    generate_audio: Boolean(job.generate_audio),
    first_frame: job.first_frame || "",
    last_frame: job.last_frame || "",
    images: (job.images || []).join("\n"),
    reference_video: job.reference_video || "",
    reference_audio: job.reference_audio || "",
    source_job_id: job.source_job_id || "",
  };
}

export function mediaStatusPath(kind: string | undefined, id: string): string {
  return kind === "image" ? `/v1/images/${encodeURIComponent(id)}` : `/v1/videos/${encodeURIComponent(id)}`;
}

export function mediaContentPath(kind: string | undefined, id: string): string {
  return `${mediaStatusPath(kind, id)}/content`;
}

export function mergeMediaJobs(items: MediaJob[], overlays: Record<string, MediaJob>): MediaJob[] {
  return items.map((item) => overlays[item.id] ?? item);
}

export function completedJobsOfKind(items: MediaJob[], kind: MediaKind): MediaJob[] {
  return items.filter((item) => (item.kind || "video") === kind && isMediaSuccess(item.status));
}

export function applyMediaMode(values: MediaFormValues, kind: MediaKind, taskType?: string): MediaFormValues {
  const task_type = taskType || defaultTaskForKind(kind);
  const fields = mediaModeFields(kind, task_type);
  const duration = Number(values.duration);
  const fps = Number(values.fps);
  return {
    ...values,
    kind,
    task_type,
    duration: fields.duration && Number.isInteger(duration) && duration >= 1 && duration <= 60 ? duration : defaultMediaForm.duration,
    fps: kind === "video" && Number.isFinite(fps) && fps >= 0 && fps <= 60 ? fps : defaultMediaForm.fps,
    generate_audio: kind === "video" ? Boolean(values.generate_audio) : false,
    images: fields.images ? values.images : "",
    first_frame: fields.firstFrame ? values.first_frame : "",
    last_frame: fields.lastFrame ? values.last_frame : "",
    reference_video: fields.referenceVideo ? values.reference_video : "",
    reference_audio: fields.referenceAudio ? values.reference_audio : "",
    source_job_id: fields.sourceJob ? values.source_job_id : "",
  };
}

export type MediaFormIssue = { path: keyof MediaFormValues; code: "required" | "invalid" };

export function mediaFormIssues(values: MediaFormValues): MediaFormIssue[] {
  const fields = mediaModeFields(values.kind, values.task_type);
  const issues: MediaFormIssue[] = [];
  if (!values.prompt.trim()) {
    issues.push({ path: "prompt", code: "required" });
  }
  if (!values.model.trim()) {
    issues.push({ path: "model", code: "required" });
  }
  if (fields.duration) {
    const duration = Number(values.duration);
    if (!Number.isInteger(duration) || duration < 1 || duration > 60) {
      issues.push({ path: "duration", code: "invalid" });
    }
  }
  if (values.kind === "video") {
    const fps = Number(values.fps);
    if (!Number.isFinite(fps) || fps < 0 || fps > 60) {
      issues.push({ path: "fps", code: "invalid" });
    }
  }
  if (!String(values.resolution || "").trim()) {
    issues.push({ path: "resolution", code: "required" });
  }
  if (fields.images && splitMediaRefs(values.images).length === 0) {
    issues.push({ path: "images", code: "required" });
  }
  if (fields.sourceJob && !values.source_job_id.trim()) {
    issues.push({ path: "source_job_id", code: "required" });
  }
  if (fields.lastFrame && !values.last_frame.trim()) {
    issues.push({ path: "last_frame", code: "required" });
  }
  if (fields.referenceVideo && !values.reference_video.trim()) {
    issues.push({ path: "reference_video", code: "required" });
  }
  if (fields.referenceAudio && !values.reference_audio.trim()) {
    issues.push({ path: "reference_audio", code: "required" });
  }
  if (fields.firstFrame && !fields.images && !values.first_frame.trim()) {
    issues.push({ path: "first_frame", code: "required" });
  }
  return issues;
}

const ADVANCED_PATHS = new Set<keyof MediaFormValues>(["resolution", "aspect_ratio", "fps", "generate_audio"]);

export function mediaIssueNeedsAdvanced(path: keyof MediaFormValues): boolean {
  return ADVANCED_PATHS.has(path);
}

export function buildMediaPayload(values: MediaFormValues): Record<string, unknown> {
  const fields = mediaModeFields(values.kind, values.task_type);
  const payload: Record<string, unknown> = {
    model: values.model.trim(),
    prompt: values.prompt.trim(),
    task_type: values.task_type,
    resolution: values.resolution,
    aspect_ratio: values.aspect_ratio,
  };
  if (fields.duration) {
    payload.duration = values.duration;
  }
  if (values.kind === "video") {
    payload.fps = values.fps;
    payload.generate_audio = values.generate_audio;
  }
  if (fields.images) {
    payload.images = splitMediaRefs(values.images);
  }
  if (fields.firstFrame && values.first_frame.trim()) {
    payload.first_frame = values.first_frame.trim();
  }
  if (fields.lastFrame && values.last_frame.trim()) {
    payload.last_frame = values.last_frame.trim();
  }
  if (fields.referenceVideo && values.reference_video.trim()) {
    payload.reference_video = values.reference_video.trim();
  }
  if (fields.referenceAudio && values.reference_audio.trim()) {
    payload.reference_audio = values.reference_audio.trim();
  }
  if (fields.sourceJob && values.source_job_id.trim()) {
    payload.source_job_id = values.source_job_id.trim();
  }
  return payload;
}

export function mediaCreatePath(values: MediaFormValues): string {
  if (values.kind === "image") {
    return values.task_type === "edit" ? "/v1/images/edits" : "/v1/images/generations";
  }
  if (values.task_type === "extend" && values.source_job_id.trim()) {
    return `/v1/videos/${encodeURIComponent(values.source_job_id.trim())}/extend`;
  }
  return "/v1/videos";
}

export function mediaListPath(query: { kind?: string; status?: string; cursor?: string; limit?: number } = {}): string {
  const params = new URLSearchParams();
  if (query.kind) {
    params.set("kind", query.kind);
  }
  if (query.status) {
    params.set("status", query.status);
  }
  if (query.cursor) {
    params.set("cursor", query.cursor);
  }
  if (query.limit) {
    params.set("limit", String(query.limit));
  }
  const qs = params.toString();
  return qs ? `/v1/me/media?${qs}` : "/v1/me/media";
}

export function readMediaListPage(body: unknown): { items: MediaJob[]; nextCursor: string } {
  const record = body && typeof body === "object" ? (body as { items?: MediaJob[]; next_cursor?: unknown }) : {};
  return {
    items: Array.isArray(record.items) ? record.items : [],
    nextCursor: typeof record.next_cursor === "string" ? record.next_cursor : "",
  };
}

export type SignedMediaUrl = { url: string; expiresAt: number };

const SIGN_TTL_SEC = 15 * 60;
const SIGN_SKEW_MS = 30_000;

export function parseSignedMedia(body: unknown, nowSec = Math.floor(Date.now() / 1000)): SignedMediaUrl | undefined {
  if (!body || typeof body !== "object") {
    return undefined;
  }
  const url = (body as { url?: unknown }).url;
  if (typeof url !== "string" || !url.trim()) {
    return undefined;
  }
  const raw = Number((body as { expires_at?: unknown }).expires_at);
  const expiresAt = Number.isFinite(raw) && raw > 0 ? raw : nowSec + SIGN_TTL_SEC;
  return { url, expiresAt };
}

export function signedMediaUsable(entry: SignedMediaUrl | undefined, nowMs = Date.now()): boolean {
  return Boolean(entry?.url && entry.expiresAt * 1000 - SIGN_SKEW_MS > nowMs);
}
