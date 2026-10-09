import { expect, test } from "@playwright/test";
import { mockViewer } from "./mock-viewer";

test.beforeEach(async ({ page }) => {
  await mockViewer(page, { roles: ["channel_admin"] });
});

test("OEM sees its children and can delist only its own model", async ({ page }) => {
  let change: Record<string, unknown> | undefined;
  await page.route("**/api/channel/me", route => route.fulfill({ json: { channel_org_id: "chn_oem_c", channel_type: "C" } }));
  await page.route("**/api/channel/subchannels?*", route => route.fulfill({ json: { items: [
    { id: "chn_child", code: "child-b", type: "B", status: "active", parent_id: "chn_oem_c" },
  ], total: 1, next_cursor: "" } }));
  await page.route("**/api/channel/models", async route => {
    if (route.request().method() === "PATCH") {
      change = route.request().postDataJSON();
      await route.fulfill({ json: { public_id: "tokenhub/echo-1", enabled: false } });
      return;
    }
    await route.fulfill({ json: { items: [
      { public_id: "tokenhub/echo-1", display_name: "Echo", vendor: "TokenHub", status: "published", enabled: true, self_enabled: true, parent_enabled: true },
    ] } });
  });
  await page.goto("/channel/subchannels");
  const childRow = page.getByRole("listitem").filter({ hasText: "child-b" });
  const child = childRow.locator('a[href^="/channel/subchannels/chn_child"]');
  await expect(child).toBeVisible();
  const target = new URL(await child.getAttribute("href") as string, "https://brand.test");
  expect(target.pathname).toBe("/channel/subchannels/chn_child");
  expect(target.searchParams.get("return_to")).toContain("/channel/subchannels?viewer_scope=usr_rbac");
  await expect(page.getByRole("link", { name: "oem-c", exact: true })).toHaveCount(0);
  await page.goto("/channel/models");
  await page.getByRole("button", { name: "下架", exact: true }).click();
  await page.getByRole("dialog").getByRole("button", { name: "确认", exact: true }).click();
  await expect.poll(() => change).toMatchObject({ public_id: "tokenhub/echo-1", enabled: false });
});

test("OEM grants a model from its own scope to a child", async ({ page }) => {
  let change: Record<string, unknown> | undefined;
  await page.route("**/api/channel/me", route => route.fulfill({ json: { channel_org_id: "chn_oem_c", channel_type: "C" } }));
  await page.route("**/api/admin/channels/chn_child", route => route.fulfill({ json: { item: { id: "chn_child", code: "child-b", type: "B", status: "active", parent_id: "chn_oem_c" } } }));
  await page.route("**/api/admin/channels/chn_child/admins", route => route.fulfill({ json: { items: [] } }));
  await page.route("**/api/channel/subchannels/chn_child/users", route => route.fulfill({ json: { items: [] } }));
  await page.route("**/api/admin/channels/chn_child/models", async route => {
    if (route.request().method() === "PATCH") {
      change = route.request().postDataJSON();
    }
    await route.fulfill({ json: { channel_type: "B", items: [
      { public_id: "tokenhub/echo-1", display_name: "Echo", vendor: "TokenHub", kind: "text", status: "published", enabled: false, parent_enabled: true },
      { public_id: "tokenhub/blocked", display_name: "Outside parent scope", vendor: "TokenHub", kind: "text", status: "published", enabled: false, parent_enabled: false },
    ] } });
  });
  await page.goto("/channel/subchannels/chn_child");
  await page.getByRole("button", { name: "编辑授权", exact: true }).click();
  await page.getByRole("checkbox", { name: /Echo tokenhub\/echo-1/ }).check();
  await expect(page.getByRole("checkbox", { name: /Outside parent scope/ })).toBeDisabled();
  await expect(page.getByLabel(/渠道结算价/)).toHaveCount(0);
  await expect(page.getByLabel("输入（USD / 百万 Token）", { exact: true })).toHaveCount(0);
  await expect(page.getByLabel("输出（USD / 百万 Token）", { exact: true })).toHaveCount(0);
  await page.getByRole("button", { name: "保存变更", exact: true }).click();
  await page.getByRole("dialog").getByRole("button", { name: "确认", exact: true }).click();
  await expect.poll(() => change).toEqual({ items: [
    { public_id: "tokenhub/echo-1", enabled: true },
  ] });
});
