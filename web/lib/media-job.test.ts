import { describe, expect, it } from "vitest";
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
  mediaCreatePath,
  mediaFormIssues,
  mediaModeFields,
  mergeMediaJobs,
} from "./media-job";

describe("media-job", () => {
  it("hides video fields for image generate", () => {
    expect(mediaModeFields("image", "generate")).toEqual({
      duration: false,
      images: false,
      firstFrame: false,
      lastFrame: false,
      referenceVideo: false,
      referenceAudio: false,
      sourceJob: false,
    });
  });

  it("asks for a completed video when extending", () => {
    expect(mediaModeFields("video", "extend").sourceJob).toBe(true);
    expect(mediaModeFields("video", "t2v").sourceJob).toBe(false);
    expect(defaultTaskForKind("image")).toBe("generate");
  });

  it("builds image payload without fps or duration", () => {
    const payload = buildMediaPayload({
      prompt: "a river",
      model: "bytedance/seedream",
      kind: "image",
      task_type: "generate",
      duration: 5,
      resolution: "720p",
      aspect_ratio: "16:9",
      fps: 24,
      generate_audio: true,
      first_frame: "x",
      last_frame: "y",
      images: "https://a.test/1.png",
      reference_video: "https://a.test/v.mp4",
      reference_audio: "https://a.test/a.wav",
      source_job_id: "vid_1",
    });
    expect(payload).toEqual({
      model: "bytedance/seedream",
      prompt: "a river",
      task_type: "generate",
      resolution: "720p",
      aspect_ratio: "16:9",
    });
    expect(
      mediaCreatePath({
        prompt: "a river",
        model: "bytedance/seedream",
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
      }),
    ).toBe("/v1/images/generations");
  });

  it("reuses a completed job without requiring a typed id", () => {
    const form = jobToFormValues({
      id: "vid_1",
      kind: "video",
      task_type: "t2v",
      status: "completed",
      model: "bytedance/seedance-1.0",
      prompt: "dusk river",
      duration: 8,
    });
    expect(form.prompt).toBe("dusk river");
    expect(form.kind).toBe("video");
    expect(form.model).toBe("bytedance/seedance-1.0");
    expect(completedJobsOfKind([{ id: "vid_1", kind: "video", status: "completed", model: "m" }], "video")).toHaveLength(1);
  });

  it("treats in-progress as live and failed as retryable", () => {
    expect(isMediaTerminal("in_progress")).toBe(false);
    expect(isMediaSuccess("completed")).toBe(true);
    expect(isMediaFailed("failed")).toBe(true);
    expect(mergeMediaJobs([{ id: "a", status: "queued", model: "m" }], { a: { id: "a", status: "completed", model: "m" } })[0].status).toBe(
      "completed",
    );
  });

  it("posts video jobs to /v1/videos with the selected model", () => {
    const values = {
      prompt: "dusk",
      model: "bytedance/seedance",
      kind: "video" as const,
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
    };
    expect(buildMediaPayload(values).model).toBe("bytedance/seedance");
    expect(mediaCreatePath(values)).toBe("/v1/videos");
  });

  it("validates only fields the current mode will send", () => {
    const blankVideo = {
      ...defaultMediaForm,
      kind: "video" as const,
      task_type: "t2v",
      prompt: "dusk",
      model: "bytedance/seedance",
      duration: 0,
    };
    expect(mediaFormIssues(blankVideo).map((item) => item.path)).toEqual(["duration"]);
    const afterImage = applyMediaMode(blankVideo, "image");
    expect(afterImage.kind).toBe("image");
    expect(afterImage.task_type).toBe("generate");
    expect(afterImage.duration).toBe(5);
    expect(mediaFormIssues(afterImage)).toEqual([]);
    expect(buildMediaPayload(afterImage)).not.toHaveProperty("duration");
  });

  it("drops image-edit assets when switching to generate", () => {
    const edit = {
      ...defaultMediaForm,
      kind: "image" as const,
      task_type: "edit",
      prompt: "fix sky",
      model: "bytedance/seedream",
      images: "",
    };
    expect(mediaFormIssues(edit).map((item) => item.path)).toEqual(["images"]);
    const generate = applyMediaMode(edit, "image", "generate");
    expect(generate.images).toBe("");
    expect(mediaFormIssues(generate)).toEqual([]);
  });

  it("requires a source job for video extend", () => {
    expect(
      mediaFormIssues({
        ...defaultMediaForm,
        kind: "video",
        task_type: "extend",
        prompt: "longer",
        model: "bytedance/seedance",
        duration: 5,
      }).map((item) => item.path),
    ).toEqual(["source_job_id"]);
  });

  it("rejects whitespace-only prompts", () => {
    expect(
      mediaFormIssues({
        ...defaultMediaForm,
        prompt: "   ",
        model: "bytedance/seedream",
      }).map((item) => item.path),
    ).toEqual(["prompt"]);
  });
});
