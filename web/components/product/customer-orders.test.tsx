// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import type { ReactNode } from "react";
import { CustomerOrders } from "./customer-records";

vi.mock("./context", () => ({ useProduct: () => ({ session: { kind: "customer" }, context: { payment_methods: [] } }) }));

const order = { id: "order-one", status: "pending", kind: "package", operator_name: "TokenHub 平台", plan_name: "测试套餐", amount_money: "80", paid_money: "0", created_at: "2026-10-29T12:00:00Z", cancel_allowed: true, payment_action: { kind: "offline", address: "TEST-ONLY", currency: "USD", amount: "80", instructions: "测试收款说明" } };
const response = (body: unknown) => new Response(JSON.stringify(body), { status: 200 });

function mount(children: ReactNode) {
  const cache = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={cache}>{children}</QueryClientProvider>);
}

afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

it("opens order details in a right-hand panel instead of a card above the list", async () => {
  vi.stubGlobal("fetch", vi.fn(async (path: RequestInfo | URL) => response(String(path).includes("/orders/order-one") ? order : { items: [order] })));
  mount(<CustomerOrders />);
  expect(screen.queryByRole("dialog")).toBeNull();
  fireEvent.click(await screen.findByRole("button", { name: "继续付款 / 查看详情" }));
  const dialog = await screen.findByRole("dialog");
  expect(dialog.className).toContain("right-0");
  expect(dialog.className).not.toContain("left-1/2");
  expect(screen.getByRole("heading", { name: "订单与到账", hidden: true })).toBeTruthy();
  expect(await screen.findByText("测试收款说明")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "收起详情" }));
  expect(screen.queryByRole("dialog")).toBeNull();
});
