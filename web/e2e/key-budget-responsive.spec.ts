import { expect, test } from "@playwright/test";
import { mockViewer } from "./mock-viewer";

for (const width of [390, 903]) {
  test(`Key actual usage and exhausted budget are visible at ${width}px`, async ({ page }) => {
    await page.setViewportSize({ width, height: 900 });
    await mockViewer(page, { roles: ["end_user"] });
    await page.route("**/v1/me/balance", route => route.fulfill({ json: { balance: { available: "-0.02", reserved: "0", available_minor: -20000 } } }));
    await page.route("**/v1/me/api-keys**", route => route.fulfill({ json: { items: [{
      id: "key-actual", name: "Actual usage", prefix: "thk_actual", status: "active", model_mode: "selected", allowlist: ["public/echo"],
      budget_limit_minor: 100000, budget_used_minor: 120000, budget_reserved_minor: 0, expires_at: "2030-01-01T00:00:00Z",
    }], next_cursor: "" } }));
    await page.route("**/public/models**", route => route.fulfill({ json: { items: [], total: 0 } }));
    await page.goto("/app/keys");
    const card = page.getByTestId("key-cards").getByRole("listitem");
    await expect(card).toBeVisible();
    await expect(card.getByText("额度已用完", { exact: true })).toBeVisible();
    await expect(card.getByText("已用 0.120000 USD · 上限 0.1 USD", { exact: true })).toBeVisible();
    await expect(card.getByText("剩余 0.000000 USD", { exact: true })).toBeVisible();
    await expect(card.getByText(/public\/echo.*2030/)).toBeVisible();
    await expect(card.getByRole("button", { name: "编辑限制", exact: true })).toBeVisible();
    await expect(page.getByTestId("balance-pill")).toHaveText("-$0.02");
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth + 1)).toBe(true);
  });
}
