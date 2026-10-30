// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import type { Entity, ResourcePage } from "@/lib/contracts";
import { OperatorNotificationGroups } from "./operator-notifications";

afterEach(cleanup);
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
});
