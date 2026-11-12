// @vitest-environment jsdom
import { expect, it, vi } from "vitest";
import type { Order } from "./contracts";
import { markPaymentOpened, paymentWasOpened, reservePaymentWindow, shouldRefreshPayment, submitPaymentForm } from "./payment-window";

it("posts the Gkash form into the window opened by the confirmation click", () => {
  const opened = { closed: false } as Window;
  vi.stubGlobal("open", vi.fn(() => opened));
  const submitted: HTMLFormElement[] = [];
  HTMLFormElement.prototype.submit = function submit() { submitted.push(this); };
  expect(reservePaymentWindow()).toBe(opened);
  expect(submitPaymentForm(opened, "https://api.gkash.my/api/PaymentForm.aspx", { CID: "merchant", v_amount: "4.18" })).toBe(true);
  expect(submitted).toHaveLength(1);
  expect(submitted[0].target).toBe("tokpath-gkash");
  expect(submitted[0].method.toLowerCase()).toBe("post");
  expect(submitted[0].action).toBe("https://api.gkash.my/api/PaymentForm.aspx");
  expect(submitted[0].querySelector("input[name=CID]")?.getAttribute("value")).toBe("merchant");
  expect(document.querySelector("form")).toBeNull();
});

it("remembers which order already opened the payment page", () => {
  expect(paymentWasOpened("order-one")).toBe(false);
  markPaymentOpened("order-one");
  expect(paymentWasOpened("order-one")).toBe(true);
  expect(paymentWasOpened("order-two")).toBe(false);
});

it("stops refreshing unusable, unopened, expired and completed checkouts", () => {
  const now = Date.now();
  const order: Order = { id: "one", kind: "topup", status: "pending", payment_state: "created", operator_name: "Test", amount_money: "2", paid_money: "0", created_at: new Date(now).toISOString(), expires_at: new Date(now + 60000).toISOString(), payment_action: { kind: "form", url: "https://pay.example.test", fields: {v_amount:"8.26"} } };
  expect(shouldRefreshPayment(order, false, now)).toBe(false);
  expect(shouldRefreshPayment(order, true, now)).toBe(true);
  for (const payment_state of ["executing", "outcome_unknown", "failed", "awaiting_settlement"]) expect(shouldRefreshPayment({ ...order, payment_state }, true, now)).toBe(false);
  expect(shouldRefreshPayment({ ...order, payment_action: undefined }, true, now)).toBe(false);
  expect(shouldRefreshPayment(order, true, now + 60000)).toBe(false);
  expect(shouldRefreshPayment({ ...order, status: "completed" }, true, now)).toBe(false);
  expect(shouldRefreshPayment({ ...order, paid_money: "2" }, false, now + 60000)).toBe(true);
});
