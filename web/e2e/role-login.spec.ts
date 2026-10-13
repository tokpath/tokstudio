import { expect, test, type Page } from "@playwright/test";

const accounts = [
  ...["platform_admin", "finance_admin", "ops_admin", "tech_admin", "audit_readonly"].map(role => ({ role, home: "/admin", nav: "平台管理" })),
  { role: "channel_admin", type: "B", home: "/channel", nav: "渠道控制台" },
  ...["channel_admin", "oem_ops", "oem_finance", "oem_audit"].map(role => ({ role, type: "C", home: "/channel", nav: "OEM 管理控制台" })),
  { role: "end_user", home: "/app", nav: "用户控制台" },
  { role: "end_user", partner: "promoter", home: "/app", nav: "用户控制台" },
  { role: "end_user", partner: "agent", home: "/app", nav: "用户控制台" },
];
type Account = typeof accounts[number];

async function authScenario(page: Page, account: Account, signedInInitially = false) {
  let signedIn = signedInInitially;
  const user = { id: "login-role-audit", email: "role-audit@example.test", display_name: "Role audit", roles: [account.role], email_verified: true, login_methods: ["password"] };
  await page.route("**/api/**", route => route.fulfill({ status: 503, json: { error: { message: "Unrelated data not loaded" } } }));
  await page.route("**/api/v1/me", route => route.fulfill({ status: signedIn ? 200 : 401, json: signedIn ? { user } : {} }));
  await page.route("**/api/v1/auth/login", route => {
    expect(route.request().method()).toBe("POST");
    expect(route.request().postDataJSON()).toEqual({ email: user.email, password: "synthetic-password" });
    signedIn = true;
    return route.fulfill({ json: { session: { user } } });
  });
  await page.route("**/api/v1/auth/google/status", route => route.fulfill({ json: { available: false } }));
  await page.route("**/api/v1/partner/me", route => route.fulfill({ status: signedIn && "partner" in account ? 200 : 403, json: "partner" in account ? { role_type: account.partner } : {} }));
  await page.route("**/api/channel/me", route => route.fulfill({ json: { channel_org_id: "role-channel", channel_type: "type" in account ? account.type : "C", channel_name: "Test channel", brand_name: "Test brand" } }));
  await page.route("**/api/channel/delivery", route => route.fulfill({ json: { item: { ready: false, checks: [], delivery: { phase: "configuring" } } } }));
  await page.route("**/api/v1/me/referral", route => route.fulfill({ json: { item: { invited_count: 0, summary: { frozen_minor: 0, available_minor: 0, paid_minor: 0 } } } }));
  await page.route("**/api/admin/ops/dashboard", route => route.fulfill({ json: { dashboard: { totals: {}, alerts: [], dimensions: {}, module_errors: {} } } }));
  await page.route("**/api/admin/metrics/series?*", route => route.fulfill({ json: { items: [] } }));
  await page.route("**/api/v1/me/api-keys?*", route => route.fulfill({ json: { items: [], next_cursor: "" } }));
  await page.route("**/api/v1/me/api-keys", route => route.fulfill({ json: { items: [], next_cursor: "" } }));
  await page.route("**/api/v1/me/balance", route => route.fulfill({ json: { balance: { available: "0" } } }));
  await page.route("**/api/v1/me/notifications/unread-count", route => route.fulfill({ json: { unread: 0 } }));
  await page.route("**/api/v1/me/notifications?*", route => route.fulfill({ json: { items: [], unread: 0, next_cursor: "" } }));
  await page.route("**/api/v1/me/notification-preferences", route => route.fulfill({ json: { email_enabled: true, smtp_available: false } }));
}

for (const account of accounts) {
  const label = `${account.role}-${"type" in account ? account.type : "partner" in account ? account.partner : "default"}`;
  test(`${label} password login, landing, notification and personal-settings return`, async ({ page }) => {
    const errors: string[] = [];
    page.on("pageerror", error => errors.push(error.message));
    await authScenario(page, account);
    await page.addInitScript(() => localStorage.setItem("console-workspace:login-role-audit", "/app"));
    await page.goto("/login?next=%2Fapp");
    await page.getByLabel("邮箱", { exact: true }).fill("role-audit@example.test");
    await page.getByLabel("密码", { exact: true }).fill("synthetic-password");
    await page.getByRole("button", { name: "登录", exact: true }).click();
    await expect(page).toHaveURL(new RegExp(`${account.home}/?$`));
    const nav = page.getByRole("navigation", { name: account.nav, exact: true });
    await expect(nav).toBeVisible();
    await expect(page.getByTestId("console-access")).toHaveCount(0);
    const hrefs = await nav.locator("a[href]").evaluateAll(links => links.map(link => link.getAttribute("href")));
    expect(hrefs.length).toBeGreaterThan(3);
    if (account.role === "tech_admin") {
      expect(hrefs).toContain("/admin/providers");
      for (const href of ["/admin/billing", "/admin/users", "/admin/payments", "/admin/channels", "/admin/commission?tab=payout"]) await expect(page.getByRole("main").locator(`a[href="${href}"]`)).toHaveCount(0);
    }
    if (account.role === "finance_admin") await expect(page.getByRole("main").locator('a[href="/admin/providers"],a[href="/admin/models"]')).toHaveCount(0);
    if (account.role === "oem_finance") expect(hrefs).not.toContain("/channel/subchannels");
    if (account.home !== "/admin") expect(hrefs.some(href => href?.startsWith("/admin"))).toBe(false);
    await page.locator(`a[href="${account.home}/notifications"]`).click();
    await expect(page.getByRole("heading", { name: "通知中心", exact: true })).toBeVisible();
    expect(await nav.locator("a[href]").evaluateAll(links => links.map(link => link.getAttribute("href")))).toEqual(hrefs);
    await page.getByRole("button", { name: "账户菜单", exact: true }).click();
    await page.getByRole("menuitem", { name: "个人资料", exact: true }).click();
    await expect(page).toHaveURL(/\/app\/settings$/);
    await expect(page.getByTestId("console-access")).toHaveCount(0);
    if (account.home !== "/app") {
      await page.getByRole("button", { name: "账户菜单", exact: true }).click();
      await page.getByRole("menuitem", { name: account.role === "platform_admin" ? "平台管理" : "返回管理工作台", exact: true }).click();
      await expect(page).toHaveURL(new RegExp(`${account.home}/?$`));
    }
    await page.goto("/enter");
    await expect(page).toHaveURL(new RegExp(`${account.home}/?$`));
    await page.reload();
    await expect(nav).toBeVisible();
    await page.goto("/app");
    await expect(page).toHaveURL(new RegExp(`${account.home}/?$`));
    await expect(nav).toBeVisible();
    await page.setViewportSize({ width: 390, height: 844 });
    await page.getByRole("button", { name: "打开导航", exact: true }).click();
    await expect(page.getByRole("dialog").getByRole("navigation", { name: account.nav, exact: true })).toBeVisible();
    expect(errors).toEqual([]);
  });
}

for (const [role, target, home] of [
  ["finance_admin", "/admin/providers", "/admin"],
  ["tech_admin", "/admin/billing", "/admin"],
  ["oem_finance", "/channel/keys", "/channel"],
  ["end_user", "/admin/staff", "/app"],
]) {
  test(`${role} rejects a forbidden login return ${target}`, async ({ page }) => {
    await authScenario(page, { role, home, nav: "", type: "C" }, true);
    await page.goto(`/login?next=${encodeURIComponent(target)}`);
    await expect(page).toHaveURL(new RegExp(`${home}/?$`));
    await expect(page.getByTestId("console-access")).toHaveCount(0);
  });
}

test("Google callback validates a staff return target", async ({ page }) => {
  await authScenario(page, { role: "tech_admin", home: "/admin", nav: "平台管理" }, true);
  await page.route("**/api/v1/auth/google/callback", route => route.fulfill({ json: { session: { next: "/admin/billing" } } }));
  await page.goto("/login/oauth/google?state=synthetic-state&code=synthetic-code");
  await expect(page).toHaveURL(/\/admin$/);
  await expect(page.getByRole("navigation", { name: "平台管理", exact: true })).toBeVisible();
});

test("OEM finance reads customers without unauthorized key or child-channel links", async ({ page }) => {
  await authScenario(page, { role: "oem_finance", home: "/channel", nav: "OEM 管理控制台", type: "C" }, true);
  await page.route("**/api/channel/customers?*", route => route.fulfill({ json: { items: [], total: 0, limit: 25, next_cursor: "" } }));
  await page.route("**/api/channel/customer-scopes", route => route.fulfill({ json: { items: [] } }));
  await page.goto("/channel/users");
  await expect(page.getByText("没有匹配的客户。", { exact: true })).toBeVisible();
  await expect(page.locator('a[href="/channel/keys"],a[href="/channel/subchannels"]')).toHaveCount(0);
});
