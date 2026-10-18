import { readFileSync } from "node:fs";
import { expect, test, type Browser, type Cookie, type Page, type Response, type TestInfo } from "@playwright/test";
import { z } from "zod";
import { SessionSchema } from "../lib/contracts";
import { customerSections, operatorSections, visibleOperatorSections } from "../lib/navigation";

// Opt in explicitly. Credentials stay in the local acceptance file and in
// browser memory; this suite never writes account data or intercepts the API.
const fixturePath = process.env.TOKENHUB_FRONTEND_ACCEPTANCE_FILE;
const FixtureSchema = z.object({ web_url: z.string().url(), operator_id: z.string(), owner_email: z.string(), owner_password: z.string(), customer_email: z.string(), customer_password: z.string() });
const fixture = fixturePath ? FixtureSchema.parse(JSON.parse(readFileSync(fixturePath, "utf8"))) : undefined;
const webOrigin = process.env.TOKENHUB_WEB_ORIGIN || fixture?.web_url;
type RouteCase = { path: string; label: string; api: string[]; allowedErrors?: string[]; evidence?: boolean };
const desktop = { width: 1365, height: 960 }, mobile = { width: 390, height: 844 };
const operatorCases: RouteCase[] = operatorSections.flatMap(section => section.items.map(([resource, label]) => ({
  path: `/manage/${fixture?.operator_id || "unconfigured"}/${resource}`,
  label: `${section.group} / ${label}`,
  api: [`/api/v1/operators/${fixture?.operator_id || "unconfigured"}/${resource === "reports" ? "reports/financial" : resource === "takeover-settlements" ? "takeovers" : resource}`],
  allowedErrors: ["pending_configuration", "unmet_requirements"],
  evidence: ["overview", "coverage", "reports"].includes(resource),
})));
const customerAPI: Record<string, string[]> = {
  "": ["credits", "keys", "plans", "requests"], models: ["/api/v1/public/models"], playground: ["models"], keys: ["keys"], requests: ["requests"], plans: ["plans/catalog", "plans"], orders: ["orders"], referrals: ["referrals"], commissions: ["commissions", "credits"], notifications: ["notifications", "notifications/preferences"],
};
const customerCases: RouteCase[] = customerSections.flatMap(section => section.items.map(([resource, label]) => ({
  path: `/workspace${resource ? `/${resource}` : ""}`,
  label: `${section.group} / ${label}`,
  api: (customerAPI[resource] || [`${resource}/workspace`]).map(path => path.startsWith("/") ? path : `/api/v1/me/${path}`),
  allowedErrors: resource === "activities" ? ["forbidden", "pending_configuration"] : ["pending_configuration", "unmet_requirements"],
  evidence: ["", "models", "plans"].includes(resource),
})));
const publicCases: RouteCase[] = [
  { path: "/", label: "品牌首页", api: ["/api/v1/public/context", "/api/v1/public/models", "/api/v1/public/plans"], evidence: true },
  { path: "/models", label: "公开模型与价格", api: ["/api/v1/public/models"] },
  { path: "/plans", label: "公开套餐", api: ["/api/v1/public/plans"] },
  { path: "/login", label: "身份登录", api: ["/api/v1/public/context", "/api/v1/auth/me"] },
  { path: "/register", label: "客户注册与归属确认", api: ["/api/v1/public/context"] },
  { path: "/forgot-password", label: "找回密码", api: ["/api/v1/public/context"] },
  { path: "/reset-password", label: "密码重设凭据缺失状态", api: ["/api/v1/public/context"] },
  { path: "/verify-email", label: "邮箱验证凭据缺失状态", api: ["/api/v1/public/context"] },
  { path: "/staff-invitation", label: "人员邀请凭据缺失状态", api: ["/api/v1/public/context"] },
];

function audit(page: Page) {
  const serverErrors: string[] = [], runtimeErrors: string[] = [];
  page.on("response", response => {
    const path = new URL(response.url()).pathname;
    if (response.status() >= 500 && (path.startsWith("/api/v1/") || response.request().resourceType() === "document")) serverErrors.push(`${response.status()} ${path}`);
  });
  page.on("pageerror", error => runtimeErrors.push(error.message));
  return { serverErrors, runtimeErrors };
}

async function mainResponse(response: Response, route: RouteCase) {
  expect(response.status(), `${route.label}: ${new URL(response.url()).pathname}`).toBeLessThan(500);
  if (!response.ok()) {
    const body = await response.json();
    const code = body?.error?.code;
    expect([403, 409], `${route.label}: unexpected HTTP ${response.status()} (${code})`).toContain(response.status());
    expect(route.allowedErrors || [], `${route.label}: unexpected business restriction ${code}`).toContain(code);
  }
}

async function noHorizontalOverflow(page: Page) {
  const dimensions = await page.evaluate(() => ({ viewport: window.innerWidth, root: document.documentElement.scrollWidth, body: document.body.scrollWidth }));
  expect(dimensions.root, `document overflow: ${JSON.stringify(dimensions)}`).toBeLessThanOrEqual(dimensions.viewport + 1);
  expect(dimensions.body, `body overflow: ${JSON.stringify(dimensions)}`).toBeLessThanOrEqual(dimensions.viewport + 1);
}

async function visit(page: Page, route: RouteCase, testInfo: TestInfo, mobileView = false) {
  const observed = audit(page);
  // Start actual response waits before navigation. A visible heading alone
  // would miss a resource whose request or runtime contract failed afterwards.
  const waits = route.api.map(path => page.waitForResponse(response => response.request().method() === "GET" && new URL(response.url()).pathname === path));
  const document = await page.goto(route.path, { waitUntil: "domcontentloaded" });
  expect(document?.status(), `${route.label}: page did not load`).toBeLessThan(400);
  for (const response of await Promise.all(waits)) await mainResponse(response, route);
  await expect(page.getByRole("heading", { level: 1 }).first()).toBeVisible();
  await expect(page.getByText("正在加载当前记录…", { exact: true })).toHaveCount(0);
  await expect(page.locator("body")).not.toContainText(/服务数据格式与当前页面不一致|contract_mismatch|无法识别的结果|Application error:|Internal Server Error|This page could not be found/);
  await expect(page.locator("body")).not.toContainText(/没有这个经营空间的访问权限|当前登录身份无法访问此空间|页面不存在/);
  expect(new URL(page.url()).pathname).toBe(route.path);
  await noHorizontalOverflow(page);
  if (mobileView && (route.path.startsWith("/workspace") || route.path.startsWith("/manage/"))) {
    await page.getByRole("button", { name: "打开工作台导航", exact: true }).click();
    const navigation = page.getByRole("navigation", { name: "移动工作台", exact: true });
    await expect(navigation).toBeVisible();
    const operatorView = route.path.startsWith("/manage/");
    const groups = (operatorView ? operatorSections : customerSections).filter(section => section.id !== "account");
    // The active group expands its children vertically. Check the primary
    // destinations independently from those extra links and the utility area.
    const groupLinks = navigation.locator(":scope > div > a");
    await expect(groupLinks).toHaveCount(groups.length);
    const base = operatorView ? `/manage/${fixture!.operator_id}` : "/workspace";
    for (let i = 0; i < groups.length; i++) {
      const resource = groups[i].items[0][0];
      await expect(groupLinks.nth(i)).toHaveAttribute("href", `${base}${resource ? `/${resource}` : ""}`);
    }
    await expect(page.getByRole("navigation", { name: "移动工作台工具", exact: true }).getByRole("link")).toHaveCount(operatorView ? 1 : 2);
    await noHorizontalOverflow(page);
    await page.getByRole("button", { name: "收起工作台导航", exact: true }).click();
  }
  expect(observed.serverErrors, `${route.label}: server failures`).toEqual([]);
  expect(observed.runtimeErrors, `${route.label}: browser exceptions`).toEqual([]);
  if (route.evidence && mobileView) {
    const image = testInfo.outputPath("live-page.png");
    await page.screenshot({ path: image, fullPage: true });
    await testInfo.attach("live-page", { path: image, contentType: "image/png" });
  }
}

async function login(browser: Browser, kind: "staff" | "customer") {
  if (!fixture) throw new Error("TOKENHUB_FRONTEND_ACCEPTANCE_FILE is required");
  const context = await browser.newContext({ baseURL: webOrigin, viewport: desktop, locale: "zh-CN" });
  try {
    const page = await context.newPage();
    await page.goto("/login");
    await page.getByRole("radio", { name: kind === "staff" ? "经营人员" : "客户", exact: true }).check();
    await page.getByLabel("邮箱", { exact: true }).fill(kind === "staff" ? fixture.owner_email : fixture.customer_email);
    await page.getByLabel("密码", { exact: true }).fill(kind === "staff" ? fixture.owner_password : fixture.customer_password);
    const response = page.waitForResponse(response => response.request().method() === "POST" && new URL(response.url()).pathname === "/api/v1/auth/login");
    await page.getByRole("button", { name: "登录", exact: true }).click();
    const result = await response;
    const body = await result.json();
    expect(result.status(), `real acceptance account login (${body.error?.code || "response"})`).toBe(200);
    const session = SessionSchema.parse(body);
    expect(session.kind).toBe(kind);
    if (session.kind === "staff") {
      const workspace = session.workspaces.find(item => item.operator.id === fixture.operator_id);
      expect(workspace, "fixture owner does not own the target workspace").toBeTruthy();
      expect(workspace?.operator.kind).toBe("platform");
      expect(visibleOperatorSections(workspace!).flatMap(section => section.items)).toHaveLength(operatorCases.length);
    }
    await expect(page).toHaveURL(kind === "staff" ? /\/manage\// : /\/workspace$/);
    return await context.cookies();
  } finally { await context.close(); }
}

test.describe("real frontend page coverage", () => {
  test.skip(!fixturePath, "Set TOKENHUB_FRONTEND_ACCEPTANCE_FILE to the local isolated acceptance fixture.");
  test.use({ baseURL: webOrigin, viewport: desktop });

  test.describe("public", () => {
    for (const route of publicCases) test(`desktop: ${route.label}`, async ({ page }, info) => { await visit(page, route, info); });
    for (const route of publicCases.filter(route => ["/", "/models", "/plans", "/login"].includes(route.path))) test(`390px: ${route.label}`, async ({ page }, info) => { await page.setViewportSize(mobile); await visit(page, route, info, true); });
  });

  test.describe("owner", () => {
    let cookies: Cookie[] = [];
    test.beforeAll(async ({ browser }) => { cookies = await login(browser, "staff"); });
    test.beforeEach(async ({ context }) => { await context.addCookies(cookies); });
    for (const route of operatorCases) test(`desktop: ${route.label}`, async ({ page }, info) => { await visit(page, route, info); });
    for (const route of operatorCases.filter(route => ["overview", "customers", "orders", "payments", "refunds", "procurement", "coverage", "branding", "payment-methods", "reports"].includes(route.path.split("/").at(-1)!))) test(`390px: ${route.label}`, async ({ page }, info) => { await page.setViewportSize(mobile); await visit(page, route, info, true); });
  });

  test.describe("customer", () => {
    let cookies: Cookie[] = [];
    test.beforeAll(async ({ browser }) => { cookies = await login(browser, "customer"); });
    test.beforeEach(async ({ context }) => { await context.addCookies(cookies); });
    for (const route of customerCases) test(`desktop: ${route.label}`, async ({ page }, info) => { await visit(page, route, info); });
    for (const route of customerCases.filter(route => ["/workspace", "/workspace/models", "/workspace/plans", "/workspace/keys", "/workspace/requests", "/workspace/referrals", "/workspace/withdrawals"].includes(route.path))) test(`390px: ${route.label}`, async ({ page }, info) => { await page.setViewportSize(mobile); await visit(page, route, info, true); });
  });
});
