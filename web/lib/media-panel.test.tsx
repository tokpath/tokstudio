/** @vitest-environment jsdom */
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import MediaPanel from "../app/console/media-panel";
import { withZh } from "./test-i18n";

vi.mock("next/navigation", () => ({
  usePathname: () => "/app/media",
  useSearchParams: () => new URLSearchParams(),
}));

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

  it("creates an image job with the catalog model", async () => {
    const fetchMock = vi.fn().mockImplementation(async (input: RequestInfo) => {
      const url = String(input);
      if (url.includes("/v1/me/media")) {
        return json({ items: [] });
      }
      if (url.includes("/v1/images/generations")) {
        return json({ id: "img_1", kind: "image", task_type: "generate", status: "queued", model: "bytedance/seedream" });
      }
      return json({});
    });
    vi.stubGlobal("fetch", fetchMock);
    render(withZh(<MediaPanel initialKind="image" initialModel="bytedance/seedream" />));
    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).getByLabelText("模型")).toHaveProperty("value", "bytedance/seedream");
    fireEvent.change(within(dialog).getByLabelText("描述你想生成的内容"), { target: { value: "a river" } });
    fireEvent.click(within(dialog).getByRole("button", { name: "新建任务" }));
    await waitFor(() => {
      const create = fetchMock.mock.calls.find((call) => String(call[0]).includes("/v1/images/generations"));
      expect(create).toBeTruthy();
      expect(JSON.parse((create?.[1] as { body: string }).body).model).toBe("bytedance/seedream");
    });
  });

  it("creates a video job with the catalog model", async () => {
    const fetchMock = vi.fn().mockImplementation(async (input: RequestInfo) => {
      const url = String(input);
      if (url.includes("/v1/me/media")) {
        return json({ items: [] });
      }
      if (url.endsWith("/v1/videos")) {
        return json({ id: "vid_1", kind: "video", task_type: "t2v", status: "queued", model: "bytedance/seedance" });
      }
      return json({});
    });
    vi.stubGlobal("fetch", fetchMock);
    render(withZh(<MediaPanel initialKind="video" initialModel="bytedance/seedance" />));
    const dialog = await screen.findByRole("dialog");
    fireEvent.change(within(dialog).getByLabelText("描述你想生成的内容"), { target: { value: "dusk" } });
    fireEvent.click(within(dialog).getByRole("button", { name: "新建任务" }));
    await waitFor(() => {
      const create = fetchMock.mock.calls.find((call) => String(call[0]).endsWith("/v1/videos"));
      expect(create).toBeTruthy();
      expect(JSON.parse((create?.[1] as { body: string }).body).model).toBe("bytedance/seedance");
    });
  });

  it("does not submit when the URL model is missing from the catalog", async () => {
    render(withZh(<MediaPanel initialModel="missing/model" modelError="missing" />));
    await waitFor(() => expect(screen.getByTestId("model-entry-error").getAttribute("data-reason")).toBe("missing"));
    expect(screen.queryByRole("dialog")).toBeNull();
    const fetchMock = vi.mocked(fetch);
    expect(fetchMock.mock.calls.some((call) => String(call[0]).includes("/v1/images") || String(call[0]).includes("/v1/videos"))).toBe(false);
  });
});
