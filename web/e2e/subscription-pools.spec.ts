import { expect, test } from "@playwright/test";
import operations from "../../backend/openapi/operator-operations.json";
import { chooseControl } from "./choose-control";

const operator = { id: "platform-test", name: "平台", legal_name: "测试平台", kind: "platform", brand_id: "brand-test" };
const brand = { id: operator.brand_id, name: "测试平台", api_base_url: "https://api.example.test/v1", theme: {} };
const api = `/api/v1/operators/${operator.id}`;
const manage = `/manage/${operator.id}`;
const pool = { id: "pool-test", name: "Claude 订阅", enabled: true, version: 1, subscription_kind: "claude" };
const poolReference = { value: pool.id, label: pool.name, defaults: { provider_id: pool.id, subscription_kind: pool.subscription_kind } };

function resource(name: string, title: string, items: unknown[] = []) {
  return { title, description: "", items, columns: [], row_operations: [], operations: operations.filter(item => item.resource === name && ["model", "record"].includes(item.name)).map(item => ({
    ...item, path: `${api}/${name}`, fields: item.fields.map(field => field.name === "provider_id" ? { ...field, lookup_url: `${api}/references/cost-suppliers` } : field.name === "supplier_model_id" ? { ...field, lookup_url: `${api}/references/cost-models` } : field),
  })) };
}

test("a platform subscription pool opens the existing supplier form and records an original-currency bill", async ({ page }) => {
  const token = "sk-ant-oat-mock-browser";
  const statement = { id: "bill-test", statement_reference: "BILL-2026-11", currency: "EUR", amount: "19.95" };
  let created = false;
  let bills: unknown[] = [];
  await page.route("**/api/v1/**", async route => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    let data: unknown = { items: [] };
    switch (path) {
      case "/api/v1/public/context":
        data = { operator, brand, theme: {}, availability: [], payment_methods: [] };
        break;
      case "/api/v1/auth/me":
        data = { kind: "staff", staff: { id: "test-owner", email: "owner@example.test", display_name: "平台负责人" }, workspaces: [{ operator, permissions: ["*"] }], active_operator_id: operator.id, csrf_token: "test_csrf" };
        break;
      case `${api}/notifications/unread-count`:
        data = { count: 0 };
        break;
      case `${api}/subscription-pools`:
        if (request.method() === "POST") {
          expect(request.postDataJSON()).toMatchObject({ action: "save_pool", pool: { name: pool.name, subscription_kind: "claude", first_account: { credential: token } } });
          expect(request.headers()["idempotency-key"]).toBeTruthy();
          created = true;
          data = { id: pool.id, status: "saved" };
        } else if (created) {
          data = { items: [{ ...pool, accounts: [{ id: "account-test", name: "首个账号", enabled: true, version: 1, status: "available" }], latest_statement: bills.length ? statement : undefined }] };
        }
        break;
      case `${api}/providers`:
        data = resource("providers", "模型供应商");
        break;
      case `${api}/supplier-costs`:
        if (request.method() === "POST") {
          const body = request.postDataJSON();
          expect(body).toMatchObject({ provider_id: pool.id, subscription_kind: "claude", statement_reference: statement.statement_reference, account_coverage: "首个账号", currency: statement.currency, amount: statement.amount });
          expect(body.supplier_model_id).toBeUndefined();
          bills = [{ id: statement.id, status: "recorded", values: { ...body, provider_name: pool.name } }];
          data = { id: statement.id, status: "recorded" };
        } else data = resource("supplier-costs", "真实上游成本", bills);
        break;
      case `${api}/references/credentials`:
      case `${api}/references/cost-suppliers`:
        data = { items: [poolReference] };
        break;
    }
    await route.fulfill({ json: data });
  });

  await page.goto(`${manage}/subscription-pools`);
  const technology = page.getByRole("navigation", { name: "平台技术内导航", exact: true });
  await expect(technology.getByRole("link", { name: "订阅账号池", exact: true })).toBeVisible();
  await expect(technology.getByRole("link", { name: "路由配置", exact: true })).toHaveAttribute("href", `${manage}/routes`);
  await page.getByRole("button", { name: "新建账号池", exact: true }).click();
  let dialog = page.getByRole("dialog");
  await dialog.getByLabel("账号池名称", { exact: true }).fill(pool.name);
  await dialog.getByRole("combobox", { name: "供应商", exact: true }).selectOption("claude");
  await dialog.locator('input[type="password"]').fill(token);
  await dialog.getByRole("button", { name: "保存", exact: true }).click();
  await expect(page.getByRole("heading", { name: pool.name, exact: true })).toBeVisible();
  await expect(page.getByText("1 / 1 个账号可用", { exact: false })).toBeVisible();
  await expect(page.locator('input[type="password"]')).toHaveCount(0);

  await page.getByRole("link", { name: "添加供应商模型", exact: true }).click();
  await expect(page).toHaveURL(new RegExp(`${manage}/providers\\?pool=`));
  dialog = page.getByRole("dialog");
  await expect(dialog.getByRole("heading", { name: "添加供应商模型", exact: true })).toBeVisible();
  await expect(dialog.locator('[data-config-path="supplier_model.credential_id"] input')).toHaveValue(pool.name);
  await expect(dialog.locator('[data-config-path="supplier_model.provider"] select')).toHaveValue("anthropic");
  await expect(dialog.getByRole("checkbox", { name: "对话", exact: true })).toBeChecked();
  await expect(dialog.locator('[data-config-path="supplier_model.base_url"], [data-config-path="supplier_model.pricing_reference"]')).toHaveCount(0);
  await dialog.getByRole("button", { name: "关闭办理面板", exact: true }).click();

  await technology.getByRole("link", { name: "真实上游成本", exact: true }).click();
  await page.getByRole("button", { name: "登记实际费用", exact: true }).click();
  dialog = page.getByRole("dialog");
  await chooseControl(dialog.getByRole("combobox", { name: "供应商", exact: true }), pool.id);
  await expect(dialog.locator('[data-command-field="supplier_model_id"]')).toHaveCount(0);
  for (const [field, value] of Object.entries({ statement_reference: statement.statement_reference, account_coverage: "首个账号", currency: statement.currency, amount: statement.amount, starts_at: "2026-11-01T00:00", ends_at: "2026-12-01T00:00" })) {
    await dialog.locator(`[data-command-field="${field}"] input`).fill(value);
  }
  await dialog.getByRole("button", { name: "登记实际费用", exact: true }).click();
  await expect(dialog.getByRole("status")).toBeVisible();
  await dialog.getByRole("button", { name: "关闭办理面板", exact: true }).click();
  await expect(page.getByLabel("19.95 EUR", { exact: true })).toBeVisible();

  await technology.getByRole("link", { name: "订阅账号池", exact: true }).click();
  await page.getByRole("link", { name: statement.statement_reference, exact: true }).click();
  await expect(page).toHaveURL(`${manage}/supplier-costs?record=${statement.id}`);
});
