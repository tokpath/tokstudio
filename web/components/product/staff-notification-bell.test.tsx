// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { StaffNotificationBell } from "./staff-notification-bell";

afterEach(cleanup);

it("renders a bell without text beside the signed-in account", () => {
  const workspaces = [{ operator: { id: "one", kind: "platform" }, permissions: ["notifications.read"] }];
  const view = render(<StaffNotificationBell workspaces={workspaces} pathname="/manage/one/payments" />);
  const link = view.getByRole("link", { name: "通知管理" });
  expect(link.getAttribute("href")).toBe("/manage/one/notifications");
  expect(link.textContent).toBe("");
  expect(link.querySelector("svg")).not.toBeNull();
  view.unmount();
  render(<StaffNotificationBell workspaces={[{ operator: { id: "one", kind: "channel" }, permissions: ["payments.read"] }]} pathname="/manage/one/payments" />);
  expect(screen.queryByRole("link", { name: "通知管理" })).toBeNull();
});
