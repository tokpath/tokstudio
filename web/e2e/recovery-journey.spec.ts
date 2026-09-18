import { expect, test } from "@playwright/test";
import { mockViewer } from "./mock-viewer";

const recovery = { id: "claim_alice", user_id: "alice", settlement_id: "settlement_alice", amount_minor: 300000, recovered_minor: 0, status: "pending", recipient: { email: "alice@example.test", display_name: "Alice" }, receipts: [] };
test("finance registers partial receipt and retries the exact operation after an uncertain response", async ({ page }) => {
  await mockViewer(page, { roles: ["finance_admin"] });
  let received = false;
  await page.route("**/admin/commission-recoveries", r => r.fulfill({ json: { items: [{ ...recovery, recovered_minor: received ? 100000 : 0, receipts: received ? [{ id: "receipt1", amount_minor: 100000, reference: "RETURN-123", actor_user_id: "finance", actor_email: "finance@example.test", note: "bank confirmed", created_at: "2026-09-18T00:00:00Z" }] : [] }] } }));
  await page.route("**/admin/settlements", r => r.fulfill({ json: { items: [] } }));
  const payloads: unknown[] = [];
  await page.route("**/admin/commission-recoveries/claim_alice/receipts", async r => {
    const body = r.request().postDataJSON(); payloads.push(body);
    expect(body).toMatchObject({ amount_minor: 100000, reference: "RETURN-123", note: "bank confirmed" });
    expect(body.idempotency_key).toBeTruthy();
    expect(r.request().headers()["x-tokenhub-confirm"]).toBe("1");
    received = true;
    if (payloads.length === 1) await r.abort(); else await r.fulfill({ json: { item: { id: "receipt1" } } });
  });
  await page.goto("/admin/commission");
  const panel = page.getByRole("region", { name: "佣金追回与收款登记" });
  await panel.getByRole("button", { name: "登记已收回款项" }).click();
  await panel.getByLabel("本次实际收回金额（USD）").fill("0.4");
  await expect(panel).toContainText("不超过 0.3 USD");
  await expect(panel.getByRole("button", { name: "核对收回款项" })).toBeDisabled();
  await panel.getByLabel("本次实际收回金额（USD）").fill("0.1");
  await expect(panel.getByRole("button", { name: "核对收回款项" })).toBeDisabled();
  await panel.getByLabel("实际收款凭证", { exact: true }).fill(" RETURN-123 ");
  await panel.getByLabel("内部备注（可选）").fill("bank confirmed");
  await panel.getByRole("button", { name: "核对收回款项" }).click();
  const dialog = page.getByRole("dialog");
  await expect(dialog).toContainText("alice@example.test");
  await expect(dialog).toContainText("预计剩余 0.2 USD");
  await expect(dialog).toContainText("不改变任何钱包余额");
  await dialog.getByRole("button", { name: "确认", exact: true }).click();
  await expect(dialog.getByRole("alert")).toContainText("按原操作重试");
  await dialog.getByRole("button", { name: "确认", exact: true }).click();
  await expect(dialog).toHaveCount(0);
  await expect(panel).toContainText("已收回 0.1 USD · 剩余 0.2 USD");
  await expect(panel).toContainText("凭证：RETURN-123");
  await expect(panel).toContainText("登记人：finance@example.test");
  expect(payloads).toHaveLength(2); expect(payloads[0]).toEqual(payloads[1]);
});

test("audit sees completed receipts without write controls or stale recovery warning", async ({ page }) => {
  await mockViewer(page, { roles: ["audit_readonly"] });
  await page.route("**/admin/commission-recoveries", r => r.fulfill({ json: { items: [{ ...recovery, status: "closed", recovered_minor: 300000 }] } }));
  await page.route("**/admin/settlements", r => r.fulfill({ json: { items: [{ id: "settlement_alice", amount_minor: 300000, status: "paid", reversed_minor: 300000, recovery_tracked: true, recovered_minor: 300000, recovery_pending_minor: 0 }] } }));
  await page.goto("/admin/commission");
  const panel = page.getByRole("region", { name: "佣金追回与收款登记" });
  await expect(panel).toContainText("已结清");
  await expect(panel.getByRole("button", { name: "登记已收回款项" })).toHaveCount(0);
  await expect(page.getByRole("region", { name: "佣金结算与打款登记" })).toContainText("已全额收回，原打款与冲正记录保留");
});

test("receipt conflict leaves review open and does not claim success", async ({ page }) => {
  await mockViewer(page, { roles: ["finance_admin"] });
  await page.route("**/admin/commission-recoveries", r => r.fulfill({ json: { items: [recovery] } }));
  await page.route("**/admin/commission-recoveries/claim_alice/receipts", r => r.fulfill({ status: 409, json: { error: { message: "该凭证已登记，请刷新核对。" } } }));
  await page.goto("/admin/commission");
  const panel = page.getByRole("region", { name: "佣金追回与收款登记" });
  await panel.getByRole("button", { name: "登记已收回款项" }).click();
  await panel.getByLabel("实际收款凭证", { exact: true }).fill("EXISTING");
  await panel.getByRole("button", { name: "核对收回款项" }).click();
  const dialog = page.getByRole("dialog");
  await dialog.getByRole("button", { name: "确认", exact: true }).click();
  await expect(dialog.getByRole("alert")).toContainText("该凭证已登记");
  await expect(panel.getByRole("status")).toHaveCount(0);
});

test("user retains closed receipts without a pending recovery warning", async ({ page }) => {
  await mockViewer(page, { roles: ["end_user"] });
  await page.route("**/v1/me/balance**", r => r.fulfill({ json: { balance: { available: "5", reserved: "0", purchased_minor: 5000000, gift_minor: 0, commission_available_minor: 0, commission_recovery_minor: 0 } } }));
  await page.route("**/v1/me/commission-recoveries", r => r.fulfill({ json: { items: [{ ...recovery, status: "closed", recovered_minor: 300000, receipts: [{ id: "receipt", amount_minor: 300000, reference: "RETURN-CLOSED", created_at: "2026-09-18T00:00:00Z" }] }] } }));
  await page.goto("/app/wallet");
  await expect(page.getByLabel("余额组成").getByText("$5.00")).toBeVisible();
  const history = page.getByRole("region", { name: "佣金收回记录" });
  await expect(history).toContainText("已结清");
  await expect(history).toContainText("RETURN-CLOSED");
  await expect(history).toContainText("剩余 0 USD");
  await expect(page.getByRole("status").filter({ hasText: "待财务核对追回" })).toHaveCount(0);
});
