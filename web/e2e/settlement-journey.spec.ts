import { expect, test } from "@playwright/test";
import { mockViewer } from "./mock-viewer";

const item = { id: "cst_alice", status: "settled", amount_minor: 1250000, period_start: "2026-09-01", recipient: { email: "alice@example.test", display_name: "Alice" }, channel_code: "CHANNEL-A" };
test("finance verifies recipient and receipt, retries uncertain payment registration without changing the operation", async ({ page }) => {
  await mockViewer(page, { roles: ["finance_admin"] });
  let paid = false;
  await page.route("**/admin/settlements", r => r.fulfill({ json: { items: [{ ...item, status: paid ? "paid" : "settled", payout_reference: paid ? "WIRE-123" : "" }] } }));
  let sends = 0;
  await page.route("**/admin/settlements/cst_alice/payout", async r => {
    expect(r.request().postDataJSON()).toEqual({ method: "manual", reference: "WIRE-123" });
    expect(r.request().headers()["x-tokenhub-confirm"]).toBe("1");
    if (++sends === 1) await r.abort(); else { paid = true; await r.fulfill({ json: { item: { ...item, status: "paid" } } }); }
  });
  await page.goto("/admin/commission");
  const panel = page.getByRole("region", { name: "佣金结算与打款登记" });
  await expect(panel.getByLabel("忽略最低结算金额")).not.toBeChecked();
  await panel.getByRole("button", { name: "登记此单打款" }).click();
  await expect(panel.getByRole("button", { name: "核对并登记" })).toBeDisabled();
  await panel.getByLabel("线下打款凭证").fill(" WIRE-123 ");
  await panel.getByRole("button", { name: "核对并登记" }).click();
  const dialog = page.getByRole("dialog");
  await expect(dialog).toContainText("alice@example.test");
  await expect(dialog).toContainText("$1.25 USD");
  await expect(dialog).toContainText("CHANNEL-A");
  await expect(dialog).toContainText("登记时同步扣减佣金钱包");
  await dialog.getByRole("button", { name: "确认", exact: true }).click();
  await expect(dialog.getByRole("alert")).toContainText("同一结算单及凭证不会重复登记");
  await dialog.getByRole("button", { name: "确认", exact: true }).click();
  await expect(dialog).toHaveCount(0);
  await expect(panel).toContainText("打款凭证：WIRE-123");
  await expect(panel.getByRole("button", { name: "登记此单打款" })).toHaveCount(0);
  expect(sends).toBe(2);
});

test("due-only unfreeze and monthly generation preserve the minimum by default", async ({ page }) => {
  await mockViewer(page, { roles: ["finance_admin"] });
  await page.route("**/admin/settlements", r => r.fulfill({ json: { items: [] } }));
  let unfreezes = 0, batches = 0;
  await page.route("**/admin/commissions/unfreeze", r => { expect(r.request().postDataJSON()).toEqual({}); unfreezes++; return r.fulfill({ json: { unfrozen: 0 } }); });
  await page.route("**/admin/commissions/settle*", r => { expect(new URL(r.request().url()).search).toBe(""); batches++; return r.fulfill({ json: { items: [] } }); });
  await page.goto("/admin/commission");
  await page.getByRole("button", { name: "解冻到期佣金", exact: true }).click();
  await page.getByRole("dialog").getByRole("button", { name: "确认", exact: true }).click();
  await expect(page.getByText("当前没有已到期且可解冻的佣金，未提前解冻任何记录。")).toBeVisible();
  await page.getByRole("button", { name: "生成结算单", exact: true }).click();
  await expect(page.getByRole("dialog")).toContainText("遵守最低结算金额");
  await page.getByRole("dialog").getByRole("button", { name: "确认", exact: true }).click();
  await expect(page.getByText("没有符合本次条件的可结算佣金，未生成空结算单。")).toBeVisible();
  expect(unfreezes).toBe(1); expect(batches).toBe(1);
});

test("audit sees cancellation and paid reversal evidence without mutation controls", async ({ page }) => {
  await mockViewer(page, { roles: ["audit_readonly"] });
  await page.route("**/admin/settlements", r => r.fulfill({ json: { items: [{ ...item, status: "cancelled" }, { ...item, id: "cst_paid", status: "paid", payout_reference: "WIRE-789", reversed_minor: 300000 }] } }));
  await page.goto("/admin/commission");
  const panel = page.getByRole("region", { name: "佣金结算与打款登记" });
  await expect(panel).toContainText("未冲正的佣金已恢复可结算");
  await expect(panel).toContainText("打款后佣金冲正 $0.30 USD");
  await expect(panel).toContainText("WIRE-789");
  await expect(panel.getByRole("button", { name: "生成结算单" })).toHaveCount(0);
  await expect(panel.getByRole("button", { name: "登记此单打款" })).toHaveCount(0);
});

test("wallet separates paid commission recovery from spendable balances", async ({ page }) => {
  await mockViewer(page, { roles: ["end_user"] });
  await page.route("**/v1/me/balance**", r => r.fulfill({ json: { balance: { available: "5", reserved: "0", purchased_minor: 5000000, gift_minor: 0, commission_available_minor: 0, commission_recovery_minor: 300000 } } }));
  await page.route("**/v1/me/ledger**", r => r.fulfill({ json: { items: [{ id: "paid", event_type: "commission_payout", amount_minor: -300000, created_at: "2026-09-18T00:00:00Z" }] } }));
  await page.goto("/app/wallet");
  await expect(page.getByLabel("余额组成").getByText("$5.00")).toBeVisible();
  await expect(page.getByRole("status").filter({ hasText: "待财务核对追回" })).toContainText("$0.30 USD");
  await expect(page.getByRole("status").filter({ hasText: "待财务核对追回" })).toContainText("未再次扣减佣金钱包");
  await expect(page.getByRole("cell", { name: "佣金打款出账", exact: true })).toBeVisible();
});
