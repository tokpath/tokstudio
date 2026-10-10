import { expect, test } from "@playwright/test";
import { mockViewer } from "./mock-viewer";

const publicPaths = [
  "/",
  "/models",
  "/quickstart",
  "/docs",
  "/docs/integrations",
  "/docs/develop",
  "/docs/changelog",
  "/enterprise",
  "/trust",
  "/trust/subprocessors",
  "/best-value",
  "/model-finder",
  "/vibe-coding",
  "/video",
  "/image",
  "/compare",
  "/promo",
  "/desktop",
  "/verify",
  "/pricing",
  "/terms",
  "/privacy",
  "/login",
];

test("model catalog vendor query stays on the models path", async ({ page }) => {
  await page.goto("/models?vendor=z-ai");
  await expect(page).toHaveURL(/\/models\?vendor=z-ai/);
  await expect(page.getByLabel("按厂商筛选")).toBeVisible();
  await expect(page.getByRole("link", { name: "全部厂商" })).toBeVisible();
});

test("login page shows only available authentication methods", async ({ page }) => {
  await page.route("**/v1/auth/google/status", (route) => route.fulfill({ json: { available: false } }));
  await page.goto("/login");
  await expect(page.getByRole("heading", { name: "注册 / 登录" })).toBeVisible();
  await expect(page.locator("body")).not.toContainText(/ofox/i);
  await expect(page.getByText("结构对齐")).toHaveCount(0);
  await expect(page.locator("header")).toHaveCount(0);
  await expect(page.locator("footer")).toHaveCount(0);
  await expect(page.getByText("发送验证码")).toHaveCount(0);
  await expect(page.getByText("用验证码登录")).toHaveCount(0);
  await expect(page.getByRole("button", { name: "使用 Google 登录" })).toHaveCount(0);
});

test("google oauth callback without code returns to login", async ({ page }) => {
  await page.goto("/login/oauth/google");
  await expect(page).toHaveURL(/\/login\?/);
  await expect(page.getByRole("alert").filter({ hasText: /Google 登录失败/ })).toBeVisible();
  await expect(page.locator("body")).not.toContainText(/ya29\.|mock:/);
  await expect(page.getByText("绑定成功")).toHaveCount(0);
});

test("available public pages render headings", async ({ page }) => {
  for (const path of publicPaths) {
    const response = await page.goto(path);
    expect(response?.ok(), path).toBeTruthy();
    await expect(page.locator("h1")).toBeVisible();
  }
});

test("user console begins with Key creation without requiring a funded account", async ({ page }) => {
  await mockViewer(page, { roles: ["end_user"] });
  await page.route("**/v1/me/balance**", route => route.fulfill({ json: { balance: { available: "0", reserved: "0" } } }));
  await page.route("**/v1/me/api-keys**", route => route.fulfill({ json: { items: [], next_cursor: "" } }));
  await page.route("**/v1/public/models**", route => route.fulfill({ json: { items: [], next_cursor: "" } }));
  await page.goto("/app");
  await expect(page.getByRole("heading", { level: 1, name: "API Key", exact: true })).toBeVisible();
  await expect(page.getByTestId("balance-pill")).toHaveText("$0.00");
  await page.getByRole("button", { name: "创建 API Key", exact: true }).first().click();
  const dialog = page.getByRole("dialog");
  await expect(dialog.getByLabel("密钥名称", { exact: true })).toBeVisible();
  await expect(dialog.getByRole("group", { name: "可用模型", exact: true })).toBeVisible();
  await expect(dialog.getByRole("group", { name: "USD 消耗上限", exact: true })).toBeVisible();
  await expect(dialog.getByRole("group", { name: "有效期", exact: true })).toBeVisible();
  await expect(dialog.getByRole("button", { name: "创建", exact: true })).toBeEnabled();
});

const consolePaths = [
  "/app", "/app/keys", "/app/usage", "/app/wallet", "/app/plans",
  "/app/referral", "/app/media", "/app/docs", "/app/settings", "/app/profile",
];

test("authenticated available user tasks render their actual headings", async ({ page }) => {
  await mockViewer(page, { roles: ["end_user"] });
  for (const path of consolePaths) {
    const response = await page.goto(path);
    expect(response?.ok(), path).toBeTruthy();
    await expect(page.getByTestId("console-shell"), path).toBeVisible();
    await expect(page.getByTestId("console-access"), path).toHaveCount(0);
    await expect(page.locator("h1"), path).toBeVisible();
  }
});

test("channel and OEM workspaces expose only their actual financial responsibilities", async ({ page }) => {
  await mockViewer(page, { roles: ["channel_admin"], channelType: "B" });
  await page.goto("/channel");
  const channelNav = page.getByRole("navigation", { name: "渠道控制台", exact: true });
  await expect(channelNav.locator('a[href="/channel/users"]')).toBeVisible();
  for (const path of ["promos", "models", "commissions"]) await expect(channelNav.locator(`a[href="/channel/${path}"]`)).toBeVisible();
  await expect(channelNav.locator('a[href="/channel/keys"]')).toHaveCount(0);
  for (const path of ["payments", "payments/lanes", "payments/rules", "ledger", "reconciliation", "rules"]) {
    await expect(channelNav.locator(`a[href="/channel/${path}"]`)).toHaveCount(0);
    await page.goto(`/channel/${path}`);
    await expect(page.getByTestId("console-access")).toBeVisible();
    await expect(page.getByRole("button", { name: "线下收款划拨", exact: true })).toHaveCount(0);
  }
  await mockViewer(page, { roles: ["channel_admin"], channelType: "C" });
  await page.goto("/channel");
  const oemNav = page.getByRole("navigation", { name: "OEM 管理控制台", exact: true });
  await expect(oemNav.locator('a[href="/channel/payments"]')).not.toBeVisible();
  await oemNav.getByRole("button", { name: "资金与结算", exact: true }).click();
  await expect(oemNav.locator('a[href="/channel/payments"]')).toBeVisible();
  await expect(oemNav.locator('a[href="/channel/ledger"]')).toBeVisible();
  await oemNav.locator('a[href="/channel/payments"]').click();
  await expect(page).toHaveURL(/\/channel\/payments$/);
  await expect(page.getByTestId("console-access")).toHaveCount(0);
  await expect(page.getByRole("navigation", { name: "支付管理", exact: true })).toBeVisible();
});

test("legacy professional commissions retain their page within the same user account", async ({ page }) => {
  await mockViewer(page, { roles: ["end_user"], partner: true });
  let requestedPage = "";
  await page.route("**/v1/me/referral?**", route => {
    requestedPage = new URL(route.request().url()).searchParams.get("page") || "";
    return route.fulfill({ json: { item: { codes: ["PROINVITE"], code_links: [{ code: "PROINVITE", share_url: "https://brand.test/login?promotion_code=PROINVITE" }], can_create: false, invited_count: 4, can_commission: true, professional_customers: true, rules: { spend_minor: 0, topup_minor: 0, gift_minor: 0 }, progress: { spend_minor: 0, largest_topup_minor: 0, gift_granted_minor: 0, gift_remaining_minor: 0 }, summary: { earned_minor: 10000000, frozen_minor: 0, available_minor: 10000000, held_minor: 0, settled_minor: 0, paid_minor: 0, reversed_minor: 0 }, rewards: [{ id: "entry-own-31", request_id: "request-own-31", kind: "direct", status: "available", amount_minor: 10000000 }], settlements: [], pagination: { page: 2, page_size: 25, rewards_total: 31, settlements_total: 0 } } } });
  });
  await page.goto("/partner/commissions?page=2");
  await expect(page).toHaveURL(/\/app\/referral\?tab=commissions&page=2$/);
  await expect(page.getByRole("navigation", { name: "用户控制台", exact: true })).toBeVisible();
  await expect(page.getByRole("heading", { level: 1, name: "邀请与收益", exact: true })).toBeVisible();
  await expect(page.getByText("request-own-31", { exact: true })).toBeVisible();
  expect(requestedPage).toBe("2");
});

test("admin workbench links confirmed financial and operational facts to their tasks", async ({ page }) => {
  await mockViewer(page, { roles: ["platform_admin"] });
  await page.route("**/api/admin/ops/dashboard", route => route.fulfill({ json: { dashboard: { totals: { pending_reconciliation_count: 7, revenue_minor: 50000000, upstream_cost_minor: 25000000, gross_profit_minor: 25000000, commission_liability_minor: 3000000 }, provider_health: { state: "healthy" } } } }));
  await page.route("**/api/admin/metrics/series?**", route => route.fulfill({ json: { items: [] } }));
  await page.route("**/api/admin/billing/report", route => route.fulfill({ json: { report: {
    scope: "platform_business", self_revenue_minor: 10000000, oem_sales_minor: 40000000, revenue_minor: 50000000,
    upstream_cost_minor: 25000000, gross_profit_minor: 25000000, commission_liability_minor: 3000000,
    marketing_minor: -3000000, operating_profit_minor: 22000000, pending_reconciliation_count: 7,
  } } }));
  await page.route("**/api/admin/oem-purchases?**", route => route.fulfill({ json: { items: [], next_cursor: "" } }));
  await page.goto("/admin");
  await expect(page.getByRole("heading", { level: 1, name: "工作台", exact: true })).toBeVisible();
  const facts = page.getByRole("region", { name: "待办与常用任务", exact: true });
  await expect(facts.locator('a[href="/admin/reconciliation"]')).toContainText("7");
  await expect(facts.locator('a[href="/admin/billing"]')).toContainText("平台 API 毛利");
  await expect(facts.locator('a[href="/admin/billing"]')).toContainText("$25.00");
  await expect(facts.locator('a[href="/admin/margin"]')).toHaveCount(0);
  await expect(facts.locator('a[href="/admin/commission?tab=payout"]')).toContainText("$3.00");
  await expect(facts.locator('a[href="/admin/providers"]')).toContainText("正常");
  for (const href of ["/admin/models", "/admin/channels", "/admin/payments", "/admin/usage?tab=requests"]) await expect(page.getByRole("main").locator(`a[href="${href}"]`)).toBeVisible();
  await facts.locator('a[href="/admin/billing"]').click();
  await expect(page).toHaveURL(/\/admin\/billing$/);
  for (const [label, amount] of [
    ["平台营业收入", "$50.00 USD"], ["平台自营消费收入", "$10.00 USD"], ["OEM 服务额度销售", "$40.00 USD"],
    ["已发生 API 服务成本", "$25.00 USD"], ["平台 API 毛利", "$25.00 USD"], ["平台 API 经营利润", "$22.00 USD"],
  ]) await expect(page.locator("dl > div").filter({ has: page.getByText(label, { exact: true }) })).toContainText(amount);

});

test("desktop compatibility path opens actual integration documentation", async ({ page }) => {
  await page.goto("/desktop");
  await expect(page).toHaveURL(/\/docs\/integrations$/);
  await expect(page.locator("h1")).toBeVisible();
  await expect(page.getByRole("link", { name: /下载安装|下载客户端/ })).toHaveCount(0);
});

test("channel customers do not treat a failed load as zero or empty", async ({ page }) => {
  await mockViewer(page, { roles: ["channel_admin"], channelType: "B" });
  await page.route("**/api/channel/customer-scopes", route => route.fulfill({ json: { items: [] } }));
  let failed = true;
  await page.route("**/api/channel/customers?**", route => route.fulfill(failed ? { status: 503, json: { error: { message: "customer service unavailable" } } } : { json: { items: [], total: 0, limit: 25, next_cursor: "" } }));
  await page.goto("/channel/users");
  // React Query exhausts its normal read retries before exposing the error.
  await expect(page.getByRole("alert").filter({ hasText: "customer service unavailable" })).toBeVisible({ timeout: 10000 });
  await expect(page.getByText("没有匹配的客户。", { exact: true })).toHaveCount(0);
  await expect(page.getByText(/匹配 0 位客户/)).toHaveCount(0);
  await expect(page.getByRole("button", { name: "从第一页重试", exact: true })).toBeVisible();
  failed = false;
  await page.getByRole("button", { name: "从第一页重试", exact: true }).click();
  await expect(page.getByText("没有匹配的客户。", { exact: true })).toBeVisible();
  await expect(page.getByRole("alert").filter({ hasText: "customer service unavailable" })).toHaveCount(0);
});

test("channel customers confirm an actual empty result within the accessible scope", async ({ page }) => {
  await mockViewer(page, { roles: ["channel_admin"], channelType: "B" });
  await page.route("**/api/channel/customer-scopes", route => route.fulfill({ json: { items: [{ id: "chn_oem_c", code: "Current channel", brand_name: "Current brand" }] } }));
  await page.route("**/api/channel/customers?**", route => route.fulfill({ json: { items: [], total: 0, limit: 25, next_cursor: "" } }));
  await page.goto("/channel/users");
  await expect(page.getByText("没有匹配的客户。", { exact: true })).toBeVisible();
  await expect(page.getByText("匹配 0 位客户 · 本页 0 位", { exact: true })).toBeVisible();
  await expect(page.getByLabel("客户渠道", { exact: true })).toHaveValue("");
  await expect(page.getByRole("button", { name: "下一页", exact: true })).toBeDisabled();
});

test("user navigation connects Key usage wallet and invitations without exposing operations", async ({ page }) => {
  await mockViewer(page, { roles: ["end_user"] });
  await page.goto("/app");
  const nav = page.getByRole("navigation", { name: "用户控制台", exact: true });
  for (const path of ["keys", "usage", "wallet", "referral"]) await expect(nav.locator(`a[href="/app/${path}"]`)).toBeVisible();
  await expect(nav.locator('a[href^="/admin/"]')).toHaveCount(0);
  await expect(nav.locator('a[href^="/channel/"]')).toHaveCount(0);
  await nav.locator('a[href="/app/keys"]').click();
  await expect(page).toHaveURL(/\/app\/keys$/);
  await expect(page.getByRole("heading", { level: 1, name: "API Key", exact: true })).toBeVisible();
});
