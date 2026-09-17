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

  it("submits image generate after an invalid video duration", async () => {
    const fetchMock = vi.fn().mockImplementation(async (input: RequestInfo) => {
      const url = String(input);
      if (url.includes("/v1/me/media")) {
        return json({ items: [] });
      }
      if (url.includes("/v1/images/generations")) {
        return json({ id: "img_switch", kind: "image", task_type: "generate", status: "queued", model: "m" });
      }
      return json({});
    });
    vi.stubGlobal("fetch", fetchMock);
    render(withZh(<MediaPanel />));
    await waitFor(() => expect(screen.getByText("暂无媒体任务")).toBeTruthy());
    fireEvent.click(screen.getAllByRole("button", { name: "新建任务" })[0]);
    const dialog = screen.getByRole("dialog");
    fireEvent.click(within(dialog).getByRole("button", { name: "生成视频" }));
    fireEvent.change(within(dialog).getByLabelText("时长"), { target: { value: "" } });
    fireEvent.click(within(dialog).getByRole("button", { name: "生成图片" }));
    fireEvent.change(within(dialog).getByLabelText("模型"), { target: { value: "bytedance/seedream" } });
    fireEvent.change(within(dialog).getByLabelText("描述你想生成的内容"), { target: { value: "a river" } });
    fireEvent.click(within(dialog).getByRole("button", { name: "新建任务" }));
    await waitFor(() => {
      const create = fetchMock.mock.calls.find((call) => String(call[0]).includes("/v1/images/generations"));
      expect(create).toBeTruthy();
      expect(JSON.parse((create?.[1] as { body: string }).body)).not.toHaveProperty("duration");
    });
  });

  it("submits generate after leaving image edit without assets", async () => {
    const fetchMock = vi.fn().mockImplementation(async (input: RequestInfo) => {
      const url = String(input);
      if (url.includes("/v1/me/media")) {
        return json({ items: [] });
      }
      if (url.includes("/v1/images/generations")) {
        return json({ id: "img_gen", kind: "image", task_type: "generate", status: "queued", model: "m" });
      }
      return json({});
    });
    vi.stubGlobal("fetch", fetchMock);
    render(withZh(<MediaPanel />));
    await waitFor(() => expect(screen.getByText("暂无媒体任务")).toBeTruthy());
    fireEvent.click(screen.getAllByRole("button", { name: "新建任务" })[0]);
    const dialog = screen.getByRole("dialog");
    fireEvent.change(within(dialog).getByLabelText("生成模式"), { target: { value: "edit" } });
    fireEvent.change(within(dialog).getByLabelText("模型"), { target: { value: "bytedance/seedream" } });
    fireEvent.change(within(dialog).getByLabelText("描述你想生成的内容"), { target: { value: "fix sky" } });
    fireEvent.change(within(dialog).getByLabelText("生成模式"), { target: { value: "generate" } });
    fireEvent.click(within(dialog).getByRole("button", { name: "新建任务" }));
    await waitFor(() => {
      expect(fetchMock.mock.calls.some((call) => String(call[0]).includes("/v1/images/generations"))).toBe(true);
      expect(fetchMock.mock.calls.some((call) => String(call[0]).includes("/v1/images/edits"))).toBe(false);
    });
  });

  it("shows a visible source error when extending without a job", async () => {
    render(withZh(<MediaPanel />));
    await waitFor(() => expect(screen.getByText("暂无媒体任务")).toBeTruthy());
    fireEvent.click(screen.getAllByRole("button", { name: "新建任务" })[0]);
    const dialog = screen.getByRole("dialog");
    fireEvent.click(within(dialog).getByRole("button", { name: "生成视频" }));
    fireEvent.change(within(dialog).getByLabelText("生成模式"), { target: { value: "extend" } });
    fireEvent.change(within(dialog).getByLabelText("模型"), { target: { value: "bytedance/seedance" } });
    fireEvent.change(within(dialog).getByLabelText("描述你想生成的内容"), { target: { value: "longer" } });
    fireEvent.click(within(dialog).getByRole("button", { name: "新建任务" }));
    await waitFor(() => expect(within(dialog).getByText("请选择要延长或编辑的源视频。")).toBeTruthy());
    expect(within(dialog).getByLabelText("延长哪条已完成的视频")).toBeTruthy();
    expect(vi.mocked(fetch).mock.calls.some((call) => String(call[0]).includes("/v1/videos"))).toBe(false);
  });

  it("shows a visible prompt error for whitespace-only descriptions", async () => {
    render(withZh(<MediaPanel />));
    await waitFor(() => expect(screen.getByText("暂无媒体任务")).toBeTruthy());
    fireEvent.click(screen.getAllByRole("button", { name: "新建任务" })[0]);
    const dialog = screen.getByRole("dialog");
    fireEvent.change(within(dialog).getByLabelText("模型"), { target: { value: "bytedance/seedream" } });
    fireEvent.change(within(dialog).getByLabelText("描述你想生成的内容"), { target: { value: "   " } });
    fireEvent.click(within(dialog).getByRole("button", { name: "新建任务" }));
    await waitFor(() => expect(within(dialog).getByText("请填写要生成的内容，不能只是空格。")).toBeTruthy());
    expect(vi.mocked(fetch).mock.calls.some((call) => String(call[0]).includes("/v1/images"))).toBe(false);
  });

  it("reuses a non-default model when creating again", async () => {
    const fetchMock = vi.fn().mockImplementation(async (input: RequestInfo, init?: RequestInit) => {
      const url = String(input);
      if (url.includes("/v1/public/models")) {
        return json({ items: [{ id: "vendor/custom-1", status: "available", kind: "image" }] });
      }
      if (url.includes("/v1/me/media")) {
        return json({
          items: [
            {
              id: "img_done",
              kind: "image",
              task_type: "generate",
              status: "completed",
              model: "vendor/custom-1",
              prompt: "sky",
            },
          ],
        });
      }
      if (url.includes("/content")) {
        return json({ url: "https://cdn.test/sky.png", expires_at: Math.floor(Date.now() / 1000) + 900 });
      }
      if (url.includes("/v1/images/generations") && init?.method === "POST") {
        return json({ id: "img_2", kind: "image", status: "queued", model: "vendor/custom-1" });
      }
      return json({});
    });
    vi.stubGlobal("fetch", fetchMock);
    render(withZh(<MediaPanel />));
    await waitFor(() => expect(screen.getByTestId("media-job-img_done")).toBeTruthy());
    fireEvent.click(screen.getByRole("button", { name: "再次使用此配置" }));
    const dialog = await screen.findByRole("dialog");
    await waitFor(() => expect(within(dialog).getByLabelText("模型")).toHaveProperty("value", "vendor/custom-1"));
    fireEvent.click(within(dialog).getByRole("button", { name: "新建任务" }));
    await waitFor(() => {
      const create = fetchMock.mock.calls.find((call) => String(call[0]).includes("/v1/images/generations"));
      expect(JSON.parse((create?.[1] as { body: string }).body).model).toBe("vendor/custom-1");
    });
  });

  it("can pick an existing image while the job list is filtered to video", async () => {
    const fetchMock = vi.fn().mockImplementation(async (input: RequestInfo) => {
      const url = String(input);
      if (url.includes("/v1/me/media") && url.includes("kind=image")) {
        return json({
          items: [{ id: "img_ref", kind: "image", status: "completed", model: "m", prompt: "ref pic" }],
        });
      }
      if (url.includes("/v1/me/media")) {
        return json({
          items: [{ id: "vid_only", kind: "video", status: "completed", model: "m", prompt: "clip" }],
        });
      }
      if (url.includes("/content")) {
        return json({ url: "https://cdn.test/ref.png", expires_at: Math.floor(Date.now() / 1000) + 900 });
      }
      return json({});
    });
    vi.stubGlobal("fetch", fetchMock);
    render(withZh(<MediaPanel />));
    await waitFor(() => expect(screen.getByTestId("media-job-vid_only")).toBeTruthy());
    fireEvent.change(screen.getByLabelText("筛选媒体类型"), { target: { value: "video" } });
    fireEvent.click(screen.getAllByRole("button", { name: "新建任务" })[0]);
    const dialog = await screen.findByRole("dialog");
    fireEvent.change(within(dialog).getByLabelText("生成模式"), { target: { value: "edit" } });
    const picker = await within(dialog).findByLabelText("从已完成任务选择素材");
    await waitFor(() => expect(within(picker).getByRole("option", { name: "ref pic" })).toBeTruthy());
    fireEvent.change(picker, { target: { value: "img_ref" } });
    await waitFor(() => expect(within(dialog).getByLabelText("参考图")).toHaveProperty("value", "https://cdn.test/ref.png"));
  });

  it("can pick an asset from the next media page", async () => {
    const fetchMock = vi.fn().mockImplementation(async (input: RequestInfo) => {
      const url = String(input);
      if (url.includes("/v1/me/media") && url.includes("kind=image") && url.includes("cursor=img_a")) {
        return json({
          items: [{ id: "img_b", kind: "image", status: "completed", model: "m", prompt: "second page" }],
        });
      }
      if (url.includes("/v1/me/media") && url.includes("kind=image")) {
        return json({
          items: [{ id: "img_a", kind: "image", status: "completed", model: "m", prompt: "first page" }],
          next_cursor: "img_a",
        });
      }
      if (url.includes("/v1/me/media")) {
        return json({ items: [] });
      }
      if (url.includes("/content")) {
        return json({ url: "https://cdn.test/b.png", expires_at: Math.floor(Date.now() / 1000) + 900 });
      }
      return json({});
    });
    vi.stubGlobal("fetch", fetchMock);
    render(withZh(<MediaPanel />));
    await waitFor(() => expect(screen.getByText("暂无媒体任务")).toBeTruthy());
    fireEvent.click(screen.getAllByRole("button", { name: "新建任务" })[0]);
    const dialog = await screen.findByRole("dialog");
    fireEvent.change(within(dialog).getByLabelText("生成模式"), { target: { value: "edit" } });
    await waitFor(() => expect(within(dialog).getByRole("button", { name: "加载更多素材" })).toBeTruthy());
    fireEvent.click(within(dialog).getByRole("button", { name: "加载更多素材" }));
    const picker = within(dialog).getByLabelText("从已完成任务选择素材");
    await waitFor(() => expect(within(picker).getByRole("option", { name: "second page" })).toBeTruthy());
    fireEvent.change(picker, { target: { value: "img_b" } });
    await waitFor(() => expect(within(dialog).getByLabelText("参考图")).toHaveProperty("value", "https://cdn.test/b.png"));
  });

  it("refetches a signed download after expiry", async () => {
    const open = vi.fn();
    vi.stubGlobal("open", open);
    let contentCalls = 0;
    const fetchMock = vi.fn().mockImplementation(async (input: RequestInfo) => {
      const url = String(input);
      if (url.includes("/v1/me/media")) {
        return json({
          items: [
            {
              id: "img_done",
              kind: "image",
              task_type: "generate",
              status: "completed",
              model: "m",
              prompt: "sky",
            },
          ],
        });
      }
      if (url.includes("/content")) {
        contentCalls += 1;
        if (contentCalls === 1) {
          return json({ url: "https://cdn.test/old.png", expires_at: 1 });
        }
        return json({ url: "https://cdn.test/new.png", expires_at: Math.floor(Date.now() / 1000) + 900 });
      }
      return json({});
    });
    vi.stubGlobal("fetch", fetchMock);
    render(withZh(<MediaPanel />));
    await waitFor(() => expect(screen.getByRole("button", { name: "下载" })).toBeTruthy());
    fireEvent.click(screen.getByRole("button", { name: "下载" }));
    await waitFor(() => expect(open).toHaveBeenCalledWith("https://cdn.test/new.png", "_blank", "noopener,noreferrer"));
  });

  it("says status updates stopped after a poll failure", async () => {
    const fetchMock = vi.fn().mockImplementation(async (input: RequestInfo) => {
      const url = String(input);
      if (url.includes("/v1/me/media")) {
        return json({
          items: [
            {
              id: "vid_live",
              kind: "video",
              task_type: "t2v",
              status: "in_progress",
              model: "m",
              prompt: "river",
            },
          ],
        });
      }
      if (url.includes("/v1/videos/vid_live")) {
        return json({ error: { message: "upstream down" } }, false, 502);
      }
      return json({});
    });
    vi.stubGlobal("fetch", fetchMock);
    render(withZh(<MediaPanel />));
    await waitFor(() => expect(screen.getByText("状态更新已中断，请刷新后再看。")).toBeTruthy());
    expect(screen.queryByText("自动更新中")).toBeNull();
  });
});
