/** @vitest-environment jsdom */
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { withZh } from "@/lib/test-i18n";
import type { Viewer } from "@/lib/rbac";
import { ConsoleAccess } from "./console-access";
const state = vi.hoisted(() => ({ path: "/admin", viewer: { signedIn: true, loading: false, roles: ["end_user"] } as Viewer }));
vi.mock("next/navigation", () => ({ usePathname: () => state.path }));
vi.mock("./viewer-context", () => ({ useViewer: () => state.viewer }));
afterEach(cleanup);
function show(viewer: Viewer, path = "/admin") {
  state.viewer = viewer;
  state.path = path;
  render(withZh(<ConsoleAccess><div>protected data</div></ConsoleAccess>));
}
describe("console access", () => {
  it("does not mount protected data while identity is loading", () => {
    show({ signedIn: false, loading: true, roles: [] });
    expect(screen.queryByText("protected data")).toBeNull();
    expect(screen.getByRole("status").textContent).toBe("正在加载");
  });
  it("asks a guest to sign in and preserves the destination", () => {
    show({ signedIn: false, loading: false, roles: [] }, "/app/keys");
    expect(screen.getByRole("link").getAttribute("href")).toBe("/login?next=%2Fapp%2Fkeys");
    expect(screen.queryByText("protected data")).toBeNull();
  });
  it("denies an end user the admin console with a route back home", () => {
    show({ signedIn: true, loading: false, roles: ["end_user"] });
    expect(screen.getByRole("alert").textContent).toBe("没有权限查看");
    expect(screen.getByRole("link").getAttribute("href")).toBe("/app");
    expect(screen.queryByText("protected data")).toBeNull();
  });
  it("denies finance access to provider credentials", () => {
    show({ signedIn: true, loading: false, roles: ["finance_admin"] }, "/admin/providers/private");
    expect(screen.queryByText("protected data")).toBeNull();
    expect(screen.getByRole("link").getAttribute("href")).toBe("/admin");
  });
  it.each([
    ["/admin/billing", ["finance_admin"], false],
    ["/channel/ledger", ["channel_admin"], false],
    ["/partner/users", ["end_user"], true],
    ["/app/keys", ["end_user"], false],
  ] as const)("allows the authorized portal %s", (path, roles, isPartner) => {
    show({ signedIn: true, loading: false, roles: [...roles], isPartner }, path);
    expect(screen.getByText("protected data")).toBeTruthy();
  });
  it("offers retry for identity failures instead of pretending the user logged out", () => {
    show({ signedIn: false, loading: false, roles: [], error: true });
    expect(screen.getByRole("button", { name: "重试" })).toBeTruthy();
    expect(screen.queryByRole("link")).toBeNull();
  });
  it("keeps a partner lookup outage from blocking the user's own console", () => {
    show({ signedIn: true, loading: false, roles: ["end_user"], partnerError: true }, "/app");
    expect(screen.getByText("protected data")).toBeTruthy();
  });
});
