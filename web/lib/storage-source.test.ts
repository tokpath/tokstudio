import { describe, expect, it } from "vitest";
import {
  STORAGE_LABEL_S3,
  STORAGE_LABEL_UNAVAILABLE,
  forbidsSilentSuccessCheck,
  storageBadge,
} from "./storage-source";

describe("W1-S3 storage source badge", () => {
  it("shows muted S3 for MinIO and green S3 for cloud", () => {
    expect(storageBadge({ source: "minio", ok: true, label: "S3" })).toEqual({
      label: STORAGE_LABEL_S3,
      tone: "muted",
      ok: true,
    });
    expect(storageBadge({ source: "s3", ok: true, label: "S3" })).toEqual({
      label: STORAGE_LABEL_S3,
      tone: "ok",
      ok: true,
    });
  });

  it("missing bucket or failure is grey 存储不可用, never a success check", () => {
    const missing = storageBadge({ source: "unavailable", ok: false, detail: "missing bucket" });
    const down = storageBadge({ source: "minio", ok: false });
    const empty = storageBadge(undefined);
    for (const view of [missing, down, empty]) {
      expect(view.label).toBe(STORAGE_LABEL_UNAVAILABLE);
      expect(view.tone).toBe("unavailable");
      expect(view.ok).toBe(false);
      expect(view.label).not.toMatch(/✓|✔|☑/);
      expect(forbidsSilentSuccessCheck(view)).toBe(true);
    }
  });
});
