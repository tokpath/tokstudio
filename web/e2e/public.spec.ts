import { expect, test } from "@playwright/test";

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
  "/leaderboards/models",
  "/leaderboards/apps",
  "/leaderboards/labs",
  "/vs/openrouter",
  "/compare",
  "/promo",
  "/promo/august",
  "/desktop",
  "/verify",
  "/awesome-ofox",
  "/pricing",
  "/blog",
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

test("login page has no ofox copy", async ({ page }) => {
  await page.goto("/login");
  await expect(page.getByRole("heading", { name: "注册 / 登录" })).toBeVisible();
  await expect(page.locator("body")).not.toContainText(/ofox/i);
  await expect(page.getByText("结构对齐")).toHaveCount(0);
});

test("public ofox replica pages render headings", async ({ page }) => {
  for (const path of publicPaths) {
    const response = await page.goto(path);
    expect(response?.ok(), path).toBeTruthy();
    await expect(page.locator("h1")).toBeVisible();
  }
});

test("user console main flow shows DESIGN.md hero cards", async ({ page }) => {
  await page.goto("/app");
  await expect(page.getByRole("heading", { name: "我的账户" })).toBeVisible();
  const overview = page.getByLabel("总览");
  await expect(overview.getByText("可用余额")).toBeVisible();
  await expect(overview.getByText("预授权占用")).toBeVisible();
  await expect(overview.getByText("API Key")).toBeVisible();
  await expect(overview.getByText("路由回单")).toBeVisible();
  await expect(page.getByRole("heading", { name: "API Key" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "用量与账单" })).toBeVisible();
});

const consolePaths = [
  "/app",
  "/app/playground",
  "/app/keys",
  "/app/catalog",
  "/app/usage",
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
  for (const path of consolePaths) {
    const response = await page.goto(path);
    expect(response?.ok(), path).toBeTruthy();
    await expect(page.locator("h1"), path).toBeVisible();
  }
});

test("channel and partner consoles use grouped real routes", async ({ page }) => {
  await page.goto("/channel");
  const channelNav = page.getByRole("navigation", { name: "渠道控制台" });
  await expect(channelNav.getByRole("link", { name: "总览" })).toBeVisible();
  await expect(channelNav.getByRole("link", { name: "本渠道用户" })).toBeVisible();
  await channelNav.getByRole("link", { name: "本渠道用户" }).click();
  await expect(page).toHaveURL(/\/channel\/users/);
  await expect(page.locator("h1")).toHaveText("本渠道用户");

  await page.goto("/partner");
  const partnerNav = page.getByRole("navigation", { name: "分销控制台" });
  await expect(partnerNav.getByRole("link", { name: "我的层级" })).toBeVisible();
  await partnerNav.getByRole("link", { name: "范围内佣金" }).click();
  await expect(page).toHaveURL(/\/partner\/commissions/);
});

test("admin overview shows DESIGN.md hero stats", async ({ page }) => {
  await page.goto("/admin");
  const overview = page.getByLabel("管理总览");
  await expect(overview.getByText("待对账")).toBeVisible();
  await expect(overview.getByText("毛利")).toBeVisible();
  await expect(overview.getByText("佣金负债")).toBeVisible();
  await expect(overview.getByText("Provider 健康")).toBeVisible();
});

test("desktop landing lists tools without a fake installer", async ({ page }) => {
  await page.goto("/desktop");
  await expect(page.getByRole("heading", { name: "本机编程工具，一个账户接入" })).toBeVisible();
  await expect(page.getByText("Claude Code", { exact: true })).toBeVisible();
  await expect(page.getByText("未发布")).toBeVisible();
  await expect(page.getByRole("link", { name: "看接入片段" })).toBeVisible();
});

test("channel users page shows empty ledger table", async ({ page }) => {
  await page.goto("/channel/users");
  await expect(page.locator("h1")).toHaveText("本渠道用户");
  await expect(page.getByText("暂无本渠道用户")).toBeVisible();
});

test("user console sidebar groups match ofox IA", async ({ page }) => {
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
