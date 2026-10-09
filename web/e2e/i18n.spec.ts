import { expect, test } from "@playwright/test";
import { mockViewer } from "./mock-viewer";

test("English Accept-Language uses the same URLs without a locale prefix", async ({ browser }) => {
  const context = await browser.newContext({
    locale: "en-US",
    extraHTTPHeaders: { "Accept-Language": "en-US,en;q=0.9" },
  });
  const page = await context.newPage();

  await page.goto("/models");
  await expect(page).toHaveURL(/\/models$/);
  await expect(page).not.toHaveURL(/\/en\//);
  await expect(page.getByRole("heading", { name: "Model catalog", exact: true })).toBeVisible();

  await mockViewer(page, { roles: ["end_user"] });
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
  await page.goto("/models");
  await expect(page).toHaveURL(/\/models$/);
  await expect(page).not.toHaveURL(/\/ja\//);
  await expect(page.getByRole("heading", { name: "モデルカタログ", exact: true })).toBeVisible();
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
  await expect(page.getByRole("heading", { name: "Access available models with one API account", exact: true })).toBeVisible();
  await expect(page.getByRole("link", { name: "Create API key", exact: true }).first()).toHaveAttribute("href", "/login?next=%2Fapp%2Fkeys");
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
  await page.goto("/models");
  await expect(page.getByRole("heading", { name: "Model catalog", exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Language" }).click();
  await page.getByRole("menuitemradio", { name: "日本語" }).click();
  await expect(page).toHaveURL(/\/models$/);
  await expect(page).not.toHaveURL(/\/ja\//);
  await expect(page.getByRole("heading", { name: "モデルカタログ", exact: true })).toBeVisible();
  await page.reload();
  await expect(page.getByRole("heading", { name: "モデルカタログ", exact: true })).toBeVisible();
  await context.close();
});
