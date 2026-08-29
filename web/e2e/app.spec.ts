import { expect, test } from "@playwright/test";

test("public storefront shows models plans and topup", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByRole("heading", { name: "可用模型" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "套餐与订阅" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "充值" })).toBeVisible();
  await expect(page.getByRole("button", { name: "兑换码充值" })).toBeVisible();
  await expect(page.getByRole("button", { name: "创建支付充值" })).toBeVisible();
});

test("user console shows API Key panel", async ({ page }) => {
  await page.goto("/app");
  await expect(page.getByRole("link", { name: "用户控制台" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "API Key" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "用量与账单" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "媒体任务" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "个人设置" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "接入示例" })).toBeVisible();
  await expect(page.getByRole("button", { name: "创建视频任务" })).toBeVisible();
  await expect(page.getByRole("button", { name: "保存资料" })).toBeVisible();
  await expect(page.getByRole("button", { name: "复制 curl" })).toBeVisible();
});

test("partner console shows scoped downline cards", async ({ page }) => {
  await page.goto("/partner");
  await expect(page.getByRole("heading", { name: "我的层级" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "范围内用户" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "范围内佣金" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "范围内结算" })).toBeVisible();
});

test("channel console shows scoped user list", async ({ page }) => {
  await page.goto("/channel");
  await expect(page.getByRole("heading", { name: "本渠道用户" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "本渠道套餐" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "推广链接" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "本渠道归因" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "本渠道用量" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "本渠道结算" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "渠道额度与佣金" })).toBeVisible();
});
