/** @vitest-environment jsdom */
import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { CheckoutPay } from "./checkout-pay";
import { withZh } from "@/lib/test-i18n";

vi.mock("qrcode", () => ({
  default: {
    toString: async () => "<svg xmlns='http://www.w3.org/2000/svg'><rect width='1' height='1'/></svg>",
  },
}));

vi.stubGlobal(
  "fetch",
  vi.fn(async () => ({
    ok: true,
    json: async () => ({ item: { status: "pending" } }),
  })),
);

describe("CheckoutPay", () => {
  it("shows sandbox copy without a pay-check button", () => {
    render(
      withZh(
        <CheckoutPay checkout={{ order: { id: "pay_sandbox", status: "pending" }, sandbox: true, webhook_url: "http://localhost/v1/payments/stripe/webhook" }} />,
      ),
    );
    expect(screen.getByText("pay_sandbox")).toBeTruthy();
    expect(screen.getByText("沙箱订单，请用渠道 webhook 完成入账。")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "我已付款" })).toBeNull();
  });

  it("renders a qr surface for live native checkout", async () => {
    render(
      withZh(
        <CheckoutPay
          checkout={{
            order: { id: "pay_wx", status: "pending" },
            sandbox: false,
            qr_code: "weixin://wxpay/bizpayurl?pr=x",
          }}
        />,
      ),
    );
    expect(await screen.findByRole("img", { name: "请用对应 App 扫码支付" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "我已付款" })).toBeTruthy();
  });
});
