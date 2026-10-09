import { expect, test, type Page } from "@playwright/test";
import { mockViewer } from "./mock-viewer";

test.use({ timezoneId: "UTC" });

const order = {
  id: "pay_acceptance", user_id: "usr_alice", user_email: "alice@example.test", user_name: "Alice",
  channel_org_id: "channel_a", payee_channel_org_id: "brand_a", channel_code: "CHANNEL-A",
  adapter: "manual", purpose: "wallet", currency: "USD", amount_minor: 2500000, credit_minor: 2500000,
  created_at: "2026-09-18T00:00:00Z", status: "pending", fulfilled_at: "", received_at: "",
};
async function facts(page: Page, current: () => typeof order) {
  await page.route(/\/api\/admin\/payments(?:\?.*)?$/, route => route.fulfill({ json: { items: [current()], next_cursor: "" } }));
  await page.route("**/admin/payments/pay_acceptance", route => route.fulfill({ json: {
    item: { order: current(), allocations: [], events: [] },
    customer: { email: "alice@example.test", display_name: "Alice" },
    channel_codes: { channel_a: "CHANNEL-A", brand_a: "OEM Brand" },
  } }));
}

test("finance finds the original user order, verifies actual receipt, and previews its refund", async ({ page }) => {
  await mockViewer(page, { roles: ["finance_admin"] });
  let current = { ...order };
  await facts(page, () => current);
  let receipts = 0, refunds = 0, previews = 0;
  await page.route("**/admin/payments/pay_acceptance/confirm", async route => {
    receipts++;
    const body = route.request().postDataJSON();
    expect(body.occurred_at).toBe("2026-09-18T00:05:00.000Z");
    expect(route.request().headers()["x-tokenhub-confirm"]).toBe("1");
    current = { ...current, status: "paid", fulfilled_at: "2026-09-18T00:06:00Z", received_at: body.occurred_at };
    await route.fulfill({ json: { item: current } });
  });
  await page.route("**/admin/payments/pay_acceptance/refund-preview", async route => {
    previews++;
    expect(refunds).toBe(0);
    await route.fulfill({ json: { item: { order: current, can_refund: true, credit_reclaim_minor: 2500000, subscription: false } } });
  });
  await page.route("**/admin/payments/pay_acceptance/refund", async route => {
    refunds++;
    expect(previews).toBe(1);
    expect(route.request().postDataJSON().occurred_at).toBe("2026-09-18T00:10:00.000Z");
    expect(route.request().headers()["x-tokenhub-confirm"]).toBe("1");
    current = { ...current, status: "refunded" };
    await route.fulfill({ json: { item: current } });
  });
  await page.goto("/admin/payments");
  await page.getByRole("textbox", { name: "筛选", exact: true }).fill("alice@example.test");
  await page.getByRole("button", { name: "搜索", exact: true }).click();
  const row = page.getByRole("row").filter({ hasText: order.id });
  await expect(row).toContainText("alice@example.test");
  await expect(row).toContainText("$2.50 USD");
  await expect(row).toContainText("发放额度（USD）: $2.50 USD");
  await row.getByRole("link", { name: order.id, exact: true }).click();
  await expect(page.getByRole("heading", { name: "订单详情", exact: true })).toBeVisible();
  // Playwright's fixture context uses UTC; displayed business facts remain UTC+8.
  await page.getByLabel("实际收款时间", { exact: true }).fill("2026-09-18T00:05");
  await page.getByRole("button", { name: "确认收款", exact: true }).click();
  const receipt = page.getByRole("dialog");
  await expect(receipt).toContainText("alice@example.test");
  await expect(receipt).toContainText(order.id);
  await expect(receipt).toContainText("$2.50 USD");
  expect(receipts).toBe(0);
  await receipt.getByRole("button", { name: "确认", exact: true }).click();
  await expect(page.getByRole("status")).toContainText(`订单 ${order.id} 已更新`);
  await expect(page.getByText("已入账", { exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "确认收款", exact: true })).toHaveCount(0);
  await page.getByLabel("实际退款时间", { exact: true }).fill("2026-09-18T00:10");
  await page.getByRole("button", { name: "登记线下退款", exact: true }).click();
  const refund = page.getByRole("dialog");
  await expect(refund).toContainText("Alice · alice@example.test");
  await expect(refund).toContainText("OEM Brand");
  await expect(refund).toContainText("回收充值额度 $2.50 USD，实际退款 $2.50 USD");
  await expect(refund).toContainText("实际退款时间");
  await expect(refund).toContainText("款项已实际退还");
  expect(refunds).toBe(0);
  await refund.getByRole("button", { name: "确认", exact: true }).click();
  await expect(page.getByText("已退款", { exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "登记线下退款", exact: true })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "退款", exact: true })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "核对原退款", exact: true })).toHaveCount(0);
  const back = page.getByRole("link", { name: "返回订单", exact: true });
  await expect(back).toHaveAttribute("href", /\/admin\/payments\?_admin_payments_q=alice%40example.test/);
  await back.click();
  await expect(page.getByRole("row").filter({ hasText: order.id })).toContainText("已退款");
  expect(receipts).toBe(1);
  expect(refunds).toBe(1);
});

test("audit can inspect original orders and receipt facts without financial forms", async ({ page }) => {
  await mockViewer(page, { roles: ["audit_readonly"] });
  await facts(page, () => order);
  await page.route("**/admin/payments/pay_acceptance/**", () => { throw new Error("Audit must not request a financial operation"); });
  await page.goto("/admin/payments");
  const row = page.getByRole("row").filter({ hasText: order.id });
  await expect(row).toContainText("alice@example.test");
  await expect(row).toContainText("$2.50 USD");
  await expect(page.getByRole("button", { name: "线下收款划拨", exact: true })).toHaveCount(0);
  await row.getByRole("link", { name: order.id, exact: true }).click();
  await expect(page.getByRole("heading", { name: "订单详情", exact: true })).toBeVisible();
  await expect(page.getByRole("main")).toContainText("Alice · alice@example.test");
  await expect(page.getByRole("main")).toContainText("OEM Brand");
  await expect(page.getByRole("button", { name: "确认收款", exact: true })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "登记线下退款", exact: true })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "退款", exact: true })).toHaveCount(0);
  await expect(page.getByLabel("实际收款时间", { exact: true })).toHaveCount(0);
  await expect(page.getByLabel("实际退款时间", { exact: true })).toHaveCount(0);
  await expect(page.getByRole("dialog")).toHaveCount(0);
});
