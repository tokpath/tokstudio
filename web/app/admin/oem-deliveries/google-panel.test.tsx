/** @vitest-environment jsdom */
import { afterEach, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { withZh } from "@/lib/test-i18n";
import { OEMGooglePanel } from "./google-panel";

vi.mock("@/components/rbac/viewer-context", () => ({ useViewer: () => ({ userId: "platform-tech", roles: ["tech_admin"] }) }));
afterEach(() => { cleanup(); vi.unstubAllGlobals(); vi.restoreAllMocks(); });
const base = { client_id: "one.apps.googleusercontent.com", secret_set: true, enabled: false, version: 4, redirect_uris: ["https://one.example/login/oauth/google", "https://admin.one.example/login/oauth/google"], verification: {} };
const json = (body: unknown, status = 200) => ({ ok: status < 400, status, json: async () => body });
function page() { return render(<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>{withZh(<OEMGooglePanel channelID="oem-one" />)}</QueryClientProvider>); }

it("shows generated callback URLs, keeps saved secrets hidden, and blocks enabling an untested client", async () => {
  vi.stubGlobal("fetch", vi.fn(async () => json({ item: base })));
  page(); await screen.findByDisplayValue(base.client_id);
  const secret = screen.getByLabelText("Google Client Secret") as HTMLInputElement;
  expect(secret.type).toBe("password"); expect(secret.value).toBe("");
  expect(screen.getByLabelText("启用 Google 登录").hasAttribute("disabled")).toBe(true);
  expect(screen.getByText(base.redirect_uris[0])).toBeTruthy();
  expect(screen.getAllByRole("button", { name: "测试登录" })).toHaveLength(2);
});

it("requires a new secret for a new client, then saves through confirmation with the loaded version", async () => {
  let sent: Record<string, unknown> = {};
  vi.stubGlobal("fetch", vi.fn(async (_url, init) => {
    if (init?.method === "PUT") { sent = JSON.parse(init.body); return json({ item: { ...base, ...sent, version: 5 } }); }
    return json({ item: base });
  }));
  page(); await screen.findByDisplayValue(base.client_id);
  fireEvent.change(screen.getByLabelText("Google Client ID"), { target: { value: "two.apps.googleusercontent.com" } });
  fireEvent.click(screen.getByRole("button", { name: "保存 Google 配置" }));
  expect(await screen.findByRole("alert")).toBeTruthy(); expect(screen.queryByRole("dialog")).toBeNull();
  fireEvent.change(screen.getByLabelText("Google Client Secret"), { target: { value: "new-synthetic-secret" } });
  fireEvent.click(screen.getByRole("button", { name: "保存 Google 配置" }));
  fireEvent.click(within(await screen.findByRole("dialog")).getByRole("button", { name: "确认", exact: true }));
  await screen.findByText("Google 配置已保存。");
  expect(sent).toMatchObject({ client_id: "two.apps.googleusercontent.com", client_secret: "new-synthetic-secret", expected_version: 4, enabled: false });
  expect((screen.getByLabelText("Google Client Secret") as HTMLInputElement).value).toBe("");
});

it("retains the original version and displays a definite conflict instead of silently retrying a credential write", async () => {
  const bodies: string[] = [];
  vi.stubGlobal("fetch", vi.fn(async (_url, init) => {
    if (init?.method === "PUT") { bodies.push(init.body); return json({ error: { message: "配置已变化，请刷新后核对。", code: "version_conflict" } }, 409); }
    return json({ item: { ...base, verification: { [base.redirect_uris[0]]: { verified_at: "2026-10-12T00:00:00Z" } } } });
  }));
  page(); await screen.findByDisplayValue(base.client_id);
  fireEvent.click(screen.getByLabelText("启用 Google 登录"));
  fireEvent.click(screen.getByRole("button", { name: "保存 Google 配置" }));
  fireEvent.click(within(await screen.findByRole("dialog")).getByRole("button", { name: "确认", exact: true }));
  await screen.findAllByText("配置已变化，请刷新后核对。");
  expect(bodies).toHaveLength(1); expect(JSON.parse(bodies[0]).expected_version).toBe(4);
  expect(screen.getByRole("dialog")).toBeTruthy();
});

it("creates a test ticket only for the selected callback and offers a usable link when a popup is blocked", async () => {
  let sent: unknown;
  vi.spyOn(window, "open").mockReturnValue(null);
  vi.stubGlobal("fetch", vi.fn(async (_url, init) => {
    if (init?.method === "POST") { sent = JSON.parse(init.body); return json({ url: "https://one.example/login/oauth/google/test?ticket=synthetic" }); }
    return json({ item: base });
  }));
  page(); await screen.findByDisplayValue(base.client_id);
  fireEvent.click(screen.getAllByRole("button", { name: "测试登录" })[0]);
  const link = await screen.findByRole("link", { name: "继续 Google 测试" });
  expect(sent).toEqual({ redirect_uri: base.redirect_uris[0] });
  expect(link.getAttribute("href")).toBe("https://one.example/login/oauth/google/test?ticket=synthetic");
});
