import { test, expect } from "@playwright/test";

test.beforeEach(async ({ page }) => {
  await page.route("**/api/**", route => route.fulfill({ status: 401, json: { error: { message: "未登录" } } }));
  await page.route("**/api/v1/me/notifications/unread-count", route => route.fulfill({ json: { unread: 1 } }));
});

test("read a no-grace renewal notice and change email preference on desktop and mobile", async ({ page }) => {
  let read = false; let enabled = true;
  await page.route("**/api/v1/me/notifications/unread-count", route => route.fulfill({ json: { unread: read ? 0 : 1 } }));
  await page.route("**/api/v1/me", route => route.fulfill({ json: { user: { id: "synthetic-notification-user", email: "notification@example.test", roles: ["end_user"], email_verified: true } } }));
  await page.route("**/api/v1/me/notification-preferences", route => { if (route.request().method() === "PUT") enabled = route.request().postDataJSON().email_enabled; return route.fulfill({ json: { email_enabled: enabled, smtp_available: false } }); });
  await page.route("**/api/v1/me/notifications?*", route => route.fulfill({ json: { items: [{ id: "synthetic-renewal", kind: "renewal_failed", title: "套餐续费失败", body: "本期套餐已到期，剩余额度失效，没有宽限期。有其他有效额度时仍可使用，全部额度不足时请求会被拒绝。", href: "/app/plans", created_at: "2026-10-12T08:00:00Z", read_at: read ? "2026-10-12T09:00:00Z" : null, email_status: "blocked_smtp" }], unread: read ? 0 : 1, next_cursor: "" } }));
  await page.route("**/api/v1/me/notifications/synthetic-renewal/read", route => { read = true; return route.fulfill({ json: { ok: true } }); });
  await page.goto("/app/notifications");
  await expect(page.getByRole("heading", { name: "通知中心", exact: true })).toBeVisible();
  await expect(page.getByText(/本期套餐已到期，剩余额度失效，没有宽限期/)).toBeVisible();
  await page.screenshot({ path: "test-results/notifications-desktop.png", fullPage: true });
  await page.getByRole("button", { name: "标为已读", exact: true }).click();
  await expect(page.getByRole("button", { name: "未读（0）" })).toBeVisible();
  await page.getByRole("checkbox", { name: "接收业务邮件通知" }).uncheck();
  await expect(page.getByRole("checkbox", { name: "接收业务邮件通知" })).not.toBeChecked();
  expect(enabled).toBe(false);
  await page.setViewportSize({ width: 390, height: 844 });
  await page.screenshot({ path: "test-results/notifications-mobile.png", fullPage: true });
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
});

test("technician saves shared SMTP, queues a test and inspects acceptance truthfully", async ({ page }) => {
  let settings = { enabled: false, available: false, host: "", port: 587, from: "", username: "", security: "starttls", password_set: false, source: "environment" };
  let queued = false; let accepted = false;
  await page.route("**/api/v1/me", route => route.fulfill({ json: { user: { id: "synthetic-technician", email: "technician@example.test", roles: ["tech_admin"], email_verified: true } } }));
  await page.route("**/api/admin/notifications/smtp", route => { if (route.request().method() === "PUT") {
    const body = route.request().postDataJSON(); expect(body.password).toBe("synthetic-smtp-password"); expect(route.request().headers()["x-tokenhub-confirm"]).toBe("1");
    settings = { ...settings, enabled: body.enabled, available: true, host: body.host, from: body.from, username: body.username, password_set: true, source: "database" };
  } return route.fulfill({ json: { item: settings } }); });
  await page.route("**/api/admin/notifications/smtp/test", route => { queued = true; return route.fulfill({ status: 202, json: { notification_id: "synthetic-test", status: "pending" } }); });
  await page.route("**/api/admin/notifications/deliveries?*", route => route.fulfill({ json: { items: queued ? [{ id: "synthetic-delivery", title: "邮件通知测试", brand_id: "brd_official", status: accepted ? "accepted" : "pending", attempts: accepted ? 1 : 0, last_error: "", created_at: "2026-10-12T08:00:00Z" }] : [], pending: queued && !accepted ? 1 : 0, blocked: 0, failed: 0, accepted: accepted ? 1 : 0 } }));
  await page.goto("/admin/settings#notifications");
  await page.getByRole("checkbox", { name: "启用平台邮件服务" }).check();
  await page.getByLabel("SMTP 服务器", { exact: true }).fill("smtp.example.test");
  await page.getByLabel("发件邮箱", { exact: true }).fill("notifications@example.test");
  await page.getByLabel("SMTP 用户名", { exact: true }).fill("synthetic-user");
  await page.getByLabel("SMTP 密码", { exact: true }).fill("synthetic-smtp-password");
  await page.getByRole("button", { name: "保存邮件配置", exact: true }).click();
  await page.getByRole("dialog").getByRole("button", { name: "确认", exact: true }).click();
  await expect(page.getByText("邮件配置已保存，可发送测试邮件检查。")).toBeVisible();
  await expect(page.getByLabel("SMTP 密码", { exact: true })).toHaveValue("");
  await page.getByRole("button", { name: "发送测试邮件到我的邮箱" }).click();
  await expect(page.getByText(/测试邮件已加入发送队列/)).toBeVisible();
  await expect(page.getByText("邮件通知测试", { exact: true })).toBeVisible();
  accepted = true; await page.getByRole("button", { name: "刷新", exact: true }).click();
  await expect(page.locator("td").getByText("邮件服务器已接收", { exact: true })).toBeVisible();
  await page.screenshot({ path: "test-results/notifications-smtp-admin.png", fullPage: true });
});

test("auditor reads delivery failures from alerts without SMTP configuration or retry access", async ({ page }) => {
  await page.route("**/api/v1/me", route => route.fulfill({ json: { user: { id: "synthetic-notification-auditor", roles: ["audit_readonly"] } } }));
  await page.route("**/api/admin/ops/alerts", route => route.fulfill({ json: { items: [] } }));
  await page.route("**/api/admin/notifications/deliveries?*", route => route.fulfill({ json: { items: [{ id: "synthetic-failed-delivery", title: "套餐续费失败", brand_id: "brd_official", status: "failed", attempts: 5, last_error: "smtp_delivery_failed", created_at: "2026-10-12T08:00:00Z" }], pending: 0, blocked: 0, failed: 1, accepted: 0 } }));
  await page.goto("/admin/alerts");
  await expect(page.getByRole("heading", { name: "邮件发送记录", exact: true })).toBeVisible();
  await expect(page.getByRole("cell", { name: /发送失败.*SMTP 发送未成功/ })).toBeVisible();
  await expect(page.getByRole("button", { name: "重新发送", exact: true })).toHaveCount(0);
  await expect(page.getByRole("heading", { name: "平台邮件通知", exact: true })).toHaveCount(0);
});
