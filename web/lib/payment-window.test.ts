// @vitest-environment jsdom
import { expect, it, vi } from "vitest";
import { markPaymentOpened, paymentWasOpened, reservePaymentWindow, submitPaymentForm } from "./payment-window";

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
