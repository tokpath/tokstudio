import { expect, test } from "@playwright/test";

test("English Accept-Language uses the same URLs without a locale prefix", async ({ browser }) => {
  const context = await browser.newContext({
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
    extraHTTPHeaders: { "Accept-Language": "ja-JP,ja;q=0.9" },
  });
  const page = await context.newPage();
  await page.goto("/desktop");
  await expect(page).toHaveURL(/\/desktop$/);
  await expect(page).not.toHaveURL(/\/ja\//);
  await expect(page.getByRole("heading", { name: "手元のコーディングツールを一つの口座で" })).toBeVisible();
  await context.close();
});

test("NEXT_LOCALE cookie overrides Accept-Language", async ({ browser }) => {
  const context = await browser.newContext({
    extraHTTPHeaders: { "Accept-Language": "en-US,en;q=0.9" },
  });
  await context.addCookies([{ name: "NEXT_LOCALE", value: "ja", domain: "127.0.0.1", path: "/" }]);
  const page = await context.newPage();
  await page.goto("/compare");
  await expect(page).toHaveURL(/\/compare$/);
  await expect(page.getByRole("heading", { name: "モデル比較" })).toBeVisible();
  await context.close();
});
