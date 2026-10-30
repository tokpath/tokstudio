// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import { StaffNotificationBell } from "./staff-notification-bell";

afterEach(() => { cleanup(); vi.unstubAllGlobals(); });
function mount(node: React.ReactNode) {
  const cache = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={cache}>{node}</QueryClientProvider>);
}

it("renders a bell without text beside the signed-in account", async () => {
  vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({ count: 0 }), { status: 200 })));
  const workspaces = [{ operator: { id: "one", kind: "platform" }, permissions: ["notifications.read"] }];
  const view = mount(<StaffNotificationBell workspaces={workspaces} pathname="/manage/one/payments" />);
  const link = await view.findByRole("link", { name: "通知管理" });
  expect(link.getAttribute("href")).toBe("/manage/one/notifications");
  expect(link.textContent).toBe("");
  expect(link.querySelector("svg")).not.toBeNull();
  view.unmount();
  mount(<StaffNotificationBell workspaces={[{ operator: { id: "one", kind: "channel" }, permissions: ["payments.read"] }]} pathname="/manage/one/payments" />);
  expect(screen.queryByRole("link", { name: "通知管理" })).toBeNull();
});

it("shows a red unread count on the bell", async () => {
  vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({ count: 3 }), { status: 200 })));
  const workspaces = [{ operator: { id: "one", kind: "platform" }, permissions: ["notifications.read"] }];
  mount(<StaffNotificationBell workspaces={workspaces} pathname="/manage/one/payments" />);
  const link = await screen.findByRole("link", { name: "通知管理，3 条未读" });
  expect(link.textContent).toBe("3");
  expect(String(vi.mocked(fetch).mock.calls[0][0])).toBe("/api/v1/operators/one/notifications/unread-count");
});
