import { expect, test, type Page } from "@playwright/test";

export async function mockViewer(page: Page, input: { roles: string[]; partner?: boolean }) {
  await page.route("**/v1/me", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ user: { id: "usr_rbac", email: "rbac@example.com", roles: input.roles } }),
    });
  });
  await page.route("**/v1/partner/me", async (route) => {
    if (input.partner) {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ role_type: "agent" }),
      });
      return;
    }
    await route.fulfill({
      status: 403,
      contentType: "application/json",
      body: JSON.stringify({ error: { message: "不是推广主体" } }),
    });
  });
}
