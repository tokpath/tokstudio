import { expect, test } from "@playwright/test";

test("English Accept-Language uses the same URLs without a locale prefix", async ({ browser }) => {
  const context = await browser.newContext({
    locale: "en-US",
    extraHTTPHeaders: { "Accept-Language": "en-US,en;q=0.9" },
  });
  const page = await context.newPage();

  await page.goto("/compare");
  await expect(page).toHaveURL(/\/compare$/);
  await expect(page).not.toHaveURL(/\/en\//);
  await expect(page.getByRole("heading", { name: "Compare models" })).toBeVisible();

  await page.goto("/app/playground");
  await expect(page).toHaveURL(/\/app\/playground$/);
  await expect(page.getByRole("heading", { name: "Playground" })).toBeVisible();

  await page.goto("/login");
  await expect(page.getByRole("heading", { name: "Sign up / Sign in" })).toBeVisible();

  await context.close();
});

test("Japanese Accept-Language keeps pathnames unchanged", async ({ browser }) => {
  const context = await browser.newContext({
    locale: "ja-JP",
    extraHTTPHeaders: { "Accept-Language": "ja-JP,ja;q=0.9" },
  });
  const page = await context.newPage();
  await page.goto("/desktop");
  await expect(page).toHaveURL(/\/desktop$/);
  await expect(page).not.toHaveURL(/\/ja\//);
  await expect(page.getByRole("heading", { name: "手元のコーディングツールを一つの口座で" })).toBeVisible();
  await context.close();
});

test("English public pages keep the same URLs and translate body copy", async ({ browser }) => {
  const context = await browser.newContext({
    locale: "en-US",
    extraHTTPHeaders: { "Accept-Language": "en-US,en;q=0.9" },
  });
  const page = await context.newPage();

  await page.goto("/");
  await expect(page).toHaveURL(/\/$/);
  await expect(page.getByRole("heading", { name: "Why TokenHub" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Start reconciling" })).toBeVisible();
  await expect(page.locator("header").getByRole("link", { name: "Console" })).toBeVisible();

  await page.goto("/models");
  await expect(page).toHaveURL(/\/models$/);
  await expect(page.getByLabel("Search models")).toBeVisible();

  await context.close();
});

test("language switch cookie overrides Accept-Language without changing the URL", async ({ browser }) => {
  const context = await browser.newContext({
    locale: "en-US",
    extraHTTPHeaders: { "Accept-Language": "en-US,en;q=0.9" },
  });
  const page = await context.newPage();
  await page.goto("/compare");
  await expect(page.getByRole("heading", { name: "Compare models" })).toBeVisible();
  await page.getByRole("button", { name: "Language" }).click();
  await page.getByRole("menuitemradio", { name: "日本語" }).click();
  await expect(page).toHaveURL(/\/compare$/);
  await expect(page).not.toHaveURL(/\/ja\//);
  await expect(page.getByRole("heading", { name: "モデル比較" })).toBeVisible();
  await context.close();
});
