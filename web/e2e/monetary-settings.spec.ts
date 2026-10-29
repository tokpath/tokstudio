import { expect, test, type Page } from "@playwright/test";
import { translate } from "../lib/i18n";

test.use({ timezoneId: "Asia/Shanghai" });

const operator = { id: "platform", name: "TokenHub", kind: "platform", brand_id: "brand", theme: {} };
const brand = { id: "brand", name: "TokenHub", api_base_url: "https://api.example.test/v1", theme: {} };
const empty = { title: "营业设置", description: "", items: [], columns: [], operations: [], row_operations: [] };
type Settings = { currency: string; atg_per_unit: string; status: "unconfigured" | "scheduled" | "locked"; version: number; effective_at?: string };
async function workspace(page: Page, initial: Settings, owner: boolean) {
  let monetary = initial;
  const writes: { path: string; body: Record<string, unknown> }[] = [];
  await page.route("**/api/v1/**", async route => {
    const request = route.request(), path = new URL(request.url()).pathname;
    if (monetary.status === "scheduled" && monetary.effective_at && new Date(monetary.effective_at).getTime() <= Date.now()) monetary = { ...monetary, status: "locked" };
    let data: unknown = empty;
    if (path === "/api/v1/public/context") data = { brand, operator, theme: {}, monetary, availability: [], payment_methods: [] };
    else if (path === "/api/v1/auth/me") data = { kind: "staff", staff: { id: "owner", email: "owner@example.test", display_name: "负责人" }, workspaces: [{ operator, permissions: ["*"], can_manage_monetary: owner }], csrf_token: "synthetic-csrf" };
    else if (path === "/api/v1/operators/platform/business-status") data = { ...empty, items: [{ id: "platform", version: 1, values: { readiness: ["registration", "new_purchase", "renewal", "model_usage", "activity", "settlement"].map(feature => ({ feature, requirements: monetary.status === "scheduled" ? [{ code: "monetary", description: "基础比例尚未生效，暂不提供采购换算。", ready: false }] : [] })) } }] };
    else if (path.startsWith("/api/v1/monetary/")) {
      if (request.method() === "POST") {
        const body = request.postDataJSON(); writes.push({ path, body });
        monetary = path.endsWith("/cancel") ? { ...monetary, status: "unconfigured", version: monetary.version + 1 } : { ...monetary, ...body, status: "scheduled", version: monetary.version + 1 };
        data = { id: "monetary", status: "saved", version: monetary.version };
      } else data = monetary;
    }
    await route.fulfill({ json: data });
  });
  return writes;
}

test("platform owner reviews irreversible settings before scheduling and can cancel before activation", async ({ page }, info) => {
  const writes = await workspace(page, { currency: "USD", atg_per_unit: "1", status: "unconfigured", version: 7 }, true);
  await page.goto("/manage/platform/business-status");
  await page.getByLabel("结算币种（ISO 代码）").fill("CNY");
  await page.getByLabel("每 1 单位结算币种购买的 ATG").fill("10.25");
  await page.getByLabel("生效时间", { exact: true }).fill("2099-01-01T09:00");
  await page.getByRole("button", { name: "核对并查看风险说明" }).click();
  expect(writes).toHaveLength(0);
  await expect(page.getByRole("button", { name: "确认预约生效" })).toBeDisabled();
  await page.screenshot({ path: info.outputPath("monetary-review-desktop.png"), fullPage: true });
  await page.setViewportSize({ width: 390, height: 844 });
  await expect(page.getByText("我已核对并理解永久锁定及换算规则")).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: info.outputPath("monetary-review-mobile.png"), fullPage: true });
  await page.getByRole("checkbox", { name: "我已核对并理解永久锁定及换算规则" }).check();
  await page.getByRole("button", { name: "确认预约生效" }).click();
  await expect(page.getByRole("button", { name: "取消预约并重新设置" })).toBeVisible();
  expect(writes[0].body).toMatchObject({ currency: "CNY", atg_per_unit: "10.25", expected_version: 7, confirmed: true });
  expect(writes[0].body.effective_at).toBe("2099-01-01T01:00:00.000Z");
  for (const locale of ["en", "ja", "ru", "zh"] as const) {
    await page.getByTestId("language-selector").selectOption(locale);
    await expect(page.getByRole("heading", { name: translate(locale, "全系统结算币种与 ATG 基础比例") })).toBeVisible();
    await expect(page.getByText(translate(locale, "已预约生效；生效前可取消并重新设置。"), { exact: false })).toBeVisible();
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  }
  await page.getByRole("button", { name: "取消预约并重新设置" }).click();
  await expect(page.getByRole("button", { name: "核对并查看风险说明" })).toBeVisible();
  expect(writes[1].body).toEqual({ expected_version: 8 });
});

test("scheduled activation automatically refreshes locked settings and business readiness", async ({ page }) => {
  await workspace(page, { currency: "CNY", atg_per_unit: "10.25", status: "scheduled", version: 8, effective_at: new Date(Date.now() + 4000).toISOString() }, true);
  await page.goto("/manage/platform/business-status");
  await expect(page.getByRole("button", { name: "开启 套餐新购", exact: true })).toBeDisabled();
  await expect(page.getByText("已生效并永久锁定。暂停营业不会解锁。", { exact: false })).toBeVisible({ timeout: 7000 });
  await expect(page.getByRole("button", { name: "开启 套餐新购", exact: true })).toBeEnabled();
  await expect(page.getByRole("button", { name: "取消预约并重新设置" })).toHaveCount(0);
});

test("effective settings stay read-only for the owner and every other staff workspace", async ({ page }) => {
  const locked: Settings = { currency: "CNY", atg_per_unit: "10.25", status: "locked", version: 8, effective_at: "2026-01-01T00:00:00Z" };
  await workspace(page, locked, true);
  await page.goto("/manage/platform/business-status");
  await expect(page.getByText("已生效并永久锁定。暂停营业不会解锁。", { exact: false })).toBeVisible();
  await expect(page.getByLabel("结算币种（ISO 代码）")).toHaveCount(0);
  await expect(page.getByRole("button", { name: "取消预约并重新设置" })).toHaveCount(0);
  await page.unroute("**/api/v1/**");
  await workspace(page, locked, false);
  await page.reload();
  await expect(page.getByText("全系统使用同一结算币种。每 1 CNY 对应 10.25 ATG。")).toBeVisible();
  await expect(page.getByLabel("结算币种（ISO 代码）")).toHaveCount(0);
});

test("order details use their frozen currency and rate while showing original payment currency separately", async ({ page }) => {
  const order = { id: "order", status: "pending_payment", kind: "package", operator_name: operator.name, amount_money: "100", paid_money: "0", created_at: "2026-10-01T00:00:00Z", monetary_snapshot: { currency: "CNY", atg_per_unit: "10.25", version: 3 }, payment_action: { kind: "offline", currency: "MYR", instructions: "Synthetic receipt instructions", settlement_money: { currency: "CNY", amount: "100" } } };
  await page.route("**/api/v1/**", async route => {
    const path = new URL(route.request().url()).pathname;
    let data: unknown = { items: [] };
    if (path === "/api/v1/public/context") data = { brand, operator, theme: {}, monetary: { currency: "EUR", atg_per_unit: "2", status: "locked", version: 5 }, availability: [], payment_methods: [] };
    else if (path === "/api/v1/auth/me") data = { kind: "customer", customer: { id: "customer", email: "customer@example.test", display_name: "客户" }, operator, brand, csrf_token: "synthetic-csrf" };
    else if (path === "/api/v1/me/orders") data = { items: [order] };
    else if (path === "/api/v1/me/orders/order") data = order;
    await route.fulfill({ json: data });
  });
  await page.goto("/workspace/orders?record=order");
  await expect(page.getByText("每 1 CNY = 10.25 ATG（版本 3）")).toBeVisible();
  await expect(page.getByLabel("100 CNY").first()).toBeVisible();
  await expect(page.getByText("MYR", { exact: true })).toBeVisible();
  await expect(page.getByLabel("100 EUR")).toHaveCount(0);
});
