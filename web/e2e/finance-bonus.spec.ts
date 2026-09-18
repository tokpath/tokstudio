import { expect, test } from "@playwright/test";
import { mockViewer } from "./mock-viewer";

const user = { id: "usr_alice", email: "alice@example.test", display_name: "Alice", channel_code: "OFFICIAL", status: "active" };

test("finance searches, confirms a recipient, and safely resumes after a lost response", async ({ page }) => {
  await mockViewer(page, { roles: ["finance_admin"] });
  await page.route("**/admin/billing/report", r => r.fulfill({ json: { report: {} } }));
  await page.route("**/admin/billing/users?**", r => r.fulfill({ json: { items: [user] } }));
  let key = "";
  let sends = 0;
  await page.route("**/admin/entitlements/bonus", async r => {
    const request = r.request();
    expect(request.postDataJSON()).toMatchObject({ user_id: user.id, amount: 2500000, expires_in_seconds: 86400 });
    const incomingKey = request.headers()["idempotency-key"];
    expect(incomingKey).toBeTruthy();
    if (++sends === 1) { key = incomingKey; await r.abort(); }
    else { expect(incomingKey).toBe(key); await r.fulfill({ json: { item: { id: "ent_one" } } }); }
  });
  await page.goto("/admin/billing");
  await page.getByLabel("收款用户", { exact: true }).fill("alice");
  await expect(page.getByRole("button", { name: "赠送额度", exact: true })).toBeDisabled();
  await page.getByRole("button", { name: "搜索用户" }).click();
  await page.getByRole("button", { name: /Alice · alice@example.test/ }).click();
  await page.getByLabel("赠送金额（USD）").fill("2.5");
  await page.getByRole("button", { name: "赠送额度", exact: true }).click();
  await expect(page.getByRole("dialog")).toContainText("alice@example.test，渠道 OFFICIAL");
  await expect(page.getByRole("dialog")).toContainText("$2.50 USD，有效期 24 小时");
  await page.getByRole("button", { name: "确认", exact: true }).click();
  await expect(page.getByRole("dialog")).toContainText("不会重复发放");
  await page.reload();
  await expect(page.getByLabel("收款用户", { exact: true })).toBeDisabled();
  await page.getByRole("button", { name: "核对原操作" }).click();
  await page.getByRole("button", { name: "确认", exact: true }).click();
  await expect(page.getByRole("status").filter({ hasText: "已向 alice@example.test 赠送 $2.50" })).toBeVisible();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  expect(sends).toBe(2);
});

test("recipient sees bonus amount and expiry even when plan requests fail", async ({ page }) => {
  await mockViewer(page, { roles: ["end_user"] });
  await page.route("**/v1/me/plans", r => r.abort());
  await page.route("**/v1/me/entitlements", r => r.fulfill({ json: { items: [
    { id: "ent_one", source_type: "bonus", unit_type: "usd_credit", granted: 2500000, remaining: 2500000, status: "active", expires_at: "2099-09-19T08:00:00Z" },
  ] } }));
  await page.goto("/app/plans");
  const credits = page.getByRole("region", { name: "我的额度与有效期" });
  await expect(credits).toContainText("限时赠送额度 · 可使用");
  await expect(credits).toContainText("当前可用：$2.50 USD");
  await expect(credits).toContainText("2099");
  await expect(credits).toContainText("到期时间：");
});
