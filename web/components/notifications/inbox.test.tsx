/** @vitest-environment jsdom */
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { withZh } from "@/lib/test-i18n";
import { NotificationInbox } from "./inbox";
import { NotificationBell } from "./bell";
import { NotificationAdminSettings } from "./admin-settings";

let roles = ["end_user"];
vi.mock("@/components/rbac/viewer-context", () => ({ useViewer: () => ({ signedIn: true, loading: false, userId: "synthetic-user", roles }) }));
vi.mock("@/components/brand-context", () => ({ useBrand: () => ({ id: "synthetic-brand" }) }));
const notice = { id: "synthetic-notice", kind: "renewal_failed", title: "套餐续费失败", body: "本期已到期，没有宽限期。全部额度不足时停止请求。", href: "/app/plans", created_at: "2026-10-12T08:00:00Z", read_at: null, email_status: "blocked_smtp" };
beforeEach(() => { roles = ["end_user"]; });
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

it("preserves unread notices after a failed read action and marks them only after success", async () => {
  let read = false; let fail = true;
  vi.stubGlobal("fetch", vi.fn(async (input: string, init?: RequestInit) => {
    if (input.endsWith("/read")) { if (!fail) read = true; return { ok: !fail, status: fail ? 500 : 200, json: async () => ({ ok: !fail }) }; }
    return { ok: true, status: 200, json: async () => input.includes("notification-preferences") ? { email_enabled: true, smtp_available: false } : { items: [{ ...notice, read_at: read ? "2026-10-12T09:00:00Z" : null }], unread: read ? 0 : 1, next_cursor: "" } };
  }));
  render(withZh(<NotificationInbox />));
  await screen.findByText("套餐续费失败");
  expect(screen.getByText("本期已到期，没有宽限期。全部额度不足时停止请求。")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "标为已读" }));
  await screen.findByText("操作失败，请稍后重试。");
  expect(screen.getByRole("button", { name: "标为已读" })).toBeTruthy();
  fail = false;
  fireEvent.click(screen.getByRole("button", { name: "标为已读" }));
  await waitFor(() => expect(screen.queryByRole("button", { name: "标为已读" })).toBeNull());
  expect(screen.getByRole("button", { name: "未读（0）" })).toBeTruthy();
});

it("changes business email preference while preserving the inbox", async () => {
  let enabled = true;
  const fetcher = vi.fn(async (input: string, init?: RequestInit) => {
    if (input.includes("notification-preferences")) { if (init?.method === "PUT") enabled = JSON.parse(String(init.body)).email_enabled; return { ok: true, status: 200, json: async () => ({ email_enabled: enabled, smtp_available: false }) }; }
    return { ok: true, status: 200, json: async () => ({ items: [notice], unread: 1, next_cursor: "" }) };
  });
  vi.stubGlobal("fetch", fetcher); render(withZh(<NotificationInbox />));
  await screen.findByText("套餐续费失败");
  fireEvent.click(screen.getByRole("checkbox", { name: "接收业务邮件通知" }));
  await waitFor(() => expect((screen.getByRole("checkbox") as HTMLInputElement).checked).toBe(false));
  expect(screen.getByText("套餐续费失败")).toBeTruthy();
  expect(enabled).toBe(false);
});

it("shows a failed unread-count request as unavailable", async () => {
  vi.stubGlobal("fetch", vi.fn(async () => ({ ok: false, status: 503, json: async () => ({}) })));
  render(withZh(<NotificationBell href="/app/notifications" />));
  await screen.findByRole("link", { name: "通知中心，未读数量暂不可用" });
});

it("restores the email preference when saving fails", async () => {
  vi.stubGlobal("fetch", vi.fn(async (input: string, init?: RequestInit) => {
    if (init?.method === "PUT") return { ok: false, status: 503, json: async () => ({}) };
    return { ok: true, status: 200, json: async () => input.includes("notification-preferences") ? { email_enabled: true, smtp_available: false } : { items: [notice], unread: 1, next_cursor: "" } };
  }));
  render(withZh(<NotificationInbox />));
  fireEvent.click(await screen.findByRole("checkbox", { name: "接收业务邮件通知" }));
  await screen.findByText("操作失败，请稍后重试。");
  expect((screen.getByRole("checkbox") as HTMLInputElement).checked).toBe(true);
});

it("lets operations inspect delivery failures without loading SMTP credentials", async () => {
  roles = ["ops_admin"];
  const fetcher = vi.fn(async (_input: RequestInfo | URL) => ({ ok: true, status: 200, json: async () => ({ items: [], pending: 0, blocked: 2, failed: 1, accepted: 0 }) }));
  vi.stubGlobal("fetch", fetcher); render(withZh(<NotificationAdminSettings />));
  await screen.findByText("待发送 0 · 受阻 2 · 失败 1 · 已接收 0");
  expect(screen.queryByRole("heading", { name: "平台邮件通知" })).toBeNull();
  expect(fetcher.mock.calls.every(call => String(call[0]).includes("/deliveries"))).toBe(true);
});

it.each([null, undefined])("renders an empty delivery log when the API returns %s items", async items => {
  roles = ["ops_admin"];
  vi.stubGlobal("fetch", vi.fn(async () => ({ ok: true, status: 200, json: async () => ({ items, pending: 0, blocked: 0, failed: 0, accepted: 0 }) })));
  render(withZh(<NotificationAdminSettings />));
  await screen.findByText("待发送 0 · 受阻 0 · 失败 0 · 已接收 0");
  expect(screen.getByText("暂无符合条件的发送记录。")).toBeTruthy();
});
