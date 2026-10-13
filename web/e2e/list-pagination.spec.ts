import { expect, test } from "@playwright/test";
import { mockViewer } from "./mock-viewer";

test.beforeEach(async ({ page }) => {
  await page.route("**/api/**", route => route.fulfill({ status: 401, json: { error: { message: "未登录" } } }));
  await page.route("**/api/v1/me/notifications/unread-count", route => route.fulfill({ json: { unread: 0 } }));
});

test("user can load media history beyond 200 records", async ({ page }) => {
  await mockViewer(page, { roles: ["end_user"] });
  const jobs = Array.from({ length: 275 }, (_, i) => ({ id: `history_${274-i}`, prompt: `历史媒体 ${274-i}`, kind: "video", task_type: "t2v", model: "synthetic/model", status: "failed" }));
  await page.route("**/api/v1/me/media**", route => {
    const params = new URL(route.request().url()).searchParams;
    const start = Number((params.get("cursor") || "opaque-0").split("-")[1]);
    const end = Math.min(start+20, jobs.length);
    return route.fulfill({ json: { items: jobs.slice(start, end), next_cursor: end < jobs.length ? `opaque-${end}` : "", storage: { source: "unavailable", ok: false, label: "存储不可用" } } });
  });
  await page.goto("/app/media");
  await expect(page.getByTestId("media-job-history_274")).toBeVisible();
  for (let end=40; end<=280; end+=20) {
    await page.getByRole("button", { name: "加载更多任务", exact: true }).click();
    await expect(page.getByTestId(`media-job-history_${Math.max(275-end,0)}`)).toBeVisible();
  }
  await expect(page.getByRole("button", { name: "加载更多任务", exact: true })).toHaveCount(0);
  expect(await page.locator("[data-testid^='media-job-history_']").count()).toBe(275);
});

for (const portal of ["admin", "channel"] as const) {
  test(`${portal} media can page through all 275 records and return`, async ({ page }) => {
    await mockViewer(page, { roles: [portal === "admin" ? "platform_admin" : "channel_admin"], channelType: "C" });
    if (portal === "channel") await page.route("**/api/admin/channels?*", route => route.fulfill({ json: { items: [{ id: "chn_oem_c", code: "Synthetic OEM", type: "C" }, { id: "chn_oem_child", code: "Synthetic child", type: "B", parent_id: "chn_oem_c" }], next_cursor: "" } }));
    const jobs = Array.from({ length: 275 }, (_, i) => ({ id: `record_${274-i}`, kind: "video", task_type: "t2v", model: "synthetic/model", status: "failed" }));
    await page.route(`**/api/${portal}/media**`, route => {
      const params = new URL(route.request().url()).searchParams;
      if (params.get("channel_id") === "chn_oem_child") return route.fulfill({ json: { items: params.get("cursor") ? [] : [{ ...jobs[0], id: "child_first" }], next_cursor: "" } });
      const start = Number((params.get("cursor") || "opaque-0").split("-")[1]);
      const end = Math.min(start+50, jobs.length);
      return route.fulfill({ json: { items: jobs.slice(start, end), next_cursor: end < jobs.length ? `opaque-${end}` : "" } });
    });
    await page.goto(`/${portal}/media`);
    await expect(page.getByRole("cell", { name: "record_274", exact: true })).toBeVisible();
    for (let end=100; end<=300; end+=50) {
      await page.getByRole("button", { name: "下一页", exact: true }).click();
      await expect(page.getByRole("cell", { name: `record_${Math.max(275-end,0)}`, exact: true })).toBeVisible();
    }
    await expect(page.getByRole("button", { name: "下一页", exact: true })).toHaveCount(0);
    await page.getByRole("button", { name: "上一页", exact: true }).click();
    await expect(page.getByRole("cell", { name: "record_74", exact: true })).toBeVisible();
    if (portal === "channel") {
      await page.locator("select").filter({ has: page.locator('option[value="chn_oem_child"]') }).selectOption("chn_oem_child");
      await expect(page.getByRole("cell", { name: "child_first", exact: true })).toBeVisible();
      expect(new URL(page.url()).searchParams.get("_channel_media_cursor")).toBeNull();
    }
  });
}

test("alert history retries a failed page, restores its URL and resets pagination when filtering", async ({ page }) => {
  await mockViewer(page, { roles: ["audit_readonly"] });
  const alerts = Array.from({ length: 120 }, (_, i) => ({ id: `alert_${119-i}`, kind: "circuit_open", severity: "medium", status: (119-i)%2 ? "resolved" : "open", message: `历史告警${String(119-i).padStart(3,"0")}` }));
  let fail = true;
  const reads: URLSearchParams[] = [];
  await page.route("**/api/admin/notifications/deliveries?*", route => route.fulfill({ json: { items: [], pending: 0, blocked: 0, failed: 0, accepted: 0 } }));
  await page.route("**/api/admin/ops/alerts?*", route => {
    const params = new URL(route.request().url()).searchParams; reads.push(params);
    const start = Number((params.get("cursor") || "opaque-0").split("-")[1]);
    if (start === 50 && fail) return route.fulfill({ status: 503, json: { error: { message: "synthetic temporary failure" } } });
    const filtered = alerts.filter(alert => (!params.get("status") || alert.status === params.get("status")) && (!params.get("q") || alert.message.includes(params.get("q")!)));
    const end = Math.min(start+50, filtered.length);
    return route.fulfill({ json: { items: filtered.slice(start, end), next_cursor: end < filtered.length ? `opaque-${end}` : "" } });
  });
  await page.goto("/admin/alerts?from=history");
  await expect(page.getByRole("cell", { name: "历史告警119", exact: true })).toBeVisible();
  await page.getByRole("button", { name: "下一页", exact: true }).click();
  await expect(page.getByRole("alert").filter({ hasText: "读取告警失败" })).toBeVisible();
  await expect(page.getByText("当前条件下暂无告警记录。")).toHaveCount(0);
  fail = false;
  await page.getByRole("button", { name: "重试", exact: true }).click();
  await expect(page.getByRole("cell", { name: "历史告警069", exact: true })).toBeVisible();
  await page.getByRole("button", { name: "下一页", exact: true }).click();
  await expect(page.getByRole("cell", { name: "历史告警000", exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "下一页", exact: true })).toHaveCount(0);
  await page.reload();
  await expect(page.getByRole("cell", { name: "历史告警000", exact: true })).toBeVisible();
  await page.getByRole("button", { name: "回到第一页", exact: true }).click();
  await page.getByLabel("告警状态", { exact: true }).selectOption("resolved");
  await expect(page.getByRole("cell", { name: "历史告警118", exact: true })).toHaveCount(0);
  await page.getByRole("textbox", { name: "筛选", exact: true }).fill("历史告警001");
  await page.getByRole("button", { name: "搜索", exact: true }).click();
  await expect(page.getByRole("cell", { name: "历史告警001", exact: true })).toBeVisible();
  expect(new URL(page.url()).searchParams.get("cursor")).toBeNull();
  expect(new URL(page.url()).searchParams.get("from")).toBe("history");
  const exportHref = await page.getByRole("link", { name: "导出 CSV", exact: true }).getAttribute("href");
  expect(new URL(exportHref!, page.url()).searchParams.get("q")).toBe("历史告警001");
  expect(reads.at(-1)?.get("status")).toBe("resolved");
  await expect(page.getByRole("button", { name: "评估告警", exact: true })).toHaveCount(0);
  await page.setViewportSize({ width: 390, height: 844 });
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth+1)).toBe(true);
  await page.screenshot({ path: "test-results/alerts-pagination-mobile.png", fullPage: true });
});
