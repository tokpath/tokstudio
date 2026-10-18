import { test, expect, type Page, type Locator } from "@playwright/test";
import { readFileSync, writeFileSync, existsSync } from "node:fs";
import { randomUUID } from "node:crypto";

const fixtureFile = process.env.TOKENHUB_FRONTEND_ACCEPTANCE_FILE;
test.skip(!fixtureFile, "Requires the isolated live application and independent protocol service");

async function login(page: Page, person: { email: string; password: string }, kind = "staff") {
  await page.goto("/login");
  await page.getByRole("radio", { name: kind === "staff" ? "经营人员" : "客户", exact: true }).check();
  await page.getByLabel("邮箱", { exact: true }).fill(person.email);
  await page.getByLabel("密码", { exact: true }).fill(person.password);
  await page.getByRole("button", { name: "登录", exact: true }).click();
  await expect(page).toHaveURL(kind === "staff" ? /\/manage\// : /\/workspace/);
}
async function field(panel: Locator, name: string, value: string | boolean) {
  const box = panel.locator(`[data-command-field="${name}"]`);
  const select = box.locator("select");
  if (await select.count()) { await select.selectOption(String(value)); return; }
  if (typeof value === "boolean") { await box.getByRole("checkbox").setChecked(value); return; }
  await box.locator("input,textarea").last().fill(value);
}
async function config(panel: Locator, name: string, value: string | boolean) {
  const box = panel.locator(`[data-config-path="${name}"]`).first();
  const select = box.locator("select");
  if (await select.count()) { await select.selectOption(String(value)); return; }
  if (typeof value === "boolean") { await box.getByRole("checkbox").setChecked(value); return; }
  await box.locator("input,textarea").last().fill(value);
}
async function submit(page: Page, label: string, path: string, status = 200) {
  const panel = page.getByRole("dialog").last();
  const confirmation = panel.locator("form > div.rounded-control").getByRole("checkbox");
  if (await confirmation.count()) await confirmation.check();
  const response = page.waitForResponse(r => new URL(r.url()).pathname === path && r.request().method() === "POST");
  await panel.getByRole("button", { name: label, exact: true }).last().click();
  const received = await response, data = await received.json();
  expect(received.status(), JSON.stringify(data)).toBe(status);
  if (status === 200) await expect(panel.getByRole("status")).toBeVisible();
  await panel.getByRole("button", { name: "关闭办理面板", exact: true }).click();
  return data;
}
async function operation(page: Page, label: string, values: Record<string, string | boolean>, path: string) {
  await page.getByRole("button", { name: label, exact: true }).click();
  const panel = page.getByRole("dialog").last();
  for (const [name, value] of Object.entries(values)) await field(panel, name, value);
  return submit(page, label, path);
}
function localTime(date = new Date()) { return new Date(date.getTime() - date.getTimezoneOffset() * 60_000).toISOString().slice(0, 16); }
function card(page: Page, id: string) { return page.locator("section.rounded-card").filter({ has: page.getByText(id, { exact: true }) }).last(); }

test("independent OEM and channel run purchasing, authorization, retail, risk, exit and takeover through actual UI", async ({ browser }, info) => {
  test.setTimeout(600_000);
  const root = JSON.parse(readFileSync(fixtureFile!, "utf8")), f = root.personas.operators;
  const journalFile = fixtureFile!.replace(/[^/]+$/, "operators-ui-journal.json");
  let journal: Record<string, any> = existsSync(journalFile) ? JSON.parse(readFileSync(journalFile, "utf8")) : {};
  if (journal.started_at !== root.started_at) journal = { started_at: root.started_at };
  const save = (key: string, value: any) => { journal[key] = value; writeFileSync(journalFile, JSON.stringify(journal, null, 2)); };
  const contexts = await Promise.all([f.oem_owner, f.channel_owner, f.customer, f.finance, f.support, f.readonly].map(() => browser.newContext({ baseURL: f.origin })));
  const pages = await Promise.all(contexts.map(c => c.newPage()));
  const [oem, channel, customer, finance, support, readonly] = pages;
  const failures: string[] = [];
  let continuitySecret: string | undefined;
  pages.forEach(page => { page.setDefaultTimeout(20_000); page.on("pageerror", error => failures.push(error.message)); });
  const oemBase = `/api/v1/operators/${f.oem_id}`, channelBase = `/api/v1/operators/${f.channel_id}`;
  try {
    await login(oem, f.oem_owner); await login(channel, f.channel_owner);
    await login(finance, f.finance); await login(support, f.support); await login(readonly, f.readonly);
    // Parent ownership is not a membership in a child's operating workspace.
    expect((await oem.request.get(`${channelBase}/customers`)).status()).toBe(403);
    expect((await oem.request.get(`${channelBase}/assets`)).status()).toBe(403);
    expect((await channel.request.get(`/api/v1/operators/${f.other_channel_id}/customers`)).status()).toBe(403);
    const descendants = await (await channel.request.get(`${channelBase}/operators`)).json();
    expect(descendants.items).toHaveLength(0);
    const channelSession = await (await channel.request.get("/api/v1/auth/me")).json();
    const forbiddenChild = await channel.request.post(`${channelBase}/operators`, { headers: { "X-CSRF-Token": channelSession.csrf_token, "Idempotency-Key": randomUUID(), Origin: f.origin }, data: { kind: "channel", name: "must never be created", legal_name: "negative relationship check", quote_reference: "negative", exit_terms_reference: "negative" } });
    expect(forbiddenChild.status()).toBe(400);
    await channel.goto(`/manage/${f.channel_id}/operators`);
    await expect(channel.getByRole("button", { name: "建立 OEM / 渠道", exact: true })).toHaveCount(0);
    await finance.goto(`/manage/${f.channel_id}/orders`);
    await expect(finance.getByRole("link", { name: "模型与定价", exact: true })).toHaveCount(0);
    await support.goto(`/manage/${f.channel_id}/customers`);
    if (!journal.takeover_executed) await expect(support.getByRole("button", { name: "客户工作台", exact: true })).toBeVisible();
    await expect(support.getByRole("link", { name: "收款与分配", exact: true })).toHaveCount(0);
    await readonly.goto(`/manage/${f.oem_id}/operators`);
    await expect(readonly.getByRole("button", { name: "建立 OEM / 渠道", exact: true })).toHaveCount(0);
    await expect(readonly.getByRole("button", { name: "建立直属渠道", exact: true })).toHaveCount(0);
    for (const [person, path] of [[finance, `${channelBase}/plans`], [support, `${channelBase}/payments`], [readonly, `${oemBase}/operators`]] as const) {
      const identity = await (await person.request.get("/api/v1/auth/me")).json();
      const denied = await person.request.post(path, { headers: { "X-CSRF-Token": identity.csrf_token, "Idempotency-Key": randomUUID(), Origin: f.origin }, data: { kind: "channel", name: "forbidden role operation", legal_name: "negative", quote_reference: "negative", exit_terms_reference: "negative" } });
      expect(denied.status()).toBe(403);
    }
    expect((await oem.request.get(`${oemBase}/providers`)).status()).toBe(403);
    await readonly.screenshot({ path: info.outputPath("operator-readonly-boundary.png"), fullPage: true });
    save("permissions", { parent_customer_and_assets: 403, sibling_customers: 403, channel_create_descendants: 400, finance_and_support_navigation_restricted: true });

    if (!journal.created_channel) {
      await oem.goto(`/manage/${f.oem_id}/operators`);
      await oem.getByRole("button", { name: "建立直属渠道", exact: true }).click();
      const panel = oem.getByRole("dialog").last();
      await expect(panel.getByLabel("合作类型", { exact: false })).toHaveCount(0);
      for (const [name, value] of Object.entries({ name: "UI 新开直属渠道", legal_name: "UI 合成独立经营主体", quote_reference: "ui-new-channel-supply", exit_terms_reference: "ui-new-channel-exit" })) await field(panel, name, value);
      save("created_channel", await submit(oem, "建立直属渠道", `${oemBase}/operators`));
    }
    if (!journal.owner_invited) {
      await oem.goto(`/manage/${f.oem_id}/operators`);
      const row = oem.getByRole("row").filter({ hasText: "UI 新开直属渠道" });
      await row.getByRole("button", { name: "邀请首位负责人", exact: true }).click();
      await field(oem.getByRole("dialog").last(), "email", "sim-browser-created-owner@operator.test");
      save("owner_invited", await submit(oem, "邀请首位负责人", `${oemBase}/operators/${journal.created_channel.id}/invite_owner`));
    }
    if (!journal.owner_accepted) {
      let invitationURL = "";
      await expect.poll(() => {
        const messages = JSON.parse(readFileSync(root.mailbox_file, "utf8"));
        const mail = messages.find((message: any) => (message.To || message.to) === "sim-browser-created-owner@operator.test");
        invitationURL = String(mail?.Text || mail?.text || "").match(/http[^\s]+/)?.[0] || ""; return !!invitationURL;
      }, { timeout: 30_000, intervals: [500, 1000] }).toBe(true);
      const context = await browser.newContext({ baseURL: f.origin }), person = await context.newPage(); contexts.push(context); pages.push(person);
      await person.goto(invitationURL); await person.getByLabel("显示名称", { exact: true }).fill("UI 新渠道独立负责人");
      await person.getByLabel("经营人员密码", { exact: true }).fill("simulation-new-owner-password");
      const pending = person.waitForResponse(r => new URL(r.url()).pathname === "/api/v1/auth/staff-invitations/accept" && r.request().method() === "POST");
      await person.getByRole("button", { name: "验证邮箱并接受邀请", exact: true }).click();
      const response = await pending; expect(response.status(), await response.text()).toBe(200);
      await login(person, { email: "sim-browser-created-owner@operator.test", password: "simulation-new-owner-password" });
      const session = await (await person.request.get("/api/v1/auth/me")).json(); expect(session.workspaces.map((w: any) => w.operator.id)).toEqual([journal.created_channel.id]);
      save("owner_accepted", { independent_workspace_only: true });
    }

    // A fresh channel receives its model permission from the OEM in the UI.
    if (!journal.authorized) {
      await oem.goto(`/manage/${f.oem_id}/pricing`);
      await oem.getByRole("button", { name: "授权或上架模型", exact: true }).click();
      const panel = oem.getByRole("dialog").last();
      await config(panel, "action", "authorization"); await config(panel, "buyer_id", f.channel_id); await config(panel, "model_id", f.model_id);
      save("authorized", await submit(oem, "授权或上架模型", `${oemBase}/pricing`));
    }
    if (!journal.listed) {
      await channel.goto(`/manage/${f.channel_id}/pricing`);
      await channel.getByRole("button", { name: "授权或上架模型", exact: true }).click();
      const panel = channel.getByRole("dialog").last(); await config(panel, "model_id", f.model_id);
      save("listed", await submit(channel, "授权或上架模型", `${channelBase}/pricing`));
    }
    if (!journal.retail_price) {
      await channel.goto(`/manage/${f.channel_id}/pricing`);
      await channel.getByRole("button", { name: "发布价格版本", exact: true }).click();
      const panel = channel.getByRole("dialog").last();
      await config(panel, "action", "bootstrap_price"); await config(panel, "price.id", randomUUID());
      await config(panel, "price.model_id", f.model_id); await config(panel, "price.effective_at", localTime(new Date(Date.now() - 60_000)));
      for (const [index, component] of ["input", "output"].entries()) {
        await panel.getByRole("button", { name: "添加计费组件", exact: true }).click();
        for (const [name, value] of Object.entries({ key: component, unit: "token", per: "1", rate: "0.004" })) await config(panel, `price.rule.components.${index}.${name}`, value);
      }
      save("retail_price", await submit(channel, "发布价格版本", `${channelBase}/pricing`));
    }

    if (!journal.quote) {
      await oem.goto(`/manage/${f.oem_id}/procurement`); await oem.getByRole("button", { name: "我方供货", exact: true }).click();
      save("quote", await operation(oem, "向直属采购方发布供货报价", { buyer_id: f.channel_id, price_usd: "80", paid_atg: "100", gift_atg: "20", terms_reference: "operator-ui-fixed-supply", "exit_terms.voluntary_refund": true, "exit_terms.method": "remaining_paid_proportional", "exit_terms.terms_reference": "operator-ui-refund-proportional" }, `${oemBase}/procurement`));
    }
    if (!journal.purchase) {
      await channel.goto(`/manage/${f.channel_id}/procurement`);
      await card(channel, journal.quote.id).getByRole("button", { name: "按原报价创建采购订单", exact: true }).click();
      save("purchase", await submit(channel, "按原报价创建采购订单", `${channelBase}/procurement/${journal.quote.id}/purchase`));
    }
    if (!journal.checkout) {
      await channel.goto(`/manage/${f.channel_id}/procurement`);
      await card(channel, journal.purchase.id).getByRole("button", { name: "发起真实付款", exact: true }).click();
      await field(channel.getByRole("dialog").last(), "merchant_id", f.oem_manual_method_id);
      save("checkout", await submit(channel, "发起真实付款", `${channelBase}/procurement/${journal.purchase.id}/checkout`));
    }
    if (!journal.receipt) {
      await oem.goto(`/manage/${f.oem_id}/payments`);
      save("receipt", await operation(oem, "登记人工核实收款", { provider_receipt_id: "synthetic-operator-ui-" + randomUUID(), gross_usd: "80", fee_status: "pending", currency: "USD", original_amount: "80", recipient_reference: "synthetic OEM recipient", payer_reference: "synthetic channel payer", valuation_evidence: "synthetic USD no FX", verification_evidence: "SYNTHETIC bank fact for actual browser simulation", paid_at: localTime(), observed_at: localTime() }, `${oemBase}/payments`));
    }
    if (!journal.funded) {
      await oem.goto(`/manage/${f.oem_id}/procurement`); await oem.getByRole("button", { name: "我方供货", exact: true }).click();
      await card(oem, journal.purchase.id).getByRole("button", { name: "分配已核实收款", exact: true }).click();
      const panel = oem.getByRole("dialog").last(); await field(panel, "receipt_id", journal.receipt.id); await field(panel, "amount_usd", "80");
      save("funded", await submit(oem, "分配已核实收款", `${oemBase}/procurement/${journal.purchase.id}/fund`));
    }
    if (!journal.delivered) {
      await oem.goto(`/manage/${f.oem_id}/procurement`); await oem.getByRole("button", { name: "我方供货", exact: true }).click();
      const order = card(oem, journal.purchase.id);
      await order.getByRole("button", { name: "重试实际库存交付", exact: true }).click();
      save("delivered", await submit(oem, "重试实际库存交付", `${oemBase}/procurement/${journal.purchase.id}/fulfill`));
    }
    if (!journal.authorization) {
      await channel.goto(`/manage/${f.channel_id}/procurement`);
      save("authorization", await operation(channel, "设定有限自动补货授权", { quote_id: journal.quote.id, trigger_available_atg: "10", single_limit_usd: "80", period_limit_usd: "160", period: "month", mode: "offline", consent_reference: "operator UI explicit offline replenishment consent" }, `${channelBase}/procurement`));
    }
    if (!journal.revised) {
      await channel.goto(`/manage/${f.channel_id}/procurement`);
      await card(channel, journal.authorization.id).getByRole("button", { name: "修订本方自动补货授权", exact: true }).click();
      const panel = channel.getByRole("dialog").last(); await field(panel, "period_limit_usd", "240"); await field(panel, "renewed_consent_reference", "operator UI revised consent 240 USD month");
      save("revised", await submit(channel, "修订本方自动补货授权", `${channelBase}/procurement`));
    }
    if (!journal.cancelled) {
      await channel.goto(`/manage/${f.channel_id}/procurement`);
      await card(channel, journal.authorization.id).getByRole("button", { name: "停止未来自动补货", exact: true }).click();
      save("cancelled", await submit(channel, "停止未来自动补货", `${channelBase}/procurement/${journal.authorization.id}/cancel_authorization`));
    }
    await channel.screenshot({ path: info.outputPath("operator-procurement-replenishment.png"), fullPage: true });
    if (!journal.exit) {
      await channel.goto(`/manage/${f.channel_id}/exits`);
      save("exit", await operation(channel, "按合同申请退出库存", { mode: "voluntary_exit", quantity: "12", evidence: "synthetic original proportional exit terms; no customer service grant yet" }, `${channelBase}/exits`));
    }
    if (!journal.oversell) {
      await oem.goto(`/manage/${f.oem_id}/coverage`);
      await card(oem, f.channel_id).getByRole("button", { name: "设置授权超卖与缓冲", exact: true }).click();
      const panel = oem.getByRole("dialog").last(); await field(panel, "allowance", "5"); await field(panel, "warning", "10");
      save("oversell", await submit(oem, "设置授权超卖与缓冲", `${oemBase}/coverage`));
    }
    await channel.goto(`/manage/${f.channel_id}/coverage`);
    await expect(channel.getByRole("button", { name: "设置授权超卖与缓冲", exact: true })).toHaveCount(0);
    await channel.screenshot({ path: info.outputPath("operator-channel-coverage-readonly.png"), fullPage: true });
    if (!journal.plan) {
      await channel.goto(`/manage/${f.channel_id}/plans`);
      save("plan", await operation(channel, "发布套餐版本", { name: "星河 UI 开业套餐", price_usd: "10", paid_atg: "20", gift_atg: "5", period: "month", terms_reference: "operator UI package terms", new_purchase_enabled: true, renewal_enabled: false }, `${channelBase}/plans`));
    }
    if (!journal.business_open) {
      for (const feature of ["new_purchase", "model_usage"]) {
        await channel.goto(`/manage/${f.channel_id}/business-status`);
        await operation(channel, "提交开业配置检查", { feature, enabled: true, evidence_reference: "UI readiness verified inventory model prices package merchant" }, `${channelBase}/business-status`);
      }
      save("business_open", true);
    }
    if (!journal.oem_plan) {
      await oem.goto(`/manage/${f.oem_id}/plans`);
      save("oem_plan", await operation(oem, "发布套餐版本", { name: "山海品牌直营套餐", price_usd: "15", paid_atg: "30", gift_atg: "5", period: "month", terms_reference: "operator OEM direct retail terms", new_purchase_enabled: true, renewal_enabled: false }, `${oemBase}/plans`));
    }
    if (!journal.oem_business_open) {
      for (const feature of ["registration", "new_purchase", "model_usage"]) {
        await oem.goto(`/manage/${f.oem_id}/business-status`);
        await operation(oem, "提交开业配置检查", { feature, enabled: true, evidence_reference: "OEM independent retail operation and takeover fulfillment readiness" }, `${oemBase}/business-status`);
      }
      save("oem_business_open", true);
    }
    await login(customer, f.customer, "customer");
    if (!journal.topup) {
      await customer.goto("/workspace/topups");
      save("topup", await operation(customer, "创建充值订单", { amount_usd: "50", external_method_id: f.channel_manual_method_id }, "/api/v1/me/topups"));
    }
    if (!journal.topup_receipt) {
      await channel.goto(`/manage/${f.channel_id}/payments`);
      save("topup_receipt", await operation(channel, "登记人工核实收款", { provider_receipt_id: "synthetic-channel-topup-" + randomUUID(), gross_usd: "50", fee_status: "pending", currency: "USD", original_amount: "50", recipient_reference: "synthetic channel recipient", payer_reference: "synthetic customer original payer", valuation_evidence: "synthetic USD no FX", verification_evidence: "SYNTHETIC customer topup bank fact", paid_at: localTime(), observed_at: localTime() }, `${channelBase}/payments`));
    }
    if (!journal.topup_allocated) {
      await channel.goto(`/manage/${f.channel_id}/orders`);
      await channel.getByLabel("搜索记录").fill(journal.topup.id); await channel.getByRole("button", { name: "查询", exact: true }).click();
      await channel.getByRole("button", { name: "查看并办理", exact: true }).click();
      await channel.getByRole("dialog").last().getByRole("button", { name: "将已核实收款分配到此单", exact: true }).click();
      const panel = channel.getByRole("dialog").last(); await field(panel, "receipt_id", journal.topup_receipt.id); await field(panel, "amount_usd", "50");
      save("topup_allocated", await submit(channel, "将已核实收款分配到此单", `${channelBase}/order-context/${journal.topup.id}/allocate`));
    }
    if (!journal.topup_delivered) {
      await channel.goto(`/manage/${f.channel_id}/orders`);
      await channel.getByLabel("搜索记录").fill(journal.topup.id); await channel.getByRole("button", { name: "查询", exact: true }).click();
      await channel.getByRole("button", { name: "查看并办理", exact: true }).click();
      await channel.getByRole("dialog").last().getByRole("button", { name: "核实资金并重试交付", exact: true }).click();
      save("topup_delivered", await submit(channel, "核实资金并重试交付", `${channelBase}/orders/${journal.topup.id}/fulfill`));
    }
    if (!journal.customer_order) {
      await customer.goto("/workspace/plans"); await customer.getByRole("button", { name: "选择此套餐", exact: true }).first().click();
      await customer.getByRole("button", { name: "继续，核对成交条款" }).click();
      await customer.getByLabel("本次使用 USD 支付余额", { exact: true }).fill("10");
      await customer.getByRole("checkbox", { name: /^我确认本次价格/ }).check();
      const pending = customer.waitForResponse(r => new URL(r.url()).pathname === "/api/v1/me/orders" && r.request().method() === "POST");
      await customer.getByRole("button", { name: "确认购买并核实到账", exact: true }).click();
      const response = await pending, order = await response.json(); expect(response.status(), JSON.stringify(order)).toBe(200); save("customer_order", order);
    }
    if (!journal.customer_key) {
      await customer.goto("/workspace/keys"); await customer.getByRole("button", { name: "创建 Key", exact: true }).click();
      await customer.getByLabel("名称", { exact: true }).fill("接管前连续使用的应用"); await customer.getByLabel("ATG 预算上限", { exact: true }).fill("10");
      const pending = customer.waitForResponse(r => new URL(r.url()).pathname === "/api/v1/me/keys" && r.request().method() === "POST");
      await customer.getByRole("button", { name: "创建 Key", exact: true }).last().click(); const response = await pending, key = await response.json(); expect(response.status()).toBe(200);
      continuitySecret = key.secret;
      save("customer_key", { id: key.key.id, name: key.key.name, budget: key.key.budget });
      await expect(customer.getByText(key.secret, { exact: true })).toBeVisible();
      await customer.getByRole("button", { name: "关闭", exact: true }).click();
    }
    if (!journal.customer_call) {
      await customer.goto("/workspace/playground");
      await customer.getByRole("combobox", { name: /^公开模型/ }).selectOption(f.model_id);
      await customer.getByRole("combobox", { name: /^能力/ }).selectOption("chat");
      await customer.getByLabel("本次输入", { exact: true }).fill("Synthetic channel operation journey, preserve the account after takeover.");
      await customer.getByLabel("最大输出 token", { exact: false }).fill("20");
      const pending = customer.waitForResponse(r => new URL(r.url()).pathname === "/api/v1/me/playground" && r.request().method() === "POST");
      await customer.getByRole("button", { name: "确认计费并发送", exact: true }).click();
      const response = await pending, data = await response.json(); expect(response.status(), JSON.stringify(data)).toBe(200);
      save("customer_call", { request_id: data.request_id, status: data.status, charged_atg: data.charged_atg });
      await expect(customer.getByText("本次实际扣费", { exact: true })).toBeVisible();
      await customer.screenshot({ path: info.outputPath("operator-customer-real-model-call.png"), fullPage: true });
    }
    const keysBefore = await (await customer.request.get("/api/v1/me/keys")).json();
    const originalKey = keysBefore.items.find((key: any) => key.name === "接管前连续使用的应用");
    expect(originalKey).toBeTruthy(); save("customer_key", { id: originalKey.id, name: originalKey.name, budget: originalKey.budget });
    const records = await (await customer.request.get("/api/v1/me/requests")).json();
    const record = records.items.find((r: any) => r.id === journal.customer_call.request_id);
    expect(record.components.every((c: any) => c.component === "precision_carry" || c.price_description.startsWith("0.004 / 1 token"))).toBe(true);
    expect(record.charged_atg).toBe("0.068"); save("customer_bill", record);
    const credits = await (await customer.request.get("/api/v1/me/credits")).json();
    expect(Number(credits.customer_atg.available)).toBeGreaterThan(24);
    await customer.goto("/workspace/credit-sources"); await customer.screenshot({ path: info.outputPath("operator-customer-delivered-package.png"), fullPage: true });
    if (!journal.risk_rule) {
      await channel.goto(`/manage/${f.channel_id}/alerts`);
      save("risk_rule", await operation(channel, "新增预警规则", { id: "operator-ui-low-inventory", metric: "procurement_available_atg", warning: "200", critical: "150", direction: "below", recipients: f.channel_owner.id, enabled: true }, `${channelBase}/alerts`));
    }
    await expect.poll(async () => {
      const data = await (await channel.request.get(`${channelBase}/alerts`)).json();
      return data.items.find((item: any) => item.id === "operator-ui-low-inventory")?.status;
    }, { timeout: 75_000, intervals: [1000, 3000] }).toBe("critical");
    save("risk_observed", { rule: "operator-ui-low-inventory", measured_critical: true });
    await channel.goto(`/manage/${f.channel_id}/alerts`); await channel.screenshot({ path: info.outputPath("operator-risk-warning.png"), fullPage: true });
    // The brand cannot read the customer's current assets before an explicit
    // reviewed whole-channel takeover. The same account survives afterwards.
    if (!journal.takeover) {
      await oem.goto(`/manage/${f.oem_id}/takeovers`); await oem.getByRole("button", { name: "发起接管准备", exact: true }).click();
      const panel = oem.getByRole("dialog").last();
      await panel.getByRole("combobox", { name: /^直属渠道/ }).selectOption(f.channel_id);
      for (const [label, value] of [["承接服务每ATG合同作价 USD（大于0）", "0.5"], ["采购库存每ATG合同抵扣 USD（大于0）", "0.5"], ["双方接管合同编号", "operator-ui-takeover-v1"], ["发起依据与证据引用", "explicit synthetic whole-channel takeover decision"]]) await panel.getByLabel(label, { exact: true }).fill(value);
      await panel.getByRole("checkbox").check();
      const pending = oem.waitForResponse(r => new URL(r.url()).pathname === `${oemBase}/takeovers` && r.request().method() === "POST");
      await panel.getByRole("button", { name: "建立准备事项", exact: true }).click();
      const response = await pending, data = await response.json(); expect(response.status(), JSON.stringify(data)).toBe(200); save("takeover", data);
    }
    if (!journal.takeover_executed) {
      await oem.goto(`/manage/${f.oem_id}/takeovers`);
      await oem.getByRole("button", { name: "排空检查并刷新快照", exact: true }).click();
      await expect(oem.getByRole("button", { name: "复核并执行接管", exact: true })).toBeVisible();
      await oem.getByRole("button", { name: "审阅完整快照", exact: true }).click();
      await expect(oem.getByText(f.customer.id, { exact: true }).first()).toBeVisible();
      await oem.screenshot({ path: info.outputPath("operator-takeover-full-snapshot.png"), fullPage: true });
      await oem.getByRole("button", { name: "复核并执行接管", exact: true }).click();
      const panel = oem.getByRole("dialog").last(); await panel.getByRole("checkbox").check();
      const pending = oem.waitForResponse(r => new URL(r.url()).pathname === `${oemBase}/takeovers/${journal.takeover.id}/execute` && r.request().method() === "POST");
      await panel.getByRole("button", { name: "确认提交", exact: true }).click();
      const response = await pending, data = await response.json(); expect(response.status(), JSON.stringify(data)).toBe(200); save("takeover_executed", data);
    }
    await customer.reload(); const me = await (await customer.request.get("/api/v1/auth/me")).json(); expect(me.operator.id).toBe(f.oem_id);
    const after = await (await customer.request.get("/api/v1/me/credits")).json(); expect(after.customer_atg.available).toBe(credits.customer_atg.available); expect(after.usd_balance.available).toBe(credits.usd_balance.available);
    const retainedKeys = await (await customer.request.get("/api/v1/me/keys")).json();
    expect(retainedKeys.items.find((key: any) => key.id === journal.customer_key.id)?.budget.limit_atg).toBe("10");
    await oem.goto(`/manage/${f.oem_id}/customers`); await expect(oem.getByText("星河真实流程客户", { exact: true })).toBeVisible();
    await oem.goto(`/manage/${f.oem_id}/takeover-settlements`); await oem.screenshot({ path: info.outputPath("operator-takeover-responsibility-preserved.png"), fullPage: true });
    save("takeover_preserved", { current_operator_id: me.operator.id, customer_atg: after.customer_atg.available, customer_usd: after.usd_balance.available });
    if (!journal.post_takeover_key_call && continuitySecret) {
      const response = await customer.request.post("/api/v1/chat/completions", { headers: { Authorization: `Bearer ${continuitySecret}`, "Idempotency-Key": randomUUID() }, data: { model: f.model_id, messages: [{ role: "user", content: "Synthetic original key remains usable after takeover" }], max_completion_tokens: 20 } });
      expect(response.status(), await response.text()).toBe(200);
      const current = await (await customer.request.get("/api/v1/me/credits")).json();
      expect(Number(after.customer_atg.available) - Number(current.customer_atg.available)).toBeCloseTo(0.034, 12);
      save("post_takeover_key_call", { original_key_id: journal.customer_key.id, actual_charged_atg: "0.034", current_brand_retail_price: "0.002" });
    }
    if (!journal.takeover_settled) {
      const amount = journal.takeover_executed.settlement_due_usd.replace(/^-/, "");
      expect(journal.takeover_executed.settlement_due_usd.startsWith("-")).toBe(true);
      await oem.getByRole("button", { name: "登记实际收付款", exact: true }).click();
      const panel = oem.getByRole("dialog").last();
      for (const [label, value] of [["实际币种", "USD"], ["实际币种金额", amount], ["本次实际结清 USD", amount], ["唯一银行流水 / 支付凭证", "synthetic-takeover-payment-" + randomUUID()], ["核实依据与当次估值证据", "synthetic USD bank payment independently verified"], ["实际收付款时间", localTime()]]) await panel.getByLabel(label, { exact: true }).fill(value);
      await panel.getByRole("checkbox").check();
      const pending = oem.waitForResponse(r => new URL(r.url()).pathname === `${oemBase}/takeovers/${journal.takeover.id}/settle` && r.request().method() === "POST");
      await panel.getByRole("button", { name: "确认提交", exact: true }).click();
      const response = await pending, data = await response.json(); expect(response.status(), JSON.stringify(data)).toBe(200); save("takeover_settled", data);
      await expect(oem.getByRole("button", { name: "登记实际收付款", exact: true })).toHaveCount(0);
      await expect(oem.getByRole("list", { name: "接管办理进度" }).getByText("实际资金结清已完成", { exact: false })).toBeVisible();
      await oem.screenshot({ path: info.outputPath("operator-negative-takeover-settled.png"), fullPage: true });
    }
    if (!journal.exit_refund) {
      await oem.goto(`/manage/${f.oem_id}/exits`);
      await oem.getByRole("button", { name: "按原来源预留本次退款", exact: true }).click();
      const panel = oem.getByRole("dialog").last(); await field(panel, "receipt_id", journal.receipt.id); await field(panel, "amount_usd", "8");
      save("exit_refund", await submit(oem, "按原来源预留本次退款", `${oemBase}/exits/${journal.exit.id}/prepare_refund`));
    }
    if (!journal.exit_intent) {
      await oem.goto(`/manage/${f.oem_id}/exits`);
      await oem.getByRole("button", { name: "记录原路银行退款意图", exact: true }).click();
      await field(oem.getByRole("dialog").last(), "evidence", "synthetic original USD bank route planned for original channel payer");
      save("exit_intent", await submit(oem, "记录原路银行退款意图", `${oemBase}/exits/${journal.exit_refund.id}/begin_manual`));
    }
    if (!journal.exit_paid) {
      await oem.goto(`/manage/${f.oem_id}/exits`);
      await oem.getByRole("button", { name: "核实原付款人实际退款", exact: true }).click();
      const panel = oem.getByRole("dialog").last();
      for (const [name, value] of Object.entries({ "proof.receipt_id": journal.receipt.id, "proof.external_reference": "synthetic-exit-refund-" + randomUUID(), "proof.money.currency": "USD", "proof.money.amount": "8", "proof.executed_at": localTime(), "proof.evidence_reference": "synthetic verified original bank refund", "proof.recipient_reference": "synthetic channel payer", "valuation.id": randomUUID(), "valuation.currency": "USD", "valuation.usd_per_unit": "1", "valuation.source": "synthetic USD no FX", "valuation.effective_at": localTime(new Date(Date.now() - 3600_000)), "valuation.expires_at": localTime(new Date(Date.now() + 3600_000)), valued_at: localTime(), fx_execution_reference: "synthetic USD refund no FX" })) await field(panel, name, String(value));
      save("exit_paid", await submit(oem, "核实原付款人实际退款", `${oemBase}/exits/${journal.exit_refund.id}/verify_manual`));
    }
    const exitRecords = await (await oem.request.get(`${oemBase}/exits`)).json();
    const finishedExit = exitRecords.items.find((item: any) => item.id === journal.exit.id);
    const finishedLeg = exitRecords.items.find((item: any) => item.id === journal.exit_refund.id);
    expect(finishedExit.values.contract_usd).toBe("8"); expect(finishedExit.values.refunded_usd).toBe("8");
    expect(finishedLeg.status).toBe("completed"); expect(finishedLeg.values.remaining_usd).toBe("0");
    save("exit_verified", { contract_usd: "8", refunded_usd: "8", remaining_usd: "0", original_receipt_id: journal.receipt.id });
    await oem.screenshot({ path: info.outputPath("operator-original-source-procurement-exit-refunded.png"), fullPage: true });
    expect(failures).toEqual([]);
    await info.attach("actual-operator-business-facts", { body: JSON.stringify(journal, null, 2), contentType: "application/json" });
  } catch (error) {
    for (const [i, page] of pages.entries()) { await page.screenshot({ path: info.outputPath(`failure-persona-${i}.png`), fullPage: true }).catch(() => {}); await info.attach(`failure-persona-${i}`, { body: await page.locator("body").innerText().catch(() => "page unavailable"), contentType: "text/plain" }); }
    throw error;
  } finally { await Promise.all(contexts.map(context => context.close())); }
});

test("offline replenishment actually creates one unpaid order without creating inventory or payment", async ({ browser }, info) => {
  test.setTimeout(180_000);
  const root = JSON.parse(readFileSync(fixtureFile!, "utf8")), f = root.personas.operators;
  const file = fixtureFile!.replace(/[^/]+$/, "operators-replenishment-journal.json");
  let journal = existsSync(file) ? JSON.parse(readFileSync(file, "utf8")) : {};
  if (journal.started_at !== root.started_at) journal = { started_at: root.started_at };
  const save = (key: string, value: any) => { journal[key] = value; writeFileSync(file, JSON.stringify(journal, null, 2)); };
  const contexts = await Promise.all([browser.newContext({ baseURL: f.origin }), browser.newContext({ baseURL: f.origin })]);
  const [oem, buyer] = await Promise.all(contexts.map(context => context.newPage()));
  const base = `/api/v1/operators/${f.other_channel_id}`, supplier = `/api/v1/operators/${f.oem_id}`;
  try {
    await login(oem, f.oem_owner); await login(buyer, f.other_owner);
    if (!journal.authorized) {
      await oem.goto(`/manage/${f.oem_id}/pricing`); await oem.getByRole("button", { name: "授权或上架模型", exact: true }).click();
      const panel = oem.getByRole("dialog").last(); await config(panel, "action", "authorization"); await config(panel, "buyer_id", f.other_channel_id); await config(panel, "model_id", f.model_id);
      save("authorized", await submit(oem, "授权或上架模型", `${supplier}/pricing`));
    }
    if (!journal.listed) {
      await buyer.goto(`/manage/${f.other_channel_id}/pricing`); await buyer.getByRole("button", { name: "授权或上架模型", exact: true }).click();
      await config(buyer.getByRole("dialog").last(), "model_id", f.model_id); save("listed", await submit(buyer, "授权或上架模型", `${base}/pricing`));
    }
    if (!journal.price) {
      await buyer.goto(`/manage/${f.other_channel_id}/pricing`); await buyer.getByRole("button", { name: "发布价格版本", exact: true }).click();
      const panel = buyer.getByRole("dialog").last(); await config(panel, "action", "bootstrap_price"); await config(panel, "price.id", randomUUID()); await config(panel, "price.model_id", f.model_id); await config(panel, "price.effective_at", localTime(new Date(Date.now() - 60_000)));
      for (const [index, component] of ["input", "output"].entries()) { await panel.getByRole("button", { name: "添加计费组件", exact: true }).click(); for (const [name, value] of Object.entries({ key: component, unit: "token", per: "1", rate: "0.004" })) await config(panel, `price.rule.components.${index}.${name}`, value); }
      save("price", await submit(buyer, "发布价格版本", `${base}/pricing`));
    }
    if (!journal.quote) {
      await oem.goto(`/manage/${f.oem_id}/procurement`); await oem.getByRole("button", { name: "我方供货", exact: true }).click();
      save("quote", await operation(oem, "向直属采购方发布供货报价", { buyer_id: f.other_channel_id, price_usd: "10", paid_atg: "20", gift_atg: "0", terms_reference: "offline actual replenishment quote", "exit_terms.voluntary_refund": false, "exit_terms.method": "non_refundable", "exit_terms.terms_reference": "offline non refundable terms" }, `${supplier}/procurement`));
    }
    if (!journal.authorization) {
      await buyer.goto(`/manage/${f.other_channel_id}/procurement`);
      save("authorization", await operation(buyer, "设定有限自动补货授权", { quote_id: journal.quote.id, trigger_available_atg: "10", single_limit_usd: "10", period_limit_usd: "20", period: "month", mode: "offline", consent_reference: "offline actual UI replenishment consent only creates payable order" }, `${base}/procurement`));
    }
    let resource: any;
    await expect.poll(async () => {
      resource = await (await buyer.request.get(`${base}/procurement`)).json();
      return resource.items.find((item: any) => item.values.record_kind === "replenishment_attempt")?.status;
    }, { timeout: 90_000, intervals: [1000, 3000] }).toBe("awaiting_manual_payment");
    const attempts = resource.items.filter((item: any) => item.values.record_kind === "replenishment_attempt");
    expect(attempts).toHaveLength(1);
    const orders = resource.items.filter((item: any) => item.values.record_kind === "order");
    expect(orders).toHaveLength(1); expect(orders[0].status).toBe("awaiting_payment"); expect(orders[0].values.allocated_usd).toBe("0");
    const coverage = await (await buyer.request.get(`${base}/coverage`)).json();
    expect(coverage.items[0].values.procurement_available).toBe("0");
    save("actual_attempt", { attempt_id: attempts[0].id, order_id: orders[0].id, state: attempts[0].status, allocated_usd: "0", procurement_available: "0" });
    await buyer.goto(`/manage/${f.other_channel_id}/procurement`);
    await expect(buyer.getByText(orders[0].id, { exact: true })).toBeVisible();
    await buyer.screenshot({ path: info.outputPath("offline-replenishment-original-unpaid-order.png"), fullPage: true });
    if (!journal.cancelled) {
      await card(buyer, journal.authorization.id).getByRole("button", { name: "停止未来自动补货", exact: true }).click();
      save("cancelled", await submit(buyer, "停止未来自动补货", `${base}/procurement/${journal.authorization.id}/cancel_authorization`));
    }
    const after = await (await buyer.request.get(`${base}/procurement`)).json();
    expect(after.items.filter((item: any) => item.values.record_kind === "order").map((item: any) => item.id)).toEqual([orders[0].id]);
    await info.attach("actual-offline-replenishment", { body: JSON.stringify(journal, null, 2), contentType: "application/json" });
  } finally { await Promise.all(contexts.map(context => context.close())); }
});
