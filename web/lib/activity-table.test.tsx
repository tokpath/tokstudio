/** @vitest-environment jsdom */
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ActivityTable } from "../app/console/activity/activity-table";
import { withZh } from "./test-i18n";

const nav = {
  search: "",
  replace: vi.fn((href: string) => {
    nav.search = href.includes("?") ? href.slice(href.indexOf("?") + 1) : "";
  }),
};

vi.mock("next/navigation", () => ({
  useRouter: () => ({ replace: nav.replace }),
  useSearchParams: () => new URLSearchParams(nav.search),
  usePathname: () => "/app/activity",
}));

describe("ActivityTable", () => {
  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
  });

  beforeEach(() => {
    nav.search = "";
    nav.replace.mockClear();
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue({
        ok: true,
        json: async () => ({
          items: [
            {
              id: "usg_1",
              request_id: "req_tiny",
              state: "confirmed",
              customer_amount_minor: 26,
              public_model_id: "tokenhub/echo-1",
              api_key_id: "key_alpha",
              prompt_tokens: 12,
              completion_tokens: 4,
              occurred_at: "2026-09-16T00:00:00.000Z",
            },
            {
              id: "usg_2",
              request_id: "req_fail",
              state: "failed",
              customer_amount_minor: 0,
              public_model_id: "tokenhub/echo-1",
              occurred_at: "2026-09-16T01:00:00.000Z",
            },
            {
              id: "usg_3",
              request_id: "req_mystery",
              state: "weird_internal_code",
              customer_amount_minor: 1_000_000,
              public_model_id: "tokenhub/other",
            },
          ],
        }),
      }),
    );
  });

  it("keeps the default table to time, model, result, fee, and details", async () => {
    render(withZh(<ActivityTable />));
    await waitFor(() => expect(screen.getByText("小于 $0.01")).toBeTruthy());
    expect(screen.getByRole("columnheader", { name: "时间" })).toBeTruthy();
    expect(screen.getByRole("columnheader", { name: "模型" })).toBeTruthy();
    expect(screen.getByRole("columnheader", { name: "结果" })).toBeTruthy();
    expect(screen.getByRole("columnheader", { name: "金额" })).toBeTruthy();
    expect(screen.getByRole("columnheader", { name: "查看详情" })).toBeTruthy();
    expect(screen.queryByRole("columnheader", { name: "输入" })).toBeNull();
    expect(screen.queryByRole("columnheader", { name: "API Key" })).toBeNull();
    expect(screen.getAllByText("已确认").length).toBeGreaterThan(0);
    expect(screen.getAllByText("失败").length).toBeGreaterThan(0);
    expect(screen.getAllByText("未知状态（weird_internal_code）").length).toBeGreaterThan(0);
    expect(screen.queryByText("req_tiny")).toBeNull();
  });

  it("writes status filters into the URL and opens failure details", async () => {
    render(withZh(<ActivityTable />));
    await waitFor(() => expect(screen.getByLabelText("结果")).toBeTruthy());
    fireEvent.change(screen.getByLabelText("结果"), { target: { value: "failed" } });
    expect(nav.replace).toHaveBeenCalledWith("/app/activity?status=failed", { scroll: false });
    fireEvent.click(screen.getAllByRole("button", { name: "查看详情" })[1]);
    await waitFor(() => expect(screen.getByText("req_fail")).toBeTruthy());
    expect(screen.getByText("这条记录没有单独的失败原因，只保留了结果状态。")).toBeTruthy();
    expect(screen.getByText("列表最多显示符合筛选的最近 100 条，不是完整周期账单。")).toBeTruthy();
  });

  it("keeps every model and key option after the list is filtered to one of them", async () => {
    nav.search = "model=tokenhub%2Fother";
    vi.mocked(fetch).mockResolvedValue({
      ok: true,
      json: async () => ({
        items: [
          {
            id: "usg_3",
            request_id: "req_other",
            state: "confirmed",
            customer_amount_minor: 1_000_000,
            public_model_id: "tokenhub/other",
            api_key_id: "key_beta",
          },
        ],
        models: [{ key: "tokenhub/echo-1" }, { key: "tokenhub/other" }],
        keys: [{ key: "key_alpha" }, { key: "key_beta" }],
      }),
    });
    render(withZh(<ActivityTable />));
    const model = await waitFor(() => screen.getByLabelText("按模型筛选"));
    expect([...model.querySelectorAll("option")].map((option) => option.value)).toEqual([
      "",
      "tokenhub/echo-1",
      "tokenhub/other",
    ]);
    const key = screen.getByLabelText("按 API Key 筛选");
    expect([...key.querySelectorAll("option")].map((option) => option.value)).toEqual(["", "key_alpha", "key_beta"]);
  });

  it("labels token counts as input and output tokens in the detail dialog", async () => {
    render(withZh(<ActivityTable />));
    await waitFor(() => expect(screen.getAllByRole("button", { name: "查看详情" }).length).toBeGreaterThan(0));
    fireEvent.click(screen.getAllByRole("button", { name: "查看详情" })[0]);
    await waitFor(() => expect(screen.getByText("输入 Token")).toBeTruthy());
    expect(screen.getByText("输出 Token")).toBeTruthy();
    expect(screen.getByText("12")).toBeTruthy();
    expect(screen.getByText("4")).toBeTruthy();
  });

  it("sends URL filters to the usage API", async () => {
    nav.search = "status=failed&model=tokenhub%2Fecho-1";
    render(withZh(<ActivityTable />));
    await waitFor(() => expect(vi.mocked(fetch).mock.calls.length).toBeGreaterThan(0));
    const url = String(vi.mocked(fetch).mock.calls[0]?.[0]);
    expect(url).toContain("state=failed");
    expect(url).toContain("public_model_id=tokenhub%2Fecho-1");
    expect(url).toContain("limit=100");
  });
});
