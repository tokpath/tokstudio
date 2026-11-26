// @vitest-environment jsdom
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import type { ReactNode } from "react";
import { CustomerOrders } from "./customer-records";
import { ResourceScreen } from "./operator-resources";

vi.mock("./context", () => ({ useProduct: () => ({ session: { kind: "customer" }, context: { payment_methods: [] } }) }));
vi.mock("next/navigation", async () => {
  const { useSyncExternalStore } = await import("react");
  const subscribe = (notify: () => void) => {
    window.addEventListener("popstate", notify);
    return () => window.removeEventListener("popstate", notify);
  };
  return {
    usePathname: () => window.location.pathname,
    useRouter: () => ({ replace: vi.fn() }),
    useSearchParams: () => new URLSearchParams(useSyncExternalStore(subscribe, () => window.location.search)),
  };
});

const order = { id: "order-one", status: "pending", kind: "package", operator_name: "TokenHub 平台", plan_name: "测试套餐", amount_money: "80", paid_money: "0", created_at: "2026-10-29T12:00:00Z", cancel_allowed: true, payment_action: { kind: "offline", address: "TEST-ONLY", currency: "USD", amount: "80", instructions: "测试收款说明" } };
const response = (body: unknown) => new Response(JSON.stringify(body), { status: 200 });

function mount(children: ReactNode) {
  const cache = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={cache}>{children}</QueryClientProvider>);
}

afterEach(() => { cleanup(); vi.unstubAllGlobals(); vi.useRealTimers(); window.history.replaceState(null, "", "/"); });

it("opens order details in a right-hand panel instead of a card above the list", async () => {
  vi.stubGlobal("fetch", vi.fn(async (path: RequestInfo | URL) => response(String(path).includes("/orders/order-one") ? order : { items: [order] })));
  mount(<CustomerOrders />);
  expect(screen.queryByRole("dialog")).toBeNull();
  fireEvent.click(await screen.findByRole("button", { name: "继续付款 / 查看详情" }));
  fireEvent.popState(window);
  const dialog = await screen.findByRole("dialog");
  expect(dialog.className).toContain("right-0");
  expect(dialog.className).not.toContain("left-1/2");
  expect(screen.getByRole("heading", { name: "订单与到账", hidden: true })).toBeTruthy();
  expect(await screen.findByText("测试收款说明")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "收起详情" }));
  fireEvent.popState(window);
  expect(screen.queryByRole("dialog")).toBeNull();
});

it.each(["orders", "topups"])("updates the visible %s history as its payment deadline passes", async (section) => {
  vi.useFakeTimers();
  window.history.replaceState(null, "", `/workspace/${section}`);
  const expiring = { ...order, expires_at: new Date(Date.now() + 1_000).toISOString() };
  const topupPage = {
    title: "充值", description: "", columns: [], operations: [], row_operations: [],
    items: [{ id: expiring.id, status: expiring.status, values: { due_money: expiring.amount_money, external_allocated_money: "0", expires_at: expiring.expires_at } }],
  };
  const fetch = vi.fn(async () => response(section === "orders" ? { items: [expiring] } : topupPage));
  vi.stubGlobal("fetch", fetch);
  await act(async () => { mount(section === "orders" ? <CustomerOrders /> : <ResourceScreen resource="topups/workspace" customer />); });
  await act(async () => { await vi.advanceTimersByTimeAsync(100); });
  expect(screen.getByText("待处理", { selector: "span" })).toBeTruthy();
  expect(screen.queryByText("已过期", { selector: "span" })).toBeNull();

  await act(async () => { await vi.advanceTimersByTimeAsync(900); });
  expect(screen.getByText("已过期", { selector: "span" })).toBeTruthy();
  expect(screen.queryByRole("button", { name: "继续付款 / 查看详情" })).toBeNull();
  expect(fetch).toHaveBeenCalledTimes(1);
});
