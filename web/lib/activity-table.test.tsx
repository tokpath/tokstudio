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
    window.history.replaceState({}, "", "/app/activity");
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue({
        ok: true,
        json: async () => ({
          items: [
            {
              id: "req_tiny",
              request_id: "req_tiny",
              result: "succeeded",
              billing_state: "confirmed",
              customer_amount_minor: 26,
              public_model_id: "tokenhub/echo-1",
              api_key_id: "key_alpha",
              prompt_tokens: 12,
              completion_tokens: 4,
              started_at: "2026-09-16T00:00:00.000Z",
            },
            {
              id: "req_fail",
              request_id: "req_fail",
              result: "failed",
              error_code: "rate_limited",
              customer_amount_minor: 0,
              public_model_id: "tokenhub/echo-1",
              started_at: "2026-09-16T01:00:00.000Z",
            },
            {
              id: "req_void",
              request_id: "req_void",
              result: "succeeded",
              billing_state: "voided",
              customer_amount_minor: 0,
              public_model_id: "tokenhub/other",
              started_at: "2026-09-16T02:00:00.000Z",
            },
          ],
        }),
      }),
    );
  });

  it("keeps request result and billing state in separate columns", async () => {
    render(withZh(<ActivityTable />));
    await waitFor(() => expect(screen.getByText("小于 $0.01")).toBeTruthy());
    expect(screen.getByRole("columnheader", { name: "时间" })).toBeTruthy();
    expect(screen.getByRole("columnheader", { name: "模型" })).toBeTruthy();
    expect(screen.getByRole("columnheader", { name: "请求结果" })).toBeTruthy();
    expect(screen.getByRole("columnheader", { name: "计费状态" })).toBeTruthy();
    expect(screen.getByRole("columnheader", { name: "金额" })).toBeTruthy();
    expect(screen.getByRole("columnheader", { name: "失败原因" })).toBeTruthy();
    expect(screen.getByRole("columnheader", { name: "查看详情" })).toBeTruthy();
    expect(screen.getAllByText("成功").length).toBeGreaterThan(0);
    expect(screen.getAllByText("失败").length).toBeGreaterThan(0);
    expect(screen.getAllByText("已确认").length).toBeGreaterThan(0);
    expect(screen.getAllByText("已作废").length).toBeGreaterThan(0);
    expect(screen.getByText("rate_limited")).toBeTruthy();
    expect(screen.queryByText("req_tiny")).toBeNull();
  });

  it("writes request-result filters into the URL and opens failure details", async () => {
    render(withZh(<ActivityTable />));
    await waitFor(() => expect(screen.getByLabelText("请求结果")).toBeTruthy());
    fireEvent.change(screen.getByLabelText("请求结果"), { target: { value: "failed" } });
    expect(nav.replace).toHaveBeenCalledWith("/app/activity?result=failed", { scroll: false });
    fireEvent.click(screen.getAllByRole("button", { name: "查看详情" })[1]);
    await waitFor(() => expect(screen.getByText("req_fail")).toBeTruthy());
    expect(screen.getAllByText("rate_limited").length).toBeGreaterThan(0);
    expect(screen.getByText("这次请求没有对应的账务事件。")).toBeTruthy();
    expect(screen.getByText("列表最多显示符合筛选的最近 100 条，不是完整周期账单。")).toBeTruthy();
  });

  it("keeps every model and key option after the list is filtered to one of them", async () => {
    nav.search = "model=tokenhub%2Fother";
    vi.mocked(fetch).mockResolvedValue({
      ok: true,
      json: async () => ({
        items: [
          {
            id: "req_other",
            request_id: "req_other",
            result: "succeeded",
            billing_state: "confirmed",
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

  it("sends URL filters to the requests API", async () => {
    nav.search = "result=failed&billing=voided&model=tokenhub%2Fecho-1";
    render(withZh(<ActivityTable />));
    await waitFor(() => expect(vi.mocked(fetch).mock.calls.length).toBeGreaterThan(0));
    const url = String(vi.mocked(fetch).mock.calls[0]?.[0]);
    expect(url).toContain("/v1/me/requests?");
    expect(url).toContain("result=failed");
    expect(url).toContain("billing_state=voided");
    expect(url).not.toContain("state=failed");
    expect(url).toContain("public_model_id=tokenhub%2Fecho-1");
    expect(url).toContain("limit=100");
  });

  it("keeps activity filters on the relogin link after a 401", async () => {
    nav.search = "result=failed";
    window.history.replaceState({}, "", "/app/activity?result=failed");
    vi.mocked(fetch).mockResolvedValue({
      ok: false,
      status: 401,
      json: async () => ({ error: { code: "authentication_error", message: "未登录" } }),
    } as Response);
    render(withZh(<ActivityTable />));
    const link = await waitFor(() => screen.getByRole("link", { name: "重新登录" }));
    expect(link.getAttribute("href")).toBe("/login?next=%2Fapp%2Factivity%3Fresult%3Dfailed");
    expect(screen.queryByRole("button", { name: "重试" })).toBeNull();
  });

  it("shows retry without relogin when permission is denied", async () => {
    vi.mocked(fetch).mockResolvedValue({
      ok: false,
      status: 403,
      json: async () => ({ error: { code: "permission_denied", message: "权限不足" } }),
    } as Response);
    render(withZh(<ActivityTable />));
    await waitFor(() => expect(screen.getByRole("button", { name: "重试" })).toBeTruthy());
    expect(screen.queryByRole("link", { name: "重新登录" })).toBeNull();
    expect(screen.getByText("权限不足")).toBeTruthy();
  });
});
