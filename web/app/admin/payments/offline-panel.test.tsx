// @vitest-environment jsdom
import { afterEach, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { OfflineReceiptPanel } from "./offline-panel";
import { withZh } from "@/lib/test-i18n";
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });
const customer = { id: "u1", email: "alice@example.test", display_name: "Alice", channel_code: "OEM-B" };
async function fill() {
  fireEvent.click(screen.getByRole("button", { name: "线下收款划拨" }));
  fireEvent.change(screen.getByLabelText("查找划拨客户"), { target: { value: "alice" } });
  fireEvent.click(screen.getByRole("button", { name: "查找客户" }));
  fireEvent.click(await screen.findByRole("button", { name: /Alice.*alice@example.test/ }));
  fireEvent.change(screen.getByLabelText("实收金额"), { target: { value: "100.25" } });
  fireEvent.change(screen.getByLabelText("发放额度（USD）"), { target: { value: "12.5" } });
  fireEvent.change(screen.getByLabelText("收款凭证"), { target: { value: "bank-001" } });
  fireEvent.click(screen.getByRole("button", { name: "核对并划拨" }));
}
it("records actual money and permanent credits under the OEM, using the selected customer", async () => {
  const fetcher = vi.fn(async (_url: unknown, init?: RequestInit) => ({ ok: true, json: async () => init?.method === "POST" ? { item: { id: "pay_1", fulfilled_at: "2026-10-09" } } : { items: [customer] } }));
  vi.stubGlobal("fetch", fetcher);
  const refreshed = vi.fn();
  render(withZh(<OfflineReceiptPanel basePath="/channel/payments" onRecorded={refreshed} />));
  await fill();
  const dialog = screen.getByRole("dialog");
  expect(within(dialog).getByText(/alice@example.test.*100.25 CNY.*12.50.*长期有效.*bank-001/)).toBeTruthy();
  fireEvent.click(within(dialog).getByRole("button", { name: "确认" }));
  await screen.findByText(/已为 alice@example.test 划拨/);
  const call = fetcher.mock.calls.find(([, init]) => init?.method === "POST")!;
  expect(String(call[0])).toContain("/channel/payments/offline");
  expect(JSON.parse(call[1]?.body as string)).toEqual({ user_id: "u1", amount_minor: 10025, credit_minor: 12500000, currency: "CNY", reference: "bank-001" });
  expect(refreshed).toHaveBeenCalledOnce();
});
it("keeps a failed allocation and its original receipt ready for a safe retry", async () => {
  vi.stubGlobal("fetch", vi.fn(async (_url, init) => ({ ok: init?.method !== "POST", json: async () => init?.method === "POST" ? { error: { message: "OEM 服务额度不足，划拨未完成" } } : { items: [customer] } })));
  const refreshed = vi.fn();
  render(withZh(<OfflineReceiptPanel basePath="/channel/payments" onRecorded={refreshed} />));
  await fill(); fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "确认" }));
  expect(await within(screen.getByRole("dialog")).findByText("OEM 服务额度不足，划拨未完成")).toBeTruthy();
  expect(within(screen.getByRole("dialog")).getByText(/bank-001/)).toBeTruthy();
  expect(refreshed).not.toHaveBeenCalled();
});
