import { expect, test } from "@playwright/test";
import { mockViewer } from "./mock-viewer";

const preview = { item: { request_id: "req_alice", user_id: "usr_alice", model_id: "echo", state: "committed", amount_minor: 4000000, wallet_minor: 2500000, entitlement_minor: 1000000, gift_minor: 500000 }, user: { email: "alice@example.test", display_name: "Alice" } };

test("finance verifies refund amounts and retries a lost response in the same dialog", async ({ page }) => {
  await mockViewer(page, { roles: ["finance_admin"] });
  await page.route("**/admin/billing/report", r => r.fulfill({ json: { report: {} } }));
  await page.route("**/admin/refunds/preview?**", r => r.fulfill({ json: preview }));
  let sends = 0;
  await page.route("**/admin/refunds", async r => {
    expect(r.request().postDataJSON()).toEqual({ request_id: "req_alice" });
    expect(r.request().headers()["x-tokenhub-confirm"]).toBe("1");
    if (++sends === 1) await r.abort(); else await r.fulfill({ json: { item: { state: "reversed" } } });
  });
  await page.goto("/admin/billing");
  const panel = page.getByRole("region", { name: "消费退款", exact: true });
  await expect(panel.getByRole("button", { name: "查询退款账单" })).toBeDisabled();
  await panel.getByLabel("消费请求编号").fill("req_alice");
  await panel.getByRole("button", { name: "查询退款账单" }).click();
  await expect(panel).toContainText("Alice · alice@example.test");
  await panel.getByRole("button", { name: "核对并退消费账单" }).click();
  const dialog = page.getByRole("dialog");
  await expect(dialog).toContainText("退回充值积分 $2.50 USD");
  await expect(dialog).toContainText("钱包赠送积分 $0.50 USD 不退回");
  await dialog.getByRole("button", { name: "确认退款" }).click();
  await expect(dialog.getByRole("alert")).toContainText("不会重复退回");
  await dialog.getByRole("button", { name: "确认退款" }).click();
  await expect(dialog).toHaveCount(0);
  await expect(panel).toContainText("该账单已退款，无需再次操作");
  await expect(panel).toContainText("相关佣金已同步冲正");
  expect(sends).toBe(2);
  await panel.getByLabel("消费请求编号").fill("another-request");
  await expect(panel).not.toContainText("alice@example.test");
  await expect(panel.getByRole("button", { name: "核对并退消费账单" })).toHaveCount(0);
});

test("unknown request displays a useful error without exposing a refund action", async ({ page }) => {
  await mockViewer(page, { roles: ["finance_admin"] });
  await page.route("**/admin/billing/report", r => r.fulfill({ json: { report: {} } }));
  await page.route("**/admin/refunds/preview?**", r => r.fulfill({ status: 404, json: { error: { message: "未找到已入账的消费账单，请到用量/账单页核对请求编号。" } } }));
  await page.goto("/admin/billing");
  await page.getByLabel("消费请求编号").fill("unknown");
  await page.getByRole("button", { name: "查询退款账单" }).click();
  await expect(page.getByRole("alert").filter({ hasText: "未找到已入账" })).toBeVisible();
  await expect(page.getByRole("button", { name: "核对并退消费账单" })).toHaveCount(0);
});
