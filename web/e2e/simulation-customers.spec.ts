import { test, expect, type Page, type BrowserContext } from "@playwright/test";
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { randomUUID } from "node:crypto";

const fixturePath = process.env.TOKENHUB_FRONTEND_ACCEPTANCE_FILE;
test.skip(!fixturePath, "Requires the isolated real application, PostgreSQL and independent HTTP model service");
const fixture = () => JSON.parse(readFileSync(fixturePath!, "utf8"));
type Persona = { id: string; email: string; password: string; invite_code: string };
async function login(page: Page, person: Persona | { email: string; password: string }, staff = false) {
  await page.goto("/login");
  await page.getByLabel("邮箱", { exact: true }).fill(person.email);
  await page.getByLabel("密码", { exact: true }).fill(person.password);
  await page.getByRole("button", { name: "登录", exact: true }).click();
  await expect(page).toHaveURL(staff ? /\/manage\// : /\/workspace/);
}
async function get(page: Page, path: string) {
  const result = await page.request.get("/api/v1" + path);
  expect(result.status(), path).toBe(200);
  return result.json();
}
async function post(page: Page, path: string, data: unknown, key = randomUUID()) {
  const me = await get(page, "/auth/me");
  return page.request.post("/api/v1" + path, { headers: { "X-CSRF-Token": me.csrf_token, "Idempotency-Key": key, Origin: new URL(page.url()).origin }, data });
}
async function createKey(page: Page, name: string, budget: string) {
  await page.goto("/workspace/keys");
  await page.getByRole("button", { name: "创建 Key", exact: true }).click();
  await page.getByLabel("名称", { exact: true }).fill(name);
  await page.getByLabel("ATG 预算上限", { exact: true }).fill(budget);
  const response = page.waitForResponse(response => response.url().endsWith("/api/v1/me/keys") && response.request().method() === "POST");
  await page.getByRole("button", { name: "创建 Key", exact: true }).last().click();
  const http = await response; expect(http.status()).toBe(200); const result = await http.json();
  await expect(page.getByText(result.secret, { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "关闭", exact: true }).click();
  await expect(page.getByText(result.secret, { exact: true })).toHaveCount(0);
  return result;
}
async function purchase(page: Page, plan: string, operatorActivity?: string, kolActivity?: string, renew = false) {
  await page.goto(`/workspace/plans?plan=${plan}`);
  if (operatorActivity) await page.getByLabel("经营方套餐活动", { exact: false }).selectOption(operatorActivity);
  if (kolActivity) await page.getByLabel("直属邀请 KOL 活动", { exact: false }).selectOption(kolActivity);
  const quoted = page.waitForResponse(response => response.url().endsWith("/api/v1/me/orders/quotes") && response.request().method() === "POST");
  await page.getByRole("button", { name: "继续，核对成交条款" }).click();
  const quoteHTTP = await quoted; expect(quoteHTTP.status()).toBe(200); const quote = await quoteHTTP.json();
  await expect(page.getByLabel("本次使用 USD 支付余额", { exact: true })).toHaveValue(quote.amount_usd);
  await expect(page.getByLabel("使用可用佣金补足", { exact: false })).not.toBeChecked();
  await expect(page.getByLabel("授权到期自动续费", { exact: true })).not.toBeChecked();
  if (renew) await page.getByLabel("授权到期自动续费", { exact: true }).check();
  await page.getByLabel("我确认本次价格", { exact: false }).check();
  const ordered = page.waitForResponse(response => response.url().endsWith("/api/v1/me/orders") && response.request().method() === "POST");
  await page.getByRole("button", { name: "确认购买并核实到账", exact: true }).click();
  const orderHTTP = await ordered; const order = await orderHTTP.json(); expect(orderHTTP.status(), JSON.stringify(order)).toBe(200);
  await expect(page.getByText("服务额度已到账，可以开始使用。", { exact: true })).toBeVisible();
  return { order, quote };
}
async function playground(page: Page, model: string) {
  await page.goto("/workspace/playground");
  await page.getByRole("combobox", { name: "公开模型", exact: true }).selectOption(model);
  await page.getByRole("combobox", { name: "能力", exact: true }).selectOption("chat");
  await page.getByLabel("本次输入", { exact: true }).fill("Synthetic customer simulation, count actual HTTP model usage.");
  const response = page.waitForResponse(response => response.url().endsWith("/api/v1/me/playground") && response.request().method() === "POST");
  await page.getByRole("button", { name: "确认计费并发送", exact: true }).click();
  const http = await response; expect(http.status()).toBe(200); const result = await http.json();
  expect(Number(result.charged_atg)).toBeGreaterThan(0);
  await expect(page.getByRole("heading", { name: "本次体验结果", exact: true })).toBeVisible();
  return result;
}
async function resourceAction(page: Page, name: string, fields: Record<string, string>, path: string) {
  await page.getByRole("button", { name, exact: true }).last().click();
  const panel = page.getByRole("dialog").last();
  for (const [label, value] of Object.entries(fields)) {
    const exact = panel.getByLabel(label, { exact: true }).and(panel.locator("input,select,textarea"));
    const field = await exact.count() ? exact : panel.getByLabel(label, { exact: false }).and(panel.locator("input,select,textarea"));
    if (await field.evaluate(element => element.tagName === "SELECT")) { await expect(field).toBeEnabled(); await field.selectOption(value); }
    else await field.fill(value);
  }
  for (const box of await panel.getByRole("checkbox").all()) await box.check();
  const response = page.waitForResponse(response => response.url().endsWith(path) && response.request().method() === "POST");
  await panel.getByRole("button", { name, exact: true }).click();
  const http = await response; const body = await http.json();
  await panel.getByRole("button", { name: "关闭办理面板", exact: true }).click();
  return { status: http.status(), body };
}

test("new customer verifies actual email, preserves selected invitation and cannot re-register into another channel", async ({ browser }, testInfo) => {
  test.setTimeout(150_000);
  const root = fixture(), f = root.personas.customers;
  const email = f.new_email.replace("@", `+${randomUUID().slice(0,8)}@`);
  const context = await browser.newContext(); const page = await context.newPage();
  try {
    await page.goto("/register?invite=INVALID-SYNTHETIC-CODE");
    await expect(page.getByRole("button", { name: "确认归属并注册", exact: true })).toBeDisabled();
    await page.getByLabel("邀请码（可选）", { exact: true }).fill(f.kol.invite_code);
    await expect(page.getByText(/邀请人：合成直属 KOL/)).toBeVisible();
    await page.getByLabel("显示名称", { exact: true }).fill("真实浏览器注册的合成客户");
    await page.getByLabel("邮箱", { exact: true }).fill(email);
    await page.getByLabel("密码", { exact: true }).fill(f.new_password);
    const registration = page.waitForResponse(response => response.url().endsWith("/api/v1/auth/registrations") && response.request().method() === "POST");
    await page.getByRole("button", { name: "确认归属并注册", exact: true }).click();
    expect((await registration).status()).toBe(200);
    let link = "";
    await expect.poll(() => {
      try {
        const mailbox = JSON.parse(readFileSync(join(dirname(fixturePath!), "mailbox.json"), "utf8"));
        const messages = Array.isArray(mailbox) ? mailbox : mailbox.messages;
        const message = messages.find((item: { to: string; text: string }) => item.to === email && item.text.includes("token="));
        link = message?.text.match(/https?:\/\/[^\s]+token=[^\s]+/)?.[0] || "";
        return !!link;
      } catch { return false; }
    }, { timeout: 40_000 }).toBe(true);
    const verifyURL = new URL(link);
    await page.goto(verifyURL.pathname + verifyURL.search);
    for (let i = 0; i < 2; i++) {
      const verified = page.waitForResponse(response => response.url().endsWith("/api/v1/auth/verify") && response.request().method() === "POST");
      await page.getByRole("button", { name: "验证邮箱", exact: true }).click();
      expect((await verified).status()).toBe(200);
    }
    await login(page, { email, password: f.new_password });
    const me = await get(page, "/auth/me"); expect(me.operator.id).toBe(f.operator_id);
    await page.goto(`/register?invite=${f.other_channel_code}`);
    const unchanged = await get(page, "/auth/me"); expect(unchanged.operator.id).toBe(f.operator_id);
    await page.goto("/workspace/referrals");
    await expect(page.getByRole("heading", { name: "成为 KOL，开启推广收益" })).toBeVisible();
    const duplicate = await post(page, "/auth/registrations", { email, password: f.new_password, display_name: "不得重复注册", invite_code: f.other_channel_code });
    expect([403, 409]).toContain(duplicate.status());
    await page.screenshot({ path: testInfo.outputPath("registered-ordinary-customer.png"), fullPage: true });
    await testInfo.attach("registration-facts", { body: JSON.stringify({ customer_id: me.customer.id, operator_id: me.operator.id, verified_twice_without_duplicate: true, duplicate_cross_channel_status: duplicate.status() }), contentType: "application/json" });
  } finally { await context.close(); }
});

test("KOL activity, explicit combination, paid entitlement, shared Key budget, two-hop commissions, withdrawal cancellation and service stop are real business journeys", async ({ browser }, testInfo) => {
  test.setTimeout(300_000);
  const root = fixture(), f = root.personas.customers;
  const contexts: BrowserContext[] = [], errors: string[] = [];
  async function persona(person: Persona, staff = false) { const context = await browser.newContext(); contexts.push(context); const page = await context.newPage(); page.on("pageerror", error => errors.push(error.message)); page.setDefaultTimeout(20_000); await login(page, person, staff); return page; }
  const kol = await persona(f.kol), buyer = await persona(f.buyer), upstream = await persona(f.upstream), other = await persona(f.other_kol);
  const staff = await persona({ id: "", invite_code: "", email: root.owner_email, password: root.owner_password }, true);
  try {
    await kol.goto("/workspace/activities");
    await kol.getByRole("button", { name: "创建推广活动", exact: true }).click();
    await kol.getByLabel("适用套餐", { exact: true }).selectOption(f.plan_id);
    await kol.getByRole("textbox", { name: "本人让利百分点", exact: true }).fill("16");
    await kol.getByLabel("每位客户最多使用次数", { exact: true }).fill("10");
    await kol.getByLabel("我确认活动让利", { exact: false }).check();
    await kol.getByRole("button", { name: "确认发布活动", exact: true }).click();
    await expect(kol.getByText("请填写大于 0 且不超过 15 的让利百分点，最多 16 位小数。", { exact: true })).toBeVisible();
    await kol.getByRole("textbox", { name: "本人让利百分点", exact: true }).fill("10");
    const publishing = kol.waitForResponse(response => response.url().endsWith("/api/v1/me/activities") && response.request().method() === "POST");
    await kol.getByRole("button", { name: "确认发布活动", exact: true }).click();
    const published = await publishing; expect(published.status()).toBe(200); const activity = await published.json();
    await expect(kol.getByText("让利 10 个百分点", { exact: true })).toBeVisible();
    await kol.screenshot({ path: testInfo.outputPath("kol-created-activity.png"), fullPage: true });
    await buyer.goto(`/workspace/plans?plan=${f.plan_id}`);
    await buyer.getByLabel("经营方套餐活动", { exact: false }).selectOption(f.merchant_campaign_id);
    await buyer.getByLabel("直属邀请 KOL 活动", { exact: false }).selectOption(activity.id);
    await expect(buyer.getByText("所选活动未开放组合，请只保留一种。", { exact: true })).toBeVisible();
    await expect(buyer.getByRole("button", { name: "继续，核对成交条款" })).toBeDisabled();
    await staff.goto(`/manage/${f.operator_id}/marketing`);
    const kolCard = staff.locator(`[data-campaign-id="${activity.id}"]`);
    await expect(kolCard.getByRole("button", { name: "修订活动", exact: true })).toHaveCount(0);
    await staff.locator(`[data-campaign-id="${f.merchant_campaign_id}"]`).getByRole("button", { name: "组合 KOL 优惠", exact: true }).click();
    await expect(staff.getByRole("combobox", { name: "同套餐 KOL 活动" })).toBeEnabled();
    await staff.getByRole("combobox", { name: "同套餐 KOL 活动" }).fill(activity.id);
    const combination = staff.waitForResponse(response => response.url().endsWith(`/operators/${f.operator_id}/marketing`) && response.request().method() === "POST");
    await staff.getByRole("button", { name: "组合 KOL 优惠", exact: true }).last().click();
    expect((await combination).status()).toBe(200);
    await staff.getByRole("button", { name: "关闭办理面板", exact: true }).click();
    const wrongKOL = await post(buyer, "/me/orders/quotes", { plan_id: f.plan_id, kol_activity_id: f.other_kol_campaign_id });
    expect([403, 409]).toContain(wrongKOL.status());
    const wrongChannel = await post(buyer, "/me/orders/quotes", { plan_id: f.other_plan_id });
    expect([403, 404, 409]).toContain(wrongChannel.status());
    const owned = await get(other, "/me/activities/workspace"); expect(owned.items.some((item: { id: string }) => item.id === activity.id)).toBe(false);
    const before = await get(buyer, "/me/credits");
    const { order, quote } = await purchase(buyer, f.plan_id, f.merchant_campaign_id, activity.id, true);
    const foreignOrder = await other.request.get(`/api/v1/me/orders/${order.id}`); expect([403, 404]).toContain(foreignOrder.status());
    expect(quote).toMatchObject({ amount_usd: "80", purchase_atg: "80", plan_gift_atg: "20", operator_gift_atg: "20", kol_gift_atg: "10", total_atg: "130" });
    // Delivery uses PostgreSQL's authoritative effective time. Wait for that
    // actual grant to be spendable instead of treating a rendered receipt as
    // proof that a near-boundary asset read has already crossed effective_at.
    await expect.poll(async () => Number((await get(buyer, "/me/credits")).customer_atg.available) - Number(before.customer_atg.available), { timeout: 10_000 }).toBe(130);
    const after = await get(buyer, "/me/credits");
    expect(Number(after.customer_atg.available) - Number(before.customer_atg.available)).toBe(130);
    expect(Number(before.usd_balance.available) - Number(after.usd_balance.available)).toBe(80);
    expect(after.qualification.confirmed_usd).toBe("0");
    const subscriptions = await get(buyer, "/me/plans");
    expect(subscriptions.items).toHaveLength(1); expect(subscriptions.items[0]).toMatchObject({ auto_renew: true, allow_commission: false });
    const key = await createKey(buyer, "共享额度应用 A", "5");
    const tight = await createKey(buyer, "共享额度应用 B", "5");
    expect((await get(buyer, "/me/credits")).customer_atg.available).toBe(after.customer_atg.available);
    const invokeID = randomUUID(), body = { model: f.model_id, messages: [{ role: "user", content: "Synthetic real customer API call" }], max_completion_tokens: 20 };
    const invoke = (secret: string, id = invokeID, data = body) => buyer.request.post("/api/v1/chat/completions", { headers: { Authorization: `Bearer ${secret}`, "Idempotency-Key": id }, data });
    const called = await invoke(key.secret); expect(called.status()).toBe(200);
    const frozen = await get(kol, "/me/credits"), indirectFrozen = await get(upstream, "/me/credits");
    expect(Number(frozen.commission_usd.frozen)).toBeGreaterThan(0); expect(indirectFrozen.commission_usd.frozen).toBe(frozen.commission_usd.frozen);
    const charged = await get(buyer, "/me/credits"); expect(Number(charged.customer_atg.available)).toBeLessThan(Number(after.customer_atg.available));
    const replay = await invoke(key.secret); expect(replay.status()).toBe(200);
    expect((await get(buyer, "/me/credits")).customer_atg.available).toBe(charged.customer_atg.available);
    expect((await invoke(key.secret, invokeID, { ...body, messages: [{ role: "user", content: "different" }] })).status()).toBe(409);
    const secondApplication = await invoke(tight.secret, randomUUID()); expect(secondApplication.status()).toBe(200);
    const sharedAfter = await get(buyer, "/me/credits"); expect(Number(sharedAfter.customer_atg.available)).toBeLessThan(Number(charged.customer_atg.available));
    const keysBeforeEdit = await get(buyer, "/me/keys"), priorBudget = keysBeforeEdit.items.find((item: { id: string }) => item.id === tight.key.id).budget.spent_atg;
    await buyer.goto("/workspace/keys");
    await buyer.locator("section").filter({ has: buyer.getByRole("heading", { name: "共享额度应用 B", exact: true }) }).getByRole("button", { name: "编辑配置", exact: true }).click();
    await buyer.getByLabel("ATG 预算上限", { exact: true }).fill("0.001");
    const budgetEdit = buyer.waitForResponse(response => response.url().endsWith(`/api/v1/me/keys/${tight.key.id}`) && response.request().method() === "PATCH");
    await buyer.getByRole("button", { name: "保存配置", exact: true }).click(); expect((await budgetEdit).status()).toBe(200);
    const keysAfterEdit = await get(buyer, "/me/keys"); expect(keysAfterEdit.items.find((item: { id: string }) => item.id === tight.key.id).budget.spent_atg).toBe(priorBudget);
    const refused = await invoke(tight.secret, randomUUID()); expect([402, 403, 409, 429]).toContain(refused.status());
    expect((await get(buyer, "/me/credits")).customer_atg.available).toBe(sharedAfter.customer_atg.available);
    const experience = await playground(buyer, f.model_id);
    await expect.poll(async () => (await get(kol, "/me/credits")).commission_usd.frozen, { timeout: 25_000 }).toBe("0");
    const released = await get(kol, "/me/credits"); expect(Number(released.commission_usd.available)).toBeGreaterThan(0);
    const commissions = await get(kol, "/me/commissions"), indirect = await get(upstream, "/me/commissions");
    expect(commissions.items).toHaveLength(3); expect(indirect.items).toHaveLength(3);
    for (const item of commissions.items) { expect(item.hop).toBe(1); expect(Number(item.amount_usd)).toBeCloseTo(Number(item.basis_usd) * 0.05, 12); }
    for (const item of indirect.items) { expect(item.hop).toBe(2); expect(Number(item.amount_usd)).toBeCloseTo(Number(item.basis_usd) * 0.05, 12); }
    await kol.goto("/workspace/commissions");
    await expect(kol.getByText("有偿消费基数", { exact: true })).toHaveCount(3);
    await kol.screenshot({ path: testInfo.outputPath("confirmed-two-hop-commissions.png"), fullPage: true });
    const walletBefore = await get(kol, "/me/credits");
    await kol.goto("/workspace/withdrawals");
    const payoutAmount = walletBefore.commission_usd.available;
    expect((await resourceAction(kol, "核算提现费用", { "提现总额 USD": payoutAmount }, "/api/v1/me/withdrawals/quotes")).status).toBe(200);
    const submitted = await resourceAction(kol, "确认费用并提交", { "本人真实收款资料": "SYNTHETIC ONLY: isolated customer's own payout reference" }, "/confirm");
    expect(submitted.status).toBe(200);
    const held = await get(kol, "/me/credits"); expect(held.commission_usd.held).toBe(payoutAmount);
    expect(held.commission_usd.available).toBe("0");
    expect((await resourceAction(kol, "取消提现申请", {}, `/api/v1/me/withdrawals/${submitted.body.id}/cancel`)).status).toBe(200);
    const restored = await get(kol, "/me/credits"); expect(restored.commission_usd.held).toBe("0"); expect(restored.commission_usd.available).toBe(walletBefore.commission_usd.available);
    await kol.goto("/workspace/account");
    expect((await resourceAction(kol, "开始注销清算", { "本人注销申请说明": "本人明确退出后续推广分佣，既有正确佣金继续结清" }, "/api/v1/me/account/lifecycle/begin-close")).status).toBe(200);
    expect((await get(kol, "/me/credits")).commission_usd.available).toBe(restored.commission_usd.available);
    const afterExitCall = await invoke(key.secret, randomUUID()); expect(afterExitCall.status()).toBe(200);
    const afterExitDirect = await get(kol, "/me/commissions"), afterExitIndirect = await get(upstream, "/me/commissions");
    expect(afterExitDirect.items).toHaveLength(3); expect(afterExitIndirect.items).toHaveLength(4);
    expect((await get(kol, "/me/referrals")).function_status).toBe("terminated");
    const cannotEraseCommission = await resourceAction(kol, "核实清算并完成注销", { "本人确认清算完成": "验证有未结佣金时不得完成注销" }, "/api/v1/me/account/lifecycle/complete-close");
    expect(cannotEraseCommission.status).toBe(409);
    expect((await get(kol, "/me/credits")).commission_usd.available).toBe(restored.commission_usd.available);
    await buyer.goto("/workspace/notifications");
    await buyer.getByLabel("本人授权营销邮件（可随时撤回）", { exact: true }).uncheck();
    const pref = buyer.waitForResponse(response => response.url().endsWith("/api/v1/me/notifications/preferences") && response.request().method() === "POST");
    await buyer.getByRole("button", { name: "保存邮件授权", exact: true }).click(); expect((await pref).status()).toBe(200);
    expect((await get(buyer, "/me/notifications/preferences")).marketing).toBe(false);
    await staff.goto(`/manage/${f.operator_id}/notifications`);
    await staff.getByRole("button", { name: "发送业务通知", exact: true }).click();
    const noticePanel = staff.getByRole("dialog").last();
    const recipient = noticePanel.locator('[data-command-field="recipient_id"]');
    await expect(recipient.getByRole("combobox")).toBeEnabled();
    await recipient.getByRole("combobox").fill(f.buyer.email);
    await staff.getByRole("option").filter({ hasText: f.buyer.email }).click();
    await noticePanel.getByLabel("通知类别", { exact: false }).and(noticePanel.locator("input,select,textarea")).selectOption("mandatory");
    const necessaryTitle = "已退订营销后仍收到的必要履约通知 " + randomUUID().slice(0,8);
    await noticePanel.getByLabel("标题", { exact: false }).and(noticePanel.locator("input,select,textarea")).fill(necessaryTitle);
    await noticePanel.getByLabel("通知正文", { exact: false }).and(noticePanel.locator("input,select,textarea")).fill("合成订单履约核对通知，必要业务授权保留。");
    const noticed = staff.waitForResponse(response => response.url().endsWith(`/api/v1/operators/${f.operator_id}/notifications`) && response.request().method() === "POST");
    await noticePanel.getByRole("button", { name: "发送业务通知", exact: true }).click();
    const noticedHTTP = await noticed; expect(noticedHTTP.status()).toBe(200); const notice = await noticedHTTP.json();
    await noticePanel.getByRole("button", { name: "关闭办理面板", exact: true }).click();
    await buyer.reload(); await expect(buyer.getByRole("heading", { name: necessaryTitle, exact: true })).toBeVisible();
    const notifications = await get(buyer, "/me/notifications"); expect(notifications.items.find((item: { id: string }) => item.id === notice.id).necessary).toBe(true);
    const read = buyer.waitForResponse(response => response.url().endsWith(`/api/v1/me/notifications/${notice.id}/read`) && response.request().method() === "POST");
    await buyer.locator("section").filter({ has: buyer.getByRole("heading", { name: necessaryTitle, exact: true }) }).getByRole("button", { name: "标为已读", exact: true }).click();
    expect((await read).status()).toBe(200);
    const assetsBeforeStop = await get(buyer, "/me/credits");
    await buyer.goto("/workspace/account");
    expect((await resourceAction(buyer, "停止使用服务", { "本人停止使用说明": "本人暂停模型使用；请保留已购买资产" }, "/api/v1/me/account/lifecycle/stop")).status).toBe(200);
    const stoppedAssets = await get(buyer, "/me/credits"); expect(stoppedAssets.customer_atg).toEqual(assetsBeforeStop.customer_atg); expect(stoppedAssets.usd_balance).toEqual(assetsBeforeStop.usd_balance);
    expect((await get(buyer, "/me/plans")).items[0].auto_renew).toBe(false);
    const stoppedKey = await invoke(key.secret, randomUUID()); expect(stoppedKey.status()).toBe(401);
    expect((await stoppedKey.json()).error.code).toBe("invalid_api_key");
    expect((await resourceAction(buyer, "恢复使用", { "本人恢复使用说明": "本人恢复使用，稍后自行重新授权 Key" }, "/api/v1/me/account/lifecycle/resume")).status).toBe(200);
    const keys = await get(buyer, "/me/keys"); expect(keys.items.filter((item: { status: string }) => item.status === "active")).toHaveLength(0);
    expect((await get(buyer, "/me/plans")).items[0].auto_renew).toBe(false);
    expect((await resourceAction(buyer, "开始注销清算", { "本人注销申请说明": "本人申请清算，保留购买ATG和现金余额，不作放弃" }, "/api/v1/me/account/lifecycle/begin-close")).status).toBe(200);
    expect((await resourceAction(buyer, "核实清算并完成注销", { "本人确认清算完成": "验证剩余购买资产和现金不能点击抹去" }, "/api/v1/me/account/lifecycle/complete-close")).status).toBe(409);
    const retainedAssets = await get(buyer, "/me/credits"); expect(retainedAssets.customer_atg).toEqual(assetsBeforeStop.customer_atg); expect(retainedAssets.usd_balance).toEqual(assetsBeforeStop.usd_balance);
    expect(errors).toEqual([]);
    await testInfo.attach("customer-business-facts", { body: JSON.stringify({ order_id: order.id, total_atg: quote.total_atg, api_replay_preserved_balance: true, budget_rejected_status: refused.status(), playground_request_id: experience.request_id, direct_commissions: commissions.items.map((item: { amount_usd: string; basis_usd: string; status: string }) => ({ amount_usd: item.amount_usd, basis_usd: item.basis_usd, status: item.status })), withdrawal_id: submitted.body.id, withdrawal_cancel_restored_exact_balance: true, exited_direct_kol_no_new_commission_indirect_continues: true, stop_preserved_assets_cancelled_renewal_and_keys: true, closure_rejected_while_cash_or_purchased_assets_remain: true }), contentType: "application/json" });
  } finally { for (const context of contexts) await context.close(); }
});

test("ordinary inviter cannot publish KOL promotion and gets no commission from invited customer's paid usage", async ({ browser }, testInfo) => {
  test.setTimeout(150_000);
  const f = fixture().personas.customers;
  const ordinaryContext = await browser.newContext(), childContext = await browser.newContext();
  const ordinary = await ordinaryContext.newPage(), child = await childContext.newPage();
  try {
    await login(ordinary, f.ordinary); await login(child, f.ordinary_child);
    await ordinary.goto("/workspace/activities");
    await expect(ordinary.getByText("当前尚未取得可用的 KOL 推广资格。", { exact: false })).toBeVisible();
    await expect(ordinary.getByRole("button", { name: "创建推广活动", exact: true })).toHaveCount(0);
    const invalid = await post(ordinary, "/me/activities", { configuration: { package_id: f.plan_id, concession_rate: "0.1", gift_atg: "0", eligibility: "any", starts_at: new Date(Date.now()-60000).toISOString(), ends_at: new Date(Date.now()+3600000).toISOString(), per_customer_limit: 1, enabled: true } });
    expect([403, 409]).toContain(invalid.status());
    const { quote } = await purchase(child, f.plan_id);
    expect(quote.total_atg).toBe("100");
    const used = await playground(child, f.model_id);
    const commissions = await get(ordinary, "/me/commissions"); expect(commissions.items).toHaveLength(0);
    const wallet = await get(ordinary, "/me/credits"); expect(wallet.commission_usd.available).toBe("0"); expect(wallet.commission_usd.frozen).toBe("0");
    await expect.poll(async () => Number((await get(child, "/me/credits")).qualification.confirmed_usd), { timeout: 20_000 }).toBeGreaterThan(0);
    const childQualification = (await get(child, "/me/credits")).qualification;
    expect(childQualification.qualified_at).toBeNull(); expect(Number(childQualification.confirmed_usd)).toBeLessThan(60);
    await ordinary.goto("/workspace/referrals"); await ordinary.screenshot({ path: testInfo.outputPath("ordinary-inviter-without-commission.png"), fullPage: true });
    await testInfo.attach("ordinary-customer-facts", { body: JSON.stringify({ denied_activity_status: invalid.status(), paid_usage_request_id: used.request_id, ordinary_commissions: 0, child_confirmed_usd: childQualification.confirmed_usd, child_remains_ordinary: true }), contentType: "application/json" });
  } finally { await ordinaryContext.close(); await childContext.close(); }
});
