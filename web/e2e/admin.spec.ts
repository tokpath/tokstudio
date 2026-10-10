import { expect, test } from "@playwright/test";
import { mockViewer } from "./mock-viewer";

test.beforeEach(async ({ page }) => { await mockViewer(page, { roles: ["platform_admin"] }); });

test("model creation saves information and first price without a manual ID", async ({ page }) => {
  let created: Record<string, unknown> | undefined;
  await page.route("**/api/admin/models", async (route) => {
    if (route.request().method() !== "POST") return route.continue();
    created = route.request().postDataJSON();
    await route.fulfill({ status: 201, json: {
      item: { id: "test/one-step", display_name: "One step", vendor: "test", status: "draft", capabilities: { kind: "text" } },
    } });
  });
  await page.goto("/admin/models/new");
  await expect(page.getByLabel("公开模型标识")).toHaveCount(0);
  await page.getByLabel("名称", { exact: true }).fill("One step");
  await page.getByLabel("原厂").fill("test");
  await page.getByRole("button", { name: "创建模型" }).click();
  await expect(page.getByText("请填写售价").first()).toBeVisible();
  await page.getByLabel("输入售价").fill("2");
  await page.getByLabel("输出售价").fill("4");
  await page.getByRole("button", { name: "创建模型" }).click();
  await page.getByRole("dialog").getByRole("button", { name: "确认", exact: true }).click();
  await expect.poll(() => created).toMatchObject({
    display_name: "One step", vendor: "test",
    capabilities: { kind: "text" },
    initial_price: { currency: "USD", customer_sell: { input: "0.000002", output: "0.000004" } },
  });
  expect(created).not.toHaveProperty("public_id");
});

test("model editor preserves historical capability metadata without switches", async ({ page }) => {
  let saved: Record<string, unknown> | undefined;
  await page.route("**/api/admin/models/test/alias", async (route) => {
    const item = {
      id: "test/alias", display_name: "Alias", vendor: "test", status: "draft", kind: "text",
      capabilities: { kind: "text", supported_parameters: ["model", "messages", "tool_choice", "response_format"] },
    };
    if (route.request().method() === "PATCH") saved = route.request().postDataJSON();
    await route.fulfill({ json: { item } });
  });
  await page.goto("/admin/models/test/alias");
  await expect(page.getByRole("group", { name: "支持的文本能力" })).toHaveCount(0);
  await page.getByLabel("名称", { exact: true }).fill("Alias updated");
  await page.getByRole("button", { name: "保存模型" }).click();
  await page.getByRole("dialog").getByRole("button", { name: "确认", exact: true }).click();
  await expect.poll(() => saved).toMatchObject({
    display_name: "Alias updated",
    capabilities: { supported_parameters: ["model", "messages", "tool_choice", "response_format"] },
  });
});

test("platform navigation shows only its current task group and expands on request",async({page})=>{
 await page.goto("/admin");const nav=page.getByRole("navigation",{name:"平台管理"});await expect(nav).toBeVisible();
 await expect(nav.getByRole("button",{name:"总览",exact:true})).toHaveAttribute("aria-expanded","true");
 await expect(nav.getByRole("link",{name:"提供商",exact:true})).not.toBeVisible();
 await nav.getByRole("button",{name:"模型与套餐",exact:true}).click();await expect(nav.getByRole("link",{name:"提供商",exact:true})).toBeVisible();await expect(nav.getByRole("link",{name:"套餐",exact:true})).toBeVisible();
 await nav.getByRole("button",{name:"资金与结算",exact:true}).click();await expect(nav.getByRole("link",{name:"提供商",exact:true})).not.toBeVisible();await expect(nav.getByRole("link",{name:"支付",exact:true})).toBeVisible();
 await expect(nav.getByRole("link",{name:"价格",exact:true})).toHaveCount(0);await expect(nav.getByRole("link",{name:"API Key",exact:true})).toHaveCount(0);await expect(nav.getByRole("link",{name:"应急手册",exact:true})).toHaveCount(0);
});

test("malformed workbench responses keep navigation usable and metrics unknown", async ({ page }) => {
 const errors: string[] = [];
 page.on("pageerror", error => errors.push(error.message));
 await page.route("**/api/admin/ops/dashboard", route => route.fulfill({ json: {} }));
 await page.route("**/api/admin/metrics/series?**", route => route.fulfill({ json: {} }));
 await page.goto("/admin");
 await expect(page.getByRole("alert").filter({hasText:"读取失败"}).first()).toBeVisible();
 const nav=page.getByRole("navigation",{name:"平台管理"});
 await nav.getByRole("button",{name:"模型与套餐",exact:true}).click();
 await expect(nav.getByRole("link",{name:"提供商",exact:true})).toBeVisible();
 await nav.getByRole("button",{name:"资金与结算",exact:true}).click();
 await expect(nav.getByRole("link",{name:"支付",exact:true})).toBeVisible();
 await expect(page.getByTestId("ops-daily-chart")).toHaveCount(0);
 expect(errors).toEqual([]);
});

test("admin margin page is TokenHub-only and never estimates cost", async ({ page }) => {
  await page.route("**/admin/margin**", async (route) => {
    if (route.request().resourceType() !== "fetch" && route.request().resourceType() !== "xhr") {
      await route.continue();
      return;
    }
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        item: { attempt_cost_minor: 0, sell_minor: 0, margin_minor: 0, pending_count: 1, items: [] },
        items: [],
      }),
    });
  });
  await page.goto("/admin/margin");
  await expect(page.getByRole("heading", { name: "成本/毛利" }).first()).toBeVisible();
  await expect(page.getByText("暂无 attempt 成本")).toBeVisible();
  await expect(page.getByText("TokenHub").first()).toBeVisible();
  await expect(page.getByRole("button", { name: "补成本" })).toBeVisible();
  await expect(page.getByRole("button", { name: "调毛利" })).toBeVisible();
  await expect(page.getByRole("button", { name: /估扣|估算扣款|estimate/i })).toHaveCount(0);
  await expect(page.getByRole("button", { name: /智能路由|smart routing/i })).toHaveCount(0);
  await expect(page.getByLabel(/手填成本|estimate cost/i)).toHaveCount(0);
  await expect(page.getByTestId("upstream-facts-badge")).toHaveCount(0);
});

test("admin margin detail rows show honest upstream-fact badges", async ({ page }) => {
  await page.route("**/admin/margin**", async (route) => {
    if (route.request().resourceType() !== "fetch" && route.request().resourceType() !== "xhr") {
      await route.continue();
      return;
    }
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        item: {
          attempt_cost_minor: 40,
          sell_minor: 20,
          margin_minor: -20,
          pending_count: 0,
          items: [
            {
              attempt_id: "atm_ok",
              request_id: "req_ok",
              provider_id: "prd_echo",
              upstream_model_id: "echo-up",
              fact_source: "sandbox",
              cost_source: "TokenHub",
              cost_minor: 20,
              sell_minor: 40,
              margin_minor: 20,
            },
            {
              attempt_id: "atm_gap",
              request_id: "req_gap",
              cost_source: "TokenHub",
              cost_minor: 0,
              sell_minor: 0,
              margin_minor: 0,
              missing_cost: true,
            },
          ],
        },
        items: [
          {
            attempt_id: "atm_ok",
            request_id: "req_ok",
            provider_id: "prd_echo",
            upstream_model_id: "echo-up",
            fact_source: "sandbox",
            cost_source: "TokenHub",
            cost_minor: 20,
            sell_minor: 40,
            margin_minor: 20,
          },
          {
            attempt_id: "atm_gap",
            request_id: "req_gap",
            cost_source: "TokenHub",
            cost_minor: 0,
            sell_minor: 0,
            margin_minor: 0,
            missing_cost: true,
          },
        ],
      }),
    });
  });
  await page.goto("/admin/margin");
  await expect(page.getByText("prd_echo / echo-up / req_ok")).toBeVisible();
  await expect(page.getByText("缺上游元数据")).toBeVisible();
  await expect(page.getByTestId("upstream-facts-badge").first()).toBeVisible();
  await expect(page.getByText("openai")).toHaveCount(0);
  await expect(page.getByRole("heading", { name: "暂无 attempt 成本" })).toHaveCount(0);
});

test("reconciliation has one task title and an explicit empty state",async({page})=>{
 await page.route("**/api/admin/usage/pending?**",r=>r.fulfill({json:{items:[]}}));await page.goto("/admin/reconciliation");
 await expect(page.getByRole("heading",{level:1,name:"待核对请求"})).toHaveCount(1);await expect(page.getByText("当前范围没有待核对请求",{exact:true})).toBeVisible();await expect(page.getByRole("button",{name:"释放所选预授权"})).toBeDisabled();await expect(page.getByTestId("admin-usage-trend-chart")).toHaveCount(0);
});

test("admin providers list and detail", async ({ page }) => {
  await page.goto("/admin/providers");
  await expect(page.getByRole("heading", { name: "提供商", exact: true })).toBeVisible();
  await expect(page.getByText("提供商是进货渠道",{exact:false})).toBeVisible();
  await expect(page.getByRole("link", { name: "新建提供商" })).toBeVisible();
  await expect(page.getByRole("columnheader", { name: "账号池" })).toBeVisible();
  await expect(page.getByRole("columnheader", { name: "已关联模型" })).toHaveCount(0);
  await expect(page.getByRole("columnheader", { name: "RPM" })).toHaveCount(0);
  await page.getByRole("link", { name: "新建提供商" }).click();
  await expect(page.getByRole("heading", { name: "新建提供商" })).toBeVisible();
  await expect(page.getByLabel("协议")).toBeVisible();
  await expect(page.getByLabel("协议")).not.toContainText("Bifrost");
  await expect(page.getByLabel("协议")).not.toContainText("沙箱回声");
  await expect(page.getByRole("button", { name: "创建提供商" })).toBeVisible();
  await page.route("**/api/admin/providers/echo-primary", route => route.fulfill({ json: {
    item: { id: "prd_echo", name: "Echo Primary", slug: "echo-primary", adapter: "sandbox", status: "active", health: "available", models: [] },
  } }));
  await page.route("**/api/admin/providers/prd_echo/accounts", route => route.fulfill({ json: { items: [] } }));
  await page.goto("/admin/providers/echo-primary");
  await expect(page.getByRole("heading", { name: "提供商详情" })).toBeVisible();
  await expect(page.getByRole("link", { name: "返回列表" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "提供商状态" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "接到的公开模型" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "同步上游" })).toHaveCount(0);
  await expect(page.getByRole("heading", { name: "凭据轮换" })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "轮换凭据" })).toHaveCount(0);
  await expect(page.getByRole("heading", { name: "账号池" })).toBeVisible();
  await expect(page.getByRole("button", { name: "读取账号" })).toHaveCount(0);
  await expect(page.getByRole("status")).toContainText("已加载 0 个账号");
  await expect(page.getByText("暂无账号，请在下方添加。")).toBeVisible();
  await page.getByRole("button", { name: "编辑" }).click();
  await expect(page.getByRole("button", { name: "保存提供商" })).toBeVisible();
});

test("low-frequency price entry returns to the selected model and list context",async({page})=>{
 await page.route("**/api/admin/models/test/price",r=>r.fulfill({json:{item:{id:"test/price",display_name:"Price",vendor:"test",status:"draft",capabilities:{kind:"text"},sell_price:{input:"0.000001",output:"0.000002"}}}}));
 const back="/admin/models?_admin_models_q=Price";await page.goto(`/admin/prices?model=test%2Fprice&return_to=${encodeURIComponent(back)}`);
 await expect(page).toHaveURL(/\/admin\/models\/test\/price.*#prices/);await expect(page.getByRole("heading",{name:"售价",exact:true})).toBeVisible();await expect(page.getByRole("link",{name:"返回模型列表"})).toHaveAttribute("href",back);
});

test("OEM detail routes to its delivery task and opens the correct navigation group",async({page})=>{
 await page.route("**/api/admin/channels/oem1",r=>r.fulfill({json:{item:{id:"oem1",code:"aurora",type:"C",status:"active",brand_id:"brand1"}}}));
 await page.route("**/api/admin/oem-deliveries/oem1",r=>r.fulfill({json:{item:{delivery:{channel_org_id:"oem1",phase:"configuring",version:1,sales_mode:"offline",sell_plans:false,handoff_evidence:{}},channel:{id:"oem1",code:"aurora"},brand:{id:"brand1",name:"Aurora",primary_domain:"aurora.example",api_domain:"api.aurora.example",admin_domain:"admin.aurora.example"},checks:[],ready:false,checked_at:"2026-10-10T00:00:00Z",administrators:[]}}}));
 await page.goto("/admin/channels/oem1?return_to=%2Fadmin%2Fchannels%3Fq%3DAurora");await expect(page).toHaveURL(/\/admin\/oem-deliveries\/oem1/);await expect(page.getByRole("heading",{level:1,name:/Aurora/})).toBeVisible();
 const nav=page.getByRole("navigation",{name:"平台管理"});await expect(nav.getByRole("button",{name:"客户与合作方",exact:true})).toHaveAttribute("aria-expanded","true");await expect(nav.getByRole("link",{name:"OEM 与渠道",exact:true})).toBeVisible();await expect(nav.getByRole("link",{name:"提供商",exact:true})).not.toBeVisible();
});

test("retired key management is a real unavailable page without a customer-key write link",async({page})=>{
 for(const path of ["/admin/keys","/app/settings/apps","/app/settings/members","/app/settings/team","/app/settings/webhooks"]){const response=await page.goto(path);expect(response?.status(),path).toBe(404);await expect(page.getByRole("link",{name:"本渠道 API Key"})).toHaveCount(0);}
});

test("admin creates a plan for multiple direct channels without a second confirmation", async ({ page }) => {
  let created: Record<string, unknown> | undefined;
  await page.route("**/api/admin/plans/eligible-channels", route => route.fulfill({ json: { items: [
    { id: "chn_own_1", code: "own-1", status: "active" },
    { id: "chn_own_2", code: "own-2", status: "active" },
  ] } }));
  await page.route("**/api/admin/channels?*", route => route.fulfill({ json: { items: [
    { id: "chn_own_1", code: "own-1", status: "active" },
    { id: "chn_own_2", code: "own-2", status: "active" },
    { id: "chn_other", code: "other", status: "active" },
  ] } }));
  await page.route("**/api/admin/plans?*", route => route.fulfill({ json: { items: [] } }));
  await page.route("**/api/admin/plans", async route => {
    if (route.request().method() !== "POST") return route.continue();
    created = route.request().postDataJSON();
    await route.fulfill({ status: 201, json: { item: { id: "pln_new", name: "Two channels" } } });
  });
  await page.goto("/admin/plans");
  await page.getByRole("button", { name: "创建套餐" }).click();
  const dialog = page.getByRole("dialog");
  await dialog.getByLabel("套餐名称").fill("Two channels");
  await dialog.getByLabel("指定渠道").check();
  await expect(dialog.getByRole("checkbox")).toHaveCount(2);
  await expect(dialog.getByText("other")).toHaveCount(0);
  await dialog.getByLabel("own-1").check();
  await dialog.getByLabel("own-2").check();
  await dialog.getByRole("button", { name: "创建套餐" }).click();
  await expect.poll(() => created).toMatchObject({
    name: "Two channels", channel_scope: "selected", channel_ids: ["chn_own_1", "chn_own_2"],
  });
  await expect(dialog).toHaveCount(0);
  await expect(page.getByText("确认创建套餐")).toHaveCount(0);
});

test("admin OEM brand download shows storage source and forbids a success check", async ({ page }) => {
  await page.route("**/admin/brands/**", async (route) => {
    if (route.request().resourceType() !== "fetch" && route.request().resourceType() !== "xhr") {
      await route.continue();
      return;
    }
    if (route.request().url().includes("/assets")) {
      await route.fulfill({
        status: 503,
        contentType: "application/json",
        body: JSON.stringify({ error: { code: "store_unavailable", message: "存储不可用" } }),
      });
      return;
    }
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        item: { id: "brd_oem", name: "OEM", logo_url: "/v1/public/brand-assets/bas_logo", theme: { brand: "#2150D6" } },
        brand: { id: "brd_oem", name: "OEM", logo_url: "/v1/public/brand-assets/bas_logo", theme: { brand: "#2150D6" } },
        customizable: true,
        storage: { source: "s3", ok: true, label: "S3" },
      }),
    });
  });
  await page.route("**/api/admin/brands?**",r=>r.fulfill({json:{items:[{id:"brd_oem",name:"OEM",primary_domain:"oem.example"}]}}));
  await page.goto("/admin/brands?brand=brd_oem");
  await expect(page.getByRole("heading", { level:1,name: "品牌设置" })).toBeVisible();
  await expect(page.getByText("存储源").first()).toBeVisible();
  await expect(page.getByTestId("storage-source-badge").first()).toHaveText("S3");
  await expect(page.getByTestId("storage-source-badge").first()).toHaveAttribute("data-tone", "ok");
  await expect(page.getByRole("link", { name: "下载 Logo" })).toBeVisible();
  await expect(page.getByText("✓")).toHaveCount(0);
});

test("retired public capabilities return real HTTP 404 without a placeholder",async({page})=>{
 for(const path of ["/blog","/awesome-ofox","/blog/example","/leaderboards/apps","/leaderboards/labs","/leaderboards/models","/promo/august","/vs/openrouter"]){const response=await page.goto(path);expect(response?.status(),path).toBe(404);}
});
