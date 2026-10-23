import { expect, test, type Page } from "@playwright/test";
import { chooseControl } from "./choose-control";

const brand = { id: "brand-one", name: "山海模型", api_base_url: "https://api.shanhai.example/v1", theme: {} };
const operator = { id: "operator-one", name: "山海经营方", legal_name: "山海服务主体", kind: "oem", brand_id: brand.id };
const customer = { id: "customer-one", status: "active", version: 2, values: { name: "林同学", email: "lin@example.test", registered_at: "2026-10-16T00:00:00Z" } };
const due = "10.000000000000000001", quantity = "20.000000000000000001";
const base = "/api/v1/operators/operator-one";
function resource(title: string, items: unknown[] = [], extra: Record<string, unknown> = {}) { return { title, description: "当前记录精确关联的原业务事实。", items, columns: [{ key: "id", label: "业务编号", kind: "text" }], operations: [], row_operations: [], ...extra }; }
const fulfill = { name: "fulfill", label: "核实资金并重试交付", method: "POST", path: `${base}/orders/{id}/fulfill`, fields: [], permission: "sales.manage", confirmation: "只重试原订单交付，重新核实足额资金与销售承载。" };

async function businessAPI(page: Page, financeOnly = false, uncertainAllocation = false) {
  let allocated = false, delivered = false;
  const mutations: { path: string; body: unknown; key?: string }[] = [];
  const exactRecords: string[] = [];
  const order = () => ({ id: "order-one", status: delivered ? "fulfilled" : allocated ? "paid_pending_delivery" : "pending_payment", version: 1, allowed_actions: delivered ? [] : ["fulfill"], values: { kind: "package", customer_id: customer.id, due_usd: due, external_allocated_usd: allocated ? due : "0", internal_usd: "0", commission_usd: "0", paid_atg: quantity, created_at: "2026-10-16T01:00:00Z", completed_at: delivered ? "2026-10-17T01:00:00Z" : null } });
  await page.route("**/api/v1/**", async route => {
    const req = route.request(), url = new URL(req.url()), path = url.pathname;
    let data: unknown = resource("业务记录");
    if (path === "/api/v1/public/context") data = { brand, operator, theme: {}, availability: [], payment_methods: [] };
    else if (path === "/api/v1/auth/me") data = { kind: "staff", staff: { id: "staff-one", email: "owner@example.test", display_name: "经营负责人" }, workspaces: [{ operator, permissions: financeOnly ? ["orders.read", "payments.read", "payments.manage"] : ["*"] }], active_operator_id: operator.id, csrf_token: "csrf_operator_test" };
    else if (req.method() === "POST") {
      const body = req.postDataJSON();
      mutations.push({ path, body, key: req.headers()["idempotency-key"] });
      if (path === `${base}/order-context/order-one/allocate`) { expect(body).toEqual({ receipt_id: "receipt-one", amount_usd: due }); allocated = true; if (uncertainAllocation && mutations.filter(item => item.path === path).length === 1) { await route.abort("failed"); return; } data = { id: "receipt-one", status: "allocated", message: "原收款已分配，请查看当前订单交付状态。" }; }
      else if (path === `${base}/orders/order-one/fulfill`) { expect(allocated).toBe(true); expect(body).toEqual({}); delivered = true; data = { id: "order-one", status: "fulfilled", message: "原订单已完成权益交付。" }; }
      else throw new Error(`Unexpected mutation ${path}`);
    }
    else if (path === `${base}/customers`) data = resource("客户服务", [customer], { columns: [{ key: "name", label: "客户", kind: "text" }, { key: "email", label: "邮箱", kind: "text" }] });
    else if (path === `${base}/orders`) data = resource("订单与交付", [order()], { columns: [{ key: "id", label: "订单", kind: "text" }, { key: "due_usd", label: "应付", kind: "usd" }], row_operations: [fulfill] });
    else if (path === `${base}/customer-context/customer-one`) data = { subject: customer, next_actions: [], sections: [
      { resource: "orders", title: "购买与交付", page: resource("购买与交付", [order()], { row_operations: [fulfill] }) },
      { resource: "assets", title: "客户资产", page: resource("客户资产", [{ id: "wallet-one", status: "open", values: { balance: "3.000000000000000001", held: "0", purpose: "customer:usd" } }], { columns: [{ key: "purpose", label: "资产用途", kind: "text" }, { key: "balance", label: "余额", kind: "usd" }] }) },
      { resource: "requests", title: "模型调用", page: resource("模型调用", [{ id: "retail-request-one", presentation: {title:"公开聊天模型 · 10月16日调用",labels:{}}, status: "succeeded", values: { model_id: "public-chat", charged_atg: "0.001" } }]) },
      { resource: "commissions", title: "推广收益", page: resource("推广收益", [], { summary: [{ label: "分佣资格", value: "已正式取得" }] }) },
      { resource: "refunds", title: "退款与异常", page: resource("退款与异常", [{ id: "case-one", status: "pending_review", values: { manage_url: "/manage/operator-one/refunds?record=case-one" } }], { columns: [{ key: "id", label: "原申请", kind: "text" }, { key: "manage_url", label: "办理", kind: "link" }] }) },
    ] };
    else if (path === `${base}/order-context/order-one`) data = { subject: order(), subject_operations: [fulfill], next_actions: [delivered ? "权益已交付，查看原订单额度来源。" : allocated ? "客户已足额付款，核实可售空间并完成原订单交付。" : "核实收款并分配到此订单，足额后交付。"], sections: [
      { resource: "payments", title: "关联收款", page: resource("关联收款", allocated ? [{ id: "receipt-one", status: "confirmed", values: { provider: "manual", gross_usd: due, order_allocated_usd: due, verification_evidence: "bank-evidence-one" } }] : [], { columns: [{ key: "provider", label: "通道", kind: "text" }, { key: "order_allocated_usd", label: "分配至此单", kind: "usd" }, { key: "verification_evidence", label: "核实证据", kind: "text" }], operations: [{ name: "allocate_existing", label: "将已核实收款分配到此单", method: "POST", path: `${base}/order-context/order-one/allocate`, permission: "payments.manage", confirmation: "只分配已经核实的原收款，不新建收付款事实。", fields: [{ name: "receipt_id", label: "选择已核实收款", kind: "text", lookup_url: `${base}/references/receipts`, required: true }, { name: "amount_usd", label: "本次分配 USD", kind: "decimal", required: true }] }] }) },
      { resource: "receipt-valuations", title: "原币待核实", page: resource("原币待核实") },
      { resource: "assets", title: "交付权益", page: resource("交付权益", delivered ? [{ id: "grant-one", status: "issued", values: { total: quantity, consumed: "0", effective_at: "2026-10-17T01:00:00Z" } }] : [], { columns: [{ key: "total", label: "已发放 ATG", kind: "atg" }, { key: "effective_at", label: "生效时间", kind: "datetime" }] }) },
    ] };
    else if (path === `${base}/references/receipts`) data = { items: [{ value: "receipt-one", label: "已核实银行收款 · 原流水 bank-one", defaults: { receipt_id: "receipt-one", amount_usd: due, currency: "USD" } }] };
    else if (path === `${base}/refunds`) { exactRecords.push(url.searchParams.get("record") || ""); data = resource("退款与原路执行", [{ id: "case-one", status: "pending_review", values: { record_kind: "case" } }, { id: "leg-one", presentation: {title:"原银行付款退款 · 10 USD",labels:{}}, status: "pending", values: { record_kind: "leg", case_id: "case-one" } }]); }
    else throw new Error(`Unexpected read ${path}`);
    await route.fulfill({ json: data });
  });
  return { mutations, exactRecords };
}

test("operator handles customer, original receipt, delivery and precise refund context in one flow", async ({ page }) => {
  const { mutations, exactRecords } = await businessAPI(page);
  await page.goto("/manage/operator-one/customers");
  await page.getByRole("link", { name: "客户工作台", exact: true }).click();
  await expect(page).toHaveURL(/\/customers\?record=customer-one/);
  const workspace = page.locator("#workspace-content");
  await expect(workspace.getByRole("heading", { name: "林同学", exact: true })).toBeVisible();
  await workspace.getByRole("tab", { name: "客户资产" }).click();
  await expect(workspace.getByLabel("3.000000000000000001 USD", { exact: true })).toBeVisible();
  await workspace.getByRole("tab", { name: "模型调用" }).click();
  await expect(workspace.getByText("公开聊天模型 · 10月16日调用", { exact: true })).toBeVisible();
  await workspace.getByRole("tab", { name: "推广收益" }).click();
  await expect(workspace.getByText("已正式取得", { exact: true })).toBeVisible();
  await workspace.getByRole("tab", { name: "购买与交付" }).click();
  await workspace.getByRole("link", { name: "查看并办理", exact: true }).click();
  await expect(page).toHaveURL(/\/orders\?record=order-one/);
  expect(new URL(page.url()).searchParams.get("returnTo")).toContain("/customers?record=customer-one");
  await expect(workspace.getByRole("heading", { name: "订单与收款办理", exact: true })).toBeVisible();
  await expect(workspace.getByLabel(`${due} USD`, { exact: true }).filter({ visible: true })).toBeVisible();
  await workspace.getByRole("button", { name: "将已核实收款分配到此单", exact: true }).click();
  const allocation = page.getByRole("dialog");
  await chooseControl(allocation.getByRole("combobox", { name: "选择已核实收款", exact: true }), "receipt-one");
  await expect(allocation.getByRole("textbox", { name: /^本次分配 USD/ })).toHaveValue(due);
  await allocation.getByRole("checkbox").check();
  await allocation.getByRole("button", { name: "将已核实收款分配到此单", exact: true }).click();
  await expect(allocation.getByText("原收款已分配，请查看当前订单交付状态。", { exact: false })).toBeVisible();
  await allocation.getByRole("button", { name: "关闭办理面板", exact: true }).click();
  await expect(workspace.getByText("bank-evidence-one", { exact: true })).toBeVisible();
  await workspace.getByRole("button", { name: "核实资金并重试交付", exact: true }).click();
  const delivery = page.getByRole("dialog");
  await delivery.getByRole("checkbox").check();
  await delivery.getByRole("button", { name: "核实资金并重试交付", exact: true }).click();
  await expect(delivery.getByText("原订单已完成权益交付。", { exact: false })).toBeVisible();
  await delivery.getByRole("button", { name: "关闭办理面板", exact: true }).click();
  await workspace.getByRole("tab", { name: "交付权益" }).click();
  await expect(workspace.getByRole("tabpanel").getByLabel(`${quantity} ATG`, { exact: true })).toBeVisible();
  await workspace.getByRole("button", { name: "返回原列表与筛选", exact: true }).click();
  await expect(page).toHaveURL(/\/customers\?record=customer-one/);
  await workspace.getByRole("tab", { name: "退款与异常" }).click();
  await workspace.getByRole("link", { name: "进入业务办理", exact: true }).click();
  await expect(page).toHaveURL(/\/refunds\?record=case-one$/);
  await expect(page.getByText("正在办理关联事项", { exact: false })).toBeVisible();
  await expect(page.getByText("原银行付款退款 · 10 USD", { exact: true })).toBeVisible();
  expect(exactRecords).toContain("case-one");
  expect(mutations.map(item => item.path)).toEqual([`${base}/order-context/order-one/allocate`, `${base}/orders/order-one/fulfill`]);
  expect(mutations.every(item => /^[0-9a-f-]{36}$/.test(item.key || ""))).toBe(true);
});

test("finance can allocate on an order without being offered sales delivery authority", async ({ page }) => {
  await businessAPI(page, true);
  await page.goto("/manage/operator-one/orders");
  await page.getByRole("link", { name: "查看并办理", exact: true }).click();
  await expect(page).toHaveURL(/\/orders\?record=order-one/);
  await expect(page.getByRole("heading", { name: "订单与收款办理", exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "将已核实收款分配到此单", exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "核实资金并重试交付", exact: true })).toHaveCount(0);
  await expect(page.getByRole("link", { name: "公开模型", exact: true })).toHaveCount(0);
});

test("uncertain receipt allocation keeps its original order and customer return context through retry", async ({ page }) => {
  const { mutations } = await businessAPI(page, false, true);
  await page.goto("/manage/operator-one/customers");
  await page.getByRole("link", { name: "客户工作台", exact: true }).click();
  await expect(page).toHaveURL(/\/customers\?record=customer-one/);
  await expect(page.getByRole("heading", { name: "林同学", exact: true })).toBeVisible();
  await page.getByRole("link", { name: "查看并办理", exact: true }).click();
  await expect(page).toHaveURL(/\/orders\?record=order-one/);
  const orderURL = page.url();
  expect(new URL(orderURL).searchParams.get("returnTo")).toContain("/customers?record=customer-one");
  await page.getByRole("button", { name: "将已核实收款分配到此单", exact: true }).click();
  const allocation = page.getByRole("dialog");
  await chooseControl(allocation.getByRole("combobox", { name: "选择已核实收款", exact: true }), "receipt-one");
  await allocation.getByRole("checkbox").check();
  await allocation.getByRole("button", { name: "将已核实收款分配到此单", exact: true }).click();
  await expect(allocation.getByRole("button", { name: "重试本次操作", exact: true })).toBeVisible();
  await expect(allocation.getByRole("button", { name: "关闭办理面板", exact: true })).toBeDisabled();
  const back = page.getByRole("button", { name: "返回原列表与筛选", exact: true, includeHidden: true });
  await expect(back).toBeDisabled();
  for (const tab of await page.getByRole("tab", { includeHidden: true }).all()) await expect(tab).toBeDisabled();
  await page.keyboard.press("Escape"); await page.mouse.click(8, 8);
  await expect(page.getByRole("dialog", { includeHidden: true })).toHaveCount(1);
  await expect(page.getByRole("heading", { name: "订单与收款办理", exact: true, includeHidden: true })).toHaveCount(1);
  await expect(page).toHaveURL(orderURL);
  await expect(allocation.getByRole("textbox", { name: /^本次分配 USD/ })).toBeDisabled();
  await allocation.getByRole("button", { name: "重试本次操作", exact: true }).click();
  await expect(allocation.getByText("原收款已分配，请查看当前订单交付状态。", { exact: false })).toBeVisible();
  await expect(allocation.getByRole("button", { name: "关闭办理面板", exact: true })).toBeEnabled();
  await allocation.getByRole("button", { name: "关闭办理面板", exact: true }).click();
  await expect(back).toBeEnabled();
  await expect(page.getByText("bank-evidence-one", { exact: true })).toBeVisible();
  expect(mutations).toHaveLength(2); expect(mutations[0]).toEqual(mutations[1]);
  expect(mutations[0].body).toEqual({ receipt_id: "receipt-one", amount_usd: due });
  await back.click();
  await expect(page).toHaveURL(/\/customers\?record=customer-one/);
  await expect(page.getByRole("heading", { name: "林同学", exact: true })).toBeVisible();
});
