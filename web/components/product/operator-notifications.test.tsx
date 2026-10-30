// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import type { Entity, ResourcePage } from "@/lib/contracts";
import { OperatorNotificationGroups } from "./operator-notifications";

afterEach(() => { cleanup(); vi.unstubAllGlobals(); });
const notice = (id: string, read: string, name: string): Record<string, string | boolean> => ({ id, title: "模型与计价计划变更", body: "价格将于明天生效。", recipient_name: name, recipient_email: `${id}@example.test`, recipient_id: id, read_status: read, state: "pending", created_at: "2026-10-30T08:33:00Z", system_generated: true });
const page = { items: [
  { id: "personal", status: "grouped", values: { record_kind: "personal", unread_count: 1, total_count: 2, notices: [notice("mine-unread", "unread", ""), notice("mine-read", "read", "")] } },
  { id: "catalog:one", status: "grouped", values: { record_kind: "event", title: "模型与计价计划变更", body: "价格将于明天生效。", system_generated: true, created_at: "2026-10-30T08:33:00Z", unread_count: 1, total_count: 2, notices: [notice("person-unread", "unread", "未读的人"), notice("person-read", "read", "已读的人")] } },
] as Entity[], row_operations: [] } as Pick<ResourcePage, "items" | "row_operations">;

it("shows my unread count and each send as its own group, with unread recipients first", () => {
  render(<OperatorNotificationGroups page={page} onRetry={() => {}} />);
  expect(screen.getByText("未读 1 条")).toBeTruthy();
  expect(screen.getByText("未读 1 人 / 共 2 人")).toBeTruthy();
  expect(screen.getAllByRole("button", { name: "查看详情" })).toHaveLength(2);
  fireEvent.click(screen.getAllByRole("button", { name: "查看详情" })[1]);
  const dialog = screen.getByRole("dialog");
  const names = within(dialog).getAllByRole("heading", { level: 3 }).map(item => item.textContent);
  expect(names).toEqual(["未读的人", "已读的人"]);
  expect(within(dialog).getAllByText("邮件尚未发送")).toHaveLength(2);
  expect(within(dialog).queryByText("待处理")).toBeNull();
  expect(within(dialog).queryByRole("button", { name: "标为已读" })).toBeNull();
});

it("marks an unread notice addressed to me as read", async () => {
  const fetch = vi.fn(async () => new Response(JSON.stringify({ id: "mine-unread", status: "updated" }), { status: 200 }));
  vi.stubGlobal("fetch", fetch);
  const cache = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(<QueryClientProvider client={cache}><OperatorNotificationGroups page={page} operatorID="operator-one" onRetry={() => {}} /></QueryClientProvider>);
  fireEvent.click(screen.getAllByRole("button", { name: "查看详情" })[0]);
  const dialog = screen.getByRole("dialog");
  expect(within(dialog).getAllByRole("button", { name: "标为已读" })).toHaveLength(1);
  fireEvent.click(within(dialog).getByRole("button", { name: "标为已读" }));
  await waitFor(() => expect(fetch).toHaveBeenCalled());
  expect(String(fetch.mock.calls[0]?.[0])).toBe("/api/v1/operators/operator-one/notifications/mine-unread/read");
  expect(fetch.mock.calls[0]?.[1]?.method).toBe("POST");
  await waitFor(() => expect(within(dialog).queryByRole("button", { name: "标为已读" })).toBeNull());
  expect(screen.getByText("未读 0 条")).toBeTruthy();
});
