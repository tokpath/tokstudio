import { expect, test, type Locator, type Page } from "@playwright/test";
import { readFileSync } from "node:fs";
import { randomUUID } from "node:crypto";

const fixturePath = process.env.TOKENHUB_FRONTEND_ACCEPTANCE_FILE;
const fixture = fixturePath ? JSON.parse(readFileSync(fixturePath, "utf8")) : undefined;
const persona = fixture?.personas?.finance;
test.skip(!persona, "Requires isolated multi-persona simulation fixture");
test.setTimeout(240_000);

// Field help carries the same business name; match its form control precisely.
function control(scope: Page | Locator, label: string, exact = false) {
  return scope.locator("input,textarea,select").and(scope.getByLabel(label, { exact }));
}

async function login(page: Page, kind: "staff" | "customer", account: { email: string; password: string }) {
  await page.goto("/login");
  await control(page, "邮箱", true).fill(account.email);
  await control(page, "密码", true).fill(account.password);
  await page.getByRole("button", { name: "登录", exact: true }).click();
  await expect(page).toHaveURL(kind === "staff" ? /\/manage\// : /\/workspace/);
}
async function fields(panel: Locator, values: Record<string, string | boolean>) {
  for (const summary of await panel.locator("details:not([open]) > summary").all()) await summary.click();
  for (const [name, value] of Object.entries(values)) {
    const field = panel.locator(`[data-command-field="${name}"]`);
    await expect(field).toBeVisible();
    if (typeof value === "boolean") await field.getByRole("radio", { name: value ? "是" : "否", exact: true }).check();
    else if (await field.locator("select").count()) await field.locator("select").selectOption(value);
    else await field.locator("input,textarea").first().fill(value);
  }
}
async function submit(page: Page, label: string, path: string, values: Record<string, string | boolean> = {}) {
  const panel = page.getByRole("dialog").last();
  await fields(panel, values);
  for (const box of await panel.locator('form > div input[type="checkbox"]').all()) await box.check();
  const response = page.waitForResponse(r => r.request().method() === "POST" && new URL(r.url()).pathname === path);
  await panel.getByRole("button", { name: label, exact: true }).click();
  const result = await response; const body = await result.json();
  expect(result.status(), `${label}: ${JSON.stringify(body)}`).toBe(200);
  await expect(panel.getByRole("status")).toBeVisible();
  await panel.getByRole("button", { name: "关闭办理面板", exact: true }).click();
  return body;
}
function localNow() { const now = new Date(); return new Date(now.getTime() - now.getTimezoneOffset() * 60000).toISOString().slice(0, 16); }

test("finance records one whole payment, links private evidence, reviews a refund and retains unknown-payment holds", async ({ browser }, info) => {
  const customerContext = await browser.newContext(), financeContext = await browser.newContext();
  const customer = await customerContext.newPage(), finance = await financeContext.newPage();
  const base = `/api/v1/operators/${persona.operator_id}`, manage = `/manage/${persona.operator_id}`;
  const errors: string[] = [];
  for (const p of [customer, finance]) { p.setDefaultTimeout(20_000); p.on("pageerror", e => errors.push(e.message)); }
  try {
    await login(customer, "customer", persona.customer);
    await login(finance, "staff", persona.finance);
    const startingBalance = Number((await (await customer.request.get("/api/v1/me/credits")).json()).money_balance.available);
    const startingFees = Number((await (await finance.request.get(`${base}/reports/financial`)).json()).costs.payment_fees_money);
    await customer.goto("/workspace/topups");
    await customer.getByRole("button", { name: "创建充值订单", exact: true }).click();
    const order = await submit(customer, "创建充值订单", "/api/v1/me/topups", { amount_money: "100", external_method_id: fixture.manual_method_id });
    const orderID = order.id || order.order?.id;
    expect(orderID).toBeTruthy();
    const receipts: string[] = [];
    await finance.goto(`${manage}/orders?record=${orderID}`);
    await expect(finance.getByRole("heading", { name: "订单与收款办理", exact: true })).toBeVisible();
    await finance.getByRole("button", { name: "登记本单线下收款", exact: true }).click();
    const receiptPanel=finance.getByRole("dialog").last();
    for(const name of ["original_amount","net_money","observed_at","valuation_evidence"]) await expect(receiptPanel.locator(`[data-command-field="${name}"]`)).toHaveCount(0);
    await receiptPanel.getByText("附加付款凭据（可选）",{exact:true}).click();
    await receiptPanel.getByLabel("选择付款凭据").setInputFiles({name:"synthetic-bank.png",mimeType:"image/png",buffer:Buffer.from("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+cWZkAAAAASUVORK5CYII=","base64")});
    await receiptPanel.getByRole("button",{name:"上传并关联凭据",exact:true}).click();
    await expect(receiptPanel.getByText("凭据已关联",{exact:false})).toBeVisible();
    const marker=await receiptPanel.locator('[data-command-field="verification_evidence"] textarea').inputValue();
    const receipt = await submit(finance, "登记本单线下收款", `${base}/payments`, {
      provider_receipt_id: `simulation-bank-${randomUUID()}`, gross_money: "100", currency: "USD",
      recipient_reference: "SYNTHETIC finance merchant", payer_reference: "SYNTHETIC customer bank",
      verification_evidence: "SYNTHETIC local acceptance bank fact\n"+marker, fee_status: "pending", paid_at: localNow(),
    });
    receipts.push(receipt.id);
    await finance.goto(`${manage}/payments?q=${receipt.id}`);
    await finance.getByRole("button",{name:"核实手续费与结算",exact:true}).click();
    await submit(finance,"核实手续费与结算",`${base}/payments/${receipt.id}/accounting`,{fee_money:"3",evidence:"Independent synthetic bank fee statement"});
    // Finance allocates verified receipts; the durable worker delivers the
    // funded order. Allow its discovery interval and the shared queue backlog.
    await expect.poll(async () => (await (await customer.request.get("/api/v1/me/credits")).json()).money_balance.available, { timeout: 60_000 }).toBe(String(startingBalance + 100));
    await finance.goto(`${manage}/balance-withdrawals?view=methods`);
    await finance.getByRole("button", { name: "新增提现方式", exact: true }).click();
    await submit(finance, "新增提现方式", `${base}/balance-withdrawals`, { payout_kind: "alipay", method_name: "仿真支付宝", fee_mode: "fixed", flat_fee_money: "0.5", enabled: true });
    await customer.goto("/workspace/refunds");
    await customer.getByLabel("提现金额 USD").fill("20");
    await customer.getByLabel("支付宝实名").fill("合成客户");
    await customer.getByLabel("支付宝账号").fill("synthetic@example.test");
    const submitted = customer.waitForResponse(r => r.request().method() === "POST" && new URL(r.url()).pathname === "/api/v1/me/balance-withdrawals");
    await customer.getByRole("button", { name: "提交提现申请", exact: true }).click();
    const withdrawalResponse = await submitted;
    const withdrawal = await withdrawalResponse.json();
    expect(withdrawalResponse.status(), JSON.stringify(withdrawal)).toBe(200);
    const withdrawalID = withdrawal.id as string;
    expect(withdrawalID).toBeTruthy();
    const heldBeforePayment = await (await customer.request.get("/api/v1/me/credits")).json();
    expect(heldBeforePayment.money_balance.available).toBe(String(startingBalance + 80)); expect(heldBeforePayment.money_balance.held).toBe("20");
    await finance.goto(`${manage}/balance-withdrawals`);
    await finance.getByRole("button", { name: "通过", exact: true }).click();
    await submit(finance, "通过", `${base}/balance-withdrawals/${withdrawalID}/approve`);
    await finance.goto(`${manage}/balance-withdrawals`);
    await finance.getByRole("button", { name: "登记付款", exact: true }).click();
    await submit(finance, "登记付款", `${base}/balance-withdrawals/${withdrawalID}/verify_paid`, { evidence: "SYNTHETIC settled payout to the declared Alipay account", currency: "USD", paid_at: localNow() });
    const credits = await (await customer.request.get("/api/v1/me/credits")).json();
    expect(credits.money_balance.available).toBe(String(startingBalance + 80)); expect(credits.money_balance.held).toBe("0");
    const reportResponse = finance.waitForResponse(r => new URL(r.url()).pathname === `${base}/reports/financial`);
    await finance.goto(`${manage}/reports`);
    const report = await (await reportResponse).json();
    expect(report.costs.payment_fees_money, "collection fee stays on the receipt; the withdrawal fee is paid by the customer").toBe(String(startingFees + 3));

    const ownerContext = await browser.newContext();
    const owner = await ownerContext.newPage();
    await login(owner, "staff", { email: fixture.owner_email, password: fixture.owner_password });
    const ownerSession = await (await owner.request.get("/api/v1/auth/me")).json();
    const planResponse = await owner.request.post(`${base}/plans`, { headers: { "X-CSRF-Token": ownerSession.csrf_token, "Idempotency-Key": randomUUID(), Origin: new URL(owner.url()).origin }, data: { name: "合成购买套餐", price_money: "40", paid_atg: "40", gift_atg: "0", period: "lifetime", timezone: "UTC", terms_summary: "合成服务约定", new_purchase_enabled: true, renewal_enabled: false, expected_version: 0 } });
    const plan = await planResponse.json();
    expect(planResponse.status(), JSON.stringify(plan)).toBe(200);
    await ownerContext.close();
    const customerSession = await (await customer.request.get("/api/v1/auth/me")).json();
    const quoteResponse = await customer.request.post("/api/v1/me/orders/quotes", { headers: { "X-CSRF-Token": customerSession.csrf_token, "Idempotency-Key": randomUUID(), Origin: new URL(customer.url()).origin }, data: { plan_id: plan.id } });
    const quote = await quoteResponse.json();
    expect(quoteResponse.status(), JSON.stringify(quote)).toBe(200);
    const orderResponse = await customer.request.post("/api/v1/me/orders", { headers: { "X-CSRF-Token": customerSession.csrf_token, "Idempotency-Key": randomUUID(), Origin: new URL(customer.url()).origin }, data: { quote_id: quote.id, terms_version: quote.terms_version, funding: { money_balance: "40", commission_money: "0" }, auto_renew: false, allow_commission: false } });
    const purchased = await orderResponse.json();
    expect(orderResponse.status(), JSON.stringify(purchased)).toBe(200);
    expect(purchased.status).toBe("completed");
    await finance.goto(`${manage}/invoices`);
    await finance.getByRole("button", { name: "编辑开票说明", exact: true }).click();
    await submit(finance, "编辑开票说明", `${base}/invoices`, { instructions: "Synthetic invoice handling instructions" });
    await customer.goto("/workspace/invoices");
    await customer.getByRole("button", { name: "申请发票", exact: true }).click();
    await customer.getByRole("combobox", {name:"购买订单",exact:true}).fill(purchased.id);
    await expect(customer.getByRole("combobox", {name:"购买订单",exact:true})).toHaveValue(/合成购买套餐/);
    await expect(customer.locator('[data-command-field="lines.0.amount_money"]')).toHaveCount(0);
    const invoice = await submit(customer, "申请发票", "/api/v1/me/invoices", { "details.title": "合成仿真客户", "details.tax_id": "SYNTHETIC-NOT-A-TAX-ID", "details.email": persona.customer.email });
    await finance.goto(`${manage}/invoices`);
    await control(finance, "搜索记录").fill(invoice.id);
    await finance.getByRole("button", { name: "查询", exact: true }).click();
    await expect(finance.getByRole("button", { name: "文件与下载", exact: true })).toHaveCount(0);
    await finance.getByRole("button", { name: "开具发票", exact: true }).click();
    const png = Buffer.from("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+cWZkAAAAASUVORK5CYII=", "base64");
    await control(finance, "票据文件（PDF/PNG/JPEG，不超过10MiB）").setInputFiles({ name: "synthetic-invoice.png", mimeType: "image/png", buffer: png });
    const uploaded = finance.waitForResponse(r => r.request().method() === "POST" && new URL(r.url()).pathname === `${base}/invoices/${invoice.id}/files`);
    await finance.getByRole("button", { name: "上传文件", exact: true }).click();
    expect((await uploaded).status()).toBe(200);
    await submit(finance, "开具发票", `${base}/invoices/${invoice.id}/register`, { invoice_reference: `SYNTHETIC-${randomUUID()}`, reason: "Local role simulation" });
    await customer.reload();
    await control(customer, "搜索记录").fill(invoice.id);
    await customer.getByRole("button", { name: "查询", exact: true }).click();
    await expect(customer.getByRole("button", { name: "文件与下载", exact: true })).toHaveCount(1);
    await customer.getByRole("button", { name: "文件与下载", exact: true }).click();
    const link = customer.getByRole("link", { name: "授权下载", exact: true });
    await expect(link).toBeVisible();
    const download = customer.waitForEvent("download");
    await link.click();
    expect((await download).suggestedFilename()).toBe("synthetic-invoice.png");
    const privateURL = await link.getAttribute("href");
    const otherContext = await browser.newContext();
    const other = await otherContext.newPage();
    await login(other, "customer", { email: fixture.customer_email, password: fixture.customer_password });
    // The private-file endpoint hides the existence of another customer's invoice.
    expect((await other.request.get(privateURL!)).status()).toBe(404);
    await otherContext.close();
    await customer.goto("/workspace/refunds");
    await customer.reload();
    await expect(customer.getByText("已核实付款", { exact: true }).first()).toBeVisible();
    await expect(customer.getByRole("button", { name: "取消提现申请", exact: true })).toHaveCount(0);
    await finance.goto(`${manage}/balance-withdrawals`);
    await expect(finance.getByText("已核实付款", { exact: true }).first()).toBeVisible();
    await finance.screenshot({ path: info.outputPath("finance-balance-withdrawal.png"), fullPage: true });
    expect(errors).toEqual([]);
    await info.attach("synthetic-finance-facts", { body: JSON.stringify({ order_id: orderID, receipt_ids: receipts, initial_customer_available_money: String(startingBalance), credited_money: "100", merchant_receipt_fees_money: "3", withdrawal_id: withdrawalID, withdrawal_amount_money: "20", customer_fee_money: "0.5", net_paid_money: "19.5", final_customer_available_money: credits.money_balance.available, final_customer_held_money: credits.money_balance.held, invoice_id: invoice.id, private_invoice_downloaded: true, other_customer_download_status: 404 }), contentType: "application/json" });
  } finally { await customerContext.close(); await financeContext.close(); }
});
