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

export function buildMediaPayload(values: MediaFormValues): Record<string, unknown> {
  const fields = mediaModeFields(values.kind, values.task_type);
  const payload: Record<string, unknown> = {
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
