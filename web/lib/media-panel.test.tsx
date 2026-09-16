/** @vitest-environment jsdom */
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import MediaPanel from "../app/console/media-panel";
import { withZh } from "./test-i18n";

function json(data: unknown, ok = true, status = 200) {
  return {
    ok,
    status,
    json: async () => data,
  };
}

describe("MediaPanel", () => {
  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
    vi.useRealTimers();
  });

  beforeEach(() => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        json({
          items: [],
          storage: { source: "minio", ok: true, label: "S3" },
        }),
      ),
    );
  });

  it("keeps text-to-image to prompt and hides fps until advanced video settings", async () => {
    render(withZh(<MediaPanel />));
    await waitFor(() => expect(screen.getByText("暂无媒体任务")).toBeTruthy());
    fireEvent.click(screen.getAllByRole("button", { name: "新建任务" })[0]);
    expect(screen.getByRole("button", { name: "生成图片" })).toBeTruthy();
    expect(screen.getByLabelText("描述你想生成的内容")).toBeTruthy();
    expect(screen.queryByLabelText("帧率")).toBeNull();
    expect(screen.queryByLabelText("时长")).toBeNull();
    expect(screen.queryByLabelText("参考视频")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "生成视频" }));
    expect(screen.getByLabelText("时长")).toBeTruthy();
    expect(screen.queryByLabelText("帧率")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "高级设置" }));
    expect(screen.getByLabelText("帧率")).toBeTruthy();
  });

  it("polls in-progress jobs and stops after a terminal status", async () => {
    const fetchMock = vi
      .fn()
      .mockImplementation(async (input: RequestInfo) => {
        const url = String(input);
        if (url.includes("/v1/me/media")) {
          return json({
            items: [
              {
                id: "vid_live",
                kind: "video",
                task_type: "t2v",
                status: "in_progress",
                model: "bytedance/seedance-1.0",
                prompt: "river",
              },
            ],
          });
        }
        if (url.includes("/v1/videos/vid_live") && !url.includes("/content")) {
          return json({
            id: "vid_live",
            kind: "video",
            task_type: "t2v",
            status: "completed",
            model: "bytedance/seedance-1.0",
            prompt: "river",
          });
        }
        if (url.includes("/content")) {
          return json({ url: "https://cdn.test/river.mp4" });
        }
        return json({});
      });
    vi.stubGlobal("fetch", fetchMock);
    render(withZh(<MediaPanel />));
    await waitFor(() => expect(screen.getByTestId("media-job-vid_live").getAttribute("data-status")).toBe("in_progress"));
    await waitFor(() => expect(screen.getByTestId("media-job-vid_live").getAttribute("data-status")).toBe("completed"));
    await waitFor(() => expect(screen.getByRole("button", { name: "再次使用此配置" })).toBeTruthy());
    const statusCalls = fetchMock.mock.calls.filter((call) => String(call[0]).includes("/v1/videos/vid_live") && !String(call[0]).includes("/content"));
    expect(statusCalls.length).toBeGreaterThan(0);
  });
});
