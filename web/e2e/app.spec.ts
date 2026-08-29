import { expect, test } from "@playwright/test";

test("user console shows API Key panel", async ({ page }) => {
  await page.goto("/app");
  await expect(page.getByRole("link", { name: "用户控制台" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "API Key" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "用量与账单" })).toBeVisible();
});
