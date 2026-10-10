import { expect, test } from "@playwright/test";

const unavailable = [
  "/awesome-ofox", "/leaderboards/models", "/leaderboards/apps", "/leaderboards/labs",
  "/blog", "/blog/old-article", "/promo/august", "/vs/openrouter",
];

test("unlaunched public pages return 404 at their actual URL", async ({ page }) => {
  for (const path of unavailable) {
    const response = await page.goto(path);
    expect(response?.status(), path).toBe(404);
    expect(new URL(page.url()).pathname).toBe(path);
  }
});

test("trust content discloses response caching and unconfirmed suppliers", async ({ page }) => {
  await page.goto("/trust");
  await expect(page.getByText(/完整响应（包括生成正文）可缓存 24 小时/)).toBeVisible();
  await expect(page.getByText(/HTTP 上游地址不提供 TLS 加密/)).toBeVisible();
  await expect(page.getByText(/不持久化 prompt\/completion 正文/)).toHaveCount(0);
  await page.goto("/trust/subprocessors");
  await expect(page.getByRole("heading", { name: "实际供应商清单待确认" })).toBeVisible();
  await expect(page.getByText(/本项目尚未上线/)).toBeVisible();
  await expect(page.getByRole("table")).toHaveCount(0);
  await expect(page.getByText("PostgreSQL", { exact: true })).toHaveCount(0);
  await expect(page.getByText("Redis", { exact: true })).toHaveCount(0);
  await page.screenshot({ path: "test-results/t10-suppliers-pending.png", fullPage: true });
});

test("public aliases retain invitation and original model task", async ({ page }) => {
  await page.goto("/desktop?model=vendor%2Fmodel-v2&promo=INVITE&next=%2Fapp%2Fkeys%3Fmodel%3Dvendor%252Fmodel-v2");
  await expect(page).toHaveURL(/\/docs\/integrations\?/);
  let target = new URL(page.url());
  expect(target.searchParams.get("model")).toBe("vendor/model-v2");
  expect(target.searchParams.get("promo")).toBe("INVITE");
  expect(target.searchParams.get("next")).toBe("/app/keys?model=vendor%2Fmodel-v2");
  await page.goto("/model-finder/original-model?promotion_code=INVITE&next=%2Fapp%2Fkeys");
  target = new URL(page.url());
  expect(target.pathname).toBe("/models/original-model");
  expect(target.searchParams.get("promotion_code")).toBe("INVITE");
  expect(target.searchParams.get("next")).toBe("/app/keys");
});
