// @vitest-environment jsdom
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import type { ReactNode } from "react";
import type { Order } from "@/lib/contracts";
import { markPaymentOpened } from "@/lib/payment-window";
import { PaymentAction } from "./customer-plans";

vi.mock("./context", () => ({ useProduct: () => ({ session: { kind: "customer" }, context: { payment_methods: [] } }) }));

const order = { id: "order-one", status: "pending", kind: "package", operator_name: "TokenHub 平台", plan_name: "测试套餐", amount_money: "80", paid_money: "0", created_at: "2026-10-29T12:00:00Z", payment_action: { kind: "form" as const, url: "https://api.gkash.my/api/PaymentForm.aspx", currency: "MYR", amount: "40.02", fields: { CID: "merchant", v_amount: "40.02" }, instructions: "请在 Gkash 完成付款。" } };
const paid = { ...order, status: "completed", paid_money: "80", delivered_at: "2026-10-29T12:05:00Z" };
const response = (body: unknown) => new Response(JSON.stringify(body), { status: 200 });

function mount(value: Order = order) {
  const cache = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const wrapper = ({ children }: { children: ReactNode }) => <QueryClientProvider client={cache}>{children}</QueryClientProvider>;
  return render(<PaymentAction order={value} />, { wrapper });
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

it("shows the original unpaid order and checkout failure without inviting payment", async () => {
  const blocked: Order = { ...order, kind: "topup", plan_name: undefined, payment_action: undefined, payment_state: "outcome_unknown", cancel_allowed: false, payment_notice: "支付通道返回的币种或金额与本单报价不一致，付款已暂停。请保留原订单并联系经营方核实。" };
  const fetch = vi.fn(async (_path: RequestInfo | URL, _init?: RequestInit) => response(blocked));
  vi.stubGlobal("fetch", fetch);
  mount(blocked);
  expect(await screen.findByText(blocked.payment_notice!)).toBeTruthy();
  expect(screen.getByText(blocked.id)).toBeTruthy();
  expect(screen.queryByText("完成本订单付款")).toBeNull();
  expect(screen.getByRole("link", { name: "充值记录", exact: true }).getAttribute("href")).toBe("/workspace/topups");
  expect(screen.queryByRole("button", { name: "取消订单", exact: true })).toBeNull();
  expect(document.querySelector("form[action]" )).toBeNull();
  await waitFor(() => expect(fetch).toHaveBeenCalled());
  expect(fetch.mock.calls.some(call => call[1]?.method === "POST")).toBe(false);
});

it("keeps cancellation available when the provider definitively failed before payment", async () => {
  const failed: Order = { ...order, payment_action: undefined, payment_state: "failed", cancel_allowed: true, payment_notice: "订单已建立，但付款指引暂时不可用。请保留原订单并联系经营方核实。" };
  vi.stubGlobal("fetch", vi.fn(async () => response(failed)));
  mount(failed);
  expect(await screen.findByText(failed.payment_notice!)).toBeTruthy();
  expect(screen.getByRole("button", { name: "取消订单", exact: true })).toBeTruthy();
  expect(screen.queryByText("完成本订单付款")).toBeNull();
});
