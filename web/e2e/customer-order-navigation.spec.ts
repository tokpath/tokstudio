import { expect, test, type Page } from "@playwright/test";

const brand = { id: "brand", name: "TokenHub", api_base_url: "https://api.example.test/v1", theme: {} };
const operator = { id: "operator", name: "测试经营方", kind: "platform", brand_id: brand.id };
const monetary = { currency: "USD", atg_per_unit: "1", version: 1 };
const method = { id: "manual", name: "线下办理", kind: "offline", status: "ready" };
const plan = { id: "test-plan", name: "合成测试套餐", price_money: "10", purchase_atg: "10", gift_atg: "0", validity: "unlimited", auto_renew_available: false, status: "active", version: 1 };
const unpaid = {
  status: "pending_payment", operator_name: operator.name, amount_money: "10", paid_money: "0",
  created_at: "2020-01-01T00:00:00Z", expires_at: "2020-01-01T01:00:00Z", monetary_snapshot: monetary,
  cancel_allowed: true, payment_action: { kind: "redirect", url: "https://payment.example.test/expired" },
};
const topup = { ...unpaid, id: "expired-topup", kind: "topup" };
const purchase = { ...unpaid, id: "expired-package", kind: "package", plan_name: plan.name };

async function customerAPI(page: Page) {
  const session = { kind: "customer", customer: { id: "customer", email: "customer@example.test", display_name: "测试客户" }, operator, brand, csrf_token: "synthetic" };
  const context = { brand, operator, theme: {}, availability: [], monetary: { ...monetary, status: "locked" }, payment_methods: [method] };
  const pendingTopup = {
    id: topup.id, status: topup.status, version: 1,
    values: { id: topup.id, kind: topup.kind, due_money: topup.amount_money, external_allocated_money: topup.paid_money, monetary_snapshot: monetary, created_at: topup.created_at, expires_at: topup.expires_at },
  };
  const topupPage = {
    title: "结算币种 充值", description: "", items: [pendingTopup], row_operations: [],
    columns: [{ key: "due_money", label: "应付", kind: "money" }],
    operations: [{ name: "topup", label: "创建充值订单", method: "POST", path: "/api/v1/me/topups", fields: [
      { name: "amount_money", label: "充值金额 结算币种", kind: "decimal", required: true },
      { name: "external_method_id", label: "收款方式", kind: "select", options: [{ value: method.id, label: method.name }] },
    ] }],
  };
  const routes: Record<string, unknown> = {
    "/api/v1/public/context": context,
    "/api/v1/auth/me": session,
    "/api/v1/auth/language": { id: "customer", status: "updated" },
    "/api/v1/me/credits": {
      customer_atg: { available: "0", held: "0", purchased_remaining: "0", gift_remaining: "0" },
      money_balance: { available: "0", held: "0" },
      commission_money: { available: "0", held: "0", frozen: "0", recovery_due: "0" },
      qualification: { confirmed_money: "0", pending_money: "0", threshold_money: "100", qualified_at: null, function_status: "normal" },
    },
    "/api/v1/me/notifications/unread-count": { count: 0 },
    "/api/v1/me/topups/workspace": topupPage,
    "/api/v1/me/orders": { items: [purchase] },
    [`/api/v1/me/orders/${topup.id}`]: topup,
    [`/api/v1/me/orders/${purchase.id}`]: purchase,
    "/api/v1/me/plans/catalog": { items: [plan] },
  };
  await page.route("**/api/v1/**", route => route.fulfill({ json: routes[new URL(route.request().url()).pathname] ?? { items: [] } }));
}

function navLink(page: Page, section: "topups" | "orders") {
  return page.locator('nav[aria-label="工作台"]').locator(`a[href="/workspace/${section}"]`);
}

test("a top-up detail stays in USD top-ups through its list, history, reload and a legacy link", async ({ page }) => {
  await customerAPI(page);
  const detailURL = `/workspace/topups?record=${topup.id}`;
  const dialog = page.getByRole("dialog", { name: "订单详情", exact: true });
  await page.goto("/workspace/topups");
  await expect(page.getByRole("heading", { name: "USD 充值", exact: true })).toBeVisible();
  await expect(page.getByRole("row").filter({ hasText: "已过期" })).toHaveCount(1);
  const detailLink = page.getByRole("link", { name: "查看订单详情", exact: true });
  await expect(detailLink).toHaveAttribute("href", detailURL);
  await detailLink.click();
  await expect(page).toHaveURL(detailURL);
  await expect(dialog.getByText(topup.id, { exact: true })).toBeVisible();
  await expect(navLink(page, "topups")).toHaveAttribute("aria-current", "page");
  await expect(navLink(page, "orders")).not.toHaveAttribute("aria-current", "page");
  await expect(dialog.getByText("已过期", { exact: true }).first()).toBeVisible();
  await expect(dialog.getByRole("link", { name: "前往支付通道" })).toHaveCount(0);

  await page.reload();
  await expect(dialog.getByText(topup.id, { exact: true })).toBeVisible();
  await page.goBack();
  await expect(page).toHaveURL("/workspace/topups");
  await expect(dialog).toHaveCount(0);
  await page.goForward();
  await expect(page).toHaveURL(detailURL);
  await expect(dialog).toBeVisible();
  await dialog.getByRole("link", { name: "充值记录", exact: true }).click();
  await expect(page).toHaveURL("/workspace/topups");
  await expect(dialog).toHaveCount(0);
  await expect(navLink(page, "topups")).toHaveAttribute("aria-current", "page");

  await page.goto(`/workspace/orders?record=${topup.id}`);
  await expect(page).toHaveURL(detailURL);
  await expect(dialog.getByText(topup.id, { exact: true })).toBeVisible();
  await expect(navLink(page, "topups")).toHaveAttribute("aria-current", "page");
  await dialog.getByRole("link", { name: "重新充值", exact: true }).click();
  await expect(page.getByRole("dialog", { name: "创建充值订单", exact: true })).toBeVisible();
  await expect(page).toHaveURL("/workspace/topups");
  await expect(dialog).toHaveCount(0);
});

test("an expired package stays in purchase history and can return to package selection", async ({ page }) => {
  await customerAPI(page);
  await page.goto("/workspace/orders");
  await expect(page.getByRole("heading", { name: purchase.plan_name, exact: true })).toBeVisible();
  await expect(page.getByText(topup.id, { exact: true })).toHaveCount(0);
  const purchaseCard = page.locator("section").filter({ has: page.getByRole("heading", { name: purchase.plan_name, exact: true }) });
  await expect(purchaseCard.getByText("已过期", { exact: true })).toBeVisible();
  await expect(purchaseCard.getByRole("button", { name: "继续付款 / 查看详情", exact: true })).toHaveCount(0);
  await purchaseCard.getByRole("button", { name: "查看订单详情", exact: true }).click();
  await expect(page).toHaveURL(`/workspace/orders?record=${purchase.id}`);
  await expect(navLink(page, "orders")).toHaveAttribute("aria-current", "page");
  const dialog = page.getByRole("dialog", { name: "订单详情", exact: true });
  await expect(dialog.getByText(purchase.id, { exact: true })).toBeVisible();
  await expect(dialog.getByText("已过期", { exact: true }).first()).toBeVisible();
  await expect(dialog.getByRole("link", { name: "前往支付通道" })).toHaveCount(0);
  await expect(dialog.getByRole("button", { name: "取消订单", exact: true })).toHaveCount(0);
  await dialog.getByRole("link", { name: "重新选择套餐", exact: true }).click();
  await expect(page).toHaveURL("/workspace/plans#buy-plans");
  await expect(page.getByRole("heading", { name: plan.name, exact: true })).toBeVisible();
});
