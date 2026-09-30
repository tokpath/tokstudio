import { expect, test } from "@playwright/test";
import { mockViewer } from "./mock-viewer";

for (const oem of [false, true]) {
  test(`${oem ? "OEM" : "platform"} staff management`, async ({ page }) => {
    const path = oem ? "/channel/staff" : "/admin/staff";
    const adminRole = oem ? "channel_admin" : "platform_admin";
    const opsRole = oem ? "oem_ops" : "ops_admin";
    let members = [{ user_id: "usr_rbac", email: "admin@example.com", display_name: "管理员", roles: [adminRole], status: "active", created_by_email: "admin@example.com", updated_by_email: "admin@example.com", created_at: "2026-09-30T01:00:00Z", updated_at: "2026-09-30T01:00:00Z" }];
    await page.route("**/api/**", route => route.fulfill({ json: { items: [], item: { enabled: false }, brand: { name: "OEM" } } }));
    await mockViewer(page, { roles: [adminRole], channelType: "C" });
    await page.route(`**/api${path}**`, async route => {
      const request = route.request();
      if (request.method() === "POST") {
        const body = request.postDataJSON();
        members = [...members, { ...members[0], user_id: "usr_staff", email: body.email, display_name: body.display_name, roles: body.roles }];
        await route.fulfill({ status: 201, json: { item: members[1] } });
      } else if (request.method() === "PATCH") {
        const body = request.postDataJSON(); members = members.map(member => member.user_id === "usr_staff" ? { ...member, ...body } : member);
        await route.fulfill({ json: { item: members[1] } });
      } else if (request.url().endsWith("/history")) {
        await route.fulfill({ json: { items: [{ id: "audit1", action: "staff.disable", actor_user_id: "usr_rbac", created_at: "2026-09-30T02:00:00Z", after: members[1] }], actors: { usr_rbac: "admin@example.com" } } });
      } else await route.fulfill({ json: { items: members, roles: oem ? [adminRole, opsRole, "oem_finance", "oem_audit"] : [adminRole, opsRole, "finance_admin", "tech_admin", "audit_readonly"] } });
    });
    await page.goto(path);
    await expect(page.getByRole("heading", { name: "员工与权限" })).toBeVisible();
    const self = page.getByRole("row").filter({ hasText: "admin@example.com" });
    await expect(self.getByRole("button", { name: "停用", exact: true })).toBeDisabled();
    await page.getByRole("button", { name: "添加员工", exact: true }).click();
    const dialog = page.getByRole("dialog");
    if (oem) await expect(dialog.getByRole("checkbox", { name: /^技术/ })).toHaveCount(0);
    await dialog.getByLabel("邮箱", { exact: true }).fill("staff@example.com");
    await dialog.getByLabel("姓名（选填）", { exact: true }).fill("测试员工");
    await dialog.getByLabel("初始密码", { exact: true }).fill("password1");
    await dialog.getByRole("checkbox", { name: /^运营/ }).check();
    await dialog.getByRole("button", { name: "添加员工", exact: true }).click();
    await expect(dialog).not.toBeVisible();
    const employee = page.getByRole("row").filter({ hasText: "staff@example.com" });
    await expect(employee).toBeVisible();
    await employee.getByRole("button", { name: "编辑岗位" }).click();
    await dialog.getByRole("checkbox", { name: /^财务/ }).check();
    await dialog.getByRole("button", { name: "保存", exact: true }).click();
    await expect(employee).toContainText("运营、财务");
    await employee.getByRole("button", { name: "停用", exact: true }).click();
    await dialog.getByRole("button", { name: "停用", exact: true }).click();
    await expect(employee).toContainText("已停用");
    await employee.getByRole("button", { name: "操作记录" }).click();
    await expect(dialog).toContainText("停用员工");
    await expect(dialog).toContainText("admin@example.com");
  });
}

test("OEM financial staff see only their business menu and cannot manage employees", async ({ page }) => {
  await page.route("**/api/**", route => route.fulfill({ json: { items: [], quota: {}, totals: { requests: 0, revenue_minor: 0, cost_minor: 0, margin_minor: 0, pending: 0 } } }));
  await mockViewer(page, { roles: ["oem_finance"], channelType: "C" });
  await page.goto("/channel/metrics");
  const sidebar = page.getByRole("navigation", { name: "OEM 管理控制台", exact: true });
  await expect(sidebar.getByRole("link", { name: "员工与权限" })).toHaveCount(0);
  await expect(sidebar.getByRole("link", { name: "用户/项目" })).toHaveCount(0);
  await expect(sidebar.getByRole("link", { name: "余额/充值" })).toBeVisible();
  await page.goto("/channel/staff");
  await expect(page.getByTestId("console-access")).toBeVisible();
  await expect(page.getByRole("button", { name: "添加员工" })).toHaveCount(0);
});
