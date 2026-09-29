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

test("admin P0 nav renders", async ({ page }) => {
  await page.goto("/admin");
  await expect(page.getByRole("navigation", { name: "平台管理" })).toBeVisible();
  await expect(page.getByRole("link", { name: "提供商" })).toBeVisible();
  await expect(page.getByRole("link", { name: "套餐审核" })).toBeVisible();
  await expect(page.getByRole("link", { name: "价格" })).toHaveCount(0);
  await expect(page.getByRole("link", { name: "API Key" })).toHaveCount(0);
  await expect(page.getByRole("link", { name: "支付" })).toBeVisible();
  await expect(page.getByRole("link", { name: "指标" })).toBeVisible();
  await expect(page.getByRole("link", { name: "成本/毛利", exact: true })).toBeVisible();
  await expect(page.getByRole("link", { name: "佣金策略" })).toBeVisible();
  await expect(page.getByRole("link", { name: "推广码" })).toBeVisible();
  await expect(page.getByRole("link", { name: "告警" })).toBeVisible();
  await expect(page.getByRole("link", { name: "对账", exact: true })).toBeVisible();
  await expect(page.getByRole("link", { name: "待对账", exact: true })).toHaveCount(0);
  await expect(page.getByRole("link", { name: "应急手册" })).toBeVisible();
  await expect(page.getByRole("link", { name: "审计日志" })).toBeVisible();
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

test("admin reconciliation headings are unique", async ({ page }) => {
  await page.goto("/admin/reconciliation");
  await expect(page.getByRole("heading", { name: "对账", exact: true })).toHaveCount(1);
  await expect(page.getByRole("heading", { name: "待对账队列", exact: true })).toHaveCount(1);
  await expect(page.getByRole("heading", { name: "待对账", exact: true })).toHaveCount(0);
  await expect(page.getByRole("link", { name: "对账", exact: true })).toBeVisible();
  await expect(page.getByRole("link", { name: "待对账", exact: true })).toHaveCount(0);
  await expect(page.getByText("缺 usage 只进队列，不按估算扣款").first()).toBeVisible();
  await expect(page.getByRole("button", { name: "标记已解" }).first()).toBeVisible();
  await expect(page.getByTestId("admin-usage-trend-chart")).toHaveCount(0);
});

test("admin providers list and detail", async ({ page }) => {
  await page.goto("/admin/providers");
  await expect(page.getByRole("heading", { name: "提供商", exact: true })).toBeVisible();
  await expect(page.getByText("提供商是进货渠道")).toBeVisible();
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

test("admin plan review and commission pages render", async ({ page }) => {
  await page.goto("/admin/plans");
  await expect(page.getByRole("heading", { name: "套餐审核" })).toBeVisible();
  await expect(page.getByRole("button", { name: "待审核" })).toBeVisible();
  await expect(page.getByLabel("按渠道筛选")).toBeVisible();
  await expect(page.getByLabel("按套餐类型筛选")).toBeVisible();
  await expect(page.getByLabel("按套餐名筛选")).toBeVisible();
  await expect(page.getByRole("button", { name: "创建套餐" })).toBeVisible();
  await page.getByRole("button", { name: "创建套餐" }).click();
  await expect(page.getByRole("heading", { name: "创建套餐" })).toBeVisible();
  await expect(page.getByLabel("购买方式")).toHaveValue("once");
  await expect(page.getByLabel("购买方式").locator("option")).toHaveCount(4);
  await expect(page.getByText("一次性额度长期有效")).toBeVisible();
  await expect(page.getByRole("group", { name: "适用渠道" })).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(page.getByRole("button", { name: "下架套餐" })).toHaveCount(0);
  await expect(page.getByRole("heading", { name: "续费扫描" })).toHaveCount(0);
  await expect(page.getByRole("columnheader", { name: "包括额度" })).toBeVisible();
  const planHeaders = await page.getByRole("columnheader").allTextContents();
  expect(planHeaders.indexOf("包括额度")).toBeLessThan(planHeaders.indexOf("适用渠道"));
  await expect(page.getByRole("columnheader", { name: "原因" })).toHaveCount(0);
  await page.goto("/admin/commission");
  await expect(page.getByRole("heading", { name: "佣金策略" })).toBeVisible();
  await expect(page.getByRole("button", { name: "读取策略" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "达线规则" })).toBeVisible();
  await page.goto("/admin/payments");
  await expect(page.getByRole("heading", { name: "支付", exact: true })).toBeVisible();
  await expect(page.getByLabel("查找订单", { exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "确认支付", exact: true })).toHaveCount(0);
  await page.goto("/admin/prices");
  await expect(page.getByRole("heading", { name: "价格", exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "发布价格" })).toBeVisible();
  await page.goto("/admin/metrics");
  await expect(page.getByRole("heading", { name: "运营看板" })).toBeVisible();
  await expect(page.getByLabel("用量维度")).toBeVisible();
  await expect(page.getByTestId("ops-echarts")).toBeVisible();
  await expect(page.getByTestId("ops-daily-chart")).toBeVisible();
  await page.goto("/admin/users");
  await expect(page.getByRole("heading", { name: "用户/项目" })).toBeVisible();
  await expect(page.getByLabel("筛选用户")).toBeVisible();
  await expect(page.getByLabel("操作原因")).toHaveCount(0);
  await page.goto("/admin/alerts");
  await expect(page.getByRole("heading", { name: "告警" })).toBeVisible();
  await expect(page.getByRole("button", { name: "评估告警" })).toBeVisible();
  await page.goto("/admin/runbooks");
  await expect(page.getByRole("heading", { name: "应急手册" })).toBeVisible();
  await page.goto("/admin/billing");
  await expect(page.getByRole("heading", { name: "余额 / 充值 / 账务" })).toBeVisible();
  await expect(page.getByRole("button", { name: "赠送额度" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "供应商支出" })).toBeVisible();
  await page.route("**/api/admin/channels/chn_reseller_b", (route) => route.fulfill({ json: {
    item: { id: "chn_reseller_b", code: "reseller-b", type: "B", status: "active", brand_id: "brd_a", parent_id: "chn_official_a" },
  } }));
  await page.goto("/admin/channels");
  await expect(page.getByRole("heading", { name: "合作平台与直属渠道" })).toBeVisible();
  await expect(page.getByRole("button", { name: "新建渠道" })).toBeVisible();
  await expect(page.getByRole("tab", { name: "渠道" })).toBeVisible();
  await expect(page.getByRole("tab", { name: "代理商" })).toBeVisible();
  await expect(page.getByRole("tab", { name: "KOL" })).toBeVisible();
  await expect(page.getByText("模型逐级授权。", { exact: false })).toBeVisible();
  await page.getByRole("button", { name: "新建渠道" }).click();
  await expect(page.getByRole("heading", { name: "创建渠道" })).toBeVisible();
  await expect(page.getByRole("button", { name: "创建渠道" })).toBeVisible();
  await page.getByRole("button", { name: "关闭" }).click();
  await page.getByRole("tab", { name: "代理商" }).click();
  await expect(page.getByRole("heading", { name: "代理商" })).toBeVisible();
  await expect(page.getByRole("button", { name: "新建代理商" })).toBeVisible();
  await page.getByRole("tab", { name: "KOL" }).click();
  await expect(page.getByRole("heading", { name: "KOL", exact: true })).toBeVisible();
  await page.goto("/admin/channels/chn_reseller_b");
  await expect(page.getByRole("heading", { name: "渠道详情" })).toBeVisible();
  await expect(page.getByRole("link", { name: "返回列表" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "渠道模型授权" })).toBeVisible();
  await expect(page.getByText("只能授权平台已发布且路由已启用的模型", { exact: false })).toBeVisible();
  await expect(page.getByRole("button", { name: "授权模型" })).toBeVisible();
  await expect(page.getByRole("link", { name: "模型" }).first()).toBeVisible();
  await expect(page.getByRole("heading", { name: "收款就绪" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "渠道盈亏" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "供应商支出" })).toBeVisible();
  await expect(page.getByText("停用后冻结新消费")).toBeVisible();
  await page.getByRole("button", { name: "编辑" }).click();
  await expect(page.getByRole("button", { name: "保存渠道" })).toBeVisible();
  await page.goto("/admin/partners/acr_b_agent");
  await expect(page.getByRole("heading", { name: "代理商详情" })).toBeVisible();
  await expect(page.getByText("不能自建提供商或模型")).toBeVisible();
  await page.goto("/admin/commission");
  await expect(page.getByRole("heading", { name: "佣金结算与打款登记" })).toBeVisible();
  await expect(page.getByRole("button", { name: "生成结算单" })).toBeVisible();
  await page.goto("/admin/settings");
  await expect(page.getByRole("heading", { name: "管理员 2FA" })).toBeVisible();
  await expect(page.getByRole("button", { name: "读取 2FA" })).toBeVisible();
  await expect(page.getByRole("button", { name: "开始绑定" })).toBeVisible();
  await expect(page.getByRole("button", { name: "确认启用" })).toBeVisible();
  await expect(page.getByRole("button", { name: "关闭 2FA" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "运维开关" })).toBeVisible();
  await expect(page.getByRole("button", { name: "健康探测" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "备份演练" })).toBeVisible();
  await expect(page.getByRole("button", { name: "备份演练" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "异常演练" })).toBeVisible();
  await expect(page.getByRole("button", { name: "支付演练" })).toBeVisible();
  await expect(page.getByRole("button", { name: "媒体演练" })).toBeVisible();
  await expect(page.getByRole("button", { name: "TLS 演练" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "OEM 证书" })).toBeVisible();
  await expect(page.getByRole("button", { name: "签发证书" })).toBeVisible();
  await page.goto("/admin/routes");
  await expect(page.getByRole("heading", { name: "路由组" })).toBeVisible();
  await expect(page.getByRole("columnheader", { name: "公开模型" }).first()).toBeVisible();
  await expect(page.getByRole("columnheader", { name: "提供商" }).first()).toBeVisible();
  await expect(page.getByRole("columnheader", { name: "策略" }).first()).toBeVisible();
  await expect(page.getByRole("link", { name: "创建路由组" })).toBeVisible();
  await page.getByRole("link", { name: "创建路由组" }).click();
  await expect(page.getByRole("heading", { name: "创建路由组" })).toBeVisible();
  await expect(page.getByRole("combobox", { name: "公开模型标识" })).toBeVisible();
  await expect(page.getByLabel("选路策略")).toHaveValue("priority");
  await expect(page.getByLabel("状态")).toHaveValue("inactive");
  await page.getByRole("button", { name: "添加提供商" }).click();
  await expect(page.getByRole("combobox", { name: "提供商" })).toBeVisible();
  await page.goto("/admin/commission");
  await expect(page.getByRole("heading", { name: "佣金核对与重算" })).toBeVisible();
  await expect(page.getByRole("button", { name: "核对佣金" })).toBeVisible();
  await page.goto("/admin/usage");
  await expect(page.getByRole("heading", { name: "用量回放" })).toBeVisible();
  await expect(page.getByRole("button", { name: "补录真实用量" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "按日用量" })).toBeVisible();
  await expect(page.getByTestId("admin-usage-trend-chart")).toBeVisible();
  await expect(page.getByLabel("按 API Key 筛选")).toBeVisible();
  await expect(page.getByLabel("按模型筛选")).toBeVisible();
  await expect(page.getByLabel("按渠道筛选")).toBeVisible();
  await expect(page.getByRole("heading", { name: "用量 / 账单" })).toBeVisible();
  await page.goto("/admin/promos");
  await expect(page.getByRole("heading", { name: "推广角色" })).toBeVisible();
  await expect(page.getByRole("button", { name: "创建推广码" })).toBeVisible();
  await page.goto("/admin/audit");
  await expect(page.getByRole("heading", { name: "Outbox 与探测" })).toBeVisible();
  await expect(page.getByRole("button", { name: "读取 Outbox" })).toBeVisible();
  await expect(page.getByRole("button", { name: "写入探测" })).toBeVisible();
  await page.goto("/admin/keys");
  await expect(page.getByText("平台不再管理用户 API Key")).toBeVisible();
  await expect(page.getByRole("link", { name: "本渠道 API Key" })).toBeVisible();
  await page.goto("/admin/models");
  await expect(page.getByRole("link", { name: "创建模型" })).toBeVisible();
  await expect(page.getByRole("columnheader", { name: "模型名称" }).first()).toBeVisible();
  await expect(page.getByRole("columnheader", { name: "调用 ID" }).first()).toBeVisible();
  await expect(page.getByRole("columnheader", { name: "接入" }).first()).toBeVisible();
  await expect(page.getByRole("columnheader", { name: "原厂" }).first()).toBeVisible();
  await page.getByRole("link", { name: "创建模型" }).click();
  await expect(page.getByRole("heading", { name: "模型信息" })).toBeVisible();
  await expect(page.getByText("填写模型信息和首次售价，一次创建完整配置。")).toHaveCount(0);
  await expect(page.getByLabel("名称")).toBeVisible();
  await expect(page.getByLabel("原厂")).toBeVisible();
  await expect(page.getByLabel("公开模型标识")).toHaveCount(0);
  await expect(page.getByLabel("类型")).toHaveValue("text");
  await expect(page.getByRole("heading", { name: "售价" })).toBeVisible();
  await expect(page.getByLabel("输入售价")).toBeVisible();
  await expect(page.getByLabel("输出售价")).toBeVisible();
  await expect(page.getByRole("button", { name: "创建模型" })).toBeVisible();
  await page.route("**/api/admin/models/tokenhub/echo-1", route => route.fulfill({ json: {
    item: { id: "tokenhub/echo-1", vendor: "tokenhub", display_name: "Echo", status: "published", kind: "text", capabilities: { kind: "text" }, sell_price: { input: "0.000001", output: "0.000002" } },
  } }));
  await page.goto("/admin/models/tokenhub/echo-1");
  await expect(page.getByRole("heading", { name: "模型信息" })).toBeVisible();
  await expect(page.getByRole("button", { name: "保存模型" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "售价" })).toBeVisible();
  await expect(page.getByRole("button", { name: "发布价格" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "模型状态" })).toBeVisible();
  await expect(page.getByRole("link", { name: "配置路由" })).toBeVisible();
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
  await page.goto("/admin/brands");
  await expect(page.getByRole("heading", { name: "OEM 品牌" }).first()).toBeVisible();
  await expect(page.getByText("存储源").first()).toBeVisible();
  await expect(page.getByTestId("storage-source-badge").first()).toHaveText("S3");
  await expect(page.getByTestId("storage-source-badge").first()).toHaveAttribute("data-tone", "ok");
  await expect(page.getByRole("link", { name: "下载 Logo" })).toBeVisible();
  await expect(page.getByText("✓")).toHaveCount(0);
});
