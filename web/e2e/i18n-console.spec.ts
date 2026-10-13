import { expect, test, type Page } from "@playwright/test";
import { mockViewer } from "./mock-viewer";
import zh from "../messages/zh.json";
import en from "../messages/en.json";
import ja from "../messages/ja.json";

function phrases(value: object): string[] {
  return Object.values(value).flatMap(item => typeof item === "string" ? [item] : phrases(item));
}
const chineseProse = phrases(zh).filter(text => (text.match(/[\u4e00-\u9fff]/g)?.length ?? 0) >= 4 && !text.includes("{"));
const adminPages = ["/admin", "/admin/providers", "/admin/providers/new", "/admin/providers/provider-fixture", "/admin/models", "/admin/models/new", "/admin/models/test/model", "/admin/routes", "/admin/routes/new", "/admin/routes/route-fixture", "/admin/plans", "/admin/channels", "/admin/partners", "/admin/promos", "/admin/billing", "/admin/payments", "/admin/prices", "/admin/commission", "/admin/usage", "/admin/reconciliation", "/admin/media", "/admin/alerts", "/admin/runbooks", "/admin/metrics", "/admin/margin", "/admin/audit", "/admin/settings", "/admin/staff", "/admin/notifications", "/app/settings"];
const channelPages = ["/channel", "/channel/users", "/channel/subchannels", "/channel/models", "/channel/plans", "/channel/promos", "/channel/payments", "/channel/payments/rules", "/channel/commission", "/channel/commissions", "/channel/ledger", "/channel/usage", "/channel/reconciliation", "/channel/media", "/channel/metrics", "/channel/margin", "/channel/brand", "/channel/staff", "/channel/settings", "/channel/audit", "/channel/notifications"];
const userPages = ["/app/keys", "/app/catalog", "/app/playground", "/app/media", "/app/docs", "/app/wallet", "/app/plans", "/app/referral", "/app/usage", "/app/reconciliation", "/app/settings", "/app/notifications"];

async function data(page: Page) {
  await page.route("**/api/**", route => route.fulfill({ status: 503, json: { error: { code: "service_unavailable", message: "Synthetic business service unavailable" } } }));
  await page.route("**/api/admin/ops/dashboard", route => route.fulfill({ json: { dashboard: { totals: {}, alerts: [], dimensions: {}, module_errors: {} } } }));
  await page.route("**/api/admin/metrics/series?*", route => route.fulfill({ json: { items: [] } }));
  await page.route("**/api/admin/providers/provider-fixture", route => route.fulfill({ json: { item: { id: "provider-fixture", name: "Fixture provider", slug: "provider-fixture", kind: "direct", adapter: "openai", status: "active", health: "available", models: [] } } }));
  await page.route("**/api/admin/providers/provider-fixture/accounts", route => route.fulfill({ json: { items: [] } }));
  await page.route("**/api/admin/providers/provider-fixture/models", route => route.fulfill({ json: { items: [] } }));
  await page.route("**/api/admin/models/test/model", route => route.fulfill({ json: { item: { public_id: "test/model", display_name: "Fixture model", vendor: "alibaba", kind: "text", status: "draft", capabilities: {} } } }));
  await page.route("**/api/admin/routes/route-fixture", route => route.fulfill({ json: { item: { id: "route-fixture", public_model_id: "test/model", status: "inactive", strategy: "priority", candidates: [] } } }));
  await page.route("**/api/admin/plans?*", route => route.fulfill({ json: { items: [] } }));
  await page.route("**/api/admin/plans/eligible-channels", route => route.fulfill({ json: { items: [] } }));
  await page.route("**/api/admin/ops/alerts?*", route => route.fulfill({ json: { items: [{ id: "historical-alert", kind: "backup_drill_missing", severity: "medium", status: "open", message: "尚未记录备份恢复演练" }], next_cursor: "" } }));
  await page.route("**/api/admin/ops/runbooks", route => route.fulfill({ json: { items: [{ id: "rb_backup", alert_kind: "backup_drill_missing", title: "备份演练", body: "1. 运行 scripts/backup_drill.sh" }] } }));
  await page.route("**/api/v1/me/notifications/unread-count", route => route.fulfill({ json: { unread: 1 } }));
  await page.route("**/api/v1/me/notifications?*", route => route.fulfill({ json: { items: [{ id: "historical-notice", kind: "alert_opened", title: "平台运营告警", body: "平台出现新的运营告警，请查看告警详情并处理。\n尚未记录备份恢复演练", href: "/admin/alerts", created_at: "2026-10-12T08:00:00Z", read_at: null, email_status: "blocked_smtp" }], unread: 1, next_cursor: "" } }));
  await page.route("**/api/v1/me/notification-preferences", route => route.fulfill({ json: { email_enabled: true, smtp_available: false } }));
  await page.route("**/api/admin/notifications/deliveries?*", route => route.fulfill({ json: { items: [{ id: "historical-delivery", kind: "renewal_failed", title: "套餐续费失败", brand_id: "brand-fixture", status: "failed", attempts: 1, last_error: "smtp_delivery_failed", created_at: "2026-10-12T08:00:00Z" }], pending: 0, blocked: 0, failed: 1, accepted: 0 } }));
}

for (const locale of ["en", "ja"] as const) {
  test(`${locale} translates console pages, options, dialogs and historical system content`, async ({ browser }) => {
    test.setTimeout(150_000);
    const context = await browser.newContext({ locale: "zh-CN", extraHTTPHeaders: { "Accept-Language": "zh-CN,zh;q=0.9" } });
    const page = await context.newPage();
    const errors: string[] = [];
    page.on("pageerror", error => errors.push(error.message));
    page.on("console", message => { if (/MISSING_MESSAGE|INVALID_MESSAGE|FORMATTING_ERROR/.test(message.text())) errors.push(message.text()); });
    await data(page);
    await mockViewer(page, { roles: ["platform_admin"] });
    await page.goto("/admin/providers/new");
    await expect(page.getByRole("heading", { name: "新建提供商", exact: true })).toBeVisible();
    await page.getByRole("button", { name: "语言", exact: true }).click();
    await page.getByRole("menuitemradio", { name: locale === "en" ? "English" : "日本語", exact: true }).click();
    await expect(page.locator("html")).toHaveAttribute("lang", locale);
    await expect(page).toHaveURL(/\/admin\/providers\/new$/);
    const messages = locale === "en" ? en : ja;
    await expect(page.getByRole("heading", { name: messages.consoleCopy.newProvider, exact: true })).toBeVisible();

    async function check(path: string) {
      await page.goto(path);
      await expect(page.locator("html")).toHaveAttribute("lang", locale);
      await expect(page.getByTestId("console-access")).toHaveCount(0);
      const main = page.getByRole("main");
      await expect(main).toBeVisible();
      if (path.endsWith("/notifications")) await expect(main.getByRole("heading").filter({ hasText: messages.noticeEvents.alert_opened.title })).toBeVisible();
      if (path === "/admin/runbooks") await expect(main.getByRole("heading", { name: messages.systemRunbooks.rb_backup.title, exact: true })).toBeVisible();
      if (path === "/admin/alerts") await expect(main.getByRole("cell", { name: messages.systemAlerts.backup, exact: true })).toBeVisible();
      const text = (await page.locator("body").innerText()).replaceAll("中文", "").replaceAll("日本語", "");
      if (locale === "en") expect(text, path).not.toMatch(/[\u4e00-\u9fff]/);
      else expect(chineseProse.filter(phrase => text.includes(phrase)), path).toEqual([]);
      expect(errors, path).toEqual([]);
    }

    for (const path of adminPages) await check(path);
    await page.goto("/admin/plans");
    await page.getByRole("button", { name: messages.adminUi.createPlan, exact: true }).click();
    const dialog = page.getByRole("dialog");
    await expect(dialog).toBeVisible();
    const dialogText = await dialog.innerText();
    if (locale === "en") expect(dialogText).not.toMatch(/[\u4e00-\u9fff]/);
    else expect(chineseProse.filter(phrase => dialogText.includes(phrase))).toEqual([]);
    await page.screenshot({ path: `test-results/i18n-${locale}-plan-dialog.png`, fullPage: true });
    await mockViewer(page, { roles: ["channel_admin"], channelType: "C" });
    for (const path of channelPages) await check(path);
    await mockViewer(page, { roles: ["end_user"], partner: true });
    for (const path of userPages) await check(path);
    await page.goto("/app/notifications");
    await expect(page.getByText(messages.systemAlerts.backup, { exact: false })).toBeVisible();
    await page.screenshot({ path: `test-results/i18n-${locale}-historical-notice.png`, fullPage: true });
    await context.close();
  });
}
