import { expect, test, type Page } from "@playwright/test";

const brand = { id: "brand-1", name: "山海模型", api_base_url: "https://api.shanhai.example/v1", theme: { primary_color: "#2444aa", accent_color: "#163377" } };
const operator = { id: "channel-1", name: "星河渠道", kind: "channel", brand_id: "brand-1", contact_email: "support@example.test", legal_name: "星河服务主体", theme: { primary_color: "#775522" } };
const context = { brand, operator, theme: { primary_color: "#775522", accent_color: "#163377" }, availability: [], payment_methods: [] };
const customer = { kind: "customer", customer: { id: "customer-1", email: "customer@example.test", display_name: "林同学" }, brand, operator, csrf_token: "csrf_test" };
const credits = { customer_atg: { available: "12.000000000000000001", held: "0.01", purchased_remaining: "10.000000000000000001", gift_remaining: "2" }, usd_balance: { available: "20", held: "0" }, commission_usd: { available: "3", held: "1", frozen: "0.000000000000000001", recovery_due: "0" }, qualification: { confirmed_usd: "50", pending_usd: "5", threshold_usd: "100", qualified_at: null, function_status: "normal" } };
const model = { id: "public-chat", name: "公开文本模型", capabilities: ["chat"], status: "published", prices: [{ component: "输入", amount_atg: "0.00000001", unit: "token" }] };
const key = { id: "key-1", name: "应用 Key", prefix: "th_lab_prefix", status: "active", model_ids: null, budget: { limit_atg: "5.000000000000000001", period: "month", timezone: "Asia/Shanghai", spent_atg: "0", held_atg: "0", overage_atg: "0", window_start: null, window_end: null }, rate_limit_rpm: null, concurrency_limit: null, version: 1, created_at: "2026-10-16T00:00:00Z" };
function resource(title: string) { return { title, description: "核实实际事实后办理，结果不明时继续查询。", items: [], columns: [{ key: "id", label: "事项", kind: "text" }], operations: [], row_operations: [] }; }
test("invitation and recovery links never forward their token through Referer", async ({ request }) => {
  for (const path of ["/staff-invitation", "/verify-email", "/reset-password"]) {
    const response = await request.get(`${path}?token=synthetic-proof`);
    expect(response.status()).toBe(200);
    expect(response.headers()["referrer-policy"]).toBe("no-referrer");
    expect(response.headers()["cache-control"]).toContain("no-store");
  }
});
async function api(page: Page, session: unknown = customer) {
  await page.route("**/api/v1/**", async route => {
    const url = new URL(route.request().url()), path = url.pathname;
    let data: unknown = { items: [] };
    if (path === "/api/v1/public/context") { const code = url.searchParams.get("invite_code"); data = code ? { ...context, invitation: { code, status: code === "BAD" ? "invalid" : "valid", type: "user", operator: { ...operator, id: code === "NEW" ? "channel-2" : "channel-1", name: code === "NEW" ? "新经营方" : operator.name }, inviter_display_name: "已脱敏邀请人", message: code === "BAD" ? "邀请码不可用，请更正或显式移除。" : undefined } } : context; }
    else if (path === "/api/v1/auth/me") data = session;
    else if (path === "/api/v1/me/credits") data = credits;
    else if (["/api/v1/public/models", "/api/v1/me/models"].includes(path)) data = { items: [model] };
    else if (path.endsWith("/workspace")) data = resource("办理事项");
    else if (path.includes("/operators/")) data = resource("票据办理");
    await route.fulfill({ json: data });
  });
}

test("public brand and operator remain distinct; model micro-prices stay exact", async ({ page }) => {
  await api(page, { kind: "anonymous" }); await page.goto("/"); await expect(page.getByText("实际经营方：星河渠道")).toBeVisible(); await expect(page.getByRole("heading", { name: /一个品牌入口.*连接你的模型与应用/ })).toBeVisible(); await page.getByRole("link", { name: "模型与价格", exact: true }).click(); await expect(page.getByRole("heading", { name: "公开文本模型" })).toBeVisible(); await expect(page.getByLabel("0.00000001 ATG", { exact: true })).toBeVisible();
});

test("explicit dark mode keeps a branded white website readable", async ({ page }) => {
  await api(page, { kind: "anonymous" });
  await page.route("**/api/v1/public/context", route => route.fulfill({ json: { ...context, theme: { ...context.theme, background_color: "#ffffff", text_color: "#111827" } } }));
  await page.goto("/");
  await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
  await page.getByLabel("显示主题").selectOption("light");
  const surface = page.locator(".product-theme");
  await expect(surface).toHaveCSS("background-color", "rgb(255, 255, 255)");
  await page.getByLabel("显示主题").selectOption("dark");
  await expect(surface).toHaveCSS("background-color", "rgb(22, 21, 19)");
  await expect(surface).toHaveCSS("color", "rgb(237, 234, 228)");
  await page.getByLabel("显示主题").selectOption("light");
  await expect(surface).toHaveCSS("color", "rgb(17, 24, 39)");
});

test("registration uses the final explicit invitation and blocks invalid fallback", async ({ page }) => {
  await api(page, { kind: "anonymous" }); let submitted: Record<string, unknown> | undefined;
  await page.route("**/api/v1/auth/registrations", async route => { submitted = route.request().postDataJSON(); expect(route.request().headers()["idempotency-key"]).toMatch(/^[0-9a-f-]{36}$/); await route.fulfill({ json: { registration_id: "reg-1", verification_required: true, message: "请查收验证邮件" } }); });
  await page.goto("/register?invite_code=BAD"); await page.getByLabel("显示名称").fill("新客户"); await page.getByLabel("邮箱", { exact: true }).fill("new@example.test"); await page.getByLabel("密码", { exact: true }).fill("a-long-password-123"); await expect(page.getByRole("button", { name: "确认归属并注册" })).toBeDisabled();
  await page.getByLabel("邀请码（可选）").fill("NEW"); await expect(page.getByText("经营方：新经营方", { exact: false })).toBeVisible(); await page.getByRole("button", { name: "确认归属并注册" }).click(); await expect(page.getByText("请查收验证邮件")).toBeVisible(); expect(submitted?.invite_code).toBe("NEW");
});

test("customer amounts stay precise and a created key secret disappears when closed", async ({ page }) => {
  await api(page); let created = false;
  await page.route("**/api/v1/me/keys", async route => { if (route.request().method() === "POST") { const body = route.request().postDataJSON(); expect(body.budget.limit_atg).toBe("5.000000000000000001"); created = true; await route.fulfill({ json: { key, secret: "th_secret_shown_once" } }); } else await route.fulfill({ json: { items: created ? [key] : [] } }); });
  await page.goto("/workspace"); await expect(page.getByLabel("12.000000000000000001 ATG", { exact: true })).toBeVisible(); await page.getByRole("navigation", { name: "工作台", exact: true }).getByRole("link", { name: "API Key", exact: true }).click(); await page.getByRole("button", { name: "创建 Key", exact: true }).click(); await page.getByLabel("名称", { exact: true }).fill("应用 Key"); await page.getByLabel("ATG 预算上限", { exact: true }).fill("5.000000000000000001"); await page.getByRole("button", { name: "创建 Key", exact: true }).last().click(); await expect(page.getByText("th_secret_shown_once")).toBeVisible(); await page.getByRole("button", { name: "关闭", exact: true }).click(); await expect(page.getByText("th_secret_shown_once")).toHaveCount(0); await expect(page.getByText("th_lab_prefix…")).toBeVisible();
});

test("unknown financial action freezes input and retries its exact key and body", async ({ page }) => {
  await api(page); const attempts: { key: string; body: string | null }[] = [];
  await page.route("**/api/v1/me/withdrawals/workspace", route => route.fulfill({ json: { ...resource("佣金提现"), operations: [{ name: "apply", label: "申请提现", method: "POST", path: "/api/v1/me/withdrawals", fields: [{ name: "amount_usd", label: "申请金额 USD", kind: "decimal", required: true }], confirmation: "已核对适用费用与净应付，审核通过后仍需实际付款核实。" }] } }));
  await page.route("**/api/v1/me/withdrawals", async route => { attempts.push({ key: route.request().headers()["idempotency-key"], body: route.request().postData() }); if (attempts.length === 1) await route.abort("connectionreset"); else await route.fulfill({ json: { id: "withdrawal-1", status: "pending", message: "提现已申请，款项尚未支付" } }); });
  await page.goto("/workspace/withdrawals"); await page.getByRole("button", { name: "申请提现" }).click(); await page.getByLabel("申请金额 USD").fill("0.000000000000000001"); await page.getByRole("checkbox").check(); await page.getByRole("button", { name: "申请提现" }).last().click(); await expect(page.getByText("连接中断，操作结果尚未核实。请沿用本次操作重试或查询记录。")).toBeVisible(); await expect(page.getByLabel("申请金额 USD")).toBeDisabled(); await page.getByRole("button", { name: "重试本次操作" }).click(); await expect(page.getByText("提现已申请，款项尚未支付")).toBeVisible(); expect(attempts).toHaveLength(2); expect(attempts[0]).toEqual(attempts[1]);
});

test("staff notification entry is a bell beside the signed-in account", async ({ page }) => {
  await api(page, { kind: "staff", staff: { id: "staff-1", email: "owner@example.test", display_name: "负责人" }, workspaces: [{ operator: { ...operator, kind: "platform" }, permissions: ["invoices.read", "notifications.read"] }], active_operator_id: operator.id, csrf_token: "csrf_test" });
  await page.goto(`/manage/${operator.id}/invoices`);
  await expect(page.getByRole("heading", { name: "票据办理" })).toBeVisible();
  await expect(page.getByRole("navigation", { name: "工作台", exact: true }).getByRole("link", { name: "通知管理" })).toHaveCount(0);
  const bell = page.getByRole("link", { name: "通知管理", exact: true });
  await expect(bell).toBeVisible();
  await expect(bell).toHaveAttribute("href", `/manage/${operator.id}/notifications`);
  await expect(bell).toHaveText("");
  const name = page.getByText("负责人", { exact: true });
  const bellBox = await bell.boundingBox();
  const nameBox = await name.boundingBox();
  expect(bellBox && nameBox && bellBox.x < nameBox.x && Math.abs(bellBox.y - nameBox.y) < 24).toBeTruthy();
});

test("staff sees only granted workspace resources; customer cannot open management", async ({ page }) => {
  await api(page, { kind: "staff", staff: { id: "staff-1", email: "finance@example.test", display_name: "财务" }, workspaces: [{ operator, permissions: ["invoices.read"] }], active_operator_id: operator.id }); await page.goto(`/manage/${operator.id}/invoices`); await expect(page.getByRole("heading", { name: "票据办理" })).toBeVisible(); await expect(page.getByRole("link", { name: "供应商与密钥" })).toHaveCount(0); await expect(page.getByRole("link", { name: "采购与补货" })).toHaveCount(0); await page.screenshot({ path: "test-results/management-contract.png", fullPage: true }); await api(page, customer); await page.goto(`/manage/${operator.id}/invoices`); await expect(page.getByText("当前登录身份无法访问此空间。", { exact: false })).toBeVisible();
});

test("mobile workspace exposes all customer flows without horizontal overflow", async ({ page }) => {
  await api(page); await page.setViewportSize({ width: 390, height: 844 }); await page.goto("/workspace");
  await page.getByRole("button", { name: "打开工作台导航", exact: true }).click();
  const nav = page.getByRole("navigation", { name: "移动工作台", exact: true });
  await expect(nav.getByRole("link")).toHaveCount(6);
  for (const title of ["概览", "模型与接入", "API Key", "用量与账单", "套餐与资金", "邀请与收益"]) await expect(nav.getByRole("link", { name: title, exact: true })).toBeVisible();
  const tools = page.getByRole("navigation", { name: "移动工作台工具", exact: true });
  await expect(tools.getByRole("link", { name: "账号与安全", exact: true })).toBeVisible();
  await expect(tools.getByRole("link", { name: "通知与订阅", exact: true })).toBeVisible();
  await nav.getByRole("link", { name: "邀请与收益", exact: true }).click();
  await page.getByRole("button", { name: "打开工作台导航", exact: true }).click();
  await expect(nav.getByRole("navigation", { name: "邀请与收益内导航", exact: true }).getByRole("link", { name: "佣金明细", exact: true })).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
  await page.screenshot({ path: "test-results/customer-mobile.png", fullPage: true });
});

test("takeover requires complete snapshot review and separates actual cash settlement",async({page})=>{
 await api(page,{kind:"staff",staff:{id:"reviewer",email:"owner@example.test",display_name:"负责人"},workspaces:[{operator:{...operator,kind:"oem"},permissions:["*"]}],active_operator_id:operator.id});
 const snapshot={customers:{"customer-one":4},assets:[{account_id:"account-one",customer_id:"customer-one",purpose:"customer:usd",unit:"USD",balance:"25.000000000000000001",held:"0",version:8}],coverage:{required:"100",space:"0"},procurement:{quantity:"200",cost:"80"},lots:[{ID:"lot-one",PaidUSD:"80",ExitTerms:{version:1,voluntary_refund:true,method:"remaining_paid_proportional",terms_reference:"original accepted exit"}}],service_atg:"100",service_usd:"100",cash_usd:"25.000000000000000001",offset_atg:"200",offset_usd:"80",residual_refund_usd:"0",settlement_due_usd:"45.000000000000000001",rounding_evidence:{rule:"exact accepted contract"}};
 const item={id:"takeover-one",channel_id:operator.id,brand_operator_id:"brand-operator",state:"awaiting_approval",method:"supply_coverage",service_usd_per_atg:"1",procurement_usd_per_atg:"0.4",extra_usd:"0",contract_reference:"signed-contract",evidence:"verified",requested_by:"another-owner",snapshot,snapshot_fingerprint:"snapshot-review-fingerprint",service_usd:"100",cash_usd:"25.000000000000000001",procurement_offset_atg:"200",procurement_offset_usd:"80",remaining_procurement_refund_usd:"0",settlement_due_usd:"45.000000000000000001",settled_usd:"0",created_at:"2026-10-16T00:00:00Z"};
 const submitted:unknown[]=[];
 await page.route("**/api/v1/operators/channel-1/takeovers**",async route=>{if(route.request().method()==="GET")return route.fulfill({json:{items:[item],channels:[{id:operator.id,name:operator.name,legal_name:operator.legal_name,lifecycle:"active"}]}});submitted.push(route.request().postDataJSON());item.state="completed";return route.fulfill({json:item})});
 await page.goto("/manage/channel-1/takeovers");await expect(page.getByRole("heading",{name:"渠道接管",exact:true})).toBeVisible();await page.getByRole("button",{name:"复核并执行接管"}).click();await expect(page.getByText("customer-one",{exact:false}).first()).toBeVisible();await expect(page.getByRole("dialog").getByText("original accepted exit",{exact:true})).toBeVisible();await expect(page.getByRole("textbox")).toHaveCount(0);await expect(page.getByRole("button",{name:"确认提交"})).toBeEnabled();await page.getByRole("checkbox").check();await page.getByRole("button",{name:"确认提交"}).click();expect(submitted).toEqual([{expected_snapshot:"snapshot-review-fingerprint"}]);
 await page.goto("/manage/channel-1/takeover-settlements");await expect(page.getByRole("heading",{name:"接管资金结清",exact:true})).toBeVisible();await page.getByRole("button",{name:"登记实际收付款"}).click();await expect(page.getByRole("textbox", { name: "唯一银行流水 / 支付凭证", exact: true })).toBeVisible();await expect(page.getByRole("textbox", { name: "本次实际结清 USD", exact: true })).toBeVisible();await page.screenshot({path:"test-results/takeover-cash-settlement.png",fullPage:true});
});
