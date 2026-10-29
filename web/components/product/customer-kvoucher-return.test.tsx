// @vitest-environment jsdom
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import type { ReactNode } from "react";
import { markPaymentOpened } from "@/lib/payment-window";
import { PaymentAction } from "./customer-plans";

vi.mock("./context", () => ({ useProduct: () => ({ session: { kind: "customer" }, context: { payment_methods: [] } }) }));

const order = { id: "order-one", status: "pending", kind: "package", operator_name: "TokenHub 平台", plan_name: "测试套餐", amount_money: "80", paid_money: "0", created_at: "2026-10-29T12:00:00Z", payment_action: { kind: "form" as const, url: "https://api.gkash.my/api/PaymentForm.aspx", currency: "MYR", amount: "40.02", fields: { CID: "merchant", v_amount: "40.02" }, instructions: "请在 Gkash 完成付款。" } };
const paid = { ...order, status: "completed", paid_money: "80", delivered_at: "2026-10-29T12:05:00Z" };
const response = (body: unknown) => new Response(JSON.stringify(body), { status: 200 });

function mount() {
  const cache = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const wrapper = ({ children }: { children: ReactNode }) => <QueryClientProvider client={cache}>{children}</QueryClientProvider>;
  return render(<PaymentAction order={order} />, { wrapper });
}

afterEach(() => { cleanup(); sessionStorage.clear(); vi.unstubAllGlobals(); });

it("leaves a KVoucher order unpaid until the payment page has been opened", async () => {
  const fetch = vi.fn(async () => response(order));
  vi.stubGlobal("fetch", fetch);
  mount();
  await screen.findByText("请在 Gkash 完成付款。");
  await waitFor(() => expect(fetch).toHaveBeenCalled());
  expect(fetch.mock.calls.some(call => call[1]?.method === "POST")).toBe(false);
});

it("asks the original channel again after the payment page was opened", async () => {
  markPaymentOpened(order.id);
  let asked = false;
  const fetch = vi.fn(async (path: RequestInfo | URL, init?: RequestInit) => {
    if (init?.method === "POST") {
      asked = true;
      expect(String(path)).toBe("/api/v1/me/orders/order-one/reconcile");
      return response({ id: order.id, status: "reconciliation_queued" });
    }
    return response(asked ? paid : order);
  });
  vi.stubGlobal("fetch", fetch);
  mount();
  await screen.findByText("服务额度已到账，可以开始使用。");
});
