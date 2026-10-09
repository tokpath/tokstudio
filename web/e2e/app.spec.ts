import { expect, test, type Page, type Route } from "@playwright/test";
import { mockViewer } from "./mock-viewer";

test.beforeEach(async ({ page }, testInfo) => {
  if (testInfo.title.startsWith("public ")) return;
  await mockViewer(page, { roles: [testInfo.title.startsWith("channel ") ? "channel_admin" : "end_user"], partner: testInfo.title.startsWith("partner ") });
});

test("public storefront shows actual tasks and sends purchases through login", async ({ page }) => {
  await page.goto("/?promo=APPINVITE");
  await expect(page.getByRole("heading", { name: "当前品牌可用模型", exact: true })).toBeVisible();
  await expect(page.getByRole("heading", { name: "当前品牌套餐", exact: true })).toBeVisible();
  await expect(page.getByRole("heading", { name: "按需充值", exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "兑换码充值" })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "创建支付充值" })).toHaveCount(0);
  const browse = new URL(await page.getByRole("link", { name: "浏览模型与价格", exact: true }).getAttribute("href") as string, "https://brand.test");
  expect(browse.pathname).toBe("/models");
  expect(browse.searchParams.get("promotion_code")).toBe("APPINVITE");
  await page.getByRole("link", { name: "创建 API Key", exact: true }).click();
  await expect(page).toHaveURL(/\/login\?/);
  const login = new URL(page.url());
  expect(login.pathname).toBe("/login");
  expect(login.searchParams.get("next")).toBe("/app/keys");
  expect(login.searchParams.get("promotion_code")).toBe("APPINVITE");
  await expect(page.getByRole("heading", { name: "注册 / 登录", exact: true })).toBeVisible();
});

test("user entry starts with Key creation and visible use limits", async ({ page }) => {
  await stubEmptyUserLists(page);
  await page.goto("/app");
  await expect(page.getByRole("navigation", { name: "用户控制台", exact: true })).toBeVisible();
  await expect(page.getByRole("heading", { level: 1, name: "API Key", exact: true })).toBeVisible();
  await expect(page.getByText("暂无 API 密钥", { exact: true })).toBeVisible();
  await expect(page.getByRole("heading", { name: "第一次使用", exact: true })).toHaveCount(0);
  await expect(page.getByRole("heading", { name: "直接体验模型", exact: true })).toHaveCount(0);
  await page.getByRole("button", { name: "创建 API Key", exact: true }).first().click();
  const dialog = page.getByRole("dialog");
  await expect(dialog.getByLabel("密钥名称", { exact: true })).toBeVisible();
  await expect(dialog.getByRole("group", { name: "可用模型", exact: true })).toBeVisible();
  await expect(dialog.getByRole("group", { name: "USD 消耗上限", exact: true })).toBeVisible();
  await expect(dialog.getByRole("group", { name: "有效期", exact: true })).toBeVisible();
  await expect(dialog.getByRole("radio", { name: "所有可用模型", exact: true })).toBeChecked();
  await expect(dialog.getByRole("radio", { name: "不设单独上限", exact: true })).toBeChecked();
  await expect(dialog.getByRole("radio", { name: "长期有效", exact: true })).toBeChecked();
});

test("user entry retains existing Keys when unrelated usage is unavailable", async ({ page }) => {
  let usageReads = 0;
  await page.route("**/v1/me/usage**", (route) => { usageReads++; return fulfillJSON(route, 500, { error: { message: "usage upstream timeout" } }); });
  await page.route("**/v1/me/api-keys**", (route) => fulfillJSON(route, 200, { items: [{ id: "key_existing", name: "Existing production Key", prefix: "thk_existing", status: "active", model_mode: "all", budget_limit_minor: null }] }));
  await page.goto("/app");
  await expect(page.getByRole("table").getByText("Existing production Key", { exact: true })).toBeVisible();
  await expect(page.getByText("暂无 API 密钥", { exact: true })).toHaveCount(0);
  await expect(page.getByRole("heading", { name: "第一次使用", exact: true })).toHaveCount(0);
  expect(usageReads).toBe(0);
});

test("user keys create dialog exposes model USD and expiry before advanced rate controls", async ({ page }) => {
  await stubEmptyUserLists(page);
  await page.goto("/app/keys");
  await expect(page.getByRole("heading", { level: 1, name: "API Key", exact: true })).toBeVisible();
  await page.getByRole("button", { name: "创建 API Key", exact: true }).first().click();
  const dialog = page.getByRole("dialog");
  await expect(dialog.getByRole("heading", { name: "创建 API Key", exact: true })).toBeVisible();
  await expect(dialog.getByLabel("密钥名称", { exact: true })).toBeVisible();
  await expect(dialog.getByPlaceholder("我的聊天客户端")).toBeVisible();
  await expect(dialog.getByRole("group", { name: "可用模型", exact: true })).toBeVisible();
  await expect(dialog.getByRole("group", { name: "USD 消耗上限", exact: true })).toBeVisible();
  await expect(dialog.getByRole("group", { name: "有效期", exact: true })).toBeVisible();
  await dialog.getByRole("radio", { name: "设置累计总上限", exact: true }).check();
  await dialog.getByLabel("USD", { exact: true }).fill("10");
  await dialog.getByRole("radio", { name: "指定到期时间", exact: true }).check();
  await expect(dialog.locator('input[type="datetime-local"]')).toBeVisible();
  await expect(dialog.getByLabel("每分钟最多请求数", { exact: true })).toBeHidden();
  await dialog.getByText("高级设置", { exact: true }).click();
  await expect(dialog.getByLabel("每分钟最多请求数", { exact: true })).toBeVisible();
  await expect(dialog.getByLabel("同时进行的请求数", { exact: true })).toBeVisible();
  await dialog.getByRole("button", { name: "取消", exact: true }).click();
});

test("user keys create query opens the dialog", async ({ page }) => {
  await page.goto("/app/keys?create=1");
  await expect(page.getByRole("heading", { name: "创建 API Key" })).toBeVisible();
  await expect(page.getByLabel("密钥名称")).toBeVisible();
});

test("user reconciliation alias opens original pending requests without estimated debit controls", async ({ page }) => {
  await stubEmptyUserLists(page);
  const request = requestReceipt("req_gap", "pending_reconciliation");
  await page.route("**/v1/me/requests?**", (route) => fulfillJSON(route, 200, { items: [request], next_cursor: "" }));
  await page.route("**/v1/me/requests/req_gap", (route) => fulfillJSON(route, 200, { request, authorization: { id: "auth_gap", status: "pending_reconciliation", amount_minor: 1000000 }, charges: [], attempts: [], usage: [{ id: "usage_gap", request_id: "req_gap", state: "pending_reconciliation", customer_amount_minor: 0 }] }));
  await page.goto("/app/reconciliation?model=tokenhub%2Fecho-1");
  await expect(page).toHaveURL(/\/app\/usage\?/);
  expect(new URL(page.url()).searchParams.get("tab")).toBe("requests");
  expect(new URL(page.url()).searchParams.get("state")).toBe("pending_reconciliation");
  await expect(page.getByText("req_gap", { exact: true })).toBeVisible();
  await page.locator('a[href^="/app/usage/requests/req_gap"]').click();
  await expect(page.getByRole("heading", { level: 1, name: "请求详情", exact: true })).toBeVisible();
  await expect(page.getByText("未产生消费账单", { exact: true })).toBeVisible();
  await expect(page.getByRole("heading", { name: "处理本次核对", exact: true })).toHaveCount(0);
  await expect(page.getByRole("button", { name: /估扣|估算扣款|释放原预授权|录入本次真实用量/ })).toHaveCount(0);
  await expect(page.getByText(/prd_echo|echo-up/)).toHaveCount(0);
  const back = new URL(await page.getByRole("link", { name: "← 返回原任务", exact: true }).getAttribute("href") as string, "https://brand.test");
  expect(back.pathname).toBe("/app/usage");
  expect(back.searchParams.get("public_model_id") || back.searchParams.get("model")).toBe("tokenhub/echo-1");
  expect(back.searchParams.get("state")).toBe("pending_reconciliation");
});

test("user usage summary covers all records and switches to request records", async ({ page }) => {
  await stubEmptyUserLists(page);
  await page.route("**/v1/me/usage/summary**", (route) => fulfillJSON(route, 200, usageSummary(151, 25000000)));
  await page.goto("/app/usage");
  await expect(page.getByTestId("usage-stat-requests")).toHaveText("151");
  await expect(page.getByTestId("usage-stat-spend")).toHaveText("$25.00");
  await expect(page.getByLabel("API Key", { exact: true })).toBeVisible();
  await expect(page.getByRole("heading", { name: "每日用量", exact: true })).toBeVisible();
  await expect(page.getByTestId("usage-trend-chart")).toBeVisible();
  await expect(page.getByRole("heading", { name: "按 API Key", exact: true })).toBeVisible();
  await page.getByRole("navigation", { name: "用量视图", exact: true }).getByRole("button", { name: "请求记录", exact: true }).click();
  expect(new URL(page.url()).searchParams.get("tab")).toBe("requests");
  await expect(page.getByText("暂无请求", { exact: true })).toBeVisible();
});

test("user usage failed summary retains unknown amounts rather than empty or zero", async ({ page }) => {
  await page.route("**/v1/me/usage/summary**", (route) => fulfillJSON(route, 500, { error: { message: "usage upstream timeout" } }));
  await page.goto("/app/usage");
  const summary = page.getByTestId("list-resource-usage");
  await expect(summary).toHaveAttribute("data-list-phase", "error");
  await expect(summary.getByRole("alert")).toBeVisible();
  await expect(summary.getByRole("button", { name: "重试读取", exact: true })).toBeVisible();
  await expect(page.getByTestId("usage-stat-requests")).toHaveText("—");
  await expect(page.getByTestId("usage-stat-spend")).toHaveText("—");
  await expect(page.getByText("暂无用量", { exact: true })).toHaveCount(0);
});

test("usage filters and cursor survive request details and return", async ({ page }) => {
  const queries: URL[] = [];
  await page.route("**/v1/me/usage/summary**", (route) => fulfillJSON(route, 200, usageSummary(151, 25000000)));
  await page.route("**/v1/me/requests?**", (route) => {
    const url = new URL(route.request().url()); queries.push(url);
    const start = url.searchParams.get("cursor") === "page-2" ? 26 : 1;
    return fulfillJSON(route, 200, { items: Array.from({ length: 25 }, (_, index) => requestReceipt(`req-${start + index}`, "confirmed")), next_cursor: start === 1 ? "page-2" : "" });
  });
  const request = requestReceipt("req-26", "confirmed");
  await page.route("**/v1/me/requests/req-26", (route) => fulfillJSON(route, 200, { request, authorization: null, charges: [{ charge_id: "charge-26", state: "committed", amount_minor: 100000 }], attempts: [{ id: "attempt-26", attempt_no: 1, status: "failed", latency_ms: 12, error_code: "rate_limited" }], usage: [] }));
  await page.goto("/app/usage?from=2026-10-01&to=2026-10-10");
  await page.getByLabel("模型", { exact: true }).selectOption("tokenhub/echo-1");
  await page.getByLabel("API Key", { exact: true }).selectOption("key_alpha");
  await page.getByLabel("账务状态", { exact: true }).selectOption("confirmed");
  await page.getByRole("navigation", { name: "用量视图", exact: true }).getByRole("button", { name: "请求记录", exact: true }).click();
  await page.getByLabel("执行结果", { exact: true }).selectOption("failed");
  await expect(page.getByText("req-1", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "下一页", exact: true }).click();
  await expect(page.getByText("req-26", { exact: true })).toBeVisible();
  const original = new URL(page.url()).pathname + new URL(page.url()).search;
  await page.locator('a[href^="/app/usage/requests/req-26"]').click();
  await expect(page).toHaveURL(/\/app\/usage\/requests\/req-26\?/);
  expect(new URL(page.url()).searchParams.get("return_to")).toBe(original);
  await expect(page.getByText("rate_limited", { exact: true })).toBeVisible();
  await expect(page.getByRole("link", { name: "冲正本次消费", exact: true })).toHaveCount(0);
  await page.getByRole("link", { name: "← 返回原任务", exact: true }).click();
  await expect(page).toHaveURL(/\/app\/usage\?/);
  await expect(page.getByText("req-26", { exact: true })).toBeVisible();
  await page.reload();
  await expect(page.getByText("req-26", { exact: true })).toBeVisible();
  const restored = new URL(page.url());
  expect(restored.searchParams.get("public_model_id")).toBe("tokenhub/echo-1");
  expect(restored.searchParams.get("api_key_id")).toBe("key_alpha");
  expect(restored.searchParams.get("state")).toBe("confirmed");
  expect(restored.searchParams.get("result")).toBe("failed");
  expect(queries.some(url => url.searchParams.get("cursor") === "page-2" && url.searchParams.get("public_model_id") === "tokenhub/echo-1" && url.searchParams.get("api_key_id") === "key_alpha" && url.searchParams.get("state") === "confirmed" && url.searchParams.get("result") === "failed")).toBe(true);
});

test("activity alias request failure stays failed and does not look empty", async ({ page }) => {
  await stubEmptyUserLists(page);
  await page.route("**/v1/me/requests?**", (route) => fulfillJSON(route, 500, { error: { message: "gateway list timeout" } }));
  await page.goto("/app/activity");
  await expect(page).toHaveURL(/\/app\/usage\?/);
  await expect(page.getByText("加载失败", { exact: true }).first()).toBeVisible();
  await expect(page.getByText("暂无请求", { exact: true })).toHaveCount(0);
  await expect(page.getByText("还没有请求记录", { exact: true })).toHaveCount(0);
});

test("activity alias session loss keeps the full filtered return path", async ({ page }) => {
  await stubEmptyUserLists(page);
  await page.route("**/v1/me/requests?**", (route) => fulfillJSON(route, 401, { error: { code: "authentication_error", message: "未登录" } }));
  await page.goto("/app/activity?result=failed&model=tokenhub%2Fecho-1");
  const relogin = page.getByRole("link", { name: "重新登录", exact: true });
  await expect(relogin).toBeVisible();
  const next = new URL(await relogin.getAttribute("href") as string, "https://brand.test").searchParams.get("next");
  const restored = new URL(next as string, "https://brand.test");
  expect(restored.pathname).toBe("/app/usage");
  expect(restored.searchParams.get("tab")).toBe("requests");
  expect(restored.searchParams.get("result")).toBe("failed");
  expect(restored.searchParams.get("model") || restored.searchParams.get("public_model_id")).toBe("tokenhub/echo-1");
  await expect(page.getByText("暂无请求", { exact: true })).toHaveCount(0);
});

test("activity alias forbidden stays in scope without a relogin or empty result", async ({ page }) => {
  await stubEmptyUserLists(page);
  await page.route("**/v1/me/requests?**", (route) => fulfillJSON(route, 403, { error: { code: "permission_denied", message: "权限不足" } }));
  await page.goto("/app/activity");
  await expect(page).toHaveURL(/\/app\/usage\?/);
  await expect(page.getByRole("alert").filter({ hasText: "没有权限查看" })).toBeVisible();
  await expect(page.getByRole("link", { name: "重新登录", exact: true })).toHaveCount(0);
  await expect(page.getByText("暂无请求", { exact: true })).toHaveCount(0);
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
  const quoteBeforePay = await page.evaluate(() => {
    const quote = document.querySelector("[data-testid=quote-summary]");
    const pay = document.querySelector("[data-testid=wallet-pay]");
    return Boolean(quote && pay && quote.compareDocumentPosition(pay) & Node.DOCUMENT_POSITION_FOLLOWING);
  });
  expect(quoteBeforePay).toBeTruthy();
  await expect(page.getByLabel("兑换码")).toBeVisible();
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

test("channel OEM pending queue is scoped and read only with original request access", async ({ page }) => {
  await mockViewer(page, { roles: ["channel_admin"], channelType: "C" });
  const gap = { id: "usage_oem", request_id: "req_oem", public_model_id: "tokenhub/echo-1", state: "pending_reconciliation", reserved_minor: 1000000, occurred_at: "2026-10-10T00:00:00Z" };
  await page.route("**/api/channel/usage/pending?**", (route) => fulfillJSON(route, 200, { items: [gap], next_cursor: "" }));
  await page.route("**/api/channel/usage/pending/usage_oem", (route) => fulfillJSON(route, 200, { item: gap }));
  await page.goto("/channel/reconciliation");
  await expect(page.getByRole("heading", { level: 1, name: "待核对请求", exact: true })).toBeVisible();
  await expect(page.getByText("req_oem", { exact: true })).toBeVisible();
  await expect(page.getByText("请核对原请求并交平台有权岗位处理预授权或补齐账单。", { exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: /估扣|估算扣款|释放所选预授权|录入本次真实用量/ })).toHaveCount(0);
  await page.getByRole("row").filter({ hasText: "req_oem" }).click();
  const request = page.getByRole("link", { name: "打开原请求", exact: true });
  await expect(request).toBeVisible();
  const href = new URL(await request.getAttribute("href") as string, "https://brand.test");
  expect(href.pathname).toBe("/channel/usage/requests/req_oem");
  expect(href.searchParams.get("return_to")).toBe("/channel/reconciliation");
});

test("partner legacy entry opens personal invitations without a second account", async ({ page }) => {
  await page.route("**/v1/me/referral?**", (route) => fulfillJSON(route, 200, personalReferral()));
  await page.goto("/partner");
  await expect(page).toHaveURL(/\/app\/referral$/);
  await expect(page.getByRole("heading", { level: 1, name: "邀请与收益", exact: true })).toBeVisible();
  await expect(page.getByText("PERSONAL", { exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "范围内用户", exact: true })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "范围内佣金", exact: true })).toHaveCount(0);
});

test("channel home directs customer and promotion work without financial tasks", async ({ page }) => {
  await mockViewer(page, { roles: ["channel_admin"], channelType: "B" });
  await page.route("**/api/channel/me", route => fulfillJSON(route, 200, { channel_org_id: "chn_b", channel_type: "B", channel_name: "Referral channel", brand_name: "Current brand" }));
  await page.route("**/v1/me/referral", route => fulfillJSON(route, 200, personalReferral()));
  await page.route("**/api/channel/customer-scopes", route => fulfillJSON(route, 200, { items: [] }));
  await page.route("**/api/channel/customers?**", route => fulfillJSON(route, 200, { items: [], total: 0, limit: 25, next_cursor: "" }));
  await page.goto("/channel");
  await expect(page.getByRole("heading", { level: 1, name: "渠道工作台", exact: true })).toBeVisible();
  const tasks = page.getByRole("region", { name: "经营任务", exact: true });
  for (const path of ["users", "promos", "models", "commissions"]) await expect(tasks.locator(`a[href="/channel/${path}"]`)).toBeVisible();
  for (const path of ["payments", "ledger", "plans", "reconciliation"]) await expect(tasks.locator(`a[href="/channel/${path}"]`)).toHaveCount(0);
  await expect(page.getByRole("button", { name: "创建渠道套餐" })).toHaveCount(0);
  await tasks.locator('a[href="/channel/users"]').click();
  await expect(page).toHaveURL(/\/channel\/users$/);
  await expect(page.getByRole("button", { name: "刷新", exact: true })).toBeVisible();
  await page.goto("/channel/plans");
  await expect(page.getByTestId("console-access")).toBeVisible();
  await expect(page.getByRole("button", { name: "创建渠道套餐" })).toHaveCount(0);
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
  await page.route("**/v1/me/usage/summary**", (route) => fulfillJSON(route, 200, usageSummary()));
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
      user: { id: "usr_rbac", display_name: "Ada", email: "ada@example.test", roles: ["end_user"], login_methods: ["password"] },
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
  await expect(page).toHaveURL(/\/app\/settings$/);
  await expect(page.getByRole("heading", { level: 1, name: "账户", exact: true })).toBeVisible();
  await expect(page.getByRole("textbox", { name: "显示名", exact: true })).toHaveValue("Ada");
  await expect(page.getByRole("main")).toContainText("ada@example.test");
  await expect(page.getByRole("button", { name: "修改密码", exact: true })).toBeVisible();
  await expect(page.getByLabel("所属渠道", { exact: true })).toHaveCount(0);
  await page.goto("/app/profile");
  await expect(page.getByRole("heading", { level: 1, name: "个人资料", exact: true })).toBeVisible();
  await expect(page.getByTestId("profile-display-name")).toHaveText("Ada");
  await expect(page.getByTestId("profile-email")).toHaveText("ada@example.test");
  await expect(page.getByTestId("profile-login-methods").locator("[data-method=password]")).toBeVisible();
  await expect(page.getByRole("button", { name: "保存资料", exact: true })).toHaveCount(0);
});

test("user shell balance failure is — never fake $0.00", async ({ page }) => {
  await page.route("**/v1/me/balance**", async (route) => {
    await fulfillJSON(route, 503, { error: { message: "余额不可用" } });
  });
  await page.route("**/v1/me", async (route) => {
    await fulfillJSON(route, 200, { user: { id: "usr_rbac", display_name: "", email: "", roles: ["end_user"] } });
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
    await fulfillJSON(route, 200, { user: { id: "usr_rbac", display_name: "Ada", email: "ada@example.test", roles: ["end_user"] } });
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
      user: { id: "usr_rbac", display_name: "Pat", email: "pat@example.test", roles: ["platform_admin"] },
    });
  });
  await page.goto("/app");
  await page.getByTestId("avatar-trigger").click();
  const adminItem = page.getByRole("menuitem", { name: "平台管理" });
  await expect(adminItem).toBeVisible();
  await expect(adminItem).toHaveAttribute("href", "/admin");
  await expect(adminItem).not.toHaveAttribute("aria-disabled", "true");
});

test("user shell logout clears the session and remains at login", async ({ page }) => {
  let signedIn = true;
  let logouts = 0;
  await page.route("**/v1/me/balance**", (route) => fulfillJSON(route, 200, { balance: { available: "1" } }));
  await page.route("**/v1/auth/logout", (route) => { signedIn = false; logouts++; return fulfillJSON(route, 200, { ok: true }); });
  await page.route("**/v1/me", (route) => fulfillJSON(route, signedIn ? 200 : 401, signedIn ? { user: { id: "usr_rbac", display_name: "Ada", email: "ada@example.test", roles: ["end_user"] } } : { error: { code: "authentication_error", message: "未登录" } }));
  await page.goto("/app");
  await page.getByTestId("avatar-trigger").click();
  await page.getByRole("menuitem", { name: "退出登录", exact: true }).click();
  await expect(page).toHaveURL(/\/login(?:\?|$)/);
  await expect(page.getByRole("heading", { name: "注册 / 登录", exact: true })).toBeVisible();
  expect(logouts).toBe(1);
});

test("narrow console uses an accessible drawer and excludes retired settings", async ({ page }) => {
  await page.setViewportSize({ width: 375, height: 812 });
  await page.goto("/app");
  await expect(page.getByRole("button", { name: "打开导航" })).toBeVisible();
  await expect(page.getByTestId("console-page-title")).toBeVisible();
  await expect(page.getByTestId("chrome-overflow-trigger")).toBeVisible();
  await expect(page.getByTestId("console-chrome-inline")).toBeHidden();
  const overflowX = await page.getByTestId("console-shell").evaluate((el) => getComputedStyle(el).overflowX);
  expect(overflowX).not.toBe("hidden");
  const noPageOverflow = await page.evaluate(
    () => document.documentElement.scrollWidth <= document.documentElement.clientWidth + 1,
  );
  expect(noPageOverflow).toBeTruthy();
  const titlePos = await page.getByTestId("console-page-title").evaluate((el) => getComputedStyle(el).position);
  expect(titlePos).not.toBe("absolute");
  await page.getByRole("button", { name: "打开导航" }).click();
  const drawer = page.getByRole("dialog");
  await expect(drawer.locator('a[href="/app/keys"]')).toBeVisible();
  await drawer.getByRole("link", { name: "设置" }).click();
  await expect(page.getByRole("link", { name: "账户" })).toBeVisible();
  await expect(page.getByRole("link", { name: "团队" })).toHaveCount(0);
  await page.goto("/app/settings/team");
  // Next's authenticated layout can stream before notFound; verify the actual
  // rendered retirement state instead of a pre-hydration response status.
  await expect(page.getByText("404", { exact: true })).toBeVisible();
  await expect(page.getByRole("heading", { name: "团队", exact: true })).toHaveCount(0);
  await expect(page.getByRole("button", { name: /重命名/ })).toHaveCount(0);
});

async function stubWalletChrome(page: Page) {
  await page.route("**/v1/me/balance**", async (route) => {
    await fulfillJSON(route, 200, { balance: { available: "12.5", reserved: "1.00" } });
  });
  await page.route("**/v1/me", async (route) => {
    await fulfillJSON(route, 200, {
      user: {
        id: "usr_rbac",
        display_name: "Verylongusernamefortheconsoleheader",
        email: "ada@example.test",
        roles: ["end_user"],
      },
    });
  });
  await page.route("**/v1/payments/checkout**", async (route) => {
    await fulfillJSON(route, 200, {
      item: {
        methods: [{ adapter: "alipay", display_name: "支付宝", pay_currency: "CNY", sandbox: true }],
        settings: { quick_amounts: [100, 300] },
      },
    });
  });
  await page.route("**/v1/payments/quote**", async (route) => {
    await fulfillJSON(route, 200, {
      item: {
        adapter: "alipay",
        pay_major: 100,
        pay_currency: "CNY",
        pay_minor: 10000,
        fee_minor: 0,
        credit_minor: 10000000,
      },
    });
  });
}

async function assertHeaderFits(page: Page) {
  const overflowX = await page.getByTestId("console-shell").evaluate((el) => getComputedStyle(el).overflowX);
  expect(overflowX).not.toBe("hidden");
  const noPageOverflow = await page.evaluate(
    () => document.documentElement.scrollWidth <= document.documentElement.clientWidth + 1,
  );
  expect(noPageOverflow).toBeTruthy();
  const headerFits = await page.locator("header").first().evaluate((el) => el.scrollWidth <= el.clientWidth + 1);
  expect(headerFits).toBeTruthy();
  const title = page.getByTestId("console-page-title");
  const avatar = page.getByTestId("avatar-trigger");
  const titleBox = await title.boundingBox();
  const avatarBox = await avatar.boundingBox();
  expect(titleBox && avatarBox).toBeTruthy();
  expect(titleBox!.x + titleBox!.width).toBeLessThanOrEqual(avatarBox!.x + 1);
  const position = await title.evaluate((el) => getComputedStyle(el).position);
  expect(["static", "relative"]).toContain(position);
}

test("wallet and console chrome keep controls in view on 375 390 and desktop", async ({ page }) => {
  await stubWalletChrome(page);
  for (const width of [375, 390, 1280]) {
    await page.setViewportSize({ width, height: 812 });
    await page.goto("/app/wallet");
    await expect(page.getByTestId("console-page-title")).toHaveText("余额/充值");
    await assertHeaderFits(page);
    const quoteBeforePay = await page.evaluate(() => {
      const quote = document.querySelector("[data-testid=quote-summary]");
      const pay = document.querySelector("[data-testid=wallet-pay]");
      return Boolean(quote && pay && quote.compareDocumentPosition(pay) & Node.DOCUMENT_POSITION_FOLLOWING);
    });
    expect(quoteBeforePay).toBeTruthy();
    await expect(page.getByLabel("兑换码")).toBeVisible();
    if (width < 768) {
      await expect(page.getByTestId("chrome-overflow-trigger")).toBeVisible();
      await expect(page.getByTestId("console-chrome-inline")).toBeHidden();
      await expect(page.getByTestId("balance-pill")).toBeHidden();
      await page.getByTestId("chrome-overflow-trigger").click();
      const overflow = page.getByTestId("chrome-overflow-menu");
      await expect(overflow).toBeVisible();
      await expect(overflow.getByRole("button", { name: "语言" })).toBeVisible();
      await expect(overflow.getByRole("button", { name: "主题" })).toBeVisible();
      const overflowBox = await overflow.boundingBox();
      expect(overflowBox).toBeTruthy();
      expect(overflowBox!.x).toBeGreaterThanOrEqual(0);
      expect(overflowBox!.x + overflowBox!.width).toBeLessThanOrEqual(width + 1);
      await page.keyboard.press("Escape");
      await page.getByTestId("avatar-trigger").click();
      await expect(page.getByTestId("menu-display-name")).toHaveText("Verylongusernamefortheconsoleheader");
      await expect(page.getByTestId("menu-balance")).toHaveText("$12.50");
      await expect(page.getByTestId("menu-balance-link")).toBeVisible();
      const menu = page.getByTestId("avatar-menu");
      const menuBox = await menu.boundingBox();
      expect(menuBox).toBeTruthy();
      expect(menuBox!.x).toBeGreaterThanOrEqual(0);
      expect(menuBox!.x + menuBox!.width).toBeLessThanOrEqual(width + 1);
      await page.keyboard.press("Escape");
    } else {
      await expect(page.getByTestId("chrome-overflow-trigger")).toBeHidden();
      await expect(page.getByTestId("console-chrome-inline")).toBeVisible();
      await expect(page.getByTestId("balance-pill")).toHaveText("$12.50");
      await page.getByTestId("avatar-trigger").click();
      await expect(page.getByTestId("menu-balance")).toHaveText("$12.50");
      await page.keyboard.press("Escape");
    }
  }
});

function usageSummary(requests = 0, amount_minor = 0) {
  const totals = { requests, confirmed: requests, pending: 0, voided: 0, amount_minor, prompt_tokens: requests * 10, completion_tokens: requests * 5, reasoning_tokens: 0 };
  return { totals, daily: requests ? [{ ...totals, key: "2026-10-10" }] : [], keys: requests ? [{ ...totals, key: "key_alpha" }] : [], models: requests ? [{ ...totals, key: "tokenhub/echo-1" }] : [], key_labels: { key_alpha: "alpha" }, facets: { keys: ["key_alpha"], models: ["tokenhub/echo-1"] }, time_zone: "Asia/Shanghai", generated_at: "2026-10-10T00:00:00Z" };
}
function requestReceipt(id: string, billing_state: string) {
  return { request_id: id, public_model_id: "tokenhub/echo-1", api_key_id: "key_alpha", result: "failed", billing_state, customer_amount_minor: billing_state === "confirmed" ? 100000 : 0, error_code: "rate_limited", started_at: "2026-10-10T00:00:00Z" };
}
function personalReferral() {
  return { item: { codes: ["PERSONAL"], code_links: [{ code: "PERSONAL", share_url: "https://brand.test/login?promotion_code=PERSONAL" }], can_create: false, invited_count: 0, can_commission: false, professional_customers: false, rules: { spend_minor: 10000000, topup_minor: 10000000, gift_minor: 1000000 }, progress: { spend_minor: 0, largest_topup_minor: 0, gift_granted_minor: 0, gift_remaining_minor: 0 }, summary: { earned_minor: 0, frozen_minor: 0, available_minor: 0, held_minor: 0, settled_minor: 0, paid_minor: 0, reversed_minor: 0 }, rewards: [], settlements: [], pagination: { page: 1, page_size: 25, rewards_total: 0, settlements_total: 0 } } };
}
