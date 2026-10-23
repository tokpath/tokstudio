// @vitest-environment jsdom
import { render, fireEvent, screen, waitFor, cleanup, within } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import { OperatorOverview, OperatorSubjectPanel, OperatorWorkflowScreen } from "./operator-workflows";
import { ResourceCollection, ResourceScreen } from "./operator-resources";
import { FinancialReportScreen } from "./financial-report";
import { TakeoversScreen } from "./takeovers";
import type { Entity, ResourcePage, Session } from "@/lib/contracts";

let currentSession: Session;
vi.mock("./context", () => ({ useProduct: () => ({ session: currentSession }) }));
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });
function staff(permissions: string[], kind: "platform" | "oem" | "channel" = "oem") { currentSession = { kind: "staff", staff: { id: "staff-1", email: "staff@example.test", display_name: "负责人" }, workspaces: [{ operator: { id: "operator-one", name: "山海服务", kind, brand_id: "brand-one" }, permissions }] }; }
function page(items: Entity[] = [], extra: Partial<ResourcePage> = {}): ResourcePage { return { search_supported: true, status_supported: true, title: "业务记录", description: "精确归属的原业务记录", items: items.map(item => ({...item,presentation:item.presentation || {title:item.id,labels:{}}})), columns: [{ key: "id", label: "编号", kind: "text" }], operations: [], row_operations: [], ...extra }; }
function show(content: React.ReactNode) { const cache = new QueryClient({ defaultOptions: { queries: { retry: false } } }); return render(<QueryClientProvider client={cache}>{content}</QueryClientProvider>); }
function response(data: unknown) { return new Response(JSON.stringify(data), { status: 200, headers: { "content-type": "application/json" } }); }

it("does not fetch a business resource outside the current explicit scope", () => {
  staff(["customers.read"]);
  const fetch = vi.fn(); vi.stubGlobal("fetch", fetch);
  show(<OperatorWorkflowScreen operatorID="operator-one" resource="payments" />);
  expect(screen.getByText("没有查看权限")).toBeTruthy();
  expect(fetch).not.toHaveBeenCalled();
});

it("preserves row allowed_actions and operation permissions in the new business collection", () => {
  staff(["orders.read"]);
  show(<ResourceCollection operatorID="operator-one" resource="orders" embedded page={page([{ id: "order-one", status: "pending", values: {}, allowed_actions: [] }], { operations: [{ name: "new", label: "越权新增", method: "POST", path: "/api/v1/operators/operator-one/orders", fields: [], permission: "sales.manage" }], row_operations: [{ name: "fulfill", label: "重试交付", method: "POST", path: "/api/v1/operators/operator-one/orders/{id}/fulfill", fields: [] }] })} />);
  expect(screen.queryByRole("button", { name: "越权新增" })).toBeNull();
  expect(screen.queryByRole("button", { name: "重试交付" })).toBeNull();
  expect(screen.getByRole("button", { name: /查看并办理/ })).toBeTruthy();
});

it("opens exact customer context and pages one section while retaining its business tabs", async () => {
  staff(["customers.read", "orders.read", "ledger.read"]);
  const customer: Entity = { id: "customer-a", status: "active", values: { name: "林同学", email: "lin@example.test" } };
  const first = { subject: customer, sections: [{ resource: "orders", title: "订单", page: page([{ id: "order-one", values: {} }], { next_cursor: "next-order" }) }, { resource: "assets", title: "四类资产", page: page([{ id: "wallet-a", values: {} }]) }] };
  const fetch = vi.fn(async (input: RequestInfo | URL) => {
    const url = String(input);
    expect(url).toContain("/customer-context/customer-a");
    return response(url.includes("cursor=") ? { subject: customer, sections: [{ resource: "orders", title: "订单", page: page([{ id: "order-two", values: {} }]) }] } : first);
  });
  vi.stubGlobal("fetch", fetch);
  show(<OperatorSubjectPanel operatorID="operator-one" resource="customers" row={customer} close={() => {}} />);
  await screen.findByText("order-one");
  expect(fetch).toHaveBeenCalledTimes(1);
  fireEvent.click(screen.getByRole("tab", { name: "四类资产" }));
  expect(screen.getByText("wallet-a")).toBeTruthy();
  expect(fetch).toHaveBeenCalledTimes(1);
  fireEvent.click(screen.getByRole("tab", { name: "订单" }));
  fireEvent.click(screen.getByRole("button", { name: "下一页关联记录" }));
  await screen.findByText("order-two");
  expect(screen.getByRole("tab", { name: "四类资产" })).toBeTruthy();
  expect(String(fetch.mock.calls[1]?.[0])).toContain("section=orders&cursor=next-order");
  fireEvent.click(screen.getByRole("button", { name: "上一页关联记录" }));
  expect(screen.getByText("order-one")).toBeTruthy();
});

it("uses the selected order as the locked allocation context and submits the original receipt path", async () => {
  staff(["orders.read", "payments.read", "payments.manage"]);
  const order: Entity = { id: "order-a", status: "partially_paid", values: { kind: "package", due_usd: "10", external_allocated_usd: "2" } };
  const context = { subject: order, sections: [{ resource: "payments", title: "关联收款", page: page([{ id: "receipt-a", status: "confirmed", values: { gross_usd: "8" }, allowed_actions: ["allocate"] }], { row_operations: [{ name: "allocate", label: "分配至原订单", method: "POST", path: "/api/v1/operators/operator-one/payments/{id}/allocate", permission: "payments.manage", fields: [{ name: "order_id", label: "原订单", kind: "text", required: true }, { name: "amount_usd", label: "分配金额 USD", kind: "decimal", required: true }] }] }) }] };
  let submitted: unknown;
  const fetch = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    if (init?.method === "POST") { expect(String(input)).toContain("/payments/receipt-a/allocate"); submitted = JSON.parse(String(init.body)); return response({ id: "receipt-a", status: "allocated" }); }
    expect(String(input)).toContain("/order-context/order-a"); return response(context);
  });
  vi.stubGlobal("fetch", fetch);
  show(<OperatorSubjectPanel operatorID="operator-one" resource="orders" row={order} close={() => {}} />);
  fireEvent.click(await screen.findByRole("button", { name: "分配至原订单" }));
  const allocation = screen.getAllByRole("dialog").at(-1)!;
  fireEvent.change(within(allocation).getByLabelText("分配金额 USD", { exact: false }), { target: { value: "8.000000000000000001" } });
  fireEvent.click(within(allocation).getByRole("button", { name: "分配至原订单", exact: true }));
  await waitFor(() => expect(submitted).toEqual({ order_id: "order-a", amount_usd: "8.000000000000000001" }));
});

it("turns known ledger metrics into Chinese operator tasks without fetching ungranted source pages", async () => {
  staff(["overview.read"]);
  const fetch = vi.fn(async (input: RequestInfo | URL) => {
    expect(String(input)).toBe("/api/v1/operators/operator-one/overview");
    return response(page([{ id: "procurement_available_atg", values: { metric: "procurement_available_atg", value: "8.000000000000000001" } }, { id: "pending_receipt_facts", values: { metric: "pending_receipt_facts", value: "2" } }]));
  });
  vi.stubGlobal("fetch", fetch);
  show(<OperatorOverview operatorID="operator-one" />);
  await screen.findByText("处理实际款入账");
  expect(screen.getByText("可用采购余额")).toBeTruthy();
  expect(screen.getByLabelText("8.000000000000000001 ATG")).toBeTruthy();
  expect(screen.queryByText("pending_receipt_facts")).toBeNull();
  expect(fetch).toHaveBeenCalledTimes(1);
});

it("explains platform upstream responsibility without presenting zero procurement or oversell capacity", async () => {
  staff(["overview.read", "alerts.read", "providers.read", "routes.read"], "platform");
  vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => response(String(input).endsWith("/alerts")
    ? page([{ id: "irrelevant-platform-procurement-risk", status: "critical", values: { metric: "procurement_available_atg", value: "0" } }])
    : page(["sales_space_atg", "procurement_available_atg", "oversell_used_atg"].map(metric => ({ id: metric, values: { metric, value: "0" } })).concat([{ id: "pending_receipt_facts", values: { metric: "pending_receipt_facts", value: "2" } }])))));
  show(<OperatorOverview operatorID="operator-one" />);
  await screen.findByText("处理实际款入账");
  expect(screen.getByText("平台直接维护供应商与上游路由")).toBeTruthy();
  expect(screen.getByText("平台没有上级采购账户。", { exact: false })).toBeTruthy();
  expect(screen.getByRole("link", { name: "配置路由" }).getAttribute("href")).toBe("/manage/operator-one/routes");
  for (const label of ["剩余销售承载空间", "可用采购余额", "已使用超卖空间"]) expect(screen.queryByText(label, { exact: true })).toBeNull();
  expect(screen.queryByLabelText("0 ATG", { exact: true })).toBeNull();
  expect(screen.queryByRole("link", { name: "采购补货", exact: false })).toBeNull();
});

it("shows unverified non-platform capacity as unknown rather than manufacturing zero balances", async () => {
  staff(["overview.read"], "channel");
  vi.stubGlobal("fetch", vi.fn(async () => response(page([
    { id: "sales_space_atg", values: { metric: "sales_space_atg", value: null } },
    { id: "oversell_used_atg", values: { metric: "oversell_used_atg", value: "unavailable" } },
  ]))));
  show(<OperatorOverview operatorID="operator-one" />);
  await screen.findAllByText("尚未核实", { exact: true });
  expect(screen.getAllByText("尚未核实", { exact: true })).toHaveLength(3);
  expect(screen.getByText("部分待办指标尚未核实，请到相应业务页继续查询。")).toBeTruthy();
  expect(screen.queryByLabelText("0 ATG", { exact: true })).toBeNull();
  expect(screen.queryByText("null")).toBeNull();
  expect(screen.queryByText("unavailable")).toBeNull();
  expect(screen.queryByText("平台直接维护供应商与上游路由")).toBeNull();
});

it("forwards an exact record deep link to the authoritative resource query", async () => {
  staff(["refunds.read"]);
  const fetch = vi.fn(async (input: RequestInfo | URL) => { expect(String(input)).toBe("/api/v1/operators/operator-one/refunds?record=case-a"); return response(page([{ id: "case-a", values: {} }, { id: "leg-a", values: { case_id: "case-a" } }])); });
  vi.stubGlobal("fetch", fetch);
  show(<ResourceScreen operatorID="operator-one" resource="refunds" record="case-a" />);
  await screen.findByText("leg-a");
  expect(screen.getByText("正在办理关联事项", { exact: false })).toBeTruthy();
  expect(screen.getByRole("link", { name: "返回全部记录" }).getAttribute("href")).toBe("/manage/operator-one/refunds");
});

it("shows server-authorized retail request context for OEM customer support", async () => {
  staff(["customers.read"]);
  const customer: Entity = { id: "customer-a", values: { name: "林同学" } };
  vi.stubGlobal("fetch", vi.fn(async () => response({ subject: customer, sections: [{ resource: "requests", title: "客户调用记录", page: page([{ id: "retail-request-a", values: { charged_atg: "1.2" } }]) }] })));
  show(<OperatorSubjectPanel operatorID="operator-one" resource="customers" row={customer} close={() => {}} />);
  await screen.findByText("retail-request-a");
  expect(screen.queryByText("没有查看权限")).toBeNull();
});

it("keeps an uncertain financial operation panel open for the original retry", async () => {
  staff(["payments.read"]);
  vi.stubGlobal("fetch", vi.fn(async () => { throw new TypeError("connection reset"); }));
  show(<ResourceCollection embedded operatorID="operator-one" resource="payments" page={page([], { operations: [{ name: "receipt", label: "登记实际收款", method: "POST", path: "/api/v1/operators/operator-one/payments", fields: [{ name: "gross_usd", label: "实收 USD", kind: "decimal", required: true }] }] })} />);
  fireEvent.click(screen.getByRole("button", { name: "登记实际收款" }));
  fireEvent.change(screen.getByLabelText("实收 USD", { exact: false }), { target: { value: "3" } });
  fireEvent.click(screen.getByRole("button", { name: "登记实际收款", exact: true }));
  await screen.findByRole("button", { name: "重试本次操作" });
  await waitFor(() => expect((screen.getByRole("button", { name: "关闭办理面板" }) as HTMLButtonElement).disabled).toBe(true));
  fireEvent.keyDown(document, { key: "Escape" });
  expect(screen.getByRole("dialog")).toBeTruthy();
  expect((screen.getByLabelText("实收 USD", { exact: false }) as HTMLInputElement).disabled).toBe(true);
});

it("offers the original order delivery action inside its context with allowed_actions intact", async () => {
  staff(["orders.read", "sales.manage"]);
  const order: Entity = { id: "order-a", status: "paid_pending_delivery", allowed_actions: ["fulfill"], values: { due_usd: "10", paid_atg: "20" } };
  let submitted: string | undefined;
  vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    if (init?.method === "POST") { submitted = String(input); return response({ id: "order-a", status: "fulfilled" }); }
    return response({ subject: order, sections: [], subject_operations: [{ name: "fulfill", label: "核实资金并重试交付", method: "POST", path: "/api/v1/operators/operator-one/orders/{id}/fulfill", permission: "sales.manage", fields: [] }] });
  }));
  show(<OperatorSubjectPanel operatorID="operator-one" resource="orders" row={order} close={() => {}} />);
  fireEvent.click(await screen.findByRole("button", { name: "核实资金并重试交付" }));
  const dialog = screen.getAllByRole("dialog").at(-1)!;
  fireEvent.click(within(dialog).getByRole("button", { name: "核实资金并重试交付", exact: true }));
  await waitFor(() => expect(submitted).toBe("/api/v1/operators/operator-one/orders/order-a/fulfill"));
});

it("locks every customer and order ancestor until an uncertain allocation is retried with its original identity", async () => {
  staff(["customers.read", "orders.read", "payments.read", "payments.manage"]);
  const customer: Entity = { id: "customer-a", values: { name: "林同学" } };
  const order: Entity = { id: "order-a", status: "pending_payment", values: { due_usd: "3" } };
  const submissions: { body: unknown; key: string | null }[] = [];
  const close = vi.fn();
  vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    if (init?.method === "POST") {
      submissions.push({ body: JSON.parse(String(init.body)), key: new Headers(init.headers).get("Idempotency-Key") });
      if (submissions.length === 1) throw new TypeError("connection reset after send");
      return response({ id: "receipt-a", status: "allocated" });
    }
    return response(String(input).includes("customer-context")
      ? { subject: customer, sections: [{ resource: "orders", title: "客户订单", page: page([order]) }, { resource: "assets", title: "客户资产", page: page() }] }
      : { subject: order, sections: [{ resource: "payments", title: "关联收款", page: page([], { next_cursor: "receipt-next", operations: [{ name: "allocate", label: "分配原收款", method: "POST", path: "/api/v1/operators/operator-one/order-context/order-a/allocate", permission: "payments.manage", fields: [{ name: "amount_usd", label: "分配 USD", kind: "decimal", required: true }] }] }) }, { resource: "assets", title: "交付权益", page: page() }] });
  }));
  show(<OperatorSubjectPanel operatorID="operator-one" resource="customers" row={customer} close={close} />);
  fireEvent.click(await screen.findByRole("button", { name: "查看并办理" }));
  fireEvent.click(await screen.findByRole("button", { name: "分配原收款" }));
  fireEvent.change(screen.getByLabelText("分配 USD", { exact: false }), { target: { value: "3.000000000000000001" } });
  fireEvent.click(screen.getByRole("button", { name: "分配原收款", exact: true }));
  await screen.findByRole("button", { name: "重试本次操作" });
  await waitFor(() => {
    const closes = screen.getAllByRole("button", { name: "关闭办理面板", hidden: true });
    expect(closes).toHaveLength(3);
    expect(closes.every(button => (button as HTMLButtonElement).disabled)).toBe(true);
    expect(screen.getAllByRole("tab", { hidden: true }).every(tab => (tab as HTMLButtonElement).disabled)).toBe(true);
    expect((screen.getByRole("button", { name: "下一页关联记录", hidden: true }) as HTMLButtonElement).disabled).toBe(true);
  });
  fireEvent.keyDown(document, { key: "Escape" });
  expect(screen.getAllByRole("dialog", { hidden: true })).toHaveLength(3);
  expect(close).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "重试本次操作" }));
  await screen.findByRole("button", { name: "本次操作已完成，关闭后查看最新记录" });
  await waitFor(() => expect(screen.getAllByRole("button", { name: "关闭办理面板", hidden: true }).every(button => !(button as HTMLButtonElement).disabled)).toBe(true));
  expect(submissions).toHaveLength(2);
  expect(submissions[0]).toEqual(submissions[1]);
  expect(submissions[0].body).toEqual({ amount_usd: "3.000000000000000001" });
  expect(submissions[0].key).toMatch(/^[0-9a-f-]{36}$/);
});

it("does not offer delivery to finance even when an order carries the operation definition", async () => {
  staff(["orders.read", "payments.manage"]);
  const order: Entity = { id: "order-a", allowed_actions: ["fulfill"], values: {} };
  vi.stubGlobal("fetch", vi.fn(async () => response({ subject: order, sections: [], subject_operations: [{ name: "fulfill", label: "重试销售交付", method: "POST", path: "/api/v1/operators/operator-one/orders/{id}/fulfill", permission: "sales.manage", fields: [] }] })));
  show(<OperatorSubjectPanel operatorID="operator-one" resource="orders" row={order} close={() => {}} />);
  await screen.findByText("原始订单");
  expect(screen.queryByRole("button", { name: "重试销售交付" })).toBeNull();
});

it("revises an existing plan with its saved values and optimistic version", async () => {
  staff(["plans.read", "sales.manage"]);
  let submitted: unknown;
  vi.stubGlobal("fetch", vi.fn(async (_input, init) => { submitted = JSON.parse(String(init?.body)); return response({ id: "plan-a", status: "published" }); }));
  show(<ResourceCollection embedded operatorID="operator-one" resource="plans" page={page([{ id: "plan-a", version: 7, values: { package_id: "plan-a", name: "月度套餐", paid_atg: "21.000000000000000001", gift_atg:"0", price_usd:"20", period:"month", new_purchase_enabled:true,renewal_enabled:true } }], { operations: [{ name: "publish", label: "发布套餐版本", method: "POST", path: "/api/v1/operators/operator-one/plans", permission: "sales.manage", fields: [{ name: "package_id", label: "现有套餐", kind: "text" }, { name: "name", label: "套餐名称", kind: "text", required: true }, { name: "paid_atg", label: "购买 ATG", kind: "decimal", required: true }, { name: "expected_version", label: "当前版本", kind: "hidden", default_value: 0 }] }] })} />);
  fireEvent.click(screen.getByRole("button", { name: "编辑套餐" }));
  const dialog = screen.getByRole("dialog");
  expect((within(dialog).getByLabelText("套餐名称", { exact: false }) as HTMLInputElement).value).toBe("月度套餐");
  expect((within(dialog).getByLabelText("购买 ATG", { exact: false, selector: "input" }) as HTMLInputElement).value).toBe("21.000000000000000001");
  fireEvent.click(within(dialog).getByRole("button", { name: "编辑套餐", exact: true }));
  await waitFor(() => expect(submitted).toEqual({ package_id: "plan-a", name: "月度套餐", paid_atg: "21.000000000000000001", expected_version: 7 }));
});

it("edits the existing theme in place with its saved fields and version", async () => {
  staff(["branding.read", "branding.manage"]);
  let submitted: unknown;
  vi.stubGlobal("fetch", vi.fn(async (input, init) => { expect(String(input)).toBe("/api/v1/operators/operator-one/branding?target_operator_id=channel-one"); submitted = JSON.parse(String(init?.body)); return response({ id: "channel-one", status: "updated" }); }));
  show(<ResourceCollection embedded operatorID="operator-one" resource="branding" page={page([{ id: "channel-one", version: 11, values: { target_operator_id: "channel-one", primary_color: "#123456", support_url: "https://support.example.test" } }], { row_operations: [{ name: "theme", label: "保存逐字段主题", method: "POST", path: "/api/v1/operators/operator-one/branding?target_operator_id={id}", permission: "branding.manage", fields: [{ name: "values.primary_color", label: "主色", kind: "text" }, { name: "values.support_url", label: "支持入口", kind: "text" }, { name: "expected_version", label: "当前版本", kind: "hidden", default_value: 0 }] }] })} />);
  fireEvent.click(screen.getByRole("button", { name: "保存逐字段主题" }));
  expect((screen.getByLabelText("主色", { exact: true }) as HTMLInputElement).value).toBe("#123456");
  fireEvent.change(screen.getByLabelText("主色", { exact: true }), { target: { value: "#654321" } });
  fireEvent.click(screen.getByRole("button", { name: "保存逐字段主题", exact: true }));
  await waitFor(() => expect(submitted).toEqual({ values: { primary_color: "#654321", support_url: "https://support.example.test" }, expected_version: 11 }));
});

it("edits a directly managed channel coverage policy with that row's actual version", async () => {
  staff(["coverage.read", "cooperation.manage"]);
  let submitted: unknown;
  vi.stubGlobal("fetch", vi.fn(async (input, init) => { expect(String(input)).toBe("/api/v1/operators/operator-one/coverage?target_operator_id=channel-one"); submitted = JSON.parse(String(init?.body)); return response({ id: "channel-one", status: "updated", version: 8 }); }));
  show(<ResourceCollection embedded operatorID="operator-one" resource="coverage" layout="cards" page={page([
    { id: "operator-one", version: 0, allowed_actions: [], values: { name: "平台汇总", has_upstream: false } },
    { id: "channel-one", version: 7, allowed_actions: ["oversell"], values: { name: "直属渠道", oversell: "9.000000000000000001", warning_buffer: "2.5" } },
  ], { row_operations: [{ name: "oversell", label: "设置授权超卖与缓冲", method: "POST", path: "/api/v1/operators/operator-one/coverage?target_operator_id={id}", permission: "cooperation.manage", fields: [{ name: "allowance", label: "销售超卖额度 ATG", kind: "decimal", required: true }, { name: "warning", label: "预警缓冲 ATG", kind: "decimal", required: true }, { name: "expected_version", label: "配置版本", kind: "hidden", default_value: 0 }] }] })} />);
  expect(screen.getAllByRole("button", { name: "设置授权超卖与缓冲" })).toHaveLength(1);
  fireEvent.click(screen.getByRole("button", { name: "设置授权超卖与缓冲" }));
  expect((screen.getByLabelText("销售超卖额度 ATG", { exact: false, selector: "input" }) as HTMLInputElement).value).toBe("9.000000000000000001");
  expect((screen.getByLabelText("预警缓冲 ATG", { exact: false, selector: "input" }) as HTMLInputElement).value).toBe("2.5");
  fireEvent.change(screen.getByLabelText("销售超卖额度 ATG", { exact: false, selector: "input" }), { target: { value: "12.000000000000000001" } });
  fireEvent.click(screen.getByRole("button", { name: "设置授权超卖与缓冲", exact: true }));
  await waitFor(() => expect(submitted).toEqual({ allowance: "12.000000000000000001", warning: "2.5", expected_version: 7 }));
});

it("retains unresolved financial facts and validates the selected UTC report interval", async () => {
  staff(["reports.view"]);
  const report = {
    operator_id: "operator-one", from: "2026-10-01T00:00:00Z", to: "2026-10-18T00:00:00Z", captured_at: "2026-10-18T00:00:00Z", status: "pending_verification", operating_result_usd: "-3.000000000000000001",
    revenue: { paid_invocation_usd: "1", expired_unused_paid_usd: "2", wholesale_consumed_usd: "3", withdrawal_fees_retained_usd: "4", total_usd: "10" },
    costs: { fulfillment_usd: "5", retail_gift_subset_usd: "1", commission_usd: "2", payment_fees_usd: "3", exception_loss_usd: "3.000000000000000001", total_usd: "13.000000000000000001", pending_fees: 1, pending_supplier_costs: 2 },
    cash: { recorded_in_usd: "20", recorded_out_usd: "7", procurement_payments_usd: "4", commission_payouts_usd: "3", takeover_in_usd: "0", takeover_out_usd: "0", provider_receivable_usd: "1" },
    liabilities: { customer_wallet_usd: "2", outstanding_grant_paid_usd: "5", outstanding_grant_atg: "8", commission_usd: "3", unfulfilled_order_funds_usd: "4", supplier_payable_usd: "1" },
    payments: { pending_receipt_facts: 3, awaiting_valuation: 2, original_money: [{ currency: "CNY", amount: "99.000000000000000001", facts: 1 }] },
    refunds: { pending_fees: 1, pending_executions: 2, outstanding_usd: "9", mismatch_count: 1 }, takeover_flows: [], takeover_balances: [], notes: ["历史责任按原始经营范围保留。"],
  };
  const fetch = vi.fn(async () => response(report)); vi.stubGlobal("fetch", fetch);
  show(<FinancialReportScreen operatorID="operator-one" />);
  await screen.findByLabelText("99.000000000000000001 CNY");
  expect(screen.getByLabelText("-3.000000000000000001 USD")).toBeTruthy();
  expect(screen.getByText("未核实供应成本 2 笔", { exact: false })).toBeTruthy();
  expect(screen.queryByRole("link", { name: "处理退款未决" })).toBeNull();
  fireEvent.change(screen.getByLabelText("起始（UTC）"), { target: { value: "2026-10-20T00:00" } });
  fireEvent.change(screen.getByLabelText("截至（UTC，含所选分钟）"), { target: { value: "2026-10-19T00:00" } });
  fireEvent.click(screen.getByRole("button", { name: "按实际区间读取" }));
  expect(screen.getByRole("alert").textContent).toContain("截止时间须晚于起始时间");
  expect(fetch).toHaveBeenCalledTimes(1);
});

const takeover = {
  id: "takeover-one", channel_id: "channel-one", brand_operator_id: "operator-one", state: "completed", method: "agreed_atg_value", service_usd_per_atg: "1", procurement_usd_per_atg: "1", extra_usd: "0", contract_reference: "contract-one", evidence: "双方已核实", requested_by: "staff-1", snapshot: null, snapshot_fingerprint: "snapshot-one", service_usd: "3", cash_usd: "4", procurement_offset_atg: "2", procurement_offset_usd: "2", remaining_procurement_refund_usd: "0", settlement_due_usd: "5", settled_usd: "0", created_at: "2026-10-16T00:00:00Z",
};

it("shows takeover facts without offering ungranted financial or preparation actions", async () => {
  staff(["takeovers.read", "takeover-settlements.read"]);
  vi.stubGlobal("fetch", vi.fn(async () => response({ items: [takeover], channels: [] })));
  show(<TakeoversScreen operatorID="operator-one" settlements />);
  await screen.findByRole("heading", { name: "channel-one" });
  expect(screen.queryByRole("button", { name: "登记实际收付款" })).toBeNull();
  expect(screen.queryByRole("button", { name: "发起接管准备" })).toBeNull();
});

it("retries an uncertain takeover settlement with the original proof and action key", async () => {
  staff(["takeover-settlements.read", "cooperation.manage", "refunds.manage"]);
  const submissions: { body: unknown; key: string | null }[] = [];
  vi.stubGlobal("fetch", vi.fn(async (_input, init) => {
    if (init?.method === "POST") {
      submissions.push({ body: JSON.parse(String(init.body)), key: new Headers(init.headers).get("Idempotency-Key") });
      if (submissions.length === 1) throw new TypeError("connection reset");
      return response({ ...takeover, settled_usd: "5" });
    }
    return response({ items: [takeover], channels: [] });
  }));
  show(<TakeoversScreen operatorID="operator-one" settlements />);
  fireEvent.click(await screen.findByRole("button", { name: "登记实际收付款" }));
  for (const [label, value] of [["实际币种金额", "5.000000000000000001"], ["本次实际结清 USD", "5.000000000000000001"], ["唯一银行流水 / 支付凭证", "bank-one"], ["核实依据与当次估值证据", "bank-proof-one"], ["实际收付款时间", "2026-10-18T12:34"]]) fireEvent.change(screen.getByLabelText(label, { exact: true }), { target: { value } });
  fireEvent.click(screen.getByRole("checkbox"));
  fireEvent.click(screen.getByRole("button", { name: "确认提交" }));
  await screen.findByRole("button", { name: "重试原操作" });
  expect((screen.getByRole("button", { name: "关闭办理面板" }) as HTMLButtonElement).disabled).toBe(true);
  fireEvent.keyDown(document, { key: "Escape" });
  expect(screen.getByRole("dialog")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "重试原操作" }));
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  expect(submissions).toHaveLength(2);
  expect(submissions[0]).toEqual(submissions[1]);
  expect(submissions[0].body).toMatchObject({ actual_usd: "5.000000000000000001", amount: "5.000000000000000001", fee_usd: "0", reference: "bank-one" });
  expect(submissions[0].key).toMatch(/^[0-9a-f-]{36}$/);
});
