import { expect, test } from "@playwright/test";
import { mockViewer } from "./mock-viewer";

test.beforeEach(async ({ page }) => {
  await page.route("**/api/**", route => route.fulfill({ status: 503, json: { error: { message: "本场景未加载此业务数据" } } }));
});

test("unsigned console button goes to login", async ({ page }) => {
  await page.route("**/v1/me", route => route.fulfill({ status: 401, json: {} }));
  await page.route("**/v1/partner/me", route => route.fulfill({ status: 401, json: {} }));
  await page.goto("/");
  const header = page.locator("header").first();
  const consoleLink = header.getByRole("link", { name: "控制台" });
  await expect(consoleLink).toBeVisible();
  await expect(header.getByRole("link", { name: "登录" })).toHaveCount(0);
  await expect(header.getByRole("link", { name: "注册" })).toHaveCount(0);
  await expect(consoleLink).toHaveAttribute("href", /\/login\?next=%2Fenter|\/enter/);
  await consoleLink.click();
  await expect(page).toHaveURL(/\/login/);
  await expect.poll(() => new URL(page.url()).searchParams.get("next")).toBe("/enter");
  await expect(page.getByRole("heading", { name: "注册 / 登录" })).toBeVisible();
});

test("platform admin console button opens admin", async ({ page }) => {
  await mockViewer(page, { roles: ["platform_admin"] });
  await page.goto("/");
  const consoleLink = page.locator("header").first().getByRole("link", { name: "控制台" });
  await expect(consoleLink).toHaveAttribute("href", "/admin");
  await consoleLink.click();
  await expect(page).toHaveURL(/\/admin\/?$/);
});

test("channel admin console button opens channel console", async ({ page }) => {
  await mockViewer(page, { roles: ["channel_admin"] });
  await page.goto("/");
  const consoleLink = page.locator("header").first().getByRole("link", { name: "控制台" });
  await expect(consoleLink).toHaveAttribute("href", "/channel");
  await consoleLink.click();
  await expect(page).toHaveURL(/\/channel\/?$/);
});

test("inviting user console button opens the same user account", async ({ page }) => {
  await mockViewer(page, { roles: ["end_user"], partner: true });
  await page.goto("/");
  const consoleLink = page.locator("header").first().getByRole("link", { name: "控制台" });
  await expect(consoleLink).toHaveAttribute("href", "/app");
  await consoleLink.click();
  await expect(page).toHaveURL(/\/app\/?$/);
});

test("end user console button opens user console", async ({ page }) => {
  await mockViewer(page, { roles: ["end_user"] });
  await page.goto("/");
  const consoleLink = page.locator("header").first().getByRole("link", { name: "控制台" });
  await expect(consoleLink).toHaveAttribute("href", "/app");
  await consoleLink.click();
  await expect(page).toHaveURL(/\/app\/?$/);
});

test("enter route dispatches platform admin to admin", async ({ page }) => {
  await mockViewer(page, { roles: ["platform_admin"] });
  await page.goto("/enter");
  await expect(page).toHaveURL(/\/admin\/?$/);
});

test("multi-role account returns to its last authorized workspace", async ({ page }) => {
  await mockViewer(page, { roles: ["platform_admin", "channel_admin"], channelType: "C" });
  await page.addInitScript(() => localStorage.setItem("console-workspace:usr_rbac", "/channel"));
  await page.goto("/");
  const consoleLink = page.locator("header").first().getByRole("link", { name: "控制台", exact: true });
  await expect(consoleLink).toHaveAttribute("href", "/channel");
  await consoleLink.click();
  await expect(page).toHaveURL(/\/channel\/?$/);
});

test("enter honors an authorized model task and preserves its return context", async ({ page }) => {
  await mockViewer(page, { roles: ["end_user"] });
  const task = "/app/keys?create=1&model=test%2Fmodel&next=%2Fmodels%2Ftest%252Fmodel";
  await page.goto(`/enter?next=${encodeURIComponent(task)}`);
  await expect.poll(() => { const url = new URL(page.url()); return url.pathname + url.search; }).toBe(task);
});

test("enter rejects a saved or requested staff workspace after the account loses that role", async ({ page }) => {
  await mockViewer(page, { roles: ["end_user"] });
  await page.addInitScript(() => localStorage.setItem("console-workspace:usr_rbac", "/admin"));
  await page.goto("/enter?next=%2Fadmin%2Fbilling");
  await expect(page).toHaveURL(/\/app\/?$/);
  await expect(page.locator('nav a[href="/admin/billing"]')).toHaveCount(0);
});
