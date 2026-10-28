import { expect, test, type Locator, type Page, type Response, type TestInfo } from "@playwright/test";
import { chooseControl } from "./choose-control";
import { readFileSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { randomUUID } from "node:crypto";

const fixturePath = process.env.TOKENHUB_FRONTEND_ACCEPTANCE_FILE;
test.skip(!fixturePath, "Requires the isolated application and independent HTTP model service");

// Field help carries the same business name; match its form control precisely.
function control(scope: Page | Locator, label: string, exact = false) {
  return scope.locator("input,textarea,select").and(scope.getByLabel(label, { exact }));
}

async function login(page: Page, account: { email: string; password: string }, kind = "staff") {
  await page.goto("/login");
  await control(page, "邮箱", true).fill(account.email);
  await control(page, "密码", true).fill(account.password);
  await page.getByRole("button", { name: "登录", exact: true }).click();
  await expect(page).toHaveURL(kind === "staff" ? /\/manage\// : /\/workspace/);
}

async function panel(page: Page, label: string, row?: Locator) {
  await (row || page).getByRole("button", { name: label, exact: true }).first().click();
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
async function setPath(dialog: Locator, path: string, value: string | boolean) {
  const box = dialog.locator(`[data-config-path="${path}"]`).first();
  const select = box.locator("select");
  if (await select.count()) {
    await expect(select).toBeEnabled();
    await select.selectOption(String(value));
    return;
  }
  if (typeof value === "boolean") {
    await box.getByRole("radio", { name: value ? "是" : "否", exact: true }).check();
    return;
  }
  await box.locator("input,textarea").last().fill(value);
}
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
  const model = `simulation-${randomUUID()}`, base = `/api/v1/operators/${p.operator_id}`;
  let route = "";
  try {
    await login(tech, p.tech);
    await tech.getByRole("link", { name: "平台技术", exact: true }).click();
    await expect(tech.getByRole("navigation", { name: "平台技术内导航", exact: true }).getByRole("link", { name: "模型供应商", exact: true })).toBeVisible();
    await expect(tech.getByRole("link", { name: "收款方式", exact: true })).toHaveCount(0);
    await tech.goto(`/manage/${p.operator_id}/providers`);
    const credentialName = `仿真供应商密钥-${randomUUID().slice(0, 8)}`;
    let dialog = await panel(tech, "添加供应商密钥");
    await control(dialog, "凭据名称", true).fill(credentialName);
    await control(dialog, "供应商密钥", true).fill("isolated-simulation-provider-key");
    await dialog.locator('[data-command-field="enabled"]').getByRole("radio", { name: "是", exact: true }).check();
    const credentialResponse = await submit(tech, dialog, "添加供应商密钥", `${base}/providers`);
    const credential = await credentialResponse.json(); await close(dialog);
    const credentials = await (await tech.request.get(`${base}/providers`)).json();
    const credentialView = credentials.items.find((row: any) => row.id === credential.id);
    expect(credentialView.presentation?.title).toBe(credentialName);
    expect(JSON.stringify(credentials)).not.toContain("isolated-simulation-provider-key");
    dialog = await panel(tech, "修改密钥", tech.getByRole("row").filter({ hasText: credentialName }));
    await control(dialog, "新的供应商密钥", true).fill("isolated-simulation-provider-key-rotated");
    const rotated = await submit(tech, dialog, "修改密钥", `${base}/providers`);
    expect((await rotated.json()).version).toBe(2);
    expect((await replay(tech, rotated)).status()).toBe(200);
    expect((await replay(tech, rotated, true)).status()).toBe(409);
    await close(dialog);

    dialog = await panel(tech, "添加供应商模型");
    await setPath(dialog, "supplier_model.credential_id", credential.id);
    await setPath(dialog, "supplier_model.provider", "openai");
    await setPath(dialog, "supplier_model.model", "test-chat");
    await setPath(dialog, "supplier_model.base_url", `${p.lab_url}/v1`);
    await dialog.getByRole("checkbox", {name:"对话",exact:true}).check();
    const supplierSaved = await submit(tech, dialog, "添加供应商模型", `${base}/providers`);
    const supplierModelID = supplierSaved.request().postDataJSON().configuration.supplier_model.id as string;
    expect(supplierModelID).toMatch(/^[0-9a-f-]{36}$/);
    await close(dialog);

    await tech.goto(`/manage/${p.operator_id}/catalog`);
    dialog = await panel(tech, "添加公开模型");
    await setPath(dialog, "model.id", model);
    await dialog.getByRole("checkbox", { name: "对话", exact: true }).check();
    await submit(tech, dialog, "添加公开模型", `${base}/catalog`); await close(dialog);
    dialog = await panel(tech, "编辑模型", tech.getByRole("row").filter({ hasText: model }));
    await expect(control(dialog, "公开模型标识", false)).toHaveValue(model);
    await submit(tech, dialog, "编辑模型", `${base}/catalog`); await close(dialog);

    await tech.goto(`/manage/${p.operator_id}/routes`);
    dialog = await panel(tech, "配置路由");
    await setPath(dialog, "route.public_model_id", model);
    await dialog.getByRole("button", { name: "添加绑定", exact: true }).click();
    await setPath(dialog, "route.bindings.0.supplier_model_id", supplierModelID);
    const routeCreated = await submit(tech, dialog, "配置路由", `${base}/routes`);
    route = (await routeCreated.json()).id;
    expect(route).toBe(model);
    await close(dialog);
    dialog = await panel(tech, "编辑路由", tech.getByRole("row").filter({ hasText: model }));
    await expect(dialog.locator('[data-config-path$=".routing"],[data-config-path$=".priority"]')).toHaveCount(0);
    await submit(tech, dialog, "编辑路由", `${base}/routes`); await close(dialog);
    const routes = await (await tech.request.get(`${base}/routes`)).json(), savedRoute = routes.items.find((row: any) => row.id === model);
    expect(savedRoute.version).toBe(2); expect(savedRoute.values.bindings[0].priority).toBeUndefined(); expect(savedRoute.values.bindings[0].supplier_model_id).toBe(supplierModelID);

    await tech.goto(`/manage/${p.operator_id}/catalog`);
    const modelRow = tech.getByRole("row").filter({ hasText: model });
    const toggle = modelRow.getByRole("switch", { name: "开放模型" });
    await expect(toggle).toBeEnabled();
    const opened = tech.waitForResponse(response => new URL(response.url()).pathname.endsWith("/availability") && response.request().method() === "POST");
    await toggle.click();
    expect((await opened).status()).toBe(200);
    await expect(toggle).toHaveAttribute("aria-checked", "true");
    await tech.screenshot({ path: testInfo.outputPath("platform-catalog.png"), fullPage: true });
    expect((await tech.request.get(`${base}/payments`)).status()).toBe(403);

    await login(owner, { email: fixture.owner_email, password: fixture.owner_password });
    const effective = new Date(Math.ceil((Date.now() + 135 * 1000) / 60_000) * 60_000);
    await owner.goto(`/manage/${p.operator_id}/pricing`);
    dialog = await panel(owner, "授权与上架");
    await chooseControl(dialog.getByRole("combobox", { name: "公开模型", exact: true }), model);
    await submit(owner, dialog, "授权与上架", `${base}/pricing`); await close(dialog);
    dialog = await panel(owner, "授权与上架");
    await dialog.getByRole("combobox", { name: "办理事项", exact: true }).selectOption("authorization");
    await chooseControl(dialog.getByRole("combobox", { name: "被授权的直属下级", exact: true }), p.channel_id);
    await chooseControl(dialog.getByRole("combobox", { name: "公开模型", exact: true }), model);
    await submit(owner, dialog, "授权与上架", `${base}/pricing`); await close(dialog);
    for (const scope of [{ kind: "retail", seller_id: p.operator_id }, { kind: "supply", seller_id: p.operator_id, buyer_id: p.channel_id }]) {
      dialog = await panel(owner, "设置模型价格");
      await expect(dialog.locator('[data-config-path="price.id"] input')).toHaveCount(0);
      await setPath(dialog, "price.scope.kind", scope.kind);
      if ("buyer_id" in scope) await setPath(dialog, "price.scope.buyer_id", scope.buyer_id);
      await setPath(dialog, "price.model_id", model);
      for (const index of [0, 1, 2]) {
        await setPath(dialog, `price.rule.components.${index}.rate`, "1");
      }
      const publication = await submit(owner, dialog, "设置模型价格", `${base}/pricing`);
      const published = await publication.json();
      const priceID = publication.request().postDataJSON().configuration.price.id;
      expect(priceID).toMatch(/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/);
      expect(published.id).toMatch(/^[0-9a-f-]{36}$/);
      await close(dialog);
    }

    await owner.goto(`/manage/${p.operator_id}/business-status`);
    const business = await (await owner.request.get(`${base}/business-status`)).json();
    if (!business.items[0].values.activity) {
      await owner.getByRole("button", { name: "开启 营销活动", exact: true }).click();
      await expect(owner.getByRole("button", { name: "暂停 营销活动", exact: true })).toBeEnabled();
    }
    await owner.goto(`/manage/${p.operator_id}/gifts`);
    dialog = await panel(owner, "发放赠送额度");
    await chooseControl(dialog.getByRole("combobox", { name: "本经营方当前客户", exact: false }), p.customer.id);
    await dialog.getByRole("combobox", { name: "赠送原因", exact: false }).selectOption("targeted_gift");
    await control(dialog, "本次新增赠送ATG", false).fill("10");
    await control(dialog, "赠送说明", false).fill("合成仿真：为真实模型协议调用发放独立赠送，不涉及现金或佣金");
    await submit(owner, dialog, "发放赠送额度", `${base}/gifts`); await close(dialog);

    await login(customer, p.customer, "customer");
    await expect.poll(async () => {
      const models = await (await customer.request.get("/api/v1/me/models")).json();
      return models.items?.some((item: any) => item.id === model && item.status === "active");
    }, { timeout: Math.max(120_000, effective.getTime() - Date.now() + 120_000), intervals: [5000] }).toBe(true);
    await customer.goto("/workspace/keys");
    await customer.getByRole("button", { name: "创建 Key", exact: true }).click();
    await control(customer, "名称", true).fill("平台仿真应用");
    await control(customer, "ATG 预算上限").fill("5");
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
    await evidence(testInfo, "platform-model-facts", { model, route, credential_id: credential.id, route_version: 2, credential_version: 2, request_id: requestID, replay_did_not_charge_again: true, customer_charged_atg: requestFact.charged_atg });
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
    await control(dialog, "主色（留空继承", false).fill("#3159A8");
    await control(dialog, "客户支持URL", false).fill("https://support.example.test/platform-simulation");
    const saved = await submit(page, dialog, "编辑网站外观", `${base}/branding`); await close(dialog);
    expect((await replay(page, saved)).status()).toBe(200); expect((await replay(page, saved, true)).status()).toBe(409);
    dialog = await panel(page, "编辑网站外观", oem);
    await expect(control(dialog, "主色（留空继承", false)).toHaveValue("#3159A8");
    await control(dialog, "背景色（留空继承）", false).fill("#F4F6FB");
    await submit(page, dialog, "编辑网站外观", `${base}/branding`); await close(dialog);
    dialog = await panel(page, "上传目标Logo", oem);
    const logoBytes = await page.screenshot({ clip: { x: 0, y: 0, width: 32, height: 32 } });
    await dialog.locator('input[type="file"]').setInputFiles({ name: "simulation-logo.png", mimeType: "image/png", buffer: logoBytes });
    const logo = await (await submit(page, dialog, "上传目标Logo", `${base}/branding/logo`)).json(); await close(dialog);
    dialog = await panel(page, "编辑网站外观", oem);
    await control(dialog, "已上传Logo地址", false).fill(logo.asset.url);
    await submit(page, dialog, "编辑网站外观", `${base}/branding`); await close(dialog);
    const brands = await (await page.request.get(`${base}/branding`)).json(), branded = brands.items.find((row: any) => row.id === p.oem_id);
    expect(branded.values.effective_theme.primary_color).toBe("#3159A8"); expect(branded.values.effective_theme.logo_url).toBe(logo.asset.url);

    await page.goto(`/manage/${p.operator_id}/channel-themes`);
    const channel = page.getByRole("row").filter({ hasText: "平台品牌仿真渠道" });
    dialog = await panel(page, "编辑网站外观", channel);
    await control(dialog, "主色（留空继承", false).fill("#9B3C5C");
    await submit(page, dialog, "编辑网站外观", `${base}/channel-themes`); await close(dialog);
    dialog = await panel(page, "编辑网站外观", channel);
    await expect(control(dialog, "主色（留空继承", false)).toHaveValue("#9B3C5C");
    await control(dialog, "主色（留空继承", false).fill("");
    await submit(page, dialog, "编辑网站外观", `${base}/channel-themes`); await close(dialog);
    const channels = await (await page.request.get(`${base}/channel-themes`)).json(), inherited = channels.items.find((row: any) => row.id === p.channel_id);
    expect(inherited.values.primary_color).toBeFalsy();
    const publicContext = await (await page.request.get("/api/v1/public/context")).json();
    expect(inherited.values.effective_theme.primary_color).toBe(publicContext.theme.primary_color);

    await page.goto(`/manage/${p.operator_id}/domains`);
    dialog = await panel(page, "申请域名验证");
    await chooseControl(dialog.getByRole("combobox", { name: "目标品牌经营方", exact: false }), p.oem_id);
    const hostname = `simulation-${randomUUID()}.example.test`;
    await control(dialog, "品牌域名", false).fill(hostname);
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
    await expect(employee.getByRole("button", { name: "开通或邀请经营人员", exact: true })).toHaveCount(0);
    expect((await employee.request.get(`${base}/catalog`)).status()).toBe(403);
    expect((await employee.request.get(`${base}/payments`)).status()).toBe(403);
    await employee.getByRole("button", { name: "退出", exact: true }).click();
    await expect(employee.getByRole("button", { name: "退出", exact: true })).toHaveCount(0);
    await login(page, { email: fixture.owner_email, password: fixture.owner_password });
    await page.goto(`/manage/${p.operator_id}/staff`);
    let dialog = await panel(page, "开通或邀请经营人员");
    const email = `invited-${randomUUID()}@simulation.test`, password = "invited-simulation-password";
    await dialog.getByRole("combobox", { name: "开通方式", exact: true }).selectOption("email");
    await control(dialog, "登录邮箱", true).fill(email);
    await dialog.getByRole("combobox", { name: "角色", exact: true }).selectOption("readonly");
    await submit(page, dialog, "开通或邀请经营人员", `${base}/staff`); await close(dialog);
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
    await control(employee, "显示名称", false).fill("已邀请受限员工");
    await control(employee, "经营人员密码", false).fill(password);
    await employee.getByRole("button", { name: "接受邀请", exact: true }).click();
    await expect(employee.getByText("邀请已接受。", { exact: false })).toBeVisible();
    await login(employee, { email, password });
    const session = await (await employee.request.get("/api/v1/auth/me")).json(), staffID = session.staff.id;
    const memberships = await (await page.request.get(`${base}/staff`)).json();
    const membershipID = memberships.items.find((item: any) => item.values.staff_id === staffID).id;
    await page.reload();
    const staffRow = page.getByRole("row").filter({ hasText: email });
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
    await expect(employee.getByRole("button", { name: "添加公开模型", exact: true })).toBeVisible();
    expect((await employee.request.get(`${base}/payments`)).status()).toBe(403);
    expect((await employee.request.get(`${base}/exports`)).status()).toBe(403);
    await employee.goto(`/manage/${p.operator_id}/staff`);
    await expect(employee.getByRole("button", { name: "调整当前成员角色", exact: true })).toHaveCount(0);
    await expect(employee.getByRole("button", { name: "开通或邀请经营人员", exact: true })).toHaveCount(0);

    await page.goto(`/manage/${p.operator_id}/alerts?view=rules`);
    dialog = await panel(page, "新增预警规则");
    const rule = `仿真请求积压提醒-${randomUUID()}`;
    await expect(control(dialog, "规则编号", false)).toHaveCount(0);
    await control(dialog, "规则名称", false).fill(rule);
    await dialog.getByRole("combobox", { name: "真实指标", exact: false }).selectOption("pending_usage");
    await control(dialog, "预警阈值", false).fill("1"); await control(dialog, "严重阈值", false).fill("3");
    await dialog.getByRole("combobox", { name: "触发方向", exact: false }).selectOption("above");
    await dialog.locator('[data-command-field="recipients"] label').filter({ hasText: fixture.owner_email }).getByRole("checkbox").check();
    const createdRule = await (await submit(page, dialog, "新增预警规则", `${base}/alerts`)).json();
    expect(createdRule.id).toMatch(/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i);
    await close(dialog);
    dialog = await panel(page, "修订此预警规则", page.getByRole("row").filter({ hasText: rule }));
    await expect(control(dialog, "预警阈值", false)).toHaveValue("1");
    await control(dialog, "预警阈值", false).fill("2");
    const revised = await submit(page, dialog, "修订此预警规则", `${base}/alerts`); await close(dialog);
    expect((await replay(page, revised)).status()).toBe(200); expect((await replay(page, revised, true)).status()).toBe(409);

    await page.goto(`/manage/${p.operator_id}/notifications`);
    dialog = await panel(page, "发送业务通知");
    const title = `仿真业务通知-${randomUUID()}`;
    const recipient = dialog.locator('[data-command-field="recipient_id"]');
    await chooseControl(recipient.getByRole("combobox"), p.customer.id);
    await dialog.getByRole("combobox", { name: "通知类别", exact: false }).selectOption("mandatory");
    await control(dialog, "标题", false).fill(title); await control(dialog, "通知正文", false).fill("平台模型和品牌配置实际仿真完成，本通知关联真实业务记录。");
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
      await page.getByRole("combobox", { name: "导出内容", exact: true }).selectOption(kind);
      const pendingExport = page.waitForResponse(response => response.url().endsWith(base + "/exports") && response.request().method() === "POST");
      await page.getByRole("button", { name: "生成 CSV", exact: true }).click();
      const exportResponse = await pendingExport; expect(exportResponse.status()).toBe(200);
      const exportResult = await exportResponse.json(); exportIDs.push(exportResult.id);
      await expect.poll(async () => { const resource = await (await page.request.get(`${base}/exports`)).json(); return resource.items.find((row: any) => row.id === exportResult.id)?.status; }, { timeout: 45_000 }).toBe("ready");
      await page.reload();
      const exports = await (await page.request.get(`${base}/exports`)).json(), exportRow = exports.items.find((item: any) => item.id === exportResult.id);
      const downloadLink = page.getByRole("link", {name:"下载 CSV",exact:true}).and(page.locator(`a[href="${base}/exports/${exportResult.id}/download"]`));
      await expect(page.getByRole("columnheader", { name: "记录数", exact: true })).toBeVisible();
      const countColumn = await page.getByRole("columnheader").allTextContents();
      const countIndex = countColumn.findIndex(label => label.trim() === "记录数");
      expect(countIndex).toBeGreaterThanOrEqual(0);
      await expect(downloadLink.locator("xpath=ancestor::tr").locator("td").nth(countIndex)).toHaveText(String(exportRow.values.row_count));
      const downloadPending = page.waitForEvent("download"); await downloadLink.click();
      const download = await downloadPending, exportFile = testInfo.outputPath(`platform-${kind}-export.csv`); await download.saveAs(exportFile);
      const exported = readFileSync(exportFile, "utf8");
      expect(exported.startsWith("\ufeff")).toBe(true);
      expect(exported).toContain(kind === "ledger" ? "变动金额" : kind === "requests" ? "净扣费 ATG" : "供货方");
      if (kind === "ledger") expect(exportRow.values.row_count).toBeGreaterThan(0);
      expect((await employee.request.get(`${base}/exports/${exportResult.id}/download`)).status()).toBe(403);
      expect((await exporter.request.get(`${base}/exports/${exportResult.id}/download`)).status()).toBe(404);
    }
    await page.screenshot({ path: testInfo.outputPath("platform-export-ready.png"), fullPage: true });
    await evidence(testInfo, "platform-staff-risk-export-facts", { invited_staff: staffID, fine_grants_saved: true, denied_payments: true, denied_exports: true, risk_rule: rule, risk_version: 2, notification_title: title, export_ids: exportIDs });
  } finally { await context.close(); await employeeContext.close(); await customerContext.close(); await exporterContext.close(); }
});

test("platform configures input and output modality billing, synchronizes Bifrost prices and records actual supplier cost", async ({ browser }, testInfo) => {
  test.setTimeout(180_000);
  const fixture = JSON.parse(readFileSync(fixturePath!, "utf8")), p = fixture.personas.platform, base = `/api/v1/operators/${p.operator_id}`;
  const context = await browser.newContext(), page = await context.newPage();
  page.setDefaultTimeout(20_000);
  const model = `multimodal-${randomUUID()}`, note = `独立联调：供应商账单合计，仅用于对比 ${randomUUID().slice(0,8)}`;
  try {
    await login(page, { email: fixture.owner_email, password: fixture.owner_password });
    await page.goto(`/manage/${p.operator_id}/catalog`);
    let dialog = await panel(page, "添加公开模型");
    await setPath(dialog, "model.id", model);
    await dialog.getByRole("checkbox", { name: "对话", exact: true }).check();
    await submit(page, dialog, "添加公开模型", `${base}/catalog`); await close(dialog);
    await page.goto(`/manage/${p.operator_id}/pricing`);
    dialog = await panel(page, "设置计费模板");
    await setPath(dialog, "model_id", model);
    await dialog.locator('[data-config-path="billing.input"]').getByRole("radio", {name:"按模态组合",exact:true}).check();
    await dialog.getByRole("checkbox", {name:"输入计量 · 图片",exact:true}).check();
    await dialog.getByRole("combobox", {name:"输入计量 · 图片 · 计量方式",exact:true}).selectOption("megapixels");
    await dialog.locator('[data-config-path="billing.output"]').getByRole("radio", {name:"按模态组合",exact:true}).check();
    const saved = await submit(page, dialog, "设置计费模板", `${base}/pricing`);
    expect(saved.request().postDataJSON().configuration.billing).toMatchObject({ input: [{ modality: "text", template: "tokens" }, { modality: "image", template: "megapixels" }], output: [{ modality: "text", template: "tokens" }] });
    await close(dialog);

    await page.goto(`/manage/${p.operator_id}/bifrost`);
    await expect(page.getByRole("button",{name:"调整价目同步",exact:true})).toHaveCount(0);
    dialog = await panel(page, "立即同步");
    await submit(page, dialog, "立即同步", `${base}/bifrost`); await close(dialog);
    await expect.poll(async () => {
      const settings = await (await page.request.get(`${base}/bifrost`)).json();
      return !!settings.items?.[0]?.values?.last_success_at && !settings.items?.[0]?.values?.sync_requested;
    }, { timeout: 30_000 }).toBe(true);
    const refs = await (await page.request.get(`${base}/references/pricing-models?q=test-chat`)).json();
    expect(refs.items.length).toBeGreaterThan(0);
    const suppliers = await (await page.request.get(`${base}/references/cost-suppliers`)).json();
    expect(suppliers.items.length).toBeGreaterThan(0);
    await page.goto(`/manage/${p.operator_id}/supplier-costs`);
    dialog = await panel(page, "登记实际费用");
    await chooseControl(dialog.getByRole("combobox", { name: "供应商", exact: true }), suppliers.items[0].value);
    await control(dialog, "期间开始", true).fill(localTime(new Date(Date.now() - 86_400_000)));
    await control(dialog, "期间结束", true).fill(localTime(new Date(Date.now() + 60_000)));
    await control(dialog, "实际费用 USD", true).fill("92");
    await control(dialog, "备注", true).fill(note);
    const recorded = await submit(page, dialog, "登记实际费用", `${base}/supplier-costs`); const actual = await recorded.json(); await close(dialog);
    let row = page.getByRole("row").filter({ hasText: note });
    await expect(row).toBeVisible();
    dialog = await panel(page, "修改登记", row);
    await control(dialog, "实际费用 USD", true).fill("91");
    await submit(page, dialog, "修改登记", `${base}/supplier-costs`); await close(dialog);
    const actuals = await (await page.request.get(`${base}/supplier-costs`)).json();
    expect(actuals.items.find((item: any) => item.id === actual.id).values.amount_usd).toBe("91");
    for (const locale of ["en", "ja", "ru", "zh"]) {
      await page.getByTestId("language-selector").selectOption(locale);
      await expect(page.locator("html")).toHaveAttribute("lang", locale === "zh" ? "zh-CN" : locale);
      await page.screenshot({ path: testInfo.outputPath(`supplier-costs-${locale}.png`), fullPage: true });
    }
    await evidence(testInfo, "model-pricing-acceptance", { model, input_and_output_combined: true, sync_hours: 24, actual_record_id: actual.id, actual_usd: "91" });
  } finally { await context.close(); }
});
