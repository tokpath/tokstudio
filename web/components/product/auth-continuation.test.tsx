// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import { AuthPage } from "./auth";

const route = vi.hoisted(() => ({ search: "", push: vi.fn() }));
vi.mock("next/navigation", () => ({ useRouter: () => ({ push: route.push }), useSearchParams: () => new URLSearchParams(route.search) }));
vi.mock("./context", () => ({ useProduct: () => ({ context: { brand: { name: "当前品牌" }, operator: { name: "当前经营方" } } }) }));
const response = (data: unknown) => new Response(JSON.stringify(data), { status: 200 });
const session = { kind: "customer", customer: { id: "customer-one", email: "customer@example.test", display_name: "客户" }, brand: { id: "brand-one", name: "当前品牌", api_base_url: "https://brand.test/v1", theme: {} }, operator: { id: "operator-one", name: "当前经营方", kind: "platform", brand_id: "brand-one" }, csrf_token: "csrf" };
function mount(mode: "login" | "register" | "verify") { const cache = new QueryClient({ defaultOptions: { queries: { retry: false } } }); return render(<QueryClientProvider client={cache}><AuthPage mode={mode} /></QueryClientProvider>); }
afterEach(() => { cleanup(); vi.unstubAllGlobals(); route.search = ""; route.push.mockReset(); });

it("carries a whitelisted plan and promotion through registration without changing invitation ownership", async () => {
  const next = "/workspace/plans?plan=plan-one&kol_activity=kol-one&invite=forged";
  route.search = new URLSearchParams({ next }).toString();
  const calls: RequestInit[] = [];
  vi.stubGlobal("fetch", vi.fn(async (_path: RequestInfo | URL, init?: RequestInit) => { calls.push(init!); return response({ registration_id: "registration-one", verification_required: true, message: "验证邮件已进入发送队列，请通过邮件完成注册" }); }));
  mount("register");
  fireEvent.change(screen.getByLabelText("显示名称"), { target: { value: "新客户" } });
  fireEvent.change(screen.getByLabelText("邮箱", { exact: true }), { target: { value: "customer@example.test" } });
  fireEvent.change(screen.getByLabelText("密码", { exact: true }), { target: { value: "customer-password" } });
  fireEvent.click(screen.getByRole("button", { name: "确认归属并注册" }));
  await waitFor(() => expect(calls).toHaveLength(1));
  expect(JSON.parse(String(calls[0].body))).toEqual({ email: "customer@example.test", password: "customer-password", display_name: "新客户", next: "/workspace/plans?plan=plan-one&kol_activity=kol-one" });
});

it("retains the email's continuation when verification finishes on another device", async () => {
  route.search = new URLSearchParams({ token: "opaque-mail-token", next: "/workspace/models?model=public-model" }).toString();
  vi.stubGlobal("fetch", vi.fn(async () => response({ id: "customer-one", status: "verified" })));
  mount("verify"); fireEvent.click(screen.getByRole("button", { name: "验证邮箱" }));
  const link = await screen.findByRole("link", { name: "前往登录" });
  expect(new URL(link.getAttribute("href")!, "https://brand.test").searchParams.get("next")).toBe("/workspace/models?model=public-model");
});

it.each(["/workspace/models?model=public-model", "//evil.test", "/\\evil.test"])("redirects login only to a whitelisted internal task: %s", async next => {
  route.search = new URLSearchParams({ next, invite: "original-inviter" }).toString();
  vi.stubGlobal("fetch", vi.fn(async () => response(session))); mount("login");
  const create = screen.getByRole("link", { name: "创建客户账号" });
  const signup = new URL(create.getAttribute("href")!, "https://brand.test");
  expect(signup.searchParams.get("invite")).toBe("original-inviter");
  fireEvent.change(screen.getByLabelText("邮箱", { exact: true }), { target: { value: "customer@example.test" } });
  fireEvent.change(screen.getByLabelText("密码", { exact: true }), { target: { value: "customer-password" } });
  fireEvent.click(screen.getByRole("button", { name: "登录", exact: true }));
  await waitFor(() => expect(route.push).toHaveBeenCalledWith(next.startsWith("/workspace/models") ? next : "/workspace"));
});
