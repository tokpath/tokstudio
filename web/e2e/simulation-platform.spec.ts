import { expect, test, type Locator, type Page, type Response, type TestInfo } from "@playwright/test";
import { readFileSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { randomUUID } from "node:crypto";

const fixturePath = process.env.TOKENHUB_FRONTEND_ACCEPTANCE_FILE;
test.skip(!fixturePath, "Requires the isolated application and independent HTTP model service");

async function login(page: Page, account: { email: string; password: string }, kind = "staff") {
  await page.goto("/login");
  await page.getByRole("radio", { name: kind === "staff" ? "经营人员" : "客户", exact: true }).check();
  await page.getByLabel("邮箱", { exact: true }).fill(account.email);
  await page.getByLabel("密码", { exact: true }).fill(account.password);
  await page.getByRole("button", { name: "登录", exact: true }).click();
  await expect(page).toHaveURL(kind === "staff" ? /\/manage\// : /\/workspace/);
}

async function panel(page: Page, label: string, row?: Locator) {
  await (row || page).getByRole("button", { name: label, exact: true }).click();
  const dialog = page.getByRole("dialog").last();
  for (const details of await dialog.locator("details:not([open]) > summary").all()) await details.click();
  return dialog;
}
async function submit(page: Page, dialog: Locator, label: string, path: string): Promise<Response> {
  const confirmation = dialog.locator('input[type="checkbox"]:not([id]):not([aria-label])');
  if (await confirmation.count()) await confirmation.check();
  const pending = page.waitForResponse(response => new URL(response.url()).pathname === path && response.request().method() === "POST");
  await dialog.getByRole("button", { name: label, exact: true }).click();
  const result = await pending;
  expect(result.status(), await result.text()).toBe(200);
  await expect(dialog.getByRole("status")).toBeVisible();
  return result;
}
async function close(dialog: Locator) { await dialog.getByRole("button", { name: "关闭办理面板", exact: true }).click(); }
async function config(dialog: Locator, value: unknown) { await dialog.getByLabel("完整技术配置 JSON", { exact: true }).fill(JSON.stringify(value)); }
const localTime = (date: Date) => new Date(date.getTime() - date.getTimezoneOffset() * 60_000).toISOString().slice(0, 16);
async function evidence(info: TestInfo, name: string, facts: unknown) {
  const path = info.outputPath(`${name}.json`); writeFileSync(path, JSON.stringify(facts, null, 2));
  await info.attach(name, { path, contentType: "application/json" });
}
async function replay(page: Page, response: Response, fresh = false) {
  const request = response.request(), original = request.headers();
  return page.request.post(request.url(), { data: request.postDataJSON(), headers: { "X-CSRF-Token": original["x-csrf-token"], "Idempotency-Key": fresh ? randomUUID() : original["idempotency-key"] } });
}

test("technical administrator configures a real provider, verified model and route; owner prices and authorizes it; customer invokes it", async ({ browser }, testInfo) => {
  test.setTimeout(420_000);
  const fixture = JSON.parse(readFileSync(fixturePath!, "utf8")), p = fixture.personas.platform;
  const techContext = await browser.newContext(), ownerContext = await browser.newContext(), customerContext = await browser.newContext();
  const tech = await techContext.newPage(), owner = await ownerContext.newPage(), customer = await customerContext.newPage();
  const errors: string[] = []; for (const page of [tech, owner, customer]) { page.setDefaultTimeout(20_000); page.on("pageerror", error => errors.push(error.message)); }
  const model = `simulation-${randomUUID()}`, route = randomUUID(), base = `/api/v1/operators/${p.operator_id}`;
  const cap = { operation: "chat", streaming: false, input_types: ["text"], parameters: ["max_completion_tokens"], required_parameters: ["max_completion_tokens"], output_limit_parameter: "max_completion_tokens", max_input_bytes: 10000, max_output: 1000, billable_bounds: { input: "1000", output: "1000" }, usage_rules: [{ component: "input", unit: "token", path: "/prompt_tokens", zero_when_absent: false }, { component: "output", unit: "token", path: "/completion_tokens", zero_when_absent: false }], evidence_id: `independent-protocol:${p.lab_url}`, verified_at: new Date().toISOString(), parameters_verified: true, usage_verified: true, bounds_verified: true, recovery_verified: true };
  try {
    await login(tech, p.tech);
    await tech.getByRole("link", { name: "平台技术", exact: true }).click();
    await expect(tech.getByRole("navigation", { name: "平台技术内导航", exact: true }).getByRole("link", { name: "模型供应商", exact: true })).toBeVisible();
    await expect(tech.getByRole("link", { name: "收款方式", exact: true })).toHaveCount(0);
    await tech.goto(`/manage/${p.operator_id}/providers`);
    let dialog = await panel(tech, "创建或轮换凭据");
    await dialog.getByLabel("新的供应商密钥", { exact: false }).fill("isolated-simulation-provider-key");
    await dialog.getByLabel("启用", { exact: false }).check();
    const credentialResponse = await submit(tech, dialog, "创建或轮换凭据", `${base}/providers`);
    const credential = await credentialResponse.json(); await close(dialog);
    const credentials = await (await tech.request.get(`${base}/providers`)).json();
    const provider = credentials.items.find((row: any) => row.id === credential.id).values.provider_id;
    expect(JSON.stringify(credentials)).not.toContain("isolated-simulation-provider-key");
    dialog = await panel(tech, "轮换凭据", tech.locator("section.rounded-card").filter({ hasText: credential.id }));
    await dialog.getByLabel("新的供应商密钥", { exact: false }).fill("isolated-simulation-provider-key-rotated");
    const rotated = await submit(tech, dialog, "创建或轮换凭据", `${base}/providers`);
    expect((await rotated.json()).version).toBe(2);
    expect((await replay(tech, rotated)).status()).toBe(200);
    expect((await replay(tech, rotated, true)).status()).toBe(409);
    await close(dialog);

    await tech.goto(`/manage/${p.operator_id}/catalog`);
    dialog = await panel(tech, "维护公开模型与能力证据");
    await config(dialog, { model: { id: model, identity: model, display_name: "平台仿真聊天模型", enabled: false, capabilities: [cap] }, expected_version: 0 });
    await submit(tech, dialog, "维护公开模型与能力证据", `${base}/catalog`); await close(dialog);
    dialog = await panel(tech, "编辑模型", tech.locator("section.rounded-card").filter({ hasText: model }));
    await expect(dialog.getByLabel("公开模型标识", { exact: false })).toHaveValue(model);
    await dialog.getByLabel("客户看到的模型名称", { exact: false }).fill("平台仿真聊天模型（已修订）");
    await submit(tech, dialog, "维护公开模型与能力证据", `${base}/catalog`); await close(dialog);

    await tech.goto(`/manage/${p.operator_id}/routes`);
    dialog = await panel(tech, "维护已验证路由");
    await config(dialog, { upstream: { id: route, public_model_id: model, identity: model, provider_id: provider, provider: "openai", model: "test-chat", credential_id: credential.id, enabled: true, healthy: true, priority: 10, endpoints: { chat: `${p.lab_url}/v1/chat/completions` }, capabilities: [cap] }, expected_version: 0 });
    await submit(tech, dialog, "维护已验证路由", `${base}/routes`); await close(dialog);
    dialog = await panel(tech, "编辑路由", tech.locator("section.rounded-card").filter({ hasText: model }));
    await dialog.getByLabel("选路优先级", { exact: false }).fill("11");
    await submit(tech, dialog, "维护已验证路由", `${base}/routes`); await close(dialog);
    dialog = await panel(tech, "设置并发、熔断与恢复", tech.locator("section.rounded-card").filter({ hasText: model }));
    for (const [label, value] of [["此上游最大同时在途", "5"], ["连续失败熔断阈值", "3"], ["未知用量熔断阈值", "2"], ["恢复所需成功探测次数", "2"], ["熔断冷却秒数", "60"]]) await dialog.getByLabel(label, { exact: false }).fill(value);
    await submit(tech, dialog, "设置并发、熔断与恢复", `${base}/routes/${route}/limits`); await close(dialog);
    const routes = await (await tech.request.get(`${base}/routes`)).json(), savedRoute = routes.items.find((row: any) => row.id === route);
    expect(savedRoute.version).toBe(3); expect(savedRoute.values.priority).toBe(11); expect(savedRoute.values.credential_id).toBe(credential.id);

    await tech.goto(`/manage/${p.operator_id}/catalog`);
    dialog = await panel(tech, "编辑模型", tech.locator("section.rounded-card").filter({ hasText: model }));
    await dialog.getByRole("checkbox", { name: "开放模型", exact: false }).check();
    await submit(tech, dialog, "维护公开模型与能力证据", `${base}/catalog`); await close(dialog);
    await tech.screenshot({ path: testInfo.outputPath("platform-catalog.png"), fullPage: true });
    expect((await tech.request.get(`${base}/payments`)).status()).toBe(403);

    await login(owner, { email: fixture.owner_email, password: fixture.owner_password });
    await owner.goto(`/manage/${p.operator_id}/catalog-changes`);
    const changePage = await (await owner.request.get(`${base}/catalog-changes`)).json();
    let lead = Number(changePage.summary.find((item: any) => item.label === "通知提前秒数")?.value || 0);
    if (!lead) {
      dialog = await panel(owner, "设置提前通知规则");
      await dialog.getByLabel("提前通知秒数", { exact: false }).fill("60");
      await dialog.getByLabel("对客户公开的通知条款依据", { exact: false }).fill("合成仿真正常上新：至少提前60秒发送真实通知后才生效");
      await submit(owner, dialog, "设置提前通知规则", `${base}/catalog-changes`); await close(dialog); lead = 60;
    }
    const effective = new Date(Math.ceil((Date.now() + (lead + 75) * 1000) / 60_000) * 60_000);
    await owner.goto(`/manage/${p.operator_id}/pricing`);
    dialog = await panel(owner, "授权或上架模型");
    await dialog.getByRole("combobox", { name: "公开模型", exact: true }).selectOption(model);
    await dialog.getByLabel("计划生效时间", { exact: true }).fill(localTime(effective));
    await dialog.getByLabel("变更及通知依据", { exact: true }).fill("合成仿真：新模型上架提前通知已有客户");
    await submit(owner, dialog, "授权或上架模型", `${base}/pricing`); await close(dialog);
    dialog = await panel(owner, "授权或上架模型");
    await dialog.getByRole("combobox", { name: "办理事项", exact: true }).selectOption("authorization");
    await dialog.getByRole("combobox", { name: "被授权的直属下级", exact: true }).selectOption(p.channel_id);
    await dialog.getByRole("combobox", { name: "公开模型", exact: true }).selectOption(model);
    await submit(owner, dialog, "授权或上架模型", `${base}/pricing`); await close(dialog);
    for (const scope of [{ kind: "retail", seller_id: p.operator_id }, { kind: "supplier", seller_id: provider, buyer_id: route }, { kind: "supply", seller_id: p.operator_id, buyer_id: p.channel_id }]) {
      dialog = await panel(owner, "发布价格版本");
      await config(dialog, { action: scope.kind === "supplier" ? "bootstrap_price" : "price", price: { id: randomUUID(), scope, model_id: model, layer: "regular", effective_at: scope.kind === "supplier" ? new Date(Date.now() - 60_000).toISOString() : effective.toISOString(), rule: { mode: "manual", components: [{ key: "input", unit: "token", per: "1", rate: "0.001" }, { key: "output", unit: "token", per: "1", rate: "0.001" }] }, notice_id: "independent HTTP protocol simulation terms", published: true }, evidence: "isolated activation with actual advance notice" });
      await submit(owner, dialog, "发布价格版本", `${base}/pricing`); await close(dialog);
    }

    await owner.goto(`/manage/${p.operator_id}/business-status`);
    const business = await (await owner.request.get(`${base}/business-status`)).json();
    if (!business.items[0].values.activity) {
      dialog = await panel(owner, "提交开业配置检查");
      await dialog.getByRole("combobox", { name: "营业项目", exact: false }).selectOption("activity");
      await dialog.getByLabel("通过真实依赖检查后开放", { exact: false }).check();
      await dialog.getByLabel("检查及授权依据", { exact: false }).fill("合成仿真：独立赠送的活动规则、真实覆盖及已配置邮件已核实");
      await submit(owner, dialog, "提交开业配置检查", `${base}/business-status`); await close(dialog);
    }
    await owner.goto(`/manage/${p.operator_id}/gifts`);
    dialog = await panel(owner, "核实并发放独立赠送");
    await dialog.getByRole("combobox", { name: "本经营方当前客户", exact: false }).selectOption(p.customer.id);
    await dialog.getByRole("combobox", { name: "明确赠送原因", exact: false }).selectOption("targeted_gift");
    await dialog.getByLabel("本次新增赠送ATG", { exact: false }).fill("10");
    await dialog.getByLabel("授权、事实和原事项关联依据", { exact: false }).fill("合成仿真：为真实模型协议调用发放独立赠送，不涉及现金或佣金");
    await submit(owner, dialog, "核实并发放独立赠送", `${base}/gifts`); await close(dialog);

    await login(customer, p.customer, "customer");
    await expect.poll(async () => {
      const models = await (await customer.request.get("/api/v1/me/models")).json();
      return models.items?.some((item: any) => item.id === model && item.status === "active");
    }, { timeout: Math.max(120_000, effective.getTime() - Date.now() + 120_000), intervals: [5000] }).toBe(true);
    await customer.goto("/workspace/keys");
    await customer.getByRole("button", { name: "创建 Key", exact: true }).click();
    await customer.getByLabel("名称", { exact: true }).fill("平台仿真应用");
    await customer.getByLabel("ATG 预算上限").fill("5");
    const keyPending = customer.waitForResponse(response => response.url().endsWith("/api/v1/me/keys") && response.request().method() === "POST");
    await customer.getByRole("button", { name: "创建 Key", exact: true }).last().click();
    const keyResponse = await keyPending; expect(keyResponse.status()).toBe(200); const key = await keyResponse.json();
    await customer.getByRole("button", { name: "关闭", exact: true }).click();
    const before = await (await customer.request.get("/api/v1/me/credits")).json(), action = randomUUID();
    const invoke = () => customer.request.post("/api/v1/chat/completions", { headers: { Authorization: `Bearer ${key.secret}`, "Idempotency-Key": action }, data: { model, messages: [{ role: "user", content: "独立真实HTTP协议仿真" }], max_completion_tokens: 20 } });
    const result = await invoke(); expect(result.status(), await result.text()).toBe(200); expect((await result.json()).choices.length).toBeGreaterThan(0);
    const requestID = result.headers()["x-tokenhub-request-id"];
    expect(requestID).toBeTruthy();
    const requestFact = await (await customer.request.get(`/api/v1/me/requests/${requestID}`)).json();
    expect(requestFact.model_id).toBe(model);
    const charged = await (await customer.request.get("/api/v1/me/credits")).json();
    expect(Number(charged.customer_atg.available)).toBeLessThan(Number(before.customer_atg.available));
    expect((await invoke()).status()).toBe(200);
    const repeated = await (await customer.request.get("/api/v1/me/credits")).json(); expect(repeated.customer_atg.available).toBe(charged.customer_atg.available);
    await customer.goto("/workspace/requests"); await expect(customer.getByText(model, { exact: true }).first()).toBeVisible();
    await customer.screenshot({ path: testInfo.outputPath("platform-customer-call.png"), fullPage: true });
    expect(errors).toEqual([]);
    await evidence(testInfo, "platform-model-facts", { model, route, credential_id: credential.id, route_version: 3, credential_version: 2, request_id: requestID, replay_did_not_charge_again: true, customer_charged_atg: requestFact.charged_atg });
  } catch (error) {
    await testInfo.attach("technical-ui-at-failure", { body: await tech.locator("body").ariaSnapshot(), contentType: "text/plain" });
    await testInfo.attach("owner-ui-at-failure", { body: await owner.locator("body").ariaSnapshot(), contentType: "text/plain" });
    throw error;
  } finally { await techContext.close(); await ownerContext.close(); await customerContext.close(); }
});

test("brand administrator edits OEM and channel appearance, uploads a logo and keeps an unverified domain inactive", async ({ browser }, testInfo) => {
  test.setTimeout(180_000);
  const fixture = JSON.parse(readFileSync(fixturePath!, "utf8")), p = fixture.personas.platform, base = `/api/v1/operators/${p.operator_id}`;
  const context = await browser.newContext(), page = await context.newPage(); page.setDefaultTimeout(20_000);
  try {
    await login(page, p.brand);
    await page.goto(`/manage/${p.operator_id}/branding`);
    const oem = page.getByRole("row").filter({ hasText: "平台品牌仿真OEM" });
    let dialog = await panel(page, "编辑网站外观", oem);
    await dialog.getByLabel("主色（留空继承", { exact: false }).fill("#3159A8");
    await dialog.getByLabel("客户支持URL", { exact: false }).fill("https://support.example.test/platform-simulation");
    const saved = await submit(page, dialog, "编辑网站外观", `${base}/branding`); await close(dialog);
    expect((await replay(page, saved)).status()).toBe(200); expect((await replay(page, saved, true)).status()).toBe(409);
    dialog = await panel(page, "编辑网站外观", oem);
    await expect(dialog.getByLabel("主色（留空继承", { exact: false })).toHaveValue("#3159A8");
    await dialog.getByLabel("背景色（留空继承）", { exact: false }).fill("#F4F6FB");
    await submit(page, dialog, "编辑网站外观", `${base}/branding`); await close(dialog);
    dialog = await panel(page, "上传目标Logo", oem);
    const logoBytes = await page.screenshot({ clip: { x: 0, y: 0, width: 32, height: 32 } });
    await dialog.locator('input[type="file"]').setInputFiles({ name: "simulation-logo.png", mimeType: "image/png", buffer: logoBytes });
    const logo = await (await submit(page, dialog, "上传目标Logo", `${base}/branding/logo`)).json(); await close(dialog);
    dialog = await panel(page, "编辑网站外观", oem);
    await dialog.getByLabel("已上传Logo地址", { exact: false }).fill(logo.asset.url);
    await submit(page, dialog, "编辑网站外观", `${base}/branding`); await close(dialog);
    const brands = await (await page.request.get(`${base}/branding`)).json(), branded = brands.items.find((row: any) => row.id === p.oem_id);
    expect(branded.values.effective_theme.primary_color).toBe("#3159A8"); expect(branded.values.effective_theme.logo_url).toBe(logo.asset.url);

    await page.goto(`/manage/${p.operator_id}/channel-themes`);
    const channel = page.getByRole("row").filter({ hasText: "平台品牌仿真渠道" });
    dialog = await panel(page, "编辑网站外观", channel);
    await dialog.getByLabel("主色（留空继承", { exact: false }).fill("#9B3C5C");
    await submit(page, dialog, "编辑网站外观", `${base}/channel-themes`); await close(dialog);
    dialog = await panel(page, "编辑网站外观", channel);
    await expect(dialog.getByLabel("主色（留空继承", { exact: false })).toHaveValue("#9B3C5C");
    await dialog.getByLabel("主色（留空继承", { exact: false }).fill("");
    await submit(page, dialog, "编辑网站外观", `${base}/channel-themes`); await close(dialog);
    const channels = await (await page.request.get(`${base}/channel-themes`)).json(), inherited = channels.items.find((row: any) => row.id === p.channel_id);
    expect(inherited.values.primary_color).toBeFalsy();
    const publicContext = await (await page.request.get("/api/v1/public/context")).json();
    expect(inherited.values.effective_theme.primary_color).toBe(publicContext.theme.primary_color);

    await page.goto(`/manage/${p.operator_id}/domains`);
    dialog = await panel(page, "申请域名验证");
    await dialog.getByRole("combobox", { name: "目标品牌经营方", exact: false }).selectOption(p.oem_id);
    const hostname = `simulation-${randomUUID()}.example.test`;
    await dialog.getByLabel("品牌域名", { exact: false }).fill(hostname);
    const domain = await (await submit(page, dialog, "申请域名验证", `${base}/domains`)).json(); await close(dialog);
    const domainRow = page.getByRole("row").filter({ hasText: hostname });
    await expect(domainRow.getByRole("button", { name: "启用已验证域名" })).toHaveCount(0);
    dialog = await panel(page, "检查DNS与TLS", domainRow);
    const checked = await (await submit(page, dialog, "检查DNS与TLS", `${base}/domains/${domain.id}/verify`)).json();
    expect(checked.status).toBe("failed"); await close(dialog);
    await expect(domainRow.getByRole("button", { name: "启用已验证域名" })).toHaveCount(0);
    expect((await page.request.get(`/api/v1/operators/${p.oem_id}/customers`)).status()).toBe(403);
    expect((await page.request.get(`${base}/payments`)).status()).toBe(403);
    await page.screenshot({ path: testInfo.outputPath("platform-brand-domain-pending.png"), fullPage: true });
    await evidence(testInfo, "platform-brand-facts", { oem: p.oem_id, logo_url: logo.asset.url, channel: p.channel_id, inherits_brand_color: true, domain_id: domain.id, hostname, dns_status: checked.status, denied_child_customers: true, denied_payments: true });
  } finally { await context.close(); }
});

test("owner invites an employee, saves fine permissions, configures risk and sends a notification, then exports real records", async ({ browser }, testInfo) => {
  test.setTimeout(240_000);
  const fixture = JSON.parse(readFileSync(fixturePath!, "utf8")), p = fixture.personas.platform, base = `/api/v1/operators/${p.operator_id}`;
  const context = await browser.newContext(), employeeContext = await browser.newContext(), customerContext = await browser.newContext(), exporterContext = await browser.newContext();
  const page = await context.newPage(), employee = await employeeContext.newPage(), customer = await customerContext.newPage(), exporter = await exporterContext.newPage();
  for (const current of [page, employee, customer, exporter]) current.setDefaultTimeout(20_000);
  try {
    await login(employee, p.restricted);
    await employee.goto(`/manage/${p.operator_id}/staff`);
    await expect(employee.getByRole("button", { name: "邮件邀请经营人员", exact: true })).toHaveCount(0);
    expect((await employee.request.get(`${base}/catalog`)).status()).toBe(403);
    expect((await employee.request.get(`${base}/payments`)).status()).toBe(403);
    await employee.getByRole("button", { name: "退出", exact: true }).click();
    await expect(employee.getByRole("button", { name: "退出", exact: true })).toHaveCount(0);
    await login(page, { email: fixture.owner_email, password: fixture.owner_password });
    await page.goto(`/manage/${p.operator_id}/staff`);
    let dialog = await panel(page, "邮件邀请经营人员");
    const email = `invited-${randomUUID()}@simulation.test`, password = "invited-simulation-password";
    await dialog.getByLabel("邮箱", { exact: false }).fill(email);
    await dialog.getByRole("combobox", { name: "受邀角色", exact: false }).selectOption("readonly");
    await submit(page, dialog, "邮件邀请经营人员", `${base}/staff`); await close(dialog);
    let invitationURL = "";
    await expect.poll(() => {
      try {
        const mailbox = JSON.parse(readFileSync(join(dirname(fixturePath!), "mailbox.json"), "utf8"));
        const messages = Array.isArray(mailbox) ? mailbox : mailbox.messages || [];
        const mail = messages.find((item: any) => (item.To || item.to) === email);
        invitationURL = String(mail?.Text || mail?.text || "").split("\n").find((line: string) => line.includes("/staff-invitation?token=")) || "";
      } catch { return false; }
      return !!invitationURL;
    }, { timeout: 45_000 }).toBe(true);
    await employee.goto(new URL(invitationURL).pathname + new URL(invitationURL).search);
    await employee.getByLabel("显示名称", { exact: false }).fill("已邀请受限员工");
    await employee.getByLabel("经营人员密码", { exact: false }).fill(password);
    await employee.getByRole("button", { name: "验证邮箱并接受邀请", exact: true }).click();
    await expect(employee.getByText("邀请已接受。", { exact: false })).toBeVisible();
    await login(employee, { email, password });
    const session = await (await employee.request.get("/api/v1/auth/me")).json(), staffID = session.staff.id;
    const memberships = await (await page.request.get(`${base}/staff`)).json();
    const membershipID = memberships.items.find((item: any) => item.values.staff_id === staffID).id;
    await page.reload();
    const staffRow = page.getByRole("row").filter({ hasText: staffID });
    dialog = await panel(page, "调整当前成员角色", staffRow);
    await dialog.getByRole("combobox", { name: "角色", exact: false }).selectOption("custom");
    for (const label of ["查看合作主体", "查看人员", "查看经营汇总", "维护平台模型与路由"]) await dialog.getByRole("checkbox", { name: label, exact: true }).check();
    await dialog.evaluate(element => element.scrollTop = 0);
    await page.screenshot({ path: testInfo.outputPath("platform-fine-permission-form.png") });
    await submit(page, dialog, "调整当前成员角色", `${base}/staff/${membershipID}/membership`); await close(dialog);
    dialog = await panel(page, "调整当前成员角色", staffRow);
    for (const label of ["查看合作主体", "查看人员", "查看经营汇总", "维护平台模型与路由"]) await expect(dialog.getByRole("checkbox", { name: label, exact: true })).toBeChecked();
    await expect(dialog.getByRole("checkbox", { name: "管理收款", exact: true })).not.toBeChecked(); await close(dialog);
    await employee.reload(); await employee.goto(`/manage/${p.operator_id}/catalog`);
    await expect(employee.getByRole("button", { name: "维护公开模型与能力证据", exact: true })).toBeVisible();
    expect((await employee.request.get(`${base}/payments`)).status()).toBe(403);
    expect((await employee.request.get(`${base}/exports`)).status()).toBe(403);
    await employee.goto(`/manage/${p.operator_id}/staff`);
    await expect(employee.getByRole("button", { name: "调整当前成员角色", exact: true })).toHaveCount(0);
    await expect(employee.getByRole("button", { name: "邮件邀请经营人员", exact: true })).toHaveCount(0);

    await page.goto(`/manage/${p.operator_id}/risk`);
    dialog = await panel(page, "新增预警规则");
    const rule = `simulation-rule-${randomUUID()}`;
    await dialog.getByLabel("规则编号", { exact: false }).fill(rule);
    await dialog.getByRole("combobox", { name: "真实指标", exact: false }).selectOption("pending_usage");
    await dialog.getByLabel("预警阈值", { exact: false }).fill("1"); await dialog.getByLabel("严重阈值", { exact: false }).fill("3");
    await dialog.getByRole("combobox", { name: "触发方向", exact: false }).selectOption("above");
    await dialog.getByLabel("通知收件人ID", { exact: false }).fill(p.customer.id);
    await submit(page, dialog, "新增预警规则", `${base}/risk`); await close(dialog);
    dialog = await panel(page, "修订此预警规则", page.getByRole("row").filter({ hasText: rule }));
    await expect(dialog.getByLabel("预警阈值", { exact: false })).toHaveValue("1");
    await dialog.getByLabel("预警阈值", { exact: false }).fill("2");
    const revised = await submit(page, dialog, "修订此预警规则", `${base}/risk`); await close(dialog);
    expect((await replay(page, revised)).status()).toBe(200); expect((await replay(page, revised, true)).status()).toBe(409);

    await page.goto(`/manage/${p.operator_id}/notifications`);
    dialog = await panel(page, "发送业务通知");
    const title = `仿真业务通知-${randomUUID()}`;
    await dialog.getByLabel("本经营范围收件人编号", { exact: false }).fill(p.customer.id);
    await dialog.getByRole("combobox", { name: "通知类别", exact: false }).selectOption("mandatory");
    await dialog.getByRole("combobox", { name: "级别", exact: false }).selectOption("info");
    await dialog.getByLabel("标题", { exact: false }).fill(title); await dialog.getByLabel("通知正文", { exact: false }).fill("平台模型和品牌配置实际仿真完成，本通知关联真实业务记录。");
    await dialog.getByLabel("业务关联依据", { exact: false }).fill(rule);
    await submit(page, dialog, "发送业务通知", `${base}/notifications`); await close(dialog);
    await login(customer, p.customer, "customer"); await customer.goto("/workspace/notifications");
    await expect(customer.getByText(title, { exact: true })).toBeVisible();
    const notification = customer.locator("section.rounded-card").filter({ has: customer.getByRole("heading", { name: title, exact: true }) });
    const readPending = customer.waitForResponse(response => response.url().includes("/api/v1/me/notifications/") && response.url().endsWith("/read") && response.request().method() === "POST");
    await notification.getByRole("button", { name: "标为已读", exact: true }).click();
    expect((await readPending).status()).toBe(200);
    await expect(notification.getByRole("button", { name: "标为已读", exact: true })).toHaveCount(0);
    await customer.screenshot({ path: testInfo.outputPath("platform-customer-notification.png"), fullPage: true });

    const exportIDs: string[] = [];
    await login(exporter, fixture.personas.finance.finance);
    for (const kind of ["ledger", "requests", "procurement"]) {
      await page.goto(`/manage/${p.operator_id}/exports`);
      dialog = await panel(page, "创建私有导出");
      await dialog.getByRole("combobox", { name: "导出业务", exact: false }).selectOption(kind);
      await dialog.getByLabel("起始时间", { exact: false }).fill(localTime(new Date(Date.now() - 86400_000)));
      await dialog.getByLabel("截至时间", { exact: false }).fill(localTime(new Date(Date.now() + 60_000)));
      const exportResult = await (await submit(page, dialog, "创建私有导出", `${base}/exports`)).json(); await close(dialog); exportIDs.push(exportResult.id);
      await expect.poll(async () => { const resource = await (await page.request.get(`${base}/exports`)).json(); return resource.items.find((row: any) => row.id === exportResult.id)?.status; }, { timeout: 45_000 }).toBe("ready");
      await page.reload();
      const exports = await (await page.request.get(`${base}/exports`)).json(), exportRow = exports.items.find((item: any) => item.id === exportResult.id);
      const downloadLink = page.locator(`a[href="${base}/exports/${exportResult.id}/download"]`);
      await expect(downloadLink.locator("xpath=ancestor::tr").locator("td").nth(2)).toHaveText(String(exportRow.values.row_count));
      const downloadPending = page.waitForEvent("download"); await downloadLink.click();
      const download = await downloadPending, exportFile = testInfo.outputPath(`platform-${kind}-export.jsonl`); await download.saveAs(exportFile);
      const exported = readFileSync(exportFile, "utf8").split("\n").filter(Boolean).map(line => JSON.parse(line));
      expect(exported.length).toBe(exportRow.values.row_count);
      if (kind === "ledger") expect(exported.length).toBeGreaterThan(0);
      if (kind === "procurement") for (const purchase of exported) expect([purchase.supplier_id, purchase.buyer_id]).toContain(p.operator_id);
      expect((await employee.request.get(`${base}/exports/${exportResult.id}/download`)).status()).toBe(403);
      expect((await exporter.request.get(`${base}/exports/${exportResult.id}/download`)).status()).toBe(404);
    }
    await page.screenshot({ path: testInfo.outputPath("platform-export-ready.png"), fullPage: true });
    await evidence(testInfo, "platform-staff-risk-export-facts", { invited_staff: staffID, fine_grants_saved: true, denied_payments: true, denied_exports: true, risk_rule: rule, risk_version: 2, notification_title: title, export_ids: exportIDs });
  } finally { await context.close(); await employeeContext.close(); await customerContext.close(); await exporterContext.close(); }
});
