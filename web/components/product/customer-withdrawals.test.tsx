// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import type { Operation, ResourcePage } from "@/lib/contracts";
import { ResourceCollection } from "./operator-resources";
import { OperationForm } from "./operation-form";

vi.mock("./context", () => ({ useProduct: () => ({ session: { kind: "customer", customer: { id: "customer-one" } } }) }));
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });
function mount(content: React.ReactNode) { const cache = new QueryClient({ defaultOptions: { queries: { retry: false } } }); return render(<QueryClientProvider client={cache}>{content}</QueryClientProvider>); }

it("offers cancellation only for the server-authorized pending withdrawal and confirms its exact request", async () => {
  const cancel: Operation = { name: "cancel", label: "取消提现申请", method: "POST", path: "/api/v1/me/withdrawals/{id}/cancel", fields: [], confirmation: "确认取消这笔尚未审核的提现申请？取消成功后释放本笔佣金占用。" };
  const page: ResourcePage = { title: "佣金提现", description: "审核前可以取消本人申请。", items: ["requested", "approved", "paying", "unknown", "paid", "cancelled", "failed"].map(status => ({ id: `withdrawal-${status}`, status, values: { amount_money: "5.000000000000000001" }, allowed_actions: status === "requested" ? ["cancel"] : [] })), columns: [{ key: "id", label: "申请编号", kind: "text" }, { key: "amount_money", label: "金额", kind: "money" }], operations: [], row_operations: [cancel] };
  const fetch = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => new Response(JSON.stringify(String(input).endsWith("/credits") ? credits : { id: "withdrawal-requested", status: "cancelled" }), { status: 200 }));
  vi.stubGlobal("fetch", fetch);
  mount(<ResourceCollection resource="withdrawals/workspace" customer page={page} />);
  expect(screen.getAllByRole("button", { name: "取消提现申请" })).toHaveLength(1);
  fireEvent.click(screen.getByRole("button", { name: "取消提现申请" }));
  const dialog = screen.getByRole("dialog");
  fireEvent.click(within(dialog).getByRole("button", { name: "取消提现申请", exact: true }));
  expect(fetch.mock.calls.some(call => call[1]?.method === "POST")).toBe(false);
  fireEvent.click(within(dialog).getByRole("checkbox"));
  fireEvent.click(within(dialog).getByRole("button", { name: "取消提现申请", exact: true }));
  await waitFor(() => expect(fetch.mock.calls.filter(call => call[1]?.method === "POST")).toHaveLength(1));
  const posted = fetch.mock.calls.find(call => call[1]?.method === "POST")!;
  expect(String(posted[0])).toBe("/api/v1/me/withdrawals/withdrawal-requested/cancel");
  expect(JSON.parse(String(posted[1]?.body))).toEqual({});
  expect((posted[1]?.headers as Headers).get("Idempotency-Key")).toMatch(/^[0-9a-f-]{36}$/);
});

const credits = { customer_atg: { available: "0", held: "0", purchased_remaining: "0", gift_remaining: "0" }, money_balance: { available: "0", held: "0" }, commission_money: { available: "0.002184", held: "0", frozen: "0.001861", recovery_due: "0" }, qualification: { confirmed_money: "0", pending_money: "0", threshold_money: "1", qualified_at: null, function_status: "active" } };

it("shows available, frozen and minimum amounts on the one-step withdrawal page", async () => {
  const rule = "单笔提现总额不能低于经营方公布的最低额度 1 结算币种。";
  const page: ResourcePage = { title: "佣金提现", description: rule, summary: [{ label: "最低提现额度", value: "1", unit: "MONEY", detail: "单笔提现总额不能低于经营方公布的这个额度。" }], items: [], columns: [{ key: "amount_money", label: "总额", kind: "money" }], operations: [], payout_methods: [], currency_digits: 2, row_operations: [] };
  vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify(credits), { status: 200 })));
  mount(<ResourceCollection resource="withdrawals/workspace" customer page={page} />);
  const bar = screen.getByText("可用金额").closest(".th-toolbar")!;
  expect(screen.getByRole("heading", { name: "佣金提现" })).toBeTruthy();
  expect(screen.getByRole("heading", { name: "申请提现" })).toBeTruthy();
  expect(screen.getByText(rule)).toBeTruthy();
  expect(screen.queryByRole("button", { name: "核算提现费用" })).toBeNull();
  expect(within(bar).getByText("可用金额")).toBeTruthy();
  expect(within(bar).getByText("冻结金额")).toBeTruthy();
  expect(within(bar).getByText("最低提现额度")).toBeTruthy();
  expect(screen.getAllByText("最低提现额度")).toHaveLength(1);
  await waitFor(() => expect(within(bar).getByLabelText("0.002184 USD")).toBeTruthy());
  expect(within(bar).getByLabelText("0.001861 USD")).toBeTruthy();
  expect(within(bar).getByLabelText("1 USD")).toBeTruthy();
});

it("submits the policy snapshot as nested configuration with its actual hidden version", async () => {
  const fetch = vi.fn(async (_path: RequestInfo | URL, _init?: RequestInit) => new Response(JSON.stringify({ id: "policy-three", status: "published", version: 3 }), { status: 200 }));
  vi.stubGlobal("fetch", fetch);
  mount(<OperationForm operatorID="operator-one" operation={{ name: "policy", label: "发布提现规则", method: "POST", path: "/api/v1/operators/operator-one/withdrawals", fields: [{ name: "configuration.minimum_money", label: "最低提现 结算币种", kind: "decimal", required: true, default_value: "5.000000000000000001" }, { name: "configuration.flat_fee_money", label: "固定手续费 结算币种", kind: "decimal", default_value: "0.100000000000000001" }, { name: "configuration.fee_rate", label: "手续费比例（0到1）", kind: "decimal", default_value: "0.01" }, { name: "configuration.requires_dual_approval", label: "必须双人审批付款", kind: "boolean", default_value: true }, { name: "configuration.method_reference", label: "对客户公示付款方式规则", kind: "text", default_value: "actual published rules" }, { name: "configuration.expected_version", label: "当前规则版本", kind: "hidden", default_value: 2 }] }} />);
  expect((screen.getByLabelText("最低提现 结算币种", { exact: false }) as HTMLInputElement).value).toBe("5.000000000000000001");
  expect(screen.queryByLabelText("当前规则版本")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "发布提现规则", exact: true }));
  await waitFor(() => expect(fetch).toHaveBeenCalledTimes(1));
  expect(JSON.parse(String(fetch.mock.calls[0][1]?.body))).toEqual({ configuration: { minimum_money: "5.000000000000000001", flat_fee_money: "0.100000000000000001", fee_rate: "0.01", requires_dual_approval: true, method_reference: "actual published rules", expected_version: 2 } });
});
