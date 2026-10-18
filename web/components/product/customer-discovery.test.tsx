// @vitest-environment jsdom
import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import { I18nProvider } from "@/components/i18n";
import { locales, translate, type Locale } from "@/lib/i18n";
import type { Model, Plan } from "@/lib/contracts";
import { activityShareURL, concessionPreview, percentage, qualificationProgress } from "./customer-discovery";
import { ConnectionGuide, CustomerModels, ModelCatalog } from "./customer-models";
import { ActivityForm, CustomerActivities } from "./customer-activities";
import { CustomerReferrals } from "./customer-referrals";
import { PlansPage, purchaseURL } from "./public";
import { Playground } from "./playground";

const scenario = vi.hoisted(() => ({ customer: true }));
vi.mock("./context", () => ({ useProduct: () => ({ context: { brand: { id: "brand", name: "Synthetic Brand", api_base_url: "https://brand.example.test/v1" }, operator: { id: "operator", name: "Original operator" } }, session: scenario.customer ? { kind: "customer", brand: { api_base_url: "https://brand.example.test/v1" }, operator: { id: "operator", name: "Original operator" } } : { kind: "anonymous" } }) }));
const models: Model[] = [
  { id: "selected/model", name: "Selected model", capabilities: ["chat"], status: "active", prices: [{ component: "input", amount_atg: "0.000000000000000001", unit: "token" }] },
  { id: "second-model", name: "Second model", capabilities: ["image"], status: "active", prices: [] },
  { id: "paused-model", name: "Paused model", capabilities: ["image"], status: "disabled", prices: [] },
];
const plan: Plan = { id: "plan-one", name: "Synthetic plan", price_usd: "10", purchase_atg: "100", gift_atg: "20", validity: "month", auto_renew_available: false, status: "active", version: 1 };
const referral = { invite_code: "original-code", invite_url: "https://brand.example.test/register?invite=original-code", direct_count: 3, indirect_count: 1, qualified: true, function_status: "active", confirmed_usd: "10", pending_usd: "2", threshold_usd: "10", direct_rate: "0.3", indirect_rate: "0.1" };
const page = { title: "", description: "", items: [], columns: [], operations: [], row_operations: [] };
const response = (value: unknown) => new Response(JSON.stringify(value), { status: 200 });
function show(component: React.ReactNode, locale: Locale = "zh") { const cache = new QueryClient({ defaultOptions: { queries: { retry: false } } }); return { ...render(<I18nProvider initialLocale={locale}><QueryClientProvider client={cache}>{component}</QueryClientProvider></I18nProvider>), cache }; }
afterEach(() => { cleanup(); vi.unstubAllGlobals(); scenario.customer = true; window.history.replaceState({}, "", "/"); });

it("previews concession and remaining commission with exact decimals and backend ATG truncation", () => {
  expect(concessionPreview("100", "20", "10", "0.3")).toEqual({ base: "120", gift: "12", total: "132", remainingPoints: "20", rate: "0.1" });
  expect(concessionPreview("9007199254740993", "0.000000000000000001", "10", "0.3")?.gift).toBe("900719925474099.3");
  expect(concessionPreview("0.000000000000000001", "0", "33.3333333333333333", "0.5")).toEqual({ base: "0.000000000000000001", gift: "0", total: "0.000000000000000001", remainingPoints: "16.6666666666666667", rate: "0.333333333333333333" });
  for (const points of ["0", "31", "1e1", "10.00000000000000001", "01", ""]) expect(concessionPreview("100", "20", points, "0.3")).toBeUndefined();
  expect(percentage("0.123456789012345678")).toBe("12.3456789012345678");
  expect(percentage("12.3456789012345678", true)).toBe("0.123456789012345678");
  expect(qualificationProgress("1", "1.000000000000000001").remaining).toBe("0.000000000000000001");
});

it("preserves invitation host and code in campaign links and retains selection through auth", () => {
  const link = new URL(activityShareURL("https://channel.example.test/register?invite=original%2Bcode&source=kol", "plan/a", "activity/b")!);
  expect(link.origin).toBe("https://channel.example.test");
  expect(link.pathname).toBe("/plans");
  expect(Object.fromEntries(link.searchParams)).toEqual({ invite: "original+code", source: "kol", plan: "plan/a", kol_activity: "activity/b" });
  expect(activityShareURL("javascript:alert(1)", "p", "a")).toBeUndefined();
  expect(activityShareURL("https://brand.example.test/register", "p", "a")).toBeUndefined();
  const guest = new URL(purchaseURL("plan/a", false, { plan: "plan/a", activity: "activity/b", invite: "original+code" }), "https://channel.example.test");
  expect(guest.pathname).toBe("/register");
  expect(guest.searchParams.get("invite")).toBe("original+code");
  expect(guest.searchParams.get("next")).toBe("/workspace/plans?plan=plan%2Fa&kol_activity=activity%2Fb");
  expect(purchaseURL("other", true, { plan: "plan/a", activity: "activity/b" })).toBe("/workspace/plans?plan=other");
});

it("combines capability and availability filters and links to the selected public model", () => {
  show(<ModelCatalog models={models} />);
  fireEvent.change(screen.getByLabelText("按模型能力筛选"), { target: { value: "image" } });
  fireEvent.change(screen.getByLabelText("按可用状态筛选"), { target: { value: "available" } });
  expect(screen.getByText("显示 1 / 3 个模型")).toBeTruthy();
  expect(screen.queryByRole("heading", { name: "Paused model" })).toBeNull();
  expect(screen.getByRole("link", { name: "接入此模型" }).getAttribute("href")).toBe("/workspace/models?model=second-model");
  fireEvent.click(screen.getByRole("button", { name: "清除筛选" }));
  expect(screen.getByText("显示 3 / 3 个模型")).toBeTruthy();
});

it("retains a removed selected model rather than substituting another model", () => {
  const view = show(<ConnectionGuide models={models} initialModel="selected/model" />);
  expect(screen.getByRole("link", { name: "打开模型体验" }).getAttribute("href")).toBe("/workspace/playground?model=selected%2Fmodel");
  view.rerender(<QueryClientProvider client={view.cache}><ConnectionGuide models={models.slice(1)} initialModel="selected/model" /></QueryClientProvider>);
  expect(screen.getByText("所选模型已不在当前目录，请明确选择其他模型；系统不会自动替换。")).toBeTruthy();
  expect((screen.getByLabelText("公开模型") as HTMLSelectElement).value).toBe("selected/model");
  expect(screen.getByText(/curl /).textContent).toContain('"model": "selected/model"');
  expect((screen.getByRole("button", { name: "复制代码" }) as HTMLButtonElement).disabled).toBe(true);
});

it("keeps inline Key creation and model selection through an unknown result and exact retry", async () => {
  window.history.replaceState({}, "", "/workspace/models?model=selected%2Fmodel&other=keep");
  const calls: RequestInit[] = [];
  vi.stubGlobal("fetch", vi.fn(async (_path: RequestInfo | URL, init?: RequestInit) => {
    if (init?.method === "POST") { calls.push(init); if (calls.length === 1) throw new TypeError("lost connection"); return response({ key: { id: "key", name: "Synthetic app", prefix: "prefix", status: "active", model_ids: ["selected/model"], budget: { limit_atg: null, period: "month", timezone: "UTC", spent_atg: "0", held_atg: "0", overage_atg: "0", window_start: null, window_end: null }, rate_limit_rpm: null, concurrency_limit: null, version: 1, created_at: "2026-10-18T00:00:00Z" }, secret: "synthetic-secret-only" }); }
    return response({ items: models });
  }));
  const { cache } = show(<CustomerModels />);
  fireEvent.click(await screen.findByRole("button", { name: "为此模型创建 Key" }));
  const editor = screen.getByLabelText("创建新的 API Key");
  fireEvent.change(within(editor).getByLabelText("名称", { exact: true }), { target: { value: "Synthetic app" } });
  fireEvent.click(within(editor).getByRole("button", { name: "创建 Key", exact: true }));
  await screen.findByRole("button", { name: "重试本次操作" });
  expect(screen.getAllByRole("button", { name: "接入此模型" }).every(button => (button as HTMLButtonElement).disabled)).toBe(true);
  act(() => { cache.setQueryData(["public-models"], { items: models.slice(1) }); });
  expect(screen.getByLabelText("名称", { exact: true }).matches(":disabled")).toBe(true);
  fireEvent.click(screen.getByRole("button", { name: "重试本次操作" }));
  await screen.findByText("synthetic-secret-only");
  expect(calls).toHaveLength(2);
  expect(JSON.parse(String(calls[0].body)).model_ids).toEqual(["selected/model"]);
  expect(calls[1].body).toBe(calls[0].body);
  expect((calls[1].headers as Headers).get("Idempotency-Key")).toBe((calls[0].headers as Headers).get("Idempotency-Key"));
  expect(new URL(window.location.href).searchParams.get("other")).toBe("keep");
});

it("updates the campaign preview and retries original terms after an unknown publication", async () => {
  const calls: RequestInit[] = [], published = vi.fn();
  vi.stubGlobal("fetch", vi.fn(async (_path: RequestInfo | URL, init?: RequestInit) => { calls.push(init!); if (calls.length === 1) throw new TypeError("lost connection"); return response({ id: "original-campaign", status: "published" }); }));
  show(<ActivityForm item={null} plans={[plan]} maximum="0.3" close={vi.fn()} published={published} />);
  fireEvent.change(screen.getByLabelText("本人让利百分点", { exact: true }), { target: { value: "10" } });
  const preview = screen.getByLabelText("活动效果预览");
  expect(preview.textContent).toContain("120 ATG");
  expect(preview.textContent).toContain("+12 ATG");
  expect(preview.textContent).toContain("20%");
  fireEvent.change(screen.getByLabelText("活动赠送预算 ATG", { exact: true }), { target: { value: "12.000000000000000001" } });
  fireEvent.click(screen.getByLabelText("我确认活动让利、期限和使用上限，发布后仅对有效直属受邀客户生效。"));
  fireEvent.click(screen.getByRole("button", { name: "确认发布活动" }));
  await screen.findByRole("button", { name: "重试原活动发布" });
  expect(screen.getByLabelText("本人让利百分点", { exact: true }).matches(":disabled")).toBe(true);
  expect((screen.getByRole("button", { name: "取消编辑" }) as HTMLButtonElement).disabled).toBe(true);
  fireEvent.click(screen.getByRole("button", { name: "重试原活动发布" }));
  await waitFor(() => expect(published).toHaveBeenCalledWith("original-campaign", "plan-one"));
  expect(JSON.parse(String(calls[0].body)).configuration).toMatchObject({ concession_rate: "0.1", gift_atg: "0", gift_budget_atg: "12.000000000000000001", package_id: "plan-one" });
  expect(calls[1].body).toBe(calls[0].body);
  expect((calls[1].headers as Headers).get("Idempotency-Key")).toBe((calls[0].headers as Headers).get("Idempotency-Key"));
});

it("provides a share link after publication without changing the original invitation", async () => {
  vi.stubGlobal("fetch", vi.fn(async (path: RequestInfo | URL, init?: RequestInit) => {
    if (init?.method === "POST") return response({ id: "new-campaign", status: "published", version: 1 });
    if (String(path).endsWith("/referrals")) return response(referral);
    if (String(path).endsWith("/catalog")) return response({ items: [plan] });
    return response(page);
  }));
  show(<CustomerActivities />);
  fireEvent.click(await screen.findByRole("button", { name: "创建推广活动" }));
  fireEvent.change(screen.getByLabelText("本人让利百分点", { exact: true }), { target: { value: "10" } });
  fireEvent.change(screen.getByLabelText("活动赠送预算 ATG", { exact: true }), { target: { value: "100" } });
  fireEvent.click(screen.getByLabelText("我确认活动让利、期限和使用上限，发布后仅对有效直属受邀客户生效。"));
  fireEvent.click(screen.getByRole("button", { name: "确认发布活动" }));
  await screen.findByText("活动已发布，可以分享给直属受邀客户。");
  expect(screen.getByText("https://brand.example.test/plans?invite=original-code&plan=plan-one&kol_activity=new-campaign")).toBeTruthy();
  expect(screen.getByRole("button", { name: "复制活动链接" })).toBeTruthy();
});

it("shows precise ordinary-user qualification progress without fetching the KOL wallet", async () => {
  const fetch = vi.fn(async () => response({ ...referral, qualified: false, confirmed_usd: "9.999999999999999999", pending_usd: "100" }));
  vi.stubGlobal("fetch", fetch);
  show(<CustomerReferrals />);
  await screen.findByRole("progressbar", { name: "KOL 资格进度" });
  expect(screen.getByLabelText("0.000000000000000001 USD")).toBeTruthy();
  expect(screen.queryByRole("heading", { name: "佣金钱包" })).toBeNull();
  expect(screen.queryByRole("link", { name: "佣金明细" })).toBeNull();
  expect(fetch).toHaveBeenCalledTimes(1);
});

it("loads the invited operator's plans and carries the campaign into registration and login", async () => {
  scenario.customer = false;
  window.history.replaceState({}, "", "/plans?plan=plan-one&kol_activity=campaign-one&invite=original-code");
  const fetch = vi.fn(async (path: RequestInfo | URL) => String(path).includes("/context") ? response({ brand: { id: "brand", name: "Synthetic Brand", api_base_url: "https://brand.example.test/v1", theme: {} }, operator: { id: "invited-operator", name: "Invited operator", kind: "channel", brand_id: "brand" }, theme: {}, availability: [], payment_methods: [], invitation: { code: "original-code", status: "valid" } }) : response({ items: [plan] }));
  vi.stubGlobal("fetch", fetch);
  show(<PlansPage />);
  const link = await screen.findByRole("link", { name: "继续，核实活动资格" });
  expect(fetch.mock.calls.some(([path]) => String(path) === "/api/v1/public/plans?invite_code=original-code")).toBe(true);
  expect(link.getAttribute("href")).toBe("/register?next=%2Fworkspace%2Fplans%3Fplan%3Dplan-one%26kol_activity%3Dcampaign-one&invite=original-code");
  expect(screen.getByRole("link", { name: "已有账号，登录核实活动" }).getAttribute("href")).toBe("/login?next=%2Fworkspace%2Fplans%3Fplan%3Dplan-one%26kol_activity%3Dcampaign-one");
  await screen.findAllByText("Invited operator");
});

it("keeps the linked model in the browser experience without sending a request", async () => {
  window.history.replaceState({}, "", "/workspace/playground?model=selected%2Fmodel");
  const fetch = vi.fn(async () => response({ items: models }));
  vi.stubGlobal("fetch", fetch);
  const { cache } = show(<Playground />);
  expect((await screen.findByLabelText("公开模型", { exact: true }) as HTMLSelectElement).value).toBe("selected/model");
  fireEvent.change(screen.getByLabelText("能力", { exact: true }), { target: { value: "chat" } });
  act(() => { cache.setQueryData(["my-models"], { items: models.slice(1) }); });
  expect((screen.getByLabelText("公开模型", { exact: true }) as HTMLSelectElement).value).toBe("selected/model");
  await waitFor(() => expect((screen.getByRole("button", { name: "确认计费并发送" }) as HTMLButtonElement).disabled).toBe(true));
  expect(fetch).toHaveBeenCalledTimes(1);
});


it.each(locales)("localizes the live campaign preview in %s while preserving authored names and precise terms", locale => {
  show(<ActivityForm item={null} plans={[plan]} maximum="0.3" close={vi.fn()} />, locale);
  const points = screen.getByLabelText(translate(locale, "本人让利百分点"), { exact: true });
  fireEvent.change(points, { target: { value: "10.0000000000000001" } });
  const preview = screen.getByLabelText(translate(locale, "活动效果预览"));
  expect(preview.textContent).toContain("Synthetic plan");
  expect(within(preview).getByText(translate(locale, "本人剩余直接佣金比例"))).toBeTruthy();
  expect((points as HTMLInputElement).value).toBe("10.0000000000000001");
  expect(preview.textContent).toContain(locale === "ru" ? "19,9999999999999999%" : "19.9999999999999999%");
});


it("includes a model's verified explicit output bound in generated integration code", () => {
  const entry: Model = { ...models[0], capability_details: [{ operation: "chat", streaming: true, input_types: ["text"], parameters: ["max_completion_tokens"], required_parameters: ["max_completion_tokens"], output_limit_parameter: "max_completion_tokens", max_input_bytes: 8192, max_output: 256 }] };
  show(<ConnectionGuide models={[entry]} initialModel={entry.id} />);
  expect(screen.getByText(/curl /).textContent).toContain('"max_completion_tokens": 256');
  expect(screen.getByText(/curl /).textContent).toContain('"model": "selected/model"');
});
