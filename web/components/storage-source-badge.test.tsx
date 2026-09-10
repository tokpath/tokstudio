/** @vitest-environment jsdom */
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { StorageSourceBadge } from "./storage-source-badge";

afterEach(() => {
  cleanup();
});

describe("StorageSourceBadge", () => {
  it("renders muted S3 for MinIO and never a checkmark", () => {
    render(<StorageSourceBadge storage={{ source: "minio", ok: true }} />);
    expect(screen.getByText("存储源")).toBeTruthy();
    const badge = screen.getByTestId("storage-source-badge");
    expect(badge.textContent).toBe("S3");
    expect(badge.getAttribute("data-ok")).toBe("true");
    expect(badge.getAttribute("data-tone")).toBe("muted");
    expect(badge.className).toContain("--muted");
    expect(badge.textContent).not.toMatch(/✓|✔|☑/);
  });

  it("renders green S3 for cloud", () => {
    render(<StorageSourceBadge storage={{ source: "s3", ok: true }} />);
    const badge = screen.getByTestId("storage-source-badge");
    expect(badge.textContent).toBe("S3");
    expect(badge.getAttribute("data-tone")).toBe("ok");
    expect(badge.className).toContain("--success");
  });

  it("renders grey 存储不可用 on missing bucket and forbids silent success", () => {
    render(<StorageSourceBadge storage={{ source: "unavailable", ok: false, detail: "missing bucket" }} />);
    const badge = screen.getByTestId("storage-source-badge");
    expect(badge.textContent).toBe("存储不可用");
    expect(badge.getAttribute("data-ok")).toBe("false");
    expect(badge.getAttribute("data-tone")).toBe("unavailable");
    expect(badge.className).not.toContain("--success");
    expect(badge.textContent).not.toMatch(/✓|✔|☑|已上传/);
  });
});
