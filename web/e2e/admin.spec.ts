import { expect, test } from "@playwright/test";

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
  await expect(page.getByText("账号池数量")).toBeVisible();
  await expect(page.getByRole("button", { name: "新建提供商" })).toBeVisible();
  await expect(page.getByRole("columnheader", { name: "账号池" })).toBeVisible();
  await expect(page.getByRole("columnheader", { name: "已关联模型" })).toHaveCount(0);
  await expect(page.getByRole("columnheader", { name: "RPM" })).toHaveCount(0);
  await page.getByRole("button", { name: "新建提供商" }).click();
  await expect(page.getByRole("heading", { name: "接入提供商" })).toBeVisible();
  await expect(page.getByLabel("协议")).toBeVisible();
  await expect(page.getByLabel("协议")).not.toContainText("Bifrost");
  await expect(page.getByLabel("协议")).not.toContainText("沙箱回声");
  await expect(page.getByRole("button", { name: "创建" })).toBeVisible();
  await page.getByRole("button", { name: "关闭" }).click();
  await page.goto("/admin/providers/echo-primary");
  await expect(page.getByRole("heading", { name: "提供商详情" })).toBeVisible();
  await expect(page.getByRole("link", { name: "返回列表" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "提供商状态" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "接到的公开模型" })).toBeVisible();
  await expect(page.getByText("更换该上游的 API Key")).toBeVisible();
  await expect(page.getByRole("heading", { name: "凭据轮换" })).toBeVisible();
  await expect(page.getByRole("button", { name: "轮换凭据" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "账号池" })).toBeVisible();
  await expect(page.getByRole("button", { name: "读取账号" })).toBeVisible();
  await page.getByRole("button", { name: "编辑" }).click();
  await expect(page.getByRole("button", { name: "保存提供商" })).toBeVisible();
});

test("admin plan review and commission pages render", async ({ page }) => {
  await page.goto("/admin/plans");
  await expect(page.getByRole("heading", { name: "套餐审核" })).toBeVisible();
  await expect(page.getByRole("button", { name: "待审核" })).toBeVisible();
  await expect(page.getByRole("button", { name: "创建套餐" })).toBeVisible();
  await page.getByRole("button", { name: "创建套餐" }).click();
  await expect(page.getByRole("heading", { name: "创建套餐" })).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(page.getByRole("button", { name: "下架套餐" })).toBeVisible();
  await page.getByRole("button", { name: "下架套餐" }).click();
  await expect(page.getByRole("heading", { name: "下架套餐" })).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(page.getByRole("heading", { name: "续费扫描" })).toBeVisible();
  await expect(page.getByRole("button", { name: "强制到期" })).toBeVisible();
  await expect(page.getByRole("button", { name: "续费扫描" })).toBeVisible();
  await page.goto("/admin/commission");
  await expect(page.getByRole("heading", { name: "佣金策略" })).toBeVisible();
  await expect(page.getByRole("button", { name: "读取策略" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "达线规则" })).toBeVisible();
  await page.goto("/admin/payments");
  await expect(page.getByRole("heading", { name: "支付", exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "确认支付" })).toBeVisible();
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
  await expect(page.getByLabel("操作原因")).toBeVisible();
  await page.goto("/admin/alerts");
  await expect(page.getByRole("heading", { name: "告警" })).toBeVisible();
  await expect(page.getByRole("button", { name: "评估告警" })).toBeVisible();
  await page.goto("/admin/runbooks");
  await expect(page.getByRole("heading", { name: "应急手册" })).toBeVisible();
  await page.goto("/admin/billing");
  await expect(page.getByRole("heading", { name: "余额 / 充值 / 账务" })).toBeVisible();
  await expect(page.getByRole("button", { name: "赠送额度" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "供应商支出" })).toBeVisible();
  await page.goto("/admin/channels");
  await expect(page.getByRole("heading", { name: "渠道租户" }).first()).toBeVisible();
  await expect(page.getByRole("button", { name: "新建渠道" })).toBeVisible();
  await expect(page.getByRole("tab", { name: "渠道" })).toBeVisible();
  await expect(page.getByRole("tab", { name: "代理商" })).toBeVisible();
  await expect(page.getByRole("tab", { name: "KOL" })).toBeVisible();
  await expect(page.getByText("不能自己添加提供商和模型")).toBeVisible();
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
  await expect(page.getByRole("heading", { name: "平台模型白名单" })).toBeVisible();
  await expect(page.getByText("租户不能自己添加提供商和模型")).toBeVisible();
  await expect(page.getByRole("button", { name: "从平台目录授权" })).toBeVisible();
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
  await expect(page.getByRole("heading", { name: "手工结算" })).toBeVisible();
  await expect(page.getByRole("button", { name: "人工打款" })).toBeVisible();
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
  await expect(page.getByRole("columnheader", { name: "公开模型标识" }).first()).toBeVisible();
  await expect(page.getByRole("columnheader", { name: "原厂" }).first()).toBeVisible();
  await expect(page.getByRole("columnheader", { name: "提供商池" }).first()).toBeVisible();
  await expect(page.getByRole("columnheader", { name: "选路策略" }).first()).toBeVisible();
  await expect(page.getByRole("button", { name: "创建路由" })).toBeVisible();
  await page.getByRole("button", { name: "创建路由" }).click();
  await expect(page.getByRole("heading", { name: "创建路由" })).toBeVisible();
  await expect(page.getByRole("combobox", { name: "公开模型标识" })).toBeVisible();
  await expect(page.getByLabel("选路策略")).toHaveValue("priority");
  await expect(page.getByLabel("路由状态")).toHaveValue("active");
  await expect(page.getByRole("combobox", { name: "提供商" })).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(page.getByRole("button", { name: "改路由策略" })).toHaveCount(0);
  await page.goto("/admin/commission");
  await expect(page.getByRole("heading", { name: "佣金重算" })).toBeVisible();
  await expect(page.getByRole("button", { name: "重算佣金" })).toBeVisible();
  await page.goto("/admin/usage");
  await expect(page.getByRole("heading", { name: "用量回放" })).toBeVisible();
  await expect(page.getByRole("button", { name: "回放 usage" })).toBeVisible();
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
  await expect(page.getByRole("tab", { name: "目录" })).toBeVisible();
  await expect(page.getByRole("tab", { name: "审核" })).toBeVisible();
  await expect(page.getByRole("button", { name: "创建模型" })).toBeVisible();
  await expect(page.getByRole("columnheader", { name: "公开模型标识" }).first()).toBeVisible();
  await expect(page.getByRole("columnheader", { name: "提供商池" }).first()).toBeVisible();
  await expect(page.getByRole("columnheader", { name: "原厂" }).first()).toBeVisible();
  await expect(page.getByRole("heading", { name: "同步上游" })).toHaveCount(0);
  await expect(page.getByRole("heading", { name: "接到哪家提供商" })).toHaveCount(0);
  await expect(page.getByRole("heading", { name: "弃用模型" })).toHaveCount(0);
  await page.getByRole("tab", { name: "审核" }).click();
  await expect(page.getByRole("heading", { name: "模型审核" })).toBeVisible();
  await expect(page.getByRole("button", { name: "待审核" })).toBeVisible();
  await expect(page.getByRole("button", { name: "创建模型" })).toBeVisible();
  await page.getByRole("button", { name: "创建模型" }).click();
  await expect(page.getByRole("heading", { name: "创建模型" })).toBeVisible();
  await expect(page.getByLabel("显示名")).toBeVisible();
  await expect(page.getByRole("combobox", { name: "原厂" })).toBeVisible();
  await expect(page.getByLabel("公开模型标识")).toBeVisible();
  await expect(page.getByRole("combobox", { name: "提供商" })).toBeVisible();
  await expect(page.getByLabel("上游模型标识")).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(page.getByRole("heading", { name: "同步上游" })).toHaveCount(0);
  await expect(page.getByRole("heading", { name: "接到哪家提供商" })).toHaveCount(0);
  await expect(page.getByRole("heading", { name: "弃用模型" })).toHaveCount(0);
  await page.goto("/admin/models/tokenhub/echo-1");
  await expect(page.getByRole("heading", { name: "客户怎么看到它" })).toBeVisible();
  await expect(page.getByRole("button", { name: "保存显示信息" })).toBeVisible();
  const tokenizer = page.getByRole("combobox", { name: "分词器" });
  await expect(tokenizer).toBeVisible();
  await tokenizer.fill("qwe");
  await expect(page.getByRole("option", { name: "qwen" })).toBeVisible();
  await tokenizer.fill("new-tokenizer");
  await expect(page.getByRole("option", { name: "new-tokenizer 新值" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "定价" })).toBeVisible();
  await expect(page.getByRole("button", { name: "发布价格" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "接到哪家提供商" })).toBeVisible();
  await expect(page.getByRole("button", { name: "接到提供商" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "上架" })).toBeVisible();
  // Playwright CI 只起 Next，没有目录 API；未加载模型时四个动作都应置灰。
  await expect(page.getByRole("button", { name: "拒绝" })).toBeDisabled();
  await expect(page.getByRole("button", { name: "通过" })).toBeDisabled();
  await expect(page.getByRole("button", { name: "发布", exact: true })).toBeDisabled();
  await expect(page.getByRole("button", { name: "弃用此模型" })).toBeDisabled();
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
