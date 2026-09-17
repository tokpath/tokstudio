import { expect, test, type Page, type Route } from "@playwright/test";

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
  await stubEmptyUserLists(page);
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
  await expect(page.getByRole("heading", { name: "第一次使用" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "直接体验模型" })).toBeVisible();
  const tryLink = page.getByRole("link", { name: "去快速试用" }).first();
  await expect(tryLink).toHaveAttribute("href", "/app/playground");
  await expect(page.getByRole("link", { name: "创建 API Key" }).first()).toHaveAttribute("href", "/app/keys?create=1");
  await expect(page.getByRole("heading", { name: "快捷入口" })).toHaveCount(0);
  await expect(page.getByRole("heading", { name: "媒体任务" })).toHaveCount(0);
  await expect(page.getByRole("heading", { name: "个人设置" })).toHaveCount(0);
  await expect(page.getByRole("heading", { name: "接入示例" })).toHaveCount(0);
});

test("user overview usage failure is not first-run", async ({ page }) => {
  await page.route("**/v1/me/usage**", (route) =>
    fulfillJSON(route, 500, { error: { message: "usage upstream timeout" } }),
  );
  await page.goto("/app");
  const usage = page.getByTestId("list-resource-overview-usage");
  await expect(usage).toHaveAttribute("data-list-phase", "error");
  await expect(usage.getByText("usage upstream timeout")).toBeVisible();
  await expect(page.getByRole("heading", { name: "第一次使用" })).toHaveCount(0);
});

test("user keys page keeps create dialog", async ({ page }) => {
  await page.goto("/app/keys");
  await expect(page.getByRole("heading", { level: 1, name: "API Key" })).toBeVisible();
  await expect(page.getByRole("button", { name: "创建 API Key" })).toBeVisible();
  await page.getByRole("button", { name: "创建 API Key" }).click();
  await expect(page.getByRole("heading", { name: "创建 API Key" })).toBeVisible();
  await expect(page.getByLabel("密钥名称")).toBeVisible();
  await expect(page.getByPlaceholder("我的聊天客户端")).toBeVisible();
  await expect(page.getByRole("button", { name: "高级设置" })).toBeVisible();
  await expect(page.getByLabel("每分钟最多请求数")).toHaveCount(0);
  await page.getByRole("button", { name: "高级设置" }).click();
  await expect(page.getByText("所有允许使用的模型", { exact: true })).toBeVisible();
  await expect(page.getByLabel("每分钟最多请求数")).toBeVisible();
  await expect(page.getByLabel("同时进行的请求数")).toBeVisible();
  await page.getByRole("button", { name: "取消" }).click();
});

test("user keys create query opens the dialog", async ({ page }) => {
  await page.goto("/app/keys?create=1");
  await expect(page.getByRole("heading", { name: "创建 API Key" })).toBeVisible();
  await expect(page.getByLabel("密钥名称")).toBeVisible();
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
              provider_id: "prd_echo",
              upstream_model_id: "echo-up",
              fact_source: "sandbox",
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
  await expect(page.getByRole("navigation", { name: "用户控制台" }).getByRole("link", { name: "成本/毛利" })).toHaveCount(0);
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
  await expect(page.getByText("prd_echo / echo-up / req_ok")).toBeVisible();
  await expect(page.getByText("缺上游元数据").first()).toBeVisible();
  await expect(page.getByText("openai")).toHaveCount(0);
});

test("user usage page is summary and links to activity", async ({ page }) => {
  await stubEmptyUserLists(page);
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

test("user usage failed load is not empty usage", async ({ page }) => {
  await page.route("**/v1/me/usage**", async (route) => {
    await route.fulfill({
      status: 500,
      contentType: "application/json",
      body: JSON.stringify({ error: { message: "usage upstream timeout" } }),
    });
  });
  await page.goto("/app/usage");
  await expect(page.getByTestId("list-resource-usage")).toHaveAttribute("data-list-phase", "error");
  await expect(page.getByTestId("list-resource-usage").getByText("usage upstream timeout")).toBeVisible();
  await expect(page.getByText("暂无用量")).toHaveCount(0);
});

test("usage summary carries filters into activity", async ({ page }) => {
  await page.route("**/v1/me/api-keys**", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ items: [{ id: "key_alpha", name: "alpha" }] }),
    });
  });
  await page.route("**/v1/me/usage**", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        items: [
          {
            id: "usg_ok",
            request_id: "req_ok",
            state: "confirmed",
            public_model_id: "tokenhub/echo-1",
            customer_amount_minor: 26,
            occurred_at: "2026-09-16T00:00:00.000Z",
          },
        ],
        keys: [],
        models: [{ key: "tokenhub/echo-1" }],
      }),
    });
  });
  await page.route("**/v1/me/requests**", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        items: [
          {
            id: "req_fail",
            request_id: "req_fail",
            result: "failed",
            error_code: "rate_limited",
            public_model_id: "tokenhub/echo-1",
            customer_amount_minor: 0,
            started_at: "2026-09-16T01:00:00.000Z",
          },
        ],
        keys: [],
        models: [{ key: "tokenhub/echo-1" }],
      }),
    });
  });
  await page.goto("/app/usage");
  await page.getByLabel("按模型筛选").selectOption("tokenhub/echo-1");
  await expect(page.getByRole("link", { name: "查看请求明细" })).toHaveAttribute("href", "/app/activity?model=tokenhub%2Fecho-1");
  await page.getByRole("link", { name: "查看请求明细" }).click();
  await expect(page).toHaveURL(/\/app\/activity\?model=tokenhub%2Fecho-1/);
  await expect(page.getByLabel("请求结果")).toBeVisible();
  await expect(page.getByLabel("计费状态")).toBeVisible();
  await page.getByLabel("请求结果").selectOption("failed");
  await expect(page).toHaveURL(/result=failed/);
  await expect(page).toHaveURL(/model=tokenhub%2Fecho-1/);
  await page.getByRole("button", { name: "查看详情" }).first().click();
  await expect(page.getByText("req_fail")).toBeVisible();
  await expect(page.getByText("rate_limited").first()).toBeVisible();
});

test("activity failed load stays failed and does not look empty", async ({ page }) => {
  await page.route("**/v1/me/requests**", async (route) => {
    await route.fulfill({
      status: 500,
      contentType: "application/json",
      body: JSON.stringify({ error: { message: "gateway list timeout" } }),
    });
  });
  await page.goto("/app/activity");
  await expect(page.getByTestId("list-resource")).toHaveAttribute("data-list-phase", "error");
  await expect(page.getByText("加载失败")).toBeVisible();
  await expect(page.getByText("gateway list timeout")).toBeVisible();
  await expect(page.getByRole("button", { name: "重试" })).toBeVisible();
  await expect(page.getByText("还没有请求记录")).toHaveCount(0);
});

test("activity session loss keeps the return path", async ({ page }) => {
  await page.route("**/v1/me/requests**", async (route) => {
    await route.fulfill({
      status: 401,
      contentType: "application/json",
      body: JSON.stringify({ error: { code: "authentication_error", message: "未登录" } }),
    });
  });
  await page.goto("/app/activity?result=failed");
  await expect(page.getByTestId("list-resource")).toHaveAttribute("data-list-phase", "unauthorized");
  await expect(page.getByRole("link", { name: "重新登录" })).toHaveAttribute(
    "href",
    "/login?next=%2Fapp%2Factivity%3Fresult%3Dfailed",
  );
  await expect(page.getByText("还没有请求记录")).toHaveCount(0);
});

test("activity forbidden stays on the page without a relogin link", async ({ page }) => {
  await page.route("**/v1/me/requests**", async (route) => {
    await route.fulfill({
      status: 403,
      contentType: "application/json",
      body: JSON.stringify({ error: { code: "permission_denied", message: "权限不足" } }),
    });
  });
  await page.goto("/app/activity");
  await expect(page.getByTestId("list-resource")).toHaveAttribute("data-list-phase", "unauthorized");
  await expect(page.getByRole("button", { name: "重试" })).toBeVisible();
  await expect(page.getByRole("link", { name: "重新登录" })).toHaveCount(0);
  await expect(page.getByText("还没有请求记录")).toHaveCount(0);
});

test("user media page is list-first with create dialog", async ({ page }) => {
  // 前端 job 无 Go API；先 mock 空列表，避免依赖 proxy :8080。
  await page.route("**/v1/me/media**", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ items: [], storage: { source: "minio", ok: true, label: "S3" } }),
    });
  });
  await page.goto("/app/media");
  await expect(page.getByRole("heading", { level: 1, name: "媒体任务" })).toBeVisible();
  await expect(page.getByText("存储源").first()).toBeVisible();
  await expect(page.getByTestId("storage-source-badge").first()).toHaveText("S3");
  await expect(page.getByTestId("storage-source-badge").first()).toHaveAttribute("data-ok", "true");
  await expect(page.getByText("✓")).toHaveCount(0);
  await expect(page.getByLabel("筛选媒体类型")).toBeVisible();
  await expect(page.getByRole("button", { name: "刷新任务" })).toBeVisible();
  await expect(page.getByRole("button", { name: "新建任务" }).first()).toBeVisible();
  await expect(page.getByText("暂无媒体任务")).toBeVisible();
  await page.getByRole("button", { name: "新建任务" }).first().click();
  await expect(page.getByRole("heading", { name: "新建任务" })).toBeVisible();
  await expect(page.getByRole("button", { name: "生成图片" })).toBeVisible();
  await expect(page.getByRole("button", { name: "生成视频" })).toBeVisible();
  await expect(page.getByLabel("描述你想生成的内容")).toBeVisible();
  await expect(page.getByLabel("模型")).toBeVisible();
  await expect(page.getByLabel("帧率")).toHaveCount(0);
  await expect(page.getByLabel("时长")).toHaveCount(0);
  await page.getByRole("button", { name: "生成视频" }).click();
  await expect(page.getByLabel("时长")).toBeVisible();
  await expect(page.getByLabel("帧率")).toHaveCount(0);
  await page.getByRole("button", { name: "高级设置" }).click();
  await expect(page.getByLabel("帧率")).toBeVisible();
  await page.getByRole("button", { name: "取消" }).click();
});

test("media create failure stays in the dialog", async ({ page }) => {
  await page.route("**/v1/me/media**", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ items: [], storage: { source: "minio", ok: true, label: "S3" } }),
    });
  });
  await page.route("**/v1/images/generations**", async (route) => {
    if (route.request().method() !== "POST") {
      await route.continue();
      return;
    }
    await route.fulfill({
      status: 502,
      contentType: "application/json",
      body: JSON.stringify({ error: { message: "上游创建失败" } }),
    });
  });
  await page.goto("/app/media");
  await page.getByRole("button", { name: "新建任务" }).first().click();
  await page.getByLabel("模型").fill("bytedance/seedream");
  await page.getByLabel("描述你想生成的内容").fill("a river at dusk");
  await page.getByRole("button", { name: "新建任务" }).last().click();
  await expect(page.getByTestId("submit-status")).toHaveText("上游创建失败");
  await expect(page.getByRole("heading", { name: "新建任务" })).toBeVisible();
});

test("media in-progress jobs update without a manual refresh", async ({ page }) => {
  await page.route("**/v1/me/media**", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        items: [
          {
            id: "vid_live",
            kind: "video",
            task_type: "t2v",
            status: "in_progress",
            model: "bytedance/seedance-1.0",
            prompt: "river",
          },
        ],
        storage: { source: "minio", ok: true, label: "S3" },
      }),
    });
  });
  await page.route("**/v1/videos/vid_live/content**", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ url: "https://cdn.test/river.mp4" }),
    });
  });
  await page.route("**/v1/videos/vid_live**", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        id: "vid_live",
        kind: "video",
        task_type: "t2v",
        status: "completed",
        model: "bytedance/seedance-1.0",
        prompt: "river",
      }),
    });
  });
  await page.goto("/app/media");
  await expect(page.getByTestId("media-job-vid_live")).toHaveAttribute("data-status", "completed");
  await expect(page.getByRole("button", { name: "再次使用此配置" })).toBeVisible();
});

test("wallet payment methods distinguish load failure from not enabled", async ({ page }) => {
  await page.route("**/v1/payments/checkout**", async (route) => {
    await route.fulfill({
      status: 502,
      contentType: "application/json",
      body: JSON.stringify({ error: { message: "checkout unavailable" } }),
    });
  });
  await page.goto("/app/wallet");
  await expect(page.getByTestId("list-resource-payments")).toHaveAttribute("data-list-phase", "error");
  await expect(page.getByText("加载失败").first()).toBeVisible();
  await expect(page.getByText("checkout unavailable")).toBeVisible();
  await expect(page.getByText("当前渠道尚未开通在线支付")).toHaveCount(0);
});

test("wallet quote ignores stale results and does not show zero while calculating", async ({ page }) => {
  let releaseSlow: (() => void) | undefined;
  const slow = new Promise<void>((resolve) => {
    releaseSlow = resolve;
  });
  await page.route("**/v1/me/balance**", async (route) => {
    await fulfillJSON(route, 200, { balance: { available: "0", reserved: "0" } });
  });
  await page.route("**/v1/me/ledger**", async (route) => {
    await fulfillJSON(route, 200, { items: [] });
  });
  await page.route("**/v1/payments/checkout**", async (route) => {
    await fulfillJSON(route, 200, {
      item: {
        methods: [
          { adapter: "alipay", display_name: "支付宝", pay_currency: "CNY", sandbox: true },
          { adapter: "stripe", display_name: "Stripe", pay_currency: "USD", sandbox: true },
        ],
        settings: { quick_amounts: [100, 300] },
      },
    });
  });
  await page.route("**/v1/payments/quote**", async (route) => {
    const url = new URL(route.request().url());
    const major = url.searchParams.get("pay_major");
    const adapter = url.searchParams.get("adapter");
    if (major === "100" && adapter === "alipay") {
      await slow;
      await fulfillJSON(route, 200, {
        item: {
          adapter: "alipay",
          pay_major: 100,
          pay_currency: "CNY",
          pay_minor: 10000,
          fee_minor: 0,
          credit_minor: 13990000,
        },
      });
      return;
    }
    await fulfillJSON(route, 200, {
      item: {
        adapter,
        pay_major: Number(major),
        pay_currency: adapter === "stripe" ? "USD" : "CNY",
        pay_minor: adapter === "stripe" ? Number(major) * 1_000_000 : Number(major) * 100,
        fee_minor: 0,
        credit_minor: Number(major) === 300 ? 41970000 : 10000000,
      },
    });
  });
  await page.goto("/app/wallet");
  await expect(page.getByRole("button", { name: "¥100" })).toBeVisible();
  await expect(page.getByTestId("quote-summary")).toHaveAttribute("data-quote-phase", "loading");
  await expect(page.getByText("正在计算").first()).toBeVisible();
  await expect(page.getByTestId("wallet-pay")).toBeDisabled();
  await expect(page.getByTestId("quote-summary")).not.toContainText("$0.00");
  await expect(page.getByTestId("quote-summary")).not.toContainText("¥0.00");
  await page.getByRole("button", { name: "¥300" }).click();
  await expect(page.getByTestId("wallet-pay")).toHaveText("支付 ¥300.00");
  await expect(page.getByTestId("wallet-pay")).toBeEnabled();
  releaseSlow?.();
  await expect(page.getByTestId("wallet-pay")).toHaveText("支付 ¥300.00");
  await expect(page.getByTestId("quote-summary").getByText("$41.97")).toBeVisible();
});

test("media download badge is grey 存储不可用 when the bucket is missing", async ({ page }) => {
  await page.route("**/v1/me/media**", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        items: [{ id: "vid_missing", kind: "video", status: "completed", model: "bytedance/seedance-1.0" }],
        storage: { source: "unavailable", ok: false, label: "存储不可用", detail: "missing bucket" },
      }),
    });
  });
  await page.route("**/v1/videos/vid_missing/content**", async (route) => {
    await route.fulfill({
      status: 503,
      contentType: "application/json",
      body: JSON.stringify({
        error: { code: "store_unavailable", message: "存储不可用" },
        storage: { source: "unavailable", ok: false, label: "存储不可用" },
      }),
    });
  });
  await page.goto("/app/media");
  await expect(page.getByRole("button", { name: "下载" })).toBeVisible();
  await expect(page.getByRole("button", { name: "再次使用此配置" })).toBeVisible();
  await expect(page.getByTestId("storage-source-badge").first()).toHaveText("存储不可用");
  await expect(page.getByTestId("storage-source-badge").first()).toHaveAttribute("data-ok", "false");
  await expect(page.getByText("✓")).toHaveCount(0);
  await page.getByRole("button", { name: "下载" }).click();
  await expect(page.getByText("存储不可用").first()).toBeVisible();
});

test("channel OEM brand download shows muted S3 and never a success check", async ({ page }) => {
  await page.route("**/channel/brand**", async (route) => {
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
        brand: { name: "OEM C", logo_url: "/v1/public/brand-assets/bas_logo", theme: { brand: "#2150D6" } },
        customizable: true,
        storage: { source: "minio", ok: true, label: "S3" },
      }),
    });
  });
  await page.goto("/channel/brand");
  await expect(page.getByRole("heading", { name: "本渠道品牌" })).toBeVisible();
  await expect(page.getByText("存储源").first()).toBeVisible();
  await expect(page.getByTestId("storage-source-badge").first()).toHaveText("S3");
  await expect(page.getByTestId("storage-source-badge").first()).toHaveAttribute("data-tone", "muted");
  await expect(page.getByRole("link", { name: "下载 Logo" })).toBeVisible();
  await expect(page.getByText("✓")).toHaveCount(0);
});

test("channel OEM brand upload failure is grey 存储不可用", async ({ page }) => {
  await page.route("**/channel/brand/assets**", async (route) => {
    if (route.request().resourceType() !== "fetch" && route.request().resourceType() !== "xhr") {
      await route.continue();
      return;
    }
    await route.fulfill({
      status: 503,
      contentType: "application/json",
      body: JSON.stringify({ error: { code: "store_unavailable", message: "存储不可用" } }),
    });
  });
  await page.route("**/channel/brand**", async (route) => {
    if (route.request().resourceType() !== "fetch" && route.request().resourceType() !== "xhr") {
      await route.continue();
      return;
    }
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        brand: { name: "OEM C", theme: { brand: "#2150D6" } },
        customizable: true,
        storage: { source: "unavailable", ok: false, label: "存储不可用", detail: "missing bucket" },
      }),
    });
  });
  await page.goto("/channel/brand");
  await expect(page.getByTestId("storage-source-badge").first()).toHaveText("存储不可用");
  await expect(page.getByTestId("storage-source-badge").first()).toHaveAttribute("data-ok", "false");
  await expect(page.getByText("✓")).toHaveCount(0);
  await expect(page.getByText("已上传")).toHaveCount(0);
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
  await expect(page.getByRole("navigation", { name: "渠道控制台" }).getByRole("link", { name: "成本/毛利" })).toHaveCount(0);
  const channelBuckets = page.getByLabel("三桶");
  await expect(channelBuckets.getByText("余额", { exact: true })).toBeVisible();
  await expect(channelBuckets.getByText("冻结", { exact: true })).toBeVisible();
  await expect(channelBuckets.getByText("可提现", { exact: true })).toBeVisible();
  await expect(page.getByText("暂无 usage")).toBeVisible();
  await expect(page.getByRole("button", { name: /估扣|估算扣款/ })).toHaveCount(0);
  await expect(page.getByTestId("usage-trend-chart")).toHaveCount(0);
  await expect(page.getByTestId("upstream-facts-badge")).toHaveCount(0);
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
  await expect(page.getByText("所有租户的模型资源都只能从平台目录出发。渠道不能自建提供商或模型，也不能引入目录外的模型。")).toBeVisible();
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

async function fulfillJSON(route: Route, status: number, body: unknown) {
  if (route.request().resourceType() !== "fetch" && route.request().resourceType() !== "xhr") {
    await route.continue();
    return;
  }
  await route.fulfill({
    status,
    contentType: "application/json",
    body: JSON.stringify(body),
  });
}

async function stubEmptyUserLists(page: Page) {
  await page.route("**/v1/me/balance**", (route) => fulfillJSON(route, 200, { balance: { available: "12.00", reserved: "0" } }));
  await page.route("**/v1/me/api-keys**", (route) => fulfillJSON(route, 200, { items: [] }));
  await page.route("**/v1/me/usage**", (route) => fulfillJSON(route, 200, { items: [], keys: [], models: [] }));
  await page.route("**/v1/me/requests**", (route) => fulfillJSON(route, 200, { items: [], keys: [], models: [] }));
}

test("user shell shows real available balance and profile dropdown", async ({ page }) => {
  await page.route("**/v1/me/balance**", async (route) => {
    await fulfillJSON(route, 200, { balance: { available: "12.5", reserved: "1.00", gift_minor: 0, commission_available_minor: 0 } });
  });
  await page.route("**/v1/me/api-keys**", async (route) => {
    await fulfillJSON(route, 200, { items: [] });
  });
  await page.route("**/v1/auth/logout", async (route) => {
    await fulfillJSON(route, 200, { ok: true });
  });
  await page.route("**/v1/me", async (route) => {
    await fulfillJSON(route, 200, {
      user: { display_name: "Ada", email: "ada@example.test", roles: ["end_user"], login_methods: ["password"] },
    });
  });
  await page.goto("/app");
  await expect(page.getByTestId("shell-bell")).toBeDisabled();
  const pill = page.getByTestId("balance-pill");
  await expect(pill).toHaveText("$12.50");
  await expect(pill).toHaveAttribute("data-field", "available");
  await expect(pill).toHaveAttribute("href", "/app/wallet");
  await expect(page.getByText("$0.00")).toHaveCount(0);
  await page.getByTestId("avatar-trigger").click();
  await expect(page.getByTestId("menu-display-name")).toHaveText("Ada");
  await expect(page.getByTestId("menu-email")).toHaveText("ada@example.test");
  await expect(page.getByRole("menuitem", { name: "个人资料" })).toBeVisible();
  await expect(page.getByRole("menuitem", { name: "API 密钥" })).toBeVisible();
  await expect(page.getByRole("menuitem", { name: "退出登录" })).toBeVisible();
  await expect(page.getByRole("menuitem", { name: "平台管理" })).toHaveCount(0);
  await expect(page.getByText("GitHub")).toHaveCount(0);
  await expect(page.getByText("新手引导")).toHaveCount(0);
  await page.getByRole("menuitem", { name: "个人资料" }).click();
  await expect(page).toHaveURL(/\/app\/profile/);
  await expect(page.getByRole("heading", { name: "个人资料" })).toBeVisible();
  await expect(page.getByTestId("profile-display-name")).toHaveText("Ada");
  await expect(page.getByTestId("profile-email")).toHaveText("ada@example.test");
  await expect(page.getByTestId("profile-login-methods").locator("[data-method=password]")).toBeVisible();
});

test("user shell balance failure is — never fake $0.00", async ({ page }) => {
  await page.route("**/v1/me/balance**", async (route) => {
    await fulfillJSON(route, 503, { error: { message: "余额不可用" } });
  });
  await page.route("**/v1/me", async (route) => {
    await fulfillJSON(route, 200, { user: { display_name: "", email: "", roles: ["end_user"] } });
  });
  await page.goto("/app");
  const pill = page.getByTestId("balance-pill");
  await expect(pill).toHaveAttribute("data-state", "error");
  await expect(pill).toHaveText("—");
  await expect(page.getByText("$0.00")).toHaveCount(0);
  await page.getByTestId("avatar-trigger").click();
  await expect(page.getByTestId("menu-display-name")).toHaveText("—");
  await expect(page.getByTestId("menu-email")).toHaveText("—");
});

test("user keys empty state is honest 暂无 API 密钥", async ({ page }) => {
  await page.route("**/v1/me/api-keys**", async (route) => {
    await fulfillJSON(route, 200, { items: [] });
  });
  await page.route("**/v1/me", async (route) => {
    await fulfillJSON(route, 200, { user: { display_name: "Ada", email: "ada@example.test", roles: ["end_user"] } });
  });
  await page.goto("/app/keys");
  await expect(page.getByText("暂无 API 密钥")).toBeVisible();
  await expect(page.getByText("thk_")).toHaveCount(0);
});

test("user keys failed load is not an empty key list", async ({ page }) => {
  await page.route("**/v1/me/api-keys**", async (route) => {
    await fulfillJSON(route, 500, { error: { message: "keys down" } });
  });
  await page.goto("/app/keys");
  await expect(page.getByTestId("list-resource-keys")).toHaveAttribute("data-list-phase", "error");
  await expect(page.getByText("keys down")).toBeVisible();
  await expect(page.getByText("暂无 API 密钥")).toHaveCount(0);
});

test("user shell shows 平台管理 only for platform_admin", async ({ page }) => {
  await page.route("**/v1/me/balance**", async (route) => {
    await fulfillJSON(route, 200, { balance: { available: "12.5" } });
  });
  await page.route("**/v1/me", async (route) => {
    await fulfillJSON(route, 200, {
      user: { display_name: "Pat", email: "pat@example.test", roles: ["platform_admin"] },
    });
  });
  await page.goto("/app");
  await page.getByTestId("avatar-trigger").click();
  const adminItem = page.getByRole("menuitem", { name: "平台管理" });
  await expect(adminItem).toBeVisible();
  await expect(adminItem).toHaveAttribute("href", "/admin");
  await expect(adminItem).not.toHaveAttribute("aria-disabled", "true");
});

test("user shell logout clears session and returns to login", async ({ page }) => {
  await page.route("**/v1/me/balance**", async (route) => {
    await fulfillJSON(route, 200, { balance: { available: "1" } });
  });
  await page.route("**/v1/auth/logout", async (route) => {
    await fulfillJSON(route, 200, { ok: true });
  });
  await page.route("**/v1/me", async (route) => {
    await fulfillJSON(route, 200, { user: { display_name: "Ada", email: "ada@example.test", roles: ["end_user"] } });
  });
  await page.goto("/app");
  await page.getByTestId("avatar-trigger").click();
  await page.getByRole("menuitem", { name: "退出登录" }).click();
  await expect(page).toHaveURL(/\/login/);
});

test("narrow console uses a drawer and labels unavailable settings at the entry", async ({ page }) => {
  await page.setViewportSize({ width: 375, height: 812 });
  await page.goto("/app");
  await expect(page.getByRole("button", { name: "打开导航" })).toBeVisible();
  const noPageOverflow = await page.evaluate(
    () => document.documentElement.scrollWidth <= document.documentElement.clientWidth + 1,
  );
  expect(noPageOverflow).toBeTruthy();
  await page.getByRole("button", { name: "打开导航" }).click();
  const drawer = page.getByRole("dialog");
  await expect(drawer.getByRole("link", { name: "总览" })).toBeVisible();
  await drawer.getByRole("link", { name: "设置" }).click();
  await expect(page.getByRole("link", { name: "账户" })).toBeVisible();
  await expect(page.getByRole("link", { name: "团队" })).toHaveCount(0);
  await expect(page.getByText("未开放").first()).toBeVisible();
  await page.goto("/app/settings/team");
  await expect(page.getByRole("heading", { name: "团队" })).toBeVisible();
  await expect(page.getByText("这个功能还没开放")).toBeVisible();
  await expect(page.getByRole("button", { name: /重命名/ })).toHaveCount(0);
});

