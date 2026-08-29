import { expect, test } from "@playwright/test";

test("user console shows API Key panel", async ({ page }) => {
  await page.goto("/app");
  await expect(page.getByText("用户控制台")).toBeVisible();
  await expect(page.getByRole("heading", { name: "API Key" })).toBeVisible();
});
