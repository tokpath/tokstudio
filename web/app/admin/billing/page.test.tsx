/** @vitest-environment jsdom */
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { withZh } from "@/lib/test-i18n";
import AdminBillingPage from "./page";
vi.mock("@tanstack/react-query", () => ({ useQuery: () => ({ data: { report: { revenue_minor: 2500000, pending_reconciliation_count: 2 } }, refetch: vi.fn() }) }));
vi.mock("./supplier-panel", () => ({ AdminSupplierPanel: () => null }));
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });
it("shows USD instead of minor units and includes expiry before granting", async () => {
  const fetcher = vi.fn(async (_input: RequestInfo | URL, _init?: RequestInit) => ({ ok: true, json: async () => ({}) }));
  vi.stubGlobal("fetch", fetcher);
  render(withZh(<AdminBillingPage />));
  expect(screen.getByText("$2.50 USD")).toBeTruthy();
  expect(screen.queryByText(/revenue_minor/)).toBeNull();
  expect((screen.getByRole("button", { name: "赠送额度" }) as HTMLButtonElement).disabled).toBe(true);
  fireEvent.change(screen.getByLabelText("用户 ID"), { target: { value: "usr_test" } });
  fireEvent.change(screen.getByLabelText("赠送金额（USD）"), { target: { value: "2.5" } });
  fireEvent.click(screen.getByRole("button", { name: "赠送额度" }));
  expect(screen.getByText(/向用户 usr_test 赠送 \$2.50 USD，有效期 24 小时/)).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "确认", exact: true }));
  await waitFor(() => expect(fetcher).toHaveBeenCalled());
  const body = JSON.parse(String(fetcher.mock.calls[0][1]?.body));
  expect(body).toMatchObject({ user_id: "usr_test", amount: 2500000, expires_in_seconds: 86400 });
});
it.each(["-1", "0", "abc", "0.0000001"])("does not submit invalid USD amount %s", (amount) => {
  render(withZh(<AdminBillingPage />));
  fireEvent.change(screen.getByLabelText("用户 ID"), { target: { value: "usr_test" } });
  fireEvent.change(screen.getByLabelText("赠送金额（USD）"), { target: { value: amount } });
  expect((screen.getByRole("button", { name: "赠送额度" }) as HTMLButtonElement).disabled).toBe(true);
});
