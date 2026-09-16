import { describe, expect, it } from "vitest";
import {
  buildMediaPayload,
  completedJobsOfKind,
  defaultTaskForKind,
  isMediaFailed,
  isMediaSuccess,
  isMediaTerminal,
  jobToFormValues,
  mediaCreatePath,
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
      prompt: "a river",
      task_type: "generate",
      resolution: "720p",
      aspect_ratio: "16:9",
    });
    expect(
      mediaCreatePath({
        prompt: "a river",
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
});
