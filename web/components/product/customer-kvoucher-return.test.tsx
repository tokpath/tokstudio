// @vitest-environment jsdom
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import type { ReactNode } from "react";
import type { Order } from "@/lib/contracts";
import { markPaymentOpened } from "@/lib/payment-window";
import { PaymentAction } from "./customer-plans";

vi.mock("./context", () => ({ useProduct: () => ({ session: { kind: "customer" }, context: { payment_methods: [] } }) }));

const order = { id: "order-one", status: "pending", kind: "package", operator_name: "TokenHub 平台", plan_name: "测试套餐", amount_money: "80", paid_money: "0", created_at: "2026-10-29T12:00:00Z", payment_action: { kind: "form" as const, gkash_opened: false, currency: "MYR", amount: "40.02", instructions: "请在 Gkash 完成付款。" } };
const paid = { ...order, status: "completed", paid_money: "80", delivered_at: "2026-10-29T12:05:00Z" };
const response = (body: unknown) => new Response(JSON.stringify(body), { status: 200 });

function mount(value: Order = order) {
  const cache = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const wrapper = ({ children }: { children: ReactNode }) => <QueryClientProvider client={cache}>{children}</QueryClientProvider>;
  return render(<PaymentAction order={value} />, { wrapper });
}

afterEach(() => { cleanup(); sessionStorage.clear(); vi.unstubAllGlobals(); vi.useRealTimers(); vi.restoreAllMocks(); });

it("leaves a KVoucher order unpaid until the payment page has been opened", async () => {
  const fetch = vi.fn(async () => response(order));
  vi.stubGlobal("fetch", fetch);
  mount();
  await screen.findByText("请在 Gkash 完成付款。");
  await waitFor(() => expect(fetch).toHaveBeenCalled());
  expect(fetch.mock.calls.some(call => call[1]?.method === "POST")).toBe(false);
});

it("refreshes the saved order after payment opens without polling the provider from the browser", async () => {
  vi.useFakeTimers();
  markPaymentOpened(order.id);
  let reads = 0;
  const fetch = vi.fn(async (path: RequestInfo | URL, init?: RequestInit) => {
    expect(String(path)).toBe("/api/v1/me/orders/order-one");
    expect(init?.method).toBe("GET");
    return response(++reads > 1 ? paid : order);
  });
  vi.stubGlobal("fetch", fetch);
  await act(async () => { mount(); });
  await act(async () => { await vi.advanceTimersByTimeAsync(5100); });
  expect(screen.getByText("服务额度已到账，可以开始使用。")).toBeTruthy();
  const completedReads = reads;
  await act(async () => { await vi.advanceTimersByTimeAsync(10000); });
  expect(reads).toBe(completedReads);
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
  expect(screen.queryByRole("button", { name: "我已付款，核实原通道", exact: true })).toBeNull();
  expect(document.querySelector("form[action]" )).toBeNull();
  await waitFor(() => expect(fetch).toHaveBeenCalled());
  expect(fetch.mock.calls.some(call => call[1]?.method === "POST")).toBe(false);
});

it("shows an unpaid channel result as a normal status after an explicit check", async () => {
  const message = "原通道尚未确认收款。未付款可继续原订单付款；已经付款请稍后刷新原订单，无须再次付款。";
  const fetch = vi.fn(async (_path: RequestInfo | URL, init?: RequestInit) => response(init?.method === "POST" ? { id: order.id, status: "pending_payment", message } : order));
  vi.stubGlobal("fetch", fetch);
  mount();
  fireEvent.click(screen.getByRole("button", { name: "我已付款，核实原通道", exact: true }));
  expect(await screen.findByText(message)).toBeTruthy();
  expect(fetch.mock.calls.filter(call => call[1]?.method === "POST")).toHaveLength(1);
  expect(screen.queryByText(/payment_not_final/)).toBeNull();
});

it("reopens Gkash on a new browser using the order's generation and only posts the newly issued form", async () => {
  const opened = {
    ...order,
    payment_action: {
      ...order.payment_action, gkash_opened: true, gkash_generation: 4,
      url: "https://payments.example.test/checkout", fields: { v_cartid: "stale-cart" },
    },
  };
  const fetch = vi.fn(async (_path: RequestInfo | URL, init?: RequestInit) => response(init?.method === "POST" ? {
    id: opened.id, status: "payment_ready", gkash_generation: 5,
    payment_action: { kind: "form", url: "https://payments.example.test/checkout", fields: { v_cartid: "new-cart" } },
  } : opened));
  vi.stubGlobal("fetch", fetch);
  const popup = { closed: false, focus: vi.fn(), close: vi.fn() };
  const open = vi.spyOn(window, "open").mockReturnValue(popup as unknown as Window);
  const submitted: FormData[] = [];
  vi.spyOn(HTMLFormElement.prototype, "submit").mockImplementation(function (this: HTMLFormElement) { submitted.push(new FormData(this)); });
  mount(opened);
  expect(screen.queryByRole("button", { name: "前往 GKash 付款" })).toBeNull();
  const reopen = screen.getByRole("button", { name: "重新打开 GKash 付款", exact: true });
  act(() => { fireEvent.click(reopen); fireEvent.click(reopen); });
  await waitFor(() => expect(submitted.map(form => form.get("v_cartid"))).toEqual(["new-cart"]));
  const posts = fetch.mock.calls.filter(([, init]) => init?.method === "POST");
  expect(posts).toHaveLength(1);
  expect(posts[0][0]).toBe("/api/v1/me/orders/order-one/payment/open");
  expect(JSON.parse(String(posts[0][1]?.body))).toEqual({ expected_generation: 4 });
  expect(open).toHaveBeenCalledTimes(1);
  popup.closed = true;
});

it.each([
  ["payment_not_final", "原通道尚未确认这笔收款。请稍后再查询，不要另付一笔。"],
  ["payment_outcome_unknown", "原支付结果尚待核实，请查询原交易，重试将沿用原业务编号"],
  ["payment_closed", "原通道已关闭本次付款，未收到款项。"],
  ["payment_checkout_cart_reused", "原通道暂未签发新的付款页。请稍后重试或核实原付款状态。"],
])("closes the empty window and shows %s after a declined reopen", async (code, message) => {
  const opened = { ...order, id: `reopen-${code}`, payment_action: { ...order.payment_action, gkash_opened: true, gkash_generation: 2 } };
  const fetch = vi.fn(async (_path: RequestInfo | URL, init?: RequestInit) => init?.method === "POST"
    ? Response.json({ error: { code, message, request_id: "test-request", retryable: false } }, { status: 409 })
    : response(opened));
  vi.stubGlobal("fetch", fetch);
  const popup = { closed: false, close: vi.fn() };
  vi.spyOn(window, "open").mockReturnValue(popup as unknown as Window);
  const submit = vi.spyOn(HTMLFormElement.prototype, "submit");
  mount(opened);
  fireEvent.click(screen.getByRole("button", { name: "重新打开 GKash 付款", exact: true }));
  expect((await screen.findByRole("alert")).textContent).toContain(message);
  expect(popup.close).toHaveBeenCalledTimes(1);
  expect(submit).not.toHaveBeenCalled();
  expect(fetch.mock.calls.filter(([, init]) => init?.method === "POST")).toHaveLength(1);
});

it.each(["partially_paid", "paid_pending_delivery", "completed"])("does not open a stale Gkash form for a %s order", status => {
  const received = { ...order, status, paid_money: status === "partially_paid" ? "1" : "80" };
  vi.stubGlobal("fetch", vi.fn(async () => response(received)));
  mount(received);
  expect(screen.queryByRole("button", { name: "前往 GKash 付款" })).toBeNull();
  expect(screen.queryByRole("button", { name: "重新打开 GKash 付款" })).toBeNull();
});

it("keeps cancellation available when the provider definitively failed before payment", async () => {
  const failed: Order = { ...order, payment_action: undefined, payment_state: "failed", cancel_allowed: true, payment_notice: "订单已建立，但付款指引暂时不可用。请保留原订单并联系经营方核实。" };
  vi.stubGlobal("fetch", vi.fn(async () => response(failed)));
  mount(failed);
  expect(await screen.findByText(failed.payment_notice!)).toBeTruthy();
  expect(screen.getByRole("button", { name: "取消订单", exact: true })).toBeTruthy();
  expect(screen.queryByText("完成本订单付款")).toBeNull();
});
