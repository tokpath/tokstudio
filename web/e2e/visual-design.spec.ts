import { test, expect, type Page } from "@playwright/test";
import { translate } from "../lib/i18n";

const brand = { id: "brand-visual", name: "TokenHub", api_base_url: "https://models.example.test/v1", theme: { primary_color: "#2f62e8", background_color: "#ffffff", text_color: "#172334" } };
const operator = { id: "operator-visual", name: "TokenHub 平台", kind: "platform", brand_id: brand.id, legal_name: "视觉验收合成主体" };
const plan = { id: "plan-visual", name: "应用开发套餐", price_usd: "80", purchase_atg: "80", gift_atg: "20", validity: "month", auto_renew_available: true, status: "active", version: 1, description: "通用额度按调用时报价使用，按成交期限有效。套餐不支持一般性退款，可停止未来续费。" };
const models = [{ id: "chat-pro", name: "Chat Pro", capabilities: ["chat"], status: "published", prices: [{ component: "输入", amount_atg: "0.000002", unit: "token" }] }, { id: "reasoning", name: "Reasoning", capabilities: ["chat"], status: "published", prices: [] }, { id: "vision", name: "Vision", capabilities: ["image"], status: "published", prices: [] }];
const customer = { kind: "customer", customer: { id: "customer-visual", email: "lin@example.test", display_name: "林同学" }, brand, operator, csrf_token: "visual-csrf" };
const staff = { kind: "staff", staff: { id: "staff-visual", email: "admin@example.test", display_name: "平台负责人" }, workspaces: [{ operator, permissions: ["*"] }], active_operator_id: operator.id, csrf_token: "visual-csrf" };
const order = { id: "order-visual", kind: "package", status: "awaiting_payment", operator_name: operator.name, plan_name: plan.name, amount_usd: "80", paid_usd: "0", purchase_atg: "80", gift_atg: "20", created_at: "2026-10-20T08:00:00Z", payment_action: { kind: "form", url: "https://payments.example.test/gkash", fields: { transaction: "synthetic-order" }, amount: "352.80", currency: "MYR" } };
const pageResource = { title: "客户服务", description: "按姓名、邮箱查找本经营方客户，查看订单、资产及服务记录。", search_supported: true, status_supported: true, items: ["林同学", "合成客户", "开发团队"].map((name, i) => ({ id: `customer-${i}`, status: "active", presentation: { title: name, labels: {} }, values: { email: `customer${i}@example.test`, registered_at: "2026-10-20T08:00:00Z" } })), columns: [{ key: "email", label: "邮箱", kind: "text" }, { key: "registered_at", label: "注册", kind: "datetime" }], operations: [{ name: "gift", label: "人工赠送与补偿", method: "POST", path: `/api/v1/operators/${operator.id}/customers`, fields: [{ name: "name", label: "名称", kind: "text", required: true }, { name: "amount_atg", label: "赠送 ATG", kind: "decimal", required: true }], confirmation: "确认赠送额度与适用客户。" }], row_operations: [] };
async function api(page: Page, session: unknown) {
  await page.route("**/api/v1/**", route => {
    const path = new URL(route.request().url()).pathname;
    let data: unknown = { items: [] };
    if (path === "/api/v1/public/context") data = { brand, operator, theme: {}, google_sign_in: true, availability: [], payment_methods: [{ id: "kvoucher", name: "kvoucher", kind: "online", status: "ready" }, { id: "bank", name: "manual", kind: "offline", status: "ready" }] };
    else if (path === "/api/v1/auth/me") data = session;
    else if (["/api/v1/public/models", "/api/v1/me/models"].includes(path)) data = { items: models };
    else if (["/api/v1/public/plans", "/api/v1/me/plans/catalog"].includes(path)) data = { items: [plan, { ...plan, id: "plan-two", name: "长期备用额度", price_usd: "20", purchase_atg: "20", gift_atg: "2", validity: "unlimited", auto_renew_available: false }] };
    else if (path === "/api/v1/me/credits") data = { customer_atg: { available: "1248.7506", held: "0.03", purchased_remaining: "1000", gift_remaining: "248.7506" }, usd_balance: { available: "80", held: "0" }, commission_usd: { available: "12.5", held: "0", frozen: "3", recovery_due: "0" }, qualification: { confirmed_usd: "200", pending_usd: "5", threshold_usd: "100", qualified_at: "2026-10-18T00:00:00Z", function_status: "normal" } };
    else if (path === "/api/v1/me/requests") data = { items: [{ id: "request-visual", model_id: "chat-pro", key_name: "我的开发应用", status: "completed", charged_atg: "0.0235", held_atg: "0", created_at: "2026-10-20T08:00:00Z" }] };
    else if (path === "/api/v1/me/orders") data = { items: [order] };
    else if (path === "/api/v1/me/orders/order-visual") data = order;
    else if (path.startsWith(`/api/v1/operators/${operator.id}/`)) data = pageResource;
    return route.fulfill({ json: data });
  });
}
async function noOverflow(page: Page) { await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth)).toBeLessThanOrEqual(1); }

test("public brand, plans and Google login retain clear hierarchy in four languages", async ({ page }, info) => {
  await api(page, { kind: "anonymous" });
  await page.goto("/"); await expect(page.locator(".th-hero-console code")).toHaveText(brand.api_base_url);
  await page.screenshot({ path: info.outputPath("public-home.png"), fullPage: true });
  await page.goto("/plans"); await expect(page.getByRole("heading", { name: plan.name })).toBeVisible();
  await page.getByText("服务条款", { exact: true }).first().click(); await expect(page.getByText(plan.description).first()).toBeVisible();
  await page.screenshot({ path: info.outputPath("public-plans.png"), fullPage: true });
  await page.goto("/login"); await expect(page.getByRole("button", { name: "使用 Google 账号继续" })).toBeVisible();
  await page.screenshot({ path: info.outputPath("login.png"), fullPage: true });
  await page.setViewportSize({ width: 390, height: 844 });
  for (const locale of ["zh", "en", "ja", "ru"] as const) {
    await page.getByTestId("language-selector").selectOption(locale); await noOverflow(page);
    await expect(page.getByRole("button", { name: translate(locale, "使用 Google 账号继续"), exact: true })).toBeVisible();
  }
  await page.screenshot({ path: info.outputPath("russian-login-mobile.png"), fullPage: true });
});

test("customer surfaces and status colors stay distinct in light and dark modes", async ({ page }, info) => {
  await api(page, customer); await page.goto("/workspace");
  await expect(page.getByLabel("1248.7506 ATG", { exact: true })).toBeVisible();
  const workspace = page.locator(".th-workspace"), card = page.locator(".th-balance-card");
  expect(await workspace.evaluate(el => getComputedStyle(el).backgroundColor)).not.toBe(await card.evaluate(el => getComputedStyle(el).backgroundColor));
  const status = page.getByText("已完成", { exact: true });
  expect(await status.evaluate(el => getComputedStyle(el).backgroundColor)).not.toBe("rgba(0, 0, 0, 0)");
  await page.screenshot({ path: info.outputPath("customer-overview.png"), fullPage: true });
  await page.getByLabel("显示主题").selectOption("dark"); await expect(page.locator("html")).toHaveClass(/dark/);
  const settledBorder = await page.locator(".th-sidebar").evaluate(el => getComputedStyle(el).borderRightColor);
  await expect.poll(() => page.locator("a.group").first().evaluate(el => getComputedStyle(el).borderColor)).toBe(settledBorder);
  await noOverflow(page); await page.screenshot({ path: info.outputPath("customer-dark.png"), fullPage: true });
  await page.setViewportSize({ width: 390, height: 844 });
  await page.getByTestId("language-selector").selectOption("ru"); await noOverflow(page);
  await page.screenshot({ path: info.outputPath("customer-russian-mobile.png"), fullPage: true });
});

test("operator list, form and help keep keyboard access and mobile containment", async ({ page }, info) => {
  await api(page, staff); await page.goto(`/manage/${operator.id}/customers`);
  await expect(page.getByRole("heading", { name: "客户服务", exact: true })).toBeVisible();
  await page.screenshot({ path: info.outputPath("operator-customers.png"), fullPage: true });
  await page.getByRole("button", { name: "人工赠送与补偿", exact: true }).click();
  const dialog = page.getByRole("dialog").last(); await expect(dialog.getByLabel("名称", { exact: false }).and(dialog.locator("input"))).toBeVisible();
  await page.screenshot({ path: info.outputPath("operator-form.png"), fullPage: true });
  await page.setViewportSize({ width: 390, height: 844 }); await noOverflow(page);
  await page.screenshot({ path: info.outputPath("operator-form-mobile.png"), fullPage: true });
  await page.keyboard.press("Escape"); await expect(dialog).toHaveCount(0);
});

test("KVoucher clearly separates USD liability from MYR payment without opening a new order", async ({ page }, info) => {
  await api(page, customer); await page.goto("/workspace/orders?record=order-visual");
  await expect(page.getByLabel("80 USD", { exact: true }).first()).toBeVisible();
  await expect(page.getByLabel("352.80 MYR", { exact: true })).toBeVisible();
  const form = page.locator('form[target="tokpath-gkash"]'); await expect(form).toHaveAttribute("action", order.payment_action.url);
  await expect(page.getByRole("button", { name: "我已付款，核实原通道" })).toBeVisible();
  await page.screenshot({ path: info.outputPath("kvoucher-order.png"), fullPage: true });
  await page.setViewportSize({ width: 390, height: 844 }); await noOverflow(page);
  await page.screenshot({ path: info.outputPath("kvoucher-mobile.png"), fullPage: true });
});
