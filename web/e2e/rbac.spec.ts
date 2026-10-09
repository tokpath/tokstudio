import { expect, test, type Page } from "@playwright/test";
import { mockViewer } from "./mock-viewer";

const customer = { id: "usr_customer", email: "customer@example.test", display_name: "Customer", status: "active", channel_org_id: "chn_official_a", channel_code: "official-a", brand_id: "brd_a", brand_name: "TokenHub", created_at: "2026-09-18T00:00:00Z" };
const model = { public_id: "test/model", display_name: "Test Model", vendor: "Test", kind: "text", status: "published", enabled: true, parent_enabled: true, wholesale: { input: "0.000001", output: "0.000002" } };
test.beforeEach(async ({ page }) => {
  // Unrelated dashboards may fail without hiding the actual authorized navigation.
  await page.route("**/api/**", route => route.fulfill({ status: 503, json: { error: { message: "本场景未加载此业务数据" } } }));
  await page.route("**/api/admin/billing/report", route => route.fulfill({ json: { report: {} } }));
});
async function openGroup(page: Page, name: string) {
  const nav = page.getByRole("navigation", { name: "平台管理", exact: true });
  const group = nav.getByRole("button", { name, exact: true });
  if (await group.getAttribute("aria-expanded") !== "true") await group.click();
  return nav;
}
async function customerRead(page: Page, role: "finance_admin" | "ops_admin" | "audit_readonly") {
  await page.route("**/api/admin/customers?**", route => route.fulfill({ json: { items: [customer], total: 1, limit: 25, next_cursor: "" } }));
  await page.route("**/api/admin/customer-scopes", route => route.fulfill({ json: { items: [{ id: customer.channel_org_id, code: customer.channel_code, brand_id: customer.brand_id, brand_name: customer.brand_name }] } }));
  await page.route(`**/api/admin/customers/${customer.id}`, route => route.fulfill({ json: {
    item: customer, permissions: { operations: role !== "finance_admin", finance: role !== "ops_admin", audit: role === "audit_readonly", manage: false, attribution: false, professional_create: false, record_payment: role === "finance_admin", read_payments: true },
    errors: {}, usage: { confirmed_count: 0, pending_count: 0, confirmed_minor: 0, refunded_minor: 0 }, activity: { count: 0, items: [] },
  } }));
  const nav = await openGroup(page, "客户与合作方");
  await nav.getByRole("link", { name: "客户", exact: true }).click();
  await expect(page).toHaveURL(/\/admin\/users$/);
  const row = page.getByRole("row").filter({ hasText: customer.email });
  await expect(row).toContainText("TokenHub");
  await row.getByRole("link", { name: "Customer", exact: true }).click();
  await expect(page.getByRole("heading", { name: "Customer", exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "封禁客户", exact: true })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "更改邀请归因", exact: true })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "开通专业推广关系", exact: true })).toHaveCount(0);
}
async function channel(page: Page, type: "B" | "C") {
  const id = type === "C" ? "chn_oem_c" : "chn_reseller_b";
  await page.route(`**/api/admin/channels/${id}`, route => route.fulfill({ json: { item: { id, code: type === "C" ? "oem-c" : "reseller-b", type, status: "active", brand_id: "brd_a", parent_id: "chn_official_a" } } }));
  await page.route(`**/api/admin/channels/${id}/models`, route => route.fulfill({ json: { channel_type: type, items: [model] } }));
  if (type === "B") await page.route(`**/api/admin/channels/${id}/onboarding`, route => route.fulfill({ json: {
    item: { channel: { id, code: "reseller-b", status: "active" }, brand_name: "TokenHub", registration_url: "", admin_count: 1, can_manage_models: false, can_manage_admins: false }, models_available: true, model_count: 1,
  } }));
  if (type === "C") {
    await page.route(`**/api/admin/oem-deliveries/${id}`, route => route.fulfill({ json: { item: {
      delivery: { channel_org_id: id, phase: "configuring", version: 1, sales_mode: "offline", sell_plans: false, handoff_evidence: {} },
      channel: { id, code: "oem-c" }, brand: { id: "brd_c", name: "OEM C", primary_domain: "oem.example.test", api_domain: "api.oem.example.test", admin_domain: "admin.oem.example.test" },
      ready: false, checked_at: "2026-10-10T00:00:00Z", administrators: [],
      checks: [{ key: "quota", ready: true, reason: "quota_empty", responsibility: "platform_finance", href: `/admin/oem-deliveries/${id}#quota` }, { key: "request", ready: false, reason: "request_evidence_missing", responsibility: "customer", href: "" }],
    } } }));
    await page.route(`**/api/admin/channel-quotas/${id}`, route => route.fulfill({ json: { quota: { available_minor: 5000000, issued_minor: 0, consumed_minor: 0 } } }));
    await page.route(`**/api/admin/channel-quotas/${id}/issue-rule`, route => route.fulfill({ json: { rule: { issue_ratio_bps: 10000 } } }));
  }
  await page.goto(`/admin/channels/${id}`);
}

test("unsigned admin requires login without rendering protected menus", async ({ page }) => {
  await page.route("**/v1/me", route => route.fulfill({ status: 401, json: {} }));
  await page.goto("/admin");
  await expect(page.getByRole("link", { name: "重新登录", exact: true })).toBeVisible();
  await expect(page.locator('nav a[href="/admin/providers"]')).toHaveCount(0);
});

test("finance reads minimal customers and finance tasks without upstream or organization write access", async ({ page }) => {
  await mockViewer(page, { roles: ["finance_admin"] });
  await page.goto("/admin");
  const nav = await openGroup(page, "资金与结算");
  await expect(nav.locator('a[href="/admin/channels"]')).not.toBeVisible();
  await expect(nav.getByRole("link", { name: "余额/充值", exact: true })).toBeVisible();
  await expect(nav.getByRole("link", { name: "佣金与结算", exact: true })).toBeVisible();
  await expect(nav.locator('a[href="/admin/providers"]')).toHaveCount(0);
  await expect(nav.locator('a[href="/admin/audit"]')).toHaveCount(0);
  await customerRead(page, "finance_admin");
  await page.goto("/admin/billing");
  await expect(page.getByRole("button", { name: "查询退款账单", exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "赠送额度", exact: true })).toBeVisible();
  await page.goto("/admin/channels");
  await expect(page.getByRole("button", { name: "新建渠道", exact: true })).toHaveCount(0);
  await expect(page.getByRole("tab", { name: "代理商", exact: true })).toHaveCount(0);
  await channel(page, "B");
  await expect(page.getByText("reseller-b · 渠道", { exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "编辑", exact: true })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "编辑授权", exact: true })).toHaveCount(0);
  await expect(page.getByRole("heading", { name: "OEM 服务额度", exact: true })).toHaveCount(0);
  await channel(page, "C");
  await expect(page).toHaveURL(/\/admin\/oem-deliveries\/chn_oem_c$/);
  await expect(page.getByRole("heading", { name: "OEM C · 交付与营业", exact: true })).toBeVisible();
  await expect(page.getByRole("heading", { name: "OEM 服务额度", exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "调整额度", exact: true })).toBeEnabled();
  await expect(page.getByRole("main")).toContainText("Test Model");
  await expect(page.getByRole("button", { name: "编辑授权", exact: true })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "保存营业方案", exact: true })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "编辑", exact: true })).toHaveCount(0);
});

test("tech admin reaches account pool without finance or customer tasks", async ({ page }) => {
  await mockViewer(page, { roles: ["tech_admin"] });
  await page.goto("/admin");
  const nav = await openGroup(page, "模型与套餐");
  await expect(nav.getByRole("link", { name: "提供商", exact: true })).toBeVisible();
  for (const href of ["/admin/keys", "/admin/billing", "/admin/commission", "/admin/users"]) await expect(nav.locator(`a[href="${href}"]`)).toHaveCount(0);
  await page.route("**/api/admin/providers/echo-primary", route => route.fulfill({ json: { item: { id: "prd_echo", name: "Echo Primary", slug: "echo-primary", adapter: "sandbox", status: "active", health: "available", models: [] } } }));
  await page.route("**/api/admin/providers/prd_echo/accounts", route => route.fulfill({ json: { items: [] } }));
  await nav.getByRole("link", { name: "提供商", exact: true }).click();
  await page.goto("/admin/providers/echo-primary");
  await expect(page.getByRole("heading", { name: "账号池", exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "添加账号", exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "轮换凭据", exact: true })).toHaveCount(0);
  await page.goto("/admin/billing");
  await expect(page.getByTestId("console-access")).toBeVisible();
  await expect(page.getByRole("button", { name: "查询退款账单", exact: true })).toHaveCount(0);
});

test("audit reads minimal customer and audit facts without any financial or probe write buttons", async ({ page }) => {
  await mockViewer(page, { roles: ["audit_readonly"] });
  await page.goto("/admin");
  const nav = await openGroup(page, "设置与审计");
  await expect(nav.getByRole("link", { name: "审计日志", exact: true })).toBeVisible();
  await expect(nav.locator('a[href="/admin/settings"]')).toHaveCount(0);
  await customerRead(page, "audit_readonly");
  await page.route(/\/api\/admin\/audit-logs(?:\?.*)?$/, route => route.fulfill({ json: { items: [{ id: "audit_original", action: "commission.payout", resource_type: "settlement", resource_id: "cst_original", actor_user_id: "finance_actor", created_at: "2026-10-10T00:00:00Z", request_id: "req_original", before: { status: "settled" }, after: { status: "paid" } }], next_cursor: "" } }));
  await page.goto("/admin/audit");
  await expect(page.getByRole("heading", { name: "审计日志", exact: true })).toBeVisible();
  await expect(page.getByRole("row").filter({ hasText: "cst_original" })).toContainText("commission.payout");
  await expect(page.getByRole("button", { name: "写入探测", exact: true })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "读取 Outbox", exact: true })).toHaveCount(0);
  await page.goto("/admin/billing");
  await expect(page.getByRole("button", { name: "查询退款账单", exact: true })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "赠送额度", exact: true })).toHaveCount(0);
});

test("ops reads minimal customers and edits model grants without refund or organization write access", async ({ page }) => {
  await mockViewer(page, { roles: ["ops_admin"] });
  await page.goto("/admin");
  const nav = await openGroup(page, "模型与套餐");
  await expect(nav.getByRole("link", { name: "模型", exact: true })).toBeVisible();
  await expect(nav.locator('a[href="/admin/audit"]')).toHaveCount(0);
  await customerRead(page, "ops_admin");
  await page.goto("/admin/billing");
  await expect(page.getByRole("button", { name: "赠送额度", exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "查询退款账单", exact: true })).toHaveCount(0);
  await channel(page, "B");
  await expect(page.getByRole("button", { name: "编辑授权", exact: true })).toBeEnabled();
  await expect(page.getByRole("button", { name: "编辑", exact: true })).toHaveCount(0);
});
