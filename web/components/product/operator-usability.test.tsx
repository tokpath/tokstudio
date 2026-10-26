// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import type { Entity, ResourcePage, Session } from "@/lib/contracts";
import { ResourceCollection, ResourceScreen } from "./operator-resources";
import { OperatorOverview, OperatorWorkflowScreen } from "./operator-workflows";
import { readResourceListState, subjectReturnURL, subjectURL } from "./operator-list-state";

let permissions = ["*"];
let operatorKind: "platform" | "oem" | "channel" = "oem";
vi.mock("./context", () => ({ useProduct: () => ({ session: { kind: "staff", staff: { id: "staff-1", email: "staff@example.test", display_name: "负责人" }, workspaces: [{ operator: { id: "operator-one", name: "经营方", kind: operatorKind, brand_id: "brand-one" }, permissions }] } as Session }) }));
afterEach(() => { cleanup(); vi.unstubAllGlobals(); window.history.replaceState(null, "", "/"); localStorage.clear(); permissions = ["*"]; operatorKind = "oem"; });
function page(items: Entity[], extra: Partial<ResourcePage> = {}): ResourcePage { return { search_supported: true, status_supported: true, title: "业务记录", description: "", columns: [{ key: "id", label: "编号", kind: "text" }], items: items.map(item => ({...item,presentation:item.presentation || {title: item.id, labels:{}}})), operations: [], row_operations: [], ...extra }; }
function show(children: React.ReactNode) { return render(<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>{children}</QueryClientProvider>); }
function response(data: unknown) { return new Response(JSON.stringify(data), { status: 200, headers: { "content-type": "application/json" } }); }

it("opens a customer as a linked main page and restores the original search and pagination", async () => {
  window.history.replaceState(null, "", "/manage/operator-one/customers?q=lin&cursor=page-two&previous=");
  const customer = { id: "customer-a", values: { name: "林同学", email: "lin@example.test" } };
  const fetch = vi.fn(async (input: RequestInfo | URL) => response(String(input).includes("customer-context") ? { subject: customer, sections: [] } : page([customer])));
  vi.stubGlobal("fetch", fetch);
  show(<OperatorWorkflowScreen operatorID="operator-one" resource="customers" />);
  const inspect = await screen.findByRole("link", { name: "客户工作台" });
  expect(inspect.getAttribute("href")).toContain("record=customer-a");
  expect(String(fetch.mock.calls[0][0])).toContain("cursor=page-two&q=lin");
  fireEvent.click(inspect);
  await screen.findByRole("heading", { name: "林同学", level: 1 });
  expect(screen.queryByRole("dialog")).toBeNull();
  expect(window.location.search).toContain("record=customer-a");
  fireEvent.click(screen.getByRole("button", { name: "返回原列表与筛选" }));
  expect((screen.getByRole("textbox", { name: "搜索记录" }) as HTMLInputElement).value).toBe("lin");
  expect(window.location.search).toContain("cursor=page-two");
  expect(window.location.search).not.toContain("record=");
  expect(fetch).toHaveBeenCalledTimes(2);
});

it("keeps the full order page and its original uncertain operation mounted for a precise retry", async () => {
  window.history.replaceState(null, "", "/manage/operator-one/orders?record=order-a");
  const order = { id: "order-a", status: "pending_payment", values: { due_usd: "3" } };
  const attempts: { key: string | null; body: unknown }[] = [];
  vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    if (init?.method === "POST") { attempts.push({ key: new Headers(init.headers).get("Idempotency-Key"), body: JSON.parse(String(init.body)) }); if (attempts.length === 1) throw new TypeError("connection reset"); return response({ id: "receipt-a", status: "allocated" }); }
    return response(String(input).includes("order-context") ? { subject: order, sections: [{ resource: "payments", title: "原收款", page: page([], { operations: [{ name: "allocate", label: "分配原收款", method: "POST", path: "/api/v1/operators/operator-one/order-context/order-a/allocate", fields: [{ name: "amount_usd", label: "分配 USD", kind: "decimal", required: true }] }] }) }] } : page([order]));
  }));
  show(<OperatorWorkflowScreen operatorID="operator-one" resource="orders" record="order-a" />);
  fireEvent.click(await screen.findByRole("button", { name: "分配原收款" }));
  fireEvent.change(screen.getByLabelText("分配 USD", { exact: false }), { target: { value: "3.000000000000000001" } });
  fireEvent.click(screen.getByRole("button", { name: "分配原收款", exact: true }));
  await screen.findByRole("button", { name: "重试本次操作" });
  await waitFor(() => expect((screen.getByRole("button", { name: "返回原列表与筛选", hidden: true }) as HTMLButtonElement).disabled).toBe(true));
  expect(screen.getAllByRole("dialog")).toHaveLength(1);
  fireEvent.click(screen.getByRole("button", { name: "重试本次操作" }));
  await screen.findByRole("button", { name: "本次操作已完成，关闭后查看最新记录" });
  expect(attempts).toHaveLength(2);
  expect(attempts[1]).toEqual(attempts[0]);
  expect(attempts[0].body).toEqual({ amount_usd: "3.000000000000000001" });
});

it("shows which procurement order holds supplier inventory and who can release it", () => {
  show(<ResourceCollection operatorID="operator-one" resource="procurement" businessView="supplier" embedded page={page([
    { id: "order-open", status: "awaiting_payment", values: { record_kind: "order", name: "月度采购", reserved_atg: "120", reserved_note: "未付款，持续占用直到付清或取消", payment_hold: "不限时，直到付清或取消" }, allowed_actions: ["cancel"] },
    { id: "order-done", status: "completed", values: { record_kind: "order", name: "已完成采购", reserved_atg: "80", reserved_note: "已释放，不再占用供货库存", payment_hold: "订单已结束，不再等待付款" }, allowed_actions: [] },
  ], {
    description: "固定报价保留成交与退出条款。未付款采购订单不设付款期限，按购买加赠送数量持续占用供货库存，付清交付或取消后释放。下表列出每一笔占用。自动扣款须另有真实支付授权；线下模式只自动建待付订单。",
    summary: [{ label: "未付款占用", value: "120", unit: "ATG", detail: "这些订单没有付款截止时间。付清并交付，或取消尚未分配收款的订单后释放。" }, { label: "待交付占用", value: "0", unit: "ATG" }, { label: "正在占用的订单", value: "1" }],
    columns: [{ key: "reserved_atg", label: "占用供货库存", kind: "atg" }, { key: "reserved_note", label: "占用说明", kind: "text" }, { key: "payment_hold", label: "付款期限", kind: "text" }],
    row_operations: [{ name: "cancel", label: "取消未付款订单", method: "POST", path: "/api/v1/operators/operator-one/procurement/{id}/cancel", fields: [{ name: "evidence", label: "取消原因", kind: "textarea", required: true }], confirmation: "确认取消这笔尚未分配收款的订单？取消后立即释放它占用的供货库存。" }],
  })} />);
  expect(screen.getByText("未付款占用")).toBeTruthy();
  expect(screen.getAllByLabelText("120 ATG")).toHaveLength(2);
  const hold = screen.getByText("未付款，持续占用直到付清或取消");
  expect(hold.parentElement?.className).toContain("text-danger");
  expect(screen.getByText("已释放，不再占用供货库存")).toBeTruthy();
  expect(screen.getByText("付款后自动发放采购额度；未付款订单可取消。")).toBeTruthy();
  expect(screen.getAllByRole("button", { name: "取消未付款订单" })).toHaveLength(1);
  expect(screen.getByText("供货订单列出每一笔采购占用的库存。未付款订单不限时，直到付清或取消后才释放。")).toBeTruthy();
  expect(screen.queryByRole("button", { name: "发布供货报价" })).toBeNull();
});

it("separates procurement quotes, orders, authorizations and attempts", () => {
  show(<ResourceCollection operatorID="operator-one" resource="procurement" businessView="buyer" embedded page={page([
    { id: "quote-a", values: { record_kind: "quote" } }, { id: "purchase-a", values: { record_kind: "order" } }, { id: "authorization-a", values: { record_kind: "authorization" } }, { id: "attempt-a", values: { record_kind: "replenishment_attempt" } },
  ])} />);
  expect(screen.getByText("purchase-a")).toBeTruthy(); expect(screen.queryByText("quote-a")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "立即补货" }));
  expect(screen.getByText("quote-a")).toBeTruthy(); expect(screen.queryByText("purchase-a")).toBeNull();
  fireEvent.click(screen.getByRole("tab", { name: "自动补货" }));
  expect(screen.getByRole("heading", { name: "补货授权" })).toBeTruthy(); expect(screen.getByRole("heading", { name: "补货执行记录" })).toBeTruthy();
  expect(screen.getByText("authorization-a")).toBeTruthy(); expect(screen.getByText("attempt-a")).toBeTruthy();
});

it("organizes refund cases with exact related legs and payments while retaining unknown fees", () => {
  show(<ResourceCollection operatorID="operator-one" resource="refunds" embedded record="case-a" page={page([
    { id: "case-a", status: "paying", values: { record_kind: "case", amount_usd: "3" } },
    { id: "leg-a", status: "unknown", values: { record_kind: "leg", case_id: "case-a", amount_usd: "1", settled_usd: "0.000000000000000001", remaining_usd: "0.999999999999999999" } },
    { id: "leg-b", values: { record_kind: "leg", case_id: "case-a", amount_usd: "2", settled_usd: "0.000000000000000002", remaining_usd: "1.999999999999999998" } },
    { id: "payment-a", values: { record_kind: "actual_refund_payment", leg_id: "leg-a", actual_payment_usd: "0.000000000000000001", fee_usd: null } },
    { id: "unrelated-leg", values: { record_kind: "leg", case_id: "other-case" } },
  ], { columns: [{ key: "amount_usd", label: "退款责任", kind: "usd" }] })} />);
  const refund = screen.getByText("case-a").closest("section")!;
  expect(within(refund).getByLabelText("0.000000000000000003 USD")).toBeTruthy();
  expect(within(refund).getByLabelText("2.999999999999999997 USD")).toBeTruthy();
  expect(within(refund).getByText("payment-a")).toBeTruthy();
  expect(within(refund).queryByText("unrelated-leg")).toBeNull();
  expect(within(refund).queryByLabelText("0 USD")).toBeNull();
  expect(within(refund).getByText("下一步：查询原退款结果并核实实际付款，保留未知金额占用。")).toBeTruthy();
  expect(screen.getByText("本页其他关联记录")).toBeTruthy();
});

it("shows verified opening blockers with only authorized setup destinations", async () => {
  permissions = ["business-status.read", "plans.read"];
  vi.stubGlobal("fetch", vi.fn(async () => response(page([{ id: "operator-one", values: { registration: false, new_purchase: false, model_usage: false, readiness: [{ feature: "new_purchase", requirements: [{ code: "package", description: "缺少可售套餐", ready: false }, { code: "verification_mail", description: "邮件尚未就绪", ready: false }] }] } }]))));
  show(<OperatorOverview operatorID="operator-one" />);
  await screen.findByRole("heading", { name: "开业前，先完成这些准备" });
  expect(screen.getByRole("link", { name: "去完成" }).getAttribute("href")).toBe("/manage/operator-one/plans");
  expect(screen.queryByRole("link", { name: "邮件服务" })).toBeNull();
  expect(screen.getByText("邮件尚未就绪")).toBeTruthy();
});

it("prioritizes finance work from authorized order and refund sources", async () => {
  permissions = ["overview.read", "orders.read", "refunds.read", "payments.manage"];
  const calls: string[] = [];
  vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => {
    const url = String(input); calls.push(url);
    return response(url.endsWith("/orders") ? page([{ id: "pending-order", status: "partial", values: {} }]) : url.endsWith("/refunds") ? page([{ id: "pending-case", status: "paying", values: { record_kind: "case" } }, { id: "refund-leg", status: "unknown", values: { record_kind: "leg", case_id: "pending-case" } }]) : page([]));
  }));
  show(<OperatorOverview operatorID="operator-one" />);
  await screen.findByRole("link", { name: /跟进订单收款与交付/ });
  expect(screen.getByRole("link", { name: /跟进未结退款事项/ }).getAttribute("href")).toBe("/manage/operator-one/refunds");
  expect(screen.getByRole("heading", { name: "今天需要核实的资金事项" })).toBeTruthy();
  expect(calls.some(url => url.endsWith("/withdrawals") || url.endsWith("/requests"))).toBe(false);
});

it("keeps search authoritative across pages without saving filter presets", async () => {
  const calls: string[] = [];
  vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => { calls.push(String(input)); return response(page([{ id: "untrimmed-server-result", values: {} }], { next_cursor: "next" })); }));
  show(<ResourceScreen operatorID="operator-one" resource="payments" />);
  await screen.findByText("untrimmed-server-result");
  fireEvent.change(screen.getByRole("textbox"), { target: { value: "bank-a" } } ); fireEvent.click(screen.getByRole("button", { name: "查询", exact: true }));
  await screen.findByText("untrimmed-server-result"); await waitFor(() => expect(calls.at(-1)).toContain("q=bank-a"));
  expect(screen.queryByRole("button", { name: "保存常用筛选" })).toBeNull();
  expect(localStorage.getItem("resource-filters:staff-1:/api/v1/operators/operator-one:payments")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "下一页" }));
  await waitFor(() => expect(calls.at(-1)).toContain("cursor=next&q=bank-a"));
});

it("offers real domain statuses before any matching record has been loaded", async () => {
  const calls: string[] = [];
  vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => { calls.push(String(input)); return response(page([{ id: "active-customer", status: "active", values: {} }], { next_cursor: "later-page" })); }));
  const view = show(<ResourceScreen operatorID="operator-one" resource="customers" />);
  await screen.findByText("active-customer");
  const select = screen.getByRole("combobox", { name: "筛选状态" });
  expect(Array.from(select.querySelectorAll("option")).map(option => option.value)).toEqual(["", "active", "stopped", "closing", "closed", "anonymized", "suspended"]);
  fireEvent.change(select, { target: { value: "suspended" } });
  await waitFor(() => expect(calls.at(-1)).toContain("status=suspended"));
  view.unmount();
  show(<ResourceScreen operatorID="operator-one" resource="orders" />);
  await screen.findByText("active-customer");
  const orderSelect = screen.getByRole("combobox", { name: "筛选状态" });
  expect(within(orderSelect).getByRole("option", { name: "部分付款" }).getAttribute("value")).toBe("partial");
  expect(within(orderSelect).getByRole("option", { name: "足额待交付" }).getAttribute("value")).toBe("paid");
  fireEvent.change(orderSelect, { target: { value: "partial" } });
  await waitFor(() => expect(calls.at(-1)).toContain("status=partial"));
});

it("restores deep-link filters and accepts only same-operator return destinations", () => {
  const original = "/manage/operator-one/customers?q=lin&cursor=page-two&previous=";
  window.history.replaceState(null, "", subjectURL("operator-one", "customers", "customer-a", original));
  expect(readResourceListState()).toEqual({ q: "lin", status: "", view: "", cursor: "page-two", history: [""] });
  expect(subjectReturnURL("operator-one", "customers")).toBe(original);
  window.history.replaceState(null, "", "/manage/operator-one/customers?record=customer-a&returnTo=https%3A%2F%2Foutside.test");
  expect(subjectReturnURL("operator-one", "customers")).toBe("/manage/operator-one/customers");
});

it("lists a public model's checked capabilities without saved filters or a detail action", async () => {
  operatorKind = "platform";
  vi.stubGlobal("fetch", vi.fn(async () => response(page([{ id: "gpt", status: "configured", presentation: { title: "GPT Sol", labels: {} }, values: { display_name: "GPT Sol", enabled: false, route_configured: false, capabilities: [{ operation: "chat" }, { operation: "responses" }, { operation: "chat" }] } }], { title: "公开模型", description: "说明", columns: [{ key: "display_name", label: "名称", kind: "text" }, { key: "capabilities", label: "能力", kind: "text" }, { key: "enabled", label: "开放模型", kind: "boolean" }], operations: [{ name: "save", label: "添加公开模型", method: "POST", path: "/api/v1/operators/operator-one/catalog", fields: [] }] }))));
  show(<ResourceScreen operatorID="operator-one" resource="catalog" />);
  await screen.findByRole("heading", { name: "公开模型" });
  expect(screen.queryByRole("button", { name: "保存常用筛选" })).toBeNull();
  expect(screen.queryByRole("button", { name: "查看详情" })).toBeNull();
  expect(screen.queryByText("查看明细与证据")).toBeNull();
  expect(screen.getByText("对话、Responses")).toBeTruthy();
  expect(screen.getByRole("button", { name: "编辑模型" })).toBeTruthy();
});

it("opens a public model from the list only after a route exists", async () => {
  const closed = { id: "gpt", version: 3, values: { display_name: "GPT Sol", enabled: false, route_configured: false, capabilities: ["对话"] } };
  const ready = { id: "ready", version: 4, values: { display_name: "Ready Chat", enabled: false, route_configured: true, capabilities: ["对话"] } };
  const posts: unknown[] = [];
  vi.stubGlobal("fetch", vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
    if (init?.method === "POST") { posts.push(JSON.parse(String(init.body))); return response({ id: "ready", status: "saved" }); }
    return response({});
  }));
  show(<ResourceCollection operatorID="operator-one" resource="catalog" page={page([closed, ready], { columns: [{ key: "display_name", label: "名称", kind: "text" }, { key: "enabled", label: "开放模型", kind: "boolean", help: "在列表里打开后，客户才能调用。没有配置路由时不能打开。" }] })} />);
  const closedSwitch = within(screen.getByRole("row", { name: /gpt/ })).getByRole("switch", { name: "开放模型" });
  expect((closedSwitch as HTMLButtonElement).disabled).toBe(true);
  expect(screen.getByText("还没有配置路由，不能开放")).toBeTruthy();
  fireEvent.click(within(screen.getByRole("row", { name: /ready/ })).getByRole("switch", { name: "开放模型" }));
  await waitFor(() => expect(posts).toEqual([{ enabled: true, expected_version: 4 }]));
});
