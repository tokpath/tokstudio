import { expect, test, type Page } from "@playwright/test";
import { translate, type Locale } from "../lib/i18n";

const brand = { id: "brand-i18n", name: "山海模型", api_base_url: "https://api.shanhai.example/v1", theme: { primary_color: "#2444aa" } };
const operator = { id: "operator-i18n", name: "星河渠道", kind: "channel", brand_id: brand.id, contact_email: "support@example.test", legal_name: "星河服务主体" };
const publicContext = { brand, operator, theme: {}, availability: [], payment_methods: [] };
const customer = { kind: "customer", customer: { id: "customer-i18n", email: "customer@example.test", display_name: "林同学" }, brand, operator, csrf_token: "csrf_i18n" };
const staff = { kind: "staff", staff: { id: "staff-i18n", email: "staff@example.test", display_name: "财务" }, workspaces: [{ operator, permissions: ["*"] }], active_operator_id: operator.id, csrf_token: "csrf_i18n" };
const credits = { customer_atg: { available: "12.000000000000000001", held: "0.01", purchased_remaining: "10.000000000000000001", gift_remaining: "2" }, money_balance: { available: "20", held: "0" }, commission_money: { available: "3", held: "1", frozen: "0.000000000000000001", recovery_due: "0" }, qualification: { confirmed_money: "50", pending_money: "5", threshold_money: "100", qualified_at: null, function_status: "normal" } };
const model = { id: "public-chat", name: "公开文本模型", capabilities: ["chat"], status: "published", prices: [{ component: "输入", amount_atg: "0.00000001", unit: "token" }] };
const existingKey = { id: "key-existing-i18n", name: "应用密钥", prefix: "th_existing_prefix", status: "active", model_ids: null, budget: { limit_atg: "5.000000000000000001", period: "month", timezone: "Asia/Shanghai", spent_atg: "0", held_atg: "0", overage_atg: "0", window_start: null, window_end: null }, rate_limit_rpm: null, concurrency_limit: null, version: 3, created_at: "2026-10-16T00:00:00Z" };
function resource(title = "业务记录") { return { title, description: "当前经营范围的真实记录；金额按原始高精度十进制展示", items: [], columns: [], operations: [], row_operations: [] }; }
function text(locale: Locale, source: string) { const value = translate(locale, source); if (locale !== "zh" && /[\u3400-\u9fff]/.test(source)) expect(value, `Missing ${locale} catalog entry: ${source}`).not.toBe(source); return value; }
async function api(page: Page, session: unknown = customer) {
  await page.route("**/api/v1/**", async route => {
    const path = new URL(route.request().url()).pathname;
    let data: unknown = { items: [] };
    if (path === "/api/v1/public/context") data = publicContext;
    else if (path === "/api/v1/auth/me") data = session;
    else if (path === "/api/v1/me/credits") data = credits;
    else if (["/api/v1/public/models", "/api/v1/me/models"].includes(path)) data = { items: [model] };
    else if (path === "/api/v1/me/keys") data = { items: [existingKey] };
    else if (path.endsWith("/workspace") || path.startsWith("/api/v1/operators/")) data = resource();
    await route.fulfill({ json: data });
  });
}

// Public pages are server-rendered with the request locale; browser API mocks
// intentionally affect only the product data fetched during hydration.
for (const [locale, acceptLanguage] of [["ru", "ru-RU"], ["ja", "ja-JP"]] as const) {
  test(`initial ${acceptLanguage} request renders the matching html language`, async ({ browser, baseURL }) => {
    const isolated = await browser.newContext({ baseURL, locale: acceptLanguage });
    try {
      const page = await isolated.newPage(); await api(page, { kind: "anonymous" });
      const response = await page.goto("/"); expect(response?.ok()).toBe(true);
      expect(await response!.text()).toMatch(new RegExp(`<html[^>]*lang="${locale}"`));
      await expect(page.locator("html")).toHaveAttribute("lang", locale);
      await expect(page.getByTestId("language-selector")).toHaveValue(locale);
      await expect(page.getByRole("link", { name: text(locale, "模型与价格"), exact: true })).toBeVisible();
    } finally { await isolated.close(); }
  });
}

test("public selector offers four languages and its host cookie survives reload and navigation", async ({ page, context }) => {
  await api(page, { kind: "anonymous" }); await page.goto("/");
  const selector = page.getByTestId("language-selector"); await expect(selector.locator("option")).toHaveCount(4);
  await expect(selector.locator("option")).toHaveText(["中文", "English", "日本語", "Русский"]);
  for (const locale of ["en", "ja", "ru", "zh"] as const) {
    await selector.selectOption(locale); await expect(page.locator("html")).toHaveAttribute("lang", locale === "zh" ? "zh-CN" : locale);
    await expect(page.getByRole("link", { name: text(locale, "模型与价格"), exact: true })).toBeVisible();
    await page.screenshot({ path: test.info().outputPath(`public-${locale}.png`), fullPage: true });
  }
  await selector.selectOption("ru");
  const cookie = (await context.cookies()).find(item => item.name === "NEXT_LOCALE");
  expect(cookie).toMatchObject({ value: "ru", domain: new URL(page.url()).hostname, path: "/", sameSite: "Lax" });
  expect(cookie!.expires).toBeGreaterThan(Date.now() / 1000 + 300 * 24 * 60 * 60);
  await page.reload(); await expect(selector).toHaveValue("ru"); await expect(page.locator("html")).toHaveAttribute("lang", "ru");
  await page.getByRole("link", { name: text("ru", "模型与价格"), exact: true }).click();
  await expect(page).toHaveURL(/\/models$/); await expect(selector).toHaveValue("ru"); await expect(page.getByRole("heading", { name: text("ru", "模型与价格"), exact: true })).toBeVisible();
});

test("customer Key edits retain authored input, precise money, and the existing Key ID across all languages", async ({ page }) => {
  await api(page); const writes: { path: string; body: Record<string, any> }[] = [];
  await page.route(`**/api/v1/me/keys/${existingKey.id}`, async route => {
    const body = route.request().postDataJSON(); writes.push({ path: new URL(route.request().url()).pathname, body });
    await route.fulfill({ json: { ...existingKey, ...body, version: 4, budget: { ...existingKey.budget, ...body.budget } } });
  });
  await page.goto("/workspace/keys"); await page.getByRole("button", { name: text("zh", "编辑配置"), exact: true }).click();
  await page.getByLabel("名称", { exact: true }).fill("查看详情"); await page.getByLabel("ATG 预算上限", { exact: true }).fill("7.000000000000000001");
  for (const locale of ["en", "ja", "ru", "zh"] as const) {
    await page.getByTestId("language-selector").selectOption(locale);
    await expect(page.getByLabel(text(locale, "名称"), { exact: true })).toHaveValue("查看详情");
    await expect(page.getByLabel(text(locale, "ATG 预算上限"), { exact: true })).toHaveValue("7.000000000000000001");
    await expect(page.getByRole("heading", { name: existingKey.name, exact: true })).toBeVisible();
    const editor = page.getByRole("region", { name: text(locale, "编辑 Key 配置"), exact: true });
    const helpTitle = text(locale, "Key 预算如何生效");
    const trigger = editor.getByRole("button", { name: translate(locale, "了解{title}", { title: helpTitle }), exact: true });
    await trigger.click();
    const help = page.getByRole("dialog", { name: helpTitle, exact: true });
    await expect(help).toBeVisible();
    await expect(help.getByText(text(locale, "例如：账号有 100 ATG，给某个 Key 设置每月 20 ATG 预算，该 Key 当月最多使用 20 ATG；创建时不会划走这 20 ATG，其他 Key 仍共享账号可用额度。"), { exact: true })).toBeVisible();
    expect(writes).toHaveLength(0);
    await page.keyboard.press("Escape");
    await expect(help).toHaveCount(0);
    await expect(trigger).toBeFocused();
    await expect(page.getByLabel(text(locale, "名称"), { exact: true })).toHaveValue("查看详情");
    await expect(page.getByLabel(text(locale, "ATG 预算上限"), { exact: true })).toHaveValue("7.000000000000000001");
  }
  await page.getByTestId("language-selector").selectOption("ru");
  await page.getByRole("button", { name: text("ru", "保存配置"), exact: true }).click();
  await expect.poll(() => writes.length).toBe(1);
  expect(writes[0].path).toBe(`/api/v1/me/keys/${existingKey.id}`);
  expect(writes[0].body).toMatchObject({ name: "查看详情", budget: { limit_atg: "7.000000000000000001", period: "month" }, version: 3 });
  expect(writes[0].body.budget.limit_atg).toEqual(expect.any(String));
});

test("an unresolved financial modal switches language and retries the exact JSON and idempotency key", async ({ page }) => {
  await api(page); const attempts: { key: string; body: string | null }[] = [];
  await page.route("**/api/v1/me/refunds/workspace", route => route.fulfill({ json: { ...resource("退款办理"), operations: [{ name: "apply", label: "确认费用并提交", method: "POST", path: "/api/v1/me/refunds", fields: [{ name: "amount_money", label: "申请金额 结算币种", kind: "decimal", required: true }, { name: "evidence", label: "付款核实说明", kind: "text", required: true }], confirmation: "确认当前行的总额、手续费和实付金额，提交后将占用本人可用佣金。" }] } }));
  await page.route("**/api/v1/me/refunds", async route => {
    attempts.push({ key: route.request().headers()["idempotency-key"], body: route.request().postData() });
    if (attempts.length === 1) await route.abort("connectionreset");
    else await route.fulfill({ json: { id: "withdrawal-i18n", status: "pending" } });
  });
  await page.goto("/workspace/refunds"); await page.getByRole("button", { name: "确认费用并提交", exact: true }).click();
  const dialog = page.getByRole("dialog");
  await dialog.getByLabel("申请金额 结算币种", { exact: false }).fill("0.000000000000000001"); await dialog.getByRole("textbox", { name: /^付款核实说明/ }).fill("查看详情");
  await dialog.getByRole("checkbox").check(); await dialog.getByRole("button", { name: "确认费用并提交", exact: true }).click();
  await expect(dialog.getByRole("button", { name: "重试本次操作", exact: true })).toBeVisible();
  await dialog.getByTestId("language-selector").selectOption("ru");
  await page.setViewportSize({ width: 390, height: 844 });
  expect(await dialog.evaluate(element => element.scrollWidth <= element.clientWidth)).toBe(true);
  await expect(dialog.getByLabel(text("ru", "申请金额 结算币种"), { exact: false })).toHaveValue("0.000000000000000001");
  await expect(dialog.getByLabel(text("ru", "申请金额 结算币种"), { exact: false })).toBeDisabled();
  await expect(dialog.getByRole("textbox", { name: new RegExp("^" + text("ru", "付款核实说明")) })).toHaveValue("查看详情");
  await expect(dialog.getByRole("button", { name: text("ru", "关闭办理面板"), exact: true })).toBeDisabled();
  await dialog.getByRole("button", { name: text("ru", "重试本次操作"), exact: true }).click();
  await expect(dialog.getByRole("button", { name: text("ru", "本次操作已完成，关闭后查看最新记录"), exact: true })).toBeVisible();
  expect(attempts).toHaveLength(2); expect(attempts[0]).toEqual(attempts[1]); expect(attempts[0].key).toMatch(/^[0-9a-f-]{36}$/);
  expect(JSON.parse(attempts[0].body!)).toEqual({ amount_money: "0.000000000000000001", evidence: "查看详情" });
});

test("operator server metadata translates while authored Chinese names and evidence remain verbatim", async ({ page }) => {
  await api(page, staff);
  await page.route(`**/api/v1/operators/${operator.id}/invoices**`, route => route.fulfill({ json: { ...resource("待复核事项"), items: [{ id: "invoice-i18n", status: "completed", values: { name: "核实收款", record_kind: "case", evidence: "查看详情", amount_money: "1.000000000000000001" } }], columns: [{ key: "name", label: "显示名称", kind: "text" }, { key: "record_kind", label: "记录类型", kind: "text" }, { key: "evidence", label: "银行核实凭证", kind: "text" }, { key: "amount_money", label: "申请金额 结算币种", kind: "money" }] } }));
  await page.route(`**/api/v1/operators/${operator.id}/invoices/invoice-i18n/files`, route => route.fulfill({json:{items:[],files_available:false,orders:[],details:{title:"核实收款",tax_id:"TEST",email:"billing@example.test"}}}));
  await page.goto(`/manage/${operator.id}/invoices`); await page.getByTestId("language-selector").selectOption("en");
  await expect(page.getByRole("heading", { name: text("en", "票据办理"), exact: true })).toBeVisible();
  await expect(page.getByRole("columnheader", { name: text("en", "名称与事项"), exact: true })).toBeVisible();
  await expect(page.getByRole("cell", { name: "核实收款", exact: true })).toBeVisible(); await expect(page.getByRole("cell", { name: "查看详情", exact: true })).toBeVisible();
  await page.getByRole("button", { name: "View details", exact: true }).click(); const dialog = page.getByRole("dialog");
  await expect(dialog.getByRole("heading", { name: "核实收款", exact: true })).toBeVisible();
  await expect(dialog.getByText("Record type", { exact: true }).filter({ visible: true })).toBeVisible();
  await expect(dialog.getByText("Refund case", { exact: true }).filter({ visible: true })).toBeVisible();
  await dialog.getByTestId("language-selector").selectOption("ru");
  await expect(dialog.getByRole("heading", { name: "核实收款", exact: true })).toBeVisible();
  await expect(dialog.getByText(text("ru", "银行核实凭证"), { exact: true }).filter({ visible: true })).toBeVisible();
  await expect(dialog.getByText("查看详情", { exact: true }).first()).toBeVisible();
});

test("Russian public and customer mobile layouts stay within the viewport", async ({ page }) => {
  await api(page); await page.setViewportSize({ width: 390, height: 844 }); await page.goto("/"); await page.getByTestId("language-selector").selectOption("ru");
  await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
  await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth)).toBeLessThanOrEqual(1);
  await page.goto("/workspace/keys"); await expect(page.getByTestId("language-selector")).toHaveValue("ru");
  await page.getByRole("button", { name: text("ru", "打开工作台导航"), exact: true }).click();
  await expect(page.getByRole("navigation", { name: text("ru", "移动工作台"), exact: true })).toBeVisible();
  await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth)).toBeLessThanOrEqual(1);
  await page.getByRole("button", { name: text("ru", "编辑配置"), exact: true }).click();
  await expect(page.getByLabel(text("ru", "ATG 预算上限"), { exact: true })).toHaveValue(existingKey.budget.limit_atg);
  await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth)).toBeLessThanOrEqual(1);
  await page.screenshot({ path: "test-results/i18n-russian-mobile.png", fullPage: true });
});


for (const locale of ["zh", "en", "ja", "ru"] as const) {
  test(`invoice workspace and original purchase details render in ${locale}`, async ({ page }) => {
    await api(page, staff);
    await page.route(`**/api/v1/operators/${operator.id}/invoices**`, route => route.fulfill({json:route.request().url().endsWith("/files") ? {
      items:[],files_available:false,details:{title:"Buyer Inc",tax_id:"TEST-1",email:"buyer@example.test"},orders:[{order_id:"purchase-one",name:"Package A",kind:"套餐购买",created_at:"2026-11-06T10:00:00Z",amount_money:"40"}]
    } : {...resource("我方开票"),items:[{id:"invoice-one",status:"requested",values:{name:"Invoice application",applicant:"Buyer Inc",applicant_kind:"客户购买",amount_money:"40"}}],columns:[{key:"applicant",label:"申请人",kind:"text"},{key:"applicant_kind",label:"申请类型",kind:"text"},{key:"amount_money",label:"开票金额",kind:"money"}]} }));
    await page.route(`**/api/v1/operators/${operator.id}/procurement-invoices**`, route => route.fulfill({json:{...resource("我方收票"),items:[{id:"received-one",status:"requested",values:{name:"Supplier invoice"}}],summary:[{label:"Supplier",value:"尚未开放采购开票申请，请联系供货方。"}]}}));
    await page.goto(`/manage/${operator.id}/invoices`);
    await page.getByTestId("language-selector").selectOption(locale);
    await expect(page.getByRole("tab",{name:text(locale,"我方开票"),exact:true})).toBeVisible();
    await expect(page.getByRole("tab",{name:text(locale,"我方收票"),exact:true})).toBeVisible();
    await expect(page.getByRole("cell",{name:text(locale,"客户购买"),exact:true})).toBeVisible();
    await page.getByRole("button",{name:text(locale,"查看详情"),exact:true}).click();
    const dialog=page.getByRole("dialog");
    await expect(dialog.getByRole("heading",{name:text(locale,"本次开票的购买订单")})).toBeVisible();
    await expect(dialog.getByText(text(locale,"订单实付金额"),{exact:true})).toBeVisible();
    await expect(dialog.getByText("buyer@example.test",{exact:true})).toBeVisible();
    await expect(dialog.getByText(text(locale,"套餐购买")+" · Package A",{exact:true})).toBeVisible();
    await page.screenshot({path:test.info().outputPath(`invoice-details-${locale}.png`),fullPage:true});
    await dialog.getByRole("button",{name:text(locale,"关闭办理面板"),exact:true}).click();
    await page.getByRole("tab",{name:text(locale,"我方收票"),exact:true}).click();
    await expect(page.getByText("Supplier invoice",{exact:true})).toBeVisible();
    await expect(page.getByText(text(locale,"尚未开放采购开票申请，请联系供货方。"),{exact:true})).toBeVisible();
    await expect(page.getByRole("button",{name:text(locale,"文件与下载"),exact:true})).toHaveCount(0);
    await page.setViewportSize({width:390,height:844});
    await expect.poll(()=>page.evaluate(()=>document.documentElement.scrollWidth-window.innerWidth)).toBeLessThanOrEqual(1);
    await page.screenshot({path:test.info().outputPath(`invoice-received-mobile-${locale}.png`),fullPage:true});
  });
}
