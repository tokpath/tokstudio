// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import { MonetaryProvider } from "./monetary";
import { CustomerWithdrawalApplications, CustomerWithdrawalRequest, type PayoutMethod } from "./customer-withdrawal-request";

afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

const methods: PayoutMethod[] = [
  { id: "bank", name: "银行卡", kind: "bank_cn", kind_label: "境内银行卡", summary: "填写银行卡资料。", flat_fee_money: "1", fee_rate: "0", minimum_money: "1", fields: [{ name: "payee_name", label: "收款人姓名", help: "须与户名一致。", kind: "text" }, { name: "bank_name", label: "开户银行", help: "银行名称。", kind: "text" }, { name: "account_number", label: "银行卡号", help: "不要空格。", kind: "text" }, { name: "branch_name", label: "开户支行", help: "支行名称。", kind: "text" }] },
  { id: "alipay", name: "支付宝", kind: "alipay", kind_label: "支付宝", summary: "填写与账号一致的实名和登录账号。", flat_fee_money: "0", fee_rate: "0.02", minimum_money: "1", fields: [{ name: "payee_name", label: "支付宝实名", help: "须与实名一致。", kind: "text" }, { name: "account", label: "支付宝账号", help: "手机号或邮箱。", kind: "text" }] },
];

function mount(content: React.ReactNode) {
  const cache = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={cache}><MonetaryProvider value={{ currency: "USD", atg_per_unit: "1", status: "locked", version: 1 }}>{content}</MonetaryProvider></QueryClientProvider>);
}

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
  expect(JSON.parse(String(fetch.mock.calls[0][1]?.body))).toEqual({ method_id: "alipay", amount_money: "10", recipient: { payee_name: "张三", account: "13800138000" } });
});

it("shows cancellation only on an application that is still awaiting review", () => {
  const cancel = { name: "cancel", label: "取消提现申请", method: "POST" as const, path: "/api/v1/me/withdrawals/{id}/cancel", fields: [] };
  const onCancel = vi.fn();
  mount(<CustomerWithdrawalApplications rows={[{ id: "one", status: "requested", values: { method_name: "支付宝", amount_money: "10", fee_money: "0.2", net_money: "9.8", recipient_reference: "支付宝实名：张三" }, allowed_actions: ["cancel"] }, { id: "two", status: "approved", values: { method_name: "支付宝", amount_money: "10", fee_money: "0.2", net_money: "9.8" }, allowed_actions: [] }]} cancel={cancel} onCancel={onCancel} />);
  expect(screen.getAllByRole("button", { name: "取消申请" })).toHaveLength(1);
  fireEvent.click(screen.getByRole("button", { name: "取消申请" }));
  expect(onCancel).toHaveBeenCalledWith(cancel, expect.objectContaining({ id: "one" }));
});
