import { expect, test } from "@playwright/test";

const publicPaths = [
  "/",
  "/models",
  "/quickstart",
  "/docs",
  "/docs/integrations",
  "/docs/develop",
  "/docs/changelog",
  "/enterprise",
  "/trust",
  "/trust/subprocessors",
  "/best-value",
  "/model-finder",
  "/vibe-coding",
  "/video",
  "/image",
  "/leaderboards/models",
  "/leaderboards/apps",
  "/leaderboards/labs",
  "/vs/openrouter",
  "/promo",
  "/promo/august",
  "/desktop",
  "/verify",
  "/awesome-ofox",
  "/pricing",
  "/blog",
  "/terms",
  "/privacy",
  "/login",
];

test("public ofox replica pages render headings", async ({ page }) => {
  for (const path of publicPaths) {
    const response = await page.goto(path);
    expect(response?.ok(), path).toBeTruthy();
    await expect(page.locator("h1")).toBeVisible();
  }
});

test("user console main flow shows DESIGN.md hero cards", async ({ page }) => {
  await page.goto("/app");
  await expect(page.getByRole("heading", { name: "我的账户" })).toBeVisible();
  await expect(page.getByText("可用余额")).toBeVisible();
  await expect(page.getByText("预授权占用")).toBeVisible();
  await expect(page.getByRole("heading", { name: "API Key" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "用量与账单" })).toBeVisible();
});
