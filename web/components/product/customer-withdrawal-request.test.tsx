// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import { allowNavigation } from "@/lib/navigation-guard";
import { translate } from "@/lib/i18n";
import { I18nProvider } from "@/components/i18n";
import { MonetaryProvider } from "./monetary";
import { CustomerWithdrawalApplications, CustomerWithdrawalRequest, type PayoutMethod } from "./customer-withdrawal-request";

afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

const methods: PayoutMethod[] = [
  { id: "bank", policy_id: "bank-v1", name: "银行卡", kind: "bank_cn", kind_label: "境内银行卡", summary: "填写银行卡资料。", flat_fee_money: "1", fee_rate: "0", minimum_money: "1", fields: [{ name: "payee_name", label: "收款人姓名", help: "须与户名一致。", kind: "text" }, { name: "bank_name", label: "开户银行", help: "银行名称。", kind: "text" }, { name: "account_number", label: "银行卡号", help: "不要空格。", kind: "text" }, { name: "branch_name", label: "开户支行", help: "支行名称。", kind: "text" }] },
  { id: "alipay", policy_id: "alipay-v1", name: "支付宝", kind: "alipay", kind_label: "支付宝", summary: "填写与账号一致的实名和登录账号。", flat_fee_money: "0", fee_rate: "0.02", minimum_money: "1", fields: [{ name: "payee_name", label: "支付宝实名", help: "须与实名一致。", kind: "text" }, { name: "account", label: "支付宝账号", help: "手机号或邮箱。", kind: "text" }] },
];

function mount(content: React.ReactNode) {
  const cache = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={cache}><MonetaryProvider value={{ currency: "USD", atg_per_unit: "1", status: "locked", version: 1 }}>{content}</MonetaryProvider></QueryClientProvider>);
}

it.each(["zh", "en", "ja", "ru"] as const)("explains invalid USDT addresses and allows correction without losing the amount (%s)", async locale => {
  const t = (source: string) => translate(locale, source, undefined, "USD");
  const message = t("USDT 钱包地址无效或与所选网络不匹配，请核对钱包地址和网络后重新提交。");
  const attempts: {key: string | null; body: any}[] = [];
  vi.stubGlobal("fetch", vi.fn(async (_url, init: RequestInit) => {
    attempts.push({key: new Headers(init.headers).get("Idempotency-Key"), body: JSON.parse(String(init.body))});
    return attempts.length === 1
      ? new Response(JSON.stringify({error: {code: "invalid_payout_address", message, request_id: "usdt-request", retryable: false}}), {status: 400})
      : new Response(JSON.stringify({id: "usdt-withdrawal", status: "requested", message: t("已提交提现申请。")}), {status: 200});
  }));
  const usdt: PayoutMethod = {id: "usdt", policy_id: "usdt-v1", name: "USDT", kind: "usdt", kind_label: "USDT", summary: "", flat_fee_money: "0", fee_rate: "0.03", minimum_money: "1", fields: [
    {name: "network", label: "网络", help: "", kind: "select", options: [{value: "TRC20", label: "TRC20"}, {value: "ERC20", label: "ERC20"}]},
    {name: "address", label: "钱包地址", help: "", kind: "text"},
  ]};
  mount(<I18nProvider initialLocale={locale}><CustomerWithdrawalRequest methods={[usdt]} digits={2} walletName="支付余额" submitPath="/api/v1/me/balance-withdrawals" /></I18nProvider>);
  const amount = screen.getByLabelText(t("提现金额 结算币种"), {exact: false}) as HTMLInputElement;
  const address = screen.getByLabelText(t("钱包地址")) as HTMLInputElement;
  fireEvent.change(amount, {target: {value: "15"}});
  fireEvent.change(screen.getByLabelText(t("网络")), {target: {value: "TRC20"}});
  fireEvent.change(address, {target: {value: "test"}});
  fireEvent.click(screen.getByRole("button", {name: t("提交提现申请")}));
  expect((await screen.findByRole("alert")).textContent).toContain(message);
  expect(amount.value).toBe("15");
  expect(address.value).toBe("test");
  expect(amount.closest("fieldset")?.disabled).toBe(false);
  expect(allowNavigation()).toBe(true);
  expect(screen.getAllByLabelText("14.55 USD").length).toBeGreaterThan(0);
  fireEvent.change(address, {target: {value: "TLa2f6VPqDgRE67v1736s7bJ8Ray5wYjU7"}});
  fireEvent.click(screen.getByRole("button", {name: t("提交提现申请")}));
  await screen.findByText(t("已提交提现申请。"));
  expect(attempts).toHaveLength(2);
  expect(attempts[1].key).not.toBe(attempts[0].key);
  expect(attempts[1].body).toEqual({method_id: "usdt", policy_id: "usdt-v1", amount_money: "15", recipient: {network: "TRC20", address: "TLa2f6VPqDgRE67v1736s7bJ8Ray5wYjU7"}});
});

it("compares fees while switching payout methods and submits the selected materials once", async () => {
  const fetch = vi.fn(async () => new Response(JSON.stringify({ id: "withdrawal-one", status: "requested", message: "已提交提现申请。" }), { status: 200 }));
  vi.stubGlobal("fetch", fetch);
  mount(<CustomerWithdrawalRequest methods={methods} digits={2} />);
  fireEvent.change(screen.getByLabelText("提现金额 USD", { exact: false }), { target: { value: "10" } });
  expect(screen.getAllByLabelText("1 USD").length).toBeGreaterThan(0);
  expect(screen.getByLabelText("0.2 USD")).toBeTruthy();
  fireEvent.click(screen.getByRole("radio", { name: /支付宝/ }));
  fireEvent.change(screen.getByLabelText("支付宝实名"), { target: { value: "张三" } });
  fireEvent.change(screen.getByLabelText("支付宝账号"), { target: { value: "13800138000" } });
  fireEvent.click(screen.getByRole("button", { name: "提交提现申请" }));
  await waitFor(() => expect(fetch).toHaveBeenCalledTimes(1));
  expect(JSON.parse(String(fetch.mock.calls[0][1]?.body))).toEqual({ method_id: "alipay", policy_id: "alipay-v1", amount_money: "10", recipient: { payee_name: "张三", account: "13800138000" } });
});

it("calls the balance withdrawal net the amount the customer receives", () => {
  mount(<CustomerWithdrawalRequest methods={methods} digits={2} walletName="支付余额" />);
  fireEvent.change(screen.getByLabelText("提现金额 USD", { exact: false }), { target: { value: "10" } });
  expect(screen.getAllByText("实得").length).toBeGreaterThan(0);
  expect(screen.queryByText("实付")).toBeNull();
  cleanup();
  const cancel = { name: "cancel", label: "取消提现申请", method: "POST" as const, path: "/api/v1/me/balance-withdrawals/{id}/cancel", fields: [] };
  mount(<CustomerWithdrawalApplications walletName="支付余额" rows={[{ id: "one", status: "requested", values: { method_name: "支付宝", amount_money: "10", fee_money: "0.5", net_money: "9.5", recipient_reference: "支付宝实名：张三" }, allowed_actions: ["cancel"] }]} cancel={cancel} onCancel={() => {}} />);
  expect(screen.getByText("实得")).toBeTruthy();
  expect(screen.queryByText("实付")).toBeNull();
});

it("shows cancellation only on an application that is still awaiting review", () => {
  const cancel = { name: "cancel", label: "取消提现申请", method: "POST" as const, path: "/api/v1/me/withdrawals/{id}/cancel", fields: [] };
  const onCancel = vi.fn();
  mount(<CustomerWithdrawalApplications rows={[{ id: "one", status: "requested", values: { method_name: "支付宝", amount_money: "10", fee_money: "0.2", net_money: "9.8", recipient_reference: "支付宝实名：张三" }, allowed_actions: ["cancel"] }, { id: "two", status: "approved", values: { method_name: "支付宝", amount_money: "10", fee_money: "0.2", net_money: "9.8" }, allowed_actions: [] }]} cancel={cancel} onCancel={onCancel} />);
  expect(screen.getAllByRole("button", { name: "取消提现申请" })).toHaveLength(1);
  fireEvent.click(screen.getByRole("button", { name: "取消提现申请" }));
  expect(onCancel).toHaveBeenCalledWith(cancel, expect.objectContaining({ id: "one" }));
});

it("locks an uncertain application and retries the same materials with the same key", async () => {
  const attempts: {key:string|null;body:unknown}[]=[];
  vi.stubGlobal("fetch",vi.fn(async (_url, init:RequestInit) => {
    attempts.push({key:new Headers(init.headers).get("Idempotency-Key"),body:JSON.parse(String(init.body))});
    if(attempts.length===1) throw new TypeError("response lost after commit");
    return new Response(JSON.stringify({id:"one",status:"requested"}),{status:200});
  }));
  mount(<CustomerWithdrawalRequest methods={methods} digits={2} />);
  const amount=screen.getByLabelText("提现金额 USD",{exact:false}) as HTMLInputElement;
  fireEvent.change(amount,{target:{value:"10"}});
  fireEvent.click(screen.getByRole("radio",{name:/支付宝/}));
  fireEvent.change(screen.getByLabelText("支付宝实名"),{target:{value:"张三"}});
  fireEvent.change(screen.getByLabelText("支付宝账号"),{target:{value:"13800138000"}});
  fireEvent.click(screen.getByRole("button",{name:"提交提现申请"}));
  await screen.findByRole("button",{name:"重试本次操作"});
  expect(amount.closest("fieldset")?.disabled).toBe(true);
  expect(allowNavigation()).toBe(false);
  fireEvent.click(screen.getByRole("button",{name:"重试本次操作"}));
  await waitFor(()=>expect(attempts.length).toBe(2));
  expect(attempts[1]).toEqual(attempts[0]);
  await waitFor(()=>expect(amount.closest("fieldset")?.disabled).toBe(false));
  expect(allowNavigation()).toBe(true);
});
