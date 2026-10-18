import { expect, test, type Page } from "@playwright/test";

const brand = { id: "brand-customer", name: "山海模型", api_base_url: "https://api.shanhai.example/v1", theme: {} };
const operator = { id: "customer-operator", name: "山海经营方", kind: "oem", brand_id: brand.id };
const amount = "10.000000000000000001";
const plan = { id: "plan-one", name: "通用套餐", price_usd: amount, purchase_atg: "20", gift_atg: "2", validity: "month", auto_renew_available: true, status: "active", version: 1 };
const quote = { id: "quote-one", plan_id: plan.id, plan_name: plan.name, operator_name: operator.name, amount_usd: amount, purchase_atg: "20", plan_gift_atg: "2", operator_gift_atg: "0", kol_gift_atg: "0", total_atg: "22", validity: "month", terms_version: "terms-one", expires_at: "2099-10-18T00:00:00Z", warnings: [] };
const order = { id: "order-one", status: "fulfilled", kind: "package", operator_name: operator.name, amount_usd: amount, paid_usd: amount, plan_name: plan.name, created_at: "2026-10-18T00:00:00Z", delivered_at: "2026-10-18T00:01:00Z" };

async function customerAPI(page: Page) {
  await page.route("**/api/v1/**", async route => {
    const path = new URL(route.request().url()).pathname;
    let data: unknown = { items: [] };
    if (path === "/api/v1/public/context") data = { brand, operator, theme: {}, availability: [], payment_methods: [{ id: "bank", name: "银行转账", kind: "offline", status: "ready" }] };
    else if (path === "/api/v1/auth/me") data = { kind: "customer", customer: { id: "customer-one", display_name: "林同学", email: "lin@example.test" }, brand, operator, csrf_token: "csrf_customer_test" };
    else if (path === "/api/v1/me/credits") data = { customer_atg: { available: "20", held: "0", purchased_remaining: "20", gift_remaining: "0" }, usd_balance: { available: amount, held: "0" }, commission_usd: { available: "100", held: "0", frozen: "50", recovery_due: "0" }, qualification: { confirmed_usd: "0", pending_usd: "0", threshold_usd: "100", qualified_at: null, function_status: "normal" } };
    else if (path === "/api/v1/me/plans/catalog") data = { items: [plan, { ...plan, id: "plan-two", name: "另一个套餐" }] };
    else if (path === "/api/v1/public/plans") data = { items: [plan] };
    else if (path === "/api/v1/me/orders/quotes") data = quote;
    else if (path === "/api/v1/me/orders/order-one") data = order;
    await route.fulfill({ json: data });
  });
}

for (const navigationMode of ["native", "fallback"] as const) test(`unknown checkout locks the plan, logout and browser Back, then recovers the original order (${navigationMode})`, async ({ page }) => {
  await customerAPI(page);
  await page.addInitScript(mode => {
    if (mode === "fallback") Object.defineProperty(window, "navigation", { value: undefined, configurable: true });
    const navigation = (window as Window & { navigation?: EventTarget }).navigation;
    const recordBack = () => { const root = document.documentElement; root.dataset.customerBackAttempts = String(Number(root.dataset.customerBackAttempts || "0") + 1); };
    if (navigation) navigation.addEventListener("navigate", raw => { if ((raw as Event & { navigationType?: string }).navigationType === "traverse") recordBack(); });
    else window.addEventListener("popstate", recordBack, true);
  }, navigationMode);
  let logoutAttempts = 0;
  await page.route("**/api/v1/auth/logout", async route => { logoutAttempts++; await route.fulfill({ json: { id: "session-one", status: "logged_out" } }); });
  const attempts: { key: string; body: string | null }[] = [];
  await page.route("**/api/v1/me/orders", async route => {
    if (route.request().method() === "GET") return route.fulfill({ json: { items: [order] } });
    attempts.push({ key: route.request().headers()["idempotency-key"], body: route.request().postData() });
    if (attempts.length === 1) return route.abort("connectionreset");
    return route.fulfill({ json: order });
  });
  await page.goto("/plans");
  const choosePlan = page.getByRole("link", { name: "选择套餐，核对成交条款" });
  await expect(choosePlan).toHaveAttribute("href", "/workspace/plans?plan=plan-one");
  await choosePlan.click();
  await expect(page).toHaveURL(/\/workspace\/plans\?plan=plan-one$/);
  await page.getByRole("button", { name: "继续，核对成交条款" }).click();
  await expect(page.getByLabel("本次使用 USD 支付余额")).toHaveValue(amount);
  await expect(page.getByLabel("使用可用佣金补足", { exact: false })).not.toBeChecked();
  await expect(page.getByLabel("授权到期自动续费")).not.toBeChecked();
  await page.getByLabel("我确认本次价格", { exact: false }).check();
  await page.getByRole("button", { name: "确认购买并核实到账" }).click();
  await expect(page.getByRole("button", { name: "重试原订单操作" })).toBeVisible();
  await expect(page.getByRole("button", { name: "选择此套餐" })).toBeDisabled();
  await expect(page.getByRole("button", { name: "返回调整活动或重新报价" })).toBeDisabled();
  await page.getByRole("button", { name: "退出", exact: true }).click();
  await expect(page.getByText("本次操作正在处理或结果待核实，请先完成原操作核实，再离开此页。")).toBeVisible();
  await expect(page).toHaveURL(/\/workspace\/plans\?plan=plan-one$/);
  expect(logoutAttempts).toBe(0);
  await page.getByRole("navigation", { name: "工作台", exact: true }).getByRole("link", { name: "API Key", exact: true }).click();
  await expect(page).toHaveURL(/\/workspace\/plans\?plan=plan-one$/);
  await page.evaluate(() => window.history.back());
  await expect(page.locator("html")).toHaveAttribute("data-customer-back-attempts", "1");
  await expect(page).toHaveURL(/\/workspace\/plans\?plan=plan-one$/);
  await expect(page.getByRole("button", { name: "重试原订单操作" })).toBeVisible();
  await page.getByRole("button", { name: "重试原订单操作" }).click();
  await expect(page.getByText("服务额度已到账，可以开始使用。")).toBeVisible();
  expect(attempts).toHaveLength(2);
  expect(attempts[1]).toEqual(attempts[0]);
  expect(logoutAttempts).toBe(0);
  expect(JSON.parse(attempts[0].body || "{}")).toEqual({ quote_id: "quote-one", terms_version: "terms-one", funding: { usd_balance: amount, commission_usd: "0" }, auto_renew: false, allow_commission: false });
  await page.getByRole("link", { name: "创建或管理 Key" }).click();
  await expect(page).toHaveURL(/\/workspace\/keys$/);
});

test("speech experience sends the model capability and its required parameters, then links to the same usage record", async ({ page }) => {
  await customerAPI(page);
  const requests: { key: string; body: unknown }[] = [];
  await page.route("**/api/v1/me/models", route => route.fulfill({ json: { items: [{ id: "public-speech", name: "公开语音模型", capabilities: ["speech"], status: "active", prices: [{ component: "characters", amount_atg: "0.000000000000000001", unit: "character" }], capability_details: [{ operation: "speech", streaming: false, input_types: ["text"], parameters: ["voice", "response_format"], required_parameters: ["voice"], max_input_bytes: 4096, max_output: 10000, parameter_constraints: { voice: { values: ["calm", "bright"] }, response_format: { values: ["mp3", "wav"] } } }] }] } }));
  await page.route("**/api/v1/me/playground", async route => { requests.push({ key: route.request().headers()["idempotency-key"], body: route.request().postDataJSON() }); await route.fulfill({ json: { request_id: "speech-request", status: "succeeded", charged_atg: "0.000000000000000001", media_kind: "audio", result_url: "/api/v1/me/requests/speech-request/result", result_expires_at: "2099-10-18T01:00:00Z" } }); });
  await page.goto("/workspace/playground");
  await page.getByRole("combobox", { name: "公开模型", exact: true }).selectOption("public-speech");
  await page.getByRole("combobox", { name: "能力", exact: true }).selectOption("speech");
  await page.getByLabel("合成语音文本").fill("你好，欢迎使用模型。");
  await page.getByRole("combobox", { name: "音色（必填）", exact: true }).selectOption("calm");
  await page.getByRole("combobox", { name: "结果格式（可选）", exact: true }).selectOption("mp3");
  await page.getByRole("button", { name: "确认计费并发送" }).click();
  await expect(page.getByLabel("0.000000000000000001 ATG", { exact: true }).first()).toBeVisible();
  await expect(page.getByRole("link", { name: "核对费用与任务状态" })).toHaveAttribute("href", "/workspace/requests?record=speech-request");
  expect(requests).toHaveLength(1);
  expect(requests[0].body).toEqual({ model: "public-speech", capability: "speech", input: "你好，欢迎使用模型。", parameters: { voice: "calm", response_format: "mp3" } });
  expect(requests[0].key).toMatch(/^[0-9a-f-]{36}$/);
});
