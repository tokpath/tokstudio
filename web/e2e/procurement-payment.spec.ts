import { expect, test } from "@playwright/test";

test("an unknown procurement payment recovers from the original order and releases navigation", async ({ page }) => {
  const base = "/api/v1/operators/operator-one/procurement";
  const operator = { id: "operator-one", name: "测试 OEM", legal_name: "测试主体", kind: "oem", brand_id: "brand-one" };
  const checkout = { name: "checkout", label: "发起真实付款", method: "POST", path: `${base}/{id}/checkout`, fields: [{ name: "merchant_id", label: "付款方式", kind: "select", default_value: "kvoucher-one", options: [{ value: "kvoucher-one", label: "kvoucher · MYR" }] }] };
  const posts: string[] = [];
  let status = "awaiting_payment";
  await page.route("**/api/v1/**", async route => {
    const req = route.request(), url = new URL(req.url());
    let out: unknown = {};
    let code = 200;
    if (url.pathname === "/api/v1/public/context") out = { brand: { id: "brand-one", name: "TokenHub", api_base_url: "http://127.0.0.1/v1", theme: {} }, operator, theme: {}, monetary: { currency: "USD", atg_per_unit: "1", status: "locked", version: 1 }, availability: [], payment_methods: [] };
    else if (url.pathname === "/api/v1/auth/me") out = { kind: "staff", staff: { id: "staff-one", email: "oem@tokenhub.local", display_name: "OEM 负责人" }, workspaces: [{ operator, permissions: ["*"] }], active_operator_id: operator.id, csrf_token: "csrf_test" };
    else if (req.method() === "POST") {
      posts.push(url.pathname);
      code = 409;
      out = { error: { code: "payment_outcome_unknown", message: "原支付结果尚待核实，请查询原交易，重试将沿用原业务编号", request_id: "request-one", retryable: true } };
    } else if (url.pathname === base) out = { title: "采购与供货", description: "采购付款", columns: [], operations: [], row_operations: [checkout], views: [{ value: "buyer", label: "我方采购" }], items: [{ id: "purchase-one", status, values: { record_kind: "order", name: "阶梯赠送供货报价", due_money: "5000", paid_atg: "5000", gift_atg: "30" }, allowed_actions: status === "completed" ? [] : ["checkout"] }] };
    await route.fulfill({ status: code, contentType: "application/json", body: JSON.stringify(out) });
  });
  await page.goto("/manage/operator-one/procurement");
  await page.getByRole("button", { name: "发起真实付款", exact: true }).click();
  const panel = page.getByRole("dialog");
  await panel.getByRole("button", { name: "发起真实付款", exact: true }).click();
  await expect(panel.getByText("原采购订单状态", { exact: true })).toBeVisible();
  await expect(panel.getByRole("button", { name: "关闭办理面板", exact: true })).toBeEnabled();
  await expect(panel.getByRole("button", { name: "返回采购订单", exact: true })).toBeEnabled();
  await expect(panel.getByRole("button", { name: "重试本次操作", exact: true })).toHaveCount(0);
  status = "completed"; // The worker or callback finishes after the response was lost.
  await expect(panel.getByText("采购付款与库存交付已完成，无须再次付款。", { exact: true })).toBeVisible({ timeout: 10000 });
  await expect(panel.getByRole("alert")).toHaveCount(0);
  expect(posts).toEqual([`${base}/purchase-one/checkout`]);
  await panel.getByRole("button", { name: "返回采购订单", exact: true }).click();
  await expect(panel).toHaveCount(0);
  await expect(page.getByText("已完成", { exact: true })).toBeVisible();
  await page.reload();
  await expect(page.getByText("已完成", { exact: true })).toBeVisible();
});
