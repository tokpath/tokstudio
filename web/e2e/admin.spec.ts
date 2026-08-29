import { expect, test } from "@playwright/test";

test("admin P0 nav renders", async ({ page }) => {
  await page.goto("/admin");
  await expect(page.getByRole("link", { name: "平台管理" })).toBeVisible();
  await expect(page.getByRole("link", { name: "提供商" })).toBeVisible();
  await expect(page.getByRole("link", { name: "套餐审核" })).toBeVisible();
  await expect(page.getByRole("link", { name: "价格" })).toBeVisible();
  await expect(page.getByRole("link", { name: "支付" })).toBeVisible();
  await expect(page.getByRole("link", { name: "指标" })).toBeVisible();
  await expect(page.getByRole("link", { name: "佣金策略" })).toBeVisible();
  await expect(page.getByRole("link", { name: "审计日志" })).toBeVisible();
});

test("admin plan review and commission pages render", async ({ page }) => {
  await page.goto("/admin/plans");
  await expect(page.getByRole("heading", { name: "套餐审核" })).toBeVisible();
  await expect(page.getByRole("button", { name: "待审核" })).toBeVisible();
  await page.goto("/admin/commission");
  await expect(page.getByRole("heading", { name: "佣金策略" })).toBeVisible();
  await expect(page.getByRole("button", { name: "读取策略" })).toBeVisible();
  await page.goto("/admin/payments");
  await expect(page.getByRole("heading", { name: "支付", exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "确认支付" })).toBeVisible();
  await page.goto("/admin/prices");
  await expect(page.getByRole("heading", { name: "价格", exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "发布价格" })).toBeVisible();
  await page.goto("/admin/metrics");
  await expect(page.getByRole("heading", { name: "运营看板" })).toBeVisible();
});
