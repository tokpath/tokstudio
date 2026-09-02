import { expect, test } from "@playwright/test";
import { mockViewer } from "./mock-viewer";

test("unsigned admin still shows the full P0 nav", async ({ page }) => {
  await page.goto("/admin");
  await expect(page.getByRole("link", { name: "提供商" })).toBeVisible();
  await expect(page.getByRole("link", { name: "用户/项目" })).toBeVisible();
  await expect(page.getByRole("link", { name: "审计日志" })).toBeVisible();
  await expect(page.getByRole("link", { name: "余额/充值" })).toBeVisible();
});

test("finance admin sees ledger menus and not upstream keys", async ({ page }) => {
  await mockViewer(page, { roles: ["finance_admin"] });
  await page.goto("/admin");
  await expect(page.getByRole("link", { name: "余额/充值" })).toBeVisible();
  await expect(page.getByRole("link", { name: "渠道租户" })).toBeVisible();
  await expect(page.getByRole("link", { name: "佣金策略" })).toBeVisible();
  await expect(page.getByRole("link", { name: "提供商" })).toHaveCount(0);
  await expect(page.getByRole("link", { name: "用户/项目" })).toHaveCount(0);
  await expect(page.getByRole("link", { name: "审计日志" })).toHaveCount(0);
  await page.goto("/admin/billing");
  await expect(page.getByRole("button", { name: "退消费账单" })).toBeVisible();
  await expect(page.getByRole("button", { name: "赠送额度" })).toBeVisible();
  await page.goto("/admin/channels");
  await expect(page.getByRole("button", { name: "新建渠道" })).toHaveCount(0);
  await expect(page.getByRole("tab", { name: "代理商" })).toHaveCount(0);
  await page.goto("/admin/channels/chn_reseller_b");
  await expect(page.getByRole("button", { name: "编辑" })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "从平台目录授权" })).toHaveCount(0);
  await expect(page.getByRole("heading", { name: "渠道额度" })).toBeVisible();
});

test("tech admin sees credentials and not refunds", async ({ page }) => {
  await mockViewer(page, { roles: ["tech_admin"] });
  await page.goto("/admin");
  await expect(page.getByRole("link", { name: "提供商" })).toBeVisible();
  await expect(page.getByRole("link", { name: "API Key" })).toBeVisible();
  await expect(page.getByRole("link", { name: "余额/充值" })).toHaveCount(0);
  await expect(page.getByRole("link", { name: "佣金策略" })).toHaveCount(0);
  await expect(page.getByRole("link", { name: "用户/项目" })).toHaveCount(0);
  await page.goto("/admin/providers");
  await expect(page.getByRole("heading", { name: "凭据轮换" })).toBeVisible();
  await expect(page.getByRole("button", { name: "轮换凭据" })).toBeVisible();
});

test("audit readonly sees logs but no write buttons", async ({ page }) => {
  await mockViewer(page, { roles: ["audit_readonly"] });
  await page.goto("/admin");
  await expect(page.getByRole("link", { name: "审计日志" })).toBeVisible();
  await expect(page.getByRole("link", { name: "余额/充值" })).toBeVisible();
  await expect(page.getByRole("link", { name: "用户/项目" })).toHaveCount(0);
  await expect(page.getByRole("link", { name: "系统设置" })).toHaveCount(0);
  await page.goto("/admin/audit");
  await expect(page.getByRole("heading", { name: "审计日志" })).toBeVisible();
  await expect(page.getByRole("button", { name: "写入探测" })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "读取 Outbox" })).toHaveCount(0);
  await page.goto("/admin/billing");
  await expect(page.getByRole("button", { name: "退消费账单" })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "赠送额度" })).toHaveCount(0);
});

test("ops admin can grant models but cannot refund", async ({ page }) => {
  await mockViewer(page, { roles: ["ops_admin"] });
  await page.goto("/admin");
  await expect(page.getByRole("link", { name: "模型" })).toBeVisible();
  await expect(page.getByRole("link", { name: "渠道租户" })).toBeVisible();
  await expect(page.getByRole("link", { name: "用户/项目" })).toHaveCount(0);
  await expect(page.getByRole("link", { name: "审计日志" })).toHaveCount(0);
  await page.goto("/admin/billing");
  await expect(page.getByRole("button", { name: "赠送额度" })).toBeVisible();
  await expect(page.getByRole("button", { name: "退消费账单" })).toHaveCount(0);
  await page.goto("/admin/channels/chn_reseller_b");
  await expect(page.getByRole("button", { name: "从平台目录授权" })).toBeVisible();
  await expect(page.getByRole("button", { name: "编辑" })).toHaveCount(0);
});
