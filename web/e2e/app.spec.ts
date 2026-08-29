import { expect, test } from "@playwright/test";

test("user console shows API Key panel", async ({ page }) => {
  await page.goto("/app");
  await expect(page.getByRole("link", { name: "用户控制台" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "API Key" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "用量与账单" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "媒体任务" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "个人设置" })).toBeVisible();
  await expect(page.getByRole("button", { name: "创建视频任务" })).toBeVisible();
  await expect(page.getByRole("button", { name: "保存资料" })).toBeVisible();
});
