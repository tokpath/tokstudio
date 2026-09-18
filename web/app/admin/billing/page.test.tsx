/** @vitest-environment jsdom */
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { withZh } from "@/lib/test-i18n";
import AdminBillingPage from "./page";
vi.mock("@tanstack/react-query", () => ({ useQuery: () => ({ data: { report: { revenue_minor: 2500000, pending_reconciliation_count: 2 } }, refetch: vi.fn() }) }));
vi.mock("./supplier-panel", () => ({ AdminSupplierPanel: () => null }));
afterEach(() => { cleanup(); sessionStorage.clear(); vi.unstubAllGlobals(); });
const recipient = { id: "usr_test", email: "alice@example.test", display_name: "Alice", channel_code: "OFFICIAL", status: "active" };
async function choose() {
  fireEvent.change(screen.getByLabelText("收款用户"), { target: { value: "alice" } });
  fireEvent.click(screen.getByRole("button", { name: "搜索用户" }));
  fireEvent.click(await screen.findByRole("button", { name: /Alice · alice@example.test/ }));
}
it("shows USD and requires selecting an identifiable recipient before granting", async () => {
  const fetcher = vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => ({ ok: true, json: async () => init?.method === "POST" ? {} : { items: [recipient] } }));
  vi.stubGlobal("fetch", fetcher);
  render(withZh(<AdminBillingPage />));
  expect(screen.getByText("$2.50 USD")).toBeTruthy();
  expect((screen.getByRole("button", { name: "赠送额度", exact: true }) as HTMLButtonElement).disabled).toBe(true);
  await choose();
  fireEvent.change(screen.getByLabelText("赠送金额（USD）"), { target: { value: "2.5" } });
  fireEvent.click(screen.getByRole("button", { name: "赠送额度", exact: true }));
  expect(screen.getByText(/alice@example.test，渠道 OFFICIAL.*\$2.50 USD，有效期 24 小时/)).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "确认", exact: true }));
  await screen.findByText(/已向 alice@example.test 赠送/);
  const call = fetcher.mock.calls.find(([, init]) => init?.method === "POST")!;
  expect(JSON.parse(String(call[1]?.body))).toMatchObject({ user_id: "usr_test", amount: 2500000, expires_in_seconds: 86400 });
  expect((call[1]?.headers as Record<string,string>)["Idempotency-Key"]).toBeTruthy();
});
it.each(["-1", "0", "abc", "0.0000001"])("does not submit invalid USD amount %s", async amount => {
  vi.stubGlobal("fetch", vi.fn(async () => ({ ok: true, json: async () => ({ items: [recipient] }) })));
  render(withZh(<AdminBillingPage />)); await choose();
  fireEvent.change(screen.getByLabelText("赠送金额（USD）"), { target: { value: amount } });
  expect((screen.getByRole("button", { name: "赠送额度", exact: true }) as HTMLButtonElement).disabled).toBe(true);
});
it("invalidates selected recipient when the search changes", async () => {
  vi.stubGlobal("fetch", vi.fn(async () => ({ ok: true, json: async () => ({ items: [recipient] }) })));
  render(withZh(<AdminBillingPage />)); await choose();
  fireEvent.change(screen.getByLabelText("收款用户"), { target: { value: "bob" } });
  expect((screen.getByRole("button", { name: "赠送额度", exact: true }) as HTMLButtonElement).disabled).toBe(true);
  expect(screen.queryByRole("button", { name: /Alice ·/ })).toBeNull();
});
it("retains an uncertain operation across remount and reuses its idempotency key", async () => {
  const requests: RequestInit[] = [];
  let fail = true;
  vi.stubGlobal("fetch", vi.fn(async (_input, init) => {
    if (init?.method === "POST") { requests.push(init); if (fail) throw new Error("lost response"); }
    return { ok: true, json: async () => ({ items: [recipient] }) };
  }));
  const first = render(withZh(<AdminBillingPage />)); await choose();
  fireEvent.click(screen.getByRole("button", { name: "赠送额度", exact: true }));
  fireEvent.click(screen.getByRole("button", { name: "确认", exact: true }));
  await screen.findByText(/暂未确认发放结果/);
  first.unmount(); render(withZh(<AdminBillingPage />));
  fireEvent.click(await screen.findByRole("button", { name: "核对原操作" }));
  fail = false;
  fireEvent.click(screen.getByRole("button", { name: "确认", exact: true }));
  await waitFor(() => expect(requests).toHaveLength(2));
  expect(requests[0].headers).toEqual(requests[1].headers);
  expect(requests[0].body).toEqual(requests[1].body);
  await screen.findByText(/已向 alice@example.test 赠送/);
});
