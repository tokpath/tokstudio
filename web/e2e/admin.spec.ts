import { expect, test } from "@playwright/test";

test("admin P0 nav renders", async ({ page }) => {
  await page.goto("/admin");
  await expect(page.getByText("平台管理")).toBeVisible();
  await expect(page.getByRole("link", { name: "提供商" })).toBeVisible();
  await expect(page.getByRole("link", { name: "审计日志" })).toBeVisible();
});
