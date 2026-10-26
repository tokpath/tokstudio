import { expect, test, type Locator, type Page } from "@playwright/test";
import { readFileSync } from "node:fs";
import { randomUUID } from "node:crypto";
import { chooseControl } from "./choose-control";

const fixturePath = process.env.TOKENHUB_FRONTEND_ACCEPTANCE_FILE;
test.skip(!fixturePath, "Requires isolated application and external model service");

async function submit(page: Page, dialog: Locator, path: string) {
  const waiting = page.waitForResponse(response => new URL(response.url()).pathname === path && response.request().method() === "POST");
  await dialog.locator('button[type="submit"]').click();
  const response = await waiting, out = await response.json();
  expect(response.status(), JSON.stringify(out)).toBe(200);
  const sent = response.request().postDataJSON();
  await dialog.getByRole("button", {name:"关闭办理面板",exact:true}).click();
  return sent;
}

test("route setup loads saved bindings, moves priority and permanently removes a binding without technical knobs", async ({page}, info) => {
  test.setTimeout(120_000);
  const f = JSON.parse(readFileSync(fixturePath!, "utf8")), base = `/api/v1/operators/${f.operator_id}`;
  await page.goto("/login");
  await page.getByLabel("邮箱", {exact:true}).fill(f.owner_email);
  await page.getByLabel("密码", {exact:true}).fill(f.owner_password);
  await page.getByRole("button", {name:"登录",exact:true}).click();
  await expect(page).toHaveURL(/\/manage\//);
  const me = await (await page.request.get("/api/v1/auth/me")).json();
  const post = async (path: string, data: unknown) => {
    const response = await page.request.post(path, {data, headers:{"X-CSRF-Token":me.csrf_token,"Idempotency-Key":randomUUID()}});
    const out = await response.json(); expect(response.status(), JSON.stringify(out)).toBe(200); return out;
  };
  const model = `route-${randomUUID().slice(0,8)}`, suppliers: string[] = [];
  // Seed the two independent provider connections; all route configuration below uses the UI.
  for (const suffix of ["Primary", "Backup"]) {
    const key = await post(`${base}/providers`, {name:`${model} ${suffix}`,secret:"synthetic-key",enabled:true,expected_version:0});
    const id = randomUUID(); suppliers.push(id);
    await post(`${base}/providers`, {supplier_model:{id,credential_id:key.id,provider:"openai",model:`${model}-${suffix}`,enabled:true,capabilities:[{operation:"chat"}]},expected_version:0});
  }
  await page.goto(`/manage/${f.operator_id}/catalog`);
  await page.getByRole("button", {name:"添加公开模型",exact:true}).click();
  let dialog = page.getByRole("dialog").last();
  await dialog.locator('[data-config-path="model.id"] input').fill(model);
  await dialog.getByRole("checkbox", {name:"对话",exact:true}).check();
  await submit(page, dialog, `${base}/catalog`);

  await page.goto(`/manage/${f.operator_id}/routes`);
  const open = async () => {
    await page.getByRole("button", {name:"配置路由",exact:true}).click();
    dialog = page.getByRole("dialog").last();
    await chooseControl(dialog.locator('[data-config-path="route.public_model_id"] [role="combobox"]'), model);
    await expect(dialog.locator('[data-config-path="route.public_model_id"] [role="combobox"]')).toHaveValue(model);
  };
  await open();
  for (let i = 0; i < suppliers.length; i++) {
    await dialog.getByRole("button", {name:"添加绑定",exact:true}).click();
    await chooseControl(dialog.locator(`[data-config-path="route.bindings.${i}.supplier_model_id"] [role="combobox"]`), suppliers[i]);
  }
  await expect(dialog.locator('[data-config-path$=".routing"],[data-config-path$=".priority"]')).toHaveCount(0);
  await dialog.getByRole("button", {name:"上移线路 2",exact:true}).click();
  const first = await submit(page, dialog, `${base}/routes`);
  expect(first.configuration.route.bindings.map((binding: {supplier_model_id:string}) => binding.supplier_model_id)).toEqual([...suppliers].reverse());

  await open();
  await expect(dialog.locator('[data-config-path="route.bindings.0.supplier_model_id"] [role="combobox"]')).toHaveValue(new RegExp("Backup"));
  await dialog.getByRole("button", {name:"移除绑定的供应商模型 2",exact:true}).click();
  const second = await submit(page, dialog, `${base}/routes`);
  expect(second.configuration.expected_version).toBe(1);
  expect(second.configuration.route.bindings).toHaveLength(1);
  await page.reload();
  await open();
  await expect(dialog.locator('[data-config-path^="route.bindings."][data-config-path$=".supplier_model_id"]')).toHaveCount(1);
  await dialog.getByRole("button", {name:"关闭办理面板",exact:true}).click();
  await page.screenshot({path:info.outputPath("routes.png"),fullPage:true});

  await page.goto(`/manage/${f.operator_id}/bifrost`);
  await expect(page.getByRole("button", {name:"调整价目同步",exact:true})).toHaveCount(0);
  await page.getByRole("button", {name:"立即同步",exact:true}).click();
  dialog = page.getByRole("dialog").last();
  await expect(dialog.locator('input:not([type="hidden"])')).toHaveCount(0);
  await submit(page, dialog, `${base}/bifrost`);
  await expect.poll(async () => {
    const settings = await (await page.request.get(`${base}/bifrost`)).json();
    return !!settings.items[0].values.last_success_at && !settings.items[0].values.sync_requested;
  }, {timeout:30_000}).toBe(true);
});
