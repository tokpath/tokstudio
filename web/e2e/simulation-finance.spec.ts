import { expect, test, type Locator, type Page } from "@playwright/test";
import { readFileSync } from "node:fs";
import { randomUUID } from "node:crypto";

const fixturePath = process.env.TOKENHUB_FRONTEND_ACCEPTANCE_FILE;
const fixture = fixturePath ? JSON.parse(readFileSync(fixturePath, "utf8")) : undefined;
const persona = fixture?.personas?.finance;
test.skip(!persona, "Requires isolated multi-persona simulation fixture");
test.setTimeout(240_000);

async function login(page: Page, kind: "staff" | "customer", account: { email: string; password: string }) {
  await page.goto("/login");
  await page.getByRole("radio", { name: kind === "staff" ? "经营人员" : "客户", exact: true }).check();
  await page.getByLabel("邮箱", { exact: true }).fill(account.email);
  await page.getByLabel("密码", { exact: true }).fill(account.password);
  await page.getByRole("button", { name: "登录", exact: true }).click();
  await expect(page).toHaveURL(kind === "staff" ? /\/manage\// : /\/workspace/);
}
async function fields(panel: Locator, values: Record<string, string | boolean>) {
  for (const [name, value] of Object.entries(values)) {
    const field = panel.locator(`[data-command-field="${name}"]`);
    await expect(field).toBeVisible();
    if (typeof value === "boolean") await field.locator('input[type="checkbox"]').setChecked(value);
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

test("finance and customer reconcile partial receipts, independently review a refund, retain unknown-payment holds and settle once", async ({ browser }, info) => {
  const customerContext = await browser.newContext(), financeContext = await browser.newContext(), reviewerContext = await browser.newContext();
  const customer = await customerContext.newPage(), finance = await financeContext.newPage(), reviewer = await reviewerContext.newPage();
  const base = `/api/v1/operators/${persona.operator_id}`, manage = `/manage/${persona.operator_id}`;
  const errors: string[] = [];
  for (const p of [customer, finance, reviewer]) { p.setDefaultTimeout(20_000); p.on("pageerror", e => errors.push(e.message)); }
  try {
    await login(customer, "customer", persona.customer);
    await login(finance, "staff", persona.finance);
    await login(reviewer, "staff", persona.reviewer);
    const startingBalance = Number((await (await customer.request.get("/api/v1/me/credits")).json()).usd_balance.available);
    const startingFees = Number((await (await finance.request.get(`${base}/reports/financial`)).json()).costs.payment_fees_usd);
    await customer.goto("/workspace/topups");
    await customer.getByRole("button", { name: "创建充值订单", exact: true }).click();
    const order = await submit(customer, "创建充值订单", "/api/v1/me/topups", { amount_usd: "100", external_method_id: fixture.manual_method_id });
    const orderID = order.id || order.order?.id;
    expect(orderID).toBeTruthy();
    const receipts: string[] = [];
    for (const [amount, fee] of [["60", "2"], ["40", "1"]]) {
      await finance.goto(`${manage}/payments`);
      await finance.getByRole("button", { name: "登记人工核实收款", exact: true }).click();
      const receipt = await submit(finance, "登记人工核实收款", `${base}/payments`, {
        provider_receipt_id: `simulation-bank-${randomUUID()}`, gross_usd: amount, currency: "USD", original_amount: amount,
        recipient_reference: "SYNTHETIC finance merchant", payer_reference: "SYNTHETIC customer bank", valuation_evidence: "Actual synthetic USD; no FX",
        verification_evidence: "SYNTHETIC local acceptance bank fact", fee_status: "confirmed", fee_usd: fee, net_usd: String(Number(amount) - Number(fee)), paid_at: localNow(), observed_at: localNow(),
      });
      receipts.push(receipt.id);
      await finance.goto(`${manage}/orders?record=${orderID}`);
      await finance.getByLabel("搜索记录").fill(orderID);
      await finance.getByRole("button", { name: "查询", exact: true }).click();
      await finance.getByRole("button", { name: "查看并办理", exact: true }).click();
      await expect(finance.getByRole("dialog").last().getByRole("button", { name: "核实资金并重试交付" })).toHaveCount(0);
      await finance.getByRole("dialog").last().getByRole("button", { name: "将已核实收款分配到此单" }).click();
      await submit(finance, "将已核实收款分配到此单", `${base}/order-context/${orderID}/allocate`, { receipt_id: receipt.id, amount_usd: amount });
      if (amount === "60") {
        const credits = await (await customer.request.get("/api/v1/me/credits")).json();
        expect(credits.usd_balance.available).toBe(String(startingBalance));
        const snapshot = await (await finance.request.get(`${base}/order-context/${orderID}`)).json();
        expect(snapshot.subject.values.external_allocated_usd).toBe("60");
      }
    }
    // Finance allocates verified receipts; the durable worker delivers the
    // funded order. Allow its discovery interval and the shared queue backlog.
    await expect.poll(async () => (await (await customer.request.get("/api/v1/me/credits")).json()).usd_balance.available, { timeout: 60_000 }).toBe(String(startingBalance + 100));
    await finance.goto(`${manage}/refunds`);
    await finance.getByRole("button", { name: "设置独立复核阈值", exact: true }).click();
    await submit(finance, "设置独立复核阈值", `${base}/refunds`, { second_review_above: "10" });
    await customer.goto("/workspace/refunds");
    await customer.getByRole("button", { name: "申请USD余额原路退回", exact: true }).click();
    const refund = await submit(customer, "申请USD余额原路退回", "/api/v1/me/refunds", { amount_usd: "20", reason: "Synthetic unused payment balance return" });
    const readCase = async () => (await (await finance.request.get(`${base}/refunds?record=${refund.id}`)).json()).items as { id: string; status: string; values: Record<string, unknown> }[];
    for (const staff of [finance, reviewer]) {
      await staff.goto(`${manage}/refunds?record=${refund.id}`);
      await staff.getByRole("button", { name: "审批/独立复核", exact: true }).click();
      await submit(staff, "审批/独立复核", `${base}/refunds/${refund.id}/review`, { decision: "approve", evidence: "Independent synthetic bank and original customer source review" });
      if (staff === finance) expect((await readCase()).find(row => row.id === refund.id)?.status).toBe("pending_review");
    }
    const approved = await readCase();
    expect(approved.find(row => row.id === refund.id)?.status).toBe("approved");
    const legs = approved.filter(row => row.values.record_kind === "leg");
    expect(legs).toHaveLength(1);
    const legID = legs[0].id;
    await finance.goto(`${manage}/refunds?record=${refund.id}`);
    await finance.getByRole("button", { name: "登记外部退款执行意图", exact: true }).click();
    await submit(finance, "登记外部退款执行意图", `${base}/refunds/${legID}/begin_manual`, { evidence: "Synthetic outside-bank instruction; no real transfer" });
    await finance.getByRole("button", { name: "结果未知，保留占用", exact: true }).click();
    await submit(finance, "结果未知，保留占用", `${base}/refunds/${legID}/unknown`, { evidence: "Synthetic bank acknowledgement delayed" });
    await expect(finance.getByRole("button", { name: "登记外部退款执行意图", exact: true })).toHaveCount(0);
    const held = await (await customer.request.get("/api/v1/me/credits")).json();
    expect(held.usd_balance.available).toBe(String(startingBalance + 80)); expect(held.usd_balance.held).toBe("20");
    await finance.getByRole("button", { name: "核实实际银行退款", exact: true }).click();
    await submit(finance, "核实实际银行退款", `${base}/refunds/${legID}/verify_manual`, { reference: `simulation-refund-${randomUUID()}`, currency: "USD", amount: "20", actual_usd: "20", fee_usd: "0.5", occurred_at: localNow(), evidence: "SYNTHETIC settled bank statement; original payer; actual USD" });
    const settled = await readCase();
    expect(settled.find(row => row.id === refund.id)?.status).toBe("completed");
    const credits = await (await customer.request.get("/api/v1/me/credits")).json();
    expect(credits.usd_balance.available).toBe(String(startingBalance + 80)); expect(credits.usd_balance.held).toBe("0");
    const reportResponse = finance.waitForResponse(r => new URL(r.url()).pathname === `${base}/reports/financial`);
    await finance.goto(`${manage}/reports`);
    const report = await (await reportResponse).json();
    expect(report.costs.payment_fees_usd, "default current report must include the just-confirmed fees").toBe(String(startingFees + 3.5));

    await finance.goto(`${manage}/invoices`);
    await finance.getByRole("button", { name: "设置零售客户开票规则", exact: true }).click();
    await submit(finance, "设置零售客户开票规则", `${base}/invoices`, { enabled: true, stage: "top_up", maximum_ratio: "1", rules_reference: "Synthetic original receipt invoice policy" });
    await customer.goto("/workspace/invoices");
    await customer.getByRole("button", { name: "按原始收款申请发票", exact: true }).click();
    const invoice = await submit(customer, "按原始收款申请发票", "/api/v1/me/invoices", { "lines.0.receipt_id": receipts[0], "lines.0.amount_usd": "40", "details.title": "合成仿真客户", "details.tax_id": "SYNTHETIC-NOT-A-TAX-ID", "details.email": persona.customer.email });
    await finance.goto(`${manage}/invoices`);
    await finance.getByLabel("搜索记录").fill(invoice.id);
    await finance.getByRole("button", { name: "查询", exact: true }).click();
    await expect(finance.getByRole("button", { name: "文件与下载", exact: true })).toHaveCount(1);
    await finance.getByRole("button", { name: "文件与下载", exact: true }).click();
    const png = Buffer.from("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+cWZkAAAAASUVORK5CYII=", "base64");
    await finance.getByLabel("票据文件（PDF/PNG/JPEG，不超过10MiB）").setInputFiles({ name: "synthetic-invoice.png", mimeType: "image/png", buffer: png });
    const uploaded = finance.waitForResponse(r => r.request().method() === "POST" && new URL(r.url()).pathname === `${base}/invoices/${invoice.id}/files`);
    await finance.getByRole("button", { name: "上传真实私有文件", exact: true }).click();
    expect((await uploaded).status()).toBe(200);
    await finance.getByRole("button", { name: "登记实际开票结果", exact: true }).click();
    await submit(finance, "登记实际开票结果", `${base}/invoices/${invoice.id}/register`, { state: "issued", invoice_reference: `SYNTHETIC-${randomUUID()}`, evidence: "SYNTHETIC invoice only; no legal document issued", reason: "Local role simulation" });
    await customer.reload();
    await customer.getByLabel("搜索记录").fill(invoice.id);
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
    await expect(customer.getByText("已完成", { exact: true }).first()).toBeVisible();
    await expect(customer.getByRole("button", { name: "撤销未执行申请", exact: true })).toHaveCount(0);
    await finance.goto(`${manage}/refunds?record=${refund.id}`);
    await expect(finance.getByText("已完成", { exact: true }).first()).toBeVisible();
    for (const label of ["退款事项", "退款分项", "实际退款付款"]) await expect(finance.getByRole("button", { name: label, exact: true })).toBeVisible();
    await finance.screenshot({ path: info.outputPath("finance-refund-reconciled.png"), fullPage: true });
    expect(errors).toEqual([]);
    await info.attach("synthetic-finance-facts", { body: JSON.stringify({ order_id: orderID, receipt_ids: receipts, initial_customer_available_usd: String(startingBalance), credited_usd: "100", merchant_receipt_fees_usd: "3", refund_usd: "20", merchant_refund_fee_usd: "0.5", final_customer_available_usd: credits.usd_balance.available, final_customer_held_usd: credits.usd_balance.held, distinct_reviewers: 2, invoice_id: invoice.id, private_invoice_downloaded: true, other_customer_download_status: 404 }), contentType: "application/json" });
  } finally { await customerContext.close(); await financeContext.close(); await reviewerContext.close(); }
});
