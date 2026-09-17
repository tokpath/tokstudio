/** @vitest-environment jsdom */
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ChannelPlans } from "@/app/channel/plans-panel";
import { withZh } from "@/lib/test-i18n";

vi.mock("next/navigation", () => ({
  usePathname: () => "/channel/plans",
}));

function jsonResponse(status: number, body: unknown) {
  return { ok: status >= 200 && status < 300, status, json: async () => body };
}

function fillPlan(name = "渠道套餐甲") {
  fireEvent.change(screen.getByLabelText("渠道套餐名"), { target: { value: name } });
  fireEvent.change(screen.getByLabelText("售价（USD）"), { target: { value: "2" } });
  fireEvent.change(screen.getByLabelText("包含额度（USD）"), { target: { value: "2" } });
}

function submitCreate() {
  const buttons = screen.getAllByRole("button", { name: "创建渠道套餐" });
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
    expect(screen.getByLabelText("渠道套餐名")).toHaveProperty("value", "渠道套餐甲");
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
    expect(screen.getByLabelText("渠道套餐名")).toHaveProperty("value", "渠道套餐甲");
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
      expect(screen.getByLabelText("渠道套餐名")).toHaveProperty("value", "新草稿");
    });
    expect(screen.queryByTestId("submit-status")).toBeNull();
  });
});
