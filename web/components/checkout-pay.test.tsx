/** @vitest-environment jsdom */
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
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
  afterEach(() => {
    cleanup();
    vi.useRealTimers();
  });
  it("shows sandbox copy without a pay-check button", () => {
    render(
      withZh(
        <CheckoutPay checkout={{ order: { id: "pay_sandbox", status: "pending" }, sandbox: true, webhook_url: "http://localhost/v1/payments/stripe/webhook" }} />,
      ),
    );
    expect(screen.getByText("pay_sandbox")).toBeTruthy();
    expect(screen.getByText("沙箱订单，请通过渠道 Webhook 完成入账。")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "我已付款" })).toBeNull();
  });

  it("names failed and expired orders instead of 待支付", () => {
    const { rerender } = render(
      withZh(<CheckoutPay checkout={{ order: { id: "pay_fail", status: "failed" }, sandbox: true }} />),
    );
    const failed = screen.getByText("pay_fail").closest("[data-checkout-status]");
    expect(failed?.getAttribute("data-checkout-status")).toBe("failed");
    expect(failed?.textContent).toContain("支付失败");
    expect(failed?.textContent).not.toContain("待支付");
    rerender(withZh(<CheckoutPay checkout={{ order: { id: "pay_exp", status: "expired" }, sandbox: true }} />));
    const expired = screen.getByText("pay_exp").closest("[data-checkout-status]");
    expect(expired?.getAttribute("data-checkout-status")).toBe("expired");
    expect(expired?.textContent).toContain("订单过期");
    expect(expired?.textContent).not.toContain("待支付");
  });

  it("says credit is processing when paid but not fulfilled", () => {
    render(
      withZh(
        <CheckoutPay checkout={{ order: { id: "pay_paid", status: "paid", credit_minor: 13_990_000 }, sandbox: true }} />,
      ),
    );
    expect(screen.getByText("已支付，额度入账处理中")).toBeTruthy();
  });

  it("shows credited amount after fulfillment", () => {
    render(
      withZh(
        <CheckoutPay
          checkout={{
            order: { id: "pay_done", status: "paid", credit_minor: 13_990_000, fulfilled_at: "2026-09-15T07:00:00Z" },
            sandbox: true,
          }}
        />,
      ),
    );
    expect(screen.getByText("已到账 $13.99")).toBeTruthy();
  });

  it("names processing, partial refund, and unknown instead of 待支付", () => {
    const { rerender } = render(
      withZh(<CheckoutPay checkout={{ order: { id: "pay_proc", status: "processing", amount_minor: 10000, currency: "CNY" }, sandbox: true }} />),
    );
    expect(screen.getByText("pay_proc").closest("[data-checkout-status]")?.getAttribute("data-checkout-status")).toBe("confirming");
    rerender(
      withZh(
        <CheckoutPay
          checkout={{ order: { id: "pay_part", status: "partially_refunded", amount_minor: 10000, currency: "CNY" }, sandbox: true }}
        />,
      ),
    );
    const partial = screen.getByText("pay_part").closest("[data-checkout-status]");
    expect(partial?.getAttribute("data-checkout-status")).toBe("partially_refunded");
    expect(partial?.textContent).toContain("已部分退款");
    expect(partial?.textContent).not.toContain("待支付");
    rerender(withZh(<CheckoutPay checkout={{ order: { id: "pay_unk", status: "mystery" }, sandbox: true }} />));
    expect(screen.getByText("订单状态未知")).toBeTruthy();
    expect(screen.getByText("pay_unk").closest("[data-checkout-status]")?.getAttribute("data-checkout-status")).toBe("unknown");
  });

  it("keeps the order due on the receipt itself", () => {
    render(
      withZh(
        <CheckoutPay
          checkout={{
            order: { id: "pay_100", status: "pending", amount_minor: 10000, currency: "CNY", adapter: "alipay" },
            sandbox: true,
          }}
        />,
      ),
    );
    expect(screen.getByTestId("checkout-order-due").textContent).toContain("¥100.00");
  });

  it("shows a sync failure on the order instead of clearing it", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => ({
        ok: false,
        status: 500,
        json: async () => ({ error: { message: "upstream timeout" } }),
      })),
    );
    render(
      withZh(
        <CheckoutPay
          checkout={{
            order: { id: "pay_sync", status: "pending", amount_minor: 10000, currency: "CNY" },
            qr_code: "weixin://wxpay/bizpayurl?pr=x",
            sandbox: false,
          }}
        />,
      ),
    );
    screen.getByRole("button", { name: "我已付款" }).click();
    expect(await screen.findByText("upstream timeout")).toBeTruthy();
    expect(screen.getByText("pay_sync")).toBeTruthy();
    expect(screen.getByTestId("checkout-order-due").textContent).toContain("¥100.00");
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

  it("keeps order B when a late paid response for A arrives", async () => {
    const late = deferredJson();
    vi.stubGlobal(
      "fetch",
      vi.fn(() => late.promise),
    );
    const { rerender } = render(withZh(<CheckoutPay checkout={qrOrder("pay_a", 10000)} />));
    screen.getByRole("button", { name: "我已付款" }).click();
    rerender(withZh(<CheckoutPay checkout={qrOrder("pay_b", 30000)} />));
    expectOpenOrder("pay_b", "¥300.00");
    late.resolve(
      true,
      paidItem("pay_a", 10000),
    );
    await waitFor(() => {
      expectOpenOrder("pay_b", "¥300.00");
    });
    expect(screen.queryByText("正在查单…")).toBeNull();
  });

  it("does not show order A's failure or timeout on order B", async () => {
    const lateFail = deferredJson();
    vi.stubGlobal(
      "fetch",
      vi.fn(() => lateFail.promise),
    );
    const { rerender } = render(withZh(<CheckoutPay checkout={qrOrder("pay_a", 10000)} />));
    screen.getByRole("button", { name: "我已付款" }).click();
    rerender(withZh(<CheckoutPay checkout={qrOrder("pay_b", 30000)} />));
    lateFail.resolve(false, { error: { message: "A 查单失败" } });
    await waitFor(() => {
      expectOpenOrder("pay_b", "¥300.00");
    });
    expect(screen.queryByText("A 查单失败")).toBeNull();

    const lateTimeout = deferredJson();
    vi.stubGlobal(
      "fetch",
      vi.fn(() => lateTimeout.promise),
    );
    rerender(withZh(<CheckoutPay checkout={qrOrder("pay_a2", 10000)} />));
    screen.getByRole("button", { name: "我已付款" }).click();
    rerender(withZh(<CheckoutPay checkout={qrOrder("pay_b2", 30000)} />));
    lateTimeout.reject(new Error("network timeout"));
    await waitFor(() => {
      expectOpenOrder("pay_b2", "¥300.00");
    });
    expect(screen.queryByText("查单失败，请稍后重试")).toBeNull();
  });

  it("ignores a late poll for the previous order the same way as manual sync", async () => {
    vi.useFakeTimers();
    const late = deferredJson();
    vi.stubGlobal(
      "fetch",
      vi.fn(() => late.promise),
    );
    const { rerender } = render(withZh(<CheckoutPay checkout={qrOrder("pay_a", 10000)} />));
    vi.advanceTimersByTime(4000);
    expect(String(vi.mocked(fetch).mock.calls[0]?.[0])).toContain("/v1/payments/orders/pay_a");
    rerender(withZh(<CheckoutPay checkout={qrOrder("pay_b", 30000)} />));
    late.resolve(true, paidItem("pay_a", 10000));
    await Promise.resolve();
    await Promise.resolve();
    expectOpenOrder("pay_b", "¥300.00");
  });
});

function qrOrder(id: string, amountMinor: number) {
  return {
    order: { id, status: "pending" as const, amount_minor: amountMinor, currency: "CNY" },
    qr_code: "weixin://wxpay/bizpayurl?pr=x",
    sandbox: false,
  };
}

function paidItem(id: string, amountMinor: number) {
  return {
    item: {
      id,
      status: "paid",
      amount_minor: amountMinor,
      currency: "CNY",
      credit_minor: 10_000_000,
      fulfilled_at: "2026-09-17T00:00:00Z",
    },
  };
}

function receipt(orderID: string) {
  const root = screen.getByText(orderID).closest("[data-checkout-status]");
  return {
    status: root?.getAttribute("data-checkout-status"),
    due: screen.getByTestId("checkout-order-due").textContent,
    text: root?.textContent || "",
  };
}

function expectOpenOrder(orderID: string, due: string) {
  const view = receipt(orderID);
  expect(view.status).toBe("pending");
  expect(view.due).toContain(due);
  expect(view.text).not.toContain("已到账");
}

function deferredJson() {
  let resolve!: (value: { ok: boolean; json: () => Promise<unknown> }) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<{ ok: boolean; json: () => Promise<unknown> }>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return {
    promise,
    resolve(ok: boolean, body: unknown) {
      resolve({ ok, json: async () => body });
    },
    reject(reason?: unknown) {
      reject(reason);
    },
  };
}
