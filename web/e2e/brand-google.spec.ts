import { expect, test } from "@playwright/test";
import { mockViewer } from "./mock-viewer";

test("platform technicians configure OEM Google clients in delivery details on desktop and mobile", async ({ page }) => {
  await page.route("**/api/**", route => route.fulfill({ status: 503, json: { error: { message: "本场景未加载此业务数据" } } }));
  await mockViewer(page, { roles: ["tech_admin"] });
  await page.route("**/api/admin/oem-deliveries/oem-google", route => route.fulfill({ json: { item: {
    delivery: { channel_org_id: "oem-google", phase: "configuring", version: 1, sales_mode: "offline", sell_plans: false, handoff_evidence: {} },
    channel: { id: "oem-google", code: "oem-google" }, brand: { id: "brand-google", name: "品牌 A", primary_domain: "brand.example", api_domain: "api.brand.example", admin_domain: "admin.brand.example" },
    ready: false, checked_at: "2026-10-12T00:00:00Z", administrators: [], checks: [],
  } } }));
  await page.route("**/api/admin/oem-deliveries/oem-google/google", route => route.fulfill({ json: { item: {
    client_id: "oem-client.apps.googleusercontent.com", secret_set: true, enabled: false, version: 1,
    redirect_uris: ["https://brand.example/login/oauth/google", "https://admin.brand.example/login/oauth/google"], verification: {},
  } } }));
  await page.goto("/admin/oem-deliveries/oem-google");
  await page.setViewportSize({ width: 1440, height: 1100 });
  const panel = page.getByRole("region", { name: "Google 登录", exact: true });
  await expect(panel).toBeVisible();
  await expect(panel.getByLabel("Google Client ID", { exact: true })).toHaveValue("oem-client.apps.googleusercontent.com");
  await expect(panel.getByLabel("Google Client Secret", { exact: true })).toHaveAttribute("type", "password");
  await expect(panel.getByLabel("启用 Google 登录")).toBeDisabled();
  await panel.scrollIntoViewIfNeeded();
  await panel.screenshot({ path: "test-results/brand-google-desktop.png" });
  await page.setViewportSize({ width: 390, height: 844 });
  await panel.scrollIntoViewIfNeeded();
  await expect(panel.getByLabel("Google Client ID", { exact: true })).toBeVisible();
  await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
  await page.setViewportSize({ width: 390, height: 1100 });
  await panel.screenshot({ path: "test-results/brand-google-mobile.png" });
});

test("a Google configuration test succeeds while the browser has no customer session", async ({ page }) => {
  await page.route("**/api/v1/me", route => route.fulfill({ status: 401, json: { error: { message: "未登录" } } }));
  await page.route("**/api/v1/auth/google/test/start?**", route => route.fulfill({ json: { state: "test-state", auth_url: "https://accounts.google.com/o/oauth2/v2/auth?state=test-state" } }));
  await page.route("https://accounts.google.com/o/oauth2/v2/auth?**", route => route.fulfill({ status: 302, headers: { location: new URL("/login/oauth/google?state=test-state&code=test-code", page.url()).toString() } }));
  await page.route("**/api/v1/auth/google/callback", route => route.fulfill({ json: { test_only: true, verified: true } }));
  await page.goto("/login/oauth/google/test?ticket=synthetic-ticket");
  await expect(page).toHaveURL(/\/login\/oauth\/google\/test\?result=success$/);
  await expect(page.getByRole("status")).toContainText("测试成功");
  expect((await page.context().cookies()).some(cookie => cookie.name === "tokenhub_session")).toBe(false);
});
