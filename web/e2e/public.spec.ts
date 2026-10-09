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

test("user console main flow shows DESIGN.md hero cards", async ({ page }) => {
  await mockViewer(page, { roles: ["end_user"] });
  await page.route("**/v1/me/balance**", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ balance: { available: "12.00", reserved: "0" } }),
    });
  });
  await page.route("**/v1/me/api-keys**", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ items: [] }),
    });
  });
  await page.route("**/v1/me/usage**", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ items: [], keys: [], models: [] }),
    });
  });
  await page.goto("/app");
  await expect(page.getByRole("heading", { name: "我的账户" })).toBeVisible();
  const overview = page.getByLabel("总览");
  await expect(overview.getByText("可用余额")).toBeVisible();
  await expect(overview.getByText("预授权占用")).toBeVisible();
  await expect(overview.getByText("API Key")).toBeVisible();
  await expect(overview.getByText("路由回单")).toBeVisible();
  await expect(page.getByRole("heading", { name: "用量趋势" })).toBeVisible();
  await expect(page.getByTestId("overview-trend-chart")).toBeVisible();
  await expect(page.getByRole("heading", { name: "第一次使用" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "快捷入口" })).toHaveCount(0);
  await expect(page.getByRole("heading", { name: "API Key" })).toHaveCount(0);
});

const consolePaths = [
  "/app",
  "/app/playground",
  "/app/keys",
  "/app/catalog",
  "/app/usage",
  "/app/reconciliation",
  "/app/activity",
  "/app/wallet",
  "/app/plans",
  "/app/media",
  "/app/referral",
  "/app/docs",
  "/app/settings",
  "/app/settings/team",
  "/app/settings/members",
  "/app/settings/billing",
  "/app/settings/quotas",
  "/app/settings/apps",
  "/app/settings/webhooks",
];

test("authenticated ofox replica pages render headings", async ({ page }) => {
  await mockViewer(page, { roles: ["end_user"] });
  for (const path of consolePaths) {
    const response = await page.goto(path);
    expect(response?.ok(), path).toBeTruthy();
    await expect(page.getByTestId("console-shell"), path).toBeVisible();
    await expect(page.getByTestId("console-access"), path).toHaveCount(0);
    await expect(page.locator("h1"), path).toBeVisible();
  }
});

test("channel and partner consoles use grouped real routes", async ({ page }) => {
  await mockViewer(page, { roles: ["channel_admin"], channelType: "B" });
  await page.goto("/channel");
  const channelNav = page.getByRole("navigation", { name: "渠道控制台" });
  await expect(channelNav.getByRole("link", { name: "总览" })).toBeVisible();
  await expect(channelNav.getByRole("link", { name: "本渠道用户" })).toBeVisible();
  await expect(channelNav.getByRole("link", { name: "本渠道 API Key" })).toBeVisible();
  await expect(channelNav.getByRole("link", { name: "收款" })).toHaveCount(0);
  await expect(channelNav.getByRole("link", { name: "进货" })).toHaveCount(0);
  await expect(channelNav.getByRole("link", { name: "规则" })).toHaveCount(0);
  await expect(channelNav.getByRole("link", { name: "对账", exact: true })).toHaveCount(0);
  await expect(channelNav.getByRole("link", { name: "待对账", exact: true })).toHaveCount(0);
  await channelNav.getByRole("link", { name: "本渠道用户" }).click();
  await expect(page).toHaveURL(/\/channel\/users/);
  await expect(page.locator("h1")).toHaveText("本渠道用户");
  await page.goto("/channel/keys");
  await expect(page.locator("h1")).toHaveText("本渠道 API Key");
  await expect(page.getByText("暂无本渠道 API Key")).toHaveCount(0);
  await expect(page.getByRole("button", { name: "重试" }).or(page.getByRole("link", { name: "重新登录" }))).toBeVisible();
  for (const path of ["/channel/payments", "/channel/payments/lanes", "/channel/payments/rules", "/channel/ledger", "/channel/reconciliation", "/channel/rules"]) {
    await page.goto(path);
    await expect(page.getByTestId("console-access"), path).toBeVisible();
    await expect(page.getByRole("navigation", { name: "支付管理" }), path).toHaveCount(0);
    await expect(page.getByRole("button", { name: "线下收款划拨", exact: true }), path).toHaveCount(0);
  }

  await mockViewer(page, { roles: ["channel_admin"], channelType: "C" });
  await page.goto("/channel");
  const oemNav = page.getByRole("navigation", { name: "OEM 管理控制台", exact: true });
  await oemNav.getByRole("link", { name: "支付", exact: true }).click();
  await expect(page).toHaveURL(/\/channel\/payments$/);
  await expect(page.getByRole("heading", { name: "支付", level: 1, exact: true })).toBeVisible();
  const paymentsNav = page.getByRole("navigation", { name: "支付管理", exact: true });
  await expect(paymentsNav.getByRole("link")).toHaveText(["支付订单", "支付通道", "收银台规则"]);
  await expect(page.getByRole("button", { name: "线下收款划拨", exact: true })).toBeVisible();
  await oemNav.getByRole("link", { name: "余额/充值", exact: true }).click();
  await expect(page).toHaveURL(/\/channel\/ledger$/);
  await expect(page.getByRole("heading", { name: "余额/充值", level: 1, exact: true })).toBeVisible();

  await mockViewer(page, { roles: ["end_user"], partner: true });
  await page.goto("/partner");
  const partnerNav = page.getByRole("navigation", { name: "分销控制台" });
  await expect(partnerNav.getByRole("link", { name: "我的层级" })).toBeVisible();
  await partnerNav.getByRole("link", { name: "范围内佣金" }).click();
  await expect(page).toHaveURL(/\/partner\/commissions/);
});

test("admin overview shows DESIGN.md hero stats", async ({ page }) => {
  await mockViewer(page, { roles: ["platform_admin"] });
  await page.goto("/admin");
  const overview = page.getByLabel("管理总览");
  await expect(overview.getByText("待对账")).toBeVisible();
  await expect(overview.getByText("毛利")).toBeVisible();
  await expect(overview.getByText("佣金负债")).toBeVisible();
  await expect(overview.getByText("Provider 健康")).toBeVisible();
});

test("desktop compatibility path opens actual integration documentation", async ({ page }) => {
  await page.goto("/desktop");
  await expect(page).toHaveURL(/\/docs\/integrations$/);
  await expect(page.locator("h1")).toBeVisible();
  await expect(page.getByRole("link", { name: /下载安装|下载客户端/ })).toHaveCount(0);
});

test("channel users page does not treat a failed load as empty", async ({ page }) => {
  await mockViewer(page, { roles: ["channel_admin"], channelType: "B" });
  await page.goto("/channel/users");
  await expect(page.locator("h1")).toHaveText("本渠道用户");
  await expect(page.getByText("暂无本渠道用户")).toHaveCount(0);
  await expect(page.getByRole("button", { name: "重试" }).or(page.getByRole("link", { name: "重新登录" }))).toBeVisible();
});

test("channel users empty account explains next step", async ({ page }) => {
  await mockViewer(page, { roles: ["channel_admin"] });
  await page.route("**/channel/users**", async (route) => {
    if (route.request().resourceType() !== "fetch" && route.request().resourceType() !== "xhr") {
      await route.continue();
      return;
    }
    await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ items: [] }) });
  });
  await page.goto("/channel/users");
  await expect(page.getByTestId("list-resource")).toHaveAttribute("data-list-phase", "empty");
  await expect(page.getByText("暂无本渠道用户")).toBeVisible();
  await expect(page.getByText("先去推广页创建推广码")).toBeVisible();
});

test("user console sidebar groups match ofox IA", async ({ page }) => {
  await mockViewer(page, { roles: ["end_user"] });
  await page.goto("/app");
  const nav = page.getByRole("navigation", { name: "用户控制台" });
  await expect(nav.getByRole("link", { name: "总览" })).toBeVisible();
  await expect(nav.getByRole("link", { name: "快速试用" })).toBeVisible();
  await expect(nav.getByRole("link", { name: "API Key" })).toBeVisible();
  await expect(nav.getByRole("link", { name: "模型广场" })).toBeVisible();
  await expect(nav.getByRole("link", { name: "请求明细" })).toBeVisible();
  await expect(nav.getByRole("link", { name: "推荐计划" })).toBeVisible();
  await nav.getByRole("link", { name: "快速试用" }).click();
  await expect(page).toHaveURL(/\/app\/playground/);
  await expect(page.getByRole("heading", { name: "快速试用" })).toBeVisible();
});
