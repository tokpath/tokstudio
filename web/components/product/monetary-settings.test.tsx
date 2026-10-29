// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import type { Monetary } from "@/lib/contracts";
import { MonetaryProvider } from "./monetary";
import { MonetarySettings } from "./monetary-settings";
import { Money } from "./shared";

afterEach(() => { cleanup(); vi.unstubAllGlobals(); });
const configured: Monetary = { currency: "CNY", atg_per_unit: "10.25", status: "locked", version: 7, effective_at: "2026-01-01T00:00:00Z" };
function show(monetary: Monetary, canManage = false) {
  return render(<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })}><MonetaryProvider value={monetary}><MonetarySettings canManage={canManage} /><Money value="12.000000000000000001" unit="MONEY" /><Money value="4" unit="EUR" /></MonetaryProvider></QueryClientProvider>);
}
it("uses the configured currency while preserving explicit original currencies and read-only access", () => {
  const fetch = vi.fn(); vi.stubGlobal("fetch", fetch);
  show(configured);
  expect(screen.getByLabelText("12.000000000000000001 CNY")).toBeTruthy();
  expect(screen.getByLabelText("4 EUR")).toBeTruthy();
  expect(screen.queryByRole("textbox")).toBeNull();
  expect(fetch).not.toHaveBeenCalled();
});
it("keeps effective settings permanently locked even for the platform owner", async () => {
  vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({ ...configured, status: "scheduled" }))));
  show(configured, true);
  await screen.findByText("已生效并永久锁定。暂停营业不会解锁。");
  expect(screen.queryByRole("textbox")).toBeNull();
  expect(screen.queryByRole("button", { name: "取消预约并重新设置" })).toBeNull();
});
it("does not present unconfigured defaults as an effective monetary rule", () => {
  show({ currency: "USD", atg_per_unit: "1", status: "unconfigured", version: 0 });
  expect(screen.getByText("营业前请选择全系统的结算币种和 ATG 基础比例。")).toBeTruthy();
  expect(screen.queryByText("全系统使用同一结算币种。每 1 USD 对应 1 ATG。")).toBeNull();
});
it("requires a separate risk confirmation, then permits cancellation before activation", async () => {
  let current: Monetary = { currency: "USD", atg_per_unit: "1", status: "unconfigured", version: 7 };
  const posts: { path: string; body: Record<string, unknown> }[] = [];
  vi.stubGlobal("fetch", vi.fn(async (path: string, init?: RequestInit) => {
    if (init?.method === "POST") {
      const body = JSON.parse(String(init.body)); posts.push({ path, body });
      current = path.endsWith("/cancel") ? { ...current, status: "unconfigured", version: 9 } : { ...current, ...body, status: "scheduled", version: 8 };
      return new Response(JSON.stringify({ id: "monetary", status: "saved", version: current.version }));
    }
    return new Response(JSON.stringify(current));
  }));
  show(current, true);
  await waitFor(() => expect((screen.getByRole("button", { name: "核对并查看风险说明" }) as HTMLButtonElement).disabled).toBe(false));
  fireEvent.change(screen.getByLabelText("结算币种（ISO 代码）"), { target: { value: "CNY" } });
  fireEvent.change(screen.getByLabelText("每 1 单位结算币种购买的 ATG"), { target: { value: "10.25" } });
  fireEvent.change(screen.getByLabelText("生效时间"), { target: { value: "2099-01-01T09:00" } });
  fireEvent.click(screen.getByRole("button", { name: "核对并查看风险说明" }));
  expect(posts).toHaveLength(0);
  expect((screen.getByRole("button", { name: "确认预约生效" }) as HTMLButtonElement).disabled).toBe(true);
  fireEvent.click(screen.getByRole("checkbox", { name: "我已核对并理解永久锁定及换算规则" }));
  fireEvent.click(screen.getByRole("button", { name: "确认预约生效" }));
  await screen.findByRole("button", { name: "取消预约并重新设置" });
  expect(posts[0].body).toMatchObject({ currency: "CNY", atg_per_unit: "10.25", expected_version: 7, confirmed: true });
  expect(new Date(String(posts[0].body.effective_at)).toISOString()).toBe(new Date("2099-01-01T09:00").toISOString());
  fireEvent.click(screen.getByRole("button", { name: "取消预约并重新设置" }));
  await screen.findByRole("button", { name: "核对并查看风险说明" });
  expect(posts[1].body).toEqual({ expected_version: 8 });
});
it("retries an uncertain configuration with its original reviewed settings and idempotency key", async () => {
  const current: Monetary = { currency: "USD", atg_per_unit: "1", status: "unconfigured", version: 7 };
  const posts: RequestInit[] = [];
  vi.stubGlobal("fetch", vi.fn(async (_path: string, init?: RequestInit) => {
    if (init?.method === "POST") {
      posts.push(init);
      if (posts.length === 1) throw new TypeError("connection interrupted");
      return new Response(JSON.stringify({ id: "monetary", status: "saved", version: 8 }));
    }
    return new Response(JSON.stringify(current));
  }));
  show(current, true);
  await waitFor(() => expect((screen.getByRole("button", { name: "核对并查看风险说明" }) as HTMLButtonElement).disabled).toBe(false));
  fireEvent.change(screen.getByLabelText("生效时间"), { target: { value: "2099-01-01T09:00" } });
  fireEvent.click(screen.getByRole("button", { name: "核对并查看风险说明" }));
  fireEvent.click(screen.getByRole("checkbox", { name: "我已核对并理解永久锁定及换算规则" }));
  fireEvent.click(screen.getByRole("button", { name: "确认预约生效" }));
  fireEvent.click(await screen.findByRole("button", { name: "重试原操作" }));
  await waitFor(() => expect(posts).toHaveLength(2));
  expect(posts[1].body).toBe(posts[0].body);
  expect(new Headers(posts[1].headers).get("Idempotency-Key")).toBe(new Headers(posts[0].headers).get("Idempotency-Key"));
});
