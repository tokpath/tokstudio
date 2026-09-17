import { describe, expect, it } from "vitest";
import {
  checkoutKind,
  checkoutOrderID,
  checkoutResponseBelongsToOrder,
  checkoutUiStatus,
  formatOrderDue,
  orderMatchesSelection,
  stripeClientOutcome,
} from "./checkout";

describe("checkoutKind", () => {
  it("prefers live qr and stripe element over sandbox", () => {
    expect(checkoutKind({ sandbox: true })).toBe("sandbox");
    expect(checkoutKind({ qr_code: "weixin://wxpay/bizpayurl?pr=x", sandbox: false })).toBe("qr");
    expect(
      checkoutKind({ client_secret: "cs_test", publishable_key: "pk_test", sandbox: false }),
    ).toBe("element");
    expect(checkoutKind({ redirect_url: "https://pay.example/go" })).toBe("redirect");
    expect(checkoutKind({ sandbox: true, mode: "qr" })).toBe("sandbox");
  });

  it("reads order id", () => {
    expect(checkoutOrderID({ order: { id: " pay_1 " } })).toBe("pay_1");
    expect(checkoutOrderID({})).toBe("");
  });

  it("rejects a response that belongs to another order", () => {
    const paidA = { id: "pay_a", status: "paid", amount_minor: 10000 };
    expect(checkoutResponseBelongsToOrder("pay_a", "pay_a", paidA)).toBe(true);
    expect(checkoutResponseBelongsToOrder("pay_a", "pay_b", paidA)).toBe(false);
    expect(checkoutResponseBelongsToOrder("pay_b", "pay_b", paidA)).toBe(false);
    expect(checkoutResponseBelongsToOrder("pay_b", "pay_b", { status: "paid" })).toBe(true);
  });

  it("maps payment statuses instead of collapsing to pending", () => {
    expect(checkoutUiStatus("pending")).toBe("pending");
    expect(checkoutUiStatus("pending", true)).toBe("confirming");
    expect(checkoutUiStatus("failed")).toBe("failed");
    expect(checkoutUiStatus("expired")).toBe("expired");
    expect(checkoutUiStatus("paid", true)).toBe("paid");
    expect(checkoutUiStatus("refunded")).toBe("refunded");
    expect(checkoutUiStatus("partially_refunded")).toBe("partially_refunded");
    expect(checkoutUiStatus("processing")).toBe("confirming");
    expect(checkoutUiStatus("mystery")).toBe("unknown");
  });

  it("never treats a Stripe SDK success as paid", () => {
    expect(stripeClientOutcome({})).toBe("confirming");
    expect(stripeClientOutcome({ error: { message: "card declined" } })).toBe("error");
  });

  it("compares an order to the current recharge selection", () => {
    expect(orderMatchesSelection({ adapter: "alipay", currency: "CNY", amount_minor: 10000 }, "alipay", 100)).toBe(true);
    expect(orderMatchesSelection({ adapter: "alipay", currency: "CNY", amount_minor: 10000 }, "alipay", 300)).toBe(false);
    expect(formatOrderDue({ currency: "CNY", amount_minor: 10000 })).toBe("¥100.00");
  });
});
