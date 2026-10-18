// @vitest-environment jsdom
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { ReactNode } from "react";
import type { Quote } from "@/lib/contracts";
import { Checkout, CustomerPlans, PaymentAction } from "./customer-plans";

vi.mock("./context", () => ({ useProduct: () => ({ context: { payment_methods: [{ id: "bank", name: "银行转账", status: "ready" }] } }) }));
const credits = { customer_atg: { available: "0", held: "0", purchased_remaining: "0", gift_remaining: "0" }, usd_balance: { available: "3.000000000000000001", held: "0" }, commission_usd: { available: "2.000000000000000001", held: "0", frozen: "50", recovery_due: "0" }, qualification: { confirmed_usd: "0", pending_usd: "0", threshold_usd: "100", qualified_at: null, function_status: "normal" } };
const quote: Quote = { id: "quote-one", plan_id: "plan-one", plan_name: "通用套餐", operator_name: "本次经营方", amount_usd: "10.000000000000000001", purchase_atg: "20", plan_gift_atg: "2", operator_gift_atg: "0", kol_gift_atg: "0", total_atg: "22", validity: "month", terms_version: "terms-one", terms_summary: "按成交订单内购赠比例耗用。", expires_at: "2099-10-18T00:00:00Z", warnings: [] };
const order = { id: "order-one", status: "pending_payment", kind: "package", operator_name: "本次经营方", amount_usd: quote.amount_usd, paid_usd: "0", created_at: "2026-10-18T00:00:00Z", payment_action: { kind: "offline", instructions: "请核对原订单后付款" } };
const response = (body: unknown) => new Response(JSON.stringify(body), { status: 200 });
function mount(children: ReactNode) {
  const cache = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const wrapper = ({ children }: { children: ReactNode }) => <QueryClientProvider client={cache}>{children}</QueryClientProvider>;
  return { ...render(children, { wrapper }), cache };
}
function terms() { return screen.getByLabelText(/我确认本次价格/); }
beforeEach(() => { HTMLElement.prototype.scrollIntoView = vi.fn(); });
afterEach(() => { cleanup(); vi.unstubAllGlobals(); vi.useRealTimers(); vi.restoreAllMocks(); });

it("keeps present commission payment and future renewal permission as separate opt-ins", async () => {
  const fetch = vi.fn(async (_path: RequestInfo | URL, init?: RequestInit) => response(init?.method === "POST" ? order : credits));
  vi.stubGlobal("fetch", fetch);
  mount(<Checkout quote={quote} supportsRenewal changeOffers={vi.fn()} />);
  expect(screen.getByText("按成交订单内购赠比例耗用。")).toBeTruthy();
  expect(screen.queryByText("terms-one")).toBeNull();
  await waitFor(() => expect((screen.getByLabelText("本次使用 USD 支付余额") as HTMLInputElement).value).toBe(credits.usd_balance.available));
  expect((screen.getByLabelText(/使用可用佣金补足/) as HTMLInputElement).checked).toBe(false);
  expect((screen.getByLabelText("授权到期自动续费") as HTMLInputElement).checked).toBe(false);
  fireEvent.click(screen.getByLabelText(/使用可用佣金补足/));
  await waitFor(() => expect((screen.getByLabelText("本次使用可用佣金 USD") as HTMLInputElement).value).toBe(credits.commission_usd.available));
  fireEvent.click(screen.getByLabelText("授权到期自动续费"));
  expect((screen.getByLabelText("未来续费也允许使用可用佣金补足") as HTMLInputElement).checked).toBe(false);
  fireEvent.click(terms());
  fireEvent.click(screen.getByRole("button", { name: "确认订单，继续支付" }));
  await screen.findByText("订单已建立。请按下方状态完成支付与到账核实。");
  const call = fetch.mock.calls.find(item => item[1]?.method === "POST")!;
  expect(JSON.parse(String(call[1]?.body))).toEqual({ quote_id: "quote-one", terms_version: "terms-one", funding: { usd_balance: "3.000000000000000001", commission_usd: "2.000000000000000001", external_method_id: "bank" }, auto_renew: true, allow_commission: false });
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
  await waitFor(() => expect((screen.getByLabelText("本次使用 USD 支付余额") as HTMLInputElement).value).toBe(credits.usd_balance.available));
  fireEvent.click(terms());
  fireEvent.click(screen.getByRole("button", { name: "确认订单，继续支付" }));
  await screen.findByRole("button", { name: "重试原订单操作" });
  expect(lock).toHaveBeenLastCalledWith(true);
  expect((screen.getByRole("button", { name: "返回调整活动或重新报价" }) as HTMLButtonElement).disabled).toBe(true);
  act(() => { cache.setQueryData(["credits"], { ...credits, usd_balance: { available: "0", held: credits.usd_balance.available } }); });
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
    if (String(path).endsWith("/catalog")) return response({ items: ["plan-one", "plan-two"].map(id => ({ id, name: id, price_usd: "10", purchase_atg: "20", gift_atg: "2", validity: "month", auto_renew_available: true, status: "active", version: 1 })) });
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
  rerender(<PaymentAction order={{ ...order, status: "refunded", paid_usd: quote.amount_usd, delivered_at: "2026-10-18T00:00:01Z" }} />);
  expect(screen.queryByText("服务额度已到账，可以开始使用。")).toBeNull();
  expect(screen.getByText(/此订单已结束/)).toBeTruthy();
});

it("shows offline bank account, original currency, precise amount and instructions without chain fields", () => {
  mount(<PaymentAction order={{ ...order, payment_action: { kind: "offline", address: "合成测试银行账号 6222-0000", currency: "USD", amount: "10.000000000000000001", instructions: "仅限隔离测试：付款备注原订单编号，由财务核对银行回单。" } }} />);
  expect(screen.getByText("合成测试银行账号 6222-0000")).toBeTruthy();
  expect(screen.getByText(/付款备注原订单编号/)).toBeTruthy();
  expect(screen.getByRole("button", { name: "复制收款账号" })).toBeTruthy();
  expect(screen.getByText("付款币种")).toBeTruthy();
  expect(screen.getByText("应付原币金额")).toBeTruthy();
  expect(screen.getAllByLabelText("10.000000000000000001 USD", { exact: true })).toHaveLength(2);
  expect(screen.queryByText("准确网络")).toBeNull();
  expect(screen.queryByText("准确代币合约")).toBeNull();
  expect(screen.queryByText(/交易哈希/)).toBeNull();
});

it("requires the current renewal plan and explicit acceptance when takeover terms changed", async () => {
  const calls: RequestInit[] = [];
  const subscription = { id: "subscription-one", name: "原经营方套餐", status: "active", purchase_remaining_atg: "2", gift_remaining_atg: "0", effective_at: "2026-10-01T00:00:00Z", expires_at: "2026-11-01T00:00:00Z", auto_renew: true, allow_commission: false, requires_confirmation: true, requires_current_plan: true, changed_terms: ["新经营方续费价格为 12 USD"], version: 4 };
  vi.stubGlobal("fetch", vi.fn(async (path: RequestInfo | URL, init?: RequestInit) => {
    if (init?.method === "PATCH") { calls.push(init); return response({ id: subscription.id, status: "saved" }); }
    if (String(path).endsWith("/api/v1/me/plans")) return response({ items: [subscription] });
    if (String(path).endsWith("/api/v1/public/plans")) return response({ items: [{ id: "renewal-plan", quote_id: "renewal-quote", terms_summary: "新经营方的实际服务约定。", name: "新经营方套餐", price_usd: "12", purchase_atg: "20", gift_atg: "2", validity: "month", auto_renew_available: true, status: "active", version: 2 }] });
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
