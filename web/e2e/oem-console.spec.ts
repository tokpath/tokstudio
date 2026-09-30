import { expect, test } from "@playwright/test";
import { mockViewer } from "./mock-viewer";

test("OEM sidebar is the platform sidebar with catalogue and OEM brands removed", async ({ page }) => {
  await page.route("**/api/**", route => route.fulfill({ json: { items: [], quota: {}, totals: { requests: 0, revenue_minor: 0, cost_minor: 0, margin_minor: 0, pending: 0 }, item: { status: "disabled" }, brand: { name: "OEM" } } }));
  await mockViewer(page, { roles: ["channel_admin"], channelType: "C" });
  await page.goto("/channel");
  const sidebar = page.getByRole("navigation", { name: "OEM 管理控制台", exact: true });
  await expect(sidebar.getByRole("link")).toHaveText(["总览", "套餐审核", "支付", "余额/充值", "用量/账单", "成本/毛利", "渠道租户", "推广码", "佣金策略", "指标", "对账", "媒体任务", "用户/项目", "员工与权限", "告警", "应急手册", "审计日志", "系统设置"]);
  await expect(sidebar.getByText("目录与网关")).toHaveCount(0);
  await expect(sidebar.getByRole("link", { name: "OEM 品牌" })).toHaveCount(0);
  for (const [label, href] of [["成本/毛利", "/channel/margin"], ["指标", "/channel/metrics"], ["媒体任务", "/channel/media"], ["告警", "/channel/alerts"], ["应急手册", "/channel/runbooks"], ["审计日志", "/channel/audit"], ["系统设置", "/channel/settings"], ["渠道租户", "/channel/subchannels"]]) {
    await sidebar.getByRole("link", { name: label, exact: true }).click();
    await expect(page).toHaveURL(new RegExp(`${href}$`));
    await expect(page.getByRole("heading", { name: label, exact: true }).first()).toBeVisible();
    await expect(page.getByText("Application error")).toHaveCount(0);
  }
  await page.getByRole("link", { name: "本品牌模型授权" }).click();
  await expect(sidebar.getByRole("link", { name: "渠道租户" })).toHaveClass(/bg-brand-soft/);
  await page.goto("/channel/keys");
  await expect(sidebar.getByRole("link", { name: "用户/项目" })).toHaveClass(/bg-brand-soft/);
});
