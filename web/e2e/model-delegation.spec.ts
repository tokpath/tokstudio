import { expect, test } from "@playwright/test";
import { mockViewer } from "./mock-viewer";

test.beforeEach(async ({ page }) => {
  await mockViewer(page, { roles: ["channel_admin"] });
});

test("OEM sees its children and can delist only its own model", async ({ page }) => {
  let change: Record<string, unknown> | undefined;
  await page.route("**/api/channel/me", route => route.fulfill({ json: { channel_org_id: "chn_oem_c", channel_type: "C" } }));
  await page.route("**/api/admin/channels?*", route => route.fulfill({ json: { items: [
    { id: "chn_oem_c", code: "oem-c", type: "C", status: "active" },
    { id: "chn_child", code: "child-b", type: "B", status: "active", parent_id: "chn_oem_c" },
  ] } }));
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
  await expect(page.getByRole("link", { name: "管理 →", exact: true }).first()).toHaveAttribute("href", "/channel/subchannels/chn_child");
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
    await route.fulfill({ json: { items: [
      { public_id: "tokenhub/echo-1", display_name: "Echo", vendor: "TokenHub", kind: "text", status: "published", enabled: false, parent_enabled: true },
    ] } });
  });
  await page.goto("/channel/subchannels/chn_child");
  await page.getByRole("button", { name: "授权模型" }).click();
  await page.getByRole("checkbox", { name: "授权 tokenhub/echo-1" }).check();
  await page.getByLabel("渠道结算价 输入（美元/M）").fill("0.7");
  await page.getByLabel("渠道结算价 输出（美元/M）").fill("1.4");
  await page.getByRole("button", { name: "保存授权" }).click();
  await page.getByRole("dialog").getByRole("button", { name: "确认", exact: true }).click();
  await expect.poll(() => change).toMatchObject({ items: [
    { public_id: "tokenhub/echo-1", enabled: true, wholesale: { input: "0.0000007", output: "0.0000014" } },
  ] });
});
