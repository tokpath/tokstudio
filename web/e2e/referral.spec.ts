import { expect, test } from "@playwright/test";
import { mockViewer } from "./mock-viewer";

const referral = {
  codes: ["THU_MY_INVITE"], can_create: false, invited_count: 3, can_commission: false,
  rules: { spend_minor: 10_000_000, topup_minor: 20_000_000, gift_minor: 1_000_000 }, rewards: [],
};

for (const query of ["promo", "promotion_code"]) {
  test(`invitation ${query} opens registration and preserves editable code`, async ({ page }) => {
    await page.goto(`/login?${query}=THU_MY_INVITE`);
    await expect(page.getByLabel("推广码")).toHaveValue("THU_MY_INVITE");
    await page.getByLabel("推广码").fill("THU_CHANGED");
    await page.getByRole("button", { name: "登录", exact: true }).click();
    await page.getByRole("button", { name: "注册", exact: true }).click();
    await expect(page.getByLabel("推广码")).toHaveValue("THU_CHANGED");
    await page.route("**/v1/auth/register", async route => {
      expect(route.request().postDataJSON().promotion_code).toBe("THU_CHANGED");
      await route.fulfill({ status: 400, contentType: "application/json", body: JSON.stringify({ error: { message: "验收：请求已收到" } }) });
    });
    await page.getByLabel("邮箱", { exact: true }).fill("invite@example.test");
    await page.getByLabel("密码", { exact: true }).fill("password1");
    await page.getByRole("button", { name: "注册", exact: true }).click();
    await expect(page.getByRole("alert").filter({ hasText: "验收：请求已收到" })).toBeVisible();
    await expect(page.getByLabel("推广码")).toHaveValue("THU_CHANGED");
  });
}

test("personal referral shows a usable link and the actual reward recipient", async ({ page }) => {
  await mockViewer(page, { roles: ["end_user"] });
  await page.route("**/v1/me/referral", route => route.fulfill({ json: { item: referral } }));
  await page.goto("/app/referral");
  await expect(page.getByLabel("推广链接", { exact: true })).toHaveValue(/\/login\?promotion_code=THU_MY_INVITE$/);
  await expect(page.getByText("已直接邀请 3 人注册")).toBeVisible();
  await expect(page.getByText(/积分发给受邀新用户/)).toBeVisible();
  await expect(page.getByText("单笔充值达到 $20.00")).toBeVisible();
  await expect(page.getByText("暂无佣金记录")).toBeVisible();
  await page.getByRole("button", { name: "复制推广链接" }).click();
  await expect(page.getByRole("status")).toContainText(/已复制|复制失败/);
});

test("referral load failure does not look like zero invitations and can be retried", async ({ page }) => {
  await mockViewer(page, { roles: ["end_user"] });
  let failed = true;
  await page.route("**/v1/me/referral", route => route.fulfill(failed
    ? { status: 503, json: { error: { message: "推广服务暂不可用" } } }
    : { json: { item: referral } }));
  await page.goto("/app/referral");
  await expect(page.getByText("推广服务暂不可用")).toBeVisible();
  await expect(page.getByLabel("推广链接", { exact: true })).toHaveCount(0);
  await expect(page.getByText("已直接邀请 0 人注册")).toHaveCount(0);
  failed = false;
  await page.getByRole("button", { name: "重试", exact: true }).click();
  await expect(page.getByText("已直接邀请 3 人注册")).toBeVisible();
});

test("a personal promoter keeps the user workspace as home", async ({ page }) => {
  await mockViewer(page, { roles: ["end_user"], partner: true });
  await page.route("**/v1/partner/me", route => route.fulfill({ json: { role_type: "promoter" } }));
  await page.goto("/enter");
  await expect(page).toHaveURL(/\/app$/);
  await expect(page.getByRole("navigation", { name: "用户控制台" })).toBeVisible();
});

test("registration supports Enter and preserves the invitation after a network failure", async ({ page }) => {
  await page.goto("/login?promo=THU_RETRY");
  await page.getByLabel("邮箱", { exact: true }).fill("retry@example.test");
  await page.getByLabel("密码", { exact: true }).fill("password1");
  let requests = 0;
  await page.route("**/v1/auth/register", async route => { requests++; await route.abort(); });
  await page.getByLabel("密码", { exact: true }).press("Enter");
  await expect(page.getByRole("alert").filter({ hasText: "连接失败" })).toBeVisible();
  expect(requests).toBe(1);
  await expect(page.getByLabel("推广码")).toHaveValue("THU_RETRY");
  await expect(page.getByLabel("邮箱", { exact: true })).toHaveValue("retry@example.test");
  await expect(page.getByRole("button", { name: "注册", exact: true })).toBeEnabled();
});

test("invitee wallet identifies signup credits and separates commission from API credit", async ({ page }) => {
  await mockViewer(page, { roles: ["end_user"] });
  await page.route("**/v1/me/balance**", route => route.fulfill({ json: { balance: { available: "1", reserved: "0", gift_minor: 1_000_000, purchased_minor: 0, commission_available_minor: 2_000_000 } } }));
  await page.route("**/v1/me/ledger**", route => route.fulfill({ json: { items: [{ id: "led_internal", event_type: "gift_credit", amount_minor: 1_000_000, created_at: "2026-09-18T00:00:00Z" }] } }));
  await page.goto("/app/wallet");
  const buckets = page.getByLabel("余额组成");
  await expect(buckets.getByText("赠送积分（USD）")).toBeVisible();
  await expect(buckets.getByText("$1.00")).toBeVisible();
  await expect(buckets.getByText("$2.00")).toBeVisible();
  await expect(page.getByRole("cell", { name: "赠送积分入账" })).toBeVisible();
  await expect(page.getByText("led_internal")).toHaveCount(0);
});
