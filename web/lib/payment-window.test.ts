// @vitest-environment jsdom
import { afterEach, expect, it, vi } from "vitest";
import type { Order } from "./contracts";
import { customerGkashOpenPath, focusPaymentWindow, markPaymentOpened, openGkashPayment, paymentWindowState, paymentWasOpened, reservePaymentWindow, shouldRefreshPayment } from "./payment-window";

afterEach(() => { sessionStorage.clear(); vi.useRealTimers(); vi.restoreAllMocks(); vi.unstubAllGlobals(); });

function fakeWindow() {
  const popup = { closed: false, focus: vi.fn(), close: vi.fn(() => { popup.closed = true; }) };
  return popup;
}

function paymentReady(id: string, cartID: string, generation = 1) {
  return Response.json({
    id,
    status: "payment_ready",
    gkash_generation: generation,
    payment_action: { kind: "form", url: "https://payments.example.test/checkout", fields: { v_cartid: cartID } },
  });
}

it("returns to the original tab and uses the issued generation to reopen after closure", async () => {
  const popup = fakeWindow(), retryPopup = fakeWindow();
  const order = { id: "ready-order", payment_action: { kind: "form" as const, gkash_generation: 0 } };
  const path = customerGkashOpenPath(order.id);
  const open = vi.spyOn(window, "open")
    .mockReturnValueOnce(popup as unknown as Window)
    .mockReturnValueOnce(retryPopup as unknown as Window);
  const fetch = vi.fn().mockResolvedValueOnce(paymentReady(order.id, "fresh-cart")).mockResolvedValueOnce(paymentReady(order.id, "retry-cart", 2));
  vi.stubGlobal("fetch", fetch);
  const submitted: HTMLFormElement[] = [];
  vi.spyOn(HTMLFormElement.prototype, "submit").mockImplementation(function (this: HTMLFormElement) { submitted.push(this); });
  const target = reservePaymentWindow();
  expect(open).toHaveBeenCalledWith("about:blank", expect.stringMatching(/^tokpath-gkash-/));
  await openGkashPayment(order, path, target);
  expect(fetch).toHaveBeenCalledWith(path, expect.objectContaining({ method: "POST", body: '{"expected_generation":0}' }));
  expect(submitted).toHaveLength(1);
  expect(submitted[0].target).toBe(open.mock.calls[0][1]);
  expect(submitted[0].method.toLowerCase()).toBe("post");
  expect(submitted[0].action).toBe("https://payments.example.test/checkout");
  expect(new FormData(submitted[0]).get("v_cartid")).toBe("fresh-cart");
  expect(document.querySelector("form")).toBeNull();
  expect(paymentWindowState(order)).toBe("open");
  expect(focusPaymentWindow(order.id)).toBe(true);
  expect(popup.focus).toHaveBeenCalledTimes(1);
  expect(focusPaymentWindow("unrelated-order")).toBe(false);
  expect(fetch).toHaveBeenCalledTimes(1);
  popup.close();
  expect(paymentWindowState(order)).toBe("opened");

  await openGkashPayment(order, path);
  expect(fetch).toHaveBeenLastCalledWith(path, expect.objectContaining({ body: '{"expected_generation":1}' }));
  expect((fetch.mock.calls[0][1].headers as Headers).get("Idempotency-Key")).not.toBe((fetch.mock.calls[1][1].headers as Headers).get("Idempotency-Key"));
  expect(new FormData(submitted[1]).get("v_cartid")).toBe("retry-cart");
  expect(submitted[1].target).toBe(open.mock.calls[1][1]);
  expect(submitted[1].target).not.toBe(submitted[0].target);
  // Another browser advanced the generation, but it may have failed to sign.
  const advanced = { ...order, payment_action: { kind: "form" as const, gkash_generation: 3 } };
  await openGkashPayment(advanced, path);
  expect(fetch).toHaveBeenCalledTimes(2);
  expect(paymentWindowState(advanced)).toBe("open");
  expect(retryPopup.focus).toHaveBeenCalledTimes(1);
  retryPopup.close();
});

it("posts two orders into independent windows and only opens one form for concurrent clicks on the same order", async () => {
  const first = fakeWindow(), duplicate = fakeWindow(), second = fakeWindow();
  const open = vi.spyOn(window, "open")
    .mockReturnValueOnce(first as unknown as Window)
    .mockReturnValueOnce(duplicate as unknown as Window)
    .mockReturnValueOnce(second as unknown as Window);
  const fetch = vi.fn().mockResolvedValueOnce(paymentReady("first-order", "first-cart")).mockResolvedValueOnce(paymentReady("second-order", "second-cart"));
  vi.stubGlobal("fetch", fetch);
  const submitted: HTMLFormElement[] = [];
  vi.spyOn(HTMLFormElement.prototype, "submit").mockImplementation(function (this: HTMLFormElement) { submitted.push(this); });
  const pending = openGkashPayment({ id: "first-order" }, customerGkashOpenPath("first-order"), reservePaymentWindow());
  await openGkashPayment({ id: "first-order" }, customerGkashOpenPath("first-order"), reservePaymentWindow());
  await pending;
  await openGkashPayment({ id: "second-order" }, customerGkashOpenPath("second-order"), reservePaymentWindow());
  expect(fetch).toHaveBeenCalledTimes(2);
  expect(duplicate.close).toHaveBeenCalledTimes(1);
  expect(submitted.map(form => form.target)).toEqual([open.mock.calls[0][1], open.mock.calls[2][1]]);
  expect(submitted[0].target).not.toBe(submitted[1].target);
  expect(focusPaymentWindow("first-order")).toBe(true);
  expect(first.focus).toHaveBeenCalledTimes(1);
  expect(second.focus).not.toHaveBeenCalled();
  first.close();
  second.close();
});

it("closes a reserved popup when the original payment page was already issued", async () => {
  const popup = fakeWindow();
  vi.spyOn(window, "open").mockReturnValue(popup as unknown as Window);
  vi.stubGlobal("fetch", vi.fn().mockResolvedValue(Response.json({ id: "existing-order", status: "payment_already_opened", gkash_generation: 1 })));
  const submit = vi.spyOn(HTMLFormElement.prototype, "submit");
  const message = await openGkashPayment({ id: "existing-order" }, customerGkashOpenPath("existing-order"), reservePaymentWindow());
  expect(message).toBe("原 Gkash 付款页已打开过。请核实原付款状态。");
  expect(submit).not.toHaveBeenCalled();
  expect(popup.close).toHaveBeenCalledTimes(1);
  expect(paymentWindowState({ id: "existing-order" })).toBe("opened");
});

it("preserves the first server claim when the browser blocks the popup", async () => {
  vi.spyOn(window, "open").mockReturnValue(null);
  const fetch = vi.fn();
  vi.stubGlobal("fetch", fetch);
  await expect(openGkashPayment({ id: "blocked-order" }, customerGkashOpenPath("blocked-order"))).rejects.toThrow("付款窗口未能打开");
  expect(fetch).not.toHaveBeenCalled();
  expect(paymentWindowState({ id: "blocked-order" })).toBe("unopened");
});

it("does not submit a Gkash form that arrives after the order's deadline", async () => {
  vi.useFakeTimers();
  const order = { id: "rollover-order", expires_at: new Date(Date.now() + 1_000).toISOString() };
  const popup = fakeWindow();
  vi.spyOn(window, "open").mockReturnValue(popup as unknown as Window);
  vi.stubGlobal("fetch", () => new Promise<Response>(resolve => {
    setTimeout(() => resolve(paymentReady(order.id, "rollover-cart")), 1_001);
  }));
  const submit = vi.spyOn(HTMLFormElement.prototype, "submit");
  const outcome = expect(openGkashPayment(order, customerGkashOpenPath(order.id))).rejects.toThrow("付款期限已过");
  await vi.advanceTimersByTimeAsync(1_001);
  await outcome;
  expect(popup.close).toHaveBeenCalledTimes(1);
  expect(submit).not.toHaveBeenCalled();
});

it("keeps the payment window reserved while the server verifies and signs on a slow connection", async () => {
  vi.useFakeTimers();
  const order = { id: "slow-order" };
  const popup = fakeWindow();
  vi.spyOn(window, "open").mockReturnValue(popup as unknown as Window);
  vi.stubGlobal("fetch", (_path: string, options: RequestInit) => new Promise<Response>((resolve, reject) => {
    options.signal?.addEventListener("abort", () => reject(new DOMException("Aborted", "AbortError")));
    setTimeout(() => resolve(paymentReady(order.id, "slow-cart")), 32_000);
  }));
  const submit = vi.spyOn(HTMLFormElement.prototype, "submit").mockImplementation(() => {});
  const open = openGkashPayment(order, customerGkashOpenPath(order.id)).catch(error => error);
  await vi.advanceTimersByTimeAsync(32_000);
  expect(await open).toBeUndefined();
  expect(submit).toHaveBeenCalledTimes(1);
  expect(popup.close).not.toHaveBeenCalled();
  popup.close();
});

it.each(["closed", "focus"])("requests a new form when the window proxy denies %s access", async denied => {
  const order = { id: `policy-${denied}` };
  const path = customerGkashOpenPath(order.id);
  const inaccessible = fakeWindow(), replacement = fakeWindow();
  vi.spyOn(window, "open")
    .mockReturnValueOnce(inaccessible as unknown as Window)
    .mockReturnValueOnce(replacement as unknown as Window);
  const fetch = vi.fn().mockResolvedValueOnce(paymentReady(order.id, "original")).mockResolvedValueOnce(paymentReady(order.id, "replacement", 2));
  vi.stubGlobal("fetch", fetch);
  const submit = vi.spyOn(HTMLFormElement.prototype, "submit").mockImplementation(() => {});
  await openGkashPayment(order, path);
  const error = new DOMException("Window proxy policy", "SecurityError");
  if (denied === "closed") Object.defineProperty(inaccessible, "closed", { get() { throw error; } });
  else inaccessible.focus.mockImplementation(() => { throw error; });

  await openGkashPayment(order, path);
  expect(fetch).toHaveBeenLastCalledWith(path, expect.objectContaining({ body: '{"expected_generation":1}' }));
  expect(submit).toHaveBeenCalledTimes(2);
  replacement.close();
});

it("remembers which order already opened the payment page", () => {
  expect(paymentWasOpened("order-one")).toBe(false);
  markPaymentOpened("order-one");
  expect(paymentWasOpened("order-one")).toBe(true);
  expect(paymentWasOpened("order-two")).toBe(false);
});

it("stops refreshing unusable, unopened, expired and completed checkouts", () => {
  const now = Date.now();
  const order: Order = { id: "one", kind: "topup", status: "pending", payment_state: "created", operator_name: "Test", amount_money: "2", paid_money: "0", created_at: new Date(now).toISOString(), expires_at: new Date(now + 60000).toISOString(), payment_action: { kind: "form" } };
  expect(shouldRefreshPayment(order, false, now)).toBe(false);
  expect(shouldRefreshPayment(order, true, now)).toBe(true);
  for (const payment_state of ["executing", "outcome_unknown", "failed", "awaiting_settlement"]) expect(shouldRefreshPayment({ ...order, payment_state }, true, now)).toBe(false);
  expect(shouldRefreshPayment({ ...order, payment_action: undefined }, true, now)).toBe(false);
  expect(shouldRefreshPayment(order, true, now + 60000)).toBe(false);
  expect(shouldRefreshPayment({ ...order, status: "completed" }, true, now)).toBe(false);
  expect(shouldRefreshPayment({ ...order, paid_money: "2" }, false, now + 60000)).toBe(true);
});
