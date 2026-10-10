import { expect, test, type Page } from "@playwright/test";
import { mockViewer } from "./mock-viewer";

const oemLinks = [
  "/channel", "/channel/delivery", "/channel/users", "/channel/subchannels", "/channel/promos",
  "/channel/models", "/channel/plans", "/channel/payments", "/channel/commission", "/channel/ledger",
  "/channel/alerts", "/channel/usage", "/channel/reconciliation", "/channel/media",
  "/channel/metrics", "/channel/margin", "/channel/brand", "/channel/staff", "/channel/settings", "/channel/audit",
];
async function viewer(page: Page, channelType: "B" | "C") {
  await page.route("**/api/**", route => route.fulfill({ status: 503, json: { error: { message: "本场景未加载此业务数据" } } }));
  await mockViewer(page, { roles: ["channel_admin"], channelType });
  await page.route("**/api/channel/me", route => route.fulfill({ json: { channel_org_id: channelType === "C" ? "chn_oem_c" : "chn_reseller_b", channel_type: channelType, channel_name: channelType === "C" ? "OEM C" : "Reseller B", brand_name: "OEM C" } }));
  await page.route("**/api/channel/delivery", route => route.fulfill({ json: { item: {
    delivery: { channel_org_id: "chn_oem_c", phase: "configuring", version: 1, sales_mode: "offline", sell_plans: false, handoff_evidence: {} },
    channel: { id: "chn_oem_c", code: "oem-c" }, brand: { id: "brd_c", name: "OEM C", primary_domain: "oem.example.test", api_domain: "api.oem.example.test", admin_domain: "admin.oem.example.test" },
    ready: false, checked_at: "2026-10-10T00:00:00Z", administrators: [], checks: [{ key: "request", ready: false, reason: "request_evidence_missing", responsibility: "customer", href: "" }],
  } } }));
  await page.route("**/api/channel/models", route => route.fulfill({ json: { items: [], total: 0, next_cursor: "" } }));
  await page.route("**/api/channel/subchannels?**", route => route.fulfill({ json: { items: [], total: 0, next_cursor: "" } }));
  await page.route("**/api/channel/api-keys", route => route.fulfill({ json: { items: [], total: 0, next_cursor: "" } }));
  await page.route("**/api/channel/commissions?**", route => route.fulfill({ json: { items: [], total: 0, next_cursor: "" } }));
  await page.route("**/v1/me/referral", route => route.fulfill({ json: { item: { invited_count: 2, summary: { frozen_minor: 100000, available_minor: 200000, paid_minor: 300000 } } } }));
}

test("OEM seven task groups expose their authorized routes and keep the current parent active", async ({ page }) => {
  await viewer(page, "C");
  await page.goto("/channel");
  const sidebar = page.getByRole("navigation", { name: "OEM 管理控制台", exact: true });
  await expect(sidebar.getByRole("button")).toHaveText(["总览", "客户与合作方", "模型与套餐", "资金与结算", "异常处理", "经营分析", "品牌与团队"]);
  await expect.poll(() => sidebar.locator("a[href]").evaluateAll(links => links.map(link => link.getAttribute("href")))).toEqual(oemLinks);
  for (const href of oemLinks.slice(2)) await expect(sidebar.locator(`a[href="${href}"]`)).not.toBeVisible();
  await expect(sidebar.getByRole("link")).toHaveText(["总览", "交付与营业"]);
  await expect(sidebar.locator('button[aria-expanded="true"]')).toHaveCount(1);
  for (const href of ["/admin/providers", "/admin/routes", "/channel/runbooks", "/admin/brands"]) await expect(sidebar.locator(`a[href="${href}"]`)).toHaveCount(0);
  for (const [group, label, href] of [
    ["总览", "交付与营业", "/channel/delivery"],
    ["客户与合作方", "渠道", "/channel/subchannels"],
    ["模型与套餐", "模型", "/channel/models"],
  ]) {
    const button = sidebar.getByRole("button", { name: group, exact: true });
    if (await button.getAttribute("aria-expanded") !== "true") {
      await expect(sidebar.locator(`a[href="${href}"]`)).not.toBeVisible();
      await button.click();
    }
    const link = sidebar.getByRole("link", { name: label, exact: true });
    await expect(link).toBeVisible();
    await expect(link).toHaveAttribute("href", href);
    await link.click();
    await expect(page).toHaveURL(new RegExp(`${href}$`));
    await expect(link).toHaveClass(/bg-brand-soft/);
    await expect(sidebar.locator('button[aria-expanded="true"]')).toHaveCount(1);
    await expect(page.getByText("Application error")).toHaveCount(0);
  }
  await page.goto("/channel/subchannels");
  await page.getByRole("main").getByRole("link", { name: "本品牌模型授权", exact: true }).click();
  await expect(page).toHaveURL(/\/channel\/models$/);
  await expect(sidebar.getByRole("link", { name: "模型", exact: true })).toHaveClass(/bg-brand-soft/);
  await page.goto("/channel/keys");
  await expect(sidebar.getByRole("link", { name: "客户", exact: true })).toHaveClass(/bg-brand-soft/);
});

test("reseller B has five primary items and no brand funds or platform technical routes", async ({ page }) => {
  await viewer(page, "B");
  await page.goto("/channel");
  const sidebar = page.getByRole("navigation", { name: "渠道控制台", exact: true });
  await expect(sidebar.getByRole("link")).toHaveText(["工作台", "本渠道用户", "推广", "本渠道模型", "我的收益"]);
  await expect.poll(() => sidebar.locator("a[href]").evaluateAll(links => links.map(link => link.getAttribute("href")))).toEqual(["/channel", "/channel/users", "/channel/promos", "/channel/models", "/channel/commissions"]);
  for (const href of ["/channel/payments", "/channel/ledger", "/channel/commission", "/channel/delivery", "/channel/staff", "/admin/providers", "/admin/routes"]) await expect(sidebar.locator(`a[href="${href}"]`)).toHaveCount(0);
  await expect(page.getByRole("region", { name: "经营任务", exact: true }).locator('a[href="/channel/payments"]')).toHaveCount(0);
  await sidebar.getByRole("link", { name: "我的收益", exact: true }).click();
  await expect(page).toHaveURL(/\/channel\/commissions$/);
  await expect(sidebar.getByRole("link", { name: "我的收益", exact: true })).toHaveClass(/bg-brand-soft/);
  await expect(page.getByRole("button", { name: "生成结算单", exact: true })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "登记此单打款", exact: true })).toHaveCount(0);
  await page.goto("/channel/ledger");
  await expect(page.getByTestId("console-access")).toBeVisible();
  await expect(page.getByRole("button", { name: "登记已收回款项", exact: true })).toHaveCount(0);
});
