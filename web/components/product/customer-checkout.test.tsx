// @vitest-environment jsdom
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { ReactNode } from "react";
import type { Quote } from "@/lib/contracts";
import { I18nProvider, LanguageControl } from "@/components/i18n";
import { translate } from "@/lib/i18n";
import { Checkout, CustomerPlans, PaymentAction } from "./customer-plans";

const { stripeLoader } = vi.hoisted(() => ({ stripeLoader: vi.fn() }));
vi.mock("@stripe/stripe-js", () => ({ loadStripe: stripeLoader }));
vi.mock("./context", () => ({ useProduct: () => ({ context: { payment_methods: [{ id: "bank", name: "银行转账", status: "ready" }, { id: "voucher", name: "kvoucher", status: "ready" }] } }) }));
const credits = { customer_atg: { available: "0", held: "0", purchased_remaining: "0", gift_remaining: "0" }, money_balance: { available: "3.000000000000000001", held: "0" }, commission_money: { available: "2.000000000000000001", held: "0", frozen: "50", recovery_due: "0" }, qualification: { confirmed_money: "0", pending_money: "0", threshold_money: "100", qualified_at: null, function_status: "normal" } };
const quote: Quote = { id: "quote-one", plan_id: "plan-one", plan_name: "通用套餐", operator_name: "本次经营方", amount_money: "10.000000000000000001", purchase_atg: "20", plan_gift_atg: "2", operator_gift_atg: "0", kol_gift_atg: "0", total_atg: "22", validity: "month", terms_version: "terms-one", terms_summary: "按成交订单内购赠比例耗用。", expires_at: "2099-10-18T00:00:00Z", warnings: [] };
const order = { id: "order-one", status: "pending_payment", kind: "package", operator_name: "本次经营方", amount_money: quote.amount_money, paid_money: "0", created_at: "2026-10-18T00:00:00Z", payment_action: { kind: "offline", instructions: "请核对原订单后付款" } };
const response = (body: unknown) => new Response(JSON.stringify(body), { status: 200 });
function mount(children: ReactNode) {
  const cache = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const wrapper = ({ children }: { children: ReactNode }) => <QueryClientProvider client={cache}>{children}</QueryClientProvider>;
  return { ...render(children, { wrapper }), cache };
}
function terms() { return screen.getByLabelText(/我确认本次价格/); }
beforeEach(() => { HTMLElement.prototype.scrollIntoView = vi.fn(); });
afterEach(() => { cleanup(); sessionStorage.clear(); vi.unstubAllGlobals(); vi.useRealTimers(); vi.restoreAllMocks(); document.cookie = "NEXT_LOCALE=; Max-Age=0; Path=/"; document.documentElement.lang = "zh-CN"; });

it("keeps the Stripe form mounted while using supported payment locales for Hindi, Malay and Thai", async () => {
  const element = { mount: vi.fn(), destroy: vi.fn(), on: vi.fn((event: string, callback: () => void) => { if (event === "ready") callback(); }) };
  const elements = { create: vi.fn(() => element), update: vi.fn() };
  const stripe = { elements: vi.fn(() => elements) };
  const stripeOrder = { ...order, payment_action: { kind: "stripe", client_secret: "synthetic-payment-secret", publishable_key: "pk_test_synthetic" } };
  stripeLoader.mockReset().mockResolvedValue(stripe);
  vi.stubGlobal("fetch", vi.fn(async () => response(stripeOrder)));
  mount(<I18nProvider initialLocale="hi"><LanguageControl /><PaymentAction order={stripeOrder} /></I18nProvider>);
  await waitFor(() => expect(stripe.elements).toHaveBeenCalledWith({ clientSecret: "synthetic-payment-secret", locale: "en" }));
  expect(screen.getByRole("button", { name: translate("hi", "确认通道支付"), exact: true })).toBeTruthy();
  for (const locale of ["ms", "th"] as const) {
    fireEvent.change(screen.getByTestId("language-selector"), { target: { value: locale } });
    await waitFor(() => expect(elements.update).toHaveBeenLastCalledWith({ locale }));
    expect(screen.getByRole("button", { name: translate(locale, "确认通道支付"), exact: true })).toBeTruthy();
  }
  expect(stripe.elements).toHaveBeenCalledTimes(1);
  expect(element.mount).toHaveBeenCalledTimes(1);
  expect(element.destroy).not.toHaveBeenCalled();
});

it("keeps present commission payment and future renewal permission as separate opt-ins", async () => {
  const fetch = vi.fn(async (_path: RequestInfo | URL, init?: RequestInit) => response(init?.method === "POST" ? order : credits));
  vi.stubGlobal("fetch", fetch);
  mount(<Checkout quote={quote} supportsRenewal changeOffers={vi.fn()} />);
  expect(screen.getByText("按成交订单内购赠比例耗用。")).toBeTruthy();
  expect(screen.queryByText("terms-one")).toBeNull();
  await waitFor(() => expect((screen.getByLabelText("本次使用 结算币种 支付余额") as HTMLInputElement).value).toBe(credits.money_balance.available));
  expect((screen.getByLabelText(/使用可用佣金补足/) as HTMLInputElement).checked).toBe(false);
  expect((screen.getByLabelText("授权到期自动续费") as HTMLInputElement).checked).toBe(false);
  fireEvent.click(screen.getByLabelText(/使用可用佣金补足/));
  await waitFor(() => expect((screen.getByLabelText("本次使用可用佣金 结算币种") as HTMLInputElement).value).toBe(credits.commission_money.available));
  fireEvent.click(screen.getByLabelText("授权到期自动续费"));
  expect((screen.getByLabelText("未来续费也允许使用可用佣金补足") as HTMLInputElement).checked).toBe(false);
  fireEvent.click(terms());
  fireEvent.click(screen.getByRole("button", { name: "确认订单，继续支付" }));
  await screen.findByText("订单已建立。请按下方状态完成支付与到账核实。");
  const call = fetch.mock.calls.find(item => item[1]?.method === "POST")!;
  expect(JSON.parse(String(call[1]?.body))).toEqual({ quote_id: "quote-one", terms_version: "terms-one", funding: { money_balance: "3.000000000000000001", commission_money: "2.000000000000000001", external_method_id: "bank" }, auto_renew: true, allow_commission: false });
});

it("retries an unknown order with the original key and body after balance and quote expiry change", async () => {
  const attempts: RequestInit[] = [], lock = vi.fn(), change = vi.fn();
  const fetch = vi.fn(async (_path: RequestInfo | URL, init?: RequestInit) => {
    if (init?.method === "POST") { attempts.push(init); if (attempts.length === 1) throw new TypeError("connection lost"); return response(order); }
    return response(credits);
  });
  vi.stubGlobal("fetch", fetch);
  const nearExpiry = { ...quote, expires_at: new Date(Date.now() + 60000).toISOString() };
  const { cache, rerender } = mount(<Checkout quote={nearExpiry} supportsRenewal changeOffers={change} lock={lock} />);
  await waitFor(() => expect((screen.getByLabelText("本次使用 结算币种 支付余额") as HTMLInputElement).value).toBe(credits.money_balance.available));
  fireEvent.click(terms());
  fireEvent.click(screen.getByRole("button", { name: "确认订单，继续支付" }));
  await screen.findByRole("button", { name: "重试原订单操作" });
  expect(lock).toHaveBeenLastCalledWith(true);
  expect((screen.getByRole("button", { name: "返回调整活动或重新报价" }) as HTMLButtonElement).disabled).toBe(true);
  act(() => { cache.setQueryData(["credits"], { ...credits, money_balance: { available: "0", held: credits.money_balance.available } }); });
  rerender(<Checkout quote={{ ...nearExpiry, expires_at: "2000-01-01T00:00:00Z" }} supportsRenewal changeOffers={change} lock={lock} />);
  const retry = screen.getByRole("button", { name: "重试原订单操作" });
  expect((retry as HTMLButtonElement).disabled).toBe(false);
  fireEvent.click(retry);
  await screen.findByText("订单已建立。请按下方状态完成支付与到账核实。");
  expect(attempts).toHaveLength(2);
  expect(attempts[1].body).toBe(attempts[0].body);
  expect((attempts[1].headers as Headers).get("Idempotency-Key")).toBe((attempts[0].headers as Headers).get("Idempotency-Key"));
  expect(change).not.toHaveBeenCalled();
  await waitFor(() => expect(lock).toHaveBeenLastCalledWith(false));
});

it("blocks plan replacement and page links while a quote result is unknown", async () => {
  const fetch = vi.fn(async (path: RequestInfo | URL, init?: RequestInit) => {
    if (init?.method === "POST") throw new TypeError("connection lost");
    if (String(path).endsWith("/catalog")) return response({ items: ["plan-one", "plan-two"].map(id => ({ id, name: id, price_money: "10", purchase_atg: "20", gift_atg: "2", validity: "month", auto_renew_available: true, status: "active", version: 1 })) });
    return response({ items: [] });
  });
  vi.stubGlobal("fetch", fetch);
  mount(<CustomerPlans initialPlan="plan-one" />);
  fireEvent.click(await screen.findByRole("button", { name: "继续，核对成交条款" }));
  await screen.findByRole("button", { name: "重试原报价" });
  expect((screen.getByRole("button", { name: "选择此套餐" }) as HTMLButtonElement).disabled).toBe(true);
  fireEvent.click(screen.getByRole("link", { name: "查看订单与到账" }));
  await screen.findByText("本次操作正在处理或结果待核实，请先完成原操作核实，再离开此页。");
  const unload = new Event("beforeunload", { cancelable: true });
  window.dispatchEvent(unload);
  expect(unload.defaultPrevented).toBe(true);
});

it("keeps payment instructions hidden for expired and refunded orders", () => {
  const { rerender } = mount(<PaymentAction order={{ ...order, expires_at: "2000-01-01T00:00:00Z" }} />);
  expect(screen.queryByText("完成本订单付款")).toBeNull();
  expect(screen.getByText(/付款期限已过/)).toBeTruthy();
  rerender(<PaymentAction order={{ ...order, status: "refunded", paid_money: quote.amount_money, delivered_at: "2026-10-18T00:00:01Z" }} />);
  expect(screen.queryByText("服务额度已到账，可以开始使用。")).toBeNull();
  expect(screen.getByText(/此订单已结束/)).toBeTruthy();
});

it("removes payment and method changes when a displayed package order reaches its deadline", async () => {
  vi.useFakeTimers();
  const expiring = {
    ...order, expires_at: new Date(Date.now() + 1000).toISOString(), method_change_allowed: true,
    payment_action: { kind: "form" as const, gkash_opened: false },
  };
  mount(<PaymentAction order={expiring} onChangeMethod={vi.fn()} />);
  expect(screen.getByRole("button", { name: "前往 GKash 付款" })).toBeTruthy();
  expect(screen.getByRole("button", { name: "改用其它付款方式" })).toBeTruthy();

  await act(async () => { await vi.advanceTimersByTimeAsync(1000); });
  expect(screen.getByText("已过期")).toBeTruthy();
  expect(screen.queryByRole("button", { name: "前往 GKash 付款" })).toBeNull();
  expect(screen.queryByRole("button", { name: "改用其它付款方式" })).toBeNull();
  expect(screen.getByRole("link", { name: "重新选择套餐" }).getAttribute("href")).toBe("/workspace/plans#buy-plans");
});

it.each([
  { state: { status: "pending", paid_money: "1" }, label: "部分付款" },
  { state: { status: "expired", has_verified_receipt: true }, label: "待核实" },
  { state: { status: "expired", payment_state: "awaiting_settlement" }, label: "待核实结算金额" },
])("preserves $label for a receipt on an order past its deadline", ({ state, label }) => {
  mount(<PaymentAction order={{ ...order, ...state, expires_at: "2000-01-01T00:00:00Z" }} />);
  expect(screen.getByText(label)).toBeTruthy();
  expect(screen.queryByText("完成本订单付款")).toBeNull();
  expect(screen.queryByRole("link", { name: "重新选择套餐" })).toBeNull();
});

it("opens a new Gkash form once and returns to the same payment window", async () => {
  const change = vi.fn();
  const payment = { ...order, method_change_allowed: true, payment_action: { kind: "form", gkash_opened: false, currency: "MYR", amount: "4.18", instructions: "请在 Gkash 完成付款。" } };
  const windowHandle = { closed: false, focus: vi.fn() };
  const open = vi.spyOn(window, "open").mockReturnValue(windowHandle as unknown as Window);
  const submit = vi.spyOn(HTMLFormElement.prototype, "submit").mockImplementation(() => {});
  const fetch = vi.fn(async () => response({
    id: payment.id, status: "payment_ready", gkash_generation: 1, payment_action: { kind: "form", url: "https://payments.example.test/checkout", fields: { v_cartid: "fresh-cart" } },
  }));
  vi.stubGlobal("fetch", fetch);
  const { rerender } = mount(<PaymentAction onChangeMethod={change} order={payment} />);
  expect(document.querySelector("form[action]")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "前往 GKash 付款" }));
  fireEvent.click(await screen.findByRole("button", { name: "重新打开 GKash 付款" }));
  expect(windowHandle.focus).toHaveBeenCalledTimes(1);
  expect(open).toHaveBeenCalledTimes(1);
  expect(submit).toHaveBeenCalledTimes(1);
  expect(fetch).toHaveBeenCalledTimes(1);
  fireEvent.click(screen.getByRole("button", { name: "改用其它付款方式" }));
  expect(change).toHaveBeenCalledTimes(1);
  rerender(<PaymentAction order={{ ...payment, status: "completed", paid_money: payment.amount_money }} />);
  expect(screen.queryByRole("button", { name: "重新打开 GKash 付款" })).toBeNull();
  windowHandle.closed = true;
});

it("opens the server-issued form after creating a package order", async () => {
  const payment = { ...order, id: "auto-package", payment_action: { kind: "form", gkash_opened: false } };
  const windowHandle = { closed: false, focus: vi.fn() };
  const open = vi.spyOn(window, "open").mockReturnValue(windowHandle as unknown as Window);
  const submitted: FormData[] = [];
  vi.spyOn(HTMLFormElement.prototype, "submit").mockImplementation(function (this: HTMLFormElement) { submitted.push(new FormData(this)); });
  const fetch = vi.fn(async (path: RequestInfo | URL, init?: RequestInit) => {
    if (init?.method !== "POST") return response(credits);
    if (String(path) === "/api/v1/me/orders") return response(payment);
    return response({ id: payment.id, status: "payment_ready", gkash_generation: 1, payment_action: { kind: "form", url: "https://payments.example.test/checkout", fields: { v_cartid: "auto-cart" } } });
  });
  vi.stubGlobal("fetch", fetch);
  mount(<Checkout quote={quote} supportsRenewal={false} changeOffers={vi.fn()} />);
  fireEvent.change(screen.getByRole("combobox", { name: "支付方式" }), { target: { value: "voucher" } });
  fireEvent.click(terms());
  fireEvent.click(screen.getByRole("button", { name: "确认订单，继续支付" }));
  await screen.findByRole("button", { name: "重新打开 GKash 付款" });
  expect(fetch.mock.calls.filter(([, options]) => options?.method === "POST").map(([path]) => path)).toEqual([
    "/api/v1/me/orders", "/api/v1/me/orders/auto-package/payment/open",
  ]);
  expect(open).toHaveBeenCalledTimes(1);
  expect(submitted.map(form => form.get("v_cartid"))).toEqual(["auto-cart"]);
  windowHandle.closed = true;
});

it("lets the customer cancel an unpaid order before sending money", async () => {
  const fetch = vi.fn(async (_path: RequestInfo | URL, init?: RequestInit) => response(init?.method === "POST" ? { ...order, status: "closed", cancel_allowed: false } : order));
  vi.stubGlobal("fetch", fetch);
  mount(<PaymentAction order={{ ...order, cancel_allowed: true, payment_action: { kind: "offline", address: "TEST-ONLY", currency: "USD", amount: "80", instructions: "测试收款说明" } }} />);
  fireEvent.click(screen.getByRole("button", { name: "取消订单" }));
  expect(screen.getByText(/还没有记录到付款/)).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "确认取消订单" }));
  await screen.findByText(/订单已取消/);
  const call = fetch.mock.calls.find(item => item[1]?.method === "POST")!;
  expect(String(call[0])).toBe("/api/v1/me/orders/order-one/cancel");
  expect(screen.queryByText("完成本订单付款")).toBeNull();
});

it("shows offline bank account, original currency, precise amount and instructions without chain fields", () => {
  mount(<PaymentAction order={{ ...order, payment_action: { kind: "offline", address: "合成测试银行账号 6222-0000", currency: "USD", amount: "10.000000000000000001", instructions: "仅限隔离测试：付款备注原订单编号，由财务核对银行回单。" } }} />);
  expect(screen.getByText("合成测试银行账号 6222-0000")).toBeTruthy();
  expect(screen.getByText(/付款备注原订单编号/)).toBeTruthy();
  expect(screen.getByRole("button", { name: "复制收款信息" })).toBeTruthy();
  expect(screen.getByText("付款币种")).toBeTruthy();
  expect(screen.getByText("应付原币金额")).toBeTruthy();
  expect(screen.getAllByLabelText("10.000000000000000001 USD", { exact: true })).toHaveLength(2);
  expect(screen.queryByText("准确网络")).toBeNull();
  expect(screen.queryByText("准确代币合约")).toBeNull();
  expect(screen.queryByText(/交易哈希/)).toBeNull();
});

it("requires the current renewal plan and explicit acceptance when takeover terms changed", async () => {
  const calls: RequestInit[] = [];
  const subscription = { id: "subscription-one", name: "原经营方套餐", status: "active", purchase_remaining_atg: "2", gift_remaining_atg: "0", effective_at: "2026-10-01T00:00:00Z", expires_at: "2026-11-01T00:00:00Z", auto_renew: true, allow_commission: false, requires_confirmation: true, requires_current_plan: true, changed_terms: ["新经营方续费价格为 12 结算币种"], version: 4 };
  vi.stubGlobal("fetch", vi.fn(async (path: RequestInfo | URL, init?: RequestInit) => {
    if (init?.method === "PATCH") { calls.push(init); return response({ id: subscription.id, status: "saved" }); }
    if (String(path).endsWith("/api/v1/me/plans")) return response({ items: [subscription] });
    if (String(path).endsWith("/api/v1/public/plans")) return response({ items: [{ id: "renewal-plan", quote_id: "renewal-quote", terms_summary: "新经营方的实际服务约定。", name: "新经营方套餐", price_money: "12", purchase_atg: "20", gift_atg: "2", validity: "month", auto_renew_available: true, status: "active", version: 2 }] });
    return response({ items: [] });
  }));
  mount(<CustomerPlans />);
  const save = await screen.findByRole("button", { name: "保存续费设置" });
  expect((save as HTMLButtonElement).disabled).toBe(true);
  fireEvent.change(await screen.findByLabelText("接管后续费套餐"), { target: { value: "renewal-plan" } });
  expect((save as HTMLButtonElement).disabled).toBe(true);
  expect(screen.getByText("新经营方的实际服务约定。")).toBeTruthy();
  fireEvent.click(screen.getByLabelText("确认以上新条款"));
  fireEvent.click(screen.getByLabelText("续费可用佣金补足"));
  fireEvent.click(save);
  await screen.findByText("续费设置已保存");
  expect(JSON.parse(String(calls[0].body))).toEqual({ auto_renew: true, allow_commission: true, version: 4, confirmed_changed_terms: true, plan_id: "renewal-plan", quote_id: "renewal-quote" });
});

it("allows stopping renewal without accepting changed terms and clears future commission permission", async () => {
  const calls: RequestInit[] = [];
  const subscription = { id: "subscription-one", name: "已授权套餐", status: "active", purchase_remaining_atg: "2", gift_remaining_atg: "0", effective_at: "2026-10-01T00:00:00Z", expires_at: "2026-11-01T00:00:00Z", auto_renew: true, allow_commission: true, requires_confirmation: true, changed_terms: ["下一次续费涨价"], version: 4 };
  vi.stubGlobal("fetch", vi.fn(async (path: RequestInfo | URL, init?: RequestInit) => {
    if (init?.method === "PATCH") { calls.push(init); return response({ id: subscription.id, status: "saved" }); }
    return response({ items: String(path).endsWith("/api/v1/me/plans") ? [subscription] : [] });
  }));
  mount(<CustomerPlans />);
  fireEvent.click(await screen.findByLabelText("到期自动续费", { exact: true }));
  expect((screen.getByLabelText("续费可用佣金补足") as HTMLInputElement).checked).toBe(false);
  expect(screen.getByLabelText("续费可用佣金补足").matches(":disabled")).toBe(true);
  fireEvent.click(screen.getByRole("button", { name: "保存续费设置" }));
  await screen.findByText("续费设置已保存");
  expect(JSON.parse(String(calls[0].body))).toEqual({ auto_renew: false, allow_commission: false, version: 4, confirmed_changed_terms: false, plan_id: "" });
});
