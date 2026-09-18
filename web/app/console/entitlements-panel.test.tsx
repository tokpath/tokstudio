/** @vitest-environment jsdom */
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { withZh } from "@/lib/test-i18n";
import { EntitlementsPanel } from "./entitlements-panel";
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });
it("shows actual amount and expiry instead of counting expired active rows as usable", async () => {
  vi.stubGlobal("fetch", vi.fn(async () => ({ ok: true, json: async () => ({ items: [
    { id: "a", source_type: "bonus", unit_type: "usd_credit", granted: 2500000, remaining: 1500000, status: "active", expires_at: "2099-01-01T00:00:00Z" },
    { id: "b", source_type: "bonus", unit_type: "usd_credit", granted: 3000000, remaining: 3000000, status: "active", expires_at: "2020-01-01T00:00:00Z" },
  ] }) })));
  render(withZh(<EntitlementsPanel />));
  expect(await screen.findByText("当前可用：$1.50 USD")).toBeTruthy();
  expect(screen.getByText("原始发放：$2.50 USD")).toBeTruthy();
  expect(screen.getByText("当前可用：$0.00 USD")).toBeTruthy();
  expect(screen.getByText(/限时赠送额度 · 已到期/)).toBeTruthy();
  expect(screen.getAllByText(/到期时间：/)).toHaveLength(2);
});
it("a failed load is not an empty account and can recover", async () => {
  const fetcher = vi.fn().mockResolvedValueOnce({ ok: false, json: async () => ({}) }).mockResolvedValueOnce({ ok: true, json: async () => ({ items: [] }) });
  vi.stubGlobal("fetch", fetcher);
  render(withZh(<EntitlementsPanel />));
  await screen.findByRole("alert");
  expect(screen.queryByText("暂无赠送或套餐额度。")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "刷新额度" }));
  expect(await screen.findByText("暂无赠送或套餐额度。")).toBeTruthy();
});
