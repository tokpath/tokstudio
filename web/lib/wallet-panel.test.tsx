/** @vitest-environment jsdom */
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import WalletPanel from "../app/console/wallet-panel";
import { withZh } from "./test-i18n";

vi.mock("next/navigation", () => ({ useSearchParams:()=>new URLSearchParams(),
  usePathname: () => "/app/wallet",
  useSearchParams: () => new URLSearchParams(),
}));

vi.mock("@/components/rbac/viewer-context",()=>({useViewer:()=>({userId:"wallet-user",loading:false,roles:["end_user"]})}));
vi.mock("@/components/brand-context",()=>({useBrand:()=>({id:"wallet-brand"})}));

function jsonResponse(status: number, body: unknown) {
  return { ok: status >= 200 && status < 300, status, json: async () => body };
}

function quote(major: number) {
  return {
    item: {
      adapter: "alipay",
      pay_major: major,
      pay_currency: "CNY",
      pay_minor: major * 100,
      fee_minor: 0,
      credit_minor: major * 100_000,
    },
  };
}

function checkout(major: number, id: string) {
  return {
    checkout: {
      adapter: "alipay",
      sandbox: true,
      order: {
        id,
        status: "pending",
        adapter: "alipay",
        currency: "CNY",
        amount_minor: major * 100,
        credit_minor: major * 100_000,
      },
    },
  };
}

describe("WalletPanel order vs quote", () => {
  afterEach(() => {
    cleanup();
    sessionStorage.clear();
    vi.unstubAllGlobals();
  });

  it("locks amount while creating and keeps the returned order when the quote later changes", async () => {
    let release!: () => void;
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input);
        if (url.includes("/v1/me/balance")) {
          return jsonResponse(200, { balance: { available: "$0.00" } });
        }
        if (url.includes("/v1/payments/checkout") && !url.includes("/orders")) {
          return jsonResponse(200, {
            item: {
              methods: [{ adapter: "alipay", display_name: "支付宝", pay_currency: "CNY" }],
              settings: { quick_amounts: [100, 300] },
            },
          });
        }
        if (url.includes("/v1/payments/quote")) {
          const major = Number(new URL(url, "http://local").searchParams.get("pay_major"));
          return jsonResponse(200, quote(major));
        }
        if (url.endsWith("/v1/payments/orders/pay_100/sync") && init?.method === "POST") return jsonResponse(200,{item:{...checkout(100,"pay_100").checkout.order,status:"paid",fulfilled_at:"2026-10-10T00:00:00Z"}});
        if (url.includes("/v1/payments/orders") && init?.method === "POST") {
          await gate;
          const original=checkout(100,"pay_100");return jsonResponse(201,{checkout:{...original.checkout,qr_code:"fixture-payment-qr"}});
        }
        return jsonResponse(200, {});
      }),
    );

    render(withZh(<WalletPanel />));
    await waitFor(() => expect(screen.getByTestId("wallet-pay").textContent).toBe("支付 ¥100.00"));
    fireEvent.click(screen.getByTestId("wallet-pay"));
    await waitFor(() => expect((screen.getByRole("button", { name: "¥300" }) as HTMLButtonElement).disabled).toBe(true));
    expect((screen.getByRole("button", { name: "¥100" }) as HTMLButtonElement).disabled).toBe(true);
    release();
    await waitFor(() => expect(screen.getByText("pay_100")).toBeTruthy());
    expect(screen.getByTestId("checkout-order-due").textContent).toContain("¥100.00");
    expect((screen.getByRole("button", { name: "¥300" }) as HTMLButtonElement).disabled).toBe(true);
    fireEvent.click(screen.getByRole("button",{name:"我已付款"}));
    await waitFor(()=>expect((screen.getByRole("button", { name: "¥300" }) as HTMLButtonElement).disabled).toBe(false));
    fireEvent.click(screen.getByRole("button", { name: "¥300" }));
    await waitFor(() => expect(screen.getByTestId("wallet-pay").textContent).toBe("支付 ¥300.00"));
    expect(screen.getByTestId("checkout-order-due").textContent).toContain("¥100.00");
    expect(screen.getByText(/当前报价 ¥300.00 与已有订单 ¥100.00/)).toBeTruthy();
  });

  it("treats a dropped create response as unconfirmed and recovers with the same key", async () => {
    const keys: string[] = [];
    let attempt = 0;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input);
        if (url.includes("/v1/me/balance")) {
          return jsonResponse(200, { balance: { available: "$0.00" } });
        }
        if (url.includes("/v1/payments/checkout") && !url.includes("/orders")) {
          return jsonResponse(200, {
            item: {
              methods: [{ adapter: "alipay", display_name: "支付宝", pay_currency: "CNY" }],
              settings: { quick_amounts: [100, 300] },
            },
          });
        }
        if (url.includes("/v1/payments/quote")) {
          return jsonResponse(200, quote(100));
        }
        if (url.includes("/v1/payments/orders") && init?.method === "POST") {
          keys.push(new Headers(init.headers).get("Idempotency-Key") || "");
          attempt += 1;
          if (attempt === 1) {
            throw new Error("network");
          }
          return jsonResponse(201, checkout(100, "pay_recover"));
        }
        return jsonResponse(200, {});
      }),
    );

    render(withZh(<WalletPanel />));
    await waitFor(() => expect(screen.getByTestId("wallet-pay").textContent).toBe("支付 ¥100.00"));
    fireEvent.click(screen.getByTestId("wallet-pay"));
    await waitFor(() => expect(screen.getByText(/创建结果尚未确认/)).toBeTruthy());
    expect(screen.getByTestId("wallet-pay").textContent).toBe("查询创建结果");
    fireEvent.click(screen.getByTestId("wallet-pay"));
    await waitFor(() => expect(screen.getByText("pay_recover")).toBeTruthy());
    expect(keys[0]).toBeTruthy();
    expect(keys[1]).toBe(keys[0]);
    expect(screen.getByTestId("checkout-order-due").textContent).toContain("¥100.00");
  });

  it("keeps an already paid operation until fulfillment and then uses a new operation",async()=>{
    let fulfilled=false;const createdKeys:string[]=[];
    const original={id:"original-paid-op",adapter:"alipay",payMajor:100,orderId:"paid-order"};
    sessionStorage.setItem("tokenhub_wallet_purchase:wallet-user:wallet-brand",JSON.stringify(original));
    vi.stubGlobal("fetch",vi.fn(async(input:RequestInfo|URL,init?:RequestInit)=>{
      const url=String(input);
      if(url.includes("/v1/me/balance"))return jsonResponse(200,{balance:{available:"0",gift_minor:0,purchased_minor:0}});
      if(url.includes("/v1/me/wallet-purchases"))return jsonResponse(200,{item:{...checkout(100,"paid-order").checkout.order,status:"paid",fulfilled_at:fulfilled ? "2026-10-10" : null}});
      if(url.includes("/v1/payments/checkout"))return jsonResponse(200,{item:{methods:[{adapter:"alipay",pay_currency:"CNY"}],settings:{quick_amounts:[100,300]}}});
      if(url.includes("/v1/payments/quote"))return jsonResponse(200,quote(100));
      if(url.includes("/v1/payments/orders") && init?.method==="POST"){createdKeys.push(new Headers(init.headers).get("Idempotency-Key") || "");return jsonResponse(201,checkout(100,"new-order"));}
      return jsonResponse(200,{});
    }));
    const mounted=render(withZh(<WalletPanel/>));
    await waitFor(()=>expect(screen.getByText("已支付，额度入账处理中")).toBeTruthy());
    expect((screen.getByRole("button",{name:"¥300"}) as HTMLButtonElement).disabled).toBe(true);
    expect(screen.queryByTestId("wallet-pay")).toBeNull();expect(createdKeys).toHaveLength(0);
    expect(JSON.parse(sessionStorage.getItem("tokenhub_wallet_purchase:wallet-user:wallet-brand") || "null").id).toBe(original.id);
    mounted.unmount();fulfilled=true;render(withZh(<WalletPanel/>));
    await waitFor(()=>expect((screen.getByRole("button",{name:"¥300"}) as HTMLButtonElement).disabled).toBe(false));
    await waitFor(()=>expect(screen.getByTestId("wallet-pay").textContent).toBe("支付 ¥100.00"));
    fireEvent.click(screen.getByTestId("wallet-pay"));await waitFor(()=>expect(createdKeys).toHaveLength(1));expect(createdKeys[0]).not.toBe(original.id);
  });

  it("retries a failed quote without changing the amount", async () => {
    let quotes = 0;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        const url = String(input);
        if (url.includes("/v1/me/balance")) {
          return jsonResponse(200, { balance: { available: "$0.00" } });
        }
        if (url.includes("/v1/payments/checkout") && !url.includes("/orders")) {
          return jsonResponse(200, {
            item: {
              methods: [{ adapter: "alipay", display_name: "支付宝", pay_currency: "CNY" }],
              settings: { quick_amounts: [100, 300] },
            },
          });
        }
        if (url.includes("/v1/payments/quote")) {
          quotes += 1;
          if (quotes === 1) {
            return jsonResponse(500, { error: { message: "quote down" } });
          }
          return jsonResponse(200, quote(100));
        }
        return jsonResponse(200, {});
      }),
    );

    render(withZh(<WalletPanel />));
    await waitFor(() => expect(screen.getByTestId("quote-retry")).toBeTruthy());
    fireEvent.click(screen.getByTestId("quote-retry"));
    await waitFor(() => expect(screen.getByTestId("wallet-pay").textContent).toBe("支付 ¥100.00"));
    expect(quotes).toBe(2);
  });

  it("keeps quote summary before the pay button in document order and names the redeem field", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        const url = String(input);
        if (url.includes("/v1/me/balance")) {
          return jsonResponse(200, { balance: { available: "$0.00" } });
        }
        if (url.includes("/v1/payments/checkout") && !url.includes("/orders")) {
          return jsonResponse(200, {
            item: {
              methods: [{ adapter: "alipay", display_name: "支付宝", pay_currency: "CNY" }],
              settings: { quick_amounts: [100, 300] },
            },
          });
        }
        if (url.includes("/v1/payments/quote")) {
          return jsonResponse(200, quote(100));
        }
        return jsonResponse(200, {});
      }),
    );

    render(withZh(<WalletPanel />));
    await waitFor(() => expect(screen.getByTestId("wallet-pay")).toBeTruthy());
    const quoteEl = screen.getByTestId("quote-summary");
    const payEl = screen.getByTestId("wallet-pay");
    expect(quoteEl.compareDocumentPosition(payEl) & Node.DOCUMENT_POSITION_FOLLOWING).toBe(Node.DOCUMENT_POSITION_FOLLOWING);
    expect(screen.getByLabelText("兑换码")).toBeTruthy();
  });
});
