import { expect, test } from "@playwright/test";
import { mockViewer } from "./mock-viewer";

test("unsigned console button goes to login", async ({ page }) => {
  await page.goto("/");
  const header = page.locator("header").first();
  const consoleLink = header.getByRole("link", { name: "控制台" });
  await expect(consoleLink).toBeVisible();
  await expect(header.getByRole("link", { name: "登录" })).toHaveCount(0);
  await expect(header.getByRole("link", { name: "注册" })).toHaveCount(0);
  await expect(consoleLink).toHaveAttribute("href", /\/login\?next=%2Fenter|\/enter/);
  await consoleLink.click();
  await expect(page).toHaveURL(/\/login/);
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

test("partner console button opens partner console", async ({ page }) => {
  await mockViewer(page, { roles: ["end_user"], partner: true });
  await page.goto("/");
  const consoleLink = page.locator("header").first().getByRole("link", { name: "控制台" });
  await expect(consoleLink).toHaveAttribute("href", "/partner");
  await consoleLink.click();
  await expect(page).toHaveURL(/\/partner\/?$/);
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
