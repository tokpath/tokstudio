// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import type { APIKey } from "@/lib/contracts";
import { CustomerKeys } from "./customer-keys";

const apiKey: APIKey = { id: "key-one", name: "现有应用", prefix: "th_prefix", status: "active", model_ids: null, budget: { limit_atg: "5.000000000000000001", period: "month", timezone: "Asia/Shanghai", spent_atg: "1.000000000000000001", held_atg: "0.000000000000000001", overage_atg: "0", window_start: null, window_end: null }, rate_limit_rpm: null, concurrency_limit: null, version: 3, created_at: "2026-10-18T00:00:00Z" };
const response = (value: unknown) => new Response(JSON.stringify(value), { status: 200 });
function show() {
  const cache = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={cache}><CustomerKeys /></QueryClientProvider>);
}
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

it("copies a stored key from the list at any time", async () => {
  const writeText = vi.fn().mockResolvedValue(undefined);
  Object.assign(navigator, { clipboard: { writeText } });
  vi.stubGlobal("fetch", vi.fn(async () => response({ items: [{ ...apiKey, secret: "th_live_secret" }] })));
  show();
  expect(await screen.findByText("th_live_secret")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "复制密钥" }));
  await waitFor(() => expect(writeText).toHaveBeenCalledWith("th_live_secret"));
});

it("creates a precise budget with an explicitly empty model scope and hides the one-time secret on close", async () => {
  const calls: RequestInit[] = [];
  vi.stubGlobal("fetch", vi.fn(async (path: RequestInfo | URL, init?: RequestInit) => {
    if (init?.method === "POST") { calls.push(init); return response({ key: apiKey, secret: "synthetic_one_time_secret" }); }
    return response({ items: String(path).endsWith("/models") ? [] : [apiKey] });
  }));
  show();
  fireEvent.click(screen.getByRole("button", { name: "创建 Key", exact: true }));
  fireEvent.change(screen.getByLabelText("名称", { exact: true }), { target: { value: "新应用" } });
  fireEvent.change(screen.getByLabelText("ATG 预算上限"), { target: { value: "0.000000000000000000000001" } });
  fireEvent.change(screen.getByLabelText("预算窗口"), { target: { value: "quarter" } });
  fireEvent.click(screen.getByLabelText("不额外限定模型"));
  fireEvent.click(screen.getByRole("button", { name: "创建 Key", exact: true }));
  await screen.findByText("synthetic_one_time_secret");
  expect(JSON.parse(String(calls[0].body))).toEqual({ name: "新应用", model_ids: [], budget: { limit_atg: "0.000000000000000000000001", period: "quarter" }, rate_limit_rpm: null, concurrency_limit: null });
  expect((screen.getByRole("button", { name: "编辑配置" }) as HTMLButtonElement).disabled).toBe(true);
  fireEvent.click(screen.getByRole("button", { name: "关闭", exact: true }));
  expect(screen.queryByText("synthetic_one_time_secret")).toBeNull();
  expect(screen.getByText("th_prefix…")).toBeTruthy();
});

it("rejects invalid counters before creating a Key", async () => {
  const fetch = vi.fn(async () => response({ items: [] }));
  vi.stubGlobal("fetch", fetch);
  show();
  fireEvent.click(screen.getByRole("button", { name: "创建 Key", exact: true }));
  fireEvent.change(screen.getByLabelText("名称", { exact: true }), { target: { value: "新应用" } });
  fireEvent.change(screen.getByLabelText("每分钟请求上限"), { target: { value: "9007199254740993" } });
  fireEvent.click(screen.getByRole("button", { name: "创建 Key", exact: true }));
  expect(screen.getByRole("alert").textContent).toContain("正整数");
  expect(fetch.mock.calls.some(call => (call as unknown as [unknown, RequestInit])[1]?.method === "POST")).toBe(false);
});

it("retains unknown Key creation through disabled close and edit actions and retries the exact request", async () => {
  const calls: RequestInit[] = [];
  vi.stubGlobal("fetch", vi.fn(async (path: RequestInfo | URL, init?: RequestInit) => {
    if (init?.method === "POST") { calls.push(init); if (calls.length === 1) throw new TypeError("connection lost"); return response({ key: apiKey, secret: "synthetic_recovered_secret" }); }
    return response({ items: String(path).endsWith("/models") ? [] : [apiKey] });
  }));
  show();
  await screen.findByText("现有应用");
  fireEvent.click(screen.getByRole("button", { name: "创建 Key", exact: true }));
  fireEvent.change(screen.getByLabelText("名称", { exact: true }), { target: { value: "新应用" } });
  fireEvent.click(screen.getByRole("button", { name: "创建 Key", exact: true }));
  await screen.findByRole("button", { name: "重试本次操作" });
  expect(screen.getByLabelText("名称", { exact: true }).matches(":disabled")).toBe(true);
  expect((screen.getByRole("button", { name: "关闭", exact: true }) as HTMLButtonElement).disabled).toBe(true);
  expect((screen.getByRole("button", { name: "编辑配置" }) as HTMLButtonElement).disabled).toBe(true);
  fireEvent.click(screen.getByRole("button", { name: "重试本次操作" }));
  await screen.findByText("synthetic_recovered_secret");
  expect(calls).toHaveLength(2);
  expect(calls[1].body).toBe(calls[0].body);
  expect((calls[1].headers as Headers).get("Idempotency-Key")).toBe((calls[0].headers as Headers).get("Idempotency-Key"));
});

it("does not replace an unresolved reset with a second destructive action", async () => {
  const calls: RequestInit[] = [];
  vi.stubGlobal("fetch", vi.fn(async (_path: RequestInfo | URL, init?: RequestInit) => {
    if (init?.method === "POST") { calls.push(init); if (calls.length === 1) throw new TypeError("connection lost"); return response({ key: apiKey, secret: "synthetic_reset_secret" }); }
    return response({ items: [apiKey] });
  }));
  show();
  fireEvent.click(await screen.findByRole("button", { name: "重置密钥" }));
  fireEvent.click(screen.getByRole("button", { name: "确认", exact: true }));
  await screen.findByRole("button", { name: "重试原操作" });
  expect((screen.getByRole("button", { name: "停用", exact: true }) as HTMLButtonElement).disabled).toBe(true);
  expect((screen.getByRole("button", { name: "删除", exact: true }) as HTMLButtonElement).disabled).toBe(true);
  expect((screen.getByRole("button", { name: "关闭", exact: true }) as HTMLButtonElement).disabled).toBe(true);
  fireEvent.click(screen.getByRole("button", { name: "重试原操作" }));
  await screen.findByText("synthetic_reset_secret");
  await waitFor(() => expect(calls).toHaveLength(2));
  expect(calls[1].body).toBe(calls[0].body);
  expect(JSON.parse(String(calls[0].body))).toEqual({ version: 3 });
});

it("uses the returned version when saving the same open Key editor again", async () => {
  const versions: number[] = [];
  vi.stubGlobal("fetch", vi.fn(async (path: RequestInfo | URL, init?: RequestInit) => {
    if (init?.method === "PATCH") { const body = JSON.parse(String(init.body)); versions.push(body.version); return response({ ...apiKey, name: body.name, version: body.version + 1 }); }
    return response({ items: String(path).endsWith("/models") ? [] : [apiKey] });
  }));
  show();
  fireEvent.click(await screen.findByRole("button", { name: "编辑配置" }));
  fireEvent.change(screen.getByLabelText("名称", { exact: true }), { target: { value: "第一次调整" } });
  fireEvent.click(screen.getByRole("button", { name: "保存配置", exact: true }));
  await screen.findByText("配置已保存，Key 历史和预算记录继续保留。");
  await waitFor(() => expect((screen.getByRole("button", { name: "保存配置", exact: true }) as HTMLButtonElement).disabled).toBe(false));
  fireEvent.change(screen.getByLabelText("名称", { exact: true }), { target: { value: "第二次调整" } });
  fireEvent.click(screen.getByRole("button", { name: "保存配置", exact: true }));
  await waitFor(() => expect(versions).toEqual([3, 4]));
});

it("refreshes the Key list when a completed creation cannot redisplay its one-time secret", async () => {
  let attempts = 0;
  vi.stubGlobal("fetch", vi.fn(async (path: RequestInfo | URL, init?: RequestInit) => {
    if (init?.method === "POST") {
      attempts++;
      if (attempts === 1) throw new TypeError("response lost after creation");
      return new Response(JSON.stringify({ error: { code: "secret_already_displayed", message: "此 Key 操作已完成，完整密钥只显示一次", request_id: "key-create-replay", retryable: false } }), { status: 409 });
    }
    return response({ items: !String(path).endsWith("/models") && attempts > 0 ? [apiKey] : [] });
  }));
  show();
  fireEvent.click(screen.getByRole("button", { name: "创建 Key", exact: true }));
  fireEvent.change(screen.getByLabelText("名称", { exact: true }), { target: { value: "新应用" } });
  fireEvent.click(screen.getByRole("button", { name: "创建 Key", exact: true }));
  fireEvent.click(await screen.findByRole("button", { name: "重试本次操作" }));
  await screen.findByText(/完整密钥不能再次显示/);
  await screen.findByText("th_prefix…");
  expect(screen.queryByLabelText("名称", { exact: true })).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "返回 Key 列表" }));
  expect((screen.getByRole("button", { name: "重置密钥" }) as HTMLButtonElement).disabled).toBe(false);
  expect(attempts).toBe(2);
});
