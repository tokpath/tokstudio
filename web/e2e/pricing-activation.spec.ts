import { expect, test } from "@playwright/test";

// HTTP responses model a queued publication followed by worker activation.
// Database tests separately verify the worker and immediate first-listing path.
test("pricing explains pending activation and updates when the listing takes effect", async ({ page }) => {
 const operator = { id: "platform", name: "TokenHub", kind: "platform", brand_id: "brand", theme: {} };
 const brand = { id: "brand", name: "TokenHub", api_base_url: "https://api.example.test/v1", theme: {} };
 let reads = 0;
 await page.route("**/api/v1/**", async route => {
  const path = new URL(route.request().url()).pathname;
  let data: unknown = { title: "", description: "", items: [], columns: [], operations: [], row_operations: [] };
  if (path === "/api/v1/public/context") data = { brand, operator, theme: {}, monetary: { currency: "USD", atg_per_unit: "1", status: "locked", version: 1 }, availability: [], payment_methods: [] };
  else if (path === "/api/v1/auth/me") data = { kind: "staff", staff: { id: "owner", email: "owner@example.test", display_name: "负责人" }, workspaces: [{ operator, permissions: ["*"] }], csrf_token: "synthetic-csrf" };
  else if (path === "/api/v1/operators/platform/pricing") {
   reads++;
   const active = reads > 1;
   data = { title: "模型与定价", description: "", search_supported: false, status_supported: false, items: [{ id: "GPT-6.1 Sol", status: active ? "available" : "not_listed", values: { name: "GPT-6.1 Sol", pending_changes: active ? [] : [{ kind: "listing", enabled: true, status: "announced", effective_at: "2030-01-01T00:00:00Z", needs_attention: false }] } }], columns: [], operations: [], row_operations: [] };
  }
  await route.fulfill({ json: data });
 });
 await page.goto("/manage/platform/pricing");
 await expect(page.getByText("上架待生效", { exact: true })).toBeVisible();
 await expect(page.getByText("生效后列表自动更新", { exact: false })).toBeVisible();
 await expect(page.getByText("尚未上架，可先准备计费与价格", { exact: true })).toHaveCount(0);
 await expect(page.getByText("已上架", { exact: true })).toBeVisible({ timeout: 10_000 });
 expect(reads).toBeGreaterThan(1);
 const afterActivation = reads;
 await page.waitForTimeout(3500);
 expect(reads).toBe(afterActivation);
});
