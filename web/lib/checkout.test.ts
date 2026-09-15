import { describe, expect, it } from "vitest";
import { checkoutKind, checkoutOrderID, checkoutUiStatus } from "./checkout";

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

  it("maps payment statuses instead of collapsing to pending", () => {
    expect(checkoutUiStatus("pending")).toBe("pending");
    expect(checkoutUiStatus("pending", true)).toBe("confirming");
    expect(checkoutUiStatus("failed")).toBe("failed");
    expect(checkoutUiStatus("expired")).toBe("expired");
    expect(checkoutUiStatus("paid", true)).toBe("paid");
    expect(checkoutUiStatus("refunded")).toBe("refunded");
  });
});
