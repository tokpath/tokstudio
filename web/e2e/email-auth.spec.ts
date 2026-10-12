import { test, expect } from "@playwright/test";

const token = `themail_${"b".repeat(48)}`;

test.beforeEach(async ({ page }) => {
  await page.route("**/api/**", route => route.fulfill({ status: 401, json: { error: { message: "未登录" } } }));
  await page.route("**/api/v1/auth/google/status", route => route.fulfill({ json: { available: false } }));
});

test("registration opens verification with a queued email and keeps the next destination", async ({ page }) => {
  let registered = false;
  await page.route("**/api/v1/me", route => route.fulfill({ status: registered ? 200 : 401, json: registered ? { user: { id: "synthetic-new-user", email: "new-member@example.test", email_verified: false, roles: ["end_user"] } } : { error: { message: "未登录" } } }));
  await page.route("**/api/v1/auth/register", route => { registered = true; return route.fulfill({ status: 201, json: { session: { user: { email: "new-member@example.test" } }, verification_email_queued: true } }); });
  await page.goto("/login?next=%2Fapp%2Fkeys");
  await page.getByRole("button", { name: "注册", exact: true }).click();
  await page.getByLabel("邮箱", { exact: true }).fill("new-member@example.test");
  await page.getByLabel("密码", { exact: true }).fill("new-member-password");
  await page.getByRole("button", { name: "注册", exact: true }).click();
  await expect(page).toHaveURL(/\/verify-email\?sent=1&next=%2Fapp%2Fkeys/);
  await expect(page.getByText(/验证邮件已加入发送队列/)).toBeVisible();
  await expect(page.getByRole("button", { name: /秒后可重发/ })).toBeDisabled();
  await expect(page.getByRole("link", { name: "继续进入控制台" })).toHaveAttribute("href", "/app/keys");
});

test("request a reset from login, show generic confirmation, and prevent immediate resending", async ({ page }) => {
  await page.route("**/api/v1/auth/password/forgot", route => route.fulfill({ status: 202, json: { ok: true, retry_after: 60 } }));
  await page.goto("/login");
  await page.getByRole("link", { name: "忘记密码？" }).click();
  await expect(page.getByRole("heading", { name: "找回密码", exact: true })).toBeVisible();
  await page.getByLabel("邮箱", { exact: true }).fill("unknown@example.test");
  await page.screenshot({ path: "test-results/email-forgot-desktop.png", fullPage: true });
  await page.getByRole("button", { name: "发送邮件" }).click();
  await expect(page.getByRole("status")).toContainText("如果该邮箱在当前品牌存在可用账号");
  await expect(page.getByRole("button", { name: /秒后可重发/ })).toBeDisabled();
});

test("reset consumes the email fragment only on submit and returns to login", async ({ page }) => {
  let submitted = false;
  await page.route("**/api/v1/auth/password/reset", route => { expect(route.request().postDataJSON()).toEqual({ token, new_password: "safe-new-password" }); submitted = true; return route.fulfill({ json: { ok: true } }); });
  await page.goto(`/reset-password#token=${token}`);
  await page.reload();
  await page.getByLabel("新密码", { exact: true }).fill("safe-new-password");
  await page.getByLabel("确认新密码").fill("mismatch-password");
  expect(submitted).toBe(false); expect(page.url()).toContain(token);
  await page.getByRole("button", { name: "重置密码" }).click();
  await expect(page.getByRole("main").getByRole("alert")).toContainText("两次输入的密码不一致");
  await page.getByLabel("确认新密码").fill("safe-new-password");
  await page.setViewportSize({ width: 390, height: 844 });
  await page.screenshot({ path: "test-results/email-reset-mobile.png", fullPage: true });
  await page.getByRole("button", { name: "重置密码" }).click();
  await expect(page.getByRole("status")).toContainText("密码已更新");
  expect(page.url()).not.toContain(token);
  await expect(page.getByRole("link", { name: "返回登录" })).toHaveAttribute("href", "/login");
});

test("email confirmation does not mutate on opening a link and an expired link can be resent", async ({ page }) => {
  let count = 0;
  await page.route("**/api/v1/auth/email/verify", route => { count++; return route.fulfill({ status: 400, json: { error: { code: "email_link_invalid" } } }); });
  await page.route("**/api/v1/auth/email/verification/request", route => route.fulfill({ status: 202, json: { ok: true, retry_after: 60 } }));
  await page.goto(`/verify-email#token=${token}`);
  await expect(page.getByRole("button", { name: "确认验证邮箱" })).toBeVisible(); expect(count).toBe(0);
  await page.screenshot({ path: "test-results/email-verification-desktop.png", fullPage: true });
  await page.getByRole("button", { name: "确认验证邮箱" }).click();
  await expect(page.getByRole("main").getByRole("alert")).toContainText("链接无效");
  await page.getByRole("button", { name: "重新申请邮件" }).click();
  await page.getByLabel("邮箱", { exact: true }).fill("member@example.test");
  await page.getByRole("button", { name: "发送邮件" }).click();
  await expect(page.getByRole("status")).toContainText("待验证");
});
