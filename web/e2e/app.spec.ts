import { expect, test } from "@playwright/test";

test("public storefront shows models plans and topup", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByRole("heading", { name: "可用模型" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "套餐与订阅" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "充值" })).toBeVisible();
  await expect(page.getByRole("button", { name: "兑换码充值" })).toBeVisible();
  await expect(page.getByRole("button", { name: "创建支付充值" })).toBeVisible();
  await expect(page.getByRole("link", { name: "去登录" })).toBeVisible();
  await page.getByRole("link", { name: "去登录" }).click();
  await expect(page).toHaveURL(/login\?next=/);
  await expect(page.getByRole("heading", { name: "注册 / 登录" })).toBeVisible();
});

test("user overview is personal stats trends and shortcuts", async ({ page }) => {
  await page.goto("/app");
  await expect(page.getByRole("navigation", { name: "用户控制台" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "我的账户" })).toBeVisible();
  const overview = page.getByLabel("总览");
  await expect(overview.getByText("可用余额")).toBeVisible();
  await expect(overview.getByText("预授权占用")).toBeVisible();
  await expect(overview.getByText("API Key")).toBeVisible();
  await expect(overview.getByText("路由回单")).toBeVisible();
  await expect(page.getByRole("heading", { name: "用量趋势" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "按日用量" })).toBeVisible();
  await expect(page.getByTestId("overview-trend-chart")).toBeVisible();
  await expect(page.getByRole("link", { name: "查看用量汇总" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "快捷入口" })).toBeVisible();
  await expect(page.getByRole("link", { name: "创建 API Key" })).toBeVisible();
  await expect(page.getByRole("link", { name: "充值余额" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "媒体任务" })).toHaveCount(0);
  await expect(page.getByRole("heading", { name: "个人设置" })).toHaveCount(0);
  await expect(page.getByRole("heading", { name: "接入示例" })).toHaveCount(0);
});

test("user keys page keeps create dialog", async ({ page }) => {
  await page.goto("/app/keys");
  await expect(page.getByRole("heading", { level: 1, name: "API Key" })).toBeVisible();
  await expect(page.getByRole("button", { name: "创建 API Key" })).toBeVisible();
  await page.getByRole("button", { name: "创建 API Key" }).click();
  await expect(page.getByRole("heading", { name: "创建 API Key" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "模型白名单" })).toBeVisible();
  await expect(page.getByLabel("模型白名单")).toBeVisible();
  await expect(page.getByLabel("并发限额")).toBeVisible();
  await page.getByRole("button", { name: "取消" }).click();
});

test("user reconciliation page is three-bucket vs usage with no estimate debit", async ({ page }) => {
  await page.route("**/v1/me/reconciliation**", async (route) => {
    if (route.request().method() === "POST") {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ item: { request_id: "req_gap", state: "pending_reconciliation" } }),
      });
      return;
    }
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        item: {
          buckets: { available_minor: 9000000, reserved_minor: 1000000, withdrawable_minor: 0 },
          usage_totals: { requests: 2, customer_minor: 160000, charge_minor: 160000, pending_count: 1 },
          items: [
            {
              request_id: "req_ok",
              usage_id: "usg_ok",
              match: true,
              status: "match",
              usage_minor: 160000,
              charge_minor: 160000,
              reserved_minor: 0,
            },
            {
              request_id: "req_gap",
              usage_id: "usg_gap",
              match: false,
              status: "mismatch",
              already_pending: true,
              usage_minor: 0,
              charge_minor: 0,
              reserved_minor: 1000000,
            },
          ],
          pending: [{ request_id: "req_gap" }],
        },
      }),
    });
  });
  await page.goto("/app/reconciliation");
  await expect(page.getByRole("heading", { level: 1, name: "对账" })).toHaveCount(1);
  await expect(page.getByRole("heading", { name: "待对账", exact: true })).toHaveCount(0);
  await expect(page.getByRole("navigation", { name: "用户控制台" }).getByRole("link", { name: "对账", exact: true })).toBeVisible();
  const userBuckets = page.getByLabel("三桶");
  await expect(userBuckets.getByText("余额", { exact: true })).toBeVisible();
  await expect(userBuckets.getByText("冻结", { exact: true })).toBeVisible();
  await expect(userBuckets.getByText("可提现", { exact: true })).toBeVisible();
  await expect(page.getByTestId("diff-match")).toBeVisible();
  await expect(page.getByTestId("diff-mismatch")).toBeVisible();
  await expect(page.getByRole("heading", { name: "待对账队列" })).toBeVisible();
  await expect(page.getByRole("button", { name: "送入待对账队列" })).toBeVisible();
  await expect(page.getByRole("button", { name: /估扣|估算扣款/ })).toHaveCount(0);
  await expect(page.getByTestId("usage-trend-chart")).toHaveCount(0);
});

test("user usage page is summary and links to activity", async ({ page }) => {
  await page.goto("/app/usage");
  await expect(page.getByRole("heading", { name: "用量汇总" }).first()).toBeVisible();
  await expect(page.getByLabel("按 API Key 筛选")).toBeVisible();
  await expect(page.getByRole("heading", { name: "按日用量" })).toBeVisible();
  await expect(page.getByTestId("usage-trend-chart")).toBeVisible();
  await expect(page.getByRole("heading", { name: "密钥汇总" })).toBeVisible();
  await expect(page.getByRole("link", { name: "查看请求明细" })).toBeVisible();
  await page.getByRole("link", { name: "查看请求明细" }).click();
  await expect(page).toHaveURL(/\/app\/activity/);
  await expect(page.getByRole("heading", { name: "请求明细" })).toBeVisible();
});

test("user media page is list-first with create dialog", async ({ page }) => {
  // 前端 job 无 Go API；先 mock 空列表，避免依赖 proxy :8080。
  await page.route("**/v1/me/media**", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ items: [] }),
    });
  });
  await page.goto("/app/media");
  await expect(page.getByRole("heading", { level: 1, name: "媒体任务" })).toBeVisible();
  await expect(page.getByLabel("筛选媒体类型")).toBeVisible();
  await expect(page.getByRole("button", { name: "刷新任务" })).toBeVisible();
  await expect(page.getByRole("button", { name: "新建任务" }).first()).toBeVisible();
  await expect(page.getByText("暂无媒体任务")).toBeVisible();
  await page.getByRole("button", { name: "新建任务" }).first().click();
  await expect(page.getByRole("heading", { name: "新建任务" })).toBeVisible();
  await expect(page.getByLabel("时长")).toBeVisible();
  await expect(page.getByLabel("分辨率")).toBeVisible();
  await expect(page.getByLabel("宽高比")).toBeVisible();
  await expect(page.getByLabel("帧率")).toBeVisible();
  await page.getByRole("button", { name: "取消" }).click();
});

test("channel reconciliation page matches user structure and forbids estimate debit", async ({ page }) => {
  await page.route("**/channel/reconciliation**", async (route) => {
    // 页面 URL 与账本 API 同路径；只 stub fetch，别把 document/RSC 导航盖成 JSON。
    if (route.request().resourceType() !== "fetch" && route.request().resourceType() !== "xhr") {
      await route.continue();
      return;
    }
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        item: {
          buckets: { available_minor: 0, reserved_minor: 0, withdrawable_minor: 0 },
          usage_totals: { requests: 0, customer_minor: 0, charge_minor: 0, pending_count: 0 },
          items: [],
          pending: [],
        },
      }),
    });
  });
  await page.goto("/channel/reconciliation");
  await expect(page.getByRole("heading", { level: 1, name: "对账" })).toHaveCount(1);
  await expect(page.getByRole("heading", { name: "待对账", exact: true })).toHaveCount(0);
  await expect(page.getByRole("navigation", { name: "渠道控制台" }).getByRole("link", { name: "对账", exact: true })).toBeVisible();
  const channelBuckets = page.getByLabel("三桶");
  await expect(channelBuckets.getByText("余额", { exact: true })).toBeVisible();
  await expect(channelBuckets.getByText("冻结", { exact: true })).toBeVisible();
  await expect(channelBuckets.getByText("可提现", { exact: true })).toBeVisible();
  await expect(page.getByText("暂无 usage")).toBeVisible();
  await expect(page.getByRole("button", { name: /估扣|估算扣款/ })).toHaveCount(0);
  await expect(page.getByTestId("usage-trend-chart")).toHaveCount(0);
});

test("partner console shows scoped downline cards", async ({ page }) => {
  await page.goto("/partner");
  await expect(page.getByRole("heading", { name: "我的层级" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "范围内用户" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "范围内佣金" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "范围内结算" })).toBeVisible();
});

test("channel console shows scoped user list", async ({ page }) => {
  await page.goto("/channel");
  await expect(page.getByRole("heading", { name: "本渠道用户" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "本渠道模型" })).toBeVisible();
  await expect(page.getByText("本渠道只能使用平台已授权的模型，不能自己添加提供商和模型。")).toBeVisible();
  await expect(page.getByRole("heading", { name: "本渠道套餐" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "创建渠道套餐" })).toBeVisible();
  await expect(page.getByRole("button", { name: "创建渠道套餐" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "推广链接" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "本渠道归因" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "本渠道用量" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "按日用量" })).toBeVisible();
  await expect(page.getByTestId("usage-trend-chart")).toBeVisible();
  await expect(page.getByRole("heading", { name: "密钥汇总" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "本渠道结算" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "渠道额度与佣金" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "已发放额度" })).toBeVisible();
  await expect(page.getByText(/换算比 .+ BPS/)).toBeVisible();
});
