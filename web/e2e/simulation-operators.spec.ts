import { test, expect, type Page, type Locator } from "@playwright/test";
import { readFileSync, writeFileSync, existsSync } from "node:fs";
import { randomUUID } from "node:crypto";

const fixtureFile = process.env.TOKENHUB_FRONTEND_ACCEPTANCE_FILE;
test.skip(!fixtureFile, "Requires the isolated live application and independent protocol service");
test.use({ actionTimeout: 20_000 });

// Field help carries the same business name; match its form control precisely.
function control(scope: Page | Locator, label: string, exact = false) {
  return scope.locator("input,textarea,select").and(scope.getByLabel(label, { exact }));
}

async function login(page: Page, person: { email: string; password: string }, kind = "staff") {
  await page.goto("/login");
  await control(page, "邮箱", true).fill(person.email);
  await control(page, "密码", true).fill(person.password);
  await page.getByRole("button", { name: "登录", exact: true }).click();
  await expect(page).toHaveURL(kind === "staff" ? /\/manage\// : /\/workspace/);
}
async function revealFields(panel: Locator) {
  for (const summary of await panel.locator("details:not([open]) > summary").all()) await summary.click();
}
async function field(panel: Locator, name: string, value: string | boolean) {
  await revealFields(panel);
  if(name==="gift_tiers"){await panel.locator('[data-command-field="gift_tiers"]').getByLabel("本段赠送 %").first().fill(String(value));return;}
  const box = panel.locator(`[data-command-field="${name}"]`);
  const select = box.locator("select");
  if (await select.count()) { await select.selectOption(String(value)); return; }
  if (typeof value === "boolean") { await box.getByRole("radio", { name: value ? "是" : "否", exact: true }).check(); return; }
  await box.locator("input,textarea").last().fill(value);
}
async function config(panel: Locator, name: string, value: string | boolean) {
  await revealFields(panel);
  const box = panel.locator(`[data-config-path="${name}"]`).first();
  const select = box.locator("select");
  if (await select.count()) { await select.selectOption(String(value)); return; }
  if (typeof value === "boolean") { await box.getByRole("radio", { name: value ? "是" : "否", exact: true }).check(); return; }
  await box.locator("input,textarea").last().fill(value);
}
async function submit(page: Page, label: string, path: string, status = 200) {
  const panel = page.getByRole("dialog").last();
  const confirmation = panel.locator(".th-review input[type=checkbox]");
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
async function workspaceAPI(page: Page, path: string, options?: { headers: Record<string, string>; data: unknown }) {
  const response = await page.evaluate(async ({ path, options }) => {
    const result = await fetch(path, options && {
      method: "POST",
      headers: { ...options.headers, "Content-Type": "application/json" },
      body: JSON.stringify(options.data),
    });
    return { status: result.status, body: await result.text() };
  }, { path, options });
  return {
    status: () => response.status,
    json: async () => JSON.parse(response.body),
    text: async () => response.body,
  };
}

async function recordRow(page: Page, id: string) {
  const current = new URL(page.url());
  const scope = current.pathname.match(/^\/manage\/([^/]+)\/([^/]+)/);
  expect(scope, "A business record must be selected within its operating workspace").toBeTruthy();
  const response = await workspaceAPI(page, `/api/v1/operators/${scope![1]}/${scope![2]}${current.search}`);
  expect(response.status()).toBe(200);
  const records = await response.json();
  const record = records.items.find((item: any) => item.id === id);
  expect(record, "The requested record must belong to the current authorized business list").toBeTruthy();
  const name = record.presentation?.title || record.values.name || record.values.display_name;
  expect(name, "The list must identify the record by its business name").toBeTruthy();
  const row = page.locator("tr, section.th-card").filter({ has: page.getByText(name, { exact: true }) });
  await expect(row).toHaveCount(1);
  return row;
}

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
    expect((await workspaceAPI(oem, `${channelBase}/customers`)).status()).toBe(403);
    expect((await workspaceAPI(oem, `${channelBase}/assets`)).status()).toBe(403);
    expect((await workspaceAPI(channel, `/api/v1/operators/${f.other_channel_id}/customers`)).status()).toBe(403);
    const descendants = await (await workspaceAPI(channel, `${channelBase}/operators`)).json();
    expect(descendants.items).toHaveLength(0);
    const channelSession = await (await workspaceAPI(channel, "/api/v1/auth/me")).json();
    const forbiddenChild = await workspaceAPI(channel, `${channelBase}/operators`, { headers: { "X-CSRF-Token": channelSession.csrf_token, "Idempotency-Key": randomUUID(), Origin: f.origin }, data: { kind: "channel", name: "must never be created", legal_name: "negative relationship check", supply_terms: "negative permission check" } });
    expect(forbiddenChild.status()).toBe(400);
    await channel.goto(`/manage/${f.channel_id}/operators`);
    await expect(channel.getByRole("button", { name: "建立 OEM / 渠道", exact: true })).toHaveCount(0);
    await finance.goto(`/manage/${f.channel_id}/orders`);
    await expect(finance.getByRole("link", { name: "模型与定价", exact: true })).toHaveCount(0);
    await support.goto(`/manage/${f.channel_id}/customers`);
    if (!journal.takeover_executed) await expect(support.getByRole("link", { name: "客户工作台", exact: true })).toBeVisible();
    await expect(support.getByRole("link", { name: "收款与分配", exact: true })).toHaveCount(0);
    await readonly.goto(`/manage/${f.oem_id}/operators`);
    await expect(readonly.getByRole("button", { name: "建立 OEM / 渠道", exact: true })).toHaveCount(0);
    await expect(readonly.getByRole("button", { name: "建立直属渠道", exact: true })).toHaveCount(0);
    for (const [person, path] of [[finance, `${channelBase}/plans`], [support, `${channelBase}/payments`], [readonly, `${oemBase}/operators`]] as const) {
      const identity = await (await workspaceAPI(person, "/api/v1/auth/me")).json();
      const denied = await workspaceAPI(person, path, { headers: { "X-CSRF-Token": identity.csrf_token, "Idempotency-Key": randomUUID(), Origin: f.origin }, data: { kind: "channel", name: "forbidden role operation", legal_name: "negative", supply_terms: "negative permission check" } });
      expect(denied.status()).toBe(403);
    }
    expect((await workspaceAPI(oem, `${oemBase}/providers`)).status()).toBe(403);
    await readonly.screenshot({ path: info.outputPath("operator-readonly-boundary.png"), fullPage: true });
    save("permissions", { parent_customer_and_assets: 403, sibling_customers: 403, channel_create_descendants: 400, finance_and_support_navigation_restricted: true });

    if (!journal.created_channel) {
      await oem.goto(`/manage/${f.oem_id}/operators`);
      await oem.getByRole("button", { name: "建立直属渠道", exact: true }).click();
      const panel = oem.getByRole("dialog").last();
      await expect(control(panel, "合作类型", false)).toHaveCount(0);
      for (const [name, value] of Object.entries({ name: "UI 新开直属渠道", legal_name: "UI 合成独立经营主体", supply_terms: "按双方确认的供货报价采购，经营配置完成后再开业" })) await field(panel, name, value);
      save("created_channel", await submit(oem, "建立直属渠道", `${oemBase}/operators`));
    }
    if (!journal.owner_invited) {
      await oem.goto(`/manage/${f.oem_id}/operators`);
      const row = oem.getByRole("row").filter({ hasText: "UI 新开直属渠道" });
      await row.getByRole("button", { name: "开通首位负责人", exact: true }).click();
      await field(oem.getByRole("dialog").last(), "delivery_mode", "email");
      await field(oem.getByRole("dialog").last(), "email", "sim-browser-created-owner@operator.test");
      save("owner_invited", await submit(oem, "开通首位负责人", `${oemBase}/operators/${journal.created_channel.id}/invite_owner`));
    }
    if (!journal.owner_accepted) {
      let invitationURL = "";
      await expect.poll(() => {
        const messages = JSON.parse(readFileSync(root.mailbox_file, "utf8"));
        const mail = messages.find((message: any) => (message.To || message.to) === "sim-browser-created-owner@operator.test");
        invitationURL = String(mail?.Text || mail?.text || "").match(/http[^\s]+/)?.[0] || ""; return !!invitationURL;
      }, { timeout: 30_000, intervals: [500, 1000] }).toBe(true);
      const context = await browser.newContext({ baseURL: f.origin }), person = await context.newPage(); contexts.push(context); pages.push(person);
      await person.goto(invitationURL); await control(person, "显示名称", true).fill("UI 新渠道独立负责人");
      await control(person, "经营人员密码", true).fill("simulation-new-owner-password");
      const pending = person.waitForResponse(r => new URL(r.url()).pathname === "/api/v1/auth/staff-invitations/accept" && r.request().method() === "POST");
      await person.getByRole("button", { name: "接受邀请", exact: true }).click();
      const response = await pending; expect(response.status(), await response.text()).toBe(200);
      await login(person, { email: "sim-browser-created-owner@operator.test", password: "simulation-new-owner-password" });
      const session = await (await workspaceAPI(person, "/api/v1/auth/me")).json(); expect(session.workspaces.map((w: any) => w.operator.id)).toEqual([journal.created_channel.id]);
      save("owner_accepted", { independent_workspace_only: true });
    }

    // A fresh channel receives its model permission from the OEM in the UI.
    if (!journal.authorized) {
      await oem.goto(`/manage/${f.oem_id}/pricing`);
      await oem.getByRole("button", { name: "授权与上架", exact: true }).click();
      const panel = oem.getByRole("dialog").last();
      await config(panel, "action", "authorization"); await config(panel, "buyer_id", f.channel_id); await config(panel, "model_id", f.model_id);
      save("authorized", await submit(oem, "授权与上架", `${oemBase}/pricing`));
    }
    if (!journal.listed) {
      await channel.goto(`/manage/${f.channel_id}/pricing`);
      await channel.getByRole("button", { name: "上架与下架", exact: true }).click();
      const panel = channel.getByRole("dialog").last(); await config(panel, "model_id", f.model_id);
      save("listed", await submit(channel, "上架与下架", `${channelBase}/pricing`));
    }
    if (!journal.retail_price) {
      await channel.goto(`/manage/${f.channel_id}/pricing`);
      await channel.getByRole("button", { name: "设置模型价格", exact: true }).click();
      const panel = channel.getByRole("dialog").last();
      await expect(panel.locator('[data-config-path="price.id"]')).toHaveCount(0);
      await config(panel, "price.model_id", f.model_id);
      await config(panel, "price.effective_at", localTime(new Date(Date.now() - 60_000)));
      for (const input of await panel.locator('[data-config-path^="price.rule.components."][data-config-path$=".rate"] input').all()) await input.fill("4000");
      const published = await submit(channel, "设置模型价格", `${channelBase}/pricing`);
      expect(published.id).toMatch(/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i);
      save("retail_price", published);
    }

    if (!journal.quote) {
      await oem.goto(`/manage/${f.oem_id}/procurement`); await oem.getByRole("button", { name: "我方供货", exact: true }).click();
      await oem.getByRole("tab", { name: "供货报价", exact: true }).click();
      save("quote", await operation(oem, "向直属采购方发布供货报价", { buyer_id: f.channel_id, gift_tiers: "50", name: "UI 星河供货报价", terms_summary: "先实际收款再交付采购库存；退出前清算存量服务责任" }, `${oemBase}/procurement`));
    }
    if (!journal.purchase) {
      await channel.goto(`/manage/${f.channel_id}/procurement`);
      await channel.getByRole("tab", { name: "采购报价", exact: true }).click();
      await (await recordRow(channel, journal.quote.id)).getByRole("button", { name: "按金额采购", exact: true }).click();
      await field(channel.getByRole("dialog").last(),"amount_money","80");
      save("purchase", await submit(channel, "按金额采购", `${channelBase}/procurement/${journal.quote.id}/purchase`));
    }
    if (!journal.checkout) {
      await channel.goto(`/manage/${f.channel_id}/procurement`);
      await (await recordRow(channel, journal.purchase.id)).getByRole("button", { name: "发起真实付款", exact: true }).click();
      await field(channel.getByRole("dialog").last(), "merchant_id", f.oem_manual_method_id);
      save("checkout", await submit(channel, "发起真实付款", `${channelBase}/procurement/${journal.purchase.id}/checkout`));
    }
    if (!journal.receipt) {
      await oem.goto(`/manage/${f.oem_id}/payments`);
      save("receipt", await operation(oem, "登记人工核实收款", { provider_receipt_id: "synthetic-operator-ui-" + randomUUID(), settlement_amount: "80", currency: "USD", recipient_reference: "synthetic OEM recipient", payer_reference: "synthetic channel payer", verification_evidence: "SYNTHETIC bank fact for actual browser simulation", paid_at: localTime() }, `${oemBase}/payments`));
    }
    if (!journal.funded) {
      await oem.goto(`/manage/${f.oem_id}/procurement`); await oem.getByRole("button", { name: "我方供货", exact: true }).click();
      await (await recordRow(oem, journal.purchase.id)).getByRole("button", { name: "关联线下收款并交付", exact: true }).click();
      const panel = oem.getByRole("dialog").last(); await field(panel, "receipt_id", journal.receipt.id);
      save("funded", await submit(oem, "关联线下收款并交付", `${oemBase}/procurement/${journal.purchase.id}/fund`));
    }
    if (!journal.delivered) {
      const page = await (await workspaceAPI(channel, `${channelBase}/procurement?section=orders`)).json();
      expect(page.items.find((row:any)=>row.id===journal.purchase.id)?.status).toBe("completed");
      save("delivered",{id:journal.purchase.id,status:"completed"});
    }
    if (!journal.authorization) {
      await channel.goto(`/manage/${f.channel_id}/procurement`);
      await channel.getByRole("tab", { name: "自动补货", exact: true }).click();
      save("authorization", await operation(channel, "设定有限自动补货授权", { quote_id: journal.quote.id, amount_money:"80", trigger_available_atg: "10", period_limit_money: "160", period: "month", mode: "offline" }, `${channelBase}/procurement`));
    }
    if (!journal.revised) {
      await channel.goto(`/manage/${f.channel_id}/procurement`);
      await channel.getByRole("tab", { name: "自动补货", exact: true }).click();
      await (await recordRow(channel, journal.authorization.id)).getByRole("button", { name: "修订本方自动补货授权", exact: true }).click();
      const panel = channel.getByRole("dialog").last(); await field(panel, "period_limit_money", "240");
      save("revised", await submit(channel, "修订本方自动补货授权", `${channelBase}/procurement`));
    }
    if (!journal.cancelled) {
      await channel.goto(`/manage/${f.channel_id}/procurement`);
      await channel.getByRole("tab", { name: "自动补货", exact: true }).click();
      await (await recordRow(channel, journal.authorization.id)).getByRole("button", { name: "停止未来自动补货", exact: true }).click();
      save("cancelled", await submit(channel, "停止未来自动补货", `${channelBase}/procurement/${journal.authorization.id}/cancel_authorization`));
    }
    await channel.screenshot({ path: info.outputPath("operator-procurement-replenishment.png"), fullPage: true });
    if (!journal.exit) {
      await channel.goto(`/manage/${f.channel_id}/exits`);
      await channel.getByRole("button", { name: "通知品牌办理退出", exact: true }).click();
      const panel = channel.getByRole("dialog").last();
      await panel.getByLabel("补充说明（可选）", { exact: true }).fill("synthetic channel exit notice; whole-customer takeover to follow");
      await panel.getByRole("checkbox").check();
      const pending = channel.waitForResponse(r => new URL(r.url()).pathname === `${channelBase}/exits` && r.request().method() === "POST");
      await panel.getByRole("button", { name: "通知品牌办理退出", exact: true }).click();
      const response = await pending;
      expect(response.status()).toBe(200);
      save("exit", await response.json());
    }
    if (!journal.oversell) {
      await oem.goto(`/manage/${f.oem_id}/coverage`);
      await (await recordRow(oem, f.channel_id)).getByRole("button", { name: "设置授权超卖与缓冲", exact: true }).click();
      const panel = oem.getByRole("dialog").last(); await field(panel, "allowance", "5"); await field(panel, "warning", "10");
      save("oversell", await submit(oem, "设置授权超卖与缓冲", `${oemBase}/coverage`));
    }
    await channel.goto(`/manage/${f.channel_id}/coverage`);
    await expect(channel.getByRole("button", { name: "设置授权超卖与缓冲", exact: true })).toHaveCount(0);
    await channel.screenshot({ path: info.outputPath("operator-channel-coverage-readonly.png"), fullPage: true });
    if (!journal.plan) {
      await channel.goto(`/manage/${f.channel_id}/plans`);
      save("plan", await operation(channel, "创建套餐", { name: "星河 UI 开业套餐", price_money: "10", paid_atg: "20", gift_atg: "5", period: "month", terms_summary: "星河渠道客户支持与服务约定", new_purchase_enabled: true, renewal_enabled: false }, `${channelBase}/plans`));
    }
    if (!journal.business_open) {
      for (const feature of ["new_purchase", "model_usage"]) {
        await channel.goto(`/manage/${f.channel_id}/business-status`);
        const label = ({ registration: "新客户注册", new_purchase: "套餐新购", model_usage: "模型调用" } as Record<string,string>)[feature];
        const enable = channel.getByRole("button", { name: "开启 " + label, exact: true });
        const pause = channel.getByRole("button", { name: "暂停 " + label, exact: true });
        await expect(enable.or(pause)).toBeEnabled();
        if (await enable.isVisible()) await enable.click();
        await expect(pause).toBeEnabled();
      }
      save("business_open", true);
    }
    if (!journal.oem_plan) {
      await oem.goto(`/manage/${f.oem_id}/plans`);
      save("oem_plan", await operation(oem, "创建套餐", { name: "山海品牌直营套餐", price_money: "15", paid_atg: "30", gift_atg: "5", period: "month", terms_summary: "山海品牌直营客户支持与服务约定", new_purchase_enabled: true, renewal_enabled: false }, `${oemBase}/plans`));
    }
    if (!journal.oem_business_open) {
      for (const feature of ["registration", "new_purchase", "model_usage"]) {
        await oem.goto(`/manage/${f.oem_id}/business-status`);
        const label = ({ registration: "新客户注册", new_purchase: "套餐新购", model_usage: "模型调用" } as Record<string,string>)[feature];
        const enable = oem.getByRole("button", { name: "开启 " + label, exact: true });
        const pause = oem.getByRole("button", { name: "暂停 " + label, exact: true });
        await expect(enable.or(pause)).toBeEnabled();
        if (await enable.isVisible()) await enable.click();
        await expect(pause).toBeEnabled();
      }
      save("oem_business_open", true);
    }
    await login(customer, f.customer, "customer");
    if (!journal.topup) {
      await customer.goto("/workspace/topups");
      save("topup", await operation(customer, "创建充值订单", { amount_money: "50", external_method_id: f.channel_manual_method_id }, "/api/v1/me/topups"));
    }
    if (!journal.topup_receipt) {
      await channel.goto(`/manage/${f.channel_id}/payments`);
      save("topup_receipt", await operation(channel, "登记人工核实收款", { provider_receipt_id: "synthetic-channel-topup-" + randomUUID(), settlement_amount: "50", currency: "USD", recipient_reference: "synthetic channel recipient", payer_reference: "synthetic customer original payer", verification_evidence: "SYNTHETIC customer topup bank fact", paid_at: localTime() }, `${channelBase}/payments`));
    }
    if (!journal.topup_allocated) {
      await channel.goto(`/manage/${f.channel_id}/orders`);
      await control(channel, "搜索记录").fill(journal.topup.id); await channel.getByRole("button", { name: "查询", exact: true }).click();
      await channel.getByRole("link", { name: "查看并办理", exact: true }).click();
      await expect(channel.getByRole("heading", { name: "订单与收款办理", exact: true })).toBeVisible();
      await expect(channel.getByRole("dialog")).toHaveCount(0);
      await channel.getByRole("button", { name: "关联已核实收款", exact: true }).click();
      const panel = channel.getByRole("dialog").last(); await field(panel, "receipt_id", journal.topup_receipt.id);
      save("topup_allocated", await submit(channel, "关联已核实收款", `${channelBase}/order-context/${journal.topup.id}/allocate`));
    }
    if (!journal.topup_delivered) {
      await channel.goto(`/manage/${f.channel_id}/orders`);
      await control(channel, "搜索记录").fill(journal.topup.id); await channel.getByRole("button", { name: "查询", exact: true }).click();
      await channel.getByRole("link", { name: "查看并办理", exact: true }).click();
      await expect(channel.getByRole("heading", { name: "订单与收款办理", exact: true })).toBeVisible();
      await expect(channel.getByRole("dialog")).toHaveCount(0);
      await channel.getByRole("button", { name: "核实资金并重试交付", exact: true }).click();
      save("topup_delivered", await submit(channel, "核实资金并重试交付", `${channelBase}/orders/${journal.topup.id}/fulfill`));
    }
    if (!journal.customer_order) {
      await customer.goto("/workspace/plans"); await customer.getByRole("button", { name: "选择此套餐", exact: true }).first().click();
      await customer.getByRole("button", { name: "继续，核对成交条款" }).click();
      await customer.getByText("精确调整抵扣金额", { exact: true }).click();
      const context = await (await workspaceAPI(customer, "/api/v1/public/context")).json();
      await control(customer, `本次使用 ${context.monetary.currency} 支付余额`, true).fill("10");
      await customer.getByRole("checkbox", { name: /^我确认本次价格/ }).check();
      const pending = customer.waitForResponse(r => new URL(r.url()).pathname === "/api/v1/me/orders" && r.request().method() === "POST");
      await customer.getByRole("button", { name: "确认购买并核实到账", exact: true }).click();
      const response = await pending, order = await response.json(); expect(response.status(), JSON.stringify(order)).toBe(200); save("customer_order", order);
    }
    if (!journal.customer_key) {
      await customer.goto("/workspace/keys"); await customer.getByRole("button", { name: "创建 Key", exact: true }).click();
      await control(customer, "名称", true).fill("接管前连续使用的应用"); await control(customer, "ATG 预算上限", true).fill("10");
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
      await control(customer, "本次输入", true).fill("Synthetic channel operation journey, preserve the account after takeover.");
      const pending = customer.waitForResponse(r => new URL(r.url()).pathname === "/api/v1/me/playground" && r.request().method() === "POST");
      await customer.getByRole("button", { name: "确认计费并发送", exact: true }).click();
      const response = await pending, data = await response.json(); expect(response.status(), JSON.stringify(data)).toBe(200);
      save("customer_call", { request_id: data.request_id, status: data.status, charged_atg: data.charged_atg });
      await expect(customer.getByText("本次实际扣费", { exact: true })).toBeVisible();
      await customer.screenshot({ path: info.outputPath("operator-customer-real-model-call.png"), fullPage: true });
    }
    const keysBefore = await (await workspaceAPI(customer, "/api/v1/me/keys")).json();
    const originalKey = keysBefore.items.find((key: any) => key.name === "接管前连续使用的应用");
    expect(originalKey).toBeTruthy(); save("customer_key", { id: originalKey.id, name: originalKey.name, budget: originalKey.budget });
    const records = await (await workspaceAPI(customer, "/api/v1/me/requests")).json();
    const record = records.items.find((r: any) => r.id === journal.customer_call.request_id);
    expect(record.components.every((c: any) => c.component === "precision_carry" || c.price_description === "4000 / 1000000 token"), JSON.stringify(record.components)).toBe(true);
    expect(record.charged_atg).toBe("0.068"); save("customer_bill", record);
    const credits = await (await workspaceAPI(customer, "/api/v1/me/credits")).json();
    expect(Number(credits.customer_atg.available)).toBeGreaterThan(24);
    await customer.goto("/workspace/credit-sources"); await customer.screenshot({ path: info.outputPath("operator-customer-delivered-package.png"), fullPage: true });
    if (!journal.risk_rule) {
      await channel.goto(`/manage/${f.channel_id}/alerts`);
      const panel = channel.getByRole("region", {name:"剩余采购余额ATG"});
      await panel.getByLabel("提醒阈值（ATG）").fill("200");
      const saved = channel.waitForResponse(r => new URL(r.url()).pathname === `${channelBase}/alerts` && r.request().method() === "POST");
      await panel.getByRole("button", {name:"保存预警设置"}).click();
      const savedRule = await (await saved).json();
      expect(savedRule.id).toMatch(/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i);
      save("risk_rule", savedRule);
    }
    await expect.poll(async () => {
      const data = await (await workspaceAPI(channel, `${channelBase}/alerts`)).json();
      return data.items.find((item: any) => item.id === journal.risk_rule.id)?.status;
    }, { timeout: 75_000, intervals: [1000, 3000] }).toBe("warning");
    save("risk_observed", { rule: journal.risk_rule.id, measured_warning: true });
    await channel.goto(`/manage/${f.channel_id}/alerts`); await channel.screenshot({ path: info.outputPath("operator-risk-warning.png"), fullPage: true });
    // The brand cannot read the customer's current assets before an explicit
    // reviewed whole-channel takeover. The same account survives afterwards.
    if (!journal.takeover) {
      await oem.goto(`/manage/${f.oem_id}/takeovers`); await oem.getByRole("button", { name: "接管渠道", exact: true }).first().click();
      const panel = oem.getByRole("dialog").last();
      await panel.getByRole("combobox", { name: /^直属渠道/ }).selectOption(f.channel_id);
      await panel.getByRole("textbox", { name: "补充说明（可选）" }).fill("explicit synthetic whole-channel takeover decision");
      await panel.getByRole("checkbox").check();
      const pending = oem.waitForResponse(r => new URL(r.url()).pathname === `${oemBase}/takeovers` && r.request().method() === "POST");
      await panel.getByRole("button", { name: "开始准备", exact: true }).click();
      const response = await pending, data = await response.json(); expect(response.status(), JSON.stringify(data)).toBe(200); save("takeover", data);
    }
    if (!journal.takeover_executed) {
      await oem.goto(`/manage/${f.oem_id}/takeovers`);
      await expect(oem.getByRole("button", { name: "确认接管", exact: true })).toBeVisible();
      const reviewed = await (await workspaceAPI(oem, `${oemBase}/takeovers`)).json();
      expect(reviewed.items.find((item: any) => item.id === journal.takeover.id)?.snapshot.customers[f.customer.id]).toBeGreaterThan(0);
      await oem.screenshot({ path: info.outputPath("operator-takeover-full-snapshot.png"), fullPage: true });
      await oem.getByRole("button", { name: "确认接管", exact: true }).click();
      const panel = oem.getByRole("dialog").last(); await panel.getByRole("checkbox").check();
      const pending = oem.waitForResponse(r => new URL(r.url()).pathname === `${oemBase}/takeovers/${journal.takeover.id}/execute` && r.request().method() === "POST");
      await panel.getByRole("button", { name: "确认提交", exact: true }).click();
      const response = await pending, data = await response.json(); expect(response.status(), JSON.stringify(data)).toBe(200); save("takeover_executed", data);
    }
    await customer.reload(); const me = await (await workspaceAPI(customer, "/api/v1/auth/me")).json(); expect(me.operator.id).toBe(f.oem_id);
    const after = await (await workspaceAPI(customer, "/api/v1/me/credits")).json(); expect(after.customer_atg.available).toBe(credits.customer_atg.available); expect(after.money_balance.available).toBe(credits.money_balance.available);
    const retainedKeys = await (await workspaceAPI(customer, "/api/v1/me/keys")).json();
    expect(retainedKeys.items.find((key: any) => key.id === journal.customer_key.id)?.budget.limit_atg).toBe("10");
    await oem.goto(`/manage/${f.oem_id}/customers`); await expect(oem.getByText("星河真实流程客户", { exact: true })).toBeVisible();
    await oem.goto(`/manage/${f.oem_id}/takeovers`); await oem.screenshot({ path: info.outputPath("operator-takeover-responsibility-preserved.png"), fullPage: true });
    save("takeover_preserved", { current_operator_id: me.operator.id, customer_atg: after.customer_atg.available, customer_money: after.money_balance.available });
    if (!journal.post_takeover_key_call && continuitySecret) {
      const response = await workspaceAPI(customer, "/api/v1/chat/completions", { headers: { Authorization: `Bearer ${continuitySecret}`, "Idempotency-Key": randomUUID() }, data: { model: f.model_id, messages: [{ role: "user", content: "Synthetic original key remains usable after takeover" }], max_completion_tokens: 20 } });
      expect(response.status(), await response.text()).toBe(200);
      const current = await (await workspaceAPI(customer, "/api/v1/me/credits")).json();
      expect(Number(after.customer_atg.available) - Number(current.customer_atg.available)).toBeCloseTo(0.034, 12);
      save("post_takeover_key_call", { original_key_id: journal.customer_key.id, actual_charged_atg: "0.034", current_brand_retail_price: "0.002" });
    }
    await expect(oem.getByRole("button", { name: "登记实际收付款", exact: true })).toHaveCount(0);
    await oem.goto(`/manage/${f.oem_id}/exits`);
    const exitRecords = await (await workspaceAPI(oem, `${oemBase}/exits`)).json();
    expect(exitRecords.items.some((item: any) => item.id === journal.exit.id)).toBe(true);
    await expect(oem.getByRole("button", { name: "按原来源预留本次退款", exact: true })).toHaveCount(0);
    save("exit_verified", { notice_id: journal.exit.id, takeover_complete: true, procurement_refund: false });
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
  for (const current of [oem, buyer]) current.setDefaultTimeout(20_000);
  const base = `/api/v1/operators/${f.other_channel_id}`, supplier = `/api/v1/operators/${f.oem_id}`;
  try {
    await login(oem, f.oem_owner); await login(buyer, f.other_owner);
    if (!journal.authorized) {
      await oem.goto(`/manage/${f.oem_id}/pricing`); await oem.getByRole("button", { name: "授权与上架", exact: true }).click();
      const panel = oem.getByRole("dialog").last(); await config(panel, "action", "authorization"); await config(panel, "buyer_id", f.other_channel_id); await config(panel, "model_id", f.model_id);
      save("authorized", await submit(oem, "授权与上架", `${supplier}/pricing`));
    }
    if (!journal.listed) {
      await buyer.goto(`/manage/${f.other_channel_id}/pricing`); await buyer.getByRole("button", { name: "上架与下架", exact: true }).click();
      await config(buyer.getByRole("dialog").last(), "model_id", f.model_id); save("listed", await submit(buyer, "上架与下架", `${base}/pricing`));
    }
    if (!journal.price) {
      await buyer.goto(`/manage/${f.other_channel_id}/pricing`); await buyer.getByRole("button", { name: "设置模型价格", exact: true }).click();
      const panel = buyer.getByRole("dialog").last();
      await expect(panel.locator('[data-config-path="price.id"]')).toHaveCount(0);
      await config(panel, "price.model_id", f.model_id);
      await config(panel, "price.effective_at", localTime(new Date(Date.now() - 60_000)));
      for (const input of await panel.locator('[data-config-path^="price.rule.components."][data-config-path$=".rate"] input').all()) await input.fill("4000");
      const published = await submit(buyer, "设置模型价格", `${base}/pricing`);
      expect(published.id).toMatch(/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i);
      save("price", published);
    }
    if (!journal.quote) {
      await oem.goto(`/manage/${f.oem_id}/procurement`); await oem.getByRole("button", { name: "我方供货", exact: true }).click();
      await oem.getByRole("tab", { name: "供货报价", exact: true }).click();
      save("quote", await operation(oem, "向直属采购方发布供货报价", { buyer_id: f.other_channel_id, gift_tiers: "100", name: "UI 离线补货报价", terms_summary: "有限补货授权仅创建待付订单，实际收款后才交付" }, `${supplier}/procurement`));
    }
    if (!journal.authorization) {
      await buyer.goto(`/manage/${f.other_channel_id}/procurement`);
      await buyer.getByRole("tab", { name: "自动补货", exact: true }).click();
      save("authorization", await operation(buyer, "设定有限自动补货授权", { quote_id: journal.quote.id, amount_money:"10", trigger_available_atg: "10", period_limit_money: "20", period: "month", mode: "offline" }, `${base}/procurement`));
    }
    let resource: any;
    await expect.poll(async () => {
      resource = await (await workspaceAPI(buyer, `${base}/procurement`)).json();
      return resource.items.find((item: any) => item.values.record_kind === "replenishment_attempt")?.status;
    }, { timeout: 90_000, intervals: [1000, 3000] }).toBe("awaiting_manual_payment");
    const attempts = resource.items.filter((item: any) => item.values.record_kind === "replenishment_attempt");
    expect(attempts).toHaveLength(1);
    const orders = resource.items.filter((item: any) => item.values.record_kind === "order");
    expect(orders).toHaveLength(1); expect(orders[0].status).toBe("awaiting_payment"); expect(orders[0].values.allocated_money).toBe("0");
    const coverage = await (await workspaceAPI(buyer, `${base}/coverage`)).json();
    expect(coverage.items[0].values.procurement_available).toBe("0");
    save("actual_attempt", { attempt_id: attempts[0].id, order_id: orders[0].id, state: attempts[0].status, allocated_money: "0", procurement_available: "0" });
    await buyer.goto(`/manage/${f.other_channel_id}/procurement`);
    await expect((await recordRow(buyer, orders[0].id)).getByRole("button", { name: "发起真实付款", exact: true })).toBeVisible();
    await buyer.screenshot({ path: info.outputPath("offline-replenishment-original-unpaid-order.png"), fullPage: true });
    if (!journal.cancelled) {
      await buyer.getByRole("tab", { name: "自动补货", exact: true }).click();
      await (await recordRow(buyer, journal.authorization.id)).getByRole("button", { name: "停止未来自动补货", exact: true }).click();
      save("cancelled", await submit(buyer, "停止未来自动补货", `${base}/procurement/${journal.authorization.id}/cancel_authorization`));
    }
    const after = await (await workspaceAPI(buyer, `${base}/procurement`)).json();
    expect(after.items.filter((item: any) => item.values.record_kind === "order").map((item: any) => item.id)).toEqual([orders[0].id]);
    await info.attach("actual-offline-replenishment", { body: JSON.stringify(journal, null, 2), contentType: "application/json" });
  } finally { await Promise.all(contexts.map(context => context.close())); }
});
