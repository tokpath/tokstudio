// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import type { ReactNode } from "react";
import { CustomerOverview } from "./customer-overview";
import { CustomerKeys, KeyEditor } from "./customer-keys";
import { Checkout, CustomerPlans } from "./customer-plans";
import { CustomerRequests } from "./customer-records";
import { totalCredits } from "./customer-finance";
import customerUX from "@/locales/customer-ux.json";

vi.mock("./context", () => ({ useProduct: () => ({ context: { brand: { api_base_url: "https://brand.example.test/v1" }, payment_methods: [{ id: "bank", name: "银行转账", status: "ready" }] } }) }));
const credits = { customer_atg: { available: "100", held: "2", purchased_remaining: "90", gift_remaining: "10" }, usd_balance: { available: "3.000000000000000001", held: "0" }, commission_usd: { available: "2", held: "0", frozen: "1", recovery_due: "0" }, qualification: { confirmed_usd: "0", pending_usd: "0", threshold_usd: "100", qualified_at: null, function_status: "normal" } };
const apiKey = { id: "key-one", name: "生产应用", prefix: "th_production", status: "active", model_ids: null, budget: { limit_atg: "20", period: "month", timezone: "Asia/Shanghai", spent_atg: "1", held_atg: "0", overage_atg: "0", window_start: null, window_end: null }, rate_limit_rpm: null, concurrency_limit: null, version: 1, created_at: "2026-10-18T00:00:00Z" };
const call = { id: "request-one", model_id: "public-model", key_name: "生产应用", status: "completed", charged_atg: "0.01", held_atg: "0", created_at: "2026-10-18T00:00:00Z" };
const plan = { id: "plan-one", name: "按需套餐", price_usd: "10", purchase_atg: "20.000000000000000000000001", gift_atg: "2", validity: "month", auto_renew_available: true, status: "active", version: 1 };
const quote = { id: "quote-one", plan_id: "plan-one", plan_name: "按需套餐", operator_name: "经营方", amount_usd: "10", purchase_atg: "20", plan_gift_atg: "2", operator_gift_atg: "0", kol_gift_atg: "0", total_atg: "22", validity: "month", terms_version: "terms-one", expires_at: "2099-10-18T00:00:00Z", warnings: [] };
const response = (data: unknown) => new Response(JSON.stringify(data), { status: 200 });
function mount(children: ReactNode) { const cache = new QueryClient({ defaultOptions: { queries: { retry: false } } }); return render(<QueryClientProvider client={cache}>{children}</QueryClientProvider>); }
function overviewFetch(items: unknown[]) { return vi.fn(async (path: RequestInfo | URL) => { const url = String(path); return response(url.endsWith("/credits") ? credits : { items: url.endsWith("/requests") ? items : url.endsWith("/keys") ? [apiKey] : [] }); }); }
afterEach(() => { cleanup(); vi.unstubAllGlobals(); window.history.replaceState(null, "", "/"); });

it("shows the first-call guide only after confirming that the account has no calls", async () => {
  vi.stubGlobal("fetch", overviewFetch([]));
  mount(<CustomerOverview />);
  expect(screen.queryByText("接入你的第一个应用")).toBeNull();
  await screen.findByText("接入你的第一个应用");
  expect(screen.getByText("https://brand.example.test/v1")).toBeTruthy();
  expect(screen.getByText("已有可用 ATG，可以继续创建或使用 Key。")).toBeTruthy();
});

it("prioritizes recent calls for returning customers without repeating the first-app guide", async () => {
  vi.stubGlobal("fetch", overviewFetch([call])); mount(<CustomerOverview />);
  await screen.findByText("public-model");
  expect(screen.queryByText("接入你的第一个应用")).toBeNull();
  expect(screen.getByRole("heading", { name: "最近调用" })).toBeTruthy();
  expect(screen.getByRole("link", { name: /public-model/ }).getAttribute("href")).toContain("record=request-one");
});

it("does not present a failed usage lookup as a new customer", async () => {
  vi.stubGlobal("fetch", vi.fn(async (path: RequestInfo | URL) => { if (String(path).endsWith("/requests")) throw new TypeError("network lost"); return response(String(path).endsWith("/credits") ? credits : { items: [] }); }));
  mount(<CustomerOverview />); await screen.findByRole("alert");
  expect(screen.queryByText("接入你的第一个应用")).toBeNull();
});

it("shows exact total plan credits and puts funding tools after the product catalog", async () => {
  vi.stubGlobal("fetch", vi.fn(async (path: RequestInfo | URL) => response({ items: String(path).endsWith("/catalog") ? [plan] : [] })));
  mount(<CustomerPlans />);
  const total = await screen.findByLabelText("22.000000000000000000000001 ATG");
  expect(total.compareDocumentPosition(screen.getByRole("link", { name: "充值支付余额" })) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  expect(screen.getByRole("navigation", { name: "套餐页面导航" })).toBeTruthy();
  expect(totalCredits("9007199254740993", "0.000000000000000000000001")).toBe("9007199254740993.000000000000000000000001");
});

it("starts with a collapsed exact allocation and keeps commission an explicit choice", async () => {
  vi.stubGlobal("fetch", vi.fn(async () => response(credits)));
  mount(<Checkout quote={quote} supportsRenewal changeOffers={vi.fn()} />);
  await waitFor(() => expect((screen.getByLabelText("本次使用 USD 支付余额") as HTMLInputElement).value).toBe(credits.usd_balance.available));
  const details = screen.getByText("精确调整抵扣金额").closest("details")!;
  expect(details.open).toBe(false);
  expect((screen.getByLabelText(/使用可用佣金补足/) as HTMLInputElement).checked).toBe(false);
  expect(screen.getByLabelText("6.999999999999999999 USD")).toBeTruthy();
  fireEvent.click(screen.getByText("精确调整抵扣金额")); expect(details.open).toBe(true);
  fireEvent.change(screen.getByLabelText("本次使用 USD 支付余额"), { target: { value: "1.000000000000000001" } });
  expect(screen.getByLabelText("8.999999999999999999 USD")).toBeTruthy();
});

it("creates a model-scoped Key in place, displays its Base URL and locks the secret until close", async () => {
  const posts: RequestInit[] = [], lock = vi.fn();
  vi.stubGlobal("fetch", vi.fn(async (_path: RequestInfo | URL, init?: RequestInit) => { if (init?.method === "POST") { posts.push(init); return response({ key: apiKey, secret: "synthetic_inline_secret" }); } return response({ items: [{ id: "public-model", name: "公开模型", capabilities: ["chat"], status: "active", prices: [] }] }); }));
  const { unmount } = mount(<KeyEditor initialModelIDs={["public-model"]} close={vi.fn()} onLockChange={lock} />);
  expect((screen.getByLabelText("不额外限定模型") as HTMLInputElement).checked).toBe(false);
  fireEvent.change(screen.getByLabelText("名称", { exact: true }), { target: { value: "模型接入应用" } });
  fireEvent.click(screen.getByRole("button", { name: "创建 Key", exact: true }));
  await screen.findByText("synthetic_inline_secret");
  expect(JSON.parse(String(posts[0].body)).model_ids).toEqual(["public-model"]);
  expect(screen.getByText("https://brand.example.test/v1")).toBeTruthy();
  expect(screen.getByRole("button", { name: "复制 Base URL", exact: true })).toBeTruthy();
  await waitFor(() => expect(lock).toHaveBeenLastCalledWith(true));
  unmount(); expect(lock).toHaveBeenLastCalledWith(false);
});

it("offers only a refresh action for an unknown Key status", async () => {
  vi.stubGlobal("fetch", vi.fn(async () => response({ items: [{ ...apiKey, status: "unknown" }] })));
  mount(<CustomerKeys />); await screen.findByText("生产应用");
  expect(screen.queryByRole("button", { name: "编辑配置" })).toBeNull();
  expect(screen.queryByRole("button", { name: "重置密钥" })).toBeNull();
  expect(screen.queryByRole("button", { name: "删除" })).toBeNull();
  expect(screen.getByRole("button", { name: "状态待核实，刷新记录" })).toBeTruthy();
});

it("locks new and other Key mutations while a reset result is unknown", async () => {
  vi.stubGlobal("fetch", vi.fn(async (_path: RequestInfo | URL, init?: RequestInit) => { if (init?.method === "POST") throw new TypeError("connection lost"); return response({ items: [apiKey, { ...apiKey, id: "key-two", name: "第二个应用" }] }); }));
  mount(<CustomerKeys />);
  fireEvent.click((await screen.findAllByRole("button", { name: "重置密钥" }))[0]); fireEvent.click(screen.getByRole("button", { name: "确认", exact: true }));
  await screen.findByRole("button", { name: "重试原操作" });
  expect((screen.getByRole("button", { name: "创建 Key", exact: true }) as HTMLButtonElement).disabled).toBe(true);
  expect(screen.getAllByRole("button", { name: "编辑配置" }).every(button => (button as HTMLButtonElement).disabled)).toBe(true);
  expect((screen.getAllByRole("button", { name: "重置密钥" })[1] as HTMLButtonElement).disabled).toBe(true);
});

it("queries all request records by status and resets the cursor when the search changes", async () => {
  const fetch = vi.fn(async (path: RequestInfo | URL) => response({ items: [call], next_cursor: String(path).includes("cursor=") || String(path).includes("q=") ? "" : "page-two" }));
  vi.stubGlobal("fetch", fetch); mount(<CustomerRequests />);
  fireEvent.click(await screen.findByRole("button", { name: "下一页" }));
  await waitFor(() => expect(fetch.mock.calls.some(([path]) => String(path).endsWith("requests?cursor=page-two"))).toBe(true));
  fireEvent.change(screen.getByLabelText("快捷查询状态"), { target: { value: "awaiting_usage" } });
  await waitFor(() => expect(fetch.mock.calls.some(([path]) => String(path).endsWith("requests?q=awaiting_usage"))).toBe(true));
  expect((screen.getByLabelText("搜索明细") as HTMLInputElement).value).toBe("awaiting_usage");
  expect(screen.queryByRole("button", { name: "上一页" })).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "清除", exact: true }));
  expect((screen.getByLabelText("搜索明细") as HTMLInputElement).value).toBe("");
});

it.each([true, false])("uses a shared KOL promotion only when the user's eligible offers contain it: %s", async eligible => {
  window.history.replaceState(null, "", "/workspace/plans?plan=plan-one&kol_activity=shared-kol");
  const posts: RequestInit[] = [];
  vi.stubGlobal("fetch", vi.fn(async (path: RequestInfo | URL, init?: RequestInit) => {
    if (init?.method === "POST") { posts.push(init); return response(quote); }
    const url = String(path);
    if (url.endsWith("/catalog")) return response({ items: [plan] });
    if (url.endsWith("/offers")) return response({ items: eligible ? [{ id: "shared-kol", name: "直属邀请活动", description: "有效加赠", kind: "kol", allow_combination: false }] : [] });
    return response({ items: [] });
  }));
  mount(<CustomerPlans initialPlan="plan-one" />);
  const select = await screen.findByLabelText("直属邀请 KOL 活动");
  await waitFor(() => { if (eligible) expect((select as HTMLSelectElement).value).toBe("shared-kol"); else expect(screen.getByText("链接中的活动当前不可用，可按当前可用条款继续购买。")).toBeTruthy(); });
  fireEvent.click(screen.getByRole("button", { name: "继续，核对成交条款" }));
  await waitFor(() => expect(posts).toHaveLength(1));
  expect(JSON.parse(String(posts[0].body))).toEqual({ plan_id: "plan-one", ...(eligible ? { kol_activity_id: "shared-kol" } : {}) });
});

it("provides complete UX translations with matching named placeholders", () => {
  const fields = (value: string) => [...value.matchAll(/\{([A-Za-z_][A-Za-z0-9_]*)\}/g)].map(match => match[1]).sort();
  for (const [source, message] of Object.entries(customerUX)) for (const locale of ["en", "ja", "ru"] as const) {
    expect(message[locale].trim()).not.toBe(""); expect(fields(message[locale])).toEqual(fields(source));
  }
});
