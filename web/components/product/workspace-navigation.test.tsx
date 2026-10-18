// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import { WorkspaceShell } from "./workspace-shell";
import { useNavigationBlock } from "@/lib/navigation-guard";
import type { Session } from "@/lib/contracts";

const state = vi.hoisted(() => ({ path: "/workspace", session: undefined as Session | undefined, push: vi.fn() }));
vi.mock("next/navigation", () => ({ usePathname: () => state.path, useRouter: () => ({ push: state.push }) }));
vi.mock("./context", () => ({ useProduct: () => ({ session: state.session, reload: vi.fn() }) }));
const customer: Session = { kind: "customer", customer: { id: "customer", email: "customer@example.test", display_name: "客户" }, brand: { id: "brand", name: "品牌", api_base_url: "https://brand.test/v1", theme: {} }, operator: { id: "operator", name: "经营方", kind: "platform", brand_id: "brand" } };
const staff = (permissions: string[], kind: "platform" | "oem" | "channel" = "platform"): Session => ({ kind: "staff", staff: { id: "staff", email: "staff@example.test", display_name: "负责人" }, workspaces: ["one", "two"].map(id => ({ operator: { id, name: `经营方 ${id}`, kind, brand_id: "brand" }, permissions })) });
function Pending() { const blocked = useNavigationBlock(true); return blocked ? <p role="alert">业务操作尚待核实</p> : <p>正在核实</p>; }
function mount(operatorID?: string, pending = false) { const cache = new QueryClient({ defaultOptions: { queries: { retry: false } } }); return render(<QueryClientProvider client={cache}><WorkspaceShell operatorID={operatorID}>{pending ? <Pending /> : <p>当前业务内容</p>}</WorkspaceShell></QueryClientProvider>); }
afterEach(() => { cleanup(); vi.unstubAllGlobals(); state.push.mockReset(); state.session = undefined; state.path = "/workspace"; window.history.replaceState(null, "", "/"); });

it("places account and notifications in the sidebar tools and expands only the current task", () => {
  vi.stubGlobal("fetch", vi.fn(async () => new Response("{}", { status: 500 })));
  state.session = customer; state.path = "/workspace/plans"; mount();
  const business = screen.getByRole("navigation", { name: "工作台", exact: true });
  const tools = screen.getByRole("navigation", { name: "工作台工具", exact: true });
  expect(within(business).queryByRole("link", { name: "账号设置" })).toBeNull();
  expect(within(business).queryByRole("link", { name: "通知与订阅" })).toBeNull();
  expect(within(tools).getByRole("link", { name: "账号与安全" }).getAttribute("href")).toBe("/workspace/account");
  expect(within(tools).getByRole("link", { name: "通知与订阅" }).getAttribute("href")).toBe("/workspace/notifications");
  const expanded = within(business).getByRole("navigation", { name: "套餐与资金内导航" });
  expect(expanded.closest("aside")).not.toBeNull();
  expect(within(expanded).getAllByRole("link")).toHaveLength(6);
  expect(within(business).queryByRole("navigation", { name: "模型与接入内导航" })).toBeNull();
});

it("expands authorized operator links vertically and keeps notifications in tools", () => {
  state.session = staff(["payments.read", "refunds.read", "notifications.read", "providers.read"]); state.path = "/manage/one/payments"; mount("one");
  const business = screen.getByRole("navigation", { name: "工作台", exact: true });
  const finance = within(business).getByRole("navigation", { name: "资金办理内导航" });
  expect(within(finance).getAllByRole("link")).toHaveLength(2);
  expect(within(finance).getByRole("link", { name: "收款与分配" }).getAttribute("aria-current")).toBe("page");
  expect(within(business).queryByRole("link", { name: "订单与交付" })).toBeNull();
  expect(within(business).queryByRole("link", { name: "通知管理" })).toBeNull();
  expect(within(screen.getByRole("navigation", { name: "工作台工具", exact: true })).getByRole("link", { name: "通知管理" })).toBeTruthy();
});

it("keeps platform-only resources and unauthorized notification tools out of a channel workspace", () => {
  state.session = staff(["payments.read", "providers.read"], "channel"); state.path = "/manage/one/payments"; mount("one");
  const business = screen.getByRole("navigation", { name: "工作台", exact: true });
  expect(within(business).queryByRole("link", { name: "平台技术" })).toBeNull();
  expect(within(screen.getByRole("navigation", { name: "工作台工具", exact: true })).queryByRole("link")).toBeNull();
});

it("retains the navigation guard for sidebar links and workspace switching", async () => {
  state.session = staff(["payments.read", "refunds.read"]); state.path = "/manage/one/payments"; mount("one", true);
  const navigation = screen.getByRole("navigation", { name: "工作台", exact: true });
  expect(fireEvent.click(within(navigation).getByRole("link", { name: "退款与冲正" }))).toBe(false);
  await screen.findByRole("alert");
  fireEvent.change(screen.getByLabelText("经营空间", { exact: true }), { target: { value: "two" } });
  expect(state.push).not.toHaveBeenCalled();
});

it("carries an anonymous model selection into the login link", async () => {
  state.session = { kind: "anonymous" }; state.path = "/workspace/models";
  window.history.replaceState(null, "", "/workspace/models?model=public-model&other=discard"); mount();
  await waitFor(() => expect(new URL(screen.getByRole("link", { name: "前往登录" }).getAttribute("href")!, "https://brand.test").searchParams.get("next")).toBe("/workspace/models?model=public-model"));
});
