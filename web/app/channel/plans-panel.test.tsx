/** @vitest-environment jsdom */
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ChannelPlans } from "@/app/channel/plans-panel";
import { withZh } from "@/lib/test-i18n";

vi.mock("@/components/rbac/viewer-context", async (importOriginal) => ({ ...(await importOriginal<typeof import("@/components/rbac/viewer-context")>()), useViewer: () => ({ signedIn: true, loading: false, roles: ["channel_admin"], userId: "test-authorized-viewer", channelType: "C" }) }));

vi.mock("next/navigation", () => ({
  usePathname: () => "/channel/plans",
}));

function jsonResponse(status: number, body: unknown) {
  return { ok: status >= 200 && status < 300, status, json: async () => body };
}

function planDialog() {
  return screen.getByRole("dialog");
}

function fillPlan(name = "渠道套餐甲") {
  if (!screen.queryByRole("dialog")) {
    fireEvent.click(screen.getByRole("button", { name: "创建套餐" }));
  }
  const dialog = planDialog();
  fireEvent.change(within(dialog).getByLabelText("套餐名"), { target: { value: name } });
  fireEvent.change(within(dialog).getByLabelText("售价（USD）"), { target: { value: "2" } });
  fireEvent.change(within(dialog).getByLabelText("包含额度（USD）"), { target: { value: "2" } });
}

function submitCreate() {
  const buttons = screen.getAllByRole("button", { name: "创建套餐" });
  fireEvent.click(buttons[buttons.length - 1]);
}

describe("ChannelPlans create", () => {
  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
  });

  it("locks double submit, keeps values on API failure, and shows network errors", async () => {
    let finish!: (value: { ok: boolean; body?: unknown } | "network") => void;
    const gate = new Promise<{ ok: boolean; body?: unknown } | "network">((resolve) => {
      finish = resolve;
    });
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.includes("/channel/plans") && init?.method === "POST") {
        const result = await gate;
        if (result === "network") {
          throw new TypeError("Failed to fetch");
        }
        return jsonResponse(result.ok ? 201 : 400, result.body ?? {});
      }
      return jsonResponse(200, { items: [] });
    });
    vi.stubGlobal("fetch", fetchMock);

    render(withZh(<ChannelPlans />));
    await screen.findByText("暂无可见套餐");
    fillPlan();
    submitCreate();
    submitCreate();
    await waitFor(() => expect(screen.getByRole("button", { name: "提交中" })).toBeTruthy());
    expect(fetchMock.mock.calls.filter((call) => String(call[0]).includes("/channel/plans") && call[1]?.method === "POST")).toHaveLength(1);

    finish({ ok: false, body: { error: { message: "售价过低需审核但渠道无效" } } });
    await waitFor(() => {
      expect(screen.getByTestId("submit-status").textContent).toContain("售价过低需审核但渠道无效");
    });
    expect(within(planDialog()).getByLabelText("套餐名")).toHaveProperty("value", "渠道套餐甲");
  });

  it("shows a network error instead of failing silently", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        if (String(input).includes("/channel/plans") && init?.method === "POST") {
          throw new TypeError("Failed to fetch");
        }
        return jsonResponse(200, { items: [] });
      }),
    );
    render(withZh(<ChannelPlans />));
    await screen.findByText("暂无可见套餐");
    fillPlan();
    submitCreate();
    await waitFor(() => {
      expect(screen.getByTestId("submit-status").textContent).toContain("网络不可用，请重试。");
    });
    expect(within(planDialog()).getByLabelText("套餐名")).toHaveProperty("value", "渠道套餐甲");
  });

  it("keeps a new draft when a stale in-flight create later succeeds", async () => {
    let finish!: (value: unknown) => void;
    const gate = new Promise<unknown>((resolve) => {
      finish = resolve;
    });
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        if (String(input).includes("/channel/plans") && init?.method === "POST") {
          await gate;
          return jsonResponse(201, { item: { id: "pln_old", name: "旧草稿", status: "pending_review" } });
        }
        return jsonResponse(200, { items: [] });
      }),
    );
    const { unmount } = render(withZh(<ChannelPlans />));
    await screen.findByText("暂无可见套餐");
    fillPlan("旧草稿");
    submitCreate();
    await waitFor(() => expect(screen.getByRole("button", { name: "提交中" })).toBeTruthy());
    unmount();
    render(withZh(<ChannelPlans />));
    await screen.findByText("暂无可见套餐");
    fillPlan("新草稿");
    finish({});
    await waitFor(() => {
      expect(within(planDialog()).getByLabelText("套餐名")).toHaveProperty("value", "新草稿");
    });
    expect(screen.queryByTestId("submit-status")).toBeNull();
  });

  it("asks the API for status, channel, period, and name together", async () => {
    const urls: string[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        const url = String(input);
        urls.push(url);
        if (url.includes("/eligible-channels")) {
          return jsonResponse(200, { items: [{ id: "ch_child", code: "child-a", status: "active" }] });
        }
        return jsonResponse(200, { items: [] });
      }),
    );
    render(withZh(<ChannelPlans />));
    await screen.findByText("暂无可见套餐");
    fireEvent.change(screen.getByLabelText("按状态筛选"), { target: { value: "published" } });
    await screen.findByRole("option", { name: "child-a" });
    fireEvent.change(screen.getByLabelText("按渠道筛选"), { target: { value: "ch_child" } });
    fireEvent.change(screen.getByLabelText("按套餐类型筛选"), { target: { value: "monthly" } });
    fireEvent.change(screen.getByLabelText("按套餐名筛选"), { target: { value: " 入门 " } });
    fireEvent.click(screen.getByRole("button", { name: "筛选" }));
    await waitFor(() => {
      expect(urls.some((url) => url.includes("/channel/plans?") && url.includes("status=published") && url.includes("channel_id=ch_child") && url.includes("billing_period=monthly") && url.includes("name="))).toBe(true);
    });
    const matched = urls.find((url) => url.includes("status=published") && url.includes("channel_id=ch_child") && url.includes("billing_period=monthly") && url.includes("name="));
    expect(decodeURIComponent(matched || "")).toContain("name=入门");
  });
});
